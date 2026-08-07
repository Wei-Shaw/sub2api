package ent

import (
	"testing"

	"entgo.io/ent/dialect/sql"
)

func TestGroupQueryCloneUninitialized(t *testing.T) {
	query := (&GroupQuery{}).Clone()
	if query == nil {
		t.Fatal("Clone returned nil for a non-nil query")
	}
	if query.ctx != nil {
		t.Fatal("Clone initialized a nil query context")
	}
	if query.sql != nil {
		t.Fatal("Clone initialized a nil SQL selector")
	}
}

func TestGroupQueryClonePreservesContext(t *testing.T) {
	ctx := &QueryContext{}
	query := (&GroupQuery{ctx: ctx}).Clone()
	if query == nil || query.ctx == nil {
		t.Fatal("Clone did not preserve an initialized query context")
	}
	if query.ctx == ctx {
		t.Fatal("Clone reused the original query context")
	}
}

func TestGroupQueryClonePreservesSQLSelector(t *testing.T) {
	selector := sql.Select("id").From(sql.Table("groups"))
	query := (&GroupQuery{sql: selector}).Clone()
	if query == nil || query.sql == nil {
		t.Fatal("Clone did not preserve an initialized SQL selector")
	}
	if query.sql == selector {
		t.Fatal("Clone reused the original SQL selector")
	}
}
