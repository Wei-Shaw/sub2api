//go:build unit

package inbox

import (
	"encoding/json"
	"testing"
)

func TestParseTargeting_Valid(t *testing.T) {
	cases := []string{
		`{"op":"all_users"}`,
		`{"op":"equals","attr":"plan","value":"pro"}`,
		`{"op":"equals","attr":"level","value":3}`,
		`{"op":"in","attr":"plan","values":["pro","team"]}`,
		`{"op":"and","clauses":[{"op":"equals","attr":"plan","value":"pro"},{"op":"in","attr":"region","values":["cn","us"]}]}`,
		`{"op":"or","clauses":[{"op":"all_users"}]}`,
	}
	for _, raw := range cases {
		if _, err := ParseTargeting(json.RawMessage(raw)); err != nil {
			t.Fatalf("expected valid targeting %q, got err: %v", raw, err)
		}
	}
}

func TestParseTargeting_Invalid(t *testing.T) {
	cases := []string{
		``,                                      // 空
		`{}`,                                    // 缺 op
		`{"op":"unknown"}`,                      // 未知 op
		`{"op":"equals","attr":"plan"}`,         // equals 缺 value
		`{"op":"equals","value":"pro"}`,         // equals 缺 attr
		`{"op":"in","attr":"plan"}`,             // in 缺 values
		`{"op":"in","attr":"plan","values":[]}`, // in values 空
		`{"op":"and","clauses":[]}`,             // and clauses 空
		`{"op":"or"}`,                           // or 缺 clauses
		`not-json`,                              // 非法 JSON
	}
	for _, raw := range cases {
		if _, err := ParseTargeting(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected invalid targeting %q to error", raw)
		}
	}
}

func TestParseTargeting_DepthLimit(t *testing.T) {
	// 构造超过 maxTargetingDepth 的深层嵌套
	raw := `{"op":"all_users"}`
	for i := 0; i <= maxTargetingDepth+1; i++ {
		raw = `{"op":"and","clauses":[` + raw + `]}`
	}
	if _, err := ParseTargeting(json.RawMessage(raw)); err == nil {
		t.Fatal("expected depth-limit error")
	}
}

func TestTargeting_Match(t *testing.T) {
	tests := []struct {
		name      string
		targeting string
		attrs     map[string]any
		want      bool
	}{
		{
			name:      "all_users always matches",
			targeting: `{"op":"all_users"}`,
			attrs:     map[string]any{},
			want:      true,
		},
		{
			name:      "equals string hit",
			targeting: `{"op":"equals","attr":"plan","value":"pro"}`,
			attrs:     map[string]any{"plan": "pro"},
			want:      true,
		},
		{
			name:      "equals string miss",
			targeting: `{"op":"equals","attr":"plan","value":"pro"}`,
			attrs:     map[string]any{"plan": "free"},
			want:      false,
		},
		{
			name:      "equals missing attr",
			targeting: `{"op":"equals","attr":"plan","value":"pro"}`,
			attrs:     map[string]any{},
			want:      false,
		},
		{
			name:      "equals numeric cross-type (int64 vs json float)",
			targeting: `{"op":"equals","attr":"level","value":3}`,
			attrs:     map[string]any{"level": int64(3)},
			want:      true,
		},
		{
			name:      "in hit",
			targeting: `{"op":"in","attr":"region","values":["cn","us"]}`,
			attrs:     map[string]any{"region": "us"},
			want:      true,
		},
		{
			name:      "in miss",
			targeting: `{"op":"in","attr":"region","values":["cn","us"]}`,
			attrs:     map[string]any{"region": "jp"},
			want:      false,
		},
		{
			name:      "and all hit",
			targeting: `{"op":"and","clauses":[{"op":"equals","attr":"plan","value":"pro"},{"op":"in","attr":"region","values":["cn"]}]}`,
			attrs:     map[string]any{"plan": "pro", "region": "cn"},
			want:      true,
		},
		{
			name:      "and one miss",
			targeting: `{"op":"and","clauses":[{"op":"equals","attr":"plan","value":"pro"},{"op":"in","attr":"region","values":["cn"]}]}`,
			attrs:     map[string]any{"plan": "pro", "region": "us"},
			want:      false,
		},
		{
			name:      "or one hit",
			targeting: `{"op":"or","clauses":[{"op":"equals","attr":"plan","value":"pro"},{"op":"equals","attr":"plan","value":"team"}]}`,
			attrs:     map[string]any{"plan": "team"},
			want:      true,
		},
		{
			name:      "or none hit",
			targeting: `{"op":"or","clauses":[{"op":"equals","attr":"plan","value":"pro"},{"op":"equals","attr":"plan","value":"team"}]}`,
			attrs:     map[string]any{"plan": "free"},
			want:      false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tg, err := ParseTargeting(json.RawMessage(tc.targeting))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := tg.Match(tc.attrs); got != tc.want {
				t.Fatalf("Match=%v want %v", got, tc.want)
			}
		})
	}
}

func TestTargeting_MatchNilSafe(t *testing.T) {
	var tg *Targeting
	if tg.Match(map[string]any{"a": 1}) {
		t.Fatal("nil targeting must not match")
	}
}
