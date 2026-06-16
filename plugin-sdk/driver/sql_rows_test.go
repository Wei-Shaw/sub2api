//go:build unit

package driver

import (
	"reflect"
	"testing"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

func TestGrpcRows_ColumnTypeDatabaseTypeName(t *testing.T) {
	resp := &pb.SQLResponse{
		Columns:     []string{"id", "name", "created_at", "data"},
		ColumnTypes: []string{"int8", "varchar", "timestamptz", "jsonb"},
		Rows:        []*pb.SQLRow{},
	}
	rows := newRows(resp)

	tests := []struct {
		index int
		want  string
	}{
		{0, "int8"},
		{1, "varchar"},
		{2, "timestamptz"},
		{3, "jsonb"},
		{-1, ""}, // negative index
		{99, ""}, // out of range
	}
	for _, tt := range tests {
		got := rows.ColumnTypeDatabaseTypeName(tt.index)
		if got != tt.want {
			t.Errorf("ColumnTypeDatabaseTypeName(%d) = %q, want %q", tt.index, got, tt.want)
		}
	}
}

func TestGrpcRows_ColumnTypeDatabaseTypeName_Empty(t *testing.T) {
	// When host sends no column_types (backward compat), all indices return "".
	resp := &pb.SQLResponse{
		Columns: []string{"id", "name"},
		Rows:    []*pb.SQLRow{},
	}
	rows := newRows(resp)

	if got := rows.ColumnTypeDatabaseTypeName(0); got != "" {
		t.Errorf("expected empty string for missing colTypes, got %q", got)
	}
}

func TestGrpcRows_ColumnTypeScanType(t *testing.T) {
	resp := &pb.SQLResponse{
		Columns:     []string{"a", "b", "c", "d", "e"},
		ColumnTypes: []string{"int8", "varchar", "timestamptz", "jsonb", "bool"},
		Rows:        []*pb.SQLRow{},
	}
	rows := newRows(resp)

	tests := []struct {
		index int
		want  reflect.Type
	}{
		{0, reflect.TypeOf(int64(0))},
		{1, reflect.TypeOf("")},
		{2, reflect.TypeOf(time.Time{})},
		{3, reflect.TypeOf([]byte{})},
		{4, reflect.TypeOf(false)},
	}
	for _, tt := range tests {
		got := rows.ColumnTypeScanType(tt.index)
		if got != tt.want {
			t.Errorf("ColumnTypeScanType(%d) = %v, want %v", tt.index, got, tt.want)
		}
	}
}

func TestPgTypeToGoType(t *testing.T) {
	tests := []struct {
		name string
		want reflect.Type
	}{
		// Integer types
		{"int2", reflect.TypeOf(int64(0))},
		{"int4", reflect.TypeOf(int64(0))},
		{"int8", reflect.TypeOf(int64(0))},
		{"serial", reflect.TypeOf(int64(0))},
		{"bigserial", reflect.TypeOf(int64(0))},
		// Float types
		{"float4", reflect.TypeOf(float64(0))},
		{"float8", reflect.TypeOf(float64(0))},
		{"numeric", reflect.TypeOf(float64(0))},
		{"decimal", reflect.TypeOf(float64(0))},
		// Bool
		{"bool", reflect.TypeOf(false)},
		// String types
		{"varchar", reflect.TypeOf("")},
		{"text", reflect.TypeOf("")},
		{"char", reflect.TypeOf("")},
		{"bpchar", reflect.TypeOf("")},
		{"name", reflect.TypeOf("")},
		{"uuid", reflect.TypeOf("")},
		// Bytes
		{"bytea", reflect.TypeOf([]byte{})},
		// Time types
		{"timestamp", reflect.TypeOf(time.Time{})},
		{"timestamptz", reflect.TypeOf(time.Time{})},
		{"date", reflect.TypeOf(time.Time{})},
		{"time", reflect.TypeOf(time.Time{})},
		{"timetz", reflect.TypeOf(time.Time{})},
		// JSON types
		{"json", reflect.TypeOf([]byte{})},
		{"jsonb", reflect.TypeOf([]byte{})},
		// Case insensitive
		{"INT8", reflect.TypeOf(int64(0))},
		{"VARCHAR", reflect.TypeOf("")},
		{"TIMESTAMPTZ", reflect.TypeOf(time.Time{})},
		// Unknown fallback
		{"hstore", reflect.TypeOf((*any)(nil)).Elem()},
		{"", reflect.TypeOf((*any)(nil)).Elem()},
	}
	for _, tt := range tests {
		got := pgTypeToGoType(tt.name)
		if got != tt.want {
			t.Errorf("pgTypeToGoType(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
