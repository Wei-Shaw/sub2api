package service

import (
	"log"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/collection"
)

// TimingWheelService wraps go-zero's TimingWheel for task scheduling
type TimingWheelService struct {
	tw       *collection.TimingWheel
	stopOnce sync.Once
}

// NewTimingWheelService creates a new TimingWheelService instance
func NewTimingWheelService() *TimingWheelService {
	// 1 second tick, 3600 slots = supports up to 1 hour delay
	// execute function: runs func() type tasks
	tw, err := collection.NewTimingWheel(1*time.Second, 3600, func(key, value any) {
		if fn, ok := value.(func()); ok {
			fn()
		}
	})
	if err != nil {
		panic(err)
	}
	return &TimingWheelService{tw: tw}
}

// Start starts the timing wheel
func (s *TimingWheelService) Start() {
	log.Println("[TimingWheel] Started (auto-start by go-zero)")
}

// Stop stops the timing wheel
func (s *TimingWheelService) Stop() {
	s.stopOnce.Do(func() {
		s.tw.Stop()
		log.Println("[TimingWheel] Stopped")
	})
}

// Schedule schedules a one-time task
func (s *TimingWheelService) Schedule(name string, delay time.Duration, fn func()) {
	_ = s.tw.SetTimer(name, fn, delay)
}

// ScheduleRecurring schedules a recurring task
func (s *TimingWheelService) ScheduleRecurring(name string, interval time.Duration, fn func()) {
	var schedule func()
	schedule = func() {
		fn()
		_ = s.tw.SetTimer(name, schedule, interval)
	}
	_ = s.tw.SetTimer(name, schedule, interval)
}

// Cancel cancels a scheduled task
func (s *TimingWheelService) Cancel(name string) {
	_ = s.tw.RemoveTimer(name)
}
