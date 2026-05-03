//go:build unit

package driver

import (
	"testing"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

func TestSqlValueToDriver_TimeValue(t *testing.T) {
	// Truncate to microsecond — PostgreSQL time precision.
	now := time.Now().UTC().Truncate(time.Microsecond)
	sv := &pb.SQLValue{Value: &pb.SQLValue_TimeValue{
		TimeValue: now.Format(time.RFC3339Nano),
	}}

	got := sqlValueToDriver(sv)
	gotTime, ok := got.(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T (%v)", got, got)
	}
	if !gotTime.Equal(now) {
		t.Errorf("time mismatch: got %v, want %v", gotTime, now)
	}
}

func TestSqlValueToDriver_TimeValue_InvalidFallback(t *testing.T) {
	sv := &pb.SQLValue{Value: &pb.SQLValue_TimeValue{
		TimeValue: "not-a-time",
	}}

	got := sqlValueToDriver(sv)
	gotStr, ok := got.(string)
	if !ok {
		t.Fatalf("expected string fallback, got %T (%v)", got, got)
	}
	if gotStr != "not-a-time" {
		t.Errorf("string mismatch: got %q, want %q", gotStr, "not-a-time")
	}
}

func TestGoValueToProto_TimeRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)

	sv, err := goValueToProto(now)
	if err != nil {
		t.Fatalf("goValueToProto: %v", err)
	}

	// Verify it uses TimeValue, not StringValue.
	if _, ok := sv.GetValue().(*pb.SQLValue_TimeValue); !ok {
		t.Fatalf("expected SQLValue_TimeValue, got %T", sv.GetValue())
	}

	got := sqlValueToDriver(sv)
	gotTime, ok := got.(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T (%v)", got, got)
	}
	if !gotTime.Equal(now) {
		t.Errorf("round-trip time mismatch: got %v, want %v", gotTime, now)
	}
}

func TestGoValueToProto_TimeZero_RoundTrip(t *testing.T) {
	zero := time.Time{}

	sv, err := goValueToProto(zero)
	if err != nil {
		t.Fatalf("goValueToProto(zero time): %v", err)
	}

	got := sqlValueToDriver(sv)
	gotTime, ok := got.(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T (%v)", got, got)
	}
	if !gotTime.Equal(zero) {
		t.Errorf("zero-time round-trip mismatch: got %v, want %v", gotTime, zero)
	}
}

func TestGoValueToProto_TimeNonUTC_RoundTrip(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	orig := time.Date(2025, 6, 15, 14, 30, 0, 123456000, loc)

	sv, err := goValueToProto(orig)
	if err != nil {
		t.Fatalf("goValueToProto(non-UTC time): %v", err)
	}

	got := sqlValueToDriver(sv)
	gotTime, ok := got.(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T (%v)", got, got)
	}
	// time.Equal compares instants, so UTC+8 14:30 == UTC 06:30.
	if !gotTime.Equal(orig) {
		t.Errorf("non-UTC round-trip: got %v, want %v (Equal should be true even if Location differs)", gotTime, orig)
	}
}

func TestSqlValueToDriver_Nil(t *testing.T) {
	got := sqlValueToDriver(nil)
	if got != nil {
		t.Errorf("sqlValueToDriver(nil) = %v (%T), want nil", got, got)
	}
}

func TestGoValueToProto_UnsupportedType(t *testing.T) {
	type custom struct{ X int }
	_, err := goValueToProto(custom{X: 1})
	if err == nil {
		t.Fatal("goValueToProto(custom struct) should return an error")
	}
}
