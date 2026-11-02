package service

import (
	"strings"
	"testing"
)

func TestDecisionsModerationIncludesQuestionsAndAllEvidence(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","content":"first evidence"},{"role":"user","content":[{"type":"input_text","text":"second evidence"},{"type":"input_image","image_url":"data:image/png;base64,BASE64SECRET"}]}],"questions":[{"instructions":"question instruction","choices":[{"value":"option","description":"option description"}],"levels":[{"label":"rubric","description":"rubric description"}]}]}`)
	got := ExtractContentModerationInput(ContentModerationProtocolOpenAIDecisions, body)
	for _, want := range []string{"first evidence", "second evidence", "question instruction", "option description", "rubric description"} {
		if !strings.Contains(got.Text, want) {
			t.Fatalf("missing %q from audit", want)
		}
	}
	if strings.Contains(got.Text, "BASE64SECRET") {
		t.Fatal("image base64 leaked into audit text")
	}
}
