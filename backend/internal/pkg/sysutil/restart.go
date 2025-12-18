package sysutil

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
)

const serviceName = "sub2api"

// findExecutable finds the full path of an executable
// by checking common system paths
func findExecutable(name string) string {
	// First try exec.LookPath (uses current PATH)
	if path, err := exec.LookPath(name); err == nil {
		return path
REDACTED

	// Fallback: check common paths
	commonPaths := []string{
		"/usr/bin/" + name,
		"/bin/" + name,
		"/usr/sbin/" + name,
		"/sbin/" + name,
REDACTED

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
	REDACTED
REDACTED

	// Return the name as-is and let exec fail with a clear error
	return name
REDACTED

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

	// Find full paths for sudo and systemctl
	// This ensures the commands work even if PATH is limited in systemd service
	sudoPath := findExecutable("sudo")
	systemctlPath := findExecutable("systemctl")

	log.Printf("Using sudo: %s, systemctl: %s", sudoPath, systemctlPath)

	// The sub2api user has NOPASSWD sudo access for systemctl commands
	// (configured by install.sh in /etc/sudoers.d/sub2api).
	// Use -n (non-interactive) to prevent sudo from waiting for password input
	//
	// Use setsid to create a new session, ensuring the child process
	// survives even if the parent process is killed by systemctl restart
	setsidPath := findExecutable("setsid")
	cmd := exec.Command(setsidPath, sudoPath, "-n", systemctlPath, "restart", serviceName)

	// Detach from parent's stdio to ensure clean separation
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

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
