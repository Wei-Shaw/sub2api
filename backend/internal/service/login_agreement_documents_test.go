package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultLoginAgreementDocumentsIncludeLocalizedContent(t *testing.T) {
	docs := defaultLoginAgreementDocuments()
	require.Len(t, docs, 5)

	expectedIDs := []string{
		"terms",
		"usage-policy",
		"supported-regions",
		"service-specific-terms",
		"privacy-data-processing",
	}
	byID := make(map[string]LoginAgreementDocument, len(docs))
	for i, doc := range docs {
		require.Equal(t, expectedIDs[i], doc.ID)
		require.NotEmpty(t, doc.Title)
		require.NotEmpty(t, doc.ContentMD)
		for _, locale := range []string{"zh", "en", "vi", "ko"} {
			require.NotEmpty(t, doc.TitleI18n[locale], "document %s missing %s title", doc.ID, locale)
			require.NotEmpty(t, doc.ContentMDI18n[locale], "document %s missing %s content", doc.ID, locale)
		}
		require.NotContains(t, byID, doc.ID, "duplicate document ID %s", doc.ID)
		byID[doc.ID] = doc
	}

	terms := byID["terms"]
	require.Contains(t, terms.ContentMDI18n["en"], "a shared AI API access and internal usage-credit service")
	require.Contains(t, terms.ContentMDI18n["en"], "internal usage credits")
	require.Contains(t, terms.ContentMDI18n["en"], "technical intermediary")
	require.Contains(t, terms.ContentMDI18n["en"], "indemnify")

	require.Contains(t, terms.ContentMDI18n["en"], "V-Claw is not under a duty to pre-screen or continuously monitor")

	usagePolicy := byID["usage-policy"]
	require.Contains(t, usagePolicy.ContentMDI18n["zh"], "共享 AI API 访问")
	require.Contains(t, usagePolicy.ContentMDI18n["en"], "shared AI API access")
	require.Contains(t, usagePolicy.ContentMDI18n["en"], "Users must use the Service only for lawful, authorized, and responsible purposes")
	require.Contains(t, usagePolicy.ContentMDI18n["en"], "Users remain responsible for all activity under their accounts and API keys")

	serviceSpecificTerms := byID["service-specific-terms"]
	require.Contains(t, serviceSpecificTerms.ContentMDI18n["en"], "service-specific rule conflicts with these general Terms")
	require.Contains(t, serviceSpecificTerms.ContentMDI18n["en"], "No installation warranty, handover warranty, fixed uptime guarantee")

	privacyNotice := byID["privacy-data-processing"]
	require.Contains(t, privacyNotice.TitleI18n["en"], "Privacy & Data Processing Notice")
	require.Contains(t, privacyNotice.ContentMDI18n["en"], "shared AI API access")
	require.Contains(t, privacyNotice.ContentMDI18n["en"], "upstream AI providers")

	badFragments := []string{
		"OPENCLAW",
		"99 CNY",
		"installation fee",
		"12-month warranty",
		"token sales",
		"Company sells",
		"dịch vụ cài đặt",
	}
	for _, doc := range docs {
		combined := doc.Title + "\n" + doc.ContentMD
		for _, value := range doc.TitleI18n {
			combined += "\n" + value
		}
		for _, value := range doc.ContentMDI18n {
			combined += "\n" + value
		}
		for _, fragment := range badFragments {
			require.NotContains(t, combined, fragment, "document %s should not contain reverted legacy fragment", doc.ID)
		}
	}
}

func TestLoginAgreementDocumentsPreserveLocalizedFields(t *testing.T) {
	docs := []LoginAgreementDocument{
		{
			ID:        "terms",
			Title:     "服务条款",
			ContentMD: "中文正文",
			TitleI18n: map[string]string{
				"zh": "服务条款",
				"vi": "Điều khoản dịch vụ",
				"en": "Terms of Service",
				"ko": "서비스 이용약관",
			},
			ContentMDI18n: map[string]string{
				"zh": "中文正文",
				"vi": "Nội dung tiếng Việt",
				"en": "English body",
				"ko": "한국어 본문",
			},
		},
	}

	raw, err := marshalLoginAgreementDocuments(docs)
	require.NoError(t, err)

	var encoded []LoginAgreementDocument
	require.NoError(t, json.Unmarshal([]byte(raw), &encoded))
	require.Len(t, encoded, 1)
	require.Equal(t, "Điều khoản dịch vụ", encoded[0].TitleI18n["vi"])
	require.Equal(t, "Nội dung tiếng Việt", encoded[0].ContentMDI18n["vi"])

	parsed := parseLoginAgreementDocuments(raw)
	require.Len(t, parsed, 1)
	require.Equal(t, "Terms of Service", parsed[0].TitleI18n["en"])
	require.Equal(t, "English body", parsed[0].ContentMDI18n["en"])
}
