package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
)

const (
	traceWorkerPollInterval  = time.Second
	traceJobTimeout          = time.Minute
	traceJobLease            = 2 * time.Minute
	traceProviderMaxAttempts = 3
	traceJobRetention        = 24 * time.Hour
	traceJobPruneInterval    = 5 * time.Minute
)

var ErrQueueFull = db.ErrTraceQueueFull
var ErrClientQueueFull = db.ErrTraceClientQueueFull
var ErrTraceNotFound = db.ErrTraceJobNotFound

type Queue struct {
	engine             *Engine
	database           *db.DB
	maxQueued          int
	maxQueuedPerClient int
	done               chan struct{}
	start              sync.Once
	lastPrunedAt       time.Time
}

func NewQueue(engine *Engine, database *db.DB, maxQueued, maxQueuedPerClient int) *Queue {
	return &Queue{engine: engine, database: database, maxQueued: maxQueued, maxQueuedPerClient: maxQueuedPerClient}
}

func (q *Queue) Start(ctx context.Context) {
	q.start.Do(func() {
		q.done = make(chan struct{})
		// Workers are sequential per network; each adapter enforces the provider's
		// own request gap, so completed jobs do not wait for the next poll tick.
		go func() {
			defer close(q.done)
			if q.database == nil {
				return
			}
			for {
				q.pruneFinished(ctx)
				if q.runOnce(ctx) {
					continue
				}
				timer := time.NewTimer(traceWorkerPollInterval)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
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

func (q *Queue) TraceGraph(ctx context.Context, address string, direction Direction, limit uint32, cursor string, maxCounterparties uint32, ranking Ranking, maxDepth uint32, retry bool, clientKey string) (*GraphResult, error) {
	if q.database == nil {
		return q.engine.ResolveGraph(ctx, address, direction, limit, cursor, maxCounterparties, ranking, maxDepth)
	}
	maxCounterparties, ranking = graphControls(maxCounterparties, ranking)
	maxDepth = graphDepth(maxDepth)
	job, err := q.database.EnqueueTraceJob(ctx, db.TraceJobQuery{Network: q.engine.Network(), Address: address, Direction: string(direction), Cursor: cursor, Limit: limit, CounterpartyLimit: maxCounterparties, Ranking: string(ranking), MaxDepth: maxDepth, ClientKey: clientKey}, retry, q.maxQueued, q.maxQueuedPerClient)
	if err != nil {
		return nil, fmt.Errorf("queue trace job: %w", err)
	}
	return q.resultForJob(address, job)
}

// TraceStatus reads an existing durable job. It is deliberately separate from
// TraceGraph so client polling cannot create queue work or consume work budget.
func (q *Queue) TraceStatus(ctx context.Context, address string, direction Direction, limit uint32, cursor string, maxCounterparties uint32, ranking Ranking, maxDepth uint32) (*GraphResult, error) {
	if q.database == nil {
		return nil, ErrTraceNotFound
	}
	maxCounterparties, ranking = graphControls(maxCounterparties, ranking)
	maxDepth = graphDepth(maxDepth)
	job, err := q.database.TraceJob(ctx, db.TraceJobQuery{Network: q.engine.Network(), Address: address, Direction: string(direction), Cursor: cursor, Limit: limit, CounterpartyLimit: maxCounterparties, Ranking: string(ranking), MaxDepth: maxDepth})
	if err != nil {
		return nil, fmt.Errorf("get trace job: %w", err)
	}
	return q.resultForJob(address, job)
}

func (q *Queue) resultForJob(address string, job *db.TraceJob) (*GraphResult, error) {
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
		return q.engine.PendingGraph(address, "Trace retrieval is in progress."), nil
	default:
		return q.engine.PendingGraph(address, "Trace retrieval is queued and will begin shortly."), nil
	}
}

type Stats struct {
	Enabled             bool    `json:"enabled"`
	Queued              int64   `json:"queued"`
	Running             int64   `json:"running"`
	Failed              int64   `json:"failed"`
	OldestQueuedSeconds float64 `json:"oldest_queued_seconds"`
}

func (q *Queue) Stats(ctx context.Context) (Stats, error) {
	if q == nil || q.database == nil {
		return Stats{}, nil
	}
	stats, err := q.database.TraceJobStats(ctx, q.engine.Network())
	return Stats{Enabled: true, Queued: stats.Queued, Running: stats.Running, Failed: stats.Failed, OldestQueuedSeconds: stats.OldestQueuedSeconds}, err
}

// Capacity is the durable per-network queue limit.
func (q *Queue) Capacity() int {
	if q == nil {
		return 0
	}
	return q.maxQueued
}

func (q *Queue) pruneFinished(ctx context.Context) {
	if time.Since(q.lastPrunedAt) < traceJobPruneInterval {
		return
	}
	q.lastPrunedAt = time.Now()
	if err := q.database.PruneFinishedTraceJobs(ctx, q.engine.Network(), traceJobRetention); err != nil {
		slog.Error("prune finished trace jobs", "error", err)
	}
}

func (q *Queue) runOnce(ctx context.Context) bool {
	if err := q.database.RecoverExpiredTraceJobs(ctx, q.engine.Network()); err != nil {
		slog.Error("recover trace jobs", "error", err)
		return false
	}
	job, err := q.database.ClaimTraceJob(ctx, q.engine.Network(), traceJobLease)
	if err != nil {
		slog.Error("claim trace job", "error", err)
		return false
	}
	if job == nil {
		return false
	}

	jobContext, cancel := context.WithTimeout(ctx, traceJobTimeout)
	result, err := q.resolveWithRetry(jobContext, job)
	interrupted := errors.Is(jobContext.Err(), context.Canceled)
	cancel()
	if err != nil {
		writeContext, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer writeCancel()
		if interrupted {
			if requeueErr := q.database.RequeueTraceJob(writeContext, job.ID); requeueErr != nil {
				slog.Error("requeue interrupted trace job", "job_id", job.ID, "error", requeueErr)
			}
			return true
		}
		slog.Warn("trace_job_failed", "job_id", job.ID, "network", job.Query.Network, "error", err)
		message := "Trace retrieval failed: " + err.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		if failErr := q.database.FailTraceJob(writeContext, job.ID, message); failErr != nil {
			slog.Error("mark trace job failed", "job_id", job.ID, "error", failErr)
		}
		return true
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		slog.Error("encode trace job result", "job_id", job.ID, "error", err)
		_ = q.database.FailTraceJob(ctx, job.ID, "Trace retrieval did not complete. Search again to retry.")
		return true
	}
	if err := q.database.CompleteTraceJob(ctx, job.ID, encoded); err != nil {
		slog.Error("complete trace job", "job_id", job.ID, "error", err)
		return true
	}
	slog.Info("trace_job_completed", "job_id", job.ID, "network", job.Query.Network, "nodes", result.TotalNodes, "edges", result.TotalEdges)
	return true
}

func (q *Queue) resolveWithRetry(ctx context.Context, job *db.TraceJob) (*GraphResult, error) {
	for attempt := 1; attempt <= traceProviderMaxAttempts; attempt++ {
		result, err := q.engine.ResolveGraph(ctx, job.Query.Address, Direction(job.Query.Direction), job.Query.Limit, job.Query.Cursor, job.Query.CounterpartyLimit, Ranking(job.Query.Ranking), job.Query.MaxDepth)
		if err == nil {
			return result, nil
		}
		delay, retry := adapter.RetryDelay(err, attempt)
		if !retry || attempt == traceProviderMaxAttempts {
			return nil, err
		}
		slog.Warn("trace_provider_retry", "job_id", job.ID, "network", job.Query.Network, "attempt", attempt, "delay", delay, "error", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, errors.New("trace provider retry attempts exhausted")
}
