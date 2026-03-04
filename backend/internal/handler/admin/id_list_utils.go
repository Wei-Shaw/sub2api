package admin

import "sort"

func normalizeInt64IDList(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
REDACTED

	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{REDACTED, len(ids))
	for _, id := range ids {
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
