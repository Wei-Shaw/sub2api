//go:build unit

package testutil

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// NewTestUser 创建一个可用的测试用户，可通过 opts 覆盖默认值。
func NewTestUser(opts ...func(*service.User)) *service.User {
	u := &service.User{
		ID:          1,
		Email:       "test@example.com",
		Username:    "testuser",
		Role:        "user",
		Balance:     100.0,
		Concurrency: 5,
		Status:      service.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// NewTestAccount 创建一个可用的测试账户，可通过 opts 覆盖默认值。
func NewTestAccount(opts ...func(*service.Account)) *service.Account {
	a := &service.Account{
		ID:          1,
		Name:        "test-account",
		Platform:    service.PlatformAnthropic,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 5,
		Priority:    1,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// NewTestAPIKey 创建一个可用的测试 API Key，可通过 opts 覆盖默认值。
func NewTestAPIKey(opts ...func(*service.APIKey)) *service.APIKey {
	groupID := int64(1)
	k := &service.APIKey{
		ID:        1,
		UserID:    1,
		Key:       "sk-test-key-12345678",
		Name:      "test-key",
		GroupID:   &groupID,
		Status:    service.StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// NewTestGroup 创建一个可用的测试分组，可通过 opts 覆盖默认值。
func NewTestGroup(opts ...func(*service.Group)) *service.Group {
	g := &service.Group{
		ID:       1,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
		Hydrated: true,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}
