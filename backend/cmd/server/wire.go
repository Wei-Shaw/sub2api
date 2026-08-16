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

func initializeAllApplication(buildInfo handler.BuildInfo) (*Application, error) {
	wire.Build(allApplicationProviderSet, wire.Value(runtime.RoleAll))
	return nil, nil
}

func initializeAPIApplication(buildInfo handler.BuildInfo) (*Application, error) {
	wire.Build(apiApplicationProviderSet, wire.Value(runtime.RoleAPI))
	return nil, nil
}

func initializeWorkerApplication(buildInfo handler.BuildInfo) (*Application, error) {
	wire.Build(workerApplicationProviderSet, wire.Value(runtime.RoleWorker))
	return nil, nil
}

func initializeSchedulerApplication(buildInfo handler.BuildInfo) (*Application, error) {
	wire.Build(schedulerApplicationProviderSet, wire.Value(runtime.RoleScheduler))
	return nil, nil
}

var infrastructureProviderSet = wire.NewSet(
	config.ProviderSet,
	repository.ProviderSet,
	provideCleanup,
)

var allApplicationProviderSet = wire.NewSet(
	infrastructureProviderSet,
	service.ProviderSet,
	securityaudit.ProviderSet,
	payment.ProviderSet,
	middleware.ProviderSet,
	handler.ProviderSet,
	server.ProviderSet,
	providePrivacyClientFactory,
	provideServiceBuildInfo,
	provideAllLifecycle,
	newHTTPServerComponent,
	wire.Struct(new(Application), "Lifecycle", "Cleanup"),
)

var apiApplicationProviderSet = wire.NewSet(
	infrastructureProviderSet,
	service.ProviderSet,
	securityaudit.ProviderSet,
	payment.ProviderSet,
	middleware.ProviderSet,
	handler.ProviderSet,
	server.ProviderSet,
	providePrivacyClientFactory,
	provideServiceBuildInfo,
	provideAPILifecycle,
	newHTTPServerComponent,
	wire.Struct(new(Application), "Lifecycle", "Cleanup"),
)

var workerApplicationProviderSet = wire.NewSet(
	infrastructureProviderSet,
	service.ProviderSet,
	securityaudit.ProviderSet,
	providePrivacyClientFactory,
	provideWorkerLifecycle,
	wire.Struct(new(Application), "Lifecycle", "Cleanup"),
)

var schedulerApplicationProviderSet = wire.NewSet(
	infrastructureProviderSet,
	service.ProviderSet,
	payment.ProviderSet,
	providePrivacyClientFactory,
	provideServiceBuildInfo,
	provideSchedulerLifecycle,
	wire.Struct(new(Application), "Lifecycle", "Cleanup"),
)

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
