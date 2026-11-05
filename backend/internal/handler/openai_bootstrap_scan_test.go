package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func referenceBootstrapKinds(body []byte, automation, delegation bool) ([]byte, bool, bool) {
	var a, d bool
	if automation {
		body, a = referenceCodexCallOutputBootstrap(body, isCodexAutomationCandidate, false)
	}
	if delegation {
		body, d = referenceCodexCallOutputBootstrap(body, isCodexDelegationCandidate, true)
	}
	return body, a, d
}

func checkBootstrapEquivalent(t testing.TB, body []byte) {
	t.Helper()
	original := bytes.Clone(body)
	for _, mode := range [][2]bool{{true, true}, {true, false}, {false, true}} {
		want, wa, wd := referenceBootstrapKinds(body, mode[0], mode[1])
		got, ga, gd := normalizeCodexBootstrapKinds(body, mode[0], mode[1])
		if wa != ga || wd != gd {
			t.Fatalf("flags mode=%v want=%v/%v got=%v/%v body=%.2000s", mode, wa, wd, ga, gd, body)
		}
		if !bytes.Equal(original, body) {
			t.Fatal("input mutated")
		}
		if !ga && !gd {
			if !bytes.Equal(got, body) || len(body) > 0 && &got[0] != &body[0] {
				t.Fatal("no-op must return original bytes and backing array")
			}
			continue
		}
		decode := func(b []byte) any {
			var v any
			dec := json.NewDecoder(bytes.NewReader(b))
			dec.UseNumber()
			if err := dec.Decode(&v); err != nil {
				t.Fatal(err)
			}
			return v
		}
		if !reflect.DeepEqual(decode(want), decode(got)) {
			t.Fatalf("semantic mismatch mode=%v\nwant=%.2000s\ngot=%.2000s", mode, want, got)
		}
	}
}

func bootstrapCandidate(name, output string) string {
	b, _ := json.Marshal(map[string]any{"type": "function_call_output", "namespace": "codex_app", "name": name, "output": output})
	return string(b)
}
func bootstrapTestItems() []string {
	a := bootstrapCandidate("automation_update", codexAutomationBootstrap("wiki", "never", "Review"))
	d := bootstrapCandidate("create_thread", delegationEnvelope)
	return []string{a, d, bootstrapCandidate("automation_update", "<heartbeat><automation_id>wiki</automation_id></heartbeat>"),
		`{"type":"message","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}`,
		`{"type":"function_call","call_id":"paired"}`, `{"type":"function_call"}`,
		`{"type":"function_call_output","call_id":"paired","output":"ordinary"}`,
		`{"type":"tool_search_output"}`, `{"type":"item_reference","id":"history"}`,
		`{"type":"item_reference","id":null}`, `null`, `42`, `["nested"]`,
		strings.TrimSuffix(a, "}") + `,"call_id":""}`,
		strings.TrimSuffix(d, "}") + `,"call_id":" "}`,
		strings.TrimSuffix(d, "}") + `,"call_id":null}`,
		strings.TrimSuffix(a, "}") + `,"call_id":"paired"}`,
		`{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":{}}`,
	}
}

func TestBootstrapSinglePassDifferential(t *testing.T) {
	items := bootstrapTestItems()
	extras := []string{"", `,"previous_response_id":""`, `,"previous_response_id":"old"`, `,"previous_response_id":null`, `,"previous_response_id":42`, `,"previous_response_id":"  "`}
	for _, x := range items {
		for _, y := range items {
			for _, extra := range extras {
				checkBootstrapEquivalent(t, []byte(`{"input":[`+x+","+y+"]"+extra+"}"))
			}
		}
	}
	rng := rand.New(rand.NewSource(7910))
	for n := 0; n < 1500; n++ {
		var selected []string
		count := rng.Intn(15) + 1
		for i := 0; i < count; i++ {
			selected = append(selected, items[rng.Intn(len(items))])
		}
		checkBootstrapEquivalent(t, []byte(`{"input":[`+strings.Join(selected, ",")+"]"+extras[rng.Intn(len(extras))]+"}"))
	}
}

