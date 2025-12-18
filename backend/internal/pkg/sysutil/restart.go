package sysutil

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
)

const serviceName = "sub2api"

// RestartService triggers a service restart via systemd.
//
// IMPORTANT: This function initiates the restart and returns immediately.
// The actual restart happens asynchronously - the current process will be killed
// by systemd and a new process will be started.
//
// We use Start() instead of Run() because:
//   - systemctl restart will kill the current process first
//   - Run() waits for completion, but the process dies before completion
//   - Start() spawns the command independently, allowing systemd to handle the full cycle
//
// Prerequisites:
//   - Linux OS with systemd
//   - NOPASSWD sudo access configured (install.sh creates /etc/sudoers.d/sub2api)
func RestartService() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd restart only available on Linux")
REDACTED

	log.Println("Initiating service restart...")

	// The sub2api user has NOPASSWD sudo access for systemctl commands
	// (configured by install.sh in /etc/sudoers.d/sub2api).
	// Use -n (non-interactive) to prevent sudo from waiting for password input
	cmd := exec.Command("sudo", "-n", "systemctl", "restart", serviceName)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to initiate service restart: %w", err)
REDACTED

	log.Println("Service restart initiated successfully")
	return nil
REDACTED

// RestartServiceAsync is a fire-and-forget version of RestartService.
// It logs errors instead of returning them, suitable for goroutine usage.
func RestartServiceAsync() {
	if err := RestartService(); err != nil {
		log.Printf("Service restart failed: %v", err)
		log.Println("Please restart the service manually: sudo systemctl restart sub2api")
REDACTED
REDACTED
