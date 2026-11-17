package securityaudit

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	ProtocolOpenAICompatible = "openai_compatible"
	ProtocolOpenAIDecisions  = "openai_decisions"

	DefaultDecisionsModel    = "gpt-6-luna"
	DefaultCategoryThreshold = 0.5
	// The Decisions API caps an input_json part at 256 KiB of compact JSON.
	// JSON escaping can expand one rune to six bytes (\u00XX), so 40000 runes
	// stay below the cap for any text.
	MaxDecisionsInputLimit = 40000

	decisionsScannerBackend = "openai-decisions"
	// DecisionsRefusalCategory marks a result where the Decisions model refused
	// at least one question; such a result always blocks.
	DecisionsRefusalCategory = "decisions_refusal"
)

// decisions_policy/ holds the fixed audit policy: rules.txt is the developer
// prompt and questions.json the typed questions. Their combined SHA-256 names the
// policy in ScannerVersion and in the shared prompt cache key. rules.txt is not
// a .md file because .dockerignore drops *.md from the image build context.
var (
	//go:embed decisions_policy/rules.txt
	decisionsRules []byte
	//go:embed decisions_policy/questions.json
	decisionsQuestionsJSON []byte
)

type decisionsChoice struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

type decisionsQuestion struct {
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Instructions string            `json:"instructions"`
	Choices      []decisionsChoice `json:"choices,omitempty"`
}

type decisionsPolicy struct {
	ID         string
	Version    int
	Digest     string
	Rules      string
	LevelName  string
	Levels     []string
	Categories []string
	Questions  []decisionsQuestion
	CacheKey   string
}

var promptDecisionsPolicy = mustLoadDecisionsPolicy(decisionsRules, decisionsQuestionsJSON)

func mustLoadDecisionsPolicy(rules, questionsJSON []byte) *decisionsPolicy {
	policy, err := loadDecisionsPolicy(rules, questionsJSON)
	if err != nil {
		panic(fmt.Sprintf("prompt audit decisions policy: %v", err))
	}
	return policy
}

