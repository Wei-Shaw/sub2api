//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBotSalesFulfillmentIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	email := fmt.Sprintf("bot-sales-idempotency-%d@example.com", time.Now().UnixNano())
	created := mustCreateUser(t, integrationEntClient, &service.User{
		Email:   email,
		Balance: 0,
	})

	users := NewUserRepository(integrationEntClient, integrationDB)
	devices := NewUserDeviceRepository(integrationEntClient)
	fulfillment := service.NewBotSalesFulfillmentService(integrationEntClient, users, devices, nil, nil, nil)
	input := service.BotSalesFulfillmentRequest{
		IdempotencyKey:      "bot-sales-idempotency-test",
		ExternalOrderCode:   "BS-IDEMPOTENCY-TEST",
		ExternalOrderItemID: "1",
		Buyer:               service.BotSalesBuyer{Email: email},
		BalanceAmount:       2.5,
		DeliveryPolicy:      service.BotSalesDeliveryPolicy{IssueAPIKey: "never"},
	}

	first, err := fulfillment.Fulfill(ctx, input)
	if err != nil {
		t.Fatalf("first fulfillment: %v", err)
	}
	second, err := fulfillment.Fulfill(ctx, input)
	if err != nil {
		t.Fatalf("replayed fulfillment: %v", err)
	}
	if !second.Replayed {
		t.Fatalf("replayed = false, want true")
	}
	if second.Balance != first.Balance || second.BalanceAdded != first.BalanceAdded {
		t.Fatalf("replayed response = %#v, want same balance response %#v", second, first)
	}

	got, err := integrationEntClient.User.Query().Where(user.IDEQ(created.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got.Balance != 2.5 {
		t.Fatalf("balance = %v, want 2.5 after replay", got.Balance)
	}
}

func TestBotSalesFulfillmentConcurrentSameUserAndGroupReusesAPIKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name:           fmt.Sprintf("bot-sales-concurrency-group-%d", suffix),
		Platform:       service.PlatformOpenAI,
		RateMultiplier: 1,
	})
	email := fmt.Sprintf("bot-sales-concurrency-%d@example.com", suffix)
	created := mustCreateUser(t, integrationEntClient, &service.User{
		Email:   email,
		Balance: 0,
	})

	users := NewUserRepository(integrationEntClient, integrationDB)
	devices := NewUserDeviceRepository(integrationEntClient)
	groups := NewGroupRepository(integrationEntClient, integrationDB)
	apiKeys := NewAPIKeyRepository(integrationEntClient, integrationDB)
	apiKeyService := service.NewAPIKeyService(apiKeys, users, groups, nil, nil, nil, &config.Config{})
	fulfillment := service.NewBotSalesFulfillmentService(integrationEntClient, users, devices, nil, nil, apiKeyService)

	groupID := group.ID
	start := make(chan struct{})
	results := make(chan struct {
		response *service.BotSalesFulfillmentResponse
		err      error
	}, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- func() (result struct {
				response *service.BotSalesFulfillmentResponse
				err      error
			}) {
				result.response, result.err = fulfillment.Fulfill(ctx, service.BotSalesFulfillmentRequest{
					IdempotencyKey:      fmt.Sprintf("bot-sales-concurrency-%d-%d", suffix, index),
					ExternalOrderCode:   fmt.Sprintf("BS-CONCURRENCY-%d-%d", suffix, index),
					ExternalOrderItemID: "1",
					Buyer:               service.BotSalesBuyer{Email: email},
					BalanceAmount:       1.25,
					BalancePackageCode:  "balance-concurrency",
					GroupID:             &groupID,
					Quantity:            1,
					DeliveryPolicy:      service.BotSalesDeliveryPolicy{IssueAPIKey: "always"},
				})
				return result
			}()
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var apiKeyID int64
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.response)
		require.NotNil(t, result.response.APIKey)
		require.Equal(t, group.ID, result.response.GroupID)
		require.Equal(t, group.ID, *result.response.APIKey.GroupID)
		if apiKeyID == 0 {
			apiKeyID = result.response.APIKey.ID
		} else {
			require.Equal(t, apiKeyID, result.response.APIKey.ID)
		}
	}

	count, err := integrationEntClient.APIKey.Query().
		Where(
			apikey.UserIDEQ(created.ID),
			apikey.GroupIDEQ(group.ID),
			apikey.StatusEQ(service.StatusActive),
			apikey.DeletedAtIsNil(),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count, "concurrent fulfillment must create one active key for the user/group pair")

	got, err := integrationEntClient.User.Query().Where(user.IDEQ(created.ID)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 2.5, got.Balance)
}
