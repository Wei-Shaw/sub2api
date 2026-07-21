package setup

import (
	"bufio"
	"strings"
	"testing"
)

func TestPromptRedisUsername(t *testing.T) {
	t.Run("accepts empty username for the default Redis user", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("\n"))

		if got := promptRedisUsername(reader); got != "" {
			t.Fatalf("promptRedisUsername() = %q, want empty username", got)
		}
	})

	t.Run("trims an ACL username", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader("  app-user  \n"))

		if got := promptRedisUsername(reader); got != "app-user" {
			t.Fatalf("promptRedisUsername() = %q, want %q", got, "app-user")
		}
	})

	t.Run("retries usernames longer than the setup API limit", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader(strings.Repeat("x", 129) + "\nvalid-user\n"))

		if got := promptRedisUsername(reader); got != "valid-user" {
			t.Fatalf("promptRedisUsername() = %q, want %q", got, "valid-user")
		}
	})
}
