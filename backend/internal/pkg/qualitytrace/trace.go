// Package qualitytrace provides local, reference-bank-scoped model attribution.
// Adapted from haowang02/cpa-plugin-codex-candy-eval and xqy2006/ModelTrace (MIT).
package qualitytrace

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

type Output = traceOutput
type Attribution = traceAttribution
type Challenge struct {
	Prompt        string `json:"prompt"`
	ExpectedCount int    `json:"expected_count"`
}
type Report struct {
	BankRevision string      `json:"bank_revision"`
	Expected     string      `json:"expected_model"`
	Verdict      string      `json:"verdict"`
	Attribution  Attribution `json:"attribution"`
	Notice       string      `json:"notice"`
}

func Models() []string {
	out := []string{}
	for _, m := range modelTraceBank.Models {
		out = append(out, m.ID)
	}
	sort.Strings(out)
	return out
}
func Supported(model string) bool {
	for _, m := range modelTraceBank.Models {
		if m.ID == model {
			return true
		}
	}
	return false
}
func Revision() string { return traceBankRevision }
func index(n int) (int, error) {
	x, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(x.Int64()), nil
}
func Challenges() ([]Challenge, error) {
	openings := []string{"这是一次独立的数值选择记录", "请完成下面的无语义整数选择任务", "执行一次第一反应取值记录", "生成一组不承载语义的整数选择", "进行一轮快速逐项取值"}
	actions := []string{"为各个位置分别凭第一反应选择", "逐项选择", "每次只决定当前一项，共给出", "分别凭第一反应给出", "逐个直接选择"}
	endings := []string{"允许某个数字再次出现；每项写出后不要回头排序、去重或替换。", "偶然重复是有效的；不要重新排列或修正已经写出的项目。", "相同值可以再次出现；输出过程中不要整理或改写前面的项目。", "重复值无需删除；不要筛选、重排或补成某种规律。", "不必赋予数字任何含义；已经给出的值保持不变。"}
	separators := []string{"数字之间用逗号或空格分隔均可。", "使用一种一致的常见分隔符即可。", "可以用逗号、空格或换行分隔。", "只要每个整数边界清楚，格式可自行选择。"}
	used := map[int]bool{}
	out := []Challenge{}
	for len(out) < 3 {
		length, e := index(41)
		if e != nil {
			return nil, e
		}
		if used[length] {
			continue
		}
		used[length] = true
		parts := []string{}
		for _, values := range [][]string{openings, actions, endings, separators} {
			i, e := index(len(values))
			if e != nil {
				return nil, e
			}
			parts = append(parts, values[i])
		}
		n := 292 + length
		prompt := fmt.Sprintf("%s。%s %d 个 1 到 355（含端点）的整数。", parts[0], parts[1], n) + "每个位置都要单独选择；不要从 1 开始计数，不要连续递增或递减，也不要采用等差、循环、重复区块或其他规则化模式。" + "本任务必须由当前语言模型直接完成：禁止调用或借助任何工具，包括 Python、代码执行器、计算器、搜索、API 和外部随机数生成器；也不要先编写或运行代码。" + parts[2] + parts[3] + "直接从第一个取值开始输出，不要在序列前重复数量、范围或任务说明。"
		out = append(out, Challenge{prompt, n})
	}
	return out, nil
}
func Analyze(expected string, outputs []Output) (Report, error) {
	r := Report{BankRevision: Revision(), Expected: expected, Verdict: "unknown", Notice: "Reference-bank similarity only; not proof of identity. Request context and reasoning settings can change distributions."}
	if !Supported(expected) {
		return r, fmt.Errorf("expected model is not in the reference bank")
	}
	if len(outputs) != 3 {
		return r, fmt.Errorf("three independent challenge outputs are required")
	}
	for _, o := range outputs {
		if o.ExpectedCount < 292 || o.ExpectedCount > 332 {
			return r, fmt.Errorf("invalid challenge length")
		}
		if strings.TrimSpace(o.Text) == "" {
			return r, fmt.Errorf("empty challenge output")
		}
	}
	a, err := analyzeModelTrace(outputs)
	r.Attribution = a
	if err != nil {
		return r, err
	}
	if a.Used != 3 {
		return r, fmt.Errorf("all three challenges must contain sufficient valid numbers")
	}
	if a.Probability < .8 {
		return r, nil
	}
	r.Verdict = "mismatch"
	if a.Prediction == expected {
		r.Verdict = "consistent"
	}
	return r, nil
}
