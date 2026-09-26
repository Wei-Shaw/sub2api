package service

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

const modelEvaluationBenchmark = "candy-shape-v1"

// Independently worded benchmark inspired by haowang02/codex-candy-eval.
// Explicit shape selection removes the blind-draw ambiguity. No answer is sent.
const modelEvaluationPrompt = `请只通过推理解决这道题，不调用任何工具。
袋中有苹果、桃子、西瓜三种口味的糖果，每颗为圆形或星形。数量如下：
              苹果  桃子  西瓜
圆形            7     9     8
星形            7     6     4
你可以通过触感选择形状，但不能识别口味。摸取不放回。开始前必须确定圆形和星形各取多少颗，之后不能根据取到的口味调整。
要保证取出的糖果中至少有一对“形状不同，且一颗苹果味、一颗桃子味”的糖果，总共最少取多少颗？
请说明保证成功的取法，并解释为什么更少无法保证。最后一行只写 FINAL_ANSWER: 后接表示最少总数的一个整数。`

var modelEvaluationAnswer = regexp.MustCompile(`^FINAL_ANSWER:\s*(0|[1-9][0-9]*)$`)

func gradeModelEvaluation(text string) (string, *int) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	match := modelEvaluationAnswer.FindStringSubmatch(last)
	if len(match) != 2 || strings.Count(text, "FINAL_ANSWER:") != 1 {
		return "ungraded", nil
	}
	answer, err := strconv.Atoi(match[1])
	if err != nil {
		return "ungraded", nil
	}
	if answer == 21 {
		return "correct", &answer
	}
	return "incorrect", &answer
}

func evaluationToken(usage gjson.Result, paths ...string) *int64 {
	for _, path := range paths {
		value := usage.Get(path)
		if value.Type == gjson.Number && value.Float() >= 0 && value.Float() == float64(value.Int()) {
			n := value.Int()
			return &n
		}
	}
	return nil
}

func parseModelEvaluationResponse(body []byte, result *ModelEvaluationResult) {
	if !gjson.ValidBytes(body) {
		result.Error = "上游返回无效响应；请检查账号连通性后重试"
		return
	}
	root := gjson.ParseBytes(body)
	setModelEvaluationUsage(root.Get("usage"), result)
	var parts []string
	root.Get("output").ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "message" && item.Get("role").String() == "assistant" {
			item.Get("content").ForEach(func(_, content gjson.Result) bool {
				if content.Get("type").String() == "output_text" {
					parts = append(parts, content.Get("text").String())
				}
				return true
			})
		}
		return true
	})
	if len(parts) == 0 && root.Get("choices.0.message.content").Type == gjson.String {
		parts = append(parts, root.Get("choices.0.message.content").String())
	}
	result.Text = strings.Join(parts, "\n")
	status := root.Get("status").String()
	finish := root.Get("choices.0.finish_reason").String()
	if root.Get("error").IsObject() || (status != "" && status != "completed") || (finish != "" && finish != "stop") {
		result.Error = "回答被截断、拒绝或未正常完成；请调整模型或推理强度后重试"
		return
	}
	if strings.TrimSpace(result.Text) == "" {
		result.Error = "上游未返回最终文本；请检查模型支持后重试"
		return
	}
	result.Status, result.Answer = gradeModelEvaluation(result.Text)
}

func setModelEvaluationUsage(usage gjson.Result, result *ModelEvaluationResult) {
	result.InputTokens = evaluationToken(usage, "input_tokens", "prompt_tokens")
	result.OutputTokens = evaluationToken(usage, "output_tokens", "completion_tokens")
	result.ReasoningTokens = evaluationToken(usage, "output_tokens_details.reasoning_tokens", "completion_tokens_details.reasoning_tokens")
	result.CachedTokens = evaluationToken(usage, "input_tokens_details.cached_tokens", "prompt_tokens_details.cached_tokens")
}
