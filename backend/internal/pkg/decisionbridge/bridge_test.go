package decisionbridge

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const dPredicate = `{"model":"gpt-6-luna","input":"hello","questions":[{"type":"predicate","name":"valid","instructions":"Is this valid?"}]}`
const sPredicate = `{"model":"jev-latest","state":"hello","questions":{"q":{"type":"noul","instructions":"Is this valid?"}}}`

func TestOneCaseBothDirections(t *testing.T) {
	s, e := ToSystemOne([]byte(dPredicate))
	require.NoError(t, e)
	require.Equal(t, "jev-latest", gjson.GetBytes(s, "model").String())
	require.Equal(t, "hello", gjson.GetBytes(s, "state").String())
	d, e := FromSystemOne([]byte(dPredicate), []byte(`{"model":"jev-1.13.0","answers":{"q0":{"type":"noul","noul":0.9}},"usage":{"input_tokens":4,"output_tokens":0}}`))
	require.NoError(t, e)
	require.Equal(t, "valid", gjson.GetBytes(d, "answers.0.name").String())
	require.Equal(t, 0.9, gjson.GetBytes(d, "answers.0.probability").Float())
	d, e = ToDecisions([]byte(sPredicate))
	require.NoError(t, e)
	require.Equal(t, "q", gjson.GetBytes(d, "questions.0.name").String())
	s, e = FromDecisions([]byte(sPredicate), d, []byte(`{"model":"gpt-6-luna","answers":[{"type":"predicate","probability":0.9}],"usage":{"input_tokens":4,"output_tokens":0}}`))
	require.NoError(t, e)
	require.Equal(t, 0.9, gjson.GetBytes(s, "answers.q.noul").Float())
}
func TestRejectNonrepresentable(t *testing.T) {
	for _, b := range []string{
		`{"model":"gpt-6-luna","input":"hi","questions":[{"type":"predicate","instructions":"x"}],"safety_identifier":"user"}`,
		`{"model":"gpt-6-luna","input":[{"role":"user","content":"hi"}],"questions":[{"type":"predicate","instructions":"x"}]}`,
	} {
		_, err := ToSystemOne([]byte(b))
		require.ErrorIs(t, err, ErrNonrepresentable)
	}
	for _, b := range []string{
		`{"model":"jev-latest","state":{"nested":"hi"},"questions":{"q":{"type":"noul","instructions":"x"}}}`,
		`{"model":"jev-latest","state":"hi","questions":{"q":{"type":"noul","instructions":"x","criteria":{"true":"yes"}}}}`,
		`{"model":"jev-latest","state":"hi","questions":{"q":{"type":"noul","instructions":"x"}},"unexpected":true}`,
	} {
		_, err := ToDecisions([]byte(b))
		require.ErrorIs(t, err, ErrNonrepresentable)
	}
	_, err := FromDecisions([]byte(sPredicate), []byte(dPredicate), []byte(`{"model":"gpt-6-luna","answers":[{"type":"refusal"}],"usage":{"input_tokens":1,"output_tokens":0}}`))
	require.ErrorIs(t, err, ErrNonrepresentable)
	_, err = FromSystemOne([]byte(dPredicate), []byte(`{"model":"jev-1.13.0","answers":{"q0":{"type":"noul","noul":0.9}},"usage":{"input_tokens":null,"output_tokens":0}}`))
	require.ErrorIs(t, err, ErrNonrepresentable)
	_, err = FromSystemOne([]byte(dPredicate), []byte(`{"model":"jev-1.13.0","answers":{"q0":{"type":"noul","noul":null}},"usage":{"input_tokens":1,"output_tokens":0}}`))
	require.ErrorIs(t, err, ErrNonrepresentable)
}
func TestOpenAIDecisionsChoiceScoreToSystemOne(t *testing.T) {
	req := []byte(`{"model":"gpt-6-luna","input":"hi","questions":[{"type":"choice","name":"route","instructions":"Which?","choices":[{"value":true,"description":"yes"},{"value":"no","description":"no"}]},{"type":"score","name":"severity","instructions":"How bad?","levels":[{"label":"low"},{"label":"high","description":"critical"}]}]}`)
	wire, err := ToSystemOne(req)
	require.NoError(t, err)
	require.Equal(t, "yes", gjson.GetBytes(wire, "questions.q0.criteria.c0").String())
	require.Equal(t, "high — critical", gjson.GetBytes(wire, "questions.q1.criteria.1").String())
	resp := []byte(`{"model":"jev-1.13.0","answers":{"q0":{"type":"choice","choice":"c0","confidence":0.9,"probabilities":{"c0":0.8,"c1":0.2}},"q1":{"type":"score","score":0.2,"confidence":0.8,"probabilities":{"0":0.8,"1":0.2},"legend":{"0":"low","1":"high — critical"}}},"usage":{"input_tokens":2,"output_tokens":0}}`)
	converted, err := FromSystemOne(req, resp)
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(converted, "answers.0.choice").Bool())
	require.Equal(t, 0.2, gjson.GetBytes(converted, "answers.1.score").Float())
	noDescription := []byte(`{"model":"gpt-6-luna","input":"hi","questions":[{"type":"choice","instructions":"Which?","choices":[{"value":true},{"value":false}]}]}`)
	_, err = ToSystemOne(noDescription)
	require.ErrorIs(t, err, ErrNonrepresentable, "unlabelled non-string options cannot be relabelled without losing their meaning")
}

