package securityaudit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	promptRecordByteBudget    int64 = 64 * 1024 * 1024
	promptRecordItemByteLimit int64 = 16 * 1024 * 1024
)

type PromptRecordService struct {
	repo                                                           PromptRecordRepository
	queue, overflow, responseQueue, responseOverflow               chan promptRecordJob
	requestSlots, responseSlots                                    chan struct{}
	workers                                                        int
	once                                                           sync.Once
	lifecycleMu                                                    sync.Mutex
	stopped                                                        bool
	producers, requestWG, responseWG, cleanupWG                    sync.WaitGroup
	ctx                                                            context.Context
	cancel                                                         context.CancelFunc
	done, cleanupStop                                              chan struct{}
	pendingMu                                                      sync.Mutex
	pendingResponses                                               map[*promptRecordCorrelation]promptRecordJob
	byteLimit, itemByteLimit                                       int64
	inFlightBytes, dropped, persistFailed                          atomic.Int64
	requestDropped, responseDropped, requestFailed, responseFailed atomic.Int64
	expiredDeleted, cleanupFailed                                  atomic.Int64
	cleanupBacklog                                                 atomic.Bool
	lastDroppedAt                                                  atomic.Pointer[time.Time]
}

func promptRecordMetadataBytes(req Request) int64 {
	return int64(1024 + len(req.RequestID) + len(req.recordingSessionID) + len(req.Username) + len(req.UserEmail) + len(req.APIKeyName) + len(req.GroupName) + len(req.Provider) + len(req.Endpoint) + len(req.Protocol) + len(req.Model) + len(req.Stage))
}

func promptRecordRequestBytes(req Request) int64 {
	size := promptRecordMetadataBytes(req)
	if !req.recordingSkipPrompt {
		size += int64(len(req.Body))
	}
	if !req.recordingSkipHeaders {
		for key, values := range req.Headers {
			size += int64(len(key) + 64 + len(values)*16)
			for _, value := range values {
				size += int64(len(value))
			}
		}
	}
	return size
}

func (s *PromptRecordService) start() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if !s.stopped {
		s.startLocked()
	}
}

func (s *PromptRecordService) startLocked() {
	s.once.Do(func() {
		for range s.workers {
			s.requestWG.Add(1)
			go s.worker(s.queue, s.overflow, false)
		}
		for range promptResponseWorkerCount {
			s.responseWG.Add(1)
			go s.worker(s.responseQueue, s.responseOverflow, true)
		}
		if _, ok := s.repo.(promptRecordExpiryRepository); ok {
			s.cleanupWG.Add(1)
			go s.cleanupLoop()
		}
	})
}

// Reserve both queue space and retained bytes before copying any request data.
// Pending responses own the same bounded reservation as ready responses.
func (s *PromptRecordService) reserve(size int64, response bool, req Request) bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	code := ""
	slots := s.requestSlots
	if response {
		slots = s.responseSlots
	}
	switch {
	case s.stopped:
		code = "prompt_record_stopped"
	case size > s.itemByteLimit:
		code = "prompt_record_item_too_large"
	case size > s.byteLimit-s.inFlightBytes.Load():
		code = "prompt_record_byte_budget_full"
	default:
		select {
		case slots <- struct{}{}:
			s.inFlightBytes.Add(size)
			s.producers.Add(1)
			s.startLocked()
			return true
		default:
			code = "prompt_record_queue_full"
		}
	}
	s.recordDrop(req, response, code)
	return false
}

func (s *PromptRecordService) releaseJob(job promptRecordJob, queued bool) {
	if job.bytes == 0 {
		return
	}
	if queued {
		if job.response == nil {
			<-s.requestSlots
		} else {
			<-s.responseSlots
		}
	}
	s.inFlightBytes.Add(-job.bytes)
}

func (s *PromptRecordService) worker(queue, overflow <-chan promptRecordJob, response bool) {
	if response {
		defer s.responseWG.Done()
	} else {
		defer s.requestWG.Done()
	}
	for queue != nil || overflow != nil {
		var job promptRecordJob
		var ok bool
		select {
		case job, ok = <-queue:
			if !ok {
				queue = nil
				continue
			}
		case job, ok = <-overflow:
			if !ok {
				overflow = nil
				continue
			}
		}
		if job.bytes > 0 {
			if response {
				<-s.responseSlots
			} else {
				<-s.requestSlots
			}
		}
		func() {
			defer s.releaseJob(job, false)
			s.persistJobSafely(job)
		}()
	}
}

func (s *PromptRecordService) recordDrop(req Request, response bool, code string) {
	if response {
		s.responseDropped.Add(1)
	} else {
		s.requestDropped.Add(1)
	}
	dropped := s.dropped.Add(1)
	now := time.Now().UTC()
	s.lastDroppedAt.Store(&now)
	if isPowerOfTwo(dropped) {
		LogWarn(EventPromptRecordDropped, mergeLogFields(requestLogFields(req), map[string]any{
			"status": "dropped", "error_code": code, "in_flight_bytes": s.inFlightBytes.Load(), "byte_capacity": s.byteLimit,
		}))
	}
}

func (s *PromptRecordService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	if !s.stopped {
		s.stopped = true
		close(s.cleanupStop)
		go s.drain()
	}
	s.lifecycleMu.Unlock()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		s.cancel()
		return ctx.Err()
	}
}

func (s *PromptRecordService) drain() {
	defer close(s.done)
	defer s.cancel()
	s.producers.Wait()
	close(s.queue)
	close(s.overflow)
	s.requestWG.Wait()
	// Every accepted request has now published success or failure. Resolve any
	// orphan response references before closing their ready queues.
	s.pendingMu.Lock()
	orphans := make([]*promptRecordCorrelation, 0, len(s.pendingResponses))
	for correlation := range s.pendingResponses {
		orphans = append(orphans, correlation)
	}
	s.pendingMu.Unlock()
	for _, correlation := range orphans {
		correlation.complete(PromptRecordKey{}, false)
	}
	close(s.responseQueue)
	close(s.responseOverflow)
	s.responseWG.Wait()
	s.cleanupWG.Wait()
}

type promptRecordExpiryRepository interface {
	DeleteExpiredPromptRecords(context.Context, int) (int64, error)
}

func (s *PromptRecordService) cleanupLoop() {
	defer s.cleanupWG.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		s.cleanupExpired()
		select {
		case <-s.cleanupStop:
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *PromptRecordService) cleanupExpired() {
	repo, ok := s.repo.(promptRecordExpiryRepository)
	if !ok {
		return
	}
	for range 10 {
		select {
		case <-s.cleanupStop:
			return
		default:
		}
		ctx, cancel := context.WithTimeout(s.ctx, promptRecordPersistTimeout)
		deleted, err := repo.DeleteExpiredPromptRecords(ctx, 1000)
		cancel()
		if err != nil {
			s.cleanupFailed.Add(1)
			return
		}
		s.expiredDeleted.Add(deleted)
		s.cleanupBacklog.Store(deleted == 1000)
		if deleted < 1000 {
			return
		}
	}
}
