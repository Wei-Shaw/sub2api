package service

import (
	"strconv"

	"github.com/tidwall/gjson"
)

type openAINativeWebSearchCounter struct {
	enabled bool
	seen    map[string]struct{}
}

func newOpenAINativeWebSearchCounter(account *Account) *openAINativeWebSearchCounter {
	return &openAINativeWebSearchCounter{enabled: account != nil && account.IsOpenAI(), seen: make(map[string]struct{})}
}

func (counter *openAINativeWebSearchCounter) Add(data []byte) {
	if counter == nil || !counter.enabled {
		return
	}
	root := gjson.ParseBytes(data)
	eventType := root.Get("type").String()
	add := func(item gjson.Result, index string) {
		if item.Get("type").String() != "web_search_call" || item.Get("status").String() != "completed" {
			return
		}
		key := item.Get("id").String()
		if key == "" {
			if index == "" {
				return
			}
			key = "output_index:" + index
		}
		counter.seen[key] = struct{}{}
	}
	switch eventType {
	case "response.output_item.done":
		add(root.Get("item"), root.Get("output_index").String())
	case "response.web_search_call.completed":
		if itemID := root.Get("item_id").String(); itemID != "" {
			counter.seen[itemID] = struct{}{}
		}
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		for index, item := range root.Get("response.output").Array() {
			add(item, strconv.Itoa(index))
		}
	case "":
		for index, item := range root.Get("output").Array() {
			add(item, strconv.Itoa(index))
		}
	}
}

func (counter *openAINativeWebSearchCounter) Count() int {
	return len(counter.seen)
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
