//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// newPluginSettingsTestService spins up a PluginSettingsService backed by sqlmock.
// Returns (service, mock, cleanup). The caller drives expectations on mock.
func newPluginSettingsTestService(t *testing.T) (*PluginSettingsService, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	svc := NewPluginSettingsService(db)
	return svc, mock, func() { _ = db.Close() }
}

func TestDropAllSubscribersForPlugin_ClosesChannels(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := newPluginSettingsTestService(t)
	defer cleanup()

	ch1, _ := svc.Subscribe("p", "")
	ch2, _ := svc.Subscribe("p", "")
	chOther, _ := svc.Subscribe("other", "")

	svc.dropAllSubscribersForPlugin("p")

	for i, ch := range []<-chan PluginSettingsChange{ch1, ch2} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatalf("ch%d expected closed, got open with value", i+1)
			}
		case <-time.After(time.Second):
			t.Fatalf("ch%d not closed after dropAll", i+1)
		}
	}

	// Other plugin's subscriber must remain open.
	select {
	case _, ok := <-chOther:
		if !ok {
			t.Fatal("other-plugin subscriber must NOT be closed")
		}
	case <-time.After(50 * time.Millisecond):
		// expected: still open, no events delivered
	}
}

func TestDropAll_ThenCleanupIsNoop(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := newPluginSettingsTestService(t)
	defer cleanup()

	_, sub1Cleanup := svc.Subscribe("p", "")
	svc.dropAllSubscribersForPlugin("p")
	// Calling cleanup after the channel was force-closed must NOT panic
	// (no double-close) — Subscribe's cleanup uses sync.Once + missing-id
	// branch and dropAll deletes the slice entry.
	sub1Cleanup()
	sub1Cleanup() // idempotent
}

func TestLookupDefault_SkipsSecret(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := newPluginSettingsTestService(t)
	defer cleanup()

	// Populate caches as RegisterSchemaWithInput would.
	svc.mu.Lock()
	svc.defaults["p"] = json.RawMessage(`{"normal":"hi","apikey":"DEFAULT"}`)
	svc.propertiesMeta["p"] = map[string]PropertyMetadata{
		"normal": {Visibility: PropertyVisibilityFrontend},
		"apikey": {Visibility: PropertyVisibilitySecret},
	}
	svc.mu.Unlock()

	if v, ok := svc.lookupDefault("p", "normal"); !ok {
		t.Fatal("normal key default must be returned")
	} else if !strings.Contains(string(v), "hi") {
		t.Fatalf("unexpected normal default: %s", v)
	}
	if _, ok := svc.lookupDefault("p", "apikey"); ok {
		t.Fatal("secret key must NOT return a default")
	}
}

