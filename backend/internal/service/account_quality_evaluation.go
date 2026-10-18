package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/qualitytrace"
	"log/slog"
	"math/big"
	"strings"
	"time"
)

func (s *AccountQualityService) applyReliabilityCoverage(ctx context.Context, rows []AccountQualityRow) {
	known := s.ops != nil && s.ops.IsMonitoringEnabled(ctx)
	for i := range rows {
		rows[i].ReliabilityKnown = known && rows[i].UnknownFailureScope == 0
		if rows[i].TraceAt != nil && time.Since(*rows[i].TraceAt) > time.Hour {
			rows[i].TraceStatus = "stale"
		}
	}
}
func qualityRoundRequestLimit(p AccountQualityPolicy, benchmark string) int {
	switch benchmark {
	case "modeltrace":
		return 3
	case "context":
		return 3
	}
	n := 3
	if p.TraceOnAnomaly {
		n += 3
	}
	if p.ContextCheck {
		n += 3
	}
	return n
}
func qualityChallengeEvidence(prompt, expected string, extra map[string]any) json.RawMessage {
	if extra == nil {
		extra = map[string]any{}
	}
	hash := sha256.Sum256([]byte(prompt))
	extra["prompt_sha256"] = hex.EncodeToString(hash[:])
	extra["prompt"] = prompt
	extra["prompt_chars"] = len([]rune(prompt))
	extra["expected_answer"] = expected
	raw, _ := json.Marshal(extra)
	return raw
}
func (s *AccountQualityService) probe(ctx context.Context, run *AccountQualityRun, p AccountQualityPolicy, id int64) {
	defer func() {
		if r := recover(); r != nil {
			s.persistenceFailures.Store(run.ID, true)
			slog.Error("account quality worker panic", "run_id", run.ID, "account_id", id)
		}
	}()
	switch run.Benchmark {
	case "modeltrace":
		s.probeTrace(ctx, run, p, id)
		return
	case "context":
		s.probeContext(ctx, run, p, id)
		return
	}
	s.probeCandy(ctx, run, p, id)
	if !p.TraceOnAnomaly && !p.ContextCheck {
		return
	}
	records, err := s.repo.Results(ctx, run.GroupID, id)
	if err != nil {
		return
	}
	anomaly := false
	usable := false
	for _, r := range records {
		if r.RunID != run.ID {
			continue
		}
		if r.Verdict == "passed" || r.Verdict == "incorrect" {
			usable = true
		}
		if r.Verdict == "incorrect" {
			anomaly = true
		}
	}
	if !usable {
		return
	}
	if anomaly && p.TraceOnAnomaly {
		s.probeTrace(ctx, run, p, id)
	}
	if p.ContextCheck {
		s.probeContext(ctx, run, p, id)
	}
}
func (s *AccountQualityService) probeTrace(ctx context.Context, run *AccountQualityRun, p AccountQualityPolicy, id int64) {
	if !qualitytrace.Supported(p.ReferenceModel) {
		return
	}
	challenges, err := qualitytrace.Challenges()
	if err != nil {
		return
	}
	outputs := []qualitytrace.Output{}
	start := time.Now()
	reason := ""
	calls := 0
	for _, c := range challenges {
		if ctx.Err() != nil || s.repo.Canceled(ctx, run.ID) {
			return
		}
		calls++
		answer, detail, _ := s.tester.RunQualityProbe(ctx, id, p, c.Prompt)
		if detail != "" {
			reason = detail
			break
		}
		outputs = append(outputs, qualitytrace.Output{Text: answer, ExpectedCount: c.ExpectedCount})
	}
	report, analysisErr := qualitytrace.Analyze(p.ReferenceModel, outputs)
	verdict := report.Verdict
	if reason != "" {
		verdict = "error"
	} else if analysisErr != nil {
		reason = "insufficient_valid_fingerprint_outputs"
	}
	evidence := map[string]any{"bank_revision": report.BankRevision, "expected_model": report.Expected, "attribution": report.Attribution, "notice": report.Notice, "challenges": challenges, "outputs": outputs, "request_endpoint": p.Endpoint, "reasoning_effort": p.Effort, "service_tier": p.ServiceTier}
	raw, _ := json.Marshal(evidence)
	answers := []string{}
	for _, o := range outputs {
		answers = append(answers, o.Text)
	}
	result := &AccountQualityResult{RunID: run.ID, GroupID: run.GroupID, AccountID: id, Model: p.Model, Effort: p.Effort, ServiceTier: p.ServiceTier, Benchmark: "modeltrace-v1", Verdict: verdict, Answer: strings.Join(answers, "\n\n"), Detail: reason, Evidence: raw, DurationMs: time.Since(start).Milliseconds(), Endpoint: p.Endpoint, RequestsUsed: calls}
	result.Evidence = qualityResponseEvidence(result.Evidence, result.Answer)
	if err = s.repo.AddResult(ctx, result); err != nil {
		s.persistenceFailures.Store(run.ID, true)
		slog.Error("fingerprint evidence not persisted", "error", err)
	}
}
func qualityRetrievalChallenge(chars int) (prompt, expected string, err error) {
	n, e := rand.Int(rand.Reader, big.NewInt(800000000))
	if e != nil {
		return "", "", e
	}
	answer := fmt.Sprint(n.Int64() + 100000000)
	keyBytes := make([]byte, 8)
	if _, e = rand.Read(keyBytes); e != nil {
		return "", "", e
	}
	key := hex.EncodeToString(keyBytes)
	var records strings.Builder
	lineLength := len(fmt.Sprintf("record_%08x = %09d; metadata=archived;\n", 0, 100000000))
	rowCount := max(1, (chars+lineLength-1)/lineLength)
	position, e := rand.Int(rand.Reader, big.NewInt(int64(rowCount)))
	if e != nil {
		return "", "", e
	}
	for i := 0; i < rowCount; i++ {
		if i == int(position.Int64()) {
			fmt.Fprintf(&records, "record_%s = %s;\n", key, answer)
		}
		fmt.Fprintf(&records, "record_%08x = %09d; metadata=archived;\n", i, (i*7919+137)%900000000+100000000)
	}
	prompt = "从下列记录中准确读取指定编号的值，不调用工具，不使用记录里的其他值。\n" + records.String() + "\n只查询 record_" + key + " 的值，仅输出 <answer>九位整数</answer>。"
	return prompt, answer, nil
}
func (s *AccountQualityService) probeContext(ctx context.Context, run *AccountQualityRun, p AccountQualityPolicy, id int64) {
	for _, spec := range []struct {
		name  string
		chars int
	}{{"context-short-v1", 1000}, {"context-medium-v1", min(8000, max(2000, p.ContextChars/2))}, {"context-long-v1", p.ContextChars}} {
		if ctx.Err() != nil || s.repo.Canceled(ctx, run.ID) {
			return
		}
		prompt, expected, err := qualityRetrievalChallenge(spec.chars)
		if err != nil {
			return
		}
		answer, detail, ms := s.tester.RunQualityProbe(ctx, id, p, prompt)
		verdict := "error"
		if detail == "" {
			verdict = judgeAccountQualityAnswer(answer, expected)
		}
		result := &AccountQualityResult{RunID: run.ID, GroupID: run.GroupID, AccountID: id, Model: p.Model, Effort: p.Effort, ServiceTier: p.ServiceTier, Endpoint: p.Endpoint, RequestsUsed: 1, Benchmark: spec.name, Verdict: verdict, Answer: answer, Detail: detail, DurationMs: ms, Evidence: qualityChallengeEvidence(prompt, expected, map[string]any{"synthetic_context": true})}
		result.Evidence = qualityResponseEvidence(result.Evidence, result.Answer)
		if err = s.repo.AddResult(ctx, result); err != nil {
			s.persistenceFailures.Store(run.ID, true)
			slog.Error("context evidence not persisted", "error", err)
			return
		}
	}
}

