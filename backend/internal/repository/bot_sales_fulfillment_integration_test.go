//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
	fulfillment := service.NewBotSalesFulfillmentService(integrationEntClient, users, devices, nil)
	input := service.BotSalesFulfillmentRequest{
		IdempotencyKey:      "bot-sales-idempotency-test",
		ExternalOrderCode:   "BS-IDEMPOTENCY-TEST",
		ExternalOrderItemID: "1",
		Buyer:               service.BotSalesBuyer{Email: email},
		BalanceAmount:       2.5,
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
