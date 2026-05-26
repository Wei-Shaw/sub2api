package plugin

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sql_table_gate.go implements the SQL table allow-list introduced in
// P12·B-1. The host SDKServer's Query/Exec/TxQuery/TxExec methods consult
// this gate before forwarding plugin SQL to the database.
//
// The implementation uses a regex parser as a pragmatic MVP. It catches the
// common SQL shapes plugins emit (FROM <t>, JOIN <t>, INSERT INTO <t>,
// UPDATE <t>, DELETE FROM <t>). It is intentionally conservative — when the
// parser cannot identify any tables it returns ErrTableGateUnparsable, which
// the caller turns into PERMISSION_DENIED so a malformed query never bypasses
// enforcement. A follow-up will replace the regex with a real PostgreSQL
// parser (cockroachdb/sqlparser or auxten/postgresql-parser); the API stays
// stable.
//
// Known limitations of the regex approach (logged here so reviewers /
// future maintainers can decide what to upgrade):
//   1. Subqueries inside DML are detected only by whatever FROM/JOIN
//      patterns the regex matches — deeply nested SELECTs may miss tables
//      that only appear in CTE expressions referenced by alias later in
//      the same query.
//   2. Schema-qualified names ("public.users") are normalised to the bare
//      table name; this matches how plugins declare OwnedTables today and
//      keeps the gate consistent with manifest entries.
//   3. Function-table syntax (FROM unnest(...)) is treated as a no-op —
//      no real table is named, so AllowedTable is not consulted; the
//      query still goes through if at least one real table is present
//      and authorised, otherwise ErrTableGateUnparsable.
//   4. Quoted identifiers ("FROM \"users\"") are unwrapped before lookup.

// ErrTableGateUnparsable is returned when the regex parser cannot identify
// any table reference in the supplied SQL. The caller should reject the
// query — silently allowing unparsable SQL would defeat the gate.
var ErrTableGateUnparsable = errors.New("sql gate: cannot identify any table in query")

// PermissionDeniedError describes a plugin whose query was blocked by the
// SQL gate. The caller wraps it in a gRPC PERMISSION_DENIED status; tests
// errors.Is against this sentinel to assert specific tables.
type PermissionDeniedError struct {
	Plugin string
	Table  string
	Reason string
}

func (e *PermissionDeniedError) Error() string {
	return fmt.Sprintf("sql gate: plugin %q denied access to table %q (%s)",
		e.Plugin, e.Table, e.Reason)
}

// sqlGate guards plugin SQL by extracting the touched tables and consulting
// the capability registry. A nil receiver makes the methods no-ops, which
// the SDKServer leverages when it is constructed without a plugin context
// (tests / standalone usage).
type sqlGate struct {
	registry *pluginCapabilityRegistry
}

func newSQLGate(reg *pluginCapabilityRegistry) *sqlGate {
	return &sqlGate{registry: reg}
}

// Authorize parses the query and rejects it when the plugin lacks rights to
// any touched table. An empty plugin name is now a hard rejection (fail-closed)
// — RequirePluginIdentity{Unary,Stream} interceptor 已经在生产链路上把匿名
// 调用挡掉, 但这里仍然 fail-closed 以确保 gate 自身不会因为上层链路漏装
// 拦截器而被绕过。任何到达 SQL gate 而没有 plugin name 的调用都视为攻击面。
func (g *sqlGate) Authorize(pluginName, query string) error {
	if g == nil || g.registry == nil {
		return nil
	}
	if pluginName == "" {
		return &PermissionDeniedError{
			Plugin: "",
			Table:  "(unresolved)",
			Reason: "anonymous caller — plugin identity missing on SQL request",
		}
	}
	reads, writes, err := extractSQLTables(query)
	if err != nil {
		return err
	}
	for _, t := range reads {
		if !g.registry.AllowedTable(pluginName, t, false) {
			return &PermissionDeniedError{
				Plugin: pluginName,
				Table:  t,
				Reason: "table not in plugin owned_tables and either not in host shared whitelist or plugin lacks db.core.read",
			}
		}
	}
	for _, t := range writes {
		if !g.registry.AllowedTable(pluginName, t, true) {
			return &PermissionDeniedError{
				Plugin: pluginName,
				Table:  t,
				Reason: "table not in plugin owned_tables and either not in host shared whitelist or plugin lacks db.core.write",
			}
		}
	}
	return nil
}

// --- Regex-based extractor --------------------------------------------------

// identPattern matches a single SQL identifier, optionally schema-qualified
// and optionally double-quoted. Captures the raw match; normalisation happens
// in normaliseTable.
const identPattern = `(?:"[^"]+"|[A-Za-z_][A-Za-z0-9_]*)(?:\.(?:"[^"]+"|[A-Za-z_][A-Za-z0-9_]*))?`

