package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateStatementAllowlist(t *testing.T) {
	t.Parallel()

	newTables := map[relationName]struct{}{"i:new_table": {}}
	tests := map[string]struct {
		sql  string
		path string
	}{
		"create table":          {sql: `CREATE TABLE new_table (id bigint PRIMARY KEY);`},
		"generated column":      {sql: `CREATE TABLE new_table (id bigint, doubled bigint GENERATED ALWAYS AS (id * 2) STORED);`},
		"create type":           {sql: `CREATE TYPE status AS ENUM ('new', 'done');`},
		"concurrent index":      {sql: `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email ON users (email);`, path: "002_users_email_notx.sql"},
		"plain index new table": {sql: `CREATE INDEX idx_new_table_id ON new_table (id);`},
		"unique on new table":   {sql: `CREATE UNIQUE INDEX uq_new ON new_table (id);`},
		"nullable column":       {sql: `ALTER TABLE users ADD COLUMN IF NOT EXISTS note timestamp with time zone;`},
		"nullable array":        {sql: `ALTER TABLE users ADD COLUMN tags varchar(32)[];`},
		"plain insert":          {sql: `INSERT INTO settings (key, value) VALUES ('k', 'v');`},
		"literal rows":          {sql: `INSERT INTO settings (key, value) VALUES ('k1', NULL), ('k2', TRUE), ('k3', -1.25e+3);`},
		"conflict nothing":      {sql: `INSERT INTO settings (key) VALUES ('k') ON CONFLICT (key) DO NOTHING;`},
		"conflict constraint":   {sql: `INSERT INTO settings (key) VALUES ('k') ON CONFLICT ON CONSTRAINT settings_pkey DO NOTHING;`},
		"quoted column names":   {sql: `INSERT INTO settings ("key", "value") VALUES ('k', 'v');`},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			statement := mustOneStatement(t, test.sql)
			decision := evaluateStatement(statement, newTables, test.path)
			require.True(t, decision.allowed, decision.reason)
		})
	}
}

func TestEvaluateStatementRejectsUnsafeAndUnknownForms(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"drop":                   `DROP TABLE users;`,
		"truncate":               `TRUNCATE usage_logs;`,
		"update":                 `UPDATE users SET enabled = true;`,
		"delete":                 `DELETE FROM users;`,
		"merge":                  `MERGE INTO users USING staged ON false WHEN MATCHED THEN DELETE;`,
		"copy from":              `COPY users FROM '/tmp/users';`,
		"rename":                 `ALTER TABLE users RENAME COLUMN old TO new;`,
		"add default":            `ALTER TABLE users ADD COLUMN note text DEFAULT '';`,
		"add serial":             `ALTER TABLE users ADD COLUMN sequence_id bigserial;`,
		"add domain":             `ALTER TABLE users ADD COLUMN required_note required_text;`,
		"add constraint":         `ALTER TABLE users ADD CONSTRAINT users_key UNIQUE (email);`,
		"multiple add":           `ALTER TABLE users ADD COLUMN one text, ADD COLUMN two text;`,
		"conflict update":        `INSERT INTO settings (key) VALUES ('k') ON CONFLICT (key) DO UPDATE SET key = EXCLUDED.key;`,
		"partial conflict":       `INSERT INTO settings (key) VALUES ('k') ON CONFLICT (key) WHERE key <> '' DO NOTHING;`,
		"function value":         `INSERT INTO settings (key, value) VALUES ('k', NOW());`,
		"side effect function":   `INSERT INTO settings (key, value) VALUES ('k', setval('danger', 2));`,
		"default value":          `INSERT INTO settings (key, value) VALUES ('k', DEFAULT);`,
		"cast value":             `INSERT INTO settings (key, value) VALUES ('k', '2'::integer);`,
		"expression column list": `INSERT INTO settings (lower(key)) VALUES ('k');`,
		"query insert":           `INSERT INTO audit (id) SELECT id FROM users;`,
		"insert writable cte":    `INSERT INTO audit (id) WITH moved AS (DELETE FROM users RETURNING id) SELECT id FROM moved;`,
		"create table as":        `CREATE TABLE archived_users AS SELECT * FROM users;`,
		"create table writable":  `CREATE TABLE archived_users AS WITH moved AS (DELETE FROM users RETURNING *) SELECT * FROM moved;`,
		"create partition":       `CREATE TABLE child PARTITION OF usage_logs FOR VALUES FROM (1) TO (2);`,
		"create inherited table": `CREATE TABLE child (extra text) INHERITS (users);`,
		"create typed table":     `CREATE TABLE child OF existing_type;`,
		"unicode escaped table":  `CREATE TABLE IF NOT EXISTS U&"u\0073ers" (id bigint);`,
		"plain index existing":   `CREATE INDEX idx_users_email ON users (email);`,
		"unique existing table":  `CREATE UNIQUE INDEX users_email_uq ON users (email);`,
		"unknown create":         `CREATE VIEW enabled_users AS SELECT * FROM users;`,
	}
	for name, sql := range tests {
		name, sql := name, sql
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			statement := mustOneStatement(t, sql)
			decision := evaluateStatement(statement, nil, "002_test.sql")
			require.False(t, decision.allowed)
			require.NotEmpty(t, decision.reason)
		})
	}
}

