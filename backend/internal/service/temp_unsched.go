package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/cache"
	"github.com/Wei-Shaw/sub2api/internal/port/scheduler"
)

// TempUnschedState 临时不可调度状态
type TempUnschedState = domain.TempUnschedState

// TempUnschedCache 临时不可调度缓存接口
type TempUnschedCache = scheduler.TempUnschedCache

// TimeoutCounterCache 超时计数器缓存接口
type TimeoutCounterCache = cache.TimeoutCounterCache
