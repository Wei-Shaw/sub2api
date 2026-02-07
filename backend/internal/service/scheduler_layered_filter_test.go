//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFilterByMinPriority(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinPriority(nil)
		require.Empty(t, result)
REDACTED)

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 5REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		result := filterByMinPriority(accounts)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
REDACTED)

	t.Run("multiple accounts same priority", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 3REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, Priority: 3REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 3, Priority: 3REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		result := filterByMinPriority(accounts)
		require.Len(t, result, 3)
REDACTED)

	t.Run("filters to min priority only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 5REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, Priority: 1REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 3, Priority: 3REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 4, Priority: 1REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		result := filterByMinPriority(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
REDACTED)
REDACTED

func TestFilterByMinLoadRate(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinLoadRate(nil)
		require.Empty(t, result)
REDACTED)

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 50REDACTEDREDACTED,
	REDACTED
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
REDACTED)

	t.Run("multiple accounts same load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 20REDACTEDREDACTED,
			{account: &Account{ID: 2REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 20REDACTEDREDACTED,
			{account: &Account{ID: 3REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 20REDACTEDREDACTED,
	REDACTED
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 3)
REDACTED)

	t.Run("filters to min load rate only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 80REDACTEDREDACTED,
			{account: &Account{ID: 2REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 10REDACTEDREDACTED,
			{account: &Account{ID: 3REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 50REDACTEDREDACTED,
			{account: &Account{ID: 4REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 10REDACTEDREDACTED,
	REDACTED
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
REDACTED)

	t.Run("zero load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 0REDACTEDREDACTED,
			{account: &Account{ID: 2REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 50REDACTEDREDACTED,
			{account: &Account{ID: 3REDACTED, loadInfo: &AccountLoadInfo{LoadRate: 0REDACTEDREDACTED,
	REDACTED
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(1), result[0].account.ID)
		require.Equal(t, int64(3), result[1].account.ID)
REDACTED)
REDACTED

func TestSelectByLRU(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	t.Run("empty slice", func(t *testing.T) {
		result := selectByLRU(nil, false)
		require.Nil(t, result)
REDACTED)

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &nowREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.account.ID)
REDACTED)

	t.Run("selects least recently used", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &nowREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, LastUsedAt: &muchEarlierREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 3, LastUsedAt: &earlierREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
REDACTED)

	t.Run("nil LastUsedAt preferred over non-nil", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &nowREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, LastUsedAt: nilREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 3, LastUsedAt: &earlierREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
REDACTED)

	t.Run("multiple nil LastUsedAt random selection", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, LastUsedAt: nil, Type: "session"REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 3, LastUsedAt: nil, Type: "session"REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		// 多次调用应该随机选择，验证结果都在候选范围内
		validIDs := map[int64]bool{1: true, 2: true, 3: trueREDACTED
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID], "selected ID should be one of the candidates")
	REDACTED
REDACTED)

	t.Run("multiple same LastUsedAt random selection", func(t *testing.T) {
		sameTime := now
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &sameTimeREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, LastUsedAt: &sameTimeREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		// 多次调用应该随机选择
		validIDs := map[int64]bool{1: true, 2: trueREDACTED
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID], "selected ID should be one of the candidates")
	REDACTED
REDACTED)

	t.Run("preferOAuth selects from OAuth accounts when multiple nil", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, LastUsedAt: nil, Type: AccountTypeOAuthREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 3, LastUsedAt: nil, Type: AccountTypeOAuthREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		// preferOAuth 时，应该从 OAuth 类型中选择
		oauthIDs := map[int64]bool{2: true, 3: trueREDACTED
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, true)
			require.NotNil(t, result)
			require.True(t, oauthIDs[result.account.ID], "should select from OAuth accounts")
	REDACTED
REDACTED)

	t.Run("preferOAuth falls back to all when no OAuth", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, LastUsedAt: nil, Type: "session"REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		// 没有 OAuth 时，从所有候选中选择
		validIDs := map[int64]bool{1: true, 2: trueREDACTED
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, true)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID])
	REDACTED
REDACTED)

	t.Run("preferOAuth only affects same LastUsedAt accounts", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &earlier, Type: "session"REDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
			{account: &Account{ID: 2, LastUsedAt: &now, Type: AccountTypeOAuthREDACTED, loadInfo: &AccountLoadInfo{REDACTEDREDACTED,
	REDACTED
		result := selectByLRU(accounts, true)
		require.NotNil(t, result)
		// 有不同 LastUsedAt 时，按时间选择最早的，不受 preferOAuth 影响
		require.Equal(t, int64(1), result.account.ID)
REDACTED)
REDACTED

func TestLayeredFilterIntegration(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	t.Run("full layered selection", func(t *testing.T) {
		// 模拟真实场景：多个账号，不同优先级、负载率、最后使用时间
		accounts := []accountWithLoad{
			// 优先级 1，负载 50%
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &nowREDACTED, loadInfo: &AccountLoadInfo{LoadRate: 50REDACTEDREDACTED,
			// 优先级 1，负载 20%（最低）
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &earlierREDACTED, loadInfo: &AccountLoadInfo{LoadRate: 20REDACTEDREDACTED,
			// 优先级 1，负载 20%（最低），更早使用
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlierREDACTED, loadInfo: &AccountLoadInfo{LoadRate: 20REDACTEDREDACTED,
			// 优先级 2（较低优先）
			{account: &Account{ID: 4, Priority: 2, LastUsedAt: &muchEarlierREDACTED, loadInfo: &AccountLoadInfo{LoadRate: 0REDACTEDREDACTED,
	REDACTED

		// 1. 取优先级最小的集合 → ID: 1, 2, 3
		step1 := filterByMinPriority(accounts)
		require.Len(t, step1, 3)

		// 2. 取负载率最低的集合 → ID: 2, 3
		step2 := filterByMinLoadRate(step1)
		require.Len(t, step2, 2)

		// 3. LRU 选择 → ID: 3（muchEarlier 最早）
		selected := selectByLRU(step2, false)
		require.NotNil(t, selected)
		require.Equal(t, int64(3), selected.account.ID)
REDACTED)

	t.Run("all same priority and load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &nowREDACTED, loadInfo: &AccountLoadInfo{LoadRate: 50REDACTEDREDACTED,
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &earlierREDACTED, loadInfo: &AccountLoadInfo{LoadRate: 50REDACTEDREDACTED,
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlierREDACTED, loadInfo: &AccountLoadInfo{LoadRate: 50REDACTEDREDACTED,
	REDACTED

		step1 := filterByMinPriority(accounts)
		require.Len(t, step1, 3)

		step2 := filterByMinLoadRate(step1)
		require.Len(t, step2, 3)

		// LRU 选择最早的
		selected := selectByLRU(step2, false)
		require.NotNil(t, selected)
		require.Equal(t, int64(3), selected.account.ID)
REDACTED)
REDACTED
