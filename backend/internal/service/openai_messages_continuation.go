package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAICompatSessionResponseBinding struct {
	ResponseID           string
	TurnState            string
	ContinuationDisabled bool
	ExpiresAt            time.Time
REDACTED

func openAICompatContinuationEnabled(account *Account, model string) bool {
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
REDACTED
	return shouldAutoInjectPromptCacheKeyForCompat(model)
REDACTED

func trimAnthropicCompatResponsesInputToLatestTurn(req *apicompat.ResponsesRequest) {
	if req == nil || len(req.Input) == 0 {
		return
REDACTED

	var items []apicompat.ResponsesInputItem
	if err := json.Unmarshal(req.Input, &items); err != nil || len(items) == 0 {
		return
REDACTED

	start := len(items) - 1
	for start > 0 && items[start].Type == "function_call_output" {
		start--
REDACTED
	trimmed := append([]apicompat.ResponsesInputItem(nil), items[start:]...)
	if len(trimmed) == len(items) {
		return
REDACTED
	if input, err := json.Marshal(trimmed); err == nil {
		req.Input = input
REDACTED
REDACTED

func isOpenAICompatPreviousResponseNotFound(statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if statusCode != http.StatusBadRequest && statusCode != http.StatusNotFound {
		return false
REDACTED
	check := func(s string) bool {
		lower := strings.ToLower(strings.TrimSpace(s))
		return strings.Contains(lower, "previous_response_not_found") ||
			(strings.Contains(lower, "previous response") && strings.Contains(lower, "not found")) ||
			(strings.Contains(lower, "unsupported parameter") && strings.Contains(lower, "previous_response_id"))
REDACTED
	if check(upstreamMsg) || check(string(upstreamBody)) {
		return true
REDACTED
	return check(gjson.GetBytes(upstreamBody, "error.code").String()) ||
		check(gjson.GetBytes(upstreamBody, "error.message").String())
REDACTED

func isOpenAICompatPreviousResponseUnsupported(statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
REDACTED
	check := func(s string) bool {
		lower := strings.ToLower(strings.TrimSpace(s))
		if !strings.Contains(lower, "previous_response_id") {
			return false
	REDACTED
		return strings.Contains(lower, "unsupported parameter") ||
			strings.Contains(lower, "only supported on responses websocket") ||
			strings.Contains(lower, "not supported")
REDACTED
	if check(upstreamMsg) || check(string(upstreamBody)) {
		return true
REDACTED
	return check(gjson.GetBytes(upstreamBody, "error.code").String()) ||
		check(gjson.GetBytes(upstreamBody, "error.message").String())
REDACTED

func openAICompatSessionResponseKey(c *gin.Context, account *Account, promptCacheKey string) string {
	key := strings.TrimSpace(promptCacheKey)
	if account == nil || key == "" {
		return ""
REDACTED
	apiKeyID := int64(0)
	if c != nil {
		apiKeyID = getAPIKeyIDFromContext(c)
REDACTED
	return strings.Join([]string{
		strconv.FormatInt(account.ID, 10),
		strconv.FormatInt(apiKeyID, 10),
		key,
REDACTED, "\x00")
REDACTED

func (s *OpenAIGatewayService) getOpenAICompatSessionResponseID(_ context.Context, c *gin.Context, account *Account, promptCacheKey string) string {
	if s == nil {
		return ""
REDACTED
	key := openAICompatSessionResponseKey(c, account, promptCacheKey)
	if key == "" {
		return ""
REDACTED
	raw, ok := s.openaiCompatSessionResponses.Load(key)
	if !ok {
		return ""
REDACTED
	binding, ok := raw.(openAICompatSessionResponseBinding)
	if !ok {
		s.openaiCompatSessionResponses.Delete(key)
		return ""
REDACTED
	if !binding.ExpiresAt.IsZero() && time.Now().After(binding.ExpiresAt) {
		s.openaiCompatSessionResponses.Delete(key)
		return ""
REDACTED
	if binding.ContinuationDisabled {
		return ""
REDACTED
	if strings.TrimSpace(binding.ResponseID) == "" {
		s.openaiCompatSessionResponses.Delete(key)
		return ""
REDACTED
	return strings.TrimSpace(binding.ResponseID)
REDACTED

func (s *OpenAIGatewayService) bindOpenAICompatSessionResponseID(_ context.Context, c *gin.Context, account *Account, promptCacheKey, responseID string) {
	if s == nil {
		return
REDACTED
	key := openAICompatSessionResponseKey(c, account, promptCacheKey)
	id := strings.TrimSpace(responseID)
	if key == "" || id == "" {
		return
REDACTED
	binding := openAICompatSessionResponseBinding{
		ResponseID: id,
		ExpiresAt:  time.Now().Add(s.openAIWSResponseStickyTTL()),
REDACTED
	if raw, ok := s.openaiCompatSessionResponses.Load(key); ok {
		if existing, ok := raw.(openAICompatSessionResponseBinding); ok {
			if existing.ContinuationDisabled {
				existing.ResponseID = ""
				existing.ExpiresAt = time.Now().Add(s.openAIWSResponseStickyTTL())
				s.openaiCompatSessionResponses.Store(key, existing)
				return
		REDACTED
			binding.TurnState = existing.TurnState
	REDACTED
REDACTED
	s.openaiCompatSessionResponses.Store(key, binding)
REDACTED

func (s *OpenAIGatewayService) deleteOpenAICompatSessionResponseID(_ context.Context, c *gin.Context, account *Account, promptCacheKey string) {
	if s == nil {
		return
REDACTED
	key := openAICompatSessionResponseKey(c, account, promptCacheKey)
	if key == "" {
		return
REDACTED
	raw, ok := s.openaiCompatSessionResponses.Load(key)
	if !ok {
		return
REDACTED
	binding, ok := raw.(openAICompatSessionResponseBinding)
	if !ok {
		s.openaiCompatSessionResponses.Delete(key)
		return
REDACTED
	binding.ResponseID = ""
	if strings.TrimSpace(binding.TurnState) == "" && !binding.ContinuationDisabled {
		s.openaiCompatSessionResponses.Delete(key)
		return
REDACTED
	binding.ExpiresAt = time.Now().Add(s.openAIWSResponseStickyTTL())
	s.openaiCompatSessionResponses.Store(key, binding)
REDACTED

func (s *OpenAIGatewayService) disableOpenAICompatSessionContinuation(_ context.Context, c *gin.Context, account *Account, promptCacheKey string) {
	if s == nil {
		return
REDACTED
	key := openAICompatSessionResponseKey(c, account, promptCacheKey)
	if key == "" {
		return
REDACTED
	binding := openAICompatSessionResponseBinding{
		ContinuationDisabled: true,
		ExpiresAt:            time.Now().Add(s.openAIWSResponseStickyTTL()),
REDACTED
	if raw, ok := s.openaiCompatSessionResponses.Load(key); ok {
		if existing, ok := raw.(openAICompatSessionResponseBinding); ok {
			binding.TurnState = existing.TurnState
	REDACTED
REDACTED
	s.openaiCompatSessionResponses.Store(key, binding)
REDACTED

func (s *OpenAIGatewayService) isOpenAICompatSessionContinuationDisabled(_ context.Context, c *gin.Context, account *Account, promptCacheKey string) bool {
	if s == nil {
		return false
REDACTED
	key := openAICompatSessionResponseKey(c, account, promptCacheKey)
	if key == "" {
		return false
REDACTED
	raw, ok := s.openaiCompatSessionResponses.Load(key)
	if !ok {
		return false
REDACTED
	binding, ok := raw.(openAICompatSessionResponseBinding)
	if !ok {
		s.openaiCompatSessionResponses.Delete(key)
		return false
REDACTED
	if !binding.ExpiresAt.IsZero() && time.Now().After(binding.ExpiresAt) {
		s.openaiCompatSessionResponses.Delete(key)
		return false
REDACTED
	return binding.ContinuationDisabled
REDACTED

func (s *OpenAIGatewayService) getOpenAICompatSessionTurnState(_ context.Context, c *gin.Context, account *Account, promptCacheKey string) string {
	if s == nil {
		return ""
REDACTED
	key := openAICompatSessionResponseKey(c, account, promptCacheKey)
	if key == "" {
		return ""
REDACTED
	raw, ok := s.openaiCompatSessionResponses.Load(key)
	if !ok {
		return ""
REDACTED
	binding, ok := raw.(openAICompatSessionResponseBinding)
	if !ok || strings.TrimSpace(binding.TurnState) == "" {
		return ""
REDACTED
	if !binding.ExpiresAt.IsZero() && time.Now().After(binding.ExpiresAt) {
		s.openaiCompatSessionResponses.Delete(key)
		return ""
REDACTED
	return strings.TrimSpace(binding.TurnState)
REDACTED

func (s *OpenAIGatewayService) bindOpenAICompatSessionTurnState(_ context.Context, c *gin.Context, account *Account, promptCacheKey, turnState string) {
	if s == nil {
		return
REDACTED
	key := openAICompatSessionResponseKey(c, account, promptCacheKey)
	state := strings.TrimSpace(turnState)
	if key == "" || state == "" {
		return
REDACTED
	binding := openAICompatSessionResponseBinding{
		TurnState: state,
		ExpiresAt: time.Now().Add(s.openAIWSResponseStickyTTL()),
REDACTED
	if raw, ok := s.openaiCompatSessionResponses.Load(key); ok {
		if existing, ok := raw.(openAICompatSessionResponseBinding); ok {
			binding.ResponseID = existing.ResponseID
			binding.ContinuationDisabled = existing.ContinuationDisabled
	REDACTED
REDACTED
	s.openaiCompatSessionResponses.Store(key, binding)
REDACTED
