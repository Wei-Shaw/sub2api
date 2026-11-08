package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// publicWallRepoStub 是 IntelligenceCheckRunRepository 的最小测试桩：
// 嵌入接口，未实现的方法一旦被调用直接 panic —— 正好暴露「公开作品墙不该碰别的仓储方法」。
type publicWallRepoStub struct {
	service.IntelligenceCheckRunRepository

	runs []*service.IntelligenceCheckRun
}

func (r *publicWallRepoStub) ListLatestPerAccount(_ context.Context, _ int) ([]*service.IntelligenceCheckRun, error) {
	return r.runs, nil
}

// publicWallResponse 是一次公开作品墙请求解出来的三份视图：
// raw 用于断言「某个字符串根本没出现」，cards 用于断言字段取值，
// rawItems 用于断言「某个键根本不存在」—— 脱敏边界正是靠后两者钉住的。
type publicWallResponse struct {
	raw      string
	cards    []publicIntelligenceCheckCard
	rawItems []map[string]any
	total    int
}

func renderPublicWall(t *testing.T, runs []*service.IntelligenceCheckRun) publicWallResponse {
	t.Helper()

	gin.SetMode(gin.TestMode)
	svc := service.NewIntelligenceCheckService(nil, &publicWallRepoStub{runs: runs}, nil, nil, nil, nil, nil)
	h := NewIntelligenceCheckPublicHandler(svc)

	engine := gin.New()
	engine.GET("/runs", h.ListPublicRuns)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runs", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var typed struct {
		Code int `json:"code"`
		Data struct {
			Items []publicIntelligenceCheckCard `json:"items"`
			Total int                           `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &typed))
	require.Equal(t, 0, typed.Code)

	var loose struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &loose))

	return publicWallResponse{
		raw:      rec.Body.String(),
		cards:    typed.Data.Items,
		rawItems: loose.Data.Items,
		total:    typed.Data.Total,
	}
}

// publicWallRun 造一条「身份特征明显」的跑测记录：账号 id / 账号名 / 上游模型名
// 都取了不易与其它数字巧合的值，这样一旦它们出现在响应里就能被稳定抓到。
func publicWallRun(id int64) *service.IntelligenceCheckRun {
	finished := time.Date(2026, 10, 1, 9, 5, 0, 0, time.UTC)
	return &service.IntelligenceCheckRun{
		ID:              id,
		AccountID:       987654321,
		ModelID:         "glm-5.3-flashx",
		UpstreamModel:   "upstream-secret-alias-42",
		ReasoningEffort: "xhigh",
		Status:          service.IntelligenceCheckStatusCompleted,
		Verdict:         service.IntelligenceCheckVerdictUnknown,
		HasHTML:         true,
		HTMLBytes:       4096,
		LatencyMs:       242100,
		CreatedAt:       time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		FinishedAt:      &finished,
	}
}

// 模型名与智力等级是这面墙的看点，必须原样带出来（含 omitempty 不能把它们吃掉）。
func TestIntelligenceCheckPublicWallExposesModelAndEffort(t *testing.T) {
	res := renderPublicWall(t, []*service.IntelligenceCheckRun{publicWallRun(11)})

	require.Equal(t, 1, res.total)
	require.Len(t, res.cards, 1)

	card := res.cards[0]
	require.Equal(t, 1, card.Index, "展示序号从 1 开始")
	require.Equal(t, "glm-5.3-flashx", card.ModelID)
	require.Equal(t, "xhigh", card.ReasoningEffort)
	require.Equal(t, int64(242100), card.LatencyMs)
	require.True(t, card.HasArtifact)
	require.Equal(t, "/api/v1/intelligence-check/runs/11/artifact", card.ArtifactURL)
	require.Equal(t, "2026-10-01T09:00:00Z", card.CreatedAt)

	// 键名也要钉住：前端按 model_id / reasoning_effort 取值，改名即破。
	require.Contains(t, res.rawItems[0], "model_id")
	require.Contains(t, res.rawItems[0], "reasoning_effort")
}

// 脱敏边界：账号 id、账号名与上游标识永远不许出现在公开响应里。
// 模型名对用户开放是主人的明确要求，上游标识不是 —— 前者暴露「用的哪个模型」，
// 后者暴露「这个账号接的哪家上游、内部叫什么」，这条线必须一直在。
func TestIntelligenceCheckPublicWallOmitsAccountIdentity(t *testing.T) {
	res := renderPublicWall(t, []*service.IntelligenceCheckRun{publicWallRun(12)})

	require.Len(t, res.rawItems, 1)
	for _, key := range []string{
		"account_id", "account_name", "account", "upstream_model", "upstream", "name",
	} {
		require.NotContains(t, res.rawItems[0], key, "公开卡片不得出现 %q 字段", key)
	}

	require.NotContains(t, res.raw, "987654321", "公开响应不得带出账号 id")
	require.NotContains(t, res.raw, "upstream-secret-alias-42", "公开响应不得带出上游模型标识")
}

// 记录里没有模型信息时（跑测还没走到选模型就失败），字段整体不出现，
// 由前端决定「整行不渲染」，而不是后端编一个空壳值。
func TestIntelligenceCheckPublicWallOmitsMissingModelFields(t *testing.T) {
	run := publicWallRun(13)
	run.ModelID = ""
	run.ReasoningEffort = ""

	res := renderPublicWall(t, []*service.IntelligenceCheckRun{run})

	require.Len(t, res.rawItems, 1)
	require.NotContains(t, res.rawItems[0], "model_id")
	require.NotContains(t, res.rawItems[0], "reasoning_effort")
	require.Empty(t, res.cards[0].ModelID)
}

// nil 记录（理论上的空洞）要被跳过，且展示序号不能因此断号。
func TestIntelligenceCheckPublicWallSkipsNilRunsWithoutIndexGaps(t *testing.T) {
	res := renderPublicWall(t, []*service.IntelligenceCheckRun{
		publicWallRun(21), nil, publicWallRun(23),
	})

	require.Equal(t, 2, res.total)
	require.Len(t, res.cards, 2)
	require.Equal(t, 1, res.cards[0].Index)
	require.Equal(t, 2, res.cards[1].Index)
	require.Equal(t, "/api/v1/intelligence-check/runs/23/artifact", res.cards[1].ArtifactURL)
}
