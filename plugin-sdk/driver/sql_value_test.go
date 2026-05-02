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
