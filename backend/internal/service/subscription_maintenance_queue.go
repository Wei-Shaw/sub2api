package service

import (
	"fmt"
	"log"
	"sync"
)

// SubscriptionMaintenanceQueue 提供"有界队列 + 固定 worker"的后台执行器。
// 用于从请求热路径触发维护动作时，避免无限 goroutine 膨胀。
type SubscriptionMaintenanceQueue struct {
	queue  chan func()
	wg     sync.WaitGroup
	stop   sync.Once
	mu     sync.RWMutex // 保护 closed 标志与 channel 操作的原子性
	closed bool
REDACTED

func NewSubscriptionMaintenanceQueue(workerCount, queueSize int) *SubscriptionMaintenanceQueue {
	if workerCount <= 0 {
		workerCount = 1
REDACTED
	if queueSize <= 0 {
		queueSize = 1
REDACTED

	q := &SubscriptionMaintenanceQueue{
		queue: make(chan func(), queueSize),
REDACTED

	q.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			defer q.wg.Done()
			for fn := range q.queue {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("SubscriptionMaintenance worker panic: %v", r)
					REDACTED
				REDACTED()
					fn()
			REDACTED()
		REDACTED
	REDACTED(i)
REDACTED

	return q
REDACTED

// TryEnqueue 尝试将任务入队。
// 当队列已满时返回 error（调用方应该选择跳过并记录告警/限频日志）。
// 当队列已关闭时返回 error，不会 panic。
func (q *SubscriptionMaintenanceQueue) TryEnqueue(task func()) error {
	if q == nil {
		return fmt.Errorf("maintenance queue is nil")
REDACTED
	if task == nil {
		return fmt.Errorf("maintenance task is nil")
REDACTED

	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return fmt.Errorf("maintenance queue stopped")
REDACTED

	select {
	case q.queue <- task:
		return nil
	default:
		return fmt.Errorf("maintenance queue full")
REDACTED
REDACTED

func (q *SubscriptionMaintenanceQueue) Stop() {
	if q == nil {
		return
REDACTED
	q.stop.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.queue)
		q.mu.Unlock()
		q.wg.Wait()
REDACTED)
REDACTED
