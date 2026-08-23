package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

const traceResultCacheTTL = 5 * time.Minute
const traceFailureWindow = 15 * time.Minute

var (
	ErrTraceQueueFull       = errors.New("trace queue is full")
	ErrTraceClientQueueFull = errors.New("client trace queue is full")
	ErrTraceJobNotFound     = errors.New("trace job not found")
)

type TraceJobQuery struct {
	Network, Address, Direction, Cursor string
	ClientKey                           string
	Limit                               uint32
	CounterpartyLimit                   uint32
	Ranking                             string
	MaxDepth                            uint32
}

type TraceJob struct {
	ID           int64
	Query        TraceJobQuery
	Status       string
	Result       []byte
	ErrorMessage string
}

func (d *DB) EnqueueTraceJob(ctx context.Context, query TraceJobQuery, retry bool, maxQueued, maxQueuedPerClient int) (*TraceJob, error) {
	query = normalizeTraceJobQuery(query)
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// ponytail: per-network lock preserves a strict bounded queue; replace it
	// with token rows only if one network's enqueue rate becomes a bottleneck.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 807841))`, query.Network); err != nil {
		return nil, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM trace_jobs WHERE network = $1 AND address = $2 AND direction = $3 AND cursor = $4 AND page_size = $5 AND counterparty_limit = $6 AND ranking = $7 AND max_depth = $8)`, query.Network, query.Address, query.Direction, query.Cursor, query.Limit, query.CounterpartyLimit, query.Ranking, query.MaxDepth).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		var queued int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM trace_jobs WHERE network = $1 AND status = 'queued'`, query.Network).Scan(&queued); err != nil {
			return nil, err
		}
		if queued >= maxQueued {
			return nil, ErrTraceQueueFull
		}
		var clientQueued int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM trace_jobs WHERE network = $1 AND status = 'queued' AND client_key = $2`, query.Network, query.ClientKey).Scan(&clientQueued); err != nil {
			return nil, err
		}
		if clientQueued >= maxQueuedPerClient {
			return nil, ErrTraceClientQueueFull
		}
	}
	const statement = `INSERT INTO trace_jobs (network, address, direction, cursor, page_size, counterparty_limit, ranking, max_depth, client_key, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'queued')
ON CONFLICT (network, address, direction, cursor, page_size, counterparty_limit, ranking, max_depth) DO UPDATE
SET status = CASE
  WHEN ($11 AND trace_jobs.status = 'failed') OR (trace_jobs.status = 'succeeded' AND trace_jobs.completed_at < now() - $10::interval) THEN 'queued'
  ELSE trace_jobs.status
END,
result_json = CASE
  WHEN ($11 AND trace_jobs.status = 'failed') OR (trace_jobs.status = 'succeeded' AND trace_jobs.completed_at < now() - $10::interval) THEN NULL
  ELSE trace_jobs.result_json
END,
error_message = CASE
  WHEN ($11 AND trace_jobs.status = 'failed') OR (trace_jobs.status = 'succeeded' AND trace_jobs.completed_at < now() - $10::interval) THEN NULL
  ELSE trace_jobs.error_message
END,
completed_at = CASE
  WHEN ($11 AND trace_jobs.status = 'failed') OR (trace_jobs.status = 'succeeded' AND trace_jobs.completed_at < now() - $10::interval) THEN NULL
  ELSE trace_jobs.completed_at
END,
updated_at = now()
RETURNING id, network, address, direction, cursor, page_size, counterparty_limit, ranking, max_depth, status, result_json, COALESCE(error_message, '')`
	job, err := scanTraceJob(tx.QueryRowContext(ctx, statement, query.Network, query.Address, query.Direction, query.Cursor, query.Limit, query.CounterpartyLimit, query.Ranking, query.MaxDepth, query.ClientKey, traceResultCacheTTL.String(), retry))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return job, nil
}

func (d *DB) TraceJob(ctx context.Context, query TraceJobQuery) (*TraceJob, error) {
	query = normalizeTraceJobQuery(query)
	const statement = `SELECT id, network, address, direction, cursor, page_size, counterparty_limit, ranking, max_depth, status, result_json, COALESCE(error_message, '')
FROM trace_jobs
WHERE network = $1 AND address = $2 AND direction = $3 AND cursor = $4 AND page_size = $5 AND counterparty_limit = $6 AND ranking = $7 AND max_depth = $8`
	job, err := scanTraceJob(d.SQL.QueryRowContext(ctx, statement, query.Network, query.Address, query.Direction, query.Cursor, query.Limit, query.CounterpartyLimit, query.Ranking, query.MaxDepth))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTraceJobNotFound
	}
	return job, err
}

type TraceJobStats struct {
	Queued              int64
	Running             int64
	Failed              int64
	OldestQueuedSeconds float64
}

func (d *DB) TraceJobStats(ctx context.Context, network string) (TraceJobStats, error) {
	stats := TraceJobStats{}
	err := d.SQL.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE status = 'queued'), count(*) FILTER (WHERE status = 'running'), count(*) FILTER (WHERE status = 'failed' AND completed_at >= now() - $2::interval), COALESCE(EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE status = 'queued')), 0) FROM trace_jobs WHERE network = $1`, network, traceFailureWindow.String()).Scan(&stats.Queued, &stats.Running, &stats.Failed, &stats.OldestQueuedSeconds)
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
RETURNING trace_jobs.id, trace_jobs.network, trace_jobs.address, trace_jobs.direction, trace_jobs.cursor, trace_jobs.page_size, trace_jobs.counterparty_limit, trace_jobs.ranking, trace_jobs.max_depth, trace_jobs.status, trace_jobs.result_json, COALESCE(trace_jobs.error_message, '')`
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
	_, err := d.SQL.ExecContext(ctx, `UPDATE trace_jobs SET status = 'failed', error_message = $2, lease_expires_at = NULL, completed_at = now(), updated_at = now() WHERE id = $1 AND status = 'running'`, id, message)
	return err
}

func (d *DB) PruneFinishedTraceJobs(ctx context.Context, network string, retention time.Duration) error {
	_, err := d.SQL.ExecContext(ctx, `DELETE FROM trace_jobs WHERE network = $1 AND status IN ('succeeded', 'failed') AND completed_at < now() - $2::interval`, network, retention.String())
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
	if err := row.Scan(&job.ID, &job.Query.Network, &job.Query.Address, &job.Query.Direction, &job.Query.Cursor, &job.Query.Limit, &job.Query.CounterpartyLimit, &job.Query.Ranking, &job.Query.MaxDepth, &job.Status, &job.Result, &job.ErrorMessage); err != nil {
		return nil, err
	}
	return job, nil
}

func normalizeTraceJobQuery(query TraceJobQuery) TraceJobQuery {
	if query.CounterpartyLimit == 0 {
		query.CounterpartyLimit = 10
	}
	if query.Ranking == "" {
		query.Ranking = "most_recent"
	}
	if query.MaxDepth == 0 {
		query.MaxDepth = 1
	}
	return query
}

func isUniqueViolation(err error) bool {
	var databaseError *pq.Error
	return errors.As(err, &databaseError) && databaseError.Code == "23505"
}
