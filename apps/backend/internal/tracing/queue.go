package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

const (
	traceWorkerPollInterval = time.Second
	traceJobTimeout         = time.Minute
	traceJobLease           = 2 * time.Minute
)

var ErrQueueFull = db.ErrTraceQueueFull

type Queue struct {
	engine    *Engine
	database  *db.DB
	maxQueued int
	done      chan struct{}
	start     sync.Once
}

func NewQueue(engine *Engine, database *db.DB, maxQueued int) *Queue {
	return &Queue{engine: engine, database: database, maxQueued: maxQueued}
}

func (q *Queue) Start(ctx context.Context) {
	q.start.Do(func() {
		q.done = make(chan struct{})
		go func() {
			defer close(q.done)
			if q.database == nil {
				return
			}
			q.runOnce(ctx)
			ticker := time.NewTicker(traceWorkerPollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					q.runOnce(ctx)
				}
			}
		}()
	})
}

func (q *Queue) Wait() {
	if q.done != nil {
		<-q.done
	}
}

func (q *Queue) TraceGraph(ctx context.Context, address string, direction Direction, limit uint32, cursor string, retry bool) (*GraphResult, error) {
	if q.database == nil {
		return q.engine.ResolveGraph(ctx, address, direction, limit, cursor)
	}
	job, err := q.database.EnqueueTraceJob(ctx, db.TraceJobQuery{Network: q.engine.Network(), Address: address, Direction: string(direction), Cursor: cursor, Limit: limit}, retry, q.maxQueued)
	if err != nil {
		return nil, fmt.Errorf("queue trace job: %w", err)
	}
	switch job.Status {
	case "succeeded":
		var result GraphResult
		if err := json.Unmarshal(job.Result, &result); err != nil {
			return nil, fmt.Errorf("decode trace job result: %w", err)
		}
		return &result, nil
	case "failed":
		result := q.engine.PendingGraph(address, "Trace retrieval did not complete. Search again to retry.")
		result.Pending = false
		return result, nil
	case "running":
		return q.engine.PendingGraph(address, "Trace retrieval is in progress. This address may need initial index data."), nil
	default:
		return q.engine.PendingGraph(address, "Trace retrieval is queued and will begin shortly."), nil
	}
}

type Stats struct {
	Enabled bool  `json:"enabled"`
	Queued  int64 `json:"queued"`
	Running int64 `json:"running"`
	Failed  int64 `json:"failed"`
}

func (q *Queue) Stats(ctx context.Context) (Stats, error) {
	if q == nil || q.database == nil {
		return Stats{}, nil
	}
	stats, err := q.database.TraceJobStats(ctx, q.engine.Network())
	return Stats{Enabled: true, Queued: stats.Queued, Running: stats.Running, Failed: stats.Failed}, err
}

func (q *Queue) runOnce(ctx context.Context) {
	if err := q.database.RecoverExpiredTraceJobs(ctx, q.engine.Network()); err != nil {
		slog.Error("recover trace jobs", "error", err)
		return
	}
	job, err := q.database.ClaimTraceJob(ctx, q.engine.Network(), traceJobLease)
	if err != nil {
		slog.Error("claim trace job", "error", err)
		return
	}
	if job == nil {
		return
	}

	jobContext, cancel := context.WithTimeout(ctx, traceJobTimeout)
	result, err := q.engine.ResolveGraph(jobContext, job.Query.Address, Direction(job.Query.Direction), job.Query.Limit, job.Query.Cursor)
	interrupted := errors.Is(jobContext.Err(), context.Canceled)
	cancel()
	if err != nil {
		writeContext, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer writeCancel()
		if interrupted {
			if requeueErr := q.database.RequeueTraceJob(writeContext, job.ID); requeueErr != nil {
				slog.Error("requeue interrupted trace job", "job_id", job.ID, "error", requeueErr)
			}
			return
		}
		slog.Warn("trace job failed", "job_id", job.ID, "error", err)
		message := "Trace retrieval failed: " + err.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		if failErr := q.database.FailTraceJob(writeContext, job.ID, message); failErr != nil {
			slog.Error("mark trace job failed", "job_id", job.ID, "error", failErr)
		}
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		slog.Error("encode trace job result", "job_id", job.ID, "error", err)
		_ = q.database.FailTraceJob(ctx, job.ID, "Trace retrieval did not complete. Search again to retry.")
		return
	}
	if err := q.database.CompleteTraceJob(ctx, job.ID, encoded); err != nil {
		slog.Error("complete trace job", "job_id", job.ID, "error", err)
	}
}
