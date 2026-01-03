package setup

import (
	"bufio"
	"fmt"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// CLI input validation functions (matching Web API validation)
func cliValidateHostname(host string) bool {
	validHost := regexp.MustCompile(`^[a-zA-Z0-9.\-:]+$`)
	return validHost.MatchString(host) && len(host) <= 253
REDACTED

func cliValidateDBName(name string) bool {
	validName := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)
	return validName.MatchString(name) && len(name) <= 63
REDACTED

func cliValidateUsername(name string) bool {
	validName := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	return validName.MatchString(name) && len(name) <= 63
REDACTED

func cliValidateEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil && len(email) <= 254
REDACTED

func cliValidatePort(port int) bool {
	return port > 0 && port <= 65535
REDACTED

func cliValidateSSLMode(mode string) bool {
	validModes := map[string]bool{
		"disable": true, "require": true, "verify-ca": true, "verify-full": true,
REDACTED
	return validModes[mode]
REDACTED

// RunCLI runs the CLI setup wizard
func RunCLI() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║       Sub2API Installation Wizard         ║")
	fmt.Println("╚═══════════════════════════════════════════╝")
	fmt.Println()

	cfg := &SetupConfig{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "release",
	REDACTED,
		JWT: JWTConfig{
			ExpireHour: 24,
	REDACTED,
REDACTED

	// Database configuration with validation
	fmt.Println("── Database Configuration ──")

	for {
		cfg.Database.Host = promptString(reader, "PostgreSQL Host", "localhost")
		if cliValidateHostname(cfg.Database.Host) {
			break
	REDACTED
		fmt.Println("  Invalid hostname format. Use alphanumeric, dots, hyphens only.")
REDACTED

	for {
		cfg.Database.Port = promptInt(reader, "PostgreSQL Port", 5432)
		if cliValidatePort(cfg.Database.Port) {
			break
	REDACTED
		fmt.Println("  Invalid port. Must be between 1 and 65535.")
REDACTED

	for {
		cfg.Database.User = promptString(reader, "PostgreSQL User", "postgres")
		if cliValidateUsername(cfg.Database.User) {
			break
	REDACTED
		fmt.Println("  Invalid username. Use alphanumeric and underscores only.")
REDACTED

	cfg.Database.Password = promptPassword("PostgreSQL Password")

	for {
		cfg.Database.DBName = promptString(reader, "Database Name", "sub2api")
		if cliValidateDBName(cfg.Database.DBName) {
			break
	REDACTED
		fmt.Println("  Invalid database name. Start with letter, use alphanumeric and underscores.")
REDACTED

	for {
		cfg.Database.SSLMode = promptString(reader, "SSL Mode", "disable")
		if cliValidateSSLMode(cfg.Database.SSLMode) {
			break
	REDACTED
		fmt.Println("  Invalid SSL mode. Use: disable, require, verify-ca, or verify-full.")
REDACTED

	fmt.Println()
	fmt.Print("Testing database connection... ")
	if err := TestDatabaseConnection(&cfg.Database); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("database connection failed: %w", err)
REDACTED
	fmt.Println("OK")

	// Redis configuration with validation
	fmt.Println()
	fmt.Println("── Redis Configuration ──")

	for {
		cfg.Redis.Host = promptString(reader, "Redis Host", "localhost")
		if cliValidateHostname(cfg.Redis.Host) {
			break
	REDACTED
		fmt.Println("  Invalid hostname format. Use alphanumeric, dots, hyphens only.")
REDACTED

	for {
		cfg.Redis.Port = promptInt(reader, "Redis Port", 6379)
		if cliValidatePort(cfg.Redis.Port) {
			break
	REDACTED
		fmt.Println("  Invalid port. Must be between 1 and 65535.")
REDACTED

	cfg.Redis.Password = promptPassword("Redis Password (optional)")

	for {
		cfg.Redis.DB = promptInt(reader, "Redis DB", 0)
		if cfg.Redis.DB >= 0 && cfg.Redis.DB <= 15 {
			break
	REDACTED
		fmt.Println("  Invalid Redis DB. Must be between 0 and 15.")
REDACTED

	fmt.Println()
	fmt.Print("Testing Redis connection... ")
	if err := TestRedisConnection(&cfg.Redis); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("redis connection failed: %w", err)
REDACTED
	fmt.Println("OK")

	// Admin configuration with validation
	fmt.Println()
	fmt.Println("── Admin Account ──")

	for {
		cfg.Admin.Email = promptString(reader, "Admin Email", "admin@example.com")
		if cliValidateEmail(cfg.Admin.Email) {
			break
	REDACTED
		fmt.Println("  Invalid email format.")
REDACTED

	for {
		cfg.Admin.Password = promptPassword("Admin Password")
		// SECURITY: Match Web API requirement of 8 characters minimum
		if len(cfg.Admin.Password) < 8 {
			fmt.Println("  Password must be at least 8 characters")
			continue
	REDACTED
		if len(cfg.Admin.Password) > 128 {
			fmt.Println("  Password must be at most 128 characters")
			continue
	REDACTED
		confirm := promptPassword("Confirm Password")
		if cfg.Admin.Password != confirm {
			fmt.Println("  Passwords do not match")
			continue
	REDACTED
		break
REDACTED

	// Server configuration with validation
	fmt.Println()
	fmt.Println("── Server Configuration ──")

	for {
		cfg.Server.Port = promptInt(reader, "Server Port", 8080)
		if cliValidatePort(cfg.Server.Port) {
			break
	REDACTED
		fmt.Println("  Invalid port. Must be between 1 and 65535.")
REDACTED

	// Confirm and install
	fmt.Println()
	fmt.Println("── Configuration Summary ──")
	fmt.Printf("Database: %s@%s:%d/%s\n", cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)
	fmt.Printf("Redis: %s:%d\n", cfg.Redis.Host, cfg.Redis.Port)
	fmt.Printf("Admin: %s\n", cfg.Admin.Email)
	fmt.Printf("Server: :%d\n", cfg.Server.Port)
	fmt.Println()

	if !promptConfirm(reader, "Proceed with installation?") {
		fmt.Println("Installation cancelled")
		return nil
REDACTED

	fmt.Println()
	fmt.Print("Installing... ")
	if err := Install(cfg); err != nil {
		fmt.Println("FAILED")
		return err
REDACTED
	fmt.Println("OK")

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║       Installation Complete!              ║")
	fmt.Println("╚═══════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Start the server with:")
	fmt.Println("  ./sub2api")
	fmt.Println()
	fmt.Printf("Admin panel: http://localhost:%d\n", cfg.Server.Port)
	fmt.Println()

	return nil
REDACTED

func promptString(reader *bufio.Reader, prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", prompt, defaultVal)
REDACTED else {
		fmt.Printf("  %s: ", prompt)
REDACTED

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
REDACTED
	return input
REDACTED

func promptInt(reader *bufio.Reader, prompt string, defaultVal int) int {
	fmt.Printf("  %s [%d]: ", prompt, defaultVal)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
REDACTED

	val, err := strconv.Atoi(input)
	if err != nil {
		return defaultVal
REDACTED
	return val
REDACTED

func promptPassword(prompt string) string {
	fmt.Printf("  %s: ", prompt)

	// Try to read password without echo
	if term.IsTerminal(int(os.Stdin.Fd())) {
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err == nil {
			return string(password)
	REDACTED
REDACTED

	// Fallback to regular input
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
REDACTED

func promptConfirm(reader *bufio.Reader, prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
REDACTED
