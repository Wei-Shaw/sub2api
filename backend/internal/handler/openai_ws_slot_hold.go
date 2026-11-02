package handler

import (
	"context"
	"sync"
	"time"
)

// openAIWSHeldAccountSlot 是轮间挂起的账号槽：轮结束不释放，挂保留定时器；下一轮在到期前取回并重新准入、续租。
// 状态变更都在 mu 下进行。释放函数由 wrapReleaseOnDone 包装，本身只执行一次；
// 这里保证 take、expire、releaseNow 三者对同一个挂起态最多只有一个拿到 release。
type openAIWSHeldAccountSlot struct {
	mu       sync.Mutex
	release  func()
	reuse    func(context.Context) (bool, error)
	timer    *time.Timer
	parkedAt time.Time
	onExpire func(heldFor time.Duration)
	// generation 每次 park 递增，定时器回调带着自己那一代的号；旧定时器已触发但回调尚未拿到锁时
	// Stop 撤不回它，若期间发生取回并再次挂起，旧回调核对代际后发现不是自己那一代，不得释放新挂起槽。
	generation uint64
}

// park 挂起一个账号槽；已有挂起态时先释放前一个。hold 到期由本代定时器释放。
func (s *openAIWSHeldAccountSlot) park(release func(), reuse func(context.Context) (bool, error), hold time.Duration, onExpire func(heldFor time.Duration)) {
	s.mu.Lock()
	previous := s.takeLocked()
	s.generation++
	generation := s.generation
	s.release = release
	s.reuse = reuse
	s.parkedAt = time.Now()
	s.onExpire = onExpire
	s.timer = time.AfterFunc(hold, func() { s.expire(generation) })
	s.mu.Unlock()
	if previous != nil {
		previous()
	}
}

func (s *openAIWSHeldAccountSlot) expire(generation uint64) {
	s.mu.Lock()
	if generation != s.generation {
		s.mu.Unlock()
		return
	}
	parkedAt := s.parkedAt
	onExpire := s.onExpire
	release := s.takeLocked()
	s.mu.Unlock()
	if release == nil {
		return
	}
	release()
	if onExpire != nil {
		onExpire(time.Since(parkedAt))
	}
}

// take 取回挂起槽并停掉定时器。定时器已触发但尚未拿到锁时，这里先拿到锁就把 release 交给调用方，
// expire 随后拿到锁发现已清空，不再释放。没有挂起态返回 nil。
func (s *openAIWSHeldAccountSlot) take() (func(), func(context.Context) (bool, error), time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.release == nil {
		return nil, nil, 0
	}
	reuse := s.reuse
	heldFor := time.Since(s.parkedAt)
	release := s.takeLocked()
	return release, reuse, heldFor
}

// releaseNow 立即释放挂起槽：连接关闭、换号、进入用户等待前调用。返回是否真的释放了一个。
func (s *openAIWSHeldAccountSlot) releaseNow() bool {
	s.mu.Lock()
	release := s.takeLocked()
	s.mu.Unlock()
	if release == nil {
		return false
	}
	release()
	return true
}

func (s *openAIWSHeldAccountSlot) parked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.release != nil
}

func (s *openAIWSHeldAccountSlot) takeLocked() func() {
	release := s.release
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.release = nil
	s.reuse = nil
	s.parkedAt = time.Time{}
	s.onExpire = nil
	return release
}
