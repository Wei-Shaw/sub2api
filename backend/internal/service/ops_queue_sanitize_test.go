package service

import (
	"strings"
	"testing"
)

func TestSanitizeOpsUpstreamErrorsForQueueBoundsAndRedacts(t *testing.T) {
	entry := &OpsInsertErrorLogInput{REDACTED
	for i := 0; i < 20; i++ {
		entry.UpstreamErrors = append(entry.UpstreamErrors, &OpsUpstreamErrorEvent{
			Platform:             strings.Repeat("p", 100),
			AccountName:          strings.Repeat("a", 300),
			UpstreamStatusCode:   500,
			UpstreamURL:          strings.Repeat("u", 3000),
			UpstreamResponseBody: `{"authorization":"Bearer secret","message":"` + strings.Repeat("x", 10_000) + `"REDACTED`,
			Message:              strings.Repeat("m", 3000),
			Detail:               `{"api_key":"secret","detail":"` + strings.Repeat("y", 10_000) + `"REDACTED`,
	REDACTED)
REDACTED

	if err := SanitizeOpsUpstreamErrorsForQueue(entry); err != nil {
		t.Fatal(err)
REDACTED
	if entry.UpstreamErrors != nil {
		t.Fatal("raw upstream event slice must be released before queueing")
REDACTED
	if entry.UpstreamErrorsJSON == nil {
		t.Fatal("sanitized upstream event JSON is missing")
REDACTED
	events, err := ParseOpsUpstreamErrors(*entry.UpstreamErrorsJSON)
	if err != nil {
		t.Fatal(err)
REDACTED
	if len(events) != 16 {
		t.Fatalf("event count = %d, want 16", len(events))
REDACTED
	for _, event := range events {
		if len(event.Platform) > 32 || len(event.AccountName) > 128 || len(event.UpstreamURL) > 2048 || len(event.Message) > 2048 {
			t.Fatalf("event fields were not bounded: %+v", event)
	REDACTED
		if len(event.UpstreamResponseBody) > OpsErrorLogQueueBodyMaxBytes || len(event.Detail) > OpsErrorLogQueueBodyMaxBytes {
			t.Fatal("event body/detail exceeded queue limit")
	REDACTED
		if strings.Contains(event.UpstreamResponseBody, "Bearer secret") || strings.Contains(event.Detail, `"secret"`) {
			t.Fatal("credential material was not redacted")
	REDACTED
REDACTED
REDACTED
