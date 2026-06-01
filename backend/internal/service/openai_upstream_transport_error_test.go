//go:build unit

package service

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
)

// TestClassifyOpenAITransportError pins which transport-level upstream failures
// are "persistent" (retrying the same proxy/account is pointless — evict + alert)
// versus "transient" (a blip — fail over to a healthy account but do not evict).
//
// The motivating incident: a SOCKS5 proxy whose credentials expired returned
// `username/password authentication failed`, yet the account kept being scheduled.
func TestClassifyOpenAITransportError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		persistent bool
REDACTED{
		// Durable — config/credential/routing problems. Retrying same proxy won't help.
		{"socks5 proxy credential rejected", errors.New(`Post "https://chatgpt.com/backend-api/codex/responses": socks connect tcp 85.255.176.68:12324->chatgpt.com:443: username/password authentication failed`), trueREDACTED,
		{"proxy connection refused", errors.New(`proxyconnect tcp: dial tcp 1.2.3.4:1080: connect: connection refused`), trueREDACTED,
		{"no route to host", errors.New(`dial tcp 1.2.3.4:443: connect: no route to host`), trueREDACTED,
		{"dns resolution failure", errors.New(`dial tcp: lookup proxy.example.com: no such host`), trueREDACTED,
		{"network unreachable", errors.New(`dial tcp 1.2.3.4:443: connect: network is unreachable`), trueREDACTED,

		// Transient — a temporary blip. Fail over, but do NOT evict the account.
		{"client timeout", errors.New(`Post "https://chatgpt.com/...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`), falseREDACTED,
		{"i/o timeout", errors.New(`dial tcp 1.2.3.4:443: i/o timeout`), falseREDACTED,
		{"connection reset by peer", errors.New(`read tcp 10.0.0.1:5->2.2.2.2:443: read: connection reset by peer`), falseREDACTED,
		{"unexpected eof", errors.New(`unexpected EOF`), falseREDACTED,
		{"broken pipe", errors.New(`write tcp 10.0.0.1:5->2.2.2.2:443: write: broken pipe`), falseREDACTED,

		{"nil error", nil, falseREDACTED,

		// ── Typed-error cases ──────────────────────────────────────────────
		// ECONNREFUSED wrapped in the canonical net.OpError shape Go produces.
		{
			"ECONNREFUSED via net.OpError",
			&net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSEDREDACTED,
		REDACTED,
			true,
	REDACTED,
		// Bare syscall error (errors.Is traverses the chain).
		{"ECONNREFUSED bare", syscall.ECONNREFUSED, trueREDACTED,
		{"EHOSTUNREACH bare", syscall.EHOSTUNREACH, trueREDACTED,
		{"ENETUNREACH bare", syscall.ENETUNREACH, trueREDACTED,

		// *net.DNSError with IsNotFound — permanent DNS lookup failure.
		{
			"DNS not found (IsNotFound=true)",
			&net.DNSError{Err: "no such host", Name: "proxy.example.com", IsNotFound: trueREDACTED,
			true,
	REDACTED,
		// *net.DNSError with IsNotFound=false — transient DNS timeout (not persistent).
		{
			"DNS timeout (IsNotFound=false)",
			&net.DNSError{Err: "i/o timeout", Name: "proxy.example.com", IsTimeout: trueREDACTED,
			false,
	REDACTED,

		// context.Canceled — client gone; NOT classified as persistent.
		{"context.Canceled", context.Canceled, falseREDACTED,
		// context.DeadlineExceeded — slow upstream; NOT persistent.
		{"context.DeadlineExceeded", context.DeadlineExceeded, falseREDACTED,
REDACTED

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyOpenAITransportError(tc.err).Persistent
			if got != tc.persistent {
				t.Fatalf("classifyOpenAITransportError(%q).Persistent = %v, want %v", errString(tc.err), got, tc.persistent)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func errString(err error) string {
	if err == nil {
		return "<nil>"
REDACTED
	return err.Error()
REDACTED
