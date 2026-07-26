package openai

import "testing"

func TestClassifyPreviousResponseIDKind(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "empty", id: " ", want: PreviousResponseIDKindEmpty},
		{name: "response_id", id: "resp_0906a621bc423a8d0169a108637ef88197b74b0e2f37ba358f", want: PreviousResponseIDKindResponseID},
		{name: "message_id", id: "msg_123456", want: PreviousResponseIDKindMessageID},
		{name: "item_id", id: "item_abcdef", want: PreviousResponseIDKindMessageID},
		{name: "unknown", id: "foo_123456", want: PreviousResponseIDKindUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyPreviousResponseIDKind(tc.id); got != tc.want {
				t.Fatalf("ClassifyPreviousResponseIDKind(%q)=%q want=%q", tc.id, got, tc.want)
			}
		})
	}
}

func TestIsPreviousResponseIDLikelyMessageID(t *testing.T) {
	if !IsPreviousResponseIDLikelyMessageID("msg_123") {
		t.Fatal("expected msg_123 to be identified as message id")
	}
	if IsPreviousResponseIDLikelyMessageID("resp_123") {
		t.Fatal("expected resp_123 not to be identified as message id")
	}
}
