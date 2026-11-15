package chatgptwebimage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha3"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/html"
)

// authenticate uses the ordinary web requirements/proof exchange. It does not
// call challenge solvers, fabricate challenge tokens, or retry a rejected POST.
// Turnstile's advisory flag has appeared in accepted normal exchanges, so the
// server decides whether the supplied requirements/proof are sufficient.
func (s *session) authenticate(ctx context.Context) error {
	r, err := s.request(ctx, http.MethodGet, "/", nil, nil)
	if err != nil {
		return s.failure("bootstrap", "transport", 0, err)
	}
	if r.StatusCode != http.StatusOK {
		_ = r.Body.Close()
		return s.status("bootstrap", r)
	}
	root, err := readLimited(r.Body, maxJSONBytes)
	_ = r.Body.Close()
	if err != nil {
		return s.failure("bootstrap", "invalid_response", 200, err)
	}
	config := proofConfig(root)
	seed, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 53))
	if err != nil {
		return s.failure("requirements", "entropy", 0, err)
	}
	initial, err := solveProof(ctx, "0."+seed.String(), "0fffff", config)
	if err != nil {
		return s.failure("requirements", "proof_failed", 0, err)
	}
	var requirements struct {
		Token  string `json:"token"`
		Arkose struct {
			Required bool `json:"required"`
		} `json:"arkose"`
		Proof struct {
			Required   bool   `json:"required"`
			Seed       string `json:"seed"`
			Difficulty string `json:"difficulty"`
		} `json:"proofofwork"`
	}
	if err := s.jsonRequest(ctx, "requirements", http.MethodPost, "/backend-api/sentinel/chat-requirements", map[string]string{"p": "gAAAAAC" + initial}, &requirements); err != nil {
		return err
	}
	if requirements.Arkose.Required {
		return s.failure("requirements", "interactive_challenge", 200, nil)
	}
	if requirements.Token == "" {
		return s.failure("requirements", "missing_requirements_token", 200, nil)
	}
	s.headers.Set("Openai-Sentinel-Chat-Requirements-Token", requirements.Token)
	if requirements.Proof.Required {
		answer, err := solveProof(ctx, requirements.Proof.Seed, requirements.Proof.Difficulty, config)
		if err != nil {
			return s.failure("requirements", "proof_failed", 200, err)
		}
		s.headers.Set("Openai-Sentinel-Proof-Token", "gAAAAAB"+answer)
	}
	return nil
}

var deploymentPattern = regexp.MustCompile(`c/[^/]*/_`)

func proofConfig(root []byte) []any {
	var script, deployment string
	tokens := html.NewTokenizer(bytes.NewReader(root))
	for {
		tokenType := tokens.Next()
		if tokenType == html.ErrorToken {
			break
		}
		if tokenType != html.StartTagToken {
			continue
		}
		tag, hasAttr := tokens.TagName()
		tagName := string(tag)
		for hasAttr {
			key, value, more := tokens.TagAttr()
			hasAttr = more
			if tagName == "html" && string(key) == "data-build" && deployment == "" {
				deployment = string(value)
			}
			if tagName == "script" && string(key) == "src" {
				if script == "" {
					script = string(value)
				}
				if match := deploymentPattern.FindString(string(value)); match != "" {
					deployment = match
				}
			}
		}
	}
	if script == "" {
		script = defaultBaseURL + "/backend-api/sentinel/sdk.js"
	}
	now := time.Now()
	date := now.In(time.FixedZone("EST", -5*60*60)).Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)"
	return []any{3000, date, 4294705152, 0, userAgent, script, deployment, "en-US", "en-US,es-US,en,es", 0,
		"webdriver−false", "location", "window", 1000, uuid.NewString(), "", 8, float64(now.UnixMilli()) - 1000}
}

// solveProof mirrors the web token's SHA3-512 nonce format with bounded work.
// Difficulty is a hexadecimal prefix threshold, not a number of zero bits.
func solveProof(ctx context.Context, seed, difficulty string, config []any) (string, error) {
	if len(seed) > 4096 || len(difficulty) == 0 || len(difficulty) > 128 || len(config) != 18 {
		return "", errors.New("invalid proof challenge")
	}
	target, err := hex.DecodeString(difficulty)
	if err != nil || len(target) == 0 {
		return "", errors.New("invalid proof difficulty")
	}
	// Serialize static fields once. The nonce and its shifted value occupy
	// positions 3 and 9; avoiding JSON encoding in the loop bounds CPU overhead.
	encode := func(v any) []byte { b, _ := json.Marshal(v); return b }
	left := strings.TrimSuffix(string(encode(config[:3])), "]") + ","
	middle := "," + strings.Trim(string(encode(config[4:9])), "[]") + ","
	right := "," + strings.TrimPrefix(string(encode(config[10:])), "[")
	for nonce := 0; nonce < 500000; nonce++ {
		if nonce%256 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		configJSON := left + strconv.Itoa(nonce) + middle + strconv.Itoa(nonce>>1) + right
		answer := base64.StdEncoding.EncodeToString([]byte(configJSON))
		hash := sha3.Sum512([]byte(seed + answer))
		// The historical protocol compares a full hash slice of len(hex text)
		// bytes against the decoded threshold. Equality of the shorter prefix
		// thus fails unless both lengths are equal (not normally possible).
		n := len(difficulty)
		if n > len(hash) {
			n = len(hash)
		}
		if bytes.Compare(hash[:n], target) <= 0 {
			return answer, nil
		}
	}
	return "", errors.New("proof work limit exceeded")
}