func TestSeedDefaults_SkipsSecretAndStampsVersion(t *testing.T) {
	t.Parallel()
	svc, mock, cleanup := newPluginSettingsTestService(t)
	defer cleanup()

	// Set up cache state seedDefaults reads.
	svc.mu.Lock()
	svc.schemaVersions["p"] = "2.0.0"
	svc.propertiesMeta["p"] = map[string]PropertyMetadata{
		"frontend_key": {Visibility: PropertyVisibilityFrontend},
		"secret_key":   {Visibility: PropertyVisibilitySecret},
	}
	svc.mu.Unlock()

	// Only the frontend key should INSERT; the secret one is skipped.
	mock.ExpectExec(`
			INSERT INTO plugin_settings (plugin_name, key, value_json, revision, schema_version_at_write, updated_at)
			VALUES ($1, $2, $3::jsonb, 1, $4, NOW())
			ON CONFLICT (plugin_name, key) DO NOTHING
		`).
		WithArgs("p", "frontend_key", `"hello"`, "2.0.0").
		WillReturnResult(sqlmock.NewResult(0, 1))

	defaultsJSON := json.RawMessage(`{"frontend_key":"hello","secret_key":"NOPE"}`)
	if err := svc.seedDefaults(context.Background(), "p", defaultsJSON); err != nil {
		t.Fatalf("seedDefaults: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGetByKey_ReturnsSchemaVersions(t *testing.T) {
	t.Parallel()
	svc, mock, cleanup := newPluginSettingsTestService(t)
	defer cleanup()

	svc.mu.Lock()
	svc.schemaVersions["p"] = "2.0.0"
	svc.mu.Unlock()

	// Row exists: stored=1.0.0, current=2.0.0 (drift case).
	mock.ExpectQuery(`
		SELECT value_json::text, revision, schema_version_at_write FROM plugin_settings
		WHERE plugin_name = $1 AND key = $2
	`).WithArgs("p", "k").
		WillReturnRows(sqlmock.NewRows([]string{"value_json", "revision", "schema_version_at_write"}).
			AddRow("42", int64(7), "1.0.0"))

	res, err := svc.GetByKey(context.Background(), "p", "k")
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if string(res.Value) != "42" || res.Revision != 7 {
		t.Fatalf("value/revision wrong: %s %d", res.Value, res.Revision)
	}
	if res.StoredVersion != "1.0.0" || res.CurrentVersion != "2.0.0" {
		t.Fatalf("schema versions: stored=%q current=%q", res.StoredVersion, res.CurrentVersion)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestGetByKey_FallbackToDefault_StampsZeroStored(t *testing.T) {
	t.Parallel()
	svc, mock, cleanup := newPluginSettingsTestService(t)
	defer cleanup()

	svc.mu.Lock()
	svc.schemaVersions["p"] = "1.0.0"
	svc.defaults["p"] = json.RawMessage(`{"k":"defaulted"}`)
	svc.propertiesMeta["p"] = map[string]PropertyMetadata{
		"k": {Visibility: PropertyVisibilityFrontend},
	}
	svc.mu.Unlock()

	mock.ExpectQuery(`
		SELECT value_json::text, revision, schema_version_at_write FROM plugin_settings
		WHERE plugin_name = $1 AND key = $2
	`).WithArgs("p", "k").
		WillReturnRows(sqlmock.NewRows([]string{"value_json", "revision", "schema_version_at_write"}))

	res, err := svc.GetByKey(context.Background(), "p", "k")
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if !strings.Contains(string(res.Value), "defaulted") {
		t.Fatalf("expected default value, got %s", res.Value)
	}
	if res.Revision != 0 {
		t.Fatalf("synthetic default must carry revision=0, got %d", res.Revision)
	}
	if res.StoredVersion != schemaVersionUndeclared {
		t.Fatalf("synthetic default must carry stored=0, got %q", res.StoredVersion)
	}
	if res.CurrentVersion != "1.0.0" {
		t.Fatalf("currentVersion: got %q", res.CurrentVersion)
	}
}

func TestSubscribe_FanOut(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := newPluginSettingsTestService(t)
	defer cleanup()

	chAll, _ := svc.Subscribe("p", "")
	chKeyOnly, _ := svc.Subscribe("p", "interesting")

	go svc.notify(PluginSettingsChange{Plugin: "p", Key: "interesting", Value: json.RawMessage(`"v"`)})
	go svc.notify(PluginSettingsChange{Plugin: "p", Key: "boring", Value: json.RawMessage(`"v"`)})

	// chAll receives both; chKeyOnly only the matching one.
	gotAll := drainFor(t, chAll, 2, 200*time.Millisecond)
	if len(gotAll) != 2 {
		t.Fatalf("chAll got %d events", len(gotAll))
	}
	gotKey := drainFor(t, chKeyOnly, 1, 200*time.Millisecond)
	if len(gotKey) != 1 || gotKey[0].Key != "interesting" {
		t.Fatalf("chKeyOnly: %+v", gotKey)
	}
}

func drainFor(t *testing.T, ch <-chan PluginSettingsChange, want int, dur time.Duration) []PluginSettingsChange {
	t.Helper()
	out := make([]PluginSettingsChange, 0, want)
	deadline := time.After(dur)
	for len(out) < want {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}

// guard against accidental import cleanup (used in older test versions)
var _ sync.Mutex