func TestUnicodeEscapedIdentifierCannotMasqueradeAsNewTable(t *testing.T) {
	statements, err := scanSQLStatements([]byte(`
CREATE TABLE IF NOT EXISTS U&"u\0073ers" (id bigint);
CREATE INDEX idx_users_email ON U&"u\0073ers" (email);
`))
	require.NoError(t, err)
	require.Len(t, statements, 2)
	created := make(map[relationName]struct{})
	first := evaluateStatement(statements[0], created, "002_test.sql")
	recordNewTableProof(first, map[relationName]struct{}{"i:users": {}}, created)
	require.Empty(t, created)
	require.False(t, first.allowed)
	require.False(t, evaluateStatement(statements[1], created, "002_test.sql").allowed)
}

func TestStatementFingerprintIgnoresLayoutButPreservesLiteralData(t *testing.T) {
	t.Parallel()

	original := mustOneStatement(t, `INSERT INTO settings (key, value) VALUES ('k', 'v');`)
	commented := mustOneStatement(t, `
		-- an added comment must not hide a replay
		insert into settings(key,value)
		values ('k', 'v');
	`)
	changedData := mustOneStatement(t, `INSERT INTO settings (key, value) VALUES ('different', 'v');`)

	require.Equal(t, statementFingerprint(original), statementFingerprint(commented))
	require.NotEqual(t, statementFingerprint(original), statementFingerprint(changedData))
}

func TestReviewedCompatibleAnnotationIsNarrow(t *testing.T) {
	t.Parallel()

	reviewedFunction := mustOneStatement(t, `
-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION f() RETURNS void LANGUAGE sql AS $$ SELECT 1 $$;
`)
	require.True(t, evaluateStatement(reviewedFunction, nil, "002_test.sql").allowed)

	unreviewedFunction := mustOneStatement(t, `
CREATE OR REPLACE FUNCTION f() RETURNS void LANGUAGE sql AS $$ SELECT 1 $$;
`)
	require.False(t, evaluateStatement(unreviewedFunction, nil, "002_test.sql").allowed)

	reviewedDrop := mustOneStatement(t, `
-- sub2api-managed-update: reviewed-compatible
DROP TABLE users;
`)
	require.False(t, evaluateStatement(reviewedDrop, nil, "002_test.sql").allowed)

	reviewedUnknown := mustOneStatement(t, `
-- sub2api-managed-update: reviewed-compatible
DO $$ BEGIN PERFORM 1; END $$;
`)
	require.False(t, evaluateStatement(reviewedUnknown, nil, "002_test.sql").allowed)
}

func TestRecordNewTableProofRequiresUnconditionalEarlierCreate(t *testing.T) {
	t.Parallel()

	baseline := map[relationName]struct{}{"i:public.i:users": {}}
	created := make(map[relationName]struct{})
	for _, sql := range []string{
		`CREATE TABLE "users" (id bigint);`,
		`CREATE TABLE IF NOT EXISTS conditional_table (id bigint);`,
		`CREATE TABLE new_table (id bigint);`,
	} {
		decision := evaluateStatement(mustOneStatement(t, sql), created, "002_test.sql")
		recordNewTableProof(decision, baseline, created)
	}
	require.NotContains(t, created, relationName("i:users"))
	require.NotContains(t, created, relationName("i:conditional_table"))
	require.Contains(t, created, relationName("i:new_table"))

	indexBeforeCreate := evaluateStatement(
		mustOneStatement(t, `CREATE INDEX idx_later ON later_table (id);`),
		created,
		"002_test.sql",
	)
	require.False(t, indexBeforeCreate.allowed)
}

