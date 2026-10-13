package domain

import "testing"

func TestGroupI18nLocalize(t *testing.T) {
	i18n := GroupI18n{
		"en":    {Name: "Standard", Description: "Pro + Max pool"},
		"zh-TW": {Name: "標準"},
		"ja":    {Description: "説明のみ"},
	}
	const name, description = "标准", "号池"

	tests := []struct {
		title           string
		acceptLanguage  string
		wantName        string
		wantDescription string
	}{
		{"exact locale", "en", "Standard", "Pro + Max pool"},
		{"region falls back to the primary language", "en-US", "Standard", "Pro + Max pool"},
		{"locale match ignores case and underscore", "ZH_tw", "標準", description},
		{"a missing field keeps the original text", "ja", name, "説明のみ"},
		{"no translation for the language keeps both", "zh", name, description},
		{"primary language does not match a regional translation", "zh-CN", name, description},
		{"empty header keeps both", "", name, description},
		{"wildcard only keeps both", "*", name, description},
		// 原字段的语言未知，不退到次选语言：浏览器的 zh 用户不应拿到英文译文。
		{"only the preferred language is considered", "zh-CN,zh;q=0.9,en;q=0.8", name, description},
		{"highest weight wins over position", "zh;q=0.5, en;q=0.9", "Standard", "Pro + Max pool"},
		{"equal weight keeps the first", "en, zh", "Standard", "Pro + Max pool"},
		{"entries with q=0 are ignored", "en;q=0, ja", name, "説明のみ"},
		{"malformed weight is skipped", "en;q=abc, ja", name, "説明のみ"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			gotName, gotDescription := i18n.Localize(tt.acceptLanguage, name, description)
			if gotName != tt.wantName || gotDescription != tt.wantDescription {
				t.Fatalf("Localize(%q) = (%q, %q), want (%q, %q)",
					tt.acceptLanguage, gotName, gotDescription, tt.wantName, tt.wantDescription)
			}
		})
	}
}

func TestGroupI18nLocalize_NilMap(t *testing.T) {
	var i18n GroupI18n

	name, description := i18n.Localize("en", "标准", "号池")

	if name != "标准" || description != "号池" {
		t.Fatalf("nil translations must keep the original text, got (%q, %q)", name, description)
	}
	if got := i18n.LocalizeName("en", "标准"); got != "标准" {
		t.Fatalf("LocalizeName on nil translations = %q", got)
	}
}

func TestGroupI18nLocalizeName(t *testing.T) {
	i18n := GroupI18n{"en": {Name: "Standard"}}

	if got := i18n.LocalizeName("en", "标准"); got != "Standard" {
		t.Fatalf("LocalizeName(en) = %q", got)
	}
	if got := i18n.LocalizeName("zh", "标准"); got != "标准" {
		t.Fatalf("LocalizeName(zh) = %q", got)
	}
}

func TestCanonicalLocale(t *testing.T) {
	tests := map[string]string{
		"en":            "en",
		"EN":            "en",
		"zh-tw":         "zh-TW",
		"ZH-Hant-tw":    "zh-Hant-TW",
		"es-419":        "es-419",
		"de-CH-1901":    "de-CH-1901",
		"en-US-x-Twain": "en-US-x-twain",
	}
	for in, want := range tests {
		if got := CanonicalLocale(in); got != want {
			t.Errorf("CanonicalLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

// 写入路径已拒绝仅大小写不同的语言代码；已存在的此类数据读取时也必须得到固定结果。
func TestGroupI18nLocalize_CaseCollidingKeysAreDeterministic(t *testing.T) {
	tests := []struct {
		title          string
		i18n           GroupI18n
		acceptLanguage string
		want           string
	}{
		{"canonical key wins", GroupI18n{"EN": {Name: "B"}, "en": {Name: "A"}, "En": {Name: "C"}}, "en", "A"},
		{"otherwise the smallest key wins", GroupI18n{"eN": {Name: "B"}, "EN": {Name: "A"}}, "en", "A"},
		{"primary language fallback is deterministic too", GroupI18n{"ZH": {Name: "B"}, "zh": {Name: "A"}}, "zh-CN", "A"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			for range 200 {
				if got := tt.i18n.LocalizeName(tt.acceptLanguage, "原名"); got != tt.want {
					t.Fatalf("LocalizeName(%q) = %q, want %q", tt.acceptLanguage, got, tt.want)
				}
			}
		})
	}
}
