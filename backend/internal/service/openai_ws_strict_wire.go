package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openAIWSStrictItemMemory is the lifecycle form of a single output item, kept
// so a reduced terminal item can be repaired from it.
type openAIWSStrictItemMemory struct {
	itemType string
	callID   string
	snapshot json.RawMessage
}

// openAIWSStrictWireNormalizer keeps output-item fields stable across the
// lifecycle events emitted by Responses-compatible upstreams. Some upstreams
// send a complete output_item.done item but a reduced response.completed item.
// Strict clients deserialize both forms and require the fields to agree.
//
// Memory is keyed by item id rather than by array position: a terminal rebuild
// may drop, reorder, or collapse items, so the index of an item in
// response.output proves nothing about which lifecycle item it came from.
//
// The normalizer describes exactly one turn. Callers that reuse a connection
// across turns must call reset before the next turn is accepted.
type openAIWSStrictWireNormalizer struct {
	items    map[string]*openAIWSStrictItemMemory
	indexIDs map[int]string
}

func newOpenAIWSStrictWireNormalizer() *openAIWSStrictWireNormalizer {
	n := &openAIWSStrictWireNormalizer{}
	n.reset()
	return n
}

// reset drops every item remembered for the current turn. A connection that
// serves several turns must call this between them, otherwise the next turn
// inherits ids and content from the previous one.
func (n *openAIWSStrictWireNormalizer) reset() {
	if n == nil {
		return
	}
	n.items = make(map[string]*openAIWSStrictItemMemory)
	n.indexIDs = make(map[int]string)
}

// normalize repairs only known Responses output item shapes. Existing values
// and unknown fields remain untouched, so this is safe for vendor extensions.
// It returns an error only when a repair could not be completed safely; the
// returned payload is then the untouched input.
func (n *openAIWSStrictWireNormalizer) normalize(data []byte) ([]byte, bool, error) {
	if n == nil || len(data) == 0 || !gjson.ValidBytes(data) {
		return data, false, nil
	}

	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		status := "in_progress"
		if eventType == "response.output_item.done" {
			status = "completed"
		}
		index := int(gjson.GetBytes(data, "output_index").Int())
		// Live lifecycle events address items by position, so the index is a
		// reliable identity here, and a single item is never ambiguous.
		return n.normalizeItem(data, "item", status, index, eventType, true, nil, nil)

	case "response.content_part.added", "response.content_part.done":
		updated, changed := normalizeOpenAIWSOutputTextPart(data, "part")
		return updated, changed, nil

	case "response.created", "response.in_progress", "response.completed", "response.done",
		"response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		responseStatus := strings.TrimSpace(gjson.GetBytes(data, "response.status").String())
		status := openAIWSStrictItemStatusForResponseStatus(responseStatus)
		if responseStatus == "" && (eventType == "response.completed" || eventType == "response.done") {
			status = "completed"
		}
		return n.normalizeOutputList(data, "response.output", status, eventType)
	default:
		if gjson.GetBytes(data, "object").String() == "response" || gjson.GetBytes(data, "output").IsArray() {
			responseStatus := strings.TrimSpace(gjson.GetBytes(data, "status").String())
			status := openAIWSStrictItemStatusForResponseStatus(responseStatus)
			if responseStatus == "" {
				status = "completed"
			}
			return n.normalizeOutputList(data, "output", status, "response")
		}
		return data, false, nil
	}
}

func (n *openAIWSStrictWireNormalizer) normalizeOutputList(
	data []byte,
	path string,
	status string,
	eventType string,
) ([]byte, bool, error) {
	output := gjson.GetBytes(data, path)
	if !output.IsArray() {
		return data, false, nil
	}
	// Walk the list once before repairing anything, so the outcome does not
	// depend on the order the items happen to appear in.
	//
	// reserved holds every memory an item names outright, by id or by call_id.
	// Those memories belong to that item, and the type fallback below must not
	// hand one to an anonymous item as well; doing so would mint a duplicate id
	// whenever the anonymous item was listed first.
	//
	// fallbackCandidates counts the items that carry neither, which can only be
	// matched by type. If a type names more than one such item none of them can
	// be proven to be the remembered one, so none may inherit.
	reserved := make(map[string]bool)
	fallbackCandidates := make(map[string]int)
	for _, item := range output.Array() {
		if id := strings.TrimSpace(item.Get("id").String()); id != "" {
			if _, ok := n.items[id]; ok {
				reserved[id] = true
			}
			continue
		}
		if callID := strings.TrimSpace(item.Get("call_id").String()); callID != "" {
			for id, memory := range n.items {
				if memory.callID == callID {
					reserved[id] = true
				}
			}
			continue
		}
		if itemType := strings.TrimSpace(item.Get("type").String()); itemType != "" {
			fallbackCandidates[itemType]++
		}
	}

	updated := data
	changed := false
	for index := range output.Array() {
		itemPath := path + "." + strconv.Itoa(index)
		// A rebuilt output list may have dropped or reordered items, so the
		// position must not be used to identify them.
		next, itemChanged, err := n.normalizeItem(updated, itemPath, status, index, eventType, false, reserved, fallbackCandidates)
		if err != nil {
			return data, false, err
		}
		if itemChanged {
			updated = next
			changed = true
		}
	}
	return updated, changed, nil
}

