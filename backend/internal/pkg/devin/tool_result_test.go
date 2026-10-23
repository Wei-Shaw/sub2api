package devin

import "testing"

func TestToolResultErrorFlag(t *testing.T) {
	body := []byte(`{"messages":[{"role":"tool","tool_call_id":"call","is_error":true,"content":"failed"}]}`)
	mapped, err := mapRequest(body, "cascade", DefaultModels()[0])
	if err != nil {
		t.Fatal(err)
	}
	message := mustProto(t, mustProto(t, mapped.wire).data(3))
	// GetChatMessageRequest.chatMessagePrompts=3;
	// ChatMessagePrompt.toolResultIsError=9 in the pinned upstream schema.
	if message.num(9) != 1 || message.text(7) != "call" || message.num(2) != 4 {
		t.Fatal("tool result error semantics lost")
	}
	_, err = mapRequest([]byte(`{"messages":[{"role":"user","is_error":true,"content":"hi"}]}`), "cascade", DefaultModels()[0])
	assertStatus(t, err, 400)
}
