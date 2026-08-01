package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/api"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/config"
	profcrypto "github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/crypto"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/runtime"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/service"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

func main() {
	var (
		listen   = flag.String("listen", "", "control API listen addr (default env/config)")
		dataDir  = flag.String("data-dir", "", "state directory")
		runtimeN = flag.String("runtime", "", "mock|sing-box")
		token    = flag.String("token", "", "bearer token (empty = no auth)")
	)
	flag.Parse()

	cfg := config.LoadFromEnv()
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}
	if *runtimeN != "" {
		cfg.Runtime = *runtimeN
	}
	if *token != "" {
		cfg.Token = *token
	}
	// Local tests default to mock probe URL if unset custom
	if cfg.Runtime == "mock" {
		cfg.ProbeURL = "mock://local"
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := cfg.Validate(); err != nil {
		log.Error("invalid config", "err", err)
		os.Exit(1)
	}

	var cipher store.ProfileKeyCipher
	if secret := cfg.ProfileSecret(); secret != "" {
		c, err := profcrypto.NewProfileCipher(secret)
		if err != nil {
			log.Error("profile cipher", "err", err)
			os.Exit(1)
		}
		cipher = c
		log.Info("profile encryption enabled (AES-256-GCM at rest)")
	} else {
		log.Warn("profile encryption disabled: set WARP_GATEWAY_PROFILE_KEY or token to encrypt private keys at rest")
	}

	st, err := store.NewWithCipher(cfg.DataDir, cfg.PortRangeStart, cfg.PortRangeEnd, cipher)
	if err != nil {
		log.Error("store init", "err", err)
		os.Exit(1)
	}
	rt, err := runtime.New(cfg.Runtime, cfg.SingBoxPath, cfg.DataDir)
	if err != nil {
		log.Error("runtime init", "err", err)
		os.Exit(1)
	}
	mgr := service.NewManager(cfg, st, rt, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.RunBackground(ctx)

	srv := api.NewServer(mgr, cfg.Token)
	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.ClientCAFile != "" {
			pem, err := os.ReadFile(cfg.ClientCAFile)
			if err != nil {
				log.Error("read client CA", "err", err)
				os.Exit(1)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				log.Error("invalid client CA PEM")
				os.Exit(1)
			}
			tlsCfg.ClientCAs = pool
			tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
			log.Info("mTLS client auth enabled")
		}
		httpSrv.TLSConfig = tlsCfg
	}

	go func() {
		log.Info("warp-gateway listening",
			"addr", cfg.Listen,
			"runtime", cfg.Runtime,
			"data_dir", cfg.DataDir,
			"tls", cfg.TLSCertFile != "",
		)
		var err error
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			err = httpSrv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			err = httpSrv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			cancel()
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		log.Info("shutdown signal")
	case <-ctx.Done():
	}

	shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shCancel()
	_ = httpSrv.Shutdown(shCtx)
	mgr.Shutdown(shCtx)
	log.Info("bye")
}
