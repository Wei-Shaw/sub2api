package handler

import (
	"context"
	"net/mail"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type weComEmailCandidate struct {
	source string
	value  string
}

func selectWeComRegistrationEmail(ctx context.Context, client *dbent.Client, fallback string, claims map[string]any) string {
	for _, candidate := range []weComEmailCandidate{
		{source: "email", value: pendingSessionStringValue(claims, "wecom_email")},
		{source: "biz_mail", value: pendingSessionStringValue(claims, "wecom_biz_mail")},
	} {
		email, ok := normalizeWeComEmail(candidate.value)
		if !ok {
			continue
		}
		claims["email"] = email
		claims["wecom_registration_email_source"] = candidate.source
		return email
	}
	email := strings.TrimSpace(strings.ToLower(fallback))
	claims["email"] = email
	claims["wecom_registration_email_source"] = "synthetic"
	return email
}

func normalizeWeComEmail(email string) (string, bool) {
	normalized := strings.TrimSpace(strings.ToLower(email))
	if normalized == "" || len(normalized) > 255 || isWeComReservedEmail(normalized) {
		return "", false
	}
	address, err := mail.ParseAddress(normalized)
	if err != nil || address.Address != normalized {
		return "", false
	}
	return normalized, true
}

func isWeComReservedEmail(email string) bool {
	normalized := strings.TrimSpace(strings.ToLower(email))
	return strings.HasSuffix(normalized, service.LinuxDoConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, service.OIDCConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, service.WeChatConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, service.WeComConnectSyntheticEmailDomain)
}
