package service

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const maxOpenAIResponsesRejectedFieldRetries = 6

var (
	openAIResponsesRejectedNamespaceParamPattern = regexp.MustCompile(`(?i)^input\[(\d+)\]\.namespace$`)
	openAIResponsesRejectedMessageParamPattern   = regexp.MustCompile(`(?i)(?:unknown|unsupported)[ _-]+parameter\s*(?::|=|is)?\s*["']?(max_output_tokens|input\[\d+\]\.namespace)(?:["']|\b)`)
)

type openAIResponsesRejectedFieldRetryState struct {
	attempts       int
	seenBodyHashes map[[sha256.Size]byte]struct{REDACTED
REDACTED

func newOpenAIResponsesRejectedFieldRetryState(initialBody []byte) *openAIResponsesRejectedFieldRetryState {
	state := &openAIResponsesRejectedFieldRetryState{
		seenBodyHashes: make(map[[sha256.Size]byte]struct{REDACTED, maxOpenAIResponsesRejectedFieldRetries+1),
REDACTED
	state.remember(initialBody)
	return state
REDACTED

func (s *openAIResponsesRejectedFieldRetryState) Allow(nextBody []byte) bool {
	if s == nil || len(nextBody) == 0 || s.attempts >= maxOpenAIResponsesRejectedFieldRetries {
		return false
REDACTED
	bodyHash := sha256.Sum256(nextBody)
	if _, seen := s.seenBodyHashes[bodyHash]; seen {
		return false
REDACTED
	s.seenBodyHashes[bodyHash] = struct{REDACTED{REDACTED
	s.attempts++
	return true
REDACTED

func (s *openAIResponsesRejectedFieldRetryState) remember(body []byte) {
	if s == nil || len(body) == 0 {
		return
REDACTED
	if s.seenBodyHashes == nil {
		s.seenBodyHashes = make(map[[sha256.Size]byte]struct{REDACTED, maxOpenAIResponsesRejectedFieldRetries+1)
REDACTED
	s.seenBodyHashes[sha256.Sum256(body)] = struct{REDACTED{REDACTED
REDACTED

func normalizeOpenAIResponsesRejectedFieldRetryBody(statusCode int, body, responseBody []byte) ([]byte, string, bool, error) {
	if statusCode != http.StatusBadRequest || len(body) == 0 || len(responseBody) == 0 {
		return nil, "", false, nil
REDACTED

	code := strings.ToLower(strings.TrimSpace(extractUpstreamErrorCode(responseBody)))
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
	if !isExplicitOpenAIResponsesFieldRejection(code, message) {
		return nil, "", false, nil
REDACTED

	param := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.param").String()))
	if param == "" {
		param = openAIResponsesRejectedParamFromMessage(message)
REDACTED
	if index, ok := openAIResponsesRejectedNamespaceIndex(param); ok {
		return removeOpenAIResponsesRejectedNamespaceAtIndex(body, index)
REDACTED
	if param == "max_output_tokens" && gjson.GetBytes(body, "max_output_tokens").Exists() {
		retryBody, err := sjson.DeleteBytes(body, "max_output_tokens")
		if err != nil {
			return nil, "", false, fmt.Errorf("delete rejected max_output_tokens: %w", err)
	REDACTED
		return retryBody, "max_output_tokens parameter rejection", true, nil
REDACTED
	return nil, "", false, nil
REDACTED

func isExplicitOpenAIResponsesFieldRejection(code, message string) bool {
	switch strings.TrimSpace(code) {
	case "unknown_parameter", "unsupported_parameter":
		return true
REDACTED
	return strings.Contains(message, "unknown parameter") ||
		strings.Contains(message, "unsupported parameter")
REDACTED

func openAIResponsesRejectedParamFromMessage(message string) string {
	match := openAIResponsesRejectedMessageParamPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 2 {
		return ""
REDACTED
	return strings.ToLower(strings.TrimSpace(match[1]))
REDACTED

func openAIResponsesRejectedNamespaceIndex(param string) (int, bool) {
	match := openAIResponsesRejectedNamespaceParamPattern.FindStringSubmatch(strings.TrimSpace(param))
	if len(match) != 2 {
		return 0, false
REDACTED
	index, err := strconv.Atoi(match[1])
	if err == nil && index >= 0 {
		return index, true
REDACTED
	return 0, false
REDACTED

func removeOpenAIResponsesRejectedNamespaceAtIndex(body []byte, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("input.%d", index)
	itemType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, itemPath+".type").String()))
	switch itemType {
	case "function_call", "tool_call", "custom_tool_call", "mcp_tool_call":
	default:
		return nil, "", false, nil
REDACTED

	namespacePath := itemPath + ".namespace"
	if !gjson.GetBytes(body, namespacePath).Exists() {
		return nil, "", false, nil
REDACTED
	retryBody, err := sjson.DeleteBytes(body, namespacePath)
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected namespace at input[%d]: %w", index, err)
REDACTED
	return retryBody, "indexed namespace parameter rejection", true, nil
REDACTED
