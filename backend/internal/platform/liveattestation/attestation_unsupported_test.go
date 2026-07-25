//go:build !darwin

package liveattestation

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedProviderReturnsExplicitPlatformError(t *testing.T) {
	_, err := NewProvider().Generate(context.Background())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Generate() error = %v, want ErrUnsupportedPlatform", err)
REDACTED
REDACTED