var (
	// Read references: FROM <t>, JOIN <t>.
	reFromOrJoin = regexp.MustCompile(`(?i)(?:\b(?:FROM|JOIN)\b)\s+(` + identPattern + `)`)

	// Write references.
	reInsertInto = regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+(` + identPattern + `)`)
	reUpdate     = regexp.MustCompile(`(?i)\bUPDATE\s+(?:ONLY\s+)?(` + identPattern + `)`)
	reDeleteFrom = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+(?:ONLY\s+)?(` + identPattern + `)`)

	// Reserved words sometimes appear in those positions (e.g. UPDATE SET ...
	// inside RETURNING). The blacklist filters them out so we do not flag a
	// keyword as a missing table.
	sqlKeywordBlacklist = map[string]struct{}{
		"select": {}, "where": {}, "set": {}, "values": {}, "returning": {},
		"as": {}, "on": {}, "and": {}, "or": {}, "with": {}, "case": {},
		"when": {}, "then": {}, "else": {}, "end": {}, "into": {}, "only": {},
	}
)

// extractSQLTables returns (reads, writes) — distinct table names the query
// touches. Tables appearing on the write side (INSERT/UPDATE/DELETE target)
// are *not* duplicated into reads. ErrTableGateUnparsable is returned when
// no table reference is found at all.
func extractSQLTables(query string) (reads, writes []string, err error) {
	stripped := stripSQLComments(query)

	writeSet := map[string]struct{}{}
	for _, re := range []*regexp.Regexp{reInsertInto, reUpdate, reDeleteFrom} {
		for _, m := range re.FindAllStringSubmatch(stripped, -1) {
			if t := normaliseTable(m[1]); t != "" {
				writeSet[t] = struct{}{}
			}
		}
	}

	readSet := map[string]struct{}{}
	for _, m := range reFromOrJoin.FindAllStringSubmatch(stripped, -1) {
		t := normaliseTable(m[1])
		if t == "" {
			continue
		}
		// FROM after DELETE is a write — skip the duplicate read entry so we
		// do not require both db.own.read and db.own.write for plain DELETEs.
		if _, alreadyWrite := writeSet[t]; alreadyWrite {
			continue
		}
		readSet[t] = struct{}{}
	}

	if len(readSet) == 0 && len(writeSet) == 0 {
		return nil, nil, ErrTableGateUnparsable
	}
	return setToSlice(readSet), setToSlice(writeSet), nil
}

func setToSlice(s map[string]struct{}) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	return out
}

// normaliseTable strips quoting + schema prefix and lowercases the result.
// "public.users" and "\"users\"" both become "users".
func normaliseTable(raw string) string {
	if raw == "" {
		return ""
	}
	// Drop schema prefix: "public.users" -> "users"
	if i := strings.LastIndex(raw, "."); i >= 0 {
		raw = raw[i+1:]
	}
	raw = strings.Trim(raw, `"`)
	raw = strings.ToLower(strings.TrimSpace(raw))
	if _, blacklisted := sqlKeywordBlacklist[raw]; blacklisted {
		return ""
	}
	return raw
}

// stripSQLComments removes -- line comments and /* block */ comments so the
// regex extractor does not match table-like tokens hiding in them.
func stripSQLComments(q string) string {
	// Block comments first (they may span lines).
	for {
		i := strings.Index(q, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(q[i+2:], "*/")
		if j < 0 {
			q = q[:i]
			break
		}
		q = q[:i] + " " + q[i+2+j+2:]
	}
	// Then line comments.
	var b strings.Builder
	for _, line := range strings.Split(q, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		// strings.Builder.WriteString / WriteByte 文档保证永远返回 nil error
		// (内部 b.buf 是切片 append, 没有 io 失败路径), 这里显式 _ 忽略以满足
		// errcheck 的 disable-default-exclusions=true 配置。
		_, _ = b.WriteString(line)
		_ = b.WriteByte('\n')
	}
	return b.String()
}

// sqlGateStatus turns a sqlGate.Authorize error into a gRPC error suitable
// for returning from SQLProxy methods. PermissionDeniedError maps to
// codes.PermissionDenied; ErrTableGateUnparsable maps to InvalidArgument
// because the plugin sent SQL the gate cannot reason about. Any other
// error path is treated as Internal — the gate should not fail unless
// the regex parser itself errored, which is unreachable today.
func sqlGateStatus(err error) error {
	if err == nil {
		return nil
	}
	var deniedErr *PermissionDeniedError
	if errors.As(err, &deniedErr) {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	if errors.Is(err, ErrTableGateUnparsable) {
		return status.Errorf(codes.InvalidArgument, "plugin SQL cannot be authorised: %s", err.Error())
	}
	return status.Errorf(codes.Internal, "plugin SQL gate: %v", err)
}
