package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var errOpenAIWSSessionPreempted = errors.New("openai ws session preempted by newer request")

const (
	openAIWSSessionPreemptOwnerTTL      = 2 * time.Hour
	openAIWSSessionPreemptWatchInterval = 2 * time.Second
	openAIWSSessionPreemptCachePrefix   = "wspreempt:"
)

// OpenAIWSSessionPreemptionCache is an optional GatewayCache capability. The
// production Redis cache implements all operations atomically; cache stubs do
// not need to implement it for ordinary gateway tests.
type OpenAIWSSessionPreemptionCache interface {
	ClaimOpenAIResponsesSessionWindow(ctx context.Context, groupID int64, sessionHash string, owner []byte, ttl time.Duration) ([]byte, error)
	CompareAndRefreshOpenAIResponsesSessionWindow(ctx context.Context, groupID int64, sessionHash string, expected []byte, ttl time.Duration) (bool, error)
	CompareAndDeleteOpenAIResponsesSessionWindow(ctx context.Context, groupID int64, sessionHash string, expected []byte) (bool, error)
REDACTED

func NewOpenAIWSSessionPreemptedError() error {
	return errOpenAIWSSessionPreempted
REDACTED

type openAIWSSessionPreemptKey struct {
	groupID     int64
	apiKeyID    int64
	sessionHash string
REDACTED

type openAIWSSessionPreemptContextKey struct{REDACTED

// BeginOpenAIWSIngressSessionPreemption keeps a persistent inbound WS session
// registered across upstream retry attempts. Nested forwarding calls reuse the
// registration so returning from one attempt cannot create a preemption gap.
func (s *OpenAIGatewayService) BeginOpenAIWSIngressSessionPreemption(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	firstClientMessage []byte,
) (context.Context, func(), bool) {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if armed, _ := ctx.Value(openAIWSSessionPreemptContextKey{REDACTED).(bool); armed {
		return ctx, func() {REDACTED, true
REDACTED

	preemptSessionHash := ""
	preemptGroupID := getOpenAIGroupIDFromContext(c)
	if account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth {
		preemptSessionHash = s.GenerateSessionHash(c, firstClientMessage)
REDACTED
	preemptCtx, cleanup, armed, preemptedPrevious := s.beginOpenAIWSSessionPreemptContext(
		ctx,
		account,
		preemptGroupID,
		getAPIKeyIDFromContext(c),
		preemptSessionHash,
		false,
	)
	if !armed {
		return ctx, func() {REDACTED, false
REDACTED
	if preemptedPrevious {
		if stateStore := s.getOpenAIWSStateStore(); stateStore != nil {
			stateStore.DeleteSessionTurnState(preemptGroupID, preemptSessionHash)
			stateStore.DeleteSessionConn(preemptGroupID, preemptSessionHash)
	REDACTED
REDACTED
	return context.WithValue(preemptCtx, openAIWSSessionPreemptContextKey{REDACTED, true), cleanup, true
REDACTED

func newOpenAIWSSessionPreemptKey(groupID, apiKeyID int64, sessionHash string) (openAIWSSessionPreemptKey, bool) {
	sessionHash = strings.TrimSpace(sessionHash)
	if groupID <= 0 || apiKeyID <= 0 || sessionHash == "" {
		return openAIWSSessionPreemptKey{REDACTED, false
REDACTED
	return openAIWSSessionPreemptKey{groupID: groupID, apiKeyID: apiKeyID, sessionHash: sessionHashREDACTED, true
REDACTED

func openAIWSSessionPreemptCacheHash(apiKeyID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", openAIWSSessionPreemptCachePrefix, apiKeyID, strings.TrimSpace(sessionHash))
REDACTED

type openAIWSSessionPreemptEntry struct {
	generation uint64
	cancel     func()
REDACTED

type openAIWSSessionPreemptRegistry struct {
	mu     sync.Mutex
	next   uint64
	active map[openAIWSSessionPreemptKey]openAIWSSessionPreemptEntry
REDACTED

func (r *openAIWSSessionPreemptRegistry) Begin(key openAIWSSessionPreemptKey, cancel func()) (cleanup func(), preemptedPrevious bool) {
	if r == nil || strings.TrimSpace(key.sessionHash) == "" {
		return func() {REDACTED, false
REDACTED
	r.mu.Lock()
	if r.active == nil {
		r.active = make(map[openAIWSSessionPreemptKey]openAIWSSessionPreemptEntry)
REDACTED
	r.next++
	generation := r.next
	previous, hadPrevious := r.active[key]
	r.active[key] = openAIWSSessionPreemptEntry{generation: generation, cancel: cancelREDACTED
	r.mu.Unlock()
	if hadPrevious && previous.cancel != nil {
		previous.cancel()
REDACTED
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		current, ok := r.active[key]
		if ok && current.generation == generation {
			delete(r.active, key)
	REDACTED
REDACTED, hadPrevious
REDACTED

func (s *OpenAIGatewayService) beginOpenAIWSSessionPreemptContext(
	ctx context.Context,
	account *Account,
	groupID, apiKeyID int64,
	sessionHash string,
	httpIngressWSOneShot bool,
) (context.Context, func(), bool, bool) {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if s == nil || account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || httpIngressWSOneShot {
		return ctx, func() {REDACTED, false, false
REDACTED
	key, ok := newOpenAIWSSessionPreemptKey(groupID, apiKeyID, sessionHash)
	if !ok {
		return ctx, func() {REDACTED, false, false
REDACTED

	preemptCtx, cancel := context.WithCancelCause(ctx)
	ownerToken := uuid.NewString()
	var preemptOnce sync.Once
	preempt := func() {
		preemptOnce.Do(func() {
			if stateStore := s.getOpenAIWSStateStore(); stateStore != nil {
				stateStore.DeleteSessionTurnState(key.groupID, key.sessionHash)
				stateStore.DeleteSessionConn(key.groupID, key.sessionHash)
		REDACTED
			cancel(errOpenAIWSSessionPreempted)
	REDACTED)
REDACTED
	previousRemoteOwner, remoteClaimed := s.claimOpenAIWSSessionPreemptOwner(ctx, key, ownerToken)
	preemptedPrevious := remoteClaimed && previousRemoteOwner != "" && previousRemoteOwner != ownerToken
	cleanupLocal, hadLocalPrevious := s.openaiWSSessionPreemptions.Begin(key, preempt)
	preemptedPrevious = preemptedPrevious || hadLocalPrevious
	stopWatch := func() {REDACTED
	if remoteClaimed {
		stopWatch = s.watchOpenAIWSSessionPreemptOwner(preemptCtx, key, ownerToken, preempt)
REDACTED

	return preemptCtx, func() {
		stopWatch()
		cleanupLocal()
		if remoteClaimed {
			s.releaseOpenAIWSSessionPreemptOwner(context.Background(), key, ownerToken)
	REDACTED
		cancel(nil)
REDACTED, true, preemptedPrevious
REDACTED

func (s *OpenAIGatewayService) openAIWSSessionPreemptionCache() OpenAIWSSessionPreemptionCache {
	if s == nil || s.cache == nil {
		return nil
REDACTED
	cache, _ := s.cache.(OpenAIWSSessionPreemptionCache)
	return cache
REDACTED

func (s *OpenAIGatewayService) claimOpenAIWSSessionPreemptOwner(ctx context.Context, key openAIWSSessionPreemptKey, ownerToken string) (string, bool) {
	cache := s.openAIWSSessionPreemptionCache()
	if cache == nil || strings.TrimSpace(ownerToken) == "" {
		return "", false
REDACTED
	cacheCtx, cancel := context.WithTimeout(ctx, openAIWSStateStoreRedisTimeout)
	defer cancel()
	previous, err := cache.ClaimOpenAIResponsesSessionWindow(
		cacheCtx,
		key.groupID,
		openAIWSSessionPreemptCacheHash(key.apiKeyID, key.sessionHash),
		[]byte(strings.TrimSpace(ownerToken)),
		openAIWSSessionPreemptOwnerTTL,
	)
	if err != nil {
		return "", false
REDACTED
	return strings.TrimSpace(string(previous)), true
REDACTED

func (s *OpenAIGatewayService) releaseOpenAIWSSessionPreemptOwner(ctx context.Context, key openAIWSSessionPreemptKey, ownerToken string) {
	cache := s.openAIWSSessionPreemptionCache()
	if cache == nil || strings.TrimSpace(ownerToken) == "" {
		return
REDACTED
	cacheCtx, cancel := context.WithTimeout(ctx, openAIWSStateStoreRedisTimeout)
	defer cancel()
	_, _ = cache.CompareAndDeleteOpenAIResponsesSessionWindow(
		cacheCtx,
		key.groupID,
		openAIWSSessionPreemptCacheHash(key.apiKeyID, key.sessionHash),
		[]byte(strings.TrimSpace(ownerToken)),
	)
REDACTED

func (s *OpenAIGatewayService) watchOpenAIWSSessionPreemptOwner(ctx context.Context, key openAIWSSessionPreemptKey, ownerToken string, onLost func()) func() {
	cache := s.openAIWSSessionPreemptionCache()
	if cache == nil || onLost == nil || strings.TrimSpace(ownerToken) == "" {
		return func() {REDACTED
REDACTED
	stopCh := make(chan struct{REDACTED)
	var once sync.Once
	go func() {
		ticker := time.NewTicker(openAIWSSessionPreemptWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				cacheCtx, cancel := context.WithTimeout(context.Background(), openAIWSStateStoreRedisTimeout)
				owned, err := cache.CompareAndRefreshOpenAIResponsesSessionWindow(
					cacheCtx,
					key.groupID,
					openAIWSSessionPreemptCacheHash(key.apiKeyID, key.sessionHash),
					[]byte(strings.TrimSpace(ownerToken)),
					openAIWSSessionPreemptOwnerTTL,
				)
				cancel()
				if err == nil && !owned {
					onLost()
					return
			REDACTED
		REDACTED
	REDACTED
REDACTED()
	return func() { once.Do(func() { close(stopCh) REDACTED) REDACTED
REDACTED

func isOpenAIWSSessionPreempted(ctx context.Context) bool {
	return ctx != nil && errors.Is(context.Cause(ctx), errOpenAIWSSessionPreempted)
REDACTED

func IsOpenAIWSSessionPreemptedError(err error) bool {
	if err == nil {
		return false
REDACTED
	if errors.Is(err, errOpenAIWSSessionPreempted) {
		return true
REDACTED
	var fallbackErr *openAIWSFallbackError
	return errors.As(err, &fallbackErr) && fallbackErr != nil && strings.TrimPrefix(strings.TrimSpace(fallbackErr.Reason), "prewarm_") == "session_preempted"
REDACTED
