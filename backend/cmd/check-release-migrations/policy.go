package main

import (
	"fmt"
	"strings"
)

type relationName string

type policyDecision struct {
	allowed                bool
	reviewEligible         bool
	createdTable           relationName
	conditionalCreateTable bool
	referencedTable        relationName
	index                  bool
	concurrentIndex        bool
	idempotentIndex        bool
	uniqueIndex            bool
	description            string
	reason                 string
}

func evaluateStatement(statement sqlStatement, newlyCreatedTables map[relationName]struct{}, migrationPath string) policyDecision {
	tokens := statement.tokens
	if len(tokens) == 0 {
		return policyDecision{allowed: true, description: "empty statement"}
	}
	if strings.HasSuffix(strings.ToLower(migrationPath), "_notx.sql") && !isConcurrentCreateIndex(tokens) {
		return rejected("non-transactional migration", "*_notx.sql migrations only support CREATE INDEX CONCURRENTLY statements")
	}

	switch keywordAt(tokens, 0) {
	case "CREATE":
		decision := evaluateCreate(tokens)
		if decision.index && decision.allowed {
			// A qualified and an unqualified relation can resolve to different
			// tables under search_path. Only an exact canonical name proves that
			// an index targets a table created in this migration set.
			_, tableCreatedInRelease := newlyCreatedTables[decision.referencedTable]
			if decision.uniqueIndex && !tableCreatedInRelease {
				decision.allowed = false
				decision.reason = fmt.Sprintf("unique index targets %s, which was not created after the schema baseline", displayRelation(decision.referencedTable))
			} else if !tableCreatedInRelease && !decision.concurrentIndex {
				decision.allowed = false
				decision.reason = fmt.Sprintf("index targets previously released table %s and must use CREATE INDEX CONCURRENTLY in a *_notx.sql migration", displayRelation(decision.referencedTable))
			} else if decision.concurrentIndex && !strings.HasSuffix(strings.ToLower(migrationPath), "_notx.sql") {
				decision.allowed = false
				decision.reason = "CREATE INDEX CONCURRENTLY must be placed in a *_notx.sql migration"
			} else if decision.concurrentIndex && !decision.idempotentIndex {
				decision.allowed = false
				decision.reason = "CREATE INDEX CONCURRENTLY must include IF NOT EXISTS for restart-safe execution"
			} else if !decision.concurrentIndex && strings.HasSuffix(strings.ToLower(migrationPath), "_notx.sql") {
				decision.allowed = false
				decision.reason = "plain CREATE INDEX cannot be placed in a *_notx.sql migration"
			}
		}
		if !decision.allowed && decision.reviewEligible && statement.reviewedCompatible {
			decision.allowed = true
			decision.reason = ""
		}
		return decision

	case "ALTER":
		decision := evaluateAlter(tokens)
		if !decision.allowed && decision.reviewEligible && statement.reviewedCompatible {
			decision.allowed = true
			decision.reason = ""
		}
		return decision

	case "INSERT":
		return evaluateInsert(tokens)

	case "DROP", "TRUNCATE", "UPDATE", "DELETE", "MERGE", "COPY":
		return policyDecision{
			description: "destructive or data-rewrite statement",
			reason:      fmt.Sprintf("top-level %s is never rollback-compatible", keywordAt(tokens, 0)),
		}

	default:
		return policyDecision{
			description: "unknown statement",
			reason:      fmt.Sprintf("top-level %s is not in the managed-update allowlist", printableKeyword(tokens[0])),
		}
	}
}

func isConcurrentCreateIndex(tokens []sqlToken) bool {
	return keywordsAt(tokens, 0, "CREATE", "INDEX", "CONCURRENTLY") ||
		keywordsAt(tokens, 0, "CREATE", "UNIQUE", "INDEX", "CONCURRENTLY")
}

