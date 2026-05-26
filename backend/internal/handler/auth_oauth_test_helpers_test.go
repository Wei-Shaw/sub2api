package handler

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func buildEncodedOAuthBindUserCookie(t *testing.T, userID int64, secret string) string {
	t.Helper()
	value, err := buildOAuthBindUserCookieValue(userID, secret)
	require.NoError(t, err)
	return value
}

func encodedCookie(name, value string) *http.Cookie {
	return &http.Cookie{
		Name:  name,
		Value: encodeCookieValue(value),
		Path:  "/",
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func decodeCookieValueForTest(t *testing.T, value string) string {
	t.Helper()
	decoded, err := decodeCookieValue(value)
	require.NoError(t, err)
	return decoded
}

func assertOAuthRedirectError(t *testing.T, location string, errorCode string, errorMessage string) {
	t.Helper()
	require.NotEmpty(t, location)

	parsed, err := url.Parse(location)
	require.NoError(t, err)

	rawValues := parsed.RawQuery
	if rawValues == "" {
		rawValues = parsed.Fragment
	}
	values, err := url.ParseQuery(rawValues)
	require.NoError(t, err)
	require.Equal(t, errorCode, values.Get("error"))
	require.Equal(t, errorMessage, values.Get("error_message"))
}
