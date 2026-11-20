// Package sysutil provides system-level utilities for process management.
package sysutil

import (
	"errors"
	"log"
	"os"
	"runtime"
	"time"
)

// ErrRestartUnsupported is returned when the current platform cannot restart
// the service by exiting (there is no systemd to bring it back).
var ErrRestartUnsupported = errors.New("service restart via exit is only supported on Linux with systemd")

// RestartSupported reports whether RestartService can restart the service on
// the current platform.
func RestartSupported() bool {
	return runtime.GOOS == "linux"
}

// RestartService triggers a service restart by gracefully exiting.
//
// This relies on systemd's Restart=always configuration to automatically
// restart the service after it exits. This is the industry-standard approach:
//   - Simple and reliable
//   - No sudo permissions needed
//   - No complex process management
//   - Leverages systemd's native restart capability
//
// Prerequisites:
//   - Linux OS with systemd
//   - Service configured with Restart=always in systemd unit file
func RestartService() error {
	if !RestartSupported() {
		return ErrRestartUnsupported
	}

	log.Println("Initiating service restart by graceful exit...")
	log.Println("systemd will automatically restart the service (Restart=always)")

	// Give a moment for logs to flush and response to be sent
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}()

	return nil
}

// RestartServiceAsync is a fire-and-forget version of RestartService.
// It logs errors instead of returning them, suitable for goroutine usage.
func RestartServiceAsync() {
	if err := RestartService(); err != nil {
		log.Printf("Service restart failed: %v", err)
		log.Println("Please restart the service manually")
	}
}
