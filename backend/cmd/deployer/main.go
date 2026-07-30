package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/deployer"
)

var (
	Version   = "dev"
	Commit    = "none"
	Date      = "unknown"
	BuildType = "dev"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "control-plane" {
		runControlPlaneCommand(os.Args[2:])
		return
	}
	configPath := flag.String("config", "/etc/sub2api-deployer/config.json", "path to deployer JSON configuration")
	check := flag.Bool("check", false, "validate configuration and exit")
	showVersion := flag.Bool("version", false, "show build identity and exit")
	// This flag is a long-lived systemd ExecStart contract. Do not rename it or
	// change its existing semantics; deployed hosts invoke it once per minute.
	activateStagedControlPlane := flag.Bool("activate-staged-control-plane", false, "activate a verified staged control-plane payload and exit")
	reconcileSlot := flag.String("reconcile-slot", "", "verify and clear a degraded latch by selecting the serving deployment slot")
	forceUnobservableDrain := flag.Bool("force-unobservable-drain", false, "during reconciliation, explicitly allow stopping a legacy container that cannot report drain blockers")
	flag.Parse()
	if *showVersion {
		fmt.Printf("Sub2API Deployer %s (commit: %s, built: %s, type: %s, arch: %s)\n", Version, Commit, Date, BuildType, runtime.GOARCH)
		return
	}

	cfg, err := deployer.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}
	executableSHA, err := deployer.CurrentExecutableSHA256()
	if err != nil {
		log.Fatalf("hash deployer executable: %v", err)
	}
	if *check {
		if *activateStagedControlPlane || *reconcileSlot != "" || *forceUnobservableDrain {
			log.Fatal("--check cannot be combined with reconciliation options")
		}
		log.Printf("configuration is valid")
		return
	}
	// Unlike reconciliation, activation intentionally runs beside the live
	// daemon. It must never take the daemon process lock or require a stale
	// socket; a separate activation lock protects host asset replacement.
	if *activateStagedControlPlane {
		if *reconcileSlot != "" || *forceUnobservableDrain {
			log.Fatal("--activate-staged-control-plane cannot be combined with reconciliation options")
		}
		if err := deployer.ActivateStagedControlPlane(context.Background(), cfg, deployer.ExecRunner{}); err != nil {
			log.Fatalf("activate staged control plane: %v", err)
		}
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
	manager, err := deployer.NewManagerWithBuildInfo(cfg, deployer.ExecRunner{}, deployer.BuildInfo{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		Type:    BuildType,
		Arch:    runtime.GOARCH,
		SHA256:  executableSHA,
	})
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

func runControlPlaneCommand(args []string) {
	if len(args) == 0 {
		log.Fatal("control-plane requires status, retry, or quarantine")
	}
	action := args[0]
	flags := flag.NewFlagSet("control-plane "+action, flag.ExitOnError)
	configPath := flags.String("config", "/etc/sub2api-deployer/config.json", "path to deployer JSON configuration")
	jobID := flags.String("job-id", "", "deployment job id")
	reason := flags.String("reason", "", "operator audit reason")
	if err := flags.Parse(args[1:]); err != nil {
		log.Fatal(err)
	}
	cfg, err := deployer.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}
	switch action {
	case "status":
		data, err := deployer.ControlPlaneStatus(cfg)
		if err != nil {
			log.Fatalf("read control-plane status: %v", err)
		}
		fmt.Println(string(data))
	case "retry":
		if err := deployer.RetryControlPlaneUpgrade(context.Background(), cfg, deployer.ExecRunner{}, *jobID); err != nil {
			log.Fatalf("retry control-plane upgrade: %v", err)
		}
	case "quarantine":
		if err := deployer.QuarantineControlPlaneUpgrade(cfg, *jobID, *reason); err != nil {
			log.Fatalf("quarantine control-plane upgrade: %v", err)
		}
	default:
		log.Fatalf("unknown control-plane action %q", action)
	}
}
