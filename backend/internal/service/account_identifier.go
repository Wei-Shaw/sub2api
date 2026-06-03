package service

import (
	"strings"
	"unicode"
)

type AccountIdentifierKind string

const (
	AccountIdentifierEmail AccountIdentifierKind = "email"
	AccountIdentifierPhone AccountIdentifierKind = "phone"
)

func NormalizeAccountIdentifier(raw string) (string, AccountIdentifierKind, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 255 || hasControlRune(value) {
		return "", "", ErrInvalidAccountIdentifier
	}
	if strings.Contains(value, "@") {
		normalized, err := NormalizeEmailAccountIdentifier(value)
		if err != nil {
			return "", "", ErrInvalidAccountIdentifier
		}
		return normalized, AccountIdentifierEmail, nil
	}

	normalized, err := normalizePhoneAccountIdentifier(value)
	if err != nil {
		return "", "", ErrInvalidAccountIdentifier
	}
	return normalized, AccountIdentifierPhone, nil
}

func NormalizeEmailAccountIdentifier(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 255 || hasControlRune(value) || strings.ContainsAny(value, " \t\r\n") {
		return "", ErrInvalidEmail
	}

	parts := strings.Split(value, "@")
	if len(parts) != 2 {
		return "", ErrInvalidEmail
	}

	local := parts[0]
	domain := strings.ToLower(parts[1])
	if !isCommonEmailLocalPart(local) || !isCommonEmailDomain(domain) {
		return "", ErrInvalidEmail
	}
	return strings.ToLower(local) + "@" + domain, nil
}

func IsEmailAccountIdentifier(raw string) bool {
	_, err := NormalizeEmailAccountIdentifier(raw)
	return err == nil
}

func normalizePhoneAccountIdentifier(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 64 {
		return "", ErrInvalidAccountIdentifier
	}

	var out strings.Builder
	plusSeen := false
	digitCount := 0
	for i, r := range value {
		switch {
		case r >= '0' && r <= '9':
			out.WriteRune(r)
			digitCount++
		case r == '+':
			if plusSeen || i != 0 {
				return "", ErrInvalidAccountIdentifier
			}
			plusSeen = true
			out.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')':
			continue
		default:
			return "", ErrInvalidAccountIdentifier
		}
	}

	if digitCount < 6 || digitCount > 20 {
		return "", ErrInvalidAccountIdentifier
	}
	normalized := out.String()
	if normalized == "" || normalized == "+" {
		return "", ErrInvalidAccountIdentifier
	}
	return normalized, nil
}

func hasControlRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func isCommonEmailLocalPart(local string) bool {
	if local == "" || len(local) > 64 || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return false
	}
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '%' || r == '+' || r == '-':
		default:
			return false
		}
	}
	return true
}

func isCommonEmailDomain(domain string) bool {
	if domain == "" || len(domain) > 253 || !strings.Contains(domain, ".") {
		return false
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= '0' && r <= '9':
			case r == '-':
			default:
				return false
			}
		}
	}
	return true
}
