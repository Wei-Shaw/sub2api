package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type promptAuditOrderCase struct {
	file       string
	function   string
	auditToken string
REDACTED

func TestPromptAuditGatePrecedesAccountBillingAndUpstreamSideEffects(t *testing.T) {
	tests := []promptAuditOrderCase{
		{file: "gateway_handler.go", function: "Messages", auditToken: "checkSecurityAudit"REDACTED,
		{file: "gateway_handler_chat_completions.go", function: "ChatCompletions", auditToken: "checkSecurityAudit"REDACTED,
		{file: "gateway_handler_responses.go", function: "Responses", auditToken: "checkSecurityAudit"REDACTED,
		{file: "gemini_v1beta_handler.go", function: "GeminiV1BetaModels", auditToken: "checkSecurityAudit"REDACTED,
		{file: "openai_gateway_handler.go", function: "Responses", auditToken: "checkSecurityAudit"REDACTED,
		{file: "openai_gateway_handler.go", function: "Messages", auditToken: "checkSecurityAudit"REDACTED,
		{file: "openai_chat_completions.go", function: "ChatCompletions", auditToken: "checkSecurityAudit"REDACTED,
		{file: "openai_images.go", function: "Images", auditToken: "checkSecurityAudit"REDACTED,
		{file: "grok_media.go", function: "handleGrokMedia", auditToken: "checkSecurityAudit"REDACTED,
		{file: "openai_embeddings.go", function: "Embeddings", auditToken: "checkSecurityAudit"REDACTED,
		{file: "openai_alpha_search.go", function: "AlphaSearch", auditToken: "checkSecurityAudit"REDACTED,
		{file: "image_task_handler.go", function: "Submit", auditToken: "checkSecurityAuditBeforeSubmit"REDACTED,
		{file: "batch_image_handler.go", function: "Submit", auditToken: "checkSecurityAuditBeforeSubmit"REDACTED,
REDACTED
	sideEffectTokens := []string{
		"CheckBillingEligibility(", "SelectAccount", ".Forward", "acquireResponsesUserSlot(",
		"AcquireUserSlot", "TryAcquireUserSlot", "acquireImageGenerationSlot(",
		"h.tasks.Create(", "h.service.Submit(",
REDACTED
	for _, tt := range tests {
		t.Run(tt.file+"/"+tt.function, func(t *testing.T) {
			functionSource := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			auditIndex := strings.Index(functionSource, tt.auditToken)
			require.NotEqual(t, -1, auditIndex, "missing Prompt Audit gate")
			foundSideEffect := false
			for _, sideEffect := range sideEffectTokens {
				index := strings.Index(functionSource, sideEffect)
				if index < 0 {
					continue
			REDACTED
				foundSideEffect = true
				require.Lessf(t, auditIndex, index, "%s must run before %s", tt.auditToken, sideEffect)
		REDACTED
			require.True(t, foundSideEffect, "coverage case must contain a downstream side effect")
	REDACTED)
REDACTED
REDACTED

func stripGoComments(source string) string {
	source = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(source, "")
	return regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(source, "")
REDACTED

func goFunctionSource(t *testing.T, filename, functionName string) string {
REDACTED
	raw, err := os.ReadFile(filename)
REDACTED
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, filename, raw, 0)
REDACTED
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
	REDACTED
		start := files.Position(function.Pos()).Offset
		end := files.Position(function.End()).Offset
		require.Greater(t, end, start)
		return string(raw[start:end])
REDACTED
	t.Fatalf("function %s not found in %s", functionName, filename)
	return ""
REDACTED
