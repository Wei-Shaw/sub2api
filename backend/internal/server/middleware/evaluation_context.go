package middleware

import (
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	evaluationTokenHeaderName = "X-Sub2API-Evaluation-Token"
	evaluationHeaderPrefix    = "X-Sub2api-Evaluation-"
)

type evaluationContextBindingError struct {
	code    string
	message string
}

func evaluationContextSignerFromConfig(cfg *config.Config) *service.EvaluationContextSigner {
	if cfg == nil || !cfg.Radar.Enabled {
		return nil
	}
	signer, err := service.NewEvaluationContextSigner(
		[]byte(cfg.Radar.SigningSecret),
		time.Duration(cfg.Radar.MaxContextTTLSeconds)*time.Second,
	)
	if err != nil {
		return nil
	}
	return signer
}

func bindEvaluationContext(c *gin.Context, apiKey *service.APIKey, signer *service.EvaluationContextSigner, traceConfig service.RouteTraceConfig, now time.Time) *evaluationContextBindingError {
	if apiKey == nil {
		return &evaluationContextBindingError{code: "EVALUATION_CONTEXT_INVALID", message: "Evaluation context is invalid"}
	}

	if !apiKey.IsEvaluation {
		if hasEvaluationHeader(c.Request.Header) {
			return &evaluationContextBindingError{
				code:    "EVALUATION_CONTEXT_FORBIDDEN",
				message: "Evaluation headers are forbidden for this API key",
			}
		}
		return nil
	}

	token := strings.TrimSpace(c.GetHeader(evaluationTokenHeaderName))
	if token == "" {
		return &evaluationContextBindingError{
			code:    "EVALUATION_CONTEXT_REQUIRED",
			message: "A signed evaluation context is required for this API key",
		}
	}
	if signer == nil {
		return &evaluationContextBindingError{code: "EVALUATION_CONTEXT_INVALID", message: "Evaluation context is invalid"}
	}

	verified, err := signer.Verify(token, apiKey.ID, now)
	if err != nil {
		return &evaluationContextBindingError{code: "EVALUATION_CONTEXT_INVALID", message: "Evaluation context is invalid"}
	}
	verified.RouteTraceID = uuid.NewString()
	ctx := service.WithEvaluationContext(c.Request.Context(), verified)
	c.Request = c.Request.WithContext(service.WithRouteTrace(ctx, service.NewRouteTrace(verified, traceConfig)))
	return nil
}

func hasEvaluationHeader(header http.Header) bool {
	for name := range header {
		if strings.HasPrefix(textproto.CanonicalMIMEHeaderKey(name), evaluationHeaderPrefix) {
			return true
		}
	}
	return false
}

func abortWithEvaluationContextError(c *gin.Context, err *evaluationContextBindingError) {
	AbortWithError(c, http.StatusForbidden, err.code, err.message)
}

func abortWithGoogleEvaluationContextError(c *gin.Context, err *evaluationContextBindingError) {
	abortWithGoogleError(c, http.StatusForbidden, err.code+": "+err.message)
}
