package accountid

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateRejectsBiasedTailBytes(t *testing.T) {
	got, err := generate(bytes.NewReader([]byte{255, 8, 250, 0, 251, 1}), 3)
	require.NoError(t, err)
	require.Equal(t, "901", got)
}

func TestGenerateFormats(t *testing.T) {
	root, err := GenerateRoot()
	require.NoError(t, err)
	require.Regexp(t, `^[1-9][0-9]{15}$`, root)
	iam, err := GenerateIAM()
	require.NoError(t, err)
	require.Regexp(t, `^[1-9][0-9]{17}$`, iam)
}

func TestGenerateConcurrentUniqueness(t *testing.T) {
	const count = 256
	results := make(chan string, count)
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := GenerateIAM()
			require.NoError(t, err)
			results <- id
		}()
	}
	wg.Wait()
	close(results)
	seen := make(map[string]struct{}, count)
	for id := range results {
		_, duplicate := seen[id]
		require.False(t, duplicate)
		seen[id] = struct{}{}
	}
}