func TestBootstrapSinglePassJSONEdges(t *testing.T) {
	a := bootstrapTestItems()[0]
	base := `{"input":[` + a + `],"extra":`
	for _, v := range []string{`null`, `true`, `-0`, `1.20e+3`, `9007199254740993`, `1e999`, `-1e999`, `1e-999`, `01`, `-`, `1.`, `1e+`, `NaN`, `"\uD800"`, `"\udc00"`, `"\u0000"`, `"\q"`, `{"a":1,"\u0061":2}`, `[{"x":1,"x":2}]`, `[1,]`, `{"x":1,}`, `"raw` + string([]byte{0xff}) + `"`} {
		t.Run(fmt.Sprintf("%q", v), func(t *testing.T) { checkBootstrapEquivalent(t, []byte(base+v+"}")) })
	}
	for _, body := range []string{"", "null", "[]", `{"input":null}`, `{"input":{"input":[` + a + `]}}`,
		`{"input":[` + strings.ReplaceAll(a, `"type"`, `"\u0074ype"`) + `]}`,
		`{"input":[` + a + `],"input":[]}`, base + `{} } {}`, base + `{} } trailing`,
		base + strings.Repeat("[", 9999) + "0" + strings.Repeat("]", 9999) + "}",
		base + strings.Repeat("[", 10000) + "0" + strings.Repeat("]", 10000) + "}",
	} {
		checkBootstrapEquivalent(t, []byte(body))
	}
	body := []byte(base + `{"x":[true,false,null,"escaped\\text"]}}`)
	for i := range body {
		checkBootstrapEquivalent(t, body[:i])
	}
}

func TestBootstrapSinglePassPreservesRawSpans(t *testing.T) {
	raw := `{ "type":"message", "content":"\u0061", "number":1.20e+3 }`
	body := []byte("{\n \"input\" : [" + raw + "," + bootstrapTestItems()[1] + "], \"other\": 9007199254740993 }")
	got, _, d := normalizeCodexBootstrapKinds(body, true, true)
	if !d || !bytes.Contains(got, []byte(raw)) || !bytes.HasPrefix(got, []byte("{\n \"input\" : [")) {
		t.Fatalf("raw spans not preserved: %s", got)
	}
	checkBootstrapEquivalent(t, body)
}

func TestBootstrapSinglePassImageHistories(t *testing.T) {
	for _, scenario := range []string{"noop", "first", "last", "blocked", "automation", "mixed"} {
		t.Run(scenario, func(t *testing.T) {
			checkBootstrapEquivalent(t, bootstrapBenchmarkBody(1<<20, scenario))
		})
	}
}

func FuzzBootstrapSinglePass(f *testing.F) {
	for _, item := range bootstrapTestItems() {
		f.Add([]byte(`{"input":[` + item + `]}`))
	}
	f.Add([]byte(`{"input":[],"extra":{"a":1,"\u0061":2}}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 256<<10 {
			t.Skip()
		}
		checkBootstrapEquivalent(t, body)
		// Keep a real candidate while mutating arbitrary JSON elsewhere.
		checkBootstrapEquivalent(t, append(append([]byte(`{"input":[`+bootstrapTestItems()[0]+`],"extra":`), body...), '}'))
	})
}

// Synthetic image histories approximate large requests without customer data.
func bootstrapBenchmarkBody(size int, scenario string) []byte {
	var items []string
	image := strings.Repeat("A", size/12)
	for i := 0; i < 12; i++ {
		items = append(items, `{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,`+image+`"}]}`)
	}
	if size >= 1<<20 {
		for i := 12; i < 400; i++ {
			items = append(items, `{"type":"message","role":"user","content":[{"type":"input_text","text":"Synthetic history"}]}`)
		}
	}
	candidate := bootstrapTestItems()[1]
	switch scenario {
	case "first":
		items = append([]string{candidate}, items...)
	case "last":
		items = append(items, candidate)
	case "blocked":
		items = append([]string{candidate}, items...)
		items = append(items, `{"type":"function_call"}`)
	case "automation":
		items = append(items, bootstrapTestItems()[0])
	case "mixed":
		items = append(items, bootstrapTestItems()[0], candidate)
	}
	return []byte(`{"model":"gpt-5","input":[` + strings.Join(items, ",") + `]}`)
}

var bootstrapBenchmarkSink []byte

func BenchmarkBootstrapSinglePass(b *testing.B) {
	for _, size := range []int{1024, 1 << 20, 18 << 20, 80 << 20} {
		label := fmt.Sprintf("%dMiB", size>>20)
		if size == 1024 {
			label = "1KiB"
		}
		for _, scenario := range []string{"noop", "first", "last", "blocked", "automation", "mixed"} {
			body := bootstrapBenchmarkBody(size, scenario)
			for _, impl := range []string{"old", "new"} {
				b.Run(fmt.Sprintf("%s/%s/%s", label, scenario, impl), func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(body)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if impl == "old" {
							bootstrapBenchmarkSink, _, _ = referenceBootstrapKinds(body, true, true)
						} else {
							bootstrapBenchmarkSink, _, _ = normalizeCodexBootstrapKinds(body, true, true)
						}
					}
				})
			}
		}
	}
}
