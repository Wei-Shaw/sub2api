// 本文件提供请求顶层字段的未消费扫描：解码器把识别不了/不支持的字段
// 记入 Dropped，避免客户端以为自己设置了 reasoning/store 等参数而实际
// 被静默丢弃。
package common

import (
	"encoding/json"
	"sort"
)

// UnconsumedFields 返回请求顶层中不在 consumed 名单内的字段名
// （"field:<name>" 形式），供解码器记入 RequestMessages.Dropped。
// 上游没有对应 wire 字段的参数必须透出为丢弃，而不是静默吞掉。
// data 不是合法 JSON 对象时返回 nil——解码错误由调用方的主解码路径上报。
func UnconsumedFields(data []byte, consumed map[string]bool) []string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil
	}
	var dropped []string
	for name := range fields {
		if !consumed[name] {
			dropped = append(dropped, "field:"+name)
		}
	}
	sort.Strings(dropped)
	return dropped
}
