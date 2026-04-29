package service

import (
	"testing"
	"time"
)

func TestOpsCleanupPlan(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		days         int
		wantOK       bool
		wantTruncate bool
		wantCutoff   time.Time
REDACTED{
		{name: "negative skips", days: -1, wantOK: falseREDACTED,
		{name: "zero truncates", days: 0, wantOK: true, wantTruncate: trueREDACTED,
		{name: "positive yields past cutoff", days: 7, wantOK: true, wantCutoff: now.AddDate(0, 0, -7)REDACTED,
REDACTED

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cutoff, truncate, ok := opsCleanupPlan(now, tc.days)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
		REDACTED
			if !ok {
				return
		REDACTED
			if truncate != tc.wantTruncate {
				t.Fatalf("truncate = %v, want %v", truncate, tc.wantTruncate)
		REDACTED
			if !tc.wantTruncate && !cutoff.Equal(tc.wantCutoff) {
				t.Fatalf("cutoff = %v, want %v", cutoff, tc.wantCutoff)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestIsMissingRelationError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
REDACTED{
		{name: "nil is not missing", err: nil, want: falseREDACTED,
		{name: "match relation does not exist", err: fakeErr(`pq: relation "ops_error_logs" does not exist`), want: trueREDACTED,
		{name: "match case-insensitive", err: fakeErr(`ERROR: Relation "x" Does Not Exist`), want: trueREDACTED,
		{name: "non-matching error", err: fakeErr("connection refused"), want: falseREDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMissingRelationError(tc.err); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

type fakeErr string

func (e fakeErr) Error() string { return string(e) REDACTED
