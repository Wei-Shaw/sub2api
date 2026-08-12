//go:build wireinject
// +build wireinject

package main

import (
	"log"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/runtime"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Lifecycle *runtime.Lifecycle
	Cleanup   func()
}

func initializeApplication(buildInfo handler.BuildInfo, role runtime.Role) (*Application, error) {
	wire.Build(
		// Infrastructure layer ProviderSets
		config.ProviderSet,

		// Business layer ProviderSets
		repository.ProviderSet,
		service.ProviderSet,
		securityaudit.ProviderSet,
		payment.ProviderSet,
		middleware.ProviderSet,
		handler.ProviderSet,

		// Server layer ProviderSet
		server.ProviderSet,

		// Privacy client factory for OpenAI training opt-out
		providePrivacyClientFactory,

		// BuildInfo provider
		provideServiceBuildInfo,

		provideLifecycle,
		newHTTPServerComponent,
		provideCleanup,

		// Application struct
		wire.Struct(new(Application), "Lifecycle", "Cleanup"),
	)
	return nil, nil
}

func providePrivacyClientFactory() service.PrivacyClientFactory {
	return repository.CreatePrivacyReqClient
}

func provideServiceBuildInfo(buildInfo handler.BuildInfo) service.BuildInfo {
	return service.BuildInfo{
		Version:   buildInfo.Version,
		BuildType: buildInfo.BuildType,
	}
}

func provideCleanup(entClient *ent.Client, rdb *redis.Client) func() {
	return func() {
		if rdb != nil {
			if err := rdb.Close(); err != nil {
				log.Printf("failed to close Redis client: %v", err)
			}
		}
		if entClient != nil {
			if err := entClient.Close(); err != nil {
				log.Printf("failed to close Ent client: %v", err)
			}
		}
	}
}
