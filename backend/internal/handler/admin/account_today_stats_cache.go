package admin

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

var accountTodayStatsBatchCache = newSnapshotCache(30 * time.Second)

func normalizeAccountIDList(accountIDs []int64) []int64 {
	if len(accountIDs) == 0 {
		return nil
REDACTED
	seen := make(map[int64]struct{REDACTED, len(accountIDs))
	out := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			continue
	REDACTED
		if _, ok := seen[id]; ok {
			continue
	REDACTED
		seen[id] = struct{REDACTED{REDACTED
		out = append(out, id)
REDACTED
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] REDACTED)
	return out
REDACTED

func buildAccountTodayStatsBatchCacheKey(accountIDs []int64) string {
	if len(accountIDs) == 0 {
		return "accounts_today_stats_empty"
REDACTED
	var b strings.Builder
	b.Grow(len(accountIDs) * 6)
	_, _ = b.WriteString("accounts_today_stats:")
	for i, id := range accountIDs {
		if i > 0 {
			_ = b.WriteByte(',')
	REDACTED
		_, _ = b.WriteString(strconv.FormatInt(id, 10))
REDACTED
	return b.String()
REDACTED
