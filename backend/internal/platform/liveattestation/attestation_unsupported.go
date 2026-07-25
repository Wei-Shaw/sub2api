//go:build !darwin

package liveattestation

import "context"

type unsupportedProvider struct{}

func NewProvider() Provider {
	return unsupportedProvider{}
}

func (unsupportedProvider) Check(context.Context) error {
	return ErrUnsupportedPlatform
}

func (unsupportedProvider) Generate(context.Context) (string, error) {
	return "", ErrUnsupportedPlatform
}
