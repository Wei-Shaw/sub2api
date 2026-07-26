package accountid

import (
	"crypto/rand"
	"fmt"
	"io"
	"sync/atomic"
)

const (
	RootLength = 16
	IAMLength  = 18
)

var collisionRetries atomic.Uint64

type Metrics struct {
	CollisionRetries uint64 `json:"collision_retries"`
}

func RecordCollisionRetry() { collisionRetries.Add(1) }

func CurrentMetrics() Metrics { return Metrics{CollisionRetries: collisionRetries.Load()} }

func GenerateRoot() (string, error) { return generate(rand.Reader, RootLength) }

func GenerateIAM() (string, error) { return generate(rand.Reader, IAMLength) }

func generate(reader io.Reader, length int) (string, error) {
	if length < 1 {
		return "", fmt.Errorf("account id length must be positive")
	}
	out := make([]byte, length)
	for i := range out {
		limit := byte(250)
		base := byte('0')
		modulus := byte(10)
		if i == 0 {
			limit = 252 // 28 complete buckets for each value in 1..9.
			base = '1'
			modulus = 9
		}
		for {
			var sample [1]byte
			if _, err := io.ReadFull(reader, sample[:]); err != nil {
				return "", fmt.Errorf("read secure random byte: %w", err)
			}
			if sample[0] >= limit {
				continue
			}
			out[i] = base + sample[0]%modulus
			break
		}
	}
	return string(out), nil
}
