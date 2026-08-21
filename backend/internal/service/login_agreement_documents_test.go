package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultLoginAgreementDocumentsIncludeLocalizedContent(t *testing.T) {
	docs := defaultLoginAgreementDocuments()
	require.NotEmpty(t, docs)

	byID := make(map[string]LoginAgreementDocument, len(docs))
	for _, doc := range docs {
		byID[doc.ID] = doc
	}

	terms := byID["terms"]
	require.NotEmpty(t, terms.TitleI18n["zh"])
	require.NotEmpty(t, terms.TitleI18n["en"])
	require.NotEmpty(t, terms.TitleI18n["vi"])
	require.NotEmpty(t, terms.TitleI18n["ko"])
	require.NotEmpty(t, terms.ContentMDI18n["zh"])
	require.NotEmpty(t, terms.ContentMDI18n["en"])
	require.NotEmpty(t, terms.ContentMDI18n["vi"])
	require.NotEmpty(t, terms.ContentMDI18n["ko"])

	usagePolicy := byID["usage-policy"]
	require.NotEmpty(t, usagePolicy.ContentMDI18n["zh"])
	require.Contains(t, usagePolicy.ContentMDI18n["zh"], "严禁非法使用")
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
