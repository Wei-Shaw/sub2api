package keywordmatcher

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeTrimsDeduplicatesAndLimits(t *testing.T) {
	got := Normalize([]string{"  Secret ", "secret", "", "世界很长"}, 2, 2)
	require.Equal(t, []string{"Se", "世界"}, got)
}

func TestMatcherUsesConfiguredOrderForOverlappingMatches(t *testing.T) {
	matcher := New([]string{"later", "early"})
	keyword, hit := matcher.Match("early appears before later")
	require.True(t, hit)
	require.Equal(t, "later", keyword)

	keyword, hit = New([]string{"bc", "abc"}).Match("abc")
	require.True(t, hit)
	require.Equal(t, "bc", keyword)
}

func TestMatcherRandomizedParityWithSimpleSubstringScan(t *testing.T) {
	rng := rand.New(rand.NewSource(20260801))
	const alphabet = "abcXYZ"
	for iteration := 0; iteration < 1000; iteration++ {
		keywords := make([]string, 1+rng.Intn(30))
		for index := range keywords {
			length := 1 + rng.Intn(8)
			var value strings.Builder
			for range length {
				_ = value.WriteByte(alphabet[rng.Intn(len(alphabet))])
			}
			keywords[index] = value.String()
		}
		var text strings.Builder
		for range 20 + rng.Intn(100) {
			_ = text.WriteByte(alphabet[rng.Intn(len(alphabet))])
		}

		wantKeyword := ""
		for _, keyword := range keywords {
			if strings.Contains(strings.ToLower(text.String()), strings.ToLower(keyword)) {
				wantKeyword = keyword
				break
			}
		}
		gotKeyword, gotHit := New(keywords).Match(text.String())
		require.Equal(t, wantKeyword != "", gotHit, "iteration %d", iteration)
		require.Equal(t, wantKeyword, gotKeyword, "iteration %d", iteration)
	}
}