func (n *openAIWSStrictWireNormalizer) normalizeItem(
	data []byte,
	path string,
	defaultStatus string,
	outputIndex int,
	eventType string,
	indexTrusted bool,
	reserved map[string]bool,
	fallbackCandidates map[string]int,
) ([]byte, bool, error) {
	item := gjson.GetBytes(data, path)
	if !item.IsObject() {
		return data, false, nil
	}

	updated := data
	changed := false
	itemType := strings.TrimSpace(item.Get("type").String())

	// Only inherit remembered state when this item can be proven to be the same
	// item that produced the snapshot. When identity cannot be established the
	// current fields are kept as they are and a fresh id is generated below.
	// output_item.added/done state which phase of the lifecycle the item is in,
	// so their own status wins. Only a rebuilt terminal item, whose status was
	// dropped, may take the status back from the snapshot.
	lifecycleEvent := eventType == "response.output_item.added" || eventType == "response.output_item.done"
	if memoryID := n.resolveMemoryID(item, itemType, outputIndex, indexTrusted, reserved, fallbackCandidates); memoryID != "" {
		if memory := n.items[memoryID]; memory != nil && len(memory.snapshot) > 0 {
			var snapshotObject map[string]json.RawMessage
			if json.Unmarshal(memory.snapshot, &snapshotObject) == nil {
				updated, changed = mergeOpenAIWSItemSnapshot(updated, path, snapshotObject, !lifecycleEvent)
				item = gjson.GetBytes(updated, path)
			}
		}
	}

	prefix, strict := openAIWSStrictItemIDPrefix(itemType)
	if !strict {
		return updated, changed, nil
	}

	itemID := strings.TrimSpace(item.Get("id").String())
	if itemID == "" {
		generated, err := generateOpenAIWSStrictItemID(prefix)
		if err != nil {
			return data, false, err
		}
		itemID = generated
		next, setErr := sjson.SetBytes(updated, path+".id", itemID)
		if setErr == nil {
			updated = next
			changed = true
		}
	}

	item = gjson.GetBytes(updated, path)
	if (item.Get("status").Type == gjson.Null || !item.Get("status").Exists()) && defaultStatus != "" {
		next, err := sjson.SetBytes(updated, path+".status", defaultStatus)
		if err == nil {
			updated = next
			changed = true
		}
	}

	if itemType == "message" {
		contentPath := path + ".content"
		content := gjson.GetBytes(updated, contentPath)
		if !content.IsArray() {
			next, err := sjson.SetRawBytes(updated, contentPath, []byte("[]"))
			if err == nil {
				updated = next
				changed = true
			}
		} else {
			for index, part := range content.Array() {
				partType := strings.TrimSpace(part.Get("type").String())
				if partType == "" || partType == "text" || partType == "output_text" {
					partPath := contentPath + "." + strconv.Itoa(index)
					next, partChanged := normalizeOpenAIWSOutputTextPart(updated, partPath)
					if partChanged {
						updated = next
						changed = true
					}
				}
			}
		}
	}

	if eventType == "response.output_item.added" || eventType == "response.output_item.done" {
		if item := gjson.GetBytes(updated, path); item.IsObject() {
			n.remember(outputIndex, itemID, itemType, item)
		}
	}
	return updated, changed, nil
}

