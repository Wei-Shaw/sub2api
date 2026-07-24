package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/deployer"
)

func main() {
	configPath := flag.String("config", "/etc/sub2api-deployer/config.json", "path to deployer JSON configuration")
	check := flag.Bool("check", false, "validate configuration and exit")
	reconcileSlot := flag.String("reconcile-slot", "", "verify and clear a degraded latch by selecting the serving deployment slot")
	forceUnobservableDrain := flag.Bool("force-unobservable-drain", false, "during reconciliation, explicitly allow stopping a legacy container that cannot report drain blockers")
	flag.Parse()

	cfg, err := deployer.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}
	if *check {
		if *reconcileSlot != "" || *forceUnobservableDrain {
			log.Fatal("--check cannot be combined with reconciliation options")
		}
		log.Printf("configuration is valid")
		return
	}
	processLock, err := deployer.AcquireProcessLock(cfg.StatePath)
	if err != nil {
		log.Fatalf("acquire deployer process lock: %v", err)
	}
	defer func() {
		if err := processLock.Close(); err != nil {
			log.Printf("release deployer process lock: %v", err)
		}
	}()
	if err := deployer.RequireDaemonStopped(cfg.SocketPath); err != nil {
		log.Fatalf("deployer startup preflight: %v", err)
	}
	manager, err := deployer.NewManager(cfg, deployer.ExecRunner{})
	if err != nil {
		log.Fatalf("initialize deployer: %v", err)
	}
	if *reconcileSlot != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if *forceUnobservableDrain {
			log.Printf("WARNING: forcing reconciliation without observable legacy drain safety")
		}
		if err := manager.ReconcileWithOptions(ctx, *reconcileSlot, *forceUnobservableDrain); err != nil {
			log.Fatalf("reconcile deployment: %v", err)
		}
		log.Printf("deployer reconciled to slot %s", *reconcileSlot)
		return
	}
	if *forceUnobservableDrain {
		log.Fatal("--force-unobservable-drain requires --reconcile-slot")
	}
	server := deployer.NewHTTPServer(cfg, manager)

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	log.Printf("sub2api deployer listening on %s", cfg.SocketPath)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		log.Printf("received %s, shutting down", sig)
	case err := <-errCh:
		if err != nil {
			log.Fatalf("deployer server failed: %v", err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown failed: %v", err)
	}
}