type qualityCandyTable struct{ AppleRound, PeachRound, OtherRound, AppleStar, PeachStar, OtherStar int }

// A failed draw is possible iff either selected shape contains only unrelated
// flavor, or both shapes can contain the same sole target flavor. Enumerating
// all shape quotas gives the exact minimum for this explicitly stated task.
func qualityCandyMinimum(t qualityCandyTable) int {
	best := t.AppleRound + t.PeachRound + t.OtherRound + t.AppleStar + t.PeachStar + t.OtherStar
	for round := 0; round <= t.AppleRound+t.PeachRound+t.OtherRound; round++ {
		for star := 0; star <= t.AppleStar+t.PeachStar+t.OtherStar; star++ {
			guaranteed := round > t.OtherRound && star > t.OtherStar && (round > t.AppleRound+t.OtherRound || star > t.AppleStar+t.OtherStar) && (round > t.PeachRound+t.OtherRound || star > t.PeachStar+t.OtherStar)
			if guaranteed && round+star < best {
				best = round + star
			}
		}
	}
	return best
}
func qualityCandyVariant() (prompt, expected string, err error) {
	counts := [6]int{}
	for i := range counts {
		n, e := rand.Int(rand.Reader, big.NewInt(12))
		if e != nil {
			return "", "", e
		}
		counts[i] = int(n.Int64()) + 3
	}
	t := qualityCandyTable{counts[0], counts[1], counts[2], counts[3], counts[4], counts[5]}
	answer := qualityCandyMinimum(t)
	prompt = fmt.Sprintf("袋中三种口味的糖果，各有圆形和星形，数量如下：\n             苹果 桃子 西瓜\n圆形          %d   %d   %d\n星形          %d   %d   %d\n你能凭触感分辨形状，但不能分辨口味。允许预先选定分别摸出多少颗圆形和多少颗星形，再在各自形状内盲取。问总共最少取出多少颗，才能保证至少有一对苹果和桃子口味的糖果且两颗形状不同？不使用工具。仅输出 <answer>整数</answer>。", t.AppleRound, t.PeachRound, t.OtherRound, t.AppleStar, t.PeachStar, t.OtherStar)
	return prompt, fmt.Sprint(answer), nil
}

func qualityResponseEvidence(raw json.RawMessage, answer string) json.RawMessage {
	e := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &e)
	}
	sum := sha256.Sum256([]byte(answer))
	e["response_sha256"] = hex.EncodeToString(sum[:])
	out, _ := json.Marshal(e)
	return out
}
