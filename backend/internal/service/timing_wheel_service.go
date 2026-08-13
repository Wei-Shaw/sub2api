package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/zeromicro/go-zero/core/collection"
)

var newTimingWheel = collection.NewTimingWheel

// TimingWheelService wraps go-zero's TimingWheel for task scheduling.
type TimingWheelService struct {
	mu      sync.RWMutex
	tw      *collection.TimingWheel
	stopped bool
}

// NewTimingWheelService creates a passive TimingWheelService instance.
func NewTimingWheelService() (*TimingWheelService, error) {
	return &TimingWheelService{}, nil
}

// Start initializes the timing wheel runtime.
func (s *TimingWheelService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return errors.New("timing wheel has stopped")
	}
	if s.tw != nil {
		return nil
	}

	// 1 second tick, 3600 slots = supports up to 1 hour delay.
	tw, err := newTimingWheel(1*time.Second, 3600, func(key, value any) {
		if fn, ok := value.(func()); ok {
			fn()
		}
	})
	if err != nil {
		return fmt.Errorf("创建 timing wheel 失败: %w", err)
	}
	s.tw = tw
	logger.LegacyPrintf("service.timing_wheel", "%s", "[TimingWheel] Started")
	return nil
}

// Stop stops the timing wheel.
func (s *TimingWheelService) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	tw := s.tw
	s.tw = nil
	s.mu.Unlock()

	if tw != nil {
		tw.Stop()
	}
	logger.LegacyPrintf("service.timing_wheel", "%s", "[TimingWheel] Stopped")
}

// Schedule schedules a one-time task.
func (s *TimingWheelService) Schedule(name string, delay time.Duration, fn func()) {
	s.mu.RLock()
	tw := s.tw
	s.mu.RUnlock()
	if tw == nil {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] SetTimer failed for %q: timing wheel is not started", name)
		return
	}
	if err := tw.SetTimer(name, fn, delay); err != nil {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] SetTimer failed for %q: %v", name, err)
	}
}

// ScheduleRecurring schedules a recurring task.
func (s *TimingWheelService) ScheduleRecurring(name string, interval time.Duration, fn func()) {
	s.mu.RLock()
	tw := s.tw
	s.mu.RUnlock()
	if tw == nil {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] initial SetTimer failed for %q: timing wheel is not started", name)
		return
	}

	var schedule func()
	schedule = func() {
		fn()
		if err := tw.SetTimer(name, schedule, interval); err != nil {
			logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] recurring SetTimer failed for %q: %v", name, err)
		}
	}
	if err := tw.SetTimer(name, schedule, interval); err != nil {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] initial SetTimer failed for %q: %v", name, err)
	}
}

// Cancel cancels a scheduled task.
func (s *TimingWheelService) Cancel(name string) {
	s.mu.RLock()
	tw := s.tw
	s.mu.RUnlock()
	if tw != nil {
		_ = tw.RemoveTimer(name)
	}
}
