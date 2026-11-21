package service

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

type openAINativeWebSearchCounter struct {
	enabled        bool
	count          int
	settledCount   int
	current        *openAINativeWebSearchResponse
	responses      map[string]*openAINativeWebSearchResponse
	settledItemIDs map[string]struct{}
}

type openAINativeWebSearchResponse struct {
	id       string
	aliases  map[string]*openAINativeWebSearchCall
	count    int
	terminal bool
	settled  bool
}

type openAINativeWebSearchCall struct {
	parent    *openAINativeWebSearchCall
	completed bool
}

func (call *openAINativeWebSearchCall) root() *openAINativeWebSearchCall {
	if call == nil {
		return nil
	}
	if call.parent != nil {
		call.parent = call.parent.root()
		return call.parent
	}
	return call
}

func newOpenAINativeWebSearchCounter(account *Account) *openAINativeWebSearchCounter {
	return &openAINativeWebSearchCounter{
		enabled:        account != nil && account.IsOpenAI(),
		responses:      make(map[string]*openAINativeWebSearchResponse),
		settledItemIDs: make(map[string]struct{}),
	}
}

func (counter *openAINativeWebSearchCounter) add(response *openAINativeWebSearchResponse, itemID, index string, completed bool) {
	if response.settled || (itemID == "" && index == "") {
		return
	}
	var call, indexedCall *openAINativeWebSearchCall
	if itemID != "" {
		call = response.aliases["id:"+itemID].root()
	}
	if index != "" {
		indexedCall = response.aliases["index:"+index].root()
	}
	if call == nil {
		call = indexedCall
	} else if indexedCall != nil && indexedCall != call {
		if call.completed && indexedCall.completed {
			counter.count--
			response.count--
		}
		call.completed = call.completed || indexedCall.completed
		indexedCall.parent = call
	}
	if call == nil {
		call = &openAINativeWebSearchCall{}
	}
	if completed && !call.completed {
		call.completed = true
		counter.count++
		response.count++
	}
	if itemID != "" {
		response.aliases["id:"+itemID] = call
	}
	if index != "" {
		response.aliases["index:"+index] = call
	}
}

func (counter *openAINativeWebSearchCounter) Add(data []byte) {
	if counter == nil || !counter.enabled {
		return
	}
	root := gjson.ParseBytes(data)
	eventType := root.Get("type").String()
	responseID := root.Get("response.id").String()
	if responseID == "" {
		responseID = root.Get("response_id").String()
	}
	if eventType == "" {
		responseID = root.Get("id").String()
	}
	if responseID == "" {
		itemID := root.Get("item_id").String()
		if itemID == "" {
			itemID = root.Get("item.id").String()
		}
		if _, settled := counter.settledItemIDs[itemID]; settled {
			return
		}
	}
	response := counter.responses[responseID]
	if responseID == "" || response == nil {
		response = counter.current
		if responseID == "" && response != nil && response.terminal {
			itemID := root.Get("item_id").String()
			if itemID == "" {
				itemID = root.Get("item.id").String()
			}
			index := root.Get("output_index").String()
			if eventType == "response.created" || (itemID != "" && response.aliases["id:"+itemID] == nil) || (itemID == "" && (index == "" || response.aliases["index:"+index] == nil)) {
				response = nil
			}
		}
		if response == nil || (responseID != "" && (response.terminal || (response.id != "" && response.id != responseID))) {
			response = &openAINativeWebSearchResponse{aliases: make(map[string]*openAINativeWebSearchCall)}
		}
		if responseID != "" {
			response.id = responseID
			counter.responses[responseID] = response
		}
		counter.current = response
	} else if eventType == "response.created" {
		counter.current = response
	}
	add := func(item gjson.Result, index string, completed bool) {
		if item.Get("type").String() != "web_search_call" {
			return
		}
		counter.add(response, item.Get("id").String(), index, completed && item.Get("status").String() == "completed")
	}
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		add(root.Get("item"), root.Get("output_index").String(), eventType == "response.output_item.done")
	case "response.web_search_call.in_progress", "response.web_search_call.searching", "response.web_search_call.completed":
		counter.add(response, root.Get("item_id").String(), root.Get("output_index").String(), eventType == "response.web_search_call.completed")
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		for index, item := range root.Get("response.output").Array() {
			add(item, strconv.Itoa(index), true)
		}
		response.terminal = true
	case "":
		for index, item := range root.Get("output").Array() {
			add(item, strconv.Itoa(index), true)
		}
		response.terminal = true
	}
}

func (counter *openAINativeWebSearchCounter) Count() int {
	return counter.count
}

func (counter *openAINativeWebSearchCounter) Settle(responseID string) int {
	response := counter.responses[responseID]
	if responseID == "" {
		response = counter.current
	}
	if response == nil || response.settled {
		return 0
	}
	response.settled = true
	response.terminal = true
	for alias := range response.aliases {
		if itemID, ok := strings.CutPrefix(alias, "id:"); ok {
			counter.settledItemIDs[itemID] = struct{}{}
		}
	}
	counter.settledCount += response.count
	return response.count
}

func (counter *openAINativeWebSearchCounter) UnsettledCount() int {
	return counter.count - counter.settledCount
}

func countOpenAINativeWebSearchCalls(account *Account, body []byte) int {
	counter := newOpenAINativeWebSearchCounter(account)
	if bodyHasSSEFraming(body) {
		forEachOpenAISSEFrame(string(body), func(eventType string, data []byte) {
			counter.Add([]byte(openAICompatPayloadWithEventType(string(data), eventType)))
		})
	} else {
		counter.Add(body)
	}
	return counter.Count()
}