// resolveMemoryID returns the id of the remembered item this item provably is,
// or an empty string when identity cannot be established. Inheriting on a weak
// match would copy one item's id, tool arguments, or reasoning payload onto a
// different item, so every branch here fails closed.
func (n *openAIWSStrictWireNormalizer) resolveMemoryID(
	item gjson.Result,
	itemType string,
	outputIndex int,
	indexTrusted bool,
	reserved map[string]bool,
	fallbackCandidates map[string]int,
) string {
	if n == nil || itemType == "" {
		return ""
	}

	// The item's own id is the strongest identity available. When it names an
	// item we never saw, nothing remembered describes it.
	if id := strings.TrimSpace(item.Get("id").String()); id != "" {
		if memory := n.items[id]; memory != nil && memory.itemType == itemType {
			return id
		}
		return ""
	}

	// Tool calls carry call_id, which survives a terminal rebuild even when the
	// item id does not.
	if callID := strings.TrimSpace(item.Get("call_id").String()); callID != "" {
		for id, memory := range n.items {
			if memory.callID == callID && memory.itemType == itemType {
				return id
			}
		}
		// A live lifecycle event may report call_id for the first time on done,
		// after added already minted an id without one. Falling through to the
		// trusted index keeps that item's id stable; a rebuilt terminal item has
		// no such fallback and stops here.
		if !indexTrusted {
			return ""
		}
	}

	// Live lifecycle events are addressed by output_index, so the position is
	// authoritative there.
	if indexTrusted {
		if id := n.indexIDs[outputIndex]; id != "" {
			if memory := n.items[id]; memory != nil && memory.itemType == itemType {
				return id
			}
		}
		return ""
	}

	// In a rebuilt list the position proves nothing. Inheriting by type alone is
	// only safe when it can name exactly one item on both sides: one remembered
	// item of this type, and one terminal item that needs it. Anything else
	// leaves two items competing for the same memory.
	if fallbackCandidates[itemType] != 1 {
		return ""
	}
	matchID := ""
	for id, memory := range n.items {
		if memory.itemType != itemType {
			continue
		}
		if matchID != "" {
			return ""
		}
		matchID = id
	}
	if matchID == "" || reserved[matchID] {
		return ""
	}
	return matchID
}

// remember records the lifecycle form of an item. An item observed again at the
// same output_index supersedes the previous recording rather than adding a
// second entry, so the per-type uniqueness check stays accurate.
func (n *openAIWSStrictWireNormalizer) remember(outputIndex int, itemID string, itemType string, item gjson.Result) {
	if n == nil || itemID == "" || item.Raw == "" {
		return
	}
	if previousID := n.indexIDs[outputIndex]; previousID != "" && previousID != itemID {
		delete(n.items, previousID)
	}
	n.indexIDs[outputIndex] = itemID
	n.items[itemID] = &openAIWSStrictItemMemory{
		itemType: itemType,
		callID:   strings.TrimSpace(item.Get("call_id").String()),
		snapshot: json.RawMessage(append([]byte(nil), item.Raw...)),
	}
}

func mergeOpenAIWSItemSnapshot(
	data []byte,
	path string,
	snapshot map[string]json.RawMessage,
	restoreStatus bool,
) ([]byte, bool) {
	updated := data
	changed := false
	for _, field := range []string{
		"id", "phase", "role", "call_id", "name", "arguments", "namespace", "input",
		"encrypted_content", "summary",
	} {
		raw, ok := snapshot[field]
		if !ok || len(raw) == 0 {
			continue
		}
		current := gjson.GetBytes(updated, path+"."+field)
		if current.Exists() && current.Type != gjson.Null {
			continue
		}
		next, err := sjson.SetRawBytes(updated, path+"."+field, raw)
		if err == nil {
			updated = next
			changed = true
		}
	}
	// For a rebuilt terminal item a status the item itself already settled on
	// outranks anything derived from the response-level status, so an item that
	// already completed is not downgraded when the response later fails. Only a
	// settled status counts: the in_progress minted for output_item.added is
	// provisional, and an item that never reached done must follow the response
	// into completed or incomplete instead of staying in flight. A lifecycle
	// event states its own phase and never takes a status back from the
	// snapshot at all.
	if restoreStatus {
		if raw, ok := snapshot["status"]; ok && len(raw) > 0 && openAIWSStrictSnapshotStatusIsTerminal(raw) {
			current := gjson.GetBytes(updated, path+".status")
			if !current.Exists() || current.Type == gjson.Null {
				next, err := sjson.SetRawBytes(updated, path+".status", raw)
				if err == nil {
					updated = next
					changed = true
				}
			}
		}
	}

	contentRaw, ok := snapshot["content"]
	if !ok || len(contentRaw) == 0 {
		return updated, changed
	}
	currentContent := gjson.GetBytes(updated, path+".content")
	if !currentContent.IsArray() {
		next, err := sjson.SetRawBytes(updated, path+".content", contentRaw)
		if err == nil {
			return next, true
		}
		return updated, changed
	}
	var snapshotParts []json.RawMessage
	if json.Unmarshal(contentRaw, &snapshotParts) != nil {
		return updated, changed
	}
	for index, partRaw := range snapshotParts {
		if index >= len(currentContent.Array()) {
			break
		}
		var snapshotPart map[string]json.RawMessage
		if json.Unmarshal(partRaw, &snapshotPart) != nil {
			continue
		}
		partPath := path + ".content." + strconv.Itoa(index)
		for _, field := range []string{"type", "text", "annotations", "logprobs"} {
			raw, exists := snapshotPart[field]
			if !exists || len(raw) == 0 {
				continue
			}
			current := gjson.GetBytes(updated, partPath+"."+field)
			if current.Exists() && current.Type != gjson.Null {
				continue
			}
			next, err := sjson.SetRawBytes(updated, partPath+"."+field, raw)
			if err == nil {
				updated = next
				changed = true
			}
		}
	}
	return updated, changed
}

