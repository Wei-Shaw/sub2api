package qualitytrace

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestModelTraceUpstreamParity(t *testing.T) {
	// Golden probabilities, scores and similarities come from the pinned
	// upstream static/fingerprint-core.js, including 1/2/3-output calibration.
	raw, err := os.ReadFile("testdata/modeltrace-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Outputs  []traceOutput
		Expected traceAttribution
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		got, err := analyzeModelTrace(tt.Outputs)
		if err != nil {
			t.Fatal(err)
		}
		want := tt.Expected
		if got.Prediction != want.Prediction || got.Used != want.Used || !reflect.DeepEqual(got.Diagnostics, want.Diagnostics) || len(got.Results) != len(want.Results) {
			t.Fatalf("attribution mismatch: %+v", got)
		}
		close := func(a, b float64) {
			t.Helper()
			if math.IsNaN(a) || math.Abs(a-b) > 1e-10 {
				t.Fatalf("got %.15g, want %.15g", a, b)
			}
		}
		close(got.Probability, want.Probability)
		close(got.FamilyProbability, want.FamilyProbability)
		for i, r := range got.Results {
			w := want.Results[i]
			if r.Model != w.Model || r.Family != w.Family {
				t.Fatalf("rank %d: %s, want %s", i, r.Model, w.Model)
			}
			close(r.Probability, w.Probability)
			close(r.Score, w.Score)
			close(r.Similarity, w.Similarity)
		}
		for i, f := range got.Families {
			close(f.Probability, want.Families[i].Probability)
		}
	}
}

func TestTraceLocalCatalogAndChallengeContract(t *testing.T) {
	if len(Models()) < 2 || !strings.HasPrefix(Revision(), "sha256:") {
		t.Fatal("embedded reference provenance missing")
	}
	for round := 0; round < 50; round++ {
		c, err := Challenges()
		if err != nil || len(c) != 3 {
			t.Fatal("three challenges required", err)
		}
		seen := map[int]bool{}
		for _, p := range c {
			if p.ExpectedCount < 292 || p.ExpectedCount > 332 || seen[p.ExpectedCount] {
				t.Fatal("challenge length invalid or duplicated")
			}
			seen[p.ExpectedCount] = true
		}
	}
}
func TestTraceDoesNotCertifyInvalidOrOutOfBankModels(t *testing.T) {
	if _, err := Analyze("unknown", make([]Output, 3)); err == nil {
		t.Fatal("out of bank model must be rejected")
	}
	m := Models()[0]
	if r, err := Analyze(m, []Output{{Text: "21", ExpectedCount: 300}}); err == nil || r.Verdict != "unknown" {
		t.Fatal("one probe cannot certify a model")
	}
	if r, err := Analyze(m, []Output{{Text: "21", ExpectedCount: 300}, {Text: "21", ExpectedCount: 301}, {Text: "21", ExpectedCount: 302}}); err == nil || r.Verdict != "unknown" {
		t.Fatal("invalid sequences must stay unknown")
	}
}
