package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchOpenAIAccountModelsFillsMissingDisplayNames(t *testing.T) {
	gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return ordinaryModelsUpstreamResponse(`{"data":[{"id":"custom-one"},{"id":"custom-two","display_name":""},{"id":"custom-three","display_name":"  "},{"id":"custom-four","display_name":"Friendly name"}]}`), nil
	}})
	svc := &AccountTestService{openaiGatewayService: gateway}
	account := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	require.Len(t, models, 4)
	for i, name := range []string{"custom-one", "custom-two", "custom-three", "Friendly name"} {
		require.Equal(t, name, models[i].DisplayName)
	}
	require.Equal(t, "custom-one", models[0].ID)
	require.Equal(t, "custom-four", models[3].ID)
	response, err := gateway.FetchOpenAIModelsList(context.Background(), account)
	require.NoError(t, err)
	require.Contains(t, string(response.Body), `"display_name":""`, "picker labels must not rewrite the shared upstream catalog")
}
