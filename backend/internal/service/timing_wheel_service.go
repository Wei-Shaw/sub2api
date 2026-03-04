package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/zeromicro/go-zero/core/collection"
)

var newTimingWheel = collection.NewTimingWheel

// TimingWheelService wraps go-zero's TimingWheel for task scheduling
type TimingWheelService struct {
	tw       *collection.TimingWheel
	stopOnce sync.Once
REDACTED

// NewTimingWheelService creates a new TimingWheelService instance
func NewTimingWheelService() (*TimingWheelService, error) {
	// 1 second tick, 3600 slots = supports up to 1 hour delay
	// execute function: runs func() type tasks
	tw, err := newTimingWheel(1*time.Second, 3600, func(key, value any) {
		if fn, ok := value.(func()); ok {
			fn()
	REDACTED
REDACTED)
	if err != nil {
		return nil, fmt.Errorf("创建 timing wheel 失败: %w", err)
REDACTED
	return &TimingWheelService{tw: twREDACTED, nil
REDACTED

// Start starts the timing wheel
func (s *TimingWheelService) Start() {
	logger.LegacyPrintf("service.timing_wheel", "%s", "[TimingWheel] Started (auto-start by go-zero)")
REDACTED

// Stop stops the timing wheel
func (s *TimingWheelService) Stop() {
	s.stopOnce.Do(func() {
		s.tw.Stop()
		logger.LegacyPrintf("service.timing_wheel", "%s", "[TimingWheel] Stopped")
REDACTED)
REDACTED

// Schedule schedules a one-time task
func (s *TimingWheelService) Schedule(name string, delay time.Duration, fn func()) {
	if err := s.tw.SetTimer(name, fn, delay); err != nil {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] SetTimer failed for %q: %v", name, err)
REDACTED
REDACTED

// ScheduleRecurring schedules a recurring task
func (s *TimingWheelService) ScheduleRecurring(name string, interval time.Duration, fn func()) {
	var schedule func()
	schedule = func() {
		fn()
		if err := s.tw.SetTimer(name, schedule, interval); err != nil {
			logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] recurring SetTimer failed for %q: %v", name, err)
	REDACTED
REDACTED
	if err := s.tw.SetTimer(name, schedule, interval); err != nil {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] initial SetTimer failed for %q: %v", name, err)
REDACTED
REDACTED

// Cancel cancels a scheduled task
func (s *TimingWheelService) Cancel(name string) {
	_ = s.tw.RemoveTimer(name)
REDACTED
