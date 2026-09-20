package ticketproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func importLines(t *testing.T, s *Store, lines ...string) ImportResult {
	t.Helper()
	proxies, duplicates, err := Parse(lines)
	require.NoError(t, err)
	result, err := s.Import(proxies, duplicates)
	require.NoError(t, err)
	return result
}

func TestParseFormatsAndSafeErrors(t *testing.T) {
	proxies, duplicates, err := Parse([]string{" 192.0.2.1:8080 ", "http://192.0.2.1:8080/", "192.0.2.2:8080:user:p:a@ss", "socks5h://u:p@[2001:db8::1]:1080", "https://proxy.example:443", ""})
	require.NoError(t, err)
	require.Len(t, proxies, 4)
	require.Equal(t, 1, duplicates)
	require.Equal(t, "p:a@ss", proxies[1].Password)
	require.Equal(t, "socks5h://u:p@[2001:db8::1]:1080", proxies[2].URL())
	for _, line := range []string{"192.0.2.1", "http://user:secret@host:0", "http://user:secret@host:65536", "ftp://host:21", "http://host:80/path", "http://host:80?x=y", "host:80:user:", "http://user@host:80", "http://host:80?"} {
		_, _, err := Parse([]string{line})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
	_, _, err = Parse([]string{"", " "})
	require.Error(t, err)
	_, _, err = Parse(make([]string, MaxProxies+1))
	require.Error(t, err)
}

func TestImportAppendDeduplicateAndRotate(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "pool.json"))
	_, managed, err := s.Acquire()
	require.NoError(t, err)
	require.False(t, managed)
	r := importLines(t, s, "192.0.2.1:80", "192.0.2.1:80")
	require.Equal(t, 1, r.Added)
	require.Equal(t, 1, r.Duplicates)
	r = importLines(t, s, "http://192.0.2.1:80", "192.0.2.2:80")
	require.Equal(t, 1, r.Added)
	require.Equal(t, 1, r.Duplicates)
	require.Equal(t, 2, r.Count)
	a, _, err := s.Acquire()
	require.NoError(t, err)
	b, _, err := s.Acquire()
	require.NoError(t, err)
	require.NotEqual(t, a.ID, b.ID)
	c, _, err := s.Acquire()
	require.NoError(t, err)
	require.Equal(t, a.ID, c.ID)
	raw, err := json.Marshal(r)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "192.0.2")
}

func TestFirstFailurePersistsAndSuccessKeepsProxy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pool.json")
	s := NewStore(path)
	importLines(t, s, "192.0.2.1:80")
	lease, _, _ := s.Acquire()
	removed, err := s.Report(lease.ID, true)
	require.NoError(t, err)
	require.False(t, removed)
	// Re-importing a duplicate preserves the live proxy identity.
	importLines(t, s, "192.0.2.1:80")
	st, _, err := s.read()
	require.NoError(t, err)
	require.Len(t, st.Proxies, 1)
	require.Equal(t, lease.ID, st.Proxies[0].ID)
	require.Zero(t, st.Proxies[0].Failures)
	s = NewStore(path)
	removed, err = s.Report(lease.ID, false)
	require.NoError(t, err)
	require.True(t, removed)
	status, err := NewStore(path).Status()
	require.NoError(t, err)
	require.True(t, status.Managed)
	require.Zero(t, status.Count)
	require.Equal(t, 1, status.Removed)
	empty, managed, err := s.Acquire()
	require.NoError(t, err)
	require.True(t, managed)
	require.Empty(t, empty.URL)
	importLines(t, s, "192.0.2.1:80")
	newLease, _, _ := s.Acquire()
	require.NotEqual(t, lease.ID, newLease.ID)
	for i := 0; i < 4; i++ {
		_, err = s.Report(lease.ID, false)
		require.NoError(t, err)
	}
	status, err = s.Status()
	require.NoError(t, err)
	require.Equal(t, 1, status.Count)
}

func TestConcurrentReportsAndImports(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "pool.json"))
	importLines(t, s, "192.0.2.1:80")
	lease, _, _ := s.Acquire()
	var wg sync.WaitGroup
	errs := make(chan error, 60)
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, err := s.Report(lease.ID, false); errs <- err }()
		go func() {
			defer wg.Done()
			p, d, err := Parse([]string{"192.0.2.2:80"})
			if err == nil {
				_, err = s.Import(p, d)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	status, err := s.Status()
	require.NoError(t, err)
	require.Equal(t, 1, status.Count)
	require.Equal(t, 1, status.Removed)
}

func TestLegacyMigrationAndCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pool.json")
	require.NoError(t, os.WriteFile(path, []byte(`[{"protocol":"http","host":"192.0.2.1","port":80,"username":"u","password":"secret"}]`), 0600))
	s := NewStore(path)
	lease, managed, err := s.Acquire()
	require.NoError(t, err)
	require.True(t, managed)
	require.NotEmpty(t, lease.ID)
	lease2, _, err := NewStore(path).Acquire()
	require.NoError(t, err)
	require.Equal(t, lease.ID, lease2.ID)
	require.NoError(t, os.WriteFile(path, []byte("broken"), 0600))
	_, managed, err = s.Acquire()
	require.Error(t, err)
	require.True(t, managed)
	p, d, err := Parse([]string{"192.0.2.2:80"})
	require.NoError(t, err)
	_, err = s.Import(p, d)
	require.Error(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "broken", strings.TrimSpace(string(raw)))
}
