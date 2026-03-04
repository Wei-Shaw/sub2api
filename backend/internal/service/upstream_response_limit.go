package service

import (
	"errors"
	"fmt"
	"io"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var ErrUpstreamResponseBodyTooLarge = errors.New("upstream response body too large")

const defaultUpstreamResponseReadMaxBytes int64 = 8 * 1024 * 1024

func resolveUpstreamResponseReadLimit(cfg *config.Config) int64 {
	if cfg != nil && cfg.Gateway.UpstreamResponseReadMaxBytes > 0 {
		return cfg.Gateway.UpstreamResponseReadMaxBytes
REDACTED
	return defaultUpstreamResponseReadMaxBytes
REDACTED

func readUpstreamResponseBodyLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("response body is nil")
REDACTED
	if maxBytes <= 0 {
		maxBytes = defaultUpstreamResponseReadMaxBytes
REDACTED

	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
REDACTED
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%w: limit=%d", ErrUpstreamResponseBodyTooLarge, maxBytes)
REDACTED
	return body, nil
REDACTED
