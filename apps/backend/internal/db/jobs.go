package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

const traceResultCacheTTL = 5 * time.Minute

var (
	ErrTraceQueueFull       = errors.New("trace queue is full")
	ErrTraceClientQueueFull = errors.New("client trace queue is full")
	ErrTraceJobNotFound     = errors.New("trace job not found")
)

type TraceJobQuery struct {
	Network, Address, Direction, Cursor string
	ClientKey                           string
	Limit                               uint32
}

type TraceJob struct {
	ID           int64
	Query        TraceJobQuery
	Status       string
	Result       []byte
	ErrorMessage string
}

func (d *DB) EnqueueTraceJob(ctx context.Context, query TraceJobQuery, retry bool, maxQueued, maxQueuedPerClient int) (*TraceJob, error) {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(807_841)); err != nil {
		return nil, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM trace_jobs WHERE network = $1 AND address = $2 AND direction = $3 AND cursor = $4 AND page_size = $5)`, query.Network, query.Address, query.Direction, query.Cursor, query.Limit).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		var queued int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM trace_jobs WHERE status = 'queued'`).Scan(&queued); err != nil {
			return nil, err
		}
		if queued >= maxQueued {
			return nil, ErrTraceQueueFull
		}
		var clientQueued int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM trace_jobs WHERE status = 'queued' AND client_key = $1`, query.ClientKey).Scan(&clientQueued); err != nil {
			return nil, err
		}
		if clientQueued >= maxQueuedPerClient {
			return nil, ErrTraceClientQueueFull
		}
	}
	const statement = `INSERT INTO trace_jobs (network, address, direction, cursor, page_size, client_key, status)
VALUES ($1, $2, $3, $4, $5, $6, 'queued')
ON CONFLICT (network, address, direction, cursor, page_size) DO UPDATE
SET status = CASE
  WHEN ($8 AND trace_jobs.status = 'failed') OR (trace_jobs.status = 'succeeded' AND trace_jobs.completed_at < now() - $7::interval) THEN 'queued'
  ELSE trace_jobs.status
END,
result_json = CASE
  WHEN ($8 AND trace_jobs.status = 'failed') OR (trace_jobs.status = 'succeeded' AND trace_jobs.completed_at < now() - $7::interval) THEN NULL
  ELSE trace_jobs.result_json
END,
error_message = CASE
  WHEN ($8 AND trace_jobs.status = 'failed') OR (trace_jobs.status = 'succeeded' AND trace_jobs.completed_at < now() - $7::interval) THEN NULL
  ELSE trace_jobs.error_message
END,
completed_at = CASE
  WHEN ($8 AND trace_jobs.status = 'failed') OR (trace_jobs.status = 'succeeded' AND trace_jobs.completed_at < now() - $7::interval) THEN NULL
  ELSE trace_jobs.completed_at
END,
updated_at = now()
RETURNING id, network, address, direction, cursor, page_size, status, result_json, COALESCE(error_message, '')`
	job, err := scanTraceJob(tx.QueryRowContext(ctx, statement, query.Network, query.Address, query.Direction, query.Cursor, query.Limit, query.ClientKey, traceResultCacheTTL.String(), retry))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return job, nil
}

func (d *DB) TraceJob(ctx context.Context, query TraceJobQuery) (*TraceJob, error) {
	const statement = `SELECT id, network, address, direction, cursor, page_size, status, result_json, COALESCE(error_message, '')
FROM trace_jobs
WHERE network = $1 AND address = $2 AND direction = $3 AND cursor = $4 AND page_size = $5`
	job, err := scanTraceJob(d.SQL.QueryRowContext(ctx, statement, query.Network, query.Address, query.Direction, query.Cursor, query.Limit))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTraceJobNotFound
	}
	return job, err
}

type TraceJobStats struct {
	Queued  int64
	Running int64
	Failed  int64
}

func (d *DB) TraceJobStats(ctx context.Context, network string) (TraceJobStats, error) {
	stats := TraceJobStats{}
	err := d.SQL.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE status = 'queued'), count(*) FILTER (WHERE status = 'running'), count(*) FILTER (WHERE status = 'failed') FROM trace_jobs WHERE network = $1`, network).Scan(&stats.Queued, &stats.Running, &stats.Failed)
	return stats, err
}

func (d *DB) RecoverExpiredTraceJobs(ctx context.Context, network string) error {
	_, err := d.SQL.ExecContext(ctx, `UPDATE trace_jobs SET status = 'queued', lease_expires_at = NULL, updated_at = now() WHERE network = $1 AND status = 'running' AND lease_expires_at < now()`, network)
	return err
}

func (d *DB) ClaimTraceJob(ctx context.Context, network string, lease time.Duration) (*TraceJob, error) {
	const statement = `WITH next_job AS (
  SELECT id FROM trace_jobs
  WHERE network = $1 AND status = 'queued'
  ORDER BY created_at
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE trace_jobs
SET status = 'running', lease_expires_at = now() + $2::interval, updated_at = now()
FROM next_job
WHERE trace_jobs.id = next_job.id
RETURNING trace_jobs.id, trace_jobs.network, trace_jobs.address, trace_jobs.direction, trace_jobs.cursor, trace_jobs.page_size, trace_jobs.status, trace_jobs.result_json, COALESCE(trace_jobs.error_message, '')`
	job, err := scanTraceJob(d.SQL.QueryRowContext(ctx, statement, network, lease.String()))
	if errors.Is(err, sql.ErrNoRows) || isUniqueViolation(err) {
		return nil, nil
	}
	return job, err
}

func (d *DB) CompleteTraceJob(ctx context.Context, id int64, result []byte) error {
	_, err := d.SQL.ExecContext(ctx, `UPDATE trace_jobs SET status = 'succeeded', result_json = $2::jsonb, error_message = NULL, lease_expires_at = NULL, completed_at = now(), updated_at = now() WHERE id = $1 AND status = 'running'`, id, result)
	return err
}

func (d *DB) FailTraceJob(ctx context.Context, id int64, message string) error {
	_, err := d.SQL.ExecContext(ctx, `UPDATE trace_jobs SET status = 'failed', error_message = $2, lease_expires_at = NULL, updated_at = now() WHERE id = $1 AND status = 'running'`, id, message)
	return err
}

func (d *DB) RequeueTraceJob(ctx context.Context, id int64) error {
	_, err := d.SQL.ExecContext(ctx, `UPDATE trace_jobs SET status = 'queued', lease_expires_at = NULL, updated_at = now() WHERE id = $1 AND status = 'running'`, id)
	return err
}

type traceJobScanner interface {
	Scan(...any) error
}

func scanTraceJob(row traceJobScanner) (*TraceJob, error) {
	job := &TraceJob{}
	if err := row.Scan(&job.ID, &job.Query.Network, &job.Query.Address, &job.Query.Direction, &job.Query.Cursor, &job.Query.Limit, &job.Status, &job.Result, &job.ErrorMessage); err != nil {
		return nil, err
	}
	return job, nil
}

func isUniqueViolation(err error) bool {
	var databaseError *pq.Error
	return errors.As(err, &databaseError) && databaseError.Code == "23505"
}
