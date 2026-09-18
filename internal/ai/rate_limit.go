package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iotatfan/sora-go/internal/config"
	"github.com/openai/openai-go/v3/responses"
)

type modelRateLimiter struct {
	semaphore chan struct{}
	mu        sync.Mutex
	next      time.Time
	blocked   time.Time
	interval  time.Duration
	quotaTTL  time.Duration
}

func logResponseUsage(kind string, response *responses.Response) {
	if response == nil {
		return
	}
	usage := response.Usage
	fmt.Printf("model_usage kind=%s model=%s input_tokens=%d output_tokens=%d cached_input_tokens=%d\n", kind, response.Model, usage.InputTokens, usage.OutputTokens, usage.InputTokensDetails.CachedTokens)
}

func logEmbeddingUsage(kind, model string, promptTokens, totalTokens int64) {
	fmt.Printf("model_usage kind=%s model=%s input_tokens=%d total_tokens=%d\n", kind, model, promptTokens, totalTokens)
}

func newModelRateLimiter(cfg *config.Config) *modelRateLimiter {
	concurrent, intervalMS, quotaSeconds := 2, 250, 60
	if cfg != nil {
		if cfg.AI.Runtime.ModelMaxConcurrent > 0 {
			concurrent = cfg.AI.Runtime.ModelMaxConcurrent
		}
		if cfg.AI.Runtime.ModelMinIntervalMS > 0 {
			intervalMS = cfg.AI.Runtime.ModelMinIntervalMS
		}
		if cfg.AI.Runtime.ModelQuotaCooldown > 0 {
			quotaSeconds = cfg.AI.Runtime.ModelQuotaCooldown
		}
	}
	return &modelRateLimiter{
		semaphore: make(chan struct{}, concurrent),
		interval:  time.Duration(intervalMS) * time.Millisecond,
		quotaTTL:  time.Duration(quotaSeconds) * time.Second,
	}
}

func (l *modelRateLimiter) acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	if time.Now().Before(l.blocked) {
		until := l.blocked
		l.mu.Unlock()
		return fmt.Errorf("model requests paused until %s after quota/rate-limit response", until.Format(time.RFC3339))
	}
	l.mu.Unlock()

	select {
	case l.semaphore <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	l.mu.Lock()
	wait := time.Until(l.next)
	if wait < 0 {
		wait = 0
	}
	l.next = time.Now().Add(wait + l.interval)
	l.mu.Unlock()
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			l.release()
			return ctx.Err()
		}
	}
	return nil
}

func (l *modelRateLimiter) release() {
	if l != nil {
		<-l.semaphore
	}
}

func (l *modelRateLimiter) recordFailure(err error) {
	if l == nil || err == nil {
		return
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "quota") || strings.Contains(s, "rate limit") || strings.Contains(s, "too many requests") {
		l.mu.Lock()
		l.blocked = time.Now().Add(l.quotaTTL)
		l.mu.Unlock()
	}
}

func (h *AIHandler) runModelCall(ctx context.Context, kind string, fn func() error) error {
	started := time.Now()
	if err := h.modelLimiter.acquire(ctx); err != nil {
		fmt.Printf("model_request kind=%s status=admission_rejected duration_ms=%d error=%v\n", kind, time.Since(started).Milliseconds(), err)
		return err
	}
	defer h.modelLimiter.release()
	err := fn()
	h.modelLimiter.recordFailure(err)
	status := "ok"
	if err != nil {
		status = "error"
		fmt.Printf("model_request kind=%s status=%s duration_ms=%d error=%v\n", kind, status, time.Since(started).Milliseconds(), err)
		return err
	}
	fmt.Printf("model_request kind=%s status=%s duration_ms=%d\n", kind, status, time.Since(started).Milliseconds())
	return err
}
