package handler

import "testing"

func TestModelFromJSONBody(t *testing.T) {
	t.Parallel()
	if got := modelFromJSONBody([]byte(`{"model":"gpt-4"}`), "fallback"); got != "gpt-4" {
		t.Fatalf("got %q", got)
	}
	if got := modelFromJSONBody([]byte(`{"model":"  "}`), "fallback"); got != "fallback" {
		t.Fatalf("empty model: %q", got)
	}
	if got := modelFromJSONBody([]byte(`{}`), "fallback"); got != "fallback" {
		t.Fatalf("missing: %q", got)
	}
}

func TestStreamFromJSONBody(t *testing.T) {
	t.Parallel()
	if got := streamFromJSONBody([]byte(`{"stream":true}`), false); !got {
		t.Fatal("expected true")
	}
	if got := streamFromJSONBody([]byte(`{"stream":false}`), true); got {
		t.Fatal("expected false")
	}
	if got := streamFromJSONBody([]byte(`{}`), true); !got {
		t.Fatal("missing keeps fallback")
	}
	if got := streamFromJSONBody([]byte(`{"stream":"yes"}`), true); !got {
		t.Fatal("invalid type keeps fallback")
	}
}
