package devin

import (
	"context"
	"net/http"
	"testing"
)

func TestFusionIsNotAdvertisedAsStandaloneOrAdaptive(t *testing.T) {
	var catalog []byte
	for _, config := range [][]byte{
		modelFixture("standalone-lead", "Lead", false, nil),
		modelFixture("adaptive", "Adaptive", false, pv(nil, 22, 3)),
		modelFixture("fusion", "Fusion", false, ps(pv(nil, 25, 1), 20, "native-harness")),
		modelFixture("fusion-standalone-lead-sidekick-swe-2", "Fusion pairing", false, ps(pv(nil, 22, 3), 20, "native-harness")),
	} {
		catalog = pb(catalog, 1, config)
	}
	client := &Client{Do: func(*http.Request) (*http.Response, error) { return responseBytes(200, catalog), nil }}
	models, err := client.Models(context.Background(), "synthetic-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "adaptive" || !models[0].Router || models[1].ID != "standalone-lead" {
		t.Fatal("unimplemented Fusion pairing was advertised, or ordinary models/router changed")
	}
}

func TestFusionFailsExplicitlyBeforeAuthAssignmentOrChat(t *testing.T) {
	client := &Client{Do: func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported Fusion request must not consume upstream usage or reach AssignModel")
		return nil, nil
	}}
	for _, model := range []Model{
		{ID: "fusion", Router: true},
		{ID: "fusion-standalone-lead-sidekick-swe-2", UID: "standalone-lead"},
		{ID: "alias", UID: "fusion-sidekick-swe-2"},
	} {
		_, err := client.Chat(context.Background(), "synthetic-token", "cascade", model, []byte(simpleBody))
		got := assertStatus(t, err, 400)
		if got.Message != "Devin Fusion lead/sidekick orchestration is not supported by this adapter; select a standalone model" {
			t.Fatal("Fusion limitation was not explicit")
		}
	}
}
