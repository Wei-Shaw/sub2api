//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
