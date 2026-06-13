//go:build unit

package service

import "context"

func (s *accountRepoStub) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	panic("unexpected ListOAuthRefreshCandidates call")
REDACTED

func (r *openAIAccountTestRepo) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	panic("unexpected ListOAuthRefreshCandidates call")
REDACTED

func (m *groupAwareMockAccountRepo) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	panic("unexpected ListOAuthRefreshCandidates call")
REDACTED

func (m *mockAccountRepoForPlatform) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	panic("unexpected ListOAuthRefreshCandidates call")
REDACTED

func (m *mockAccountRepoForGemini) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	return m.ListActive(context.Background())
REDACTED
