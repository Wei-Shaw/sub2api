package apperr

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// pqBracketRe extracts the structured pq metadata from the host gRPC error
// format "pq[CODE,constraint=...,column=...,table=...]: message". The host's
// errInternal emits this format for *pq.Error so the plugin can classify
// database errors even though the native *pq.Error type is lost in transit.
var pqBracketRe = regexp.MustCompile(`pq\[([^\]]+)\]:\s*(.*)`)

// parsedPQ holds the structured fields extracted from a gRPC pq error message.
type parsedPQ struct {
	Code       string // SQLSTATE, e.g. "23505"
	Constraint string
	Column     string
	Table      string
	Message    string // pq human message (after the bracket)
}

// parsePQFromGRPC attempts to parse the "pq[CODE,...]: msg" format from an
// error's text. Returns nil when the format is not found.
func parsePQFromGRPC(err error) *parsedPQ {
	if err == nil {
		return nil
	}
	m := pqBracketRe.FindStringSubmatch(err.Error())
	if m == nil {
		return nil
	}
	p := &parsedPQ{Message: m[2]}
	for i, part := range strings.Split(m[1], ",") {
		if i == 0 {
			p.Code = part
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		switch k {
		case "constraint":
			p.Constraint = v
		case "column":
			p.Column = v
		case "table":
			p.Table = v
		}
	}
	return p
}

// ClassifyDBError examines err for the structured pq metadata emitted by the
// host gRPC layer and returns an appropriate 4xx/5xx ApplicationError. Returns
// nil when err is not a recognisable pq error, letting the caller fall through
// to default handling.
//
// Recognised SQLSTATE codes:
//   - 23502 not_null_violation  → 400
//   - 23505 unique_violation    → 409
//   - 23514 check_violation     → 400
//   - 22001 string_data_right_truncation → 400
//   - 42703 undefined_column    → 500
func ClassifyDBError(err error) *ApplicationError {
	p := parsePQFromGRPC(err)
	if p == nil {
		return nil
	}
	switch p.Code {
	case "23502": // not_null_violation
		field := p.Column
		if field == "" {
			field = "unknown"
		}
		return BadRequest("NOT_NULL_VIOLATION",
			fmt.Sprintf("field %s cannot be null", field)).WithCause(err)
	case "23505": // unique_violation
		detail := p.Constraint
		if detail == "" {
			detail = "unknown"
		}
		return Conflict("UNIQUE_VIOLATION",
			fmt.Sprintf("duplicate value violates constraint: %s", detail)).WithCause(err)
	case "23514": // check_violation
		detail := p.Constraint
		if detail == "" {
			detail = "unknown"
		}
		return BadRequest("CHECK_VIOLATION",
			fmt.Sprintf("value violates check constraint: %s", detail)).WithCause(err)
	case "22001": // string_data_right_truncation
		return BadRequest("VALUE_TOO_LONG",
			"value too long for field").WithCause(err)
	case "42703": // undefined_column
		col := p.Column
		if col == "" {
			col = p.Message
		}
		return InternalServer("SCHEMA_MISMATCH",
			fmt.Sprintf("schema mismatch: %s", col)).WithCause(err)
	default:
		return nil
	}
}

// ClassifyGRPCError examines err for common gRPC transport/marshaling errors
// and returns a structured ApplicationError. Returns nil for unrecognised errors.
func ClassifyGRPCError(err error) *ApplicationError {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "invalid UTF-8") {
		return InternalServer("ENCODING_ERROR",
			"a field contains binary data that cannot be transmitted as text").WithCause(err)
	}
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "Unavailable") {
		return New(http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE",
			"backend service temporarily unavailable").WithCause(err)
	}
	return nil
}

// IsPQCode returns true when err carries the structured pq metadata from the
// host gRPC layer and the SQLSTATE code matches. Useful for repo-layer checks
// like isUniqueViolation that need to detect specific database errors after
// they have crossed the gRPC boundary.
func IsPQCode(err error, sqlstate string) bool {
	p := parsePQFromGRPC(err)
	return p != nil && p.Code == sqlstate
}
