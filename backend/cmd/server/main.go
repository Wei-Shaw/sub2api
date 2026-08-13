package main

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/runtime"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/setup"
	"github.com/Wei-Shaw/sub2api/internal/web"

	"github.com/gin-gonic/gin"
)

//go:embed VERSION
var embeddedVersion string

// Build-time variables (can be set by ldflags)
var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source" // "source" for manual builds, "release" for CI builds (set by ldflags)
)

func init() {
	// 如果 Version 已通过 ldflags 注入（例如 -X main.Version=...），则不要覆盖。
	if strings.TrimSpace(Version) != "" {
		return
	}

	// 默认从 embedded VERSION 文件读取版本号（编译期打包进二进制）。
	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
	}
}

// initLogger configures the default slog handler based on gin.Mode().
// In non-release mode, Debug level logs are enabled.
func main() {
	// Parse command line flags
	setupMode := flag.Bool("setup", false, "Run setup wizard in CLI mode")
	showVersion := flag.Bool("version", false, "Show version information")
	roleValue := ""
	flag.Func("role", "Runtime role: all, api, worker, scheduler, migrate, or bootstrap", func(value string) error {
		roleValue = value
		return nil
	})
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	roleSet := false
	flag.Visit(func(current *flag.Flag) {
		if current.Name == "role" {
			roleSet = true
		}
	})
	role, err := runtime.ResolveRole(roleSet, roleValue, os.LookupEnv)
	if err != nil {
		log.Fatalf("Invalid runtime role: %v", err)
	}
	logger.InitBootstrap()
	defer logger.Sync()

	if err := dispatchRole(role, *setupMode, roleLaunchers{
		runCLISetup:      setup.RunCLI,
		needsSetup:       setup.NeedsSetup,
		autoSetupEnabled: setup.AutoSetupEnabled,
		runAutoSetup:     setup.AutoSetupFromEnv,
		runSetupServer:   runSetupServer,
		runResident:      runMainServer,
		runMigrate:       runMigrations,
		runBootstrap:     runBootstrap,
	}); err != nil {
		log.Fatalf("Runtime role %q failed: %v", role, err)
	}
}

func runSetupServer() {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(config.CORSConfig{}))
	r.Use(middleware.SecurityHeaders(config.CSPConfig{Enabled: true, Policy: config.DefaultCSPPolicy}, nil))

	// Register setup routes
	setup.RegisterRoutes(r)

	// Serve embedded frontend if available
	if web.HasEmbeddedFrontend() {
		r.Use(web.ServeEmbeddedFrontend())
	}

	// Get server address from config.yaml or environment variables (SERVER_HOST, SERVER_PORT)
	// This allows users to run setup on a different address if needed
	addr := config.GetServerAddress()
	log.Printf("Setup wizard available at http://%s", addr)
	log.Println("Complete the setup wizard to configure Sub2API")

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
		Protocols:         protocols,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to start setup server: %v", err)
	}
}

func runMigrations() error {
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	_, db, err := repository.OpenEnt(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return repository.ApplyMigrations(ctx, db)
}

func runBootstrap() error {
	if !setup.AutoSetupEnabled() {
		return errors.New("bootstrap role requires environment-driven setup")
	}
	if err := setup.PrepareBootstrapFromEnv(); err != nil {
		return err
	}
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	client, db, err := repository.OpenEnt(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer client.Close()
	return repository.BootstrapInstallationWithFinalizer(context.Background(), db, client, cfg, setup.FinalizeBootstrapFromEnv)
}

func requireResidentBootstrap() residentAdmission {
	return func(ctx context.Context) error {
		cfg, err := config.LoadForBootstrap()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		client, db, err := repository.OpenEnt(cfg)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer client.Close()
		if err := repository.RequireMigrationsApplied(ctx, db); err != nil {
			return err
		}
		return repository.RequireBootstrapComplete(ctx, db)
	}
}

func runMainServer(role runtime.Role) error {
	if err := admitResidentRole(role, requireResidentBootstrap()); err != nil {
		return fmt.Errorf("admit resident role: %w", err)
	}
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := logger.Init(logger.OptionsFromConfig(cfg.Log)); err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	if cfg.RunMode == config.RunModeSimple {
		log.Println("⚠️  WARNING: Running in SIMPLE mode - billing and quota checks are DISABLED")
	}

	buildInfo := handler.BuildInfo{
		Version:   Version,
		BuildType: BuildType,
	}

	app, err := initializeApplication(buildInfo, role)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer app.Cleanup()
	if err := app.Lifecycle.Start(context.Background(), role); err != nil {
		return fmt.Errorf("start application lifecycle: %w", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Lifecycle.Stop(ctx); err != nil {
		log.Printf("Application shutdown failed: %v", err)
	}

	log.Println("Server exited")
	return nil
}
