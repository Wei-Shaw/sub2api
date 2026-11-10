package meta

import (
	"bytes"
	"net/http"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	ClientVersion     = "1.4.14"
	ClientBuild       = "dec7d7497fecdad135ec5835a8161dbc92e55b58"
	ClientIDHeader    = "tbh:exec"
	contributorSuffix = "-contributor"
)

func UserAgentFor(goos, goarch string) string {
	switch goos {
	case "darwin":
		goos = "macos"
	}
	switch goarch {
	case "amd64":
		goarch = "x86_64"
	case "arm64":
		goarch = "aarch64"
	}
	return "muse-build/" + ClientVersion + " (non-interactive; " + goos + "-" + goarch + "; build " + ClientBuild + ")"
}

func UserAgent() string {
	return UserAgentFor(runtime.GOOS, runtime.GOARCH)
}

func NewSessionID() string {
	value, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return value.String()
}

func ApplyClientIdentity(header http.Header, sessionID string) {
	if header == nil {
		return
	}
	header.Set("User-Agent", UserAgent())
	header.Set("X-Client-Id", ClientIDHeader)
	if strings.TrimSpace(header.Get("X-Tbh-Session-Id")) != "" {
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = NewSessionID()
	}
	header.Set("X-Tbh-Session-Id", sessionID)
	header.Set("X-Meta-Ai-Gateway-Session-Id", sessionID)
	header.Set("X-Tbh-Thread-Id", sessionID)
	header.Set("X-Tbh-Root-Session-Id", sessionID)
	header.Set("X-Tbh-Agent-Role", "main")
}

func ApplyContributorMode(header http.Header, model string) {
	if header == nil || !IsContributorModel(model) {
		return
	}
	header.Set("X-Tbh-Contributor-Mode", "1")
}

func IsContributorModel(model string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(model)), contributorSuffix)
}

const museNamespace = `{"type":"namespace","name":"muse","description":"Muse Code tool set.","tools":[{"type":"function","name":"submit_reminder_decision","description":"Submit this reminder child's explicit decision.","parameters":{"additionalProperties":false,"properties":{"advisory_text":{"description":"Short text to deliver to the main agent when reminding.","type":["null","string"]},"confidence":{"description":"Confidence in the decision.","enum":["low","medium","high"],"type":["null","string"]},"decision":{"enum":["remind","none"],"type":"string"},"priority":{"description":"Requested reminder priority.","enum":["low","normal","high"],"type":["null","string"]},"reason":{"description":"Brief reason for reminding or staying silent","type":"string"},"skill_id":{"description":"The listed skill id to remind when needs_reminder is true.","type":["null","string"]},"visible_for_steps":{"description":"Requested ephemeral visibility length in main model steps.","maximum":8,"minimum":1,"type":["null","integer"]}},"required":["advisory_text","confidence","decision","priority","reason","skill_id","visible_for_steps"],"type":"object"},"strict":true}]}`

func EnsureMuseToolNamespace(body []byte) []byte {
	if len(body) == 0 || !bytes.Contains(body, []byte("\"tools\"")) {
		return body
	}
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() || len(tools.Array()) == 0 {
		return body
	}
	for _, tool := range tools.Array() {
		if tool.Get("type").String() == "namespace" && tool.Get("name").String() == "muse" {
			return body
		}
	}
	updated, err := sjson.SetRawBytes(body, "tools.-1", []byte(museNamespace))
	if err != nil {
		return body
	}
	return updated
}
