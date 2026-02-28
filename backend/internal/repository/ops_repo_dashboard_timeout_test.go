package repository

import (
	"context"
	"fmt"
	"testing"
)

func TestIsQueryTimeoutErr(t *testing.T) {
	if !isQueryTimeoutErr(context.DeadlineExceeded) {
		t.Fatalf("context.DeadlineExceeded should be treated as query timeout")
REDACTED
	if !isQueryTimeoutErr(fmt.Errorf("wrapped: %w", context.DeadlineExceeded)) {
		t.Fatalf("wrapped context.DeadlineExceeded should be treated as query timeout")
REDACTED
	if isQueryTimeoutErr(context.Canceled) {
		t.Fatalf("context.Canceled should not be treated as query timeout")
REDACTED
	if isQueryTimeoutErr(fmt.Errorf("wrapped: %w", context.Canceled)) {
		t.Fatalf("wrapped context.Canceled should not be treated as query timeout")
REDACTED
REDACTED