func normalizeOpenAIWSOutputTextPart(data []byte, path string) ([]byte, bool) {
	part := gjson.GetBytes(data, path)
	if !part.IsObject() {
		return data, false
	}
	partType := strings.TrimSpace(part.Get("type").String())
	if partType != "" && partType != "text" && partType != "output_text" {
		return data, false
	}
	updated := data
	changed := false
	if partType == "" || partType == "text" {
		next, err := sjson.SetBytes(updated, path+".type", "output_text")
		if err == nil {
			updated = next
			changed = true
		}
	}
	part = gjson.GetBytes(updated, path)
	if part.Get("text").Type != gjson.String {
		next, err := sjson.SetBytes(updated, path+".text", "")
		if err == nil {
			updated = next
			changed = true
		}
	}
	if !part.Get("annotations").IsArray() {
		next, err := sjson.SetRawBytes(updated, path+".annotations", []byte("[]"))
		if err == nil {
			updated = next
			changed = true
		}
	}
	if !part.Get("logprobs").IsArray() {
		next, err := sjson.SetRawBytes(updated, path+".logprobs", []byte("[]"))
		if err == nil {
			updated = next
			changed = true
		}
	}
	return updated, changed
}

// openAIWSStrictItemStatusForResponseStatus maps a response-level status onto
// a status the Responses protocol allows on an output item. Response terminal
// states such as failed or cancelled have no item-level counterpart, so items
// that never reported one of their own are reported as incomplete. An empty
// result means "say nothing", which leaves the item status to the lifecycle
// snapshot instead of inventing a value strict clients would reject.
func openAIWSStrictItemStatusForResponseStatus(responseStatus string) string {
	switch strings.TrimSpace(responseStatus) {
	case "completed":
		return "completed"
	case "in_progress", "queued":
		return "in_progress"
	case "incomplete", "failed", "cancelled", "canceled":
		return "incomplete"
	default:
		return ""
	}
}

// openAIWSStrictSnapshotStatusIsTerminal reports whether a remembered status is
// one the item settled on, as opposed to the provisional in_progress that
// output_item.added mints before the item has finished.
func openAIWSStrictSnapshotStatusIsTerminal(raw json.RawMessage) bool {
	var status string
	if json.Unmarshal(raw, &status) != nil {
		return false
	}
	switch strings.TrimSpace(status) {
	case "completed", "incomplete":
		return true
	default:
		return false
	}
}

func openAIWSStrictItemIDPrefix(itemType string) (string, bool) {
	switch strings.TrimSpace(itemType) {
	case "message":
		return "msg", true
	case "reasoning":
		return "rs", true
	case "function_call":
		return "fc", true
	case "custom_tool_call":
		return "ctc", true
	case "tool_search_call":
		return "tsc", true
	default:
		return "", false
	}
}

// generateOpenAIWSStrictItemID mints an item id. A failing entropy source must
// not silently yield an all-zero, repeatable id, so the error is returned and
// the caller leaves the payload untouched rather than emitting a colliding id.
//
// On the current toolchain this error is unreachable: crypto/rand.Read is
// documented never to return an error and crashes the process instead. The
// error is still surfaced rather than discarded, so the guarantee lives in one
// place if that contract or the package's Reader ever changes.
func generateOpenAIWSStrictItemID(prefix string) (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate strict responses item id: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(buf), nil
}
