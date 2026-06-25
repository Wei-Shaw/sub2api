//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestOpenAIAccountSelectionErrorResponse_DefaultWithoutRule(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	h := &OpenAIGatewayHandler{}

	status, errType, message := h.openAIAccountSelectionErrorResponse(c, nil, service.OpenAIAccountSelectionNoAvailableMessage, "Service temporarily unavailable")

	if status != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d", status, http.StatusServiceUnavailable)
	}
	if errType != "api_error" {
		t.Fatalf("errType=%s want api_error", errType)
	}
	if message != "Service temporarily unavailable" {
		t.Fatalf("message=%q", message)
	}
}

func TestGatewayAccountSelectionErrorResponse_CustomRule(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	responseCode := http.StatusTooManyRequests
	customMessage := "账户额度已用完"
	svc := service.NewErrorPassthroughService(&accountSelectionErrorRuleRepo{rules: []*model.ErrorPassthroughRule{
		{
			ID:              1,
			Name:            "OpenAI no available accounts",
			Enabled:         true,
			Priority:        0,
			ErrorCodes:      []int{service.OpenAIAccountSelectionNoAvailableStatus},
			Keywords:        []string{service.OpenAIAccountSelectionNoAvailableMessage},
			MatchMode:       model.MatchModeAll,
			Platforms:       []string{model.PlatformOpenAI},
			PassthroughCode: false,
			ResponseCode:    &responseCode,
			PassthroughBody: false,
			CustomMessage:   &customMessage,
		},
	}}, nil)
	h := &GatewayHandler{errorPassthroughService: svc}

	status, errType, message := h.openAIAccountSelectionErrorResponse(c, service.PlatformGemini, nil, service.OpenAIAccountSelectionNoAvailableMessage, "No available accounts: no available accounts")

	if status != http.StatusTooManyRequests {
		t.Fatalf("status=%d want %d", status, http.StatusTooManyRequests)
	}
	if errType != "upstream_error" {
		t.Fatalf("errType=%s want upstream_error", errType)
	}
	if message != customMessage {
		t.Fatalf("message=%q want %q", message, customMessage)
	}
}

type accountSelectionErrorRuleRepo struct {
	rules []*model.ErrorPassthroughRule
}

func (r *accountSelectionErrorRuleRepo) List(context.Context) ([]*model.ErrorPassthroughRule, error) {
	return r.rules, nil
}

func (r *accountSelectionErrorRuleRepo) GetByID(context.Context, int64) (*model.ErrorPassthroughRule, error) {
	panic("unexpected GetByID")
}

func (r *accountSelectionErrorRuleRepo) Create(context.Context, *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	panic("unexpected Create")
}

func (r *accountSelectionErrorRuleRepo) Update(context.Context, *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	panic("unexpected Update")
}

func (r *accountSelectionErrorRuleRepo) Delete(context.Context, int64) error {
	panic("unexpected Delete")
}
