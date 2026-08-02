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
	// CompanyLength is the total length of a company identifier, including the
	// leading 'c' prefix (1 prefix character followed by 15 digits).
	CompanyLength = 16
)

var collisionRetries atomic.Uint64

type Metrics struct {
	CollisionRetries uint64 `json:"collision_retries"`
}

func RecordCollisionRetry() { collisionRetries.Add(1) }

func CurrentMetrics() Metrics { return Metrics{CollisionRetries: collisionRetries.Load()} }

func GenerateRoot() (string, error) { return generate(rand.Reader, RootLength) }

func GenerateIAM() (string, error) { return generate(rand.Reader, IAMLength) }

// GenerateCompany returns a company account identifier: a leading 'c' prefix
// followed by 15 digits (first digit 1-9), e.g. "c123456789012345". The prefix
// keeps company identifiers visually distinct from the numeric root/IAM account
// identifiers while still fitting within a VARCHAR(16) column.
func GenerateCompany() (string, error) {
	digits, err := generate(rand.Reader, CompanyLength-1)
	if err != nil {
		return "", err
	}
	return "c" + digits, nil
}

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
