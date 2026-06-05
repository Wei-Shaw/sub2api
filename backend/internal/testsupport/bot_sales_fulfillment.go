package testsupport

import (
	"database/sql"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// NewBotSalesFulfillmentService wires BotSalesFulfillmentService with real
// repositories for integration-style tests outside handler/service packages.
func NewBotSalesFulfillmentService(client *dbent.Client, db *sql.DB) *service.BotSalesFulfillmentService {
	userRepo := repository.NewUserRepository(client, db)
	groupRepo := repository.NewGroupRepository(client, db)
	userSubRepo := repository.NewUserSubscriptionRepository(client)
	apiKeyRepo := repository.NewAPIKeyRepository(client, db)
	userDeviceRepo := repository.NewUserDeviceRepository(client)
	userSvc := service.NewUserService(userRepo, nil, nil, nil)
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo, userRepo, groupRepo, userSubRepo, nil, nil, &config.Config{Default: config.DefaultConfig{APIKeyPrefix: "sk-test-"}})
	subscriptionSvc := service.NewSubscriptionService(groupRepo, userSubRepo, nil, client, nil)
	return service.NewBotSalesFulfillmentService(client, userSvc, subscriptionSvc, apiKeySvc, userDeviceRepo)
}
