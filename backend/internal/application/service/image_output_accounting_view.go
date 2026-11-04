package service

import "github.com/tidwall/gjson"

// All values borrow the current input and must be consumed synchronously.
type openAIImageAccountingView struct {
	object                                                    gjson.Result
	eventType, data, output, item, response, usage, toolUsage gjson.Result
	duplicate                                                 bool
}

func projectOpenAIImageAccounting(root gjson.Result) openAIImageAccountingView {
	var view openAIImageAccountingView
	if !root.IsObject() {
		return view
	}
	view.object = root
	root.ForEach(func(key, value gjson.Result) bool {
		var field *gjson.Result
		switch key.Str {
		case "type":
			field = &view.eventType
		case "data":
			field = &view.data
		case "output":
			field = &view.output
		case "item":
			field = &view.item
		case "response":
			field = &view.response
		case "usage":
			field = &view.usage
		case "tool_usage":
			field = &view.toolUsage
		}
		// Match gjson.Get's first-key behavior, including explicit null.
		if field != nil {
			if field.Exists() {
				view.duplicate = true
			} else {
				*field = value
			}
		}
		return true
	})
	return view
}

func restoreImageAccountingDuplicatePaths(view openAIImageAccountingView) openAIImageAccountingView {
	if !view.duplicate || !view.object.Exists() {
		return view
	}
	view.eventType = view.object.Get("type")
	view.data = view.object.Get("data")
	view.output = view.object.Get("output")
	view.item = view.object.Get("item")
	view.response = view.object.Get("response")
	view.usage = view.object.Get("usage")
	view.toolUsage = view.object.Get("tool_usage")
	return view
}

func countOpenAIImageUsageFromViews(root, response openAIImageAccountingView) int {
	data := projectOpenAIImageAccounting(root.data)
	dataResponse := projectOpenAIImageAccounting(data.response)
	if root.duplicate || response.duplicate || data.duplicate || dataResponse.duplicate {
		// A missing nested value can make gjson continue at a later duplicate
		// parent. Preserve that behavior with the original full paths.
		return countOpenAIImageUsageFromObject(root.object)
	}
	count := 0
	for _, view := range [...]openAIImageAccountingView{root, response, data, dataResponse} {
		count = max(count, max(int(view.usage.Get("images").Int()), int(view.toolUsage.Get("image_gen.images").Int())))
	}
	return count
}

func countOpenAIImageUsageFromObject(root gjson.Result) int {
	count := 0
	for _, path := range [...]string{
		"usage.images", "response.usage.images", "data.usage.images", "data.response.usage.images",
		"tool_usage.image_gen.images", "response.tool_usage.image_gen.images", "data.tool_usage.image_gen.images", "data.response.tool_usage.image_gen.images",
	} {
		count = max(count, int(root.Get(path).Int()))
	}
	return count
}
