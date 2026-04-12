package dto

import "github.com/Wei-Shaw/sub2api/internal/service"

// NotifyEmailEntry represents a notification email with enable/disable and verification state.
// Email="" is a placeholder for the "primary email" (user's registration email or first admin email).
type NotifyEmailEntry struct {
	Email    string `json:"email"`
	Disabled bool   `json:"disabled"`
	Verified bool   `json:"verified"`
REDACTED

// NotifyEmailEntriesFromService converts service entries to DTO entries.
func NotifyEmailEntriesFromService(entries []service.NotifyEmailEntry) []NotifyEmailEntry {
	if entries == nil {
		return nil
REDACTED
	result := make([]NotifyEmailEntry, len(entries))
	for i, e := range entries {
		result[i] = NotifyEmailEntry{
			Email:    e.Email,
			Disabled: e.Disabled,
			Verified: e.Verified,
	REDACTED
REDACTED
	return result
REDACTED

// NotifyEmailEntriesToService converts DTO entries to service entries.
func NotifyEmailEntriesToService(entries []NotifyEmailEntry) []service.NotifyEmailEntry {
	if entries == nil {
		return nil
REDACTED
	result := make([]service.NotifyEmailEntry, len(entries))
	for i, e := range entries {
		result[i] = service.NotifyEmailEntry{
			Email:    e.Email,
			Disabled: e.Disabled,
			Verified: e.Verified,
	REDACTED
REDACTED
	return result
REDACTED