func evaluateCreate(tokens []sqlToken) policyDecision {
	switch {
	case keywordsAt(tokens, 0, "CREATE", "TABLE"):
		conditional := keywordsAt(tokens, 2, "IF", "NOT", "EXISTS")
		name, next, ok := parseRelation(tokens, skipIfNotExists(tokens, 2))
		if !ok {
			return rejected("CREATE TABLE", "could not determine the created table")
		}
		afterColumns, columnsOK := skipBalancedParentheses(tokens, next)
		if !columnsOK || afterColumns != len(tokens) {
			return rejected("CREATE TABLE", "only a standalone parenthesized column definition is in the additive allowlist; AS, PARTITION OF, INHERITS, typed OF, and trailing table clauses are rejected")
		}
		return policyDecision{
			allowed:                true,
			createdTable:           name,
			conditionalCreateTable: conditional,
			description:            "CREATE TABLE",
		}

	case keywordsAt(tokens, 0, "CREATE", "TYPE"), keywordsAt(tokens, 0, "CREATE", "DOMAIN"):
		return policyDecision{allowed: true, description: "new PostgreSQL type"}

	case keywordsAt(tokens, 0, "CREATE", "INDEX"):
		return evaluateIndex(tokens, 2, false)

	case keywordsAt(tokens, 0, "CREATE", "UNIQUE", "INDEX"):
		return evaluateIndex(tokens, 3, true)

	case keywordsAt(tokens, 0, "CREATE", "FUNCTION"),
		keywordsAt(tokens, 0, "CREATE", "PROCEDURE"),
		keywordsAt(tokens, 0, "CREATE", "TRIGGER"),
		keywordsAt(tokens, 0, "CREATE", "OR", "REPLACE", "FUNCTION"),
		keywordsAt(tokens, 0, "CREATE", "OR", "REPLACE", "PROCEDURE"):
		return policyDecision{
			reviewEligible: true,
			description:    "behavior-defining CREATE statement",
			reason:         fmt.Sprintf("%s requires a leading %q annotation after explicit compatibility review", statementPrefix(tokens), reviewedCompatibleAnnotation),
		}

	default:
		return rejected("unknown CREATE statement", "CREATE form is not in the managed-update allowlist")
	}
}

func evaluateIndex(tokens []sqlToken, indexPos int, unique bool) policyDecision {
	i := indexPos
	concurrent := keywordAt(tokens, i) == "CONCURRENTLY"
	if concurrent {
		i++
	}
	idempotent := keywordsAt(tokens, i, "IF", "NOT", "EXISTS")
	i = skipIfNotExists(tokens, i)
	_, i, ok := parseRelation(tokens, i) // index name
	if !ok || keywordAt(tokens, i) != "ON" {
		return rejected("CREATE INDEX", "could not determine the indexed table")
	}
	i++
	if keywordAt(tokens, i) == "ONLY" {
		i++
	}
	table, _, ok := parseRelation(tokens, i)
	if !ok {
		return rejected("CREATE INDEX", "could not determine the indexed table")
	}
	return policyDecision{
		allowed:         true,
		index:           true,
		concurrentIndex: concurrent,
		idempotentIndex: idempotent,
		uniqueIndex:     unique,
		referencedTable: table,
		description:     map[bool]string{true: "CREATE UNIQUE INDEX", false: "CREATE INDEX"}[unique],
	}
}

