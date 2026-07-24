package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanSQLStatementsPostgreSQLLexicalRegions(t *testing.T) {
	t.Parallel()

	src := []byte(`
-- a semicolon here does not end a statement: ; DROP TABLE users;
INSERT INTO notes (body) VALUES (E'quote\'; -- still a string');
/* outer ; DROP TABLE x; /* nested ; */ still a comment */
-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION test_fn()
RETURNS void LANGUAGE plpgsql AS $body$
BEGIN
  PERFORM 'semi;colon';
END;
$body$;
ALTER/**/TABLE users ADD COLUMN note text;
`)

	statements, err := scanSQLStatements(src)
	require.NoError(t, err)
	require.Len(t, statements, 3)
	require.Equal(t, "INSERT", keywordAt(statements[0].tokens, 0))
	require.True(t, statements[1].reviewedCompatible)
	require.True(t, keywordsAt(statements[1].tokens, 0, "CREATE", "OR", "REPLACE", "FUNCTION"))
	require.False(t, statements[2].reviewedCompatible)
	require.True(t, keywordsAt(statements[2].tokens, 0, "ALTER", "TABLE"))
}

func TestScanSQLStatementsDoesNotAcceptEmbeddedAnnotation(t *testing.T) {
	t.Parallel()

	statements, err := scanSQLStatements([]byte(`
INSERT INTO notes (body) VALUES ('-- sub2api-managed-update: reviewed-compatible');
CREATE FUNCTION test_fn() RETURNS void LANGUAGE sql AS $$ SELECT 1 $$;
`))
	require.NoError(t, err)
	require.Len(t, statements, 2)
	require.False(t, statements[0].reviewedCompatible)
	require.False(t, statements[1].reviewedCompatible)
}

func TestScanSQLStatementsUESCAPEDoesNotChangeQuoteBoundaries(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"unicode string": `
INSERT INTO notes (body) VALUES (U&'safe\' UESCAPE '!');
DROP TABLE users;
-- '
`,
		"unicode identifier": `
CREATE TABLE U&"safe\" UESCAPE '!' (id bigint);
DROP TABLE users;
-- "
`,
	}
	for name, sql := range tests {
		name, sql := name, sql
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			statements, err := scanSQLStatements([]byte(sql))
			require.NoError(t, err)
			require.Len(t, statements, 2)
			require.Equal(t, "DROP", keywordAt(statements[1].tokens, 0))
			if name == "unicode identifier" {
				require.Equal(t, tokenUnicodeIdentifier, statements[0].tokens[2].kind)
			}
		})
	}
}

func TestScanSQLStatementsRejectsUnterminatedRegions(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"single quote":  "INSERT INTO t VALUES ('unterminated);",
		"double quote":  `CREATE TABLE "unterminated (id bigint);`,
		"dollar quote":  "CREATE FUNCTION f() RETURNS void AS $tag$ BEGIN;",
		"block comment": "CREATE TABLE t (id bigint); /* unterminated",
	}
	for name, sql := range tests {
		name, sql := name, sql
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := scanSQLStatements([]byte(sql))
			require.Error(t, err)
		})
	}
}

func TestScanSQLStatementsRecognizesNumericLiterals(t *testing.T) {
	t.Parallel()

	statement := mustOneStatement(t, `INSERT INTO values_table VALUES (1, -2, .5, 6., 7e3, 8.5E-2);`)
	var numbers []string
	for _, token := range statement.tokens {
		if token.kind == tokenNumber {
			numbers = append(numbers, token.text)
		}
	}
	require.Equal(t, []string{"1", "2", ".5", "6.", "7e3", "8.5E-2"}, numbers)
}

func TestScanSQLStatementsRejectsIdentifiersPostgreSQLWouldFoldOrTruncateDifferently(t *testing.T) {
	t.Parallel()

	_, err := scanSQLStatements([]byte("CREATE TABLE usu\xc3\xa1rios (id bigint);"))
	require.ErrorContains(t, err, "non-ASCII unquoted identifiers")

	longName := strings.Repeat("a", postgresIdentifierMaxBytes+1)
	_, err = scanSQLStatements([]byte("CREATE TABLE " + longName + " (id bigint);"))
	require.ErrorContains(t, err, "63-byte limit")

	_, err = scanSQLStatements([]byte(`CREATE TABLE "` + longName + `" (id bigint);`))
	require.ErrorContains(t, err, "63-byte limit")
}