func TestBridgeRejectsDroppedResponseFields(t *testing.T) {
	for _, raw := range []string{
		`{"model":"jev-1.13.0","answers":{"q0":{"type":"noul","noul":0.9,"confidence":0.2}},"usage":{"input_tokens":4,"output_tokens":0}}`,
		`{"model":"jev-1.13.0","answers":{"q0":{"type":"noul","noul":0.9}},"usage":{"input_tokens":4,"output_tokens":0},"trace":"secret"}`,
	} {
		_, err := FromSystemOne([]byte(dPredicate), []byte(raw))
		require.ErrorIs(t, err, ErrNonrepresentable)
	}
	for _, raw := range []string{
		`{"model":"gpt-6-luna","answers":[{"type":"predicate","name":"other","probability":0.9}],"usage":{"input_tokens":4,"output_tokens":0}}`,
		`{"model":"gpt-6-luna","answers":[{"type":"predicate","probability":0.9}],"usage":{"input_tokens":4,"output_tokens":0,"input_tokens_details":{"cached_tokens":1}}}`,
	} {
		_, err := FromDecisions([]byte(sPredicate), []byte(dPredicate), []byte(raw))
		require.ErrorIs(t, err, ErrNonrepresentable)
	}
}

func TestBridgeRejectsAmbiguousDuplicateKeys(t *testing.T) {
	_, err := ToSystemOne([]byte(`{"model":"gpt-6-luna","model":"gpt-6-luna","input":"hi","questions":[{"type":"predicate","instructions":"x"}]}`))
	require.ErrorIs(t, err, ErrNonrepresentable)
	_, err = ToDecisions([]byte(`{"model":"jev-latest","state":"hi","questions":{"q":{"type":"choice","instructions":"x","criteria":{"a":"first","a":"second"}}}}`))
	require.ErrorIs(t, err, ErrNonrepresentable)
	_, err = FromSystemOne([]byte(dPredicate), []byte(`{"model":"jev-1.13.0","answers":{"q0":{"type":"noul","noul":0.9,"noul":0.1}},"usage":{"input_tokens":1,"output_tokens":0}}`))
	require.ErrorIs(t, err, ErrNonrepresentable)
	choice := `{"model":"gpt-6-luna","input":"hi","questions":[{"type":"choice","instructions":"x","choices":[{"value":"yes"},{"value":"no"}]}]}`
	_, err = FromSystemOne([]byte(choice), []byte(`{"model":"jev-1.13.0","answers":{"q0":{"type":"choice","choice":"c0","confidence":0.9,"probabilities":{"c0":0.2,"c0":0.8,"c1":0.2}}},"usage":{"input_tokens":1,"output_tokens":0}}`))
	require.ErrorIs(t, err, ErrNonrepresentable)
}

func TestBridgeRejectsDroppedDistributionMetadata(t *testing.T) {
	original := []byte(`{"model":"jev-latest","state":"hi","questions":{"q":{"type":"choice","instructions":"Which?","criteria":{"yes":"Yes","no":"No"}}}}`)
	converted, err := ToDecisions(original)
	require.NoError(t, err)
	response := []byte(`{"model":"gpt-6-luna","answers":[{"type":"choice","choice":"yes","confidence":0.9,"probabilities":[{"value":"yes","probability":0.9,"note":"must-not-disappear"},{"value":"no","probability":0.1}]}],"usage":{"input_tokens":2,"output_tokens":0}}`)
	_, err = FromDecisions(original, converted, response)
	require.ErrorIs(t, err, ErrNonrepresentable)
}

