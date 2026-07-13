package service

import "github.com/tidwall/gjson"

// HasCompactionTriggerInInput detects an input item with
// type="compaction_trigger". The handler combines this body signal with the
// request path, stream flag, and Codex beta feature header to distinguish the
// native remote compaction v2 wire from the legacy /responses/compact bridge.
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
REDACTED
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
REDACTED
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
	REDACTED
		return true
REDACTED)
	return found
REDACTED
