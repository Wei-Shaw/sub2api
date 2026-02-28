package service

import "testing"

func TestClassifyOpenAIPreviousResponseIDKind(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
REDACTED{
		{name: "empty", id: " ", want: OpenAIPreviousResponseIDKindEmptyREDACTED,
		{name: "response_id", id: "resp_0906a621bc423a8d0169a108637ef88197b74b0e2f37ba358f", want: OpenAIPreviousResponseIDKindResponseIDREDACTED,
		{name: "message_id", id: "msg_123456", want: OpenAIPreviousResponseIDKindMessageIDREDACTED,
		{name: "item_id", id: "item_abcdef", want: OpenAIPreviousResponseIDKindMessageIDREDACTED,
		{name: "unknown", id: "foo_123456", want: OpenAIPreviousResponseIDKindUnknownREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyOpenAIPreviousResponseIDKind(tc.id); got != tc.want {
				t.Fatalf("ClassifyOpenAIPreviousResponseIDKind(%q)=%q want=%q", tc.id, got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestIsOpenAIPreviousResponseIDLikelyMessageID(t *testing.T) {
	if !IsOpenAIPreviousResponseIDLikelyMessageID("msg_123") {
		t.Fatal("expected msg_123 to be identified as message id")
REDACTED
	if IsOpenAIPreviousResponseIDLikelyMessageID("resp_123") {
		t.Fatal("expected resp_123 not to be identified as message id")
REDACTED
REDACTED
