//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/infrastructure"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Application struct {
	Server  *http.Server
	Cleanup func()
REDACTED

func initializeApplication(buildInfo handler.BuildInfo) (*Application, error) {
	wire.Build(
		// 基础设施层 ProviderSets
		config.ProviderSet,
		infrastructure.ProviderSet,

		// 业务层 ProviderSets
		repository.ProviderSet,
		service.ProviderSet,
		middleware.ProviderSet,
		handler.ProviderSet,

		// 服务器层 ProviderSet
		server.ProviderSet,

		// BuildInfo provider
		provideServiceBuildInfo,

		// 清理函数提供者
		provideCleanup,

		// 应用程序结构体
		wire.Struct(new(Application), "Server", "Cleanup"),
	)
	return nil, nil
REDACTED

func provideServiceBuildInfo(buildInfo handler.BuildInfo) service.BuildInfo {
	return service.BuildInfo{
		Version:   buildInfo.Version,
		BuildType: buildInfo.BuildType,
REDACTED
REDACTED

func provideCleanup(
	db *gorm.DB,
	rdb *redis.Client,
	tokenRefresh *service.TokenRefreshService,
	pricing *service.PricingService,
	emailQueue *service.EmailQueueService,
	oauth *service.OAuthService,
	openaiOAuth *service.OpenAIOAuthService,
) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Cleanup steps in reverse dependency order
		cleanupSteps := []struct {
			name string
			fn   func() error
	REDACTED{
			{"TokenRefreshService", func() error {
				tokenRefresh.Stop()
				return nil
	REDACTED
			{"PricingService", func() error {
				pricing.Stop()
				return nil
	REDACTED
			{"EmailQueueService", func() error {
				emailQueue.Stop()
				return nil
	REDACTED
			{"OAuthService", func() error {
				oauth.Stop()
				return nil
	REDACTED
			{"OpenAIOAuthService", func() error {
				openaiOAuth.Stop()
				return nil
	REDACTED
			{"Redis", func() error {
				return rdb.Close()
	REDACTED
			{"Database", func() error {
				sqlDB, err := db.DB()
				if err != nil {
					return err
			REDACTED
				return sqlDB.Close()
	REDACTED
	REDACTED

		for _, step := range cleanupSteps {
			if err := step.fn(); err != nil {
				log.Printf("[Cleanup] %s failed: %v", step.name, err)
				// Continue with remaining cleanup steps even if one fails
		REDACTED else {
				log.Printf("[Cleanup] %s succeeded", step.name)
		REDACTED
	REDACTED

		// Check if context timed out
		select {
		case <-ctx.Done():
			log.Printf("[Cleanup] Warning: cleanup timed out after 10 seconds")
		default:
			log.Printf("[Cleanup] All cleanup steps completed")
	REDACTED
REDACTED
REDACTED
