package plugin

import (
	"errors"
	"strings"
	"testing"
)

// Tests for the P12·B-1 SQL table allow-list gate. They cover the five
// scenarios called out in the design:
//   1. plugin queries an owned table → ok
//   2. plugin queries an unowned table without db.core.read → denied
//   3. plugin with db.core.read queries a host shared whitelist table → ok
//   4. plugin with db.core.read queries a host table NOT in the whitelist → denied
//   5. plugin tries to write but only holds db.own.read → denied

func newTestRegistry(plugin string, caps []string, tables []string) *pluginCapabilityRegistry {
	r := newPluginCapabilityRegistry()
	r.Set(plugin, caps)
	r.SetTables(plugin, tables)
	return r
}

func TestSQLGate_OwnedTableQueryAllowed(t *testing.T) {
	reg := newTestRegistry("ch", []string{"db.own.read", "db.own.write"}, []string{"channel_monitors"})
	g := newSQLGate(reg)
	if err := g.Authorize("ch", "SELECT id FROM channel_monitors WHERE id = $1"); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
}

func TestSQLGate_UnownedTableQueryDenied(t *testing.T) {
	reg := newTestRegistry("ch", []string{"db.own.read", "db.own.write"}, []string{"channel_monitors"})
	g := newSQLGate(reg)
	err := g.Authorize("ch", "SELECT id FROM users WHERE id = $1")
	if err == nil {
		t.Fatal("expected denial")
	}
	var pe *PermissionDeniedError
	if !errors.As(err, &pe) {
		t.Fatalf("expected PermissionDeniedError, got %T %v", err, err)
	}
	if pe.Table != "users" {
		t.Fatalf("expected table=users, got %s", pe.Table)
	}
}

func TestSQLGate_HostSharedWithCoreReadAllowed(t *testing.T) {
	reg := newTestRegistry("ch",
		[]string{"db.own.read", "db.own.write", "db.core.read"},
		[]string{"channel_monitors"})
	g := newSQLGate(reg)
	if err := g.Authorize("ch", "SELECT id FROM users WHERE id = $1"); err != nil {
		t.Fatalf("expected allow with db.core.read, got %v", err)
	}
	// Even with db.core.read, payment_orders is in whitelist → allowed.
	if err := g.Authorize("ch", "SELECT * FROM payment_orders WHERE user_id = $1"); err != nil {
		t.Fatalf("expected payment_orders allow, got %v", err)
	}
}

func TestSQLGate_NonWhitelistedTableStillDeniedWithCoreRead(t *testing.T) {
	reg := newTestRegistry("ch",
		[]string{"db.own.read", "db.core.read"},
		[]string{"channel_monitors"})
	g := newSQLGate(reg)
	// api_keys is not on hostSharedReadableTables → still denied.
	err := g.Authorize("ch", "SELECT raw_key FROM api_keys WHERE user_id = $1")
	if err == nil {
		t.Fatal("expected denial — api_keys is not in host shared whitelist")
	}
	var pe *PermissionDeniedError
	if !errors.As(err, &pe) || pe.Table != "api_keys" {
		t.Fatalf("expected denial on api_keys, got %v", err)
	}
}

func TestSQLGate_WriteWithOnlyReadCapDenied(t *testing.T) {
	// Plugin owns table but only holds db.own.read; INSERT must fail.
	reg := newTestRegistry("ch",
		[]string{"db.own.read"},
		[]string{"channel_monitors"})
	g := newSQLGate(reg)
	err := g.Authorize("ch", "INSERT INTO channel_monitors (id, name) VALUES ($1, $2)")
	if err == nil {
		t.Fatal("expected denial — db.own.write not held")
	}
	var pe *PermissionDeniedError
	if !errors.As(err, &pe) || pe.Table != "channel_monitors" {
		t.Fatalf("expected denial on channel_monitors, got %v", err)
	}
}

// TestSQLGate_EmptyPluginNameRejected: 旧实现遇到空 plugin name 直接放行,
// 这成为攻击者绕过 gate 的天然通道。fail-closed 后空 caller 必须立即拒绝,
// 与 RequirePluginIdentity 拦截器形成双重防御。
func TestSQLGate_EmptyPluginNameRejected(t *testing.T) {
	reg := newTestRegistry("ch",
		[]string{"db.own.read"},
		[]string{"channel_monitors"})
	g := newSQLGate(reg)
	err := g.Authorize("", "SELECT * FROM users")
	if err == nil {
		t.Fatal("expected denial for empty plugin name (fail-closed)")
	}
	var pe *PermissionDeniedError
	if !errors.As(err, &pe) {
		t.Fatalf("expected PermissionDeniedError, got %T %v", err, err)
	}
}

func TestSQLGate_UnparsableSQLRejected(t *testing.T) {
	reg := newTestRegistry("ch", []string{"db.own.read"}, []string{"channel_monitors"})
	g := newSQLGate(reg)
	err := g.Authorize("ch", "VACUUM ANALYZE")
	if !errors.Is(err, ErrTableGateUnparsable) {
		t.Fatalf("expected ErrTableGateUnparsable, got %v", err)
	}
}

// --- extractor unit tests --------------------------------------------------

func TestExtractSQLTables_VariousShapes(t *testing.T) {
	cases := []struct {
		name   string
		sql    string
		reads  []string
		writes []string
	}{
		{
			name:  "simple select",
			sql:   "SELECT id FROM users",
			reads: []string{"users"},
		},
		{
			name:  "schema qualified",
			sql:   "SELECT id FROM public.users u",
			reads: []string{"users"},
		},
		{
			name:  "join",
			sql:   "SELECT a.id FROM channels a JOIN channel_groups b ON a.id = b.channel_id",
			reads: []string{"channels", "channel_groups"},
		},
		{
			name:   "insert",
			sql:    "INSERT INTO channel_monitors (id) VALUES ($1)",
			writes: []string{"channel_monitors"},
		},
		{
			name:   "update",
			sql:    "UPDATE channel_monitors SET name = $1 WHERE id = $2",
			writes: []string{"channel_monitors"},
		},
		{
			name:   "delete",
			sql:    "DELETE FROM channel_monitors WHERE id = $1",
			writes: []string{"channel_monitors"},
		},
		{
			name:   "delete with subquery",
			sql:    "DELETE FROM channel_monitors WHERE channel_id IN (SELECT id FROM channels)",
			writes: []string{"channel_monitors"},
			reads:  []string{"channels"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := extractSQLTables(tc.sql)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !sliceSetEqual(r, tc.reads) {
				t.Errorf("reads: want %v got %v", tc.reads, r)
			}
			if !sliceSetEqual(w, tc.writes) {
				t.Errorf("writes: want %v got %v", tc.writes, w)
			}
		})
	}
}

func sliceSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]struct{}, len(a))
	for _, v := range a {
		m[strings.ToLower(v)] = struct{}{}
	}
	for _, v := range b {
		if _, ok := m[strings.ToLower(v)]; !ok {
			return false
		}
	}
	return true
}
