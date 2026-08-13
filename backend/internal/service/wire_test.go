package service

import (
	"errors"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/collection"
)

func TestProvideTimingWheelService_ReturnsError(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, _ collection.Execute) (*collection.TimingWheel, error) {
		return nil, errors.New("boom")
	}

	svc, err := ProvideTimingWheelService()
	if err != nil {
		t.Fatalf("构造不应初始化 timing wheel: %v", err)
	}
	if err := svc.Start(); err == nil {
		t.Fatalf("期望 Start 返回 error，但得到 nil")
	}
}

func TestProvideTimingWheelService_Success(t *testing.T) {
	svc, err := ProvideTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	if err := svc.Start(); err != nil {
		t.Fatalf("启动 timing wheel 失败: %v", err)
	}
	svc.Stop()
}
