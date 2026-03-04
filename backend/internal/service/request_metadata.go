package service

import (
	"context"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type requestMetadataContextKey struct{REDACTED

var requestMetadataKey = requestMetadataContextKey{REDACTED

type RequestMetadata struct {
	IsMaxTokensOneHaikuRequest *bool
	ThinkingEnabled            *bool
	PrefetchedStickyAccountID  *int64
	PrefetchedStickyGroupID    *int64
	SingleAccountRetry         *bool
	AccountSwitchCount         *int
REDACTED

var (
	requestMetadataFallbackIsMaxTokensOneHaikuTotal atomic.Int64
	requestMetadataFallbackThinkingEnabledTotal     atomic.Int64
	requestMetadataFallbackPrefetchedStickyAccount  atomic.Int64
	requestMetadataFallbackPrefetchedStickyGroup    atomic.Int64
	requestMetadataFallbackSingleAccountRetryTotal  atomic.Int64
	requestMetadataFallbackAccountSwitchCountTotal  atomic.Int64
)

func RequestMetadataFallbackStats() (isMaxTokensOneHaiku, thinkingEnabled, prefetchedStickyAccount, prefetchedStickyGroup, singleAccountRetry, accountSwitchCount int64) {
	return requestMetadataFallbackIsMaxTokensOneHaikuTotal.Load(),
		requestMetadataFallbackThinkingEnabledTotal.Load(),
		requestMetadataFallbackPrefetchedStickyAccount.Load(),
		requestMetadataFallbackPrefetchedStickyGroup.Load(),
		requestMetadataFallbackSingleAccountRetryTotal.Load(),
		requestMetadataFallbackAccountSwitchCountTotal.Load()
REDACTED

func metadataFromContext(ctx context.Context) *RequestMetadata {
	if ctx == nil {
		return nil
REDACTED
	md, _ := ctx.Value(requestMetadataKey).(*RequestMetadata)
	return md
REDACTED

func updateRequestMetadata(
	ctx context.Context,
	bridgeOldKeys bool,
	update func(md *RequestMetadata),
	legacyBridge func(ctx context.Context) context.Context,
) context.Context {
	if ctx == nil {
		return nil
REDACTED
	current := metadataFromContext(ctx)
	next := &RequestMetadata{REDACTED
	if current != nil {
		*next = *current
REDACTED
	update(next)
	ctx = context.WithValue(ctx, requestMetadataKey, next)
	if bridgeOldKeys && legacyBridge != nil {
		ctx = legacyBridge(ctx)
REDACTED
	return ctx
REDACTED

func WithIsMaxTokensOneHaikuRequest(ctx context.Context, value bool, bridgeOldKeys bool) context.Context {
	return updateRequestMetadata(ctx, bridgeOldKeys, func(md *RequestMetadata) {
		v := value
		md.IsMaxTokensOneHaikuRequest = &v
REDACTED, func(base context.Context) context.Context {
		return context.WithValue(base, ctxkey.IsMaxTokensOneHaikuRequest, value)
REDACTED)
REDACTED

func WithThinkingEnabled(ctx context.Context, value bool, bridgeOldKeys bool) context.Context {
	return updateRequestMetadata(ctx, bridgeOldKeys, func(md *RequestMetadata) {
		v := value
		md.ThinkingEnabled = &v
REDACTED, func(base context.Context) context.Context {
		return context.WithValue(base, ctxkey.ThinkingEnabled, value)
REDACTED)
REDACTED

func WithPrefetchedStickySession(ctx context.Context, accountID, groupID int64, bridgeOldKeys bool) context.Context {
	return updateRequestMetadata(ctx, bridgeOldKeys, func(md *RequestMetadata) {
		account := accountID
		group := groupID
		md.PrefetchedStickyAccountID = &account
		md.PrefetchedStickyGroupID = &group
REDACTED, func(base context.Context) context.Context {
		bridged := context.WithValue(base, ctxkey.PrefetchedStickyAccountID, accountID)
		return context.WithValue(bridged, ctxkey.PrefetchedStickyGroupID, groupID)
REDACTED)
REDACTED

func WithSingleAccountRetry(ctx context.Context, value bool, bridgeOldKeys bool) context.Context {
	return updateRequestMetadata(ctx, bridgeOldKeys, func(md *RequestMetadata) {
		v := value
		md.SingleAccountRetry = &v
REDACTED, func(base context.Context) context.Context {
		return context.WithValue(base, ctxkey.SingleAccountRetry, value)
REDACTED)
REDACTED

func WithAccountSwitchCount(ctx context.Context, value int, bridgeOldKeys bool) context.Context {
	return updateRequestMetadata(ctx, bridgeOldKeys, func(md *RequestMetadata) {
		v := value
		md.AccountSwitchCount = &v
REDACTED, func(base context.Context) context.Context {
		return context.WithValue(base, ctxkey.AccountSwitchCount, value)
REDACTED)
REDACTED

func IsMaxTokensOneHaikuRequestFromContext(ctx context.Context) (bool, bool) {
	if md := metadataFromContext(ctx); md != nil && md.IsMaxTokensOneHaikuRequest != nil {
		return *md.IsMaxTokensOneHaikuRequest, true
REDACTED
	if ctx == nil {
		return false, false
REDACTED
	if value, ok := ctx.Value(ctxkey.IsMaxTokensOneHaikuRequest).(bool); ok {
		requestMetadataFallbackIsMaxTokensOneHaikuTotal.Add(1)
		return value, true
REDACTED
	return false, false
REDACTED

func ThinkingEnabledFromContext(ctx context.Context) (bool, bool) {
	if md := metadataFromContext(ctx); md != nil && md.ThinkingEnabled != nil {
		return *md.ThinkingEnabled, true
REDACTED
	if ctx == nil {
		return false, false
REDACTED
	if value, ok := ctx.Value(ctxkey.ThinkingEnabled).(bool); ok {
		requestMetadataFallbackThinkingEnabledTotal.Add(1)
		return value, true
REDACTED
	return false, false
REDACTED

func PrefetchedStickyGroupIDFromContext(ctx context.Context) (int64, bool) {
	if md := metadataFromContext(ctx); md != nil && md.PrefetchedStickyGroupID != nil {
		return *md.PrefetchedStickyGroupID, true
REDACTED
	if ctx == nil {
		return 0, false
REDACTED
	v := ctx.Value(ctxkey.PrefetchedStickyGroupID)
	switch t := v.(type) {
	case int64:
		requestMetadataFallbackPrefetchedStickyGroup.Add(1)
		return t, true
	case int:
		requestMetadataFallbackPrefetchedStickyGroup.Add(1)
		return int64(t), true
REDACTED
	return 0, false
REDACTED

func PrefetchedStickyAccountIDFromContext(ctx context.Context) (int64, bool) {
	if md := metadataFromContext(ctx); md != nil && md.PrefetchedStickyAccountID != nil {
		return *md.PrefetchedStickyAccountID, true
REDACTED
	if ctx == nil {
		return 0, false
REDACTED
	v := ctx.Value(ctxkey.PrefetchedStickyAccountID)
	switch t := v.(type) {
	case int64:
		requestMetadataFallbackPrefetchedStickyAccount.Add(1)
		return t, true
	case int:
		requestMetadataFallbackPrefetchedStickyAccount.Add(1)
		return int64(t), true
REDACTED
	return 0, false
REDACTED

func SingleAccountRetryFromContext(ctx context.Context) (bool, bool) {
	if md := metadataFromContext(ctx); md != nil && md.SingleAccountRetry != nil {
		return *md.SingleAccountRetry, true
REDACTED
	if ctx == nil {
		return false, false
REDACTED
	if value, ok := ctx.Value(ctxkey.SingleAccountRetry).(bool); ok {
		requestMetadataFallbackSingleAccountRetryTotal.Add(1)
		return value, true
REDACTED
	return false, false
REDACTED

func AccountSwitchCountFromContext(ctx context.Context) (int, bool) {
	if md := metadataFromContext(ctx); md != nil && md.AccountSwitchCount != nil {
		return *md.AccountSwitchCount, true
REDACTED
	if ctx == nil {
		return 0, false
REDACTED
	v := ctx.Value(ctxkey.AccountSwitchCount)
	switch t := v.(type) {
	case int:
		requestMetadataFallbackAccountSwitchCountTotal.Add(1)
		return t, true
	case int64:
		requestMetadataFallbackAccountSwitchCountTotal.Add(1)
		return int(t), true
REDACTED
	return 0, false
REDACTED
