package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// customMenuCanonicalItem 只保留展示相关字段，且 map 迭代顺序由 encoding/json 保证按 key 字母序输出。
// 这里不使用 struct 以便在字段被后续版本增删时仍能通过输入 JSON 的实际字段决定 hash 输入；
// 每次实现时需要明确列出参与 hash 的字段列表，不参与的字段（如内部审计时间戳）永远不进入这个 map。
type customMenuCanonicalItem struct {
	Action     string `json:"action,omitempty"`
	IconSvg    string `json:"icon_svg,omitempty"`
	ID         string `json:"id,omitempty"`
	Label      string `json:"label,omitempty"`
	PageSlug   string `json:"page_slug,omitempty"`
	SortOrder  int    `json:"sort_order"`
	URL        string `json:"url,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// rawCustomMenuItem 用于反序列化输入 JSON，所有字段都是可选。
type rawCustomMenuItem struct {
	Action     string      `json:"action"`
	IconSvg    string      `json:"icon_svg"`
	ID         string      `json:"id"`
	Label      string      `json:"label"`
	PageSlug   string      `json:"page_slug"`
	SortOrder  json.Number `json:"sort_order"`
	URL        string      `json:"url"`
	Visibility string      `json:"visibility"`
}

// ComputeCustomMenuVersion 返回 custom_menu_items 与 red-dot 开关组合下的稳定短版本 hash。
//
// 规则：
//  1. 反序列化 itemsJSON → []rawCustomMenuItem，容错：空串或非法 JSON 均按空数组处理；
//  2. 按 sort_order 升序稳定排序，sort_order 相同则按 id 字母序；
//  3. 每项映射到 customMenuCanonicalItem，仅保留展示字段；json.Marshal 保证 map key 字母序；
//  4. 组装为 {"enabled":<bool>,"items":[...]} 无空白 JSON；
//  5. sha256 → hex 前 12 字符（小写）作为 version。
//
// 该函数是纯函数：相同输入 → 相同输出，相同 canonical 输入 → 相同输出。
func ComputeCustomMenuVersion(itemsJSON string, enabled bool) string {
	trimmed := strings.TrimSpace(itemsJSON)
	var raws []rawCustomMenuItem
	if trimmed != "" {
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&raws); err != nil {
			raws = nil
		}
	}

	items := make([]customMenuCanonicalItem, 0, len(raws))
	for _, r := range raws {
		sortOrder := 0
		if r.SortOrder != "" {
			if v, err := r.SortOrder.Int64(); err == nil {
				sortOrder = int(v)
			} else if f, err := r.SortOrder.Float64(); err == nil {
				sortOrder = int(f)
			}
		}
		items = append(items, customMenuCanonicalItem{
			Action:     strings.TrimSpace(r.Action),
			IconSvg:    r.IconSvg,
			ID:         strings.TrimSpace(r.ID),
			Label:      r.Label,
			PageSlug:   strings.TrimSpace(r.PageSlug),
			SortOrder:  sortOrder,
			URL:        strings.TrimSpace(r.URL),
			Visibility: strings.TrimSpace(r.Visibility),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].ID < items[j].ID
	})

	// 使用匿名结构体保证 JSON 输出字段顺序为 enabled, items（Go json 按 struct 字段定义顺序输出）。
	payload := struct {
		Enabled bool                      `json:"enabled"`
		Items   []customMenuCanonicalItem `json:"items"`
	}{
		Enabled: enabled,
		Items:   items,
	}

	// json.Marshal 对 map 会按 key 字母序输出；对 struct 按定义顺序；此处已用 struct 明确顺序。
	buf, err := json.Marshal(payload)
	if err != nil {
		// 极端情况下 fallback 到空数组的稳定 hash，避免 panic。
		buf = []byte(`{"enabled":false,"items":[]}`)
	}

	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])[:12]
}