func TestConditionalOrLaterCreateDoesNotAuthorizeBlockingIndex(t *testing.T) {
	t.Parallel()

	created := make(map[relationName]struct{})
	conditional := evaluateStatement(
		mustOneStatement(t, `CREATE TABLE IF NOT EXISTS dynamic_partition (id bigint);`),
		created,
		"002_dynamic.sql",
	)
	require.True(t, conditional.allowed)
	recordNewTableProof(conditional, nil, created)
	require.Empty(t, created)
	require.False(t, evaluateStatement(
		mustOneStatement(t, `CREATE UNIQUE INDEX uq_dynamic ON dynamic_partition (id);`),
		created,
		"002_dynamic.sql",
	).allowed)

	require.False(t, evaluateStatement(
		mustOneStatement(t, `CREATE INDEX idx_later ON later_table (id);`),
		created,
		"003_order.sql",
	).allowed)
	unconditional := evaluateStatement(
		mustOneStatement(t, `CREATE TABLE later_table (id bigint);`),
		created,
		"003_order.sql",
	)
	recordNewTableProof(unconditional, nil, created)
	require.Contains(t, created, relationName("i:later_table"))
}

func TestUniqueIndexRequiresExactNewTableRelation(t *testing.T) {
	t.Parallel()

	qualified := mustOneStatement(t, `CREATE UNIQUE INDEX uq ON app.new_table (id);`)
	unqualified := mustOneStatement(t, `CREATE UNIQUE INDEX uq ON new_table (id);`)

	require.True(t, evaluateStatement(
		qualified,
		map[relationName]struct{}{"i:app.i:new_table": {}},
		"002_test.sql",
	).allowed)
	require.False(t, evaluateStatement(
		qualified,
		map[relationName]struct{}{"i:new_table": {}},
		"002_test.sql",
	).allowed)
	require.False(t, evaluateStatement(
		unqualified,
		map[relationName]struct{}{"i:app.i:new_table": {}},
		"002_test.sql",
	).allowed)
}

func TestIndexExecutionModeMatchesTargetTableLifecycle(t *testing.T) {
	t.Parallel()

	newTables := map[relationName]struct{}{"i:new_table": {}}
	tests := []struct {
		name    string
		sql     string
		path    string
		tables  map[relationName]struct{}
		allowed bool
	}{
		{
			name:    "concurrent existing table in notx migration",
			sql:     `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email ON users (email);`,
			path:    "002_users_email_notx.sql",
			allowed: true,
		},
		{
			name: "plain existing table index blocks writes",
			sql:  `CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);`,
			path: "002_users_email.sql",
		},
		{
			name: "concurrent index in transactional migration",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email ON users (email);`,
			path: "002_users_email.sql",
		},
		{
			name: "concurrent index without restart guard",
			sql:  `CREATE INDEX CONCURRENTLY idx_users_email ON users (email);`,
			path: "002_users_email_notx.sql",
		},
		{
			name:    "plain index on table created in release",
			sql:     `CREATE INDEX idx_new_table_id ON new_table (id);`,
			path:    "002_new_table.sql",
			tables:  newTables,
			allowed: true,
		},
		{
			name:   "plain index cannot run in notx migration",
			sql:    `CREATE INDEX idx_new_table_id ON new_table (id);`,
			path:   "002_new_table_notx.sql",
			tables: newTables,
		},
		{
			name: "non-index statement cannot run in notx migration",
			sql:  `ALTER TABLE users ADD COLUMN note text;`,
			path: "002_users_note_notx.sql",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decision := evaluateStatement(mustOneStatement(t, test.sql), test.tables, test.path)
			require.Equal(t, test.allowed, decision.allowed, decision.reason)
			if !test.allowed {
				require.NotEmpty(t, decision.reason)
			}
		})
	}
}

func mustOneStatement(t *testing.T, sql string) sqlStatement {
	t.Helper()
	statements, err := scanSQLStatements([]byte(sql))
	require.NoError(t, err)
	require.Len(t, statements, 1)
	return statements[0]
}
