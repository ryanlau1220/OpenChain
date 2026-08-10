package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

const (
	traceWorkerPollInterval = time.Second
	traceJobTimeout         = 10 * time.Minute
	traceJobLease           = 11 * time.Minute
)

type Queue struct {
	engine   *Engine
	database *db.DB
	done     chan struct{}
	start    sync.Once
}

func NewQueue(engine *Engine, database *db.DB) *Queue {
	return &Queue{engine: engine, database: database}
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
	job, err := q.database.EnqueueTraceJob(ctx, db.TraceJobQuery{Network: "ethereum-mainnet", Address: address, Direction: string(direction), Cursor: cursor, Limit: limit}, retry)
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

func (q *Queue) runOnce(ctx context.Context) {
	if err := q.database.RecoverExpiredTraceJobs(ctx); err != nil {
		slog.Error("recover trace jobs", "error", err)
		return
	}
	job, err := q.database.ClaimTraceJob(ctx, traceJobLease)
	if err != nil {
		slog.Error("claim trace job", "error", err)
		return
	}
	if job == nil {
		return
	}

	jobContext, cancel := context.WithTimeout(ctx, traceJobTimeout)
	result, err := q.engine.ResolveGraph(jobContext, job.Query.Address, Direction(job.Query.Direction), job.Query.Limit, job.Query.Cursor)
	cancel()
	if err != nil {
		slog.Warn("trace job failed", "job_id", job.ID, "error", err)
		if failErr := q.database.FailTraceJob(ctx, job.ID, "Trace retrieval did not complete. Search again to retry."); failErr != nil {
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
