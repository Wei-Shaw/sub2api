//go:build unit

package plugin

import (
	"fmt"
	"testing"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// ---------------------------------------------------------------------------
// interfaceToSQLValue: time.Time encoding regression tests
// ---------------------------------------------------------------------------

func TestInterfaceToSQLValue_TimeUsesTimeValue(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Microsecond)
	sv := interfaceToSQLValue(now)
	if sv == nil {
		t.Fatal("interfaceToSQLValue returned nil for time.Time")
	}
	tv, ok := sv.GetValue().(*pluginsdk.SQLValue_TimeValue)
	if !ok {
		t.Fatalf("time.Time should encode as TimeValue, got %T", sv.GetValue())
	}
	parsed, err := time.Parse(time.RFC3339Nano, tv.TimeValue)
	if err != nil {
		t.Fatalf("failed to parse TimeValue back: %v", err)
	}
	if !parsed.Equal(now) {
		t.Errorf("round-trip mismatch: got %v, want %v", parsed, now)
	}
}

func TestInterfaceToSQLValue_TimeUTC(t *testing.T) {
	t.Parallel()
	utc := time.Date(2025, 6, 15, 12, 30, 45, 123456000, time.UTC)
	sv := interfaceToSQLValue(utc)
	tv, ok := sv.GetValue().(*pluginsdk.SQLValue_TimeValue)
	if !ok {
		t.Fatalf("expected TimeValue, got %T", sv.GetValue())
	}
	parsed, err := time.Parse(time.RFC3339Nano, tv.TimeValue)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !parsed.Equal(utc) {
		t.Errorf("UTC time round-trip mismatch: got %v, want %v", parsed, utc)
	}
}

func TestInterfaceToSQLValue_TimeZero(t *testing.T) {
	t.Parallel()
	zero := time.Time{}
	sv := interfaceToSQLValue(zero)
	if _, ok := sv.GetValue().(*pluginsdk.SQLValue_TimeValue); !ok {
		t.Fatalf("zero time should still use TimeValue, got %T", sv.GetValue())
	}
}

// ---------------------------------------------------------------------------
// interfaceToSQLValue: nil -> Null
// ---------------------------------------------------------------------------

func TestInterfaceToSQLValue_Nil(t *testing.T) {
	t.Parallel()
	sv := interfaceToSQLValue(nil)
	if sv == nil {
		t.Fatal("interfaceToSQLValue returned nil for nil input")
	}
	if _, ok := sv.GetValue().(*pluginsdk.SQLValue_Null); !ok {
		t.Fatalf("nil should encode as Null, got %T", sv.GetValue())
	}
}

// ---------------------------------------------------------------------------
// interfaceToSQLValue: other scalar types (sanity)
// ---------------------------------------------------------------------------

func TestInterfaceToSQLValue_ScalarTypes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    any
		wantType string
	}{
		{"bool_true", true, "*pluginsdk.SQLValue_BoolValue"},
		{"bool_false", false, "*pluginsdk.SQLValue_BoolValue"},
		{"int", 42, "*pluginsdk.SQLValue_IntValue"},
		{"int32", int32(99), "*pluginsdk.SQLValue_IntValue"},
		{"int64", int64(100), "*pluginsdk.SQLValue_IntValue"},
		{"float32", float32(1.5), "*pluginsdk.SQLValue_FloatValue"},
		{"float64", 3.14, "*pluginsdk.SQLValue_FloatValue"},
		{"string", "hello", "*pluginsdk.SQLValue_StringValue"},
		{"bytes", []byte{0x01, 0x02}, "*pluginsdk.SQLValue_BytesValue"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sv := interfaceToSQLValue(tc.input)
			if sv == nil {
				t.Fatal("returned nil")
			}
			got := fmt.Sprintf("%T", sv.GetValue())
			if got != tc.wantType {
				t.Errorf("got %s, want %s", got, tc.wantType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// interfaceToSQLValue: unknown type -> fallback string
// ---------------------------------------------------------------------------

func TestInterfaceToSQLValue_UnknownFallback(t *testing.T) {
	t.Parallel()
	type custom struct{ X int }
	sv := interfaceToSQLValue(custom{X: 7})
	s, ok := sv.GetValue().(*pluginsdk.SQLValue_StringValue)
	if !ok {
		t.Fatalf("unknown type should fallback to StringValue, got %T", sv.GetValue())
	}
	if s.StringValue == "" {
		t.Error("fallback string should not be empty")
	}
}

// ---------------------------------------------------------------------------
// sqlValueToInterface: round-trip parity (proto -> Go)
// ---------------------------------------------------------------------------

func TestSqlValueToInterface_RoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input any
	}{
		{"nil", nil},
		{"bool", true},
		{"int64", int64(42)},
		{"float64", 3.14},
		{"string", "hello"},
		{"bytes", []byte{0xDE, 0xAD}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sv := interfaceToSQLValue(tc.input)
			got := sqlValueToInterface(sv)
			switch want := tc.input.(type) {
			case nil:
				if got != nil {
					t.Errorf("expected nil, got %v (%T)", got, got)
				}
			case bool:
				if got != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case int64:
				if got != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case float64:
				if got != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case string:
				if got != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case []byte:
				gotBytes, ok := got.([]byte)
				if !ok {
					t.Fatalf("expected []byte, got %T", got)
				}
				if len(gotBytes) != len(want) {
					t.Errorf("len mismatch: got %d, want %d", len(gotBytes), len(want))
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// convertSQLValues: batch conversion
// ---------------------------------------------------------------------------

func TestConvertSQLValues_Mixed(t *testing.T) {
	t.Parallel()
	vals := []*pluginsdk.SQLValue{
		{Value: &pluginsdk.SQLValue_IntValue{IntValue: 1}},
		{Value: &pluginsdk.SQLValue_StringValue{StringValue: "two"}},
		{Value: &pluginsdk.SQLValue_Null{Null: true}},
	}
	got := convertSQLValues(vals)
	if len(got) != 3 {
		t.Fatalf("expected 3 values, got %d", len(got))
	}
	if got[0] != int64(1) {
		t.Errorf("[0] got %v, want 1", got[0])
	}
	if got[1] != "two" {
		t.Errorf("[1] got %v, want 'two'", got[1])
	}
	if got[2] != nil {
		t.Errorf("[2] got %v, want nil", got[2])
	}
}

func TestConvertSQLValues_Empty(t *testing.T) {
	t.Parallel()
	got := convertSQLValues(nil)
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}
