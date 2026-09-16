//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitorrequesttemplate"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestApplyChannelMonitorTemplatePreservesDuplicateOperationMetadata(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()

	template, err := client.ChannelMonitorRequestTemplate.Create().
		SetName("duplicate-metadata-template").
		SetProvider(channelmonitorrequesttemplate.ProviderOpenai).
		SetAPIMode(service.MonitorAPIModeResponses).
		SetExtraHeaders(map[string]string{"User-Agent": "template-client"}).
		SetBodyOverrideMode(service.MonitorBodyOverrideModeOff).
		Save(ctx)
	require.NoError(t, err)

	monitor, err := client.ChannelMonitor.Create().
		SetName("duplicate-copy").
		SetProvider(channelmonitor.ProviderOpenai).
		SetAPIMode(service.MonitorAPIModeResponses).
		SetEndpoint("https://api.example.com").
		SetAPIKeyEncrypted("encrypted-key").
		SetPrimaryModel("gpt-5.4-mini").
		SetIntervalSeconds(60).
		SetCreatedBy(1).
		SetTemplateID(template.ID).
		SetExtraHeaders(map[string]string{
			"X-Original": "replaced",
			service.ChannelMonitorDuplicateOperationIDMetadataKey: "operation-digest",
		}).
		Save(ctx)
	require.NoError(t, err)

	repo := NewChannelMonitorRequestTemplateRepository(integrationEntClient, integrationDB)
	affected, err := repo.ApplyToMonitors(ctx, template.ID, []int64{monitor.ID})
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)

	stored, err := client.ChannelMonitor.Get(ctx, monitor.ID)
	require.NoError(t, err)
	require.Equal(t, "template-client", stored.ExtraHeaders["User-Agent"])
	require.NotContains(t, stored.ExtraHeaders, "X-Original")
	require.Equal(t, "operation-digest", stored.ExtraHeaders[service.ChannelMonitorDuplicateOperationIDMetadataKey])

	runtimeMonitor := entToServiceMonitor(stored)
	require.Equal(t, "operation-digest", runtimeMonitor.DuplicateOperationID)
	require.NotContains(t, runtimeMonitor.ExtraHeaders, service.ChannelMonitorDuplicateOperationIDMetadataKey)
}
