package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func buildEncodedOAuthBindUserCookie(t *testing.T, userID int64, secret string) string {
REDACTED
	value, err := buildOAuthBindUserCookieValue(userID, secret)
REDACTED
	return value
REDACTED

func encodedCookie(name, value string) *http.Cookie {
	return &http.Cookie{
		Name:  name,
		Value: encodeCookieValue(value),
		Path:  "/",
REDACTED
REDACTED

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
	REDACTED
REDACTED
	return nil
REDACTED

func requireCookieCleared(t *testing.T, recorder *httptest.ResponseRecorder, name string) {
REDACTED
	cookie := findCookie(recorder.Result().Cookies(), name)
	require.NotNil(t, cookie)
	require.Equal(t, -1, cookie.MaxAge)
REDACTED

func decodeCookieValueForTest(t *testing.T, value string) string {
REDACTED
	decoded, err := decodeCookieValue(value)
REDACTED
	return decoded
REDACTED

func assertOAuthRedirectError(t *testing.T, location string, errorCode string, errorMessage string) {
REDACTED
	values := parseOAuthRedirectFragment(t, location)
	require.Equal(t, errorCode, values.Get("error"))
	require.Equal(t, errorMessage, values.Get("error_message"))
REDACTED

func parseOAuthRedirectFragment(t *testing.T, location string) url.Values {
REDACTED
	require.NotEmpty(t, location)

	parsed, err := url.Parse(location)
REDACTED

	rawValues := parsed.RawQuery
	if rawValues == "" {
		rawValues = parsed.Fragment
REDACTED
	values, err := url.ParseQuery(rawValues)
REDACTED
	return values
REDACTED
