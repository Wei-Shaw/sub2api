package service

import (
	"strings"
	"testing"
)

func TestSanitizeOpsUpstreamErrorsForQueueBoundsAndRedacts(t *testing.T) {
	entry := &OpsInsertErrorLogInput{}
	for i := 0; i < 20; i++ {
		proxyID := int64(i + 1)
		entry.UpstreamErrors = append(entry.UpstreamErrors, &OpsUpstreamErrorEvent{
			ProxyID:              &proxyID,
			ProxyName:            "proxy-" + string(rune('a'+i)),
			Platform:             strings.Repeat("p", 100),
			AccountName:          strings.Repeat("a", 300),
			UpstreamStatusCode:   500,
			UpstreamURL:          strings.Repeat("u", 3000),
			UpstreamResponseBody: `{"authorization":"Bearer secret","message":"` + strings.Repeat("x", 10_000) + `"}`,
			Message:              strings.Repeat("m", 3000),
			Detail:               `{"api_key":"secret","detail":"` + strings.Repeat("y", 10_000) + `"}`,
		})
	}

	if err := SanitizeOpsUpstreamErrorsForQueue(entry); err != nil {
		t.Fatal(err)
	}
	if entry.UpstreamErrors != nil {
		t.Fatal("raw upstream event slice must be released before queueing")
	}
	if entry.UpstreamErrorsJSON == nil {
		t.Fatal("sanitized upstream event JSON is missing")
	}
	events, err := ParseOpsUpstreamErrors(*entry.UpstreamErrorsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 20 {
		t.Fatalf("event count = %d, want 20", len(events))
	}
	for i, event := range events {
		if event.ProxyID == nil || *event.ProxyID != int64(i+1) {
			t.Fatalf("event %d proxy id = %v, want %d", i, event.ProxyID, i+1)
		}
		if len(event.Platform) > 32 || len(event.AccountName) > 128 || len(event.UpstreamURL) > 2048 || len(event.Message) > 2048 {
			t.Fatalf("event fields were not bounded: %+v", event)
		}
		if i < 4 && (len(event.UpstreamURL) > opsUpstreamErrorsOlderURLMaxLen || len(event.Message) > opsUpstreamErrorsOlderMessageMaxLen) {
			t.Fatalf("older event %d kept a full-size url/message: url=%d message=%d", i, len(event.UpstreamURL), len(event.Message))
		}
		if event.DroppedEarlierAttempts != 0 {
			t.Fatalf("event %d reported dropped attempts without any drop: %d", i, event.DroppedEarlierAttempts)
		}
		if len(event.UpstreamResponseBody) > OpsErrorLogQueueBodyMaxBytes || len(event.Detail) > OpsErrorLogQueueBodyMaxBytes {
			t.Fatal("event body/detail exceeded queue limit")
		}
		if strings.Contains(event.UpstreamResponseBody, "Bearer secret") || strings.Contains(event.Detail, `"secret"`) {
			t.Fatal("credential material was not redacted")
		}
		if i < 4 && (event.UpstreamResponseBody != "" || event.Detail != "") {
			t.Fatal("older event retained a large body outside the queue body window")
		}
	}
}

func TestSanitizeOpsUpstreamErrorsForQueueHardBoundsEventCount(t *testing.T) {
	entry := &OpsInsertErrorLogInput{}
	total := opsUpstreamErrorsMaxEvents + 40
	for i := 0; i < total; i++ {
		entry.UpstreamErrors = append(entry.UpstreamErrors, &OpsUpstreamErrorEvent{
			AtUnixMs:           int64(i + 1),
			ProxyName:          opsProxyNameDirect,
			UpstreamStatusCode: 500,
			Message:            "attempt",
		})
	}
	if err := SanitizeOpsUpstreamErrorsForQueue(entry); err != nil {
		t.Fatal(err)
	}
	events, err := ParseOpsUpstreamErrors(*entry.UpstreamErrorsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != opsUpstreamErrorsMaxEvents {
		t.Fatalf("event count = %d, want %d", len(events), opsUpstreamErrorsMaxEvents)
	}
	// Newest attempts win; the oldest retained one carries the drop count.
	if events[len(events)-1].AtUnixMs != int64(total) {
		t.Fatalf("newest event lost: at=%d want %d", events[len(events)-1].AtUnixMs, total)
	}
	if events[0].AtUnixMs != int64(total-opsUpstreamErrorsMaxEvents+1) {
		t.Fatalf("oldest retained at=%d", events[0].AtUnixMs)
	}
	if events[0].DroppedEarlierAttempts != 40 {
		t.Fatalf("dropped_earlier_attempts = %d, want 40", events[0].DroppedEarlierAttempts)
	}
	if events[1].DroppedEarlierAttempts != 0 {
		t.Fatal("drop count must only be stamped on the oldest retained event")
	}
}

func TestSanitizeOpsUpstreamErrorsForQueueHardBoundsSerializedBytes(t *testing.T) {
	entry := &OpsInsertErrorLogInput{}
	// Every bounded field at its cap: the 16 newest attempts weigh ~20KB each
	// (8KB body + 8KB detail + 2KB url + 2KB message), older ones ~2KB. At the
	// count cap that is well past the byte budget, so bytes must win here.
	const total = opsUpstreamErrorsMaxEvents
	for i := 0; i < total; i++ {
		entry.UpstreamErrors = append(entry.UpstreamErrors, &OpsUpstreamErrorEvent{
			AtUnixMs:           int64(i + 1),
			Platform:           strings.Repeat("p", 40),
			AccountName:        strings.Repeat("a", 200),
			ProxyName:          strings.Repeat("n", 200),
			UpstreamRequestID:  strings.Repeat("r", 200),
			UpstreamStatusCode: 502,
			UpstreamURL:        "https://example.com/" + strings.Repeat("u", 3000),
			// Plain-text payloads are truncated, not JSON-trimmed, so they keep
			// their full queue-limit weight.
			UpstreamResponseBody: strings.Repeat("b", 10_000),
			Kind:                 strings.Repeat("k", 80),
			Stage:                strings.Repeat("s", 80),
			Scope:                strings.Repeat("c", 80),
			Reason:               strings.Repeat("e", 200),
			Message:              strings.Repeat("m", 3000),
			Detail:               strings.Repeat("d", 10_000),
		})
	}
	if err := SanitizeOpsUpstreamErrorsForQueue(entry); err != nil {
		t.Fatal(err)
	}
	// The newest event is always kept even when it alone exceeds the remaining
	// budget, so allow one max-size event of slack.
	if len(*entry.UpstreamErrorsJSON) > opsUpstreamErrorsQueueMaxBytes+24*1024 {
		t.Fatalf("serialized upstream_errors = %d bytes, budget %d", len(*entry.UpstreamErrorsJSON), opsUpstreamErrorsQueueMaxBytes)
	}
	events, err := ParseOpsUpstreamErrors(*entry.UpstreamErrorsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || len(events) >= total {
		t.Fatalf("expected the byte budget to drop some events, kept %d of %d", len(events), total)
	}
	if events[len(events)-1].AtUnixMs != int64(total) {
		t.Fatal("newest attempt must always be retained")
	}
	if events[0].DroppedEarlierAttempts != total-len(events) {
		t.Fatalf("dropped_earlier_attempts = %d, want %d", events[0].DroppedEarlierAttempts, total-len(events))
	}
}

func TestSanitizeOpsUpstreamErrorsForQueueDropsEmptyButKeepsOlderTrimmedEvents(t *testing.T) {
	entry := &OpsInsertErrorLogInput{}
	// 20 events: the first 4 fall outside the body window. Event 0 is a real
	// attempt whose only payload is a detail (status 0, no message); it must
	// survive the window even though its detail is cleared. Event 1 is truly
	// empty and must be dropped.
	for i := 0; i < 20; i++ {
		ev := &OpsUpstreamErrorEvent{AtUnixMs: int64(i + 1), ProxyName: opsProxyNameDirect}
		switch i {
		case 0:
			ev.Detail = "proxy handshake failed"
		case 1:
			// fully empty
		default:
			ev.UpstreamStatusCode = 500
		}
		entry.UpstreamErrors = append(entry.UpstreamErrors, ev)
	}
	if err := SanitizeOpsUpstreamErrorsForQueue(entry); err != nil {
		t.Fatal(err)
	}
	events, err := ParseOpsUpstreamErrors(*entry.UpstreamErrorsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 19 {
		t.Fatalf("event count = %d, want 19", len(events))
	}
	if events[0].AtUnixMs != 1 || events[0].Detail != "" {
		t.Fatalf("detail-only older attempt must be kept with detail cleared: %+v", events[0])
	}
	if events[1].AtUnixMs != 3 {
		t.Fatalf("fully-empty attempt must be dropped, got at=%d", events[1].AtUnixMs)
	}
}

func TestSanitizeOpsUpstreamErrorsForQueueIgnoresCallerSuppliedDropCount(t *testing.T) {
	entry := &OpsInsertErrorLogInput{UpstreamErrors: []*OpsUpstreamErrorEvent{
		{UpstreamStatusCode: 500, ProxyName: opsProxyNameDirect, DroppedEarlierAttempts: 99},
	}}
	if err := SanitizeOpsUpstreamErrorsForQueue(entry); err != nil {
		t.Fatal(err)
	}
	events, err := ParseOpsUpstreamErrors(*entry.UpstreamErrorsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].DroppedEarlierAttempts != 0 {
		t.Fatalf("caller-supplied drop count must be reset: %+v", events[0])
	}
}