func evaluateAlter(tokens []sqlToken) policyDecision {
	if !keywordsAt(tokens, 0, "ALTER", "TABLE") {
		return rejected("ALTER statement", "only plain nullable ALTER TABLE ... ADD COLUMN is allowed")
	}

	i := 2
	if keywordsAt(tokens, i, "IF", "EXISTS") {
		i += 2
	}
	if keywordAt(tokens, i) == "ONLY" {
		i++
	}
	_, i, ok := parseRelation(tokens, i)
	if !ok {
		return rejected("ALTER TABLE", "could not determine the altered table")
	}
	if keywordAt(tokens, i) == "DROP" {
		i++
		if keywordAt(tokens, i) != "CONSTRAINT" {
			return rejected("ALTER TABLE", "only plain nullable ADD COLUMN is allowed")
		}
		i++
		if keywordsAt(tokens, i, "IF", "EXISTS") {
			i += 2
		}
		if _, next, ok := parseIdentifier(tokens, i); !ok || next != len(tokens) {
			return rejected("ALTER TABLE DROP CONSTRAINT", "only a single named constraint can be reviewed")
		}
		return policyDecision{
			reviewEligible: true,
			description:    "ALTER TABLE DROP CONSTRAINT",
			reason:         fmt.Sprintf("constraint removal requires a leading %q annotation after explicit compatibility review", reviewedCompatibleAnnotation),
		}
	}
	if keywordAt(tokens, i) != "ADD" {
		return rejected("ALTER TABLE", "only plain nullable ADD COLUMN is allowed")
	}
	i++
	if keywordAt(tokens, i) == "COLUMN" {
		i++
	}
	if keywordsAt(tokens, i, "IF", "NOT", "EXISTS") {
		i += 3
	}
	if keywordAt(tokens, i) == "CONSTRAINT" {
		i++
		if _, next, ok := parseIdentifier(tokens, i); !ok || keywordAt(tokens, next) != "CHECK" {
			return rejected("ALTER TABLE ADD CONSTRAINT", "only CHECK constraints can be explicitly reviewed")
		} else {
			afterCheck, balanced := skipBalancedParentheses(tokens, next+1)
			validSuffix := afterCheck == len(tokens) ||
				keywordsAt(tokens, afterCheck, "NOT", "VALID") && afterCheck+2 == len(tokens)
			if !balanced || !validSuffix {
				return rejected("ALTER TABLE ADD CONSTRAINT CHECK", "expected one CHECK expression with an optional NOT VALID clause")
			}
		}
		return policyDecision{
			reviewEligible: true,
			description:    "ALTER TABLE ADD CONSTRAINT CHECK",
			reason:         fmt.Sprintf("CHECK constraints on existing tables require a leading %q annotation after explicit compatibility review", reviewedCompatibleAnnotation),
		}
	}
	if _, next, ok := parseIdentifier(tokens, i); !ok {
		return rejected("ALTER TABLE ADD COLUMN", "could not determine the new column")
	} else {
		i = next
	}
	next, reason := consumeNullableBuiltinType(tokens, i)
	if reason != "" {
		return rejected("ALTER TABLE ADD COLUMN", reason)
	}
	if keywordAt(tokens, next) == "NULL" {
		next++
	}
	if next != len(tokens) {
		return rejected(
			"ALTER TABLE ADD COLUMN",
			fmt.Sprintf("column modifier %s is outside the plain nullable-column allowlist", printableKeyword(tokens[next])),
		)
	}

	return policyDecision{allowed: true, description: "plain nullable ADD COLUMN using a known nullable built-in type"}
}

func consumeNullableBuiltinType(tokens []sqlToken, start int) (int, string) {
	first := keywordAt(tokens, start)
	if first == "" {
		return start, "column type must be an unquoted PostgreSQL built-in type"
	}
	switch first {
	case "SMALLSERIAL", "SERIAL", "BIGSERIAL", "SERIAL2", "SERIAL4", "SERIAL8":
		return start, fmt.Sprintf("%s has implicit NOT NULL, sequence, and default behavior", first)
	}

	i := start + 1
	switch first {
	case "DOUBLE":
		if keywordAt(tokens, i) != "PRECISION" {
			return start, "DOUBLE must use the built-in DOUBLE PRECISION type"
		}
		i++
	case "CHARACTER", "BIT":
		if keywordAt(tokens, i) == "VARYING" {
			i++
		}
	default:
		if !nullableBuiltinTypes[first] {
			return start, fmt.Sprintf("type %s is not a proven-nullable built-in type; custom and qualified domain types require a maintenance release", first)
		}
	}

	var ok bool
	i, ok = consumeOptionalTypeModifier(tokens, i)
	if !ok {
		return start, "column type has an invalid or unterminated modifier"
	}
	if first == "TIMESTAMP" || first == "TIME" {
		if keywordAt(tokens, i) == "WITH" || keywordAt(tokens, i) == "WITHOUT" {
			i++
			if !keywordsAt(tokens, i, "TIME", "ZONE") {
				return start, "timestamp/time qualifier must end in TIME ZONE"
			}
			i += 2
		}
	}

	for symbolAt(tokens, i, "[") {
		i++
		for i < len(tokens) && !symbolAt(tokens, i, "]") {
			if tokens[i].kind != tokenSymbol || tokens[i].text < "0" || tokens[i].text > "9" {
				return start, "array bounds must be numeric"
			}
			i++
		}
		if !symbolAt(tokens, i, "]") {
			return start, "column type has an unterminated array bound"
		}
		i++
	}
	return i, ""
}

