package adapter

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RawAcquisition is an exact provider response captured while resolving a trace.
// RequestIdentity intentionally excludes credentials.
type RawAcquisition struct {
	Provider        string
	RequestIdentity string
	Response        []byte
	RetrievedAt     time.Time
}

type acquisitionRecorder struct {
	mu    sync.Mutex
	items []RawAcquisition
}

type acquisitionContextKey struct{}

func WithAcquisitionRecorder(ctx context.Context) (context.Context, *acquisitionRecorder) {
	recorder := &acquisitionRecorder{}
	return context.WithValue(ctx, acquisitionContextKey{}, recorder), recorder
}

func (r *acquisitionRecorder) Items() []RawAcquisition {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]RawAcquisition, len(r.items))
	copy(items, r.items)
	return items
}

func recordAcquisition(ctx context.Context, provider string, request *http.Request, response []byte) {
	recorder, _ := ctx.Value(acquisitionContextKey{}).(*acquisitionRecorder)
	if recorder == nil {
		return
	}
	url := *request.URL
	query := url.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "signature") {
			query.Set(key, "redacted")
		}
	}
	url.RawQuery = query.Encode()
	identity := request.Method + " " + url.String()
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err == nil {
			defer body.Close()
			if value, readErr := io.ReadAll(io.LimitReader(body, 1<<20)); readErr == nil {
				hash := sha256.Sum256(value)
				identity += " body-sha256=" + fmt.Sprintf("%x", hash[:])
			}
		}
	}
	recorder.mu.Lock()
	recorder.items = append(recorder.items, RawAcquisition{Provider: provider, RequestIdentity: identity, Response: append([]byte(nil), response...), RetrievedAt: time.Now().UTC()})
	recorder.mu.Unlock()
}
