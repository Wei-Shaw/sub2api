package testsupport

import (
	"context"
	"database/sql"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func SetSettings(ctx context.Context, client *dbent.Client, values map[string]string) error {
	return repository.NewSettingRepository(client).SetMultiple(ctx, values)
}

// NewBotSalesFulfillmentService wires BotSalesFulfillmentService with real
// repositories for integration-style tests outside handler/service packages.
func NewBotSalesFulfillmentService(client *dbent.Client, db *sql.DB) *service.BotSalesFulfillmentService {
	userRepo := repository.NewUserRepository(client, db)
	groupRepo := repository.NewGroupRepository(client, db)
	settingRepo := repository.NewSettingRepository(client)
	userSubRepo := repository.NewUserSubscriptionRepository(client)
	apiKeyRepo := repository.NewAPIKeyRepository(client, db)
	userDeviceRepo := repository.NewUserDeviceRepository(client)
	cfg := &config.Config{Default: config.DefaultConfig{APIKeyPrefix: "sk-test-", UserConcurrency: 5}}
	settingSvc := service.NewSettingService(settingRepo, cfg)
	userSvc := service.NewUserService(userRepo, nil, nil, nil)
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo, userRepo, groupRepo, userSubRepo, nil, nil, cfg)
	subscriptionSvc := service.NewSubscriptionService(groupRepo, userSubRepo, nil, client, nil)
	return service.NewBotSalesFulfillmentService(client, userSvc, settingSvc, subscriptionSvc, apiKeySvc, userDeviceRepo)
}
