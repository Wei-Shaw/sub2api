package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var registrationEmailDomainPattern = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9-]{0,61REDACTED[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61REDACTED[a-z0-9])?)+$`,
)

// RegistrationEmailSuffix extracts normalized suffix in "@domain" form.
func RegistrationEmailSuffix(email string) string {
	_, domain, ok := splitEmailForPolicy(email)
	if !ok {
		return ""
REDACTED
	return "@" + domain
REDACTED

// IsRegistrationEmailSuffixAllowed checks whether an email is allowed by suffix whitelist.
// Empty whitelist means allow all.
func IsRegistrationEmailSuffixAllowed(email string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true
REDACTED
	_, domain, ok := splitEmailForPolicy(email)
	if !ok {
		return false
REDACTED
	suffix := "@" + domain
	for _, allowed := range whitelist {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if strings.HasPrefix(allowed, "@") && suffix == allowed {
			return true
	REDACTED
		if strings.HasPrefix(allowed, "*.") && registrationEmailDomainMatchesWildcard(domain, allowed) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// NormalizeRegistrationEmailSuffixWhitelist normalizes and validates suffix whitelist items.
func NormalizeRegistrationEmailSuffixWhitelist(raw []string) ([]string, error) {
	return normalizeRegistrationEmailSuffixWhitelist(raw, true)
REDACTED

// ParseRegistrationEmailSuffixWhitelist parses persisted JSON into normalized suffixes.
// Invalid entries are ignored to keep old misconfigurations from breaking runtime reads.
func ParseRegistrationEmailSuffixWhitelist(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{REDACTED
REDACTED
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{REDACTED
REDACTED
	normalized, _ := normalizeRegistrationEmailSuffixWhitelist(items, false)
	if len(normalized) == 0 {
		return []string{REDACTED
REDACTED
	return normalized
REDACTED

func normalizeRegistrationEmailSuffixWhitelist(raw []string, strict bool) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
REDACTED

	seen := make(map[string]struct{REDACTED, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		normalized, err := normalizeRegistrationEmailSuffix(item)
		if err != nil {
			if strict {
				return nil, err
		REDACTED
			continue
	REDACTED
		if normalized == "" {
			continue
	REDACTED
		if _, ok := seen[normalized]; ok {
			continue
	REDACTED
		seen[normalized] = struct{REDACTED{REDACTED
		out = append(out, normalized)
REDACTED

	if len(out) == 0 {
		return nil, nil
REDACTED
	return out, nil
REDACTED

func normalizeRegistrationEmailSuffix(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", nil
REDACTED

	if strings.HasPrefix(value, "*.") {
		domain := strings.TrimPrefix(value, "*.")
		if !isValidRegistrationEmailDomain(domain) {
			return "", fmt.Errorf("invalid email suffix: %q", raw)
	REDACTED
		return "*." + domain, nil
REDACTED

	domain := value
	if strings.Contains(value, "@") {
		if !strings.HasPrefix(value, "@") || strings.Count(value, "@") != 1 {
			return "", fmt.Errorf("invalid email suffix: %q", raw)
	REDACTED
		domain = strings.TrimPrefix(value, "@")
REDACTED

	if !isValidRegistrationEmailDomain(domain) {
		return "", fmt.Errorf("invalid email suffix: %q", raw)
REDACTED

	return "@" + domain, nil
REDACTED

func isValidRegistrationEmailDomain(domain string) bool {
	return domain != "" &&
		!strings.Contains(domain, "@") &&
		registrationEmailDomainPattern.MatchString(domain)
REDACTED

func registrationEmailDomainMatchesWildcard(domain string, allowed string) bool {
	base := strings.TrimPrefix(allowed, "*.")
	if !isValidRegistrationEmailDomain(base) {
		return false
REDACTED
	return domain == base || strings.HasSuffix(domain, "."+base)
REDACTED

func splitEmailForPolicy(raw string) (local string, domain string, ok bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	local, domain, found := strings.Cut(email, "@")
	if !found || local == "" || domain == "" || strings.Contains(domain, "@") {
		return "", "", false
REDACTED
	return local, domain, true
REDACTED