func consumeOptionalTypeModifier(tokens []sqlToken, start int) (int, bool) {
	if !symbolAt(tokens, start, "(") {
		return start, true
	}
	depth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].kind != tokenSymbol {
			continue
		}
		switch tokens[i].text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i + 1, true
			}
			if depth < 0 {
				return start, false
			}
		}
	}
	return start, false
}

var nullableBuiltinTypes = map[string]bool{
	"BIGINT": true, "INT8": true, "INTEGER": true, "INT": true, "INT4": true,
	"SMALLINT": true, "INT2": true, "NUMERIC": true, "DECIMAL": true, "REAL": true,
	"FLOAT4": true, "FLOAT8": true, "MONEY": true, "VARCHAR": true, "CHAR": true,
	"TEXT": true, "BYTEA": true, "TIMESTAMP": true, "TIMESTAMPTZ": true, "DATE": true,
	"TIME": true, "TIMETZ": true, "INTERVAL": true, "BOOLEAN": true, "BOOL": true,
	"POINT": true, "LINE": true, "LSEG": true, "BOX": true, "PATH": true, "POLYGON": true,
	"CIRCLE": true, "INET": true, "CIDR": true, "MACADDR": true, "MACADDR8": true,
	"BIT": true, "VARBIT": true, "TSVECTOR": true, "TSQUERY": true, "UUID": true,
	"XML": true, "JSON": true, "JSONB": true, "INT4RANGE": true, "INT8RANGE": true,
	"NUMRANGE": true, "TSRANGE": true, "TSTZRANGE": true, "DATERANGE": true,
	"INT4MULTIRANGE": true, "INT8MULTIRANGE": true, "NUMMULTIRANGE": true,
	"TSMULTIRANGE": true, "TSTZMULTIRANGE": true, "DATEMULTIRANGE": true,
	"OID": true, "REGCLASS": true, "REGCOLLATION": true, "REGCONFIG": true,
	"REGDICTIONARY": true, "REGNAMESPACE": true, "REGOPER": true, "REGOPERATOR": true,
	"REGPROC": true, "REGPROCEDURE": true, "REGROLE": true, "REGTYPE": true,
	"PG_LSN": true, "TXID_SNAPSHOT": true,
}

