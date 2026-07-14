package service

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

const (
	OpenAIAuthModeAgentIdentity          = "agentIdentity"
	agentIdentityAuthAPIBaseURL          = "https://auth.openai.com/api/accounts"
	agentIdentityTaskRegistrationTimeout = 30 * time.Second
)

var openAIAgentIdentityAuthAPIBaseURL = agentIdentityAuthAPIBaseURL

var agentIdentityTaskLocks sync.Map // map[int64]*sync.Mutex

type agentIdentityKey struct {
	runtimeID  string
	privateKey ed25519.PrivateKey
	taskID     string
REDACTED

type agentIdentityTaskRegistrationResponse struct {
	TaskID               string `json:"task_id"`
	TaskIDCamel          string `json:"taskId"`
	EncryptedTaskID      string `json:"encrypted_task_id"`
	EncryptedTaskIDCamel string `json:"encryptedTaskId"`
REDACTED

type agentIdentityTaskRecoveredError struct{REDACTED

func (e *agentIdentityTaskRecoveredError) Error() string {
	return "agent identity task recovered"
REDACTED

func (a *Account) IsOpenAIAgentIdentity() bool {
	if a == nil || !a.IsOpenAIOAuth() {
		return false
REDACTED
	return strings.EqualFold(strings.TrimSpace(a.GetCredential(openAIAuthModeCredentialKey)), OpenAIAuthModeAgentIdentity)
REDACTED

func agentIdentityPrivateKey(account *Account) (ed25519.PrivateKey, error) {
	if account == nil {
		return nil, errors.New("agent identity account is nil")
REDACTED
	raw := strings.TrimSpace(account.GetCredential("agent_private_key"))
	if raw == "" {
		return nil, errors.New("agent identity private key is missing")
REDACTED
	der, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("agent identity private key is not valid base64")
REDACTED
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, errors.New("agent identity private key is not valid PKCS#8")
REDACTED
	privateKey, ok := key.(ed25519.PrivateKey)
	if !ok || len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("agent identity private key is not Ed25519")
REDACTED
	return privateKey, nil
REDACTED

// ValidateOpenAIAgentIdentityPrivateKey validates the stored PKCS#8 Ed25519
// form without returning or logging the key material.
func ValidateOpenAIAgentIdentityPrivateKey(encoded string) error {
	account := &Account{Credentials: map[string]any{"agent_private_key": encodedREDACTEDREDACTED
	_, err := agentIdentityPrivateKey(account)
	return err
REDACTED

func agentIdentityKeyFromAccount(account *Account) (agentIdentityKey, error) {
	privateKey, err := agentIdentityPrivateKey(account)
	if err != nil {
		return agentIdentityKey{REDACTED, err
REDACTED
	runtimeID := strings.TrimSpace(account.GetCredential("agent_runtime_id"))
	if runtimeID == "" {
		return agentIdentityKey{REDACTED, errors.New("agent identity runtime id is missing")
REDACTED
	return agentIdentityKey{
		runtimeID:  runtimeID,
		privateKey: privateKey,
		taskID:     strings.TrimSpace(account.GetCredential("task_id")),
REDACTED, nil
REDACTED

func buildAgentAssertion(key agentIdentityKey, now time.Time) (string, error) {
	if key.runtimeID == "" || key.taskID == "" {
		return "", errors.New("agent identity runtime or task id is missing")
REDACTED
	timestamp := now.UTC().Format(time.RFC3339)
	payload := []byte(key.runtimeID + ":" + key.taskID + ":" + timestamp)
	signature, err := key.privateKey.Sign(nil, payload, crypto.Hash(0))
	if err != nil {
		return "", errors.New("failed to sign agent assertion")
REDACTED
	envelope := map[string]string{
		"agent_runtime_id": key.runtimeID,
		"task_id":          key.taskID,
		"timestamp":        timestamp,
		"signature":        base64.StdEncoding.EncodeToString(signature),
REDACTED
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", errors.New("failed to serialize agent assertion")
REDACTED
	return "AgentAssertion " + base64.RawURLEncoding.EncodeToString(encoded), nil
REDACTED

func signAgentTaskRegistration(key agentIdentityKey, timestamp time.Time) (string, string, error) {
	if key.runtimeID == "" {
		return "", "", errors.New("agent identity runtime id is missing")
REDACTED
	formatted := timestamp.UTC().Format(time.RFC3339)
	signature, err := key.privateKey.Sign(nil, []byte(key.runtimeID+":"+formatted), crypto.Hash(0))
	if err != nil {
		return "", "", errors.New("failed to sign agent task registration")
REDACTED
	return formatted, base64.StdEncoding.EncodeToString(signature), nil
REDACTED

func decryptAgentTaskID(key agentIdentityKey, encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", errors.New("encrypted agent task id is not valid base64")
REDACTED
	seed := key.privateKey.Seed()
	digest := sha512.Sum512(seed)
	var curvePrivate [32]byte
	copy(curvePrivate[:], digest[:32])
	curvePrivate[0] &= 248
	curvePrivate[31] &= 127
	curvePrivate[31] |= 64
	curvePublicBytes, err := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)
	if err != nil {
		return "", errors.New("failed to derive agent identity decryption key")
REDACTED
	var curvePublic [32]byte
	copy(curvePublic[:], curvePublicBytes)
	plaintext, ok := box.OpenAnonymous(nil, ciphertext, &curvePublic, &curvePrivate)
	if !ok {
		return "", errors.New("failed to decrypt encrypted agent task id")
REDACTED
	taskID := strings.TrimSpace(string(plaintext))
	if taskID == "" {
		return "", errors.New("decrypted agent task id is empty")
REDACTED
	return taskID, nil
REDACTED

func registerAgentIdentityTask(ctx context.Context, account *Account) (string, error) {
	key, err := agentIdentityKeyFromAccount(account)
	if err != nil {
		return "", err
REDACTED
	timestamp, signature, err := signAgentTaskRegistration(key, time.Now())
	if err != nil {
		return "", err
REDACTED
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxyURL,
		Timeout:               agentIdentityTaskRegistrationTimeout,
		ResponseHeaderTimeout: 15 * time.Second,
REDACTED)
	if err != nil {
		return "", errors.New("invalid proxy configuration for agent task registration")
REDACTED
	body, err := json.Marshal(map[string]string{
		"timestamp": timestamp,
		"signature": signature,
REDACTED)
	if err != nil {
		return "", errors.New("failed to serialize agent task registration")
REDACTED
	url := strings.TrimRight(strings.TrimSpace(openAIAgentIdentityAuthAPIBaseURL), "/") + "/v1/agent/" + key.runtimeID + "/task/register"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return "", errors.New("failed to build agent task registration request")
REDACTED
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("agent task registration request failed")
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("agent task registration returned status %d", resp.StatusCode)
REDACTED
	var result agentIdentityTaskRegistrationResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&result); err != nil {
		return "", errors.New("agent task registration response is invalid")
REDACTED
	if taskID := strings.TrimSpace(result.TaskID); taskID != "" {
		return taskID, nil
REDACTED
	if taskID := strings.TrimSpace(result.TaskIDCamel); taskID != "" {
		return taskID, nil
REDACTED
	encrypted := strings.TrimSpace(result.EncryptedTaskID)
	if encrypted == "" {
		encrypted = strings.TrimSpace(result.EncryptedTaskIDCamel)
REDACTED
	if encrypted == "" {
		return "", errors.New("agent task registration response omitted task id")
REDACTED
	return decryptAgentTaskID(key, encrypted)
REDACTED

func ensureAgentIdentityTaskForAccount(ctx context.Context, repo AccountRepository, pool *openAIWSConnPool, taskMu *sync.Mutex, account *Account, expectedTaskID string) error {
	if account == nil || !account.IsOpenAIAgentIdentity() {
		return nil
REDACTED
	credAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, repo, account)
		if err != nil {
			return err
	REDACTED
		credAccount = resolved
REDACTED
	if credAccount == nil || !credAccount.IsOpenAIAgentIdentity() {
		return errors.New("agent identity credentials are unavailable")
REDACTED
	currentTaskID := strings.TrimSpace(credAccount.GetCredential("task_id"))
	if currentTaskID != "" && (expectedTaskID == "" || currentTaskID != expectedTaskID) {
		return nil
REDACTED
	if taskMu == nil {
		return errors.New("agent identity task lock is unavailable")
REDACTED
	sharedTaskMu := taskMu
	if credAccount.ID > 0 {
		candidate := &sync.Mutex{REDACTED
		actual, _ := agentIdentityTaskLocks.LoadOrStore(credAccount.ID, candidate)
		sharedTaskMu = actual.(*sync.Mutex)
REDACTED
	sharedTaskMu.Lock()
	defer sharedTaskMu.Unlock()
	// Re-read inside the shared lock. Different request paths often receive
	// independent repository snapshots; checking only the caller's snapshot
	// would allow sequential duplicate registrations after the first writer
	// has already persisted a new task.
	if repo != nil && credAccount.ID > 0 {
		if refreshed, refreshErr := repo.GetByID(ctx, credAccount.ID); refreshErr == nil && refreshed != nil {
			if refreshed.IsShadow() {
				if resolved, resolveErr := resolveCredentialAccount(ctx, repo, refreshed); resolveErr == nil && resolved != nil {
					refreshed = resolved
			REDACTED
		REDACTED
			if refreshed.IsOpenAIAgentIdentity() {
				credAccount = refreshed
				if !account.IsShadow() {
					account.Credentials = shallowCopyMap(credAccount.Credentials)
			REDACTED
		REDACTED
	REDACTED
REDACTED
	currentTaskID = strings.TrimSpace(credAccount.GetCredential("task_id"))
	if currentTaskID != "" && (expectedTaskID == "" || currentTaskID != expectedTaskID) {
		return nil
REDACTED
	newTaskID, err := registerAgentIdentityTask(ctx, credAccount)
	if err != nil {
		return err
REDACTED
	credentials := make(map[string]any, len(credAccount.Credentials)+1)
	for key, value := range credAccount.Credentials {
		credentials[key] = value
REDACTED
	credentials["task_id"] = newTaskID
	if err := persistAccountCredentials(ctx, repo, credAccount, credentials); err != nil {
		return err
REDACTED
	if !account.IsShadow() && account != credAccount {
		account.Credentials = shallowCopyMap(credAccount.Credentials)
REDACTED
	if pool != nil {
		pool.ClearAccount(credAccount.ID)
REDACTED
	return nil
REDACTED

func (s *OpenAIGatewayService) ensureAgentIdentityTask(ctx context.Context, account *Account, expectedTaskID string) error {
	if s == nil {
		return errors.New("openai gateway service is nil")
REDACTED
	return ensureAgentIdentityTaskForAccount(ctx, s.accountRepo, s.openaiWSPool, &s.agentIdentityTaskMu, account, expectedTaskID)
REDACTED

func isAgentIdentityTaskInvalidHTTPResponse(statusCode int, body []byte) bool {
	if statusCode != http.StatusUnauthorized {
		return false
REDACTED
	lower := strings.ToLower(string(body))
	for _, marker := range []string{
		"invalid task",
		"task_id",
		"task id",
		"task_not_found",
		"task_expired",
		"unknown task",
REDACTED {
		if strings.Contains(lower, marker) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func isAgentIdentityTaskInvalidWSDialError(err *openAIWSDialError) bool {
	return err != nil && isAgentIdentityTaskInvalidHTTPResponse(err.StatusCode, err.ResponseBody)
REDACTED

func (s *OpenAIGatewayService) buildOpenAIAuthenticationHeaders(ctx context.Context, account *Account, token string) (http.Header, error) {
	if account == nil {
		return nil, errors.New("account is nil")
REDACTED
	credAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return nil, err
	REDACTED
		credAccount = resolved
REDACTED
	headers := make(http.Header)
	if credAccount != nil && credAccount.IsOpenAIAgentIdentity() {
		agentHeaders, err := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.openaiWSPool, &s.agentIdentityTaskMu, credAccount)
		if err != nil {
			return nil, err
	REDACTED
		return agentHeaders, nil
REDACTED
	headers.Set("Authorization", "Bearer "+token)
	return headers, nil
REDACTED

func buildAgentIdentityAuthenticationHeaders(ctx context.Context, repo AccountRepository, pool *openAIWSConnPool, taskMu *sync.Mutex, account *Account) (http.Header, error) {
	if account == nil || !account.IsOpenAIAgentIdentity() {
		return nil, errors.New("agent identity account is required")
REDACTED
	if err := ensureAgentIdentityTaskForAccount(ctx, repo, pool, taskMu, account, ""); err != nil {
		return nil, err
REDACTED
	key, err := agentIdentityKeyFromAccount(account)
	if err != nil {
		return nil, err
REDACTED
	assertion, err := buildAgentAssertion(key, time.Now())
	if err != nil {
		return nil, err
REDACTED
	headers := make(http.Header)
	headers.Set("Authorization", assertion)
	return headers, nil
REDACTED

func (s *OpenAIGatewayService) refreshOpenAIAgentIdentityHeaders(ctx context.Context, account *Account, headers http.Header) (http.Header, error) {
	if account == nil {
		return cloneHeader(headers), nil
REDACTED
	credAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return nil, err
	REDACTED
		credAccount = resolved
REDACTED
	if !credAccount.IsOpenAIAgentIdentity() {
		return cloneHeader(headers), nil
REDACTED
	refreshed := cloneHeader(headers)
	if refreshed == nil {
		refreshed = make(http.Header)
REDACTED
	authHeaders, err := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.openaiWSPool, &s.agentIdentityTaskMu, credAccount)
	if err != nil {
		return nil, err
REDACTED
	refreshed.Set("Authorization", authHeaders.Get("Authorization"))
	return refreshed, nil
REDACTED

func (s *OpenAIGatewayService) recoverAgentIdentityTask(ctx context.Context, account *Account, expectedTaskID string) error {
	if account != nil && account.IsShadow() {
		if resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account); err == nil && resolved != nil && strings.TrimSpace(expectedTaskID) == "" {
			expectedTaskID = strings.TrimSpace(resolved.GetCredential("task_id"))
	REDACTED
REDACTED
	return s.ensureAgentIdentityTask(ctx, account, expectedTaskID)
REDACTED

func (s *OpenAIGatewayService) isAgentIdentityAccount(ctx context.Context, account *Account) bool {
	if account == nil {
		return false
REDACTED
	credAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return false
	REDACTED
		credAccount = resolved
REDACTED
	return credAccount != nil && credAccount.IsOpenAIAgentIdentity()
REDACTED

// redactAgentIdentitySensitiveBody removes credential values before an
// upstream error can reach logs, ops events, or returned error text. Agent
// Identity responses should not echo these values, but keeping this boundary
// defensive prevents accidental disclosure if an upstream error does.
func redactAgentIdentitySensitiveBodyForAccount(ctx context.Context, repo AccountRepository, account *Account, body []byte) []byte {
	if account == nil || len(body) == 0 {
		return body
REDACTED
	credAccount := account
	if account != nil && account.IsShadow() {
		if resolved, err := resolveCredentialAccount(ctx, repo, account); err == nil && resolved != nil {
			credAccount = resolved
	REDACTED
REDACTED
	if credAccount == nil || !credAccount.IsOpenAIAgentIdentity() {
		return body
REDACTED
	redacted := string(body)
	for _, key := range []string{
		"agent_private_key",
		"agent_runtime_id",
		"task_id",
		"access_token",
		"refresh_token",
		"id_token",
		"api_key",
		"session_key",
		"cookie",
REDACTED {
		if value := strings.TrimSpace(credAccount.GetCredential(key)); value != "" {
			redacted = strings.ReplaceAll(redacted, value, "[redacted]")
	REDACTED
REDACTED
	for {
		start := strings.Index(redacted, "AgentAssertion ")
		if start < 0 {
			break
	REDACTED
		end := start + len("AgentAssertion ")
		for end < len(redacted) && !strings.ContainsRune(" \t\r\n\"',REDACTED", rune(redacted[end])) {
			end++
	REDACTED
		redacted = redacted[:start] + "AgentAssertion [redacted]" + redacted[end:]
REDACTED
	return []byte(redacted)
REDACTED

func (s *OpenAIGatewayService) redactAgentIdentitySensitiveBody(ctx context.Context, account *Account, body []byte) []byte {
	if !s.isAgentIdentityAccount(ctx, account) {
		return body
REDACTED
	return redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, account, body)
REDACTED
