//go:build !darwin

package liveattestation

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedProviderReturnsExplicitPlatformError(t *testing.T) {
	provider := NewProvider()
	if err := provider.Check(context.Background()); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Check() error = %v, want ErrUnsupportedPlatform", err)
	}
	_, err := provider.Generate(context.Background())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Generate() error = %v, want ErrUnsupportedPlatform", err)
	}
}