func evaluateInsert(tokens []sqlToken) policyDecision {
	i := 1
	if keywordAt(tokens, i) != "INTO" {
		return rejected("INSERT", "only INSERT INTO ... VALUES is allowed")
	}
	i++
	if keywordAt(tokens, i) == "ONLY" {
		i++
	}
	if _, next, ok := parseRelation(tokens, i); !ok {
		return rejected("INSERT", "could not determine the target table")
	} else {
		i = next
	}
	if symbolAt(tokens, i, "(") {
		var ok bool
		i, ok = consumeIdentifierList(tokens, i)
		if !ok {
			return rejected("INSERT", "column list must contain only identifiers")
		}
	}
	if keywordAt(tokens, i) != "VALUES" {
		return rejected("INSERT", "query-form INSERT can contain data-modifying CTEs; only literal VALUES rows are allowed")
	}
	i++

	var ok bool
	i, ok = consumeLiteralValuesRows(tokens, i)
	if !ok {
		return rejected("INSERT VALUES", "VALUES rows may contain only string, numeric, boolean, or NULL literals")
	}

	if i == len(tokens) {
		return policyDecision{allowed: true, description: "literal INSERT VALUES"}
	}
	if !keywordsAt(tokens, i, "ON", "CONFLICT") {
		return rejected("INSERT VALUES", fmt.Sprintf("modifier %s is outside the additive INSERT allowlist", printableKeyword(tokens[i])))
	}
	i += 2
	if keywordsAt(tokens, i, "ON", "CONSTRAINT") {
		if _, next, parsed := parseIdentifier(tokens, i+2); !parsed {
			return rejected("INSERT ON CONFLICT", "ON CONSTRAINT must name one constraint")
		} else {
			i = next
		}
	} else if symbolAt(tokens, i, "(") {
		var parsed bool
		i, parsed = consumeIdentifierList(tokens, i)
		if !parsed {
			return rejected("INSERT ON CONFLICT", "conflict target must contain only identifiers")
		}
	}
	if !keywordsAt(tokens, i, "DO", "NOTHING") || i+2 != len(tokens) {
		return rejected("INSERT ON CONFLICT", "only a terminal ON CONFLICT ... DO NOTHING clause is allowed")
	}
	return policyDecision{allowed: true, description: "literal INSERT VALUES ON CONFLICT DO NOTHING"}
}

func consumeIdentifierList(tokens []sqlToken, start int) (int, bool) {
	if !symbolAt(tokens, start, "(") {
		return start, false
	}
	i := start + 1
	if _, next, ok := parseIdentifier(tokens, i); !ok {
		return start, false
	} else {
		i = next
	}
	for symbolAt(tokens, i, ",") {
		if _, next, ok := parseIdentifier(tokens, i+1); !ok {
			return start, false
		} else {
			i = next
		}
	}
	if !symbolAt(tokens, i, ")") {
		return start, false
	}
	return i + 1, true
}

func consumeLiteralValuesRows(tokens []sqlToken, start int) (int, bool) {
	i := start
	for {
		if !symbolAt(tokens, i, "(") {
			return start, false
		}
		i++
		var ok bool
		if i, ok = consumeLiteralValue(tokens, i); !ok {
			return start, false
		}
		for symbolAt(tokens, i, ",") {
			if i, ok = consumeLiteralValue(tokens, i+1); !ok {
				return start, false
			}
		}
		if !symbolAt(tokens, i, ")") {
			return start, false
		}
		i++
		if symbolAt(tokens, i, ",") {
			i++
			continue
		}
		return i, true
	}
}

func consumeLiteralValue(tokens []sqlToken, start int) (int, bool) {
	if start < 0 || start >= len(tokens) {
		return start, false
	}
	if tokens[start].kind == tokenString || tokens[start].kind == tokenNumber {
		return start + 1, true
	}
	if (symbolAt(tokens, start, "+") || symbolAt(tokens, start, "-")) && start+1 < len(tokens) && tokens[start+1].kind == tokenNumber {
		return start + 2, true
	}
	switch keywordAt(tokens, start) {
	case "NULL", "TRUE", "FALSE":
		return start + 1, true
	default:
		return start, false
	}
}

func skipBalancedParentheses(tokens []sqlToken, start int) (int, bool) {
	if !symbolAt(tokens, start, "(") {
		return start, false
	}
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch {
		case symbolAt(tokens, i, "("):
			depth++
		case symbolAt(tokens, i, ")"):
			depth--
			if depth == 0 {
				return i + 1, true
			}
			if depth < 0 {
				return start, false
			}
		}
	}
	return start, false
}

func evaluateCreateOnly(tokens []sqlToken) policyDecision {
	decision := evaluateCreate(tokens)
	if decision.allowed && decision.createdTable != "" {
		return decision
	}
	return policyDecision{}
}

