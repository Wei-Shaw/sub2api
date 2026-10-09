package securityaudit

import (
	"strings"
	"testing"
)

func TestDecisionsPromptSnapshotCoversQuestions(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"evidence"},{"type":"input_image","image_url":"data:image/png;base64,BASE64SECRET"}]}],"questions":[{"instructions":"instruction","choices":[{"value":"choice","description":"choice description"}],"levels":[{"label":"label","description":"rubric description"}]}]}`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_decisions", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"evidence", "instruction", "choice description", "rubric description"} {
		if !strings.Contains(snapshot.ScanText, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(snapshot.ScanText, "BASE64SECRET") {
		t.Fatal("image base64 leaked into prompt snapshot")
	}
}
