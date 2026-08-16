package handler

import (
	"context"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type asyncImageUsageCollectorContextKey struct{}

type asyncImageUsageTask struct {
	task    service.UsageRecordTask
	abandon service.UsageRecordTask
}

type asyncImageUsageCollector struct {
	mu        sync.Mutex
	collected []asyncImageUsageTask
}

func withAsyncImageUsageCollector(ctx context.Context) (context.Context, *asyncImageUsageCollector) {
	if ctx == nil {
		ctx = context.Background()
	}
	collector := &asyncImageUsageCollector{}
	return context.WithValue(ctx, asyncImageUsageCollectorContextKey{}, collector), collector
}

func collectAsyncImageUsageTaskWithAbandon(ctx context.Context, task, abandon service.UsageRecordTask) bool {
	if ctx == nil || task == nil {
		return false
	}
	collector, _ := ctx.Value(asyncImageUsageCollectorContextKey{}).(*asyncImageUsageCollector)
	if collector == nil {
		return false
	}
	collector.mu.Lock()
	collector.collected = append(collector.collected, asyncImageUsageTask{task: task, abandon: abandon})
	collector.mu.Unlock()
	return true
}

func (c *asyncImageUsageCollector) take() []asyncImageUsageTask {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	collected := c.collected
	c.collected = nil
	return collected
}

func (c *asyncImageUsageCollector) abandon(ctx context.Context) {
	for _, collected := range c.take() {
		if collected.abandon != nil {
			collected.abandon(ctx)
		}
	}
}
