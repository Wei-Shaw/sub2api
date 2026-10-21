package minimax

import (
	"reflect"
	"testing"
)

func TestDefaultModelsFlashFirst(t *testing.T) {
	want := []string{"MiniMax-M3.1-Flash", "MiniMax-M3"}
	if got := DefaultModelIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("default models = %v, want %v", got, want)
	}
	if DefaultTestModel != want[0] {
		t.Fatalf("default test model = %q", DefaultTestModel)
	}
	for _, model := range DefaultModels {
		if model.ContextWindow != 1000000 || !reflect.DeepEqual(model.Modalities, []string{"text", "image", "video"}) {
			t.Fatalf("unexpected capabilities: %+v", model)
		}
	}
	if !reflect.DeepEqual(DefaultModels[0].Thinking, []string{"adaptive"}) {
		t.Fatal("Flash must advertise adaptive thinking only")
	}
	if !reflect.DeepEqual(DefaultModels[1].Thinking, []string{"adaptive", "disabled"}) {
		t.Fatal("M3 thinking modes must be preserved")
	}
}