func loadDecisionsPolicy(rules, questionsJSON []byte) (*decisionsPolicy, error) {
	var spec struct {
		PolicyID      string `json:"policy_id"`
		PolicyVersion int    `json:"policy_version"`
		LevelQuestion struct {
			Name         string            `json:"name"`
			Instructions string            `json:"instructions"`
			Choices      []decisionsChoice `json:"choices"`
		} `json:"level_question"`
		CategoryQuestionTemplate string `json:"category_question_template"`
		Categories               []struct {
			ID string `json:"id"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(questionsJSON, &spec); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bytes.Join([][]byte{rules, questionsJSON}, []byte{0}))
	digestHex := hex.EncodeToString(digest[:])
	policy := &decisionsPolicy{
		ID: spec.PolicyID, Version: spec.PolicyVersion, Digest: digestHex, Rules: string(rules),
		LevelName: spec.LevelQuestion.Name,
		// Shared by every caller: the cacheable prefix holds only the rules text.
		CacheKey: "prompt-audit-" + digestHex[:32],
	}
	policy.Questions = append(policy.Questions, decisionsQuestion{
		Type: "choice", Name: spec.LevelQuestion.Name, Instructions: spec.LevelQuestion.Instructions,
		Choices: spec.LevelQuestion.Choices,
	})
	for _, choice := range spec.LevelQuestion.Choices {
		policy.Levels = append(policy.Levels, choice.Value)
	}
	names := map[string]struct{}{spec.LevelQuestion.Name: {}}
	for _, category := range spec.Categories {
		if _, ok := ScannerCatalog[category.ID]; !ok {
			return nil, fmt.Errorf("category %q is not a scanner id", category.ID)
		}
		if _, dup := names[category.ID]; dup {
			return nil, fmt.Errorf("question name %q repeated", category.ID)
		}
		names[category.ID] = struct{}{}
		policy.Categories = append(policy.Categories, category.ID)
		policy.Questions = append(policy.Questions, decisionsQuestion{
			Type: "predicate", Name: category.ID,
			Instructions: strings.ReplaceAll(spec.CategoryQuestionTemplate, "{id}", category.ID),
		})
	}
	if len(policy.Categories) != len(AllScannerIDs) {
		return nil, fmt.Errorf("policy covers %d of %d categories", len(policy.Categories), len(AllScannerIDs))
	}
	return policy, nil
}

func (p *decisionsPolicy) requestBody(model, text string) ([]byte, error) {
	payload := map[string]any{
		"model": model,
		"input": []any{
			map[string]any{
				"role": "developer",
				"content": []any{map[string]any{
					"type": "input_text", "text": p.Rules,
					"prompt_cache_breakpoint": map[string]string{"mode": "explicit"},
				}},
			},
			// The audited text sits in a data position so it cannot act as instructions.
			map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "input_json", "data": map[string]any{"audit_target": map[string]string{"text": text}}}},
			},
		},
		"questions":            p.Questions,
		"prompt_cache_key":     p.CacheKey,
		"prompt_cache_options": map[string]string{"mode": "explicit"},
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}

type decisionsAnswer struct {
	Type          string           `json:"type"`
	Name          string           `json:"name"`
	Choice        *string          `json:"choice"`
	Probability   *float64         `json:"probability"`
	Probabilities []map[string]any `json:"probabilities"`
}

type decisionsAnswers struct {
	Safety string
	// Categories holds the probability of every answered category question.
	Categories map[string]float64
	// Refused lists questions the model refused, in policy order.
	Refused []string
}

// parseDecisionsResponse validates every answer. A missing or repeated answer
// or a probability outside [0, 1] is an invalid response. A refusal is a valid
// answer that the caller turns into a block; it is never read as Safe.
func (p *decisionsPolicy) parseDecisionsResponse(body []byte) (*decisionsAnswers, error) {
	var response struct {
		Answers []decisionsAnswer `json:"answers"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Answers == nil {
		return nil, errors.New("decisions response has no answers array")
	}
	expected := make(map[string]string, len(p.Questions))
	for _, question := range p.Questions {
		expected[question.Name] = question.Type
	}
	answers := make(map[string]decisionsAnswer, len(response.Answers))
	for _, answer := range response.Answers {
		questionType, ok := expected[answer.Name]
		if !ok {
			return nil, fmt.Errorf("unexpected answer %q", answer.Name)
		}
		if _, dup := answers[answer.Name]; dup {
			return nil, fmt.Errorf("answer %q repeated", answer.Name)
		}
		if answer.Type != questionType && answer.Type != "refusal" {
			return nil, fmt.Errorf("question %q answered with type %q", answer.Name, answer.Type)
		}
		answers[answer.Name] = answer
	}
	if len(answers) != len(expected) {
		return nil, fmt.Errorf("%d of %d questions answered", len(answers), len(expected))
	}
	result := &decisionsAnswers{Categories: make(map[string]float64, len(p.Categories))}
	for _, question := range p.Questions {
		if answers[question.Name].Type == "refusal" {
			result.Refused = append(result.Refused, question.Name)
		}
	}
	if level := answers[p.LevelName]; level.Type != "refusal" {
		if level.Choice == nil || !containsString(p.Levels, *level.Choice) {
			return nil, errors.New("safety level choice is not a policy level")
		}
		result.Safety = *level.Choice
	}
	for _, category := range p.Categories {
		if answers[category].Type == "refusal" {
			continue
		}
		probability := answers[category].Probability
		if probability == nil || math.IsNaN(*probability) || *probability < 0 || *probability > 1 {
			return nil, fmt.Errorf("category %q probability invalid", category)
		}
		result.Categories[category] = *probability
	}
	return result, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func DecisionsURL(base string) (string, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return "", err
	}
	return normalized + "/v1/decisions", nil
}

func (s *OpenAICompatibleScanner) scanDecisions(ctx context.Context, endpoint ActiveEndpoint, chunk string, enabledScanners []string) (*NormalizedResult, error) {
	requestURL, err := DecisionsURL(endpoint.BaseURL)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Cause: err}
	}
	policy := promptDecisionsPolicy
	body, err := policy.requestBody(endpoint.Model, chunk)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: err}
	}
	responseBody, err := s.postJSON(ctx, endpoint, requestURL, body)
	if err != nil {
		return nil, err
	}
	answers, err := policy.parseDecisionsResponse(responseBody)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: err}
	}
	threshold := endpoint.CategoryThreshold
	if threshold <= 0 {
		threshold = DefaultCategoryThreshold
	}
	result := decisionsVerdict(answers, threshold, enabledScanners)
	result.ScannerBackend = decisionsScannerBackend
	result.ScannerVersion = endpoint.Model + "@" + policy.Digest[:12]
	result.PolicyID = policy.ID
	result.PolicyVersion = policy.Version
	result.GuardEndpointID = endpoint.ID
	return result, nil
}

// decisionsVerdict applies the shared verdict rules to the answered questions.
// Any refusal blocks the request: the model refuses mostly on egregious
// content, so letting the request through would defeat the audit.
func decisionsVerdict(answers *decisionsAnswers, threshold float64, enabledScanners []string) *NormalizedResult {
	known := map[string]struct{}{}
	for category, probability := range answers.Categories {
		if probability >= threshold {
			known[category] = struct{}{}
		}
	}
	score := func(category string) float64 { return answers.Categories[category] }
	if len(answers.Refused) == 0 {
		return classifyGuardVerdict(answers.Safety, orderedScannerKeys(known), nil, enabledScanners, score)
	}
	result := classifyGuardVerdict("Unsafe", orderedScannerKeys(known), nil, enabledScanners, score)
	result.Categories = append(result.Categories, DecisionsRefusalCategory)
	result.ScannerScores[DecisionsRefusalCategory] = 1
	result.ScannerEvidence[DecisionsRefusalCategory] = "refused: " + strings.Join(answers.Refused, ", ")
	result.Decision, result.RiskLevel, result.Action = EventCritical, RiskCritical, ActionBlock
	return result
}