func recordNewTableProof(decision policyDecision, baselineTables, createdTables map[relationName]struct{}) {
	if !decision.allowed || decision.createdTable == "" || decision.conditionalCreateTable ||
		relationSetContains(baselineTables, decision.createdTable) {
		return
	}
	createdTables[decision.createdTable] = struct{}{}
}

func relationSetContains(relations map[relationName]struct{}, candidate relationName) bool {
	if _, ok := relations[candidate]; ok {
		return true
	}
	for existing := range relations {
		if relationsCouldMatch(existing, candidate) {
			return true
		}
	}
	return false
}

func relationsCouldMatch(left, right relationName) bool {
	l := strings.Split(string(left), ".")
	r := strings.Split(string(right), ".")
	if string(left) == string(right) {
		return true
	}
	return len(l) != len(r) && l[len(l)-1] == r[len(r)-1]
}

func parseRelation(tokens []sqlToken, start int) (relationName, int, bool) {
	part, next, ok := parseIdentifier(tokens, start)
	if !ok {
		return "", start, false
	}
	parts := []string{part}
	for next+1 < len(tokens) && symbolAt(tokens, next, ".") {
		part, after, ok := parseIdentifier(tokens, next+1)
		if !ok {
			return "", start, false
		}
		parts = append(parts, part)
		next = after
	}
	return relationName(strings.Join(parts, ".")), next, true
}

func parseIdentifier(tokens []sqlToken, i int) (string, int, bool) {
	if i < 0 || i >= len(tokens) {
		return "", i, false
	}
	switch tokens[i].kind {
	case tokenWord:
		if len(tokens[i].text) > postgresIdentifierMaxBytes {
			return "", i, false
		}
		// PostgreSQL folds only ASCII A-Z in unquoted identifiers.
		return "i:" + strings.ToLower(tokens[i].text), i + 1, true
	case tokenQuotedIdentifier:
		if len(tokens[i].text) > postgresIdentifierMaxBytes {
			return "", i, false
		}
		if tokens[i].text == strings.ToLower(tokens[i].text) {
			return "i:" + tokens[i].text, i + 1, true
		}
		return "q:" + tokens[i].text, i + 1, true
	default:
		return "", i, false
	}
}

func skipIfNotExists(tokens []sqlToken, i int) int {
	if keywordsAt(tokens, i, "IF", "NOT", "EXISTS") {
		return i + 3
	}
	return i
}

func keywordAt(tokens []sqlToken, i int) string {
	if i < 0 || i >= len(tokens) || tokens[i].kind != tokenWord {
		return ""
	}
	return strings.ToUpper(tokens[i].text)
}

func keywordsAt(tokens []sqlToken, start int, expected ...string) bool {
	if start < 0 || start+len(expected) > len(tokens) {
		return false
	}
	for i, keyword := range expected {
		if keywordAt(tokens, start+i) != keyword {
			return false
		}
	}
	return true
}

func symbolAt(tokens []sqlToken, i int, expected string) bool {
	return i >= 0 && i < len(tokens) && tokens[i].kind == tokenSymbol && tokens[i].text == expected
}

func rejected(description, reason string) policyDecision {
	return policyDecision{description: description, reason: reason}
}

func statementPrefix(tokens []sqlToken) string {
	parts := make([]string, 0, 5)
	for _, token := range tokens {
		if token.kind != tokenWord {
			break
		}
		parts = append(parts, strings.ToUpper(token.text))
		if len(parts) == 5 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func printableKeyword(token sqlToken) string {
	if token.kind == tokenWord {
		return strings.ToUpper(token.text)
	}
	return token.text
}

func displayRelation(name relationName) string {
	parts := strings.Split(string(name), ".")
	for i := range parts {
		parts[i] = strings.TrimPrefix(strings.TrimPrefix(parts[i], "i:"), "q:")
	}
	return strings.Join(parts, ".")
}