func TestChoiceScoreRoundTrips(t *testing.T) {
	system := `{"model":"jev-latest","state":"hi","questions":{"c":{"type":"choice","instructions":"Which?","criteria":{"billing":"Billing","support":"Support"}},"s":{"type":"score","instructions":"How much?","criteria":["low","high"]}}}`
	d, e := ToDecisions([]byte(system))
	require.NoError(t, e)
	response := `{"model":"gpt-6-luna","answers":[{"type":"choice","choice":"billing","confidence":0.9,"probabilities":[{"value":"support","probability":0.1},{"value":"billing","probability":0.9}]},{"type":"score","score":0.2,"confidence":0.8,"probabilities":[{"value":1,"label":"high","probability":0.2},{"value":0,"label":"low","probability":0.8}]}],"usage":{"input_tokens":2,"output_tokens":0}}`
	s, e := FromDecisions([]byte(system), d, []byte(response))
	require.NoError(t, e)
	require.Equal(t, "billing", gjson.GetBytes(s, "answers.c.choice").String())
	require.Equal(t, 0.9, gjson.GetBytes(s, "answers.c.probabilities.billing").Float())
	require.Equal(t, 0.2, gjson.GetBytes(s, "answers.s.probabilities.1").Float())
	// The opposite direction deliberately uses generated q0/q1 and c0/c1
	// identifiers rather than exposing the System One ingress question keys.
	_, e = FromSystemOne(d, []byte(`{"model":"jev-1.13.0","answers":{"q0":{"type":"choice","choice":"invalid","confidence":0.9,"probabilities":{"c0":0.9,"c1":0.1}}},"usage":{"input_tokens":1,"output_tokens":0}}`))
	require.True(t, errors.Is(e, ErrNonrepresentable))
}

func TestBridgePreservesScoreProbabilitiesWithIntegralFloatValues(t *testing.T) {
	system := `{"model":"jev-latest","state":"hi","questions":{"s":{"type":"score","instructions":"How much?","criteria":["low","high"]}}}`
	decisionsRequest, err := ToDecisions([]byte(system))
	require.NoError(t, err)
	response := []byte(`{"model":"gpt-6-luna","answers":[{"type":"score","score":0.2,"confidence":0.8,"probabilities":[{"value":0.0,"label":"low","probability":0.8},{"value":1.0,"label":"high","probability":0.2}]}],"usage":{"input_tokens":2,"output_tokens":0}}`)
	converted, err := FromDecisions([]byte(system), decisionsRequest, response)
	require.NoError(t, err)
	require.Equal(t, 0.8, gjson.GetBytes(converted, "answers.s.probabilities.0").Float())
	require.Equal(t, 0.2, gjson.GetBytes(converted, "answers.s.probabilities.1").Float())
}

func TestBridgePreservesUndescribedStringChoiceLabels(t *testing.T) {
	req := []byte(`{"model":"gpt-6-luna","input":"hi","questions":[{"type":"choice","instructions":"Pick a fruit","choices":[{"value":"apple"},{"value":"banana"}]}]}`)
	wire, err := ToSystemOne(req)
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(wire, "questions.q0.criteria.apple").Exists())
	require.True(t, gjson.GetBytes(wire, "questions.q0.criteria.banana").Exists())
	converted, err := FromSystemOne(req, []byte(`{"model":"jev-1.13.0","answers":{"q0":{"type":"choice","choice":"apple","confidence":0.9,"probabilities":{"apple":0.8,"banana":0.2}}},"usage":{"input_tokens":4,"output_tokens":0}}`))
	require.NoError(t, err)
	require.Equal(t, "apple", gjson.GetBytes(converted, "answers.0.choice").String())
	_, err = ToSystemOne([]byte(`{"model":"gpt-6-luna","input":"hi","questions":[{"type":"choice","instructions":"Pick","choices":[{"value":true},{"value":"banana"}]}]}`))
	require.ErrorIs(t, err, ErrNonrepresentable)
}

func TestBridgeAcceptsRedundantZeroUsageDetails(t *testing.T) {
	request := []byte(`{"model":"jev-latest","state":"hi","questions":{"q":{"type":"noul","instructions":"x"}}}`)
	converted, err := ToDecisions(request)
	require.NoError(t, err)
	response := []byte(`{"model":"gpt-6-luna","answers":[{"type":"predicate","name":"q","probability":0.9}],"usage":{"input_tokens":4,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens":0,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":4}}`)
	_, err = FromDecisions(request, converted, response)
	require.NoError(t, err)
	response = []byte(`{"model":"gpt-6-luna","answers":[{"type":"predicate","name":"q","probability":0.9}],"usage":{"input_tokens":4,"input_tokens_details":{"cached_tokens":1},"output_tokens":0,"total_tokens":4}}`)
	_, err = FromDecisions(request, converted, response)
	require.ErrorIs(t, err, ErrNonrepresentable)
}
