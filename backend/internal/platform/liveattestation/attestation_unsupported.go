//go:build !darwin

package liveattestation

import "context"

type unsupportedProvider struct{REDACTED

func NewProvider() Provider {
	return unsupportedProvider{REDACTED
REDACTED

func (unsupportedProvider) Generate(context.Context) (string, error) {
	return "", ErrUnsupportedPlatform
REDACTED
