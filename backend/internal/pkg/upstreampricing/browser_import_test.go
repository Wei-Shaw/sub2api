package upstreampricing

import (
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestBrowserImportIsolationExpiryAndReplay(t *testing.T) {
	b := newBrowserImports()
	now := time.Now()
	b.now = func() time.Time { return now }
	c := Config{Provider: "newapi", BaseURL: "https://example.com", UserID: "42", Token: "synthetic-should-not-store"}
	start, err := b.start(1, 7, 3, "fp", "http://localhost:8080", c)
	require.NoError(t, err)
	_, err = b.Status(start.ID, 2, 7)
	require.Error(t, err)
	_, err = b.Status(start.ID, 1, 8)
	require.Error(t, err)
	_, err = b.ticket(start.Ticket, "http://127.0.0.1:8080", true)
	require.Error(t, err)
	e, err := b.ticket(start.Ticket, "http://localhost:8080", true)
	require.NoError(t, err)
	require.Empty(t, e.config.Token)
	_, err = b.ticket(start.Ticket, "http://localhost:8080", true)
	require.Error(t, err)
	b.finish(start.ID, "synthetic-encrypted", "42", []Group{{Name: "paid"}}, nil)
	status, err := b.Status(start.ID, 1, 7)
	require.NoError(t, err)
	payload, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "synthetic")
	require.NotContains(t, string(payload), start.Ticket)
	credential, err := b.resolve(start.ID, 7, 3, "fp", c)
	require.NoError(t, err)
	require.Equal(t, "synthetic-encrypted", credential)
	for _, changed := range []Config{{Provider: "newapi", BaseURL: "https://other.example", UserID: "42"}, {Provider: "newapi", BaseURL: c.BaseURL, UserID: "43"}} {
		_, err = b.resolve(start.ID, 7, 3, "fp", changed)
		require.Error(t, err)
	}
	_, err = b.resolve(start.ID, 7, 4, "fp", c)
	require.Error(t, err)
	_, err = b.resolve(start.ID, 7, 3, "changed-fp", c)
	require.Error(t, err)
	now = now.Add(5 * time.Minute)
	_, err = b.resolve(start.ID, 7, 3, "fp", c)
	require.Error(t, err)
	require.Empty(t, b.entries)
}

func TestBrowserImportFailureNeverReturnsDiagnostics(t *testing.T) {
	b := newBrowserImports()
	start, err := b.start(1, 7, 1, "fp", "http://localhost:8080", Config{})
	require.NoError(t, err)
	b.finish(start.ID, "", "", nil, errors.New("synthetic-secret diagnostic"))
	status, err := b.Status(start.ID, 1, 7)
	require.NoError(t, err)
	require.Equal(t, "failed", status.Status)
	require.NotContains(t, status.Error, "synthetic-secret")
	_, err = b.resolve(start.ID, 7, 1, "fp", Config{})
	require.Error(t, err)
}

func TestBrowserImportOriginBinding(t *testing.T) {
	require.NoError(t, ValidateBrowserOrigin("http://localhost:8080", "localhost:8080", "http"))
	for _, origin := range []string{"https://evil.example", "http://localhost:8081", "http://user@localhost:8080", "http://localhost:8080/", "http://localhost:8080?x=1", "https://localhost:8080"} {
		require.Error(t, ValidateBrowserOrigin(origin, "localhost:8080", "http"))
	}
}

func TestBrowserImportConnectorFailureConsumesTicket(t *testing.T) {
	m := NewManager(nil, nil, nil)
	defer m.Stop()
	start, err := m.Imports.start(1, 7, 3, "fp", "http://localhost:8080", Config{})
	require.NoError(t, err)
	require.NoError(t, m.FailBrowserImport(start.Ticket, "http://localhost:8080"))
	state, err := m.Imports.Status(start.ID, 1, 7)
	require.NoError(t, err)
	require.Equal(t, "failed", state.Status)
	_, err = m.BrowserImportInfo(start.Ticket, "http://localhost:8080")
	require.Error(t, err)
}
