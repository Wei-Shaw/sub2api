//go:build darwin

package liveattestation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	chatGPTApplicationPath = "/Applications/ChatGPT.app"
	attestationTimeout     = 5 * time.Second
)

type darwinProvider struct {
	appSessionID string
	appPaths     []string
REDACTED

type deviceSignals struct {
	SchemaVersion      int      `json:"schemaVersion"`
	PreferredLanguages []string `json:"preferredLanguages"`
	Locale             string   `json:"locale"`
	Timezone           string   `json:"timezone"`
	ScreenSizeSum      int      `json:"screenSizeSum"`
	ScreenScale        float64  `json:"screenScale"`
	AppSessionID       string   `json:"appSessionId"`
REDACTED

type macOSSignals struct {
	Locale    string   `json:"locale"`
	Languages []string `json:"languages"`
	Timezone  string   `json:"timezone"`
	Width     float64  `json:"width"`
	Height    float64  `json:"height"`
	Scale     float64  `json:"scale"`
REDACTED

func NewProvider() Provider {
	paths := []string{chatGPTApplicationPathREDACTED
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		paths = append(paths, filepath.Join(home, "Applications", "ChatGPT.app"))
REDACTED
	return &darwinProvider{
		appSessionID: uuid.NewString(),
		appPaths:     paths,
REDACTED
REDACTED

func (p *darwinProvider) Check(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, attestationTimeout)
	defer cancel()
	_, _, _, err := p.resolveRuntime(checkCtx)
	return err
REDACTED

func (p *darwinProvider) resolveRuntime(ctx context.Context) (string, string, string, error) {
	if runtime.GOARCH != "arm64" {
		return "", "", "", errors.New("live attestation currently requires Apple Silicon; Intel macOS is not supported")
REDACTED
	appPath, err := p.findApplication()
	if err != nil {
		return "", "", "", err
REDACTED
	resourcesPath := filepath.Join(appPath, "Contents", "Resources")
	nodePath := filepath.Join(resourcesPath, "cua_node", "bin", "node")
	modulePath := filepath.Join(resourcesPath, "native", "devicecheck.node")
	for filePath, label := range map[string]string{
		nodePath:   "bundled Node.js runtime",
		modulePath: "DeviceCheck native module",
REDACTED {
		if info, statErr := os.Stat(filePath); statErr != nil || info.IsDir() {
			return "", "", "", fmt.Errorf("%w: ChatGPT app is missing its %s", ErrChatGPTAppMissing, label)
	REDACTED
REDACTED
	bundleID, err := readBundleIdentifier(ctx, appPath)
	if err != nil {
		return "", "", "", err
REDACTED
	return nodePath, modulePath, bundleID, nil
REDACTED

func (p *darwinProvider) Generate(ctx context.Context) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, attestationTimeout)
	defer cancel()
	nodePath, modulePath, bundleID, err := p.resolveRuntime(runCtx)
	if err != nil {
		return "", err
REDACTED
	signals, err := p.readSignals(runCtx)
	if err != nil {
		return "", err
REDACTED
	signalsJSON, err := json.Marshal(signals)
	if err != nil {
		return "", fmt.Errorf("encode Live attestation signals: %w", err)
REDACTED

	command := exec.CommandContext(runCtx, nodePath, "-e", deviceCheckScript)
	command.Env = []string{
		"PATH=/usr/bin:/bin",
		"SUB2API_DEVICECHECK_MODULE=" + modulePath,
		"SUB2API_ATTESTATION_BUNDLE_ID=" + bundleID,
		"SUB2API_ATTESTATION_SIGNALS=" + string(signalsJSON),
REDACTED
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return "", errors.New("ChatGPT DeviceCheck token generation timed out")
	REDACTED
		reason := strings.TrimSpace(stderr.String())
		if len(reason) > 240 {
			reason = reason[:240]
	REDACTED
		if reason == "" {
			reason = err.Error()
	REDACTED
		return "", fmt.Errorf("ChatGPT DeviceCheck token generation failed: %s", reason)
REDACTED
	header := strings.TrimSpace(stdout.String())
	if len(header) < 20 || len(header) > 16*1024 || !json.Valid([]byte(header)) {
		return "", errors.New("ChatGPT DeviceCheck returned a malformed attestation")
REDACTED
	return header, nil
REDACTED

func (p *darwinProvider) findApplication() (string, error) {
	for _, appPath := range p.appPaths {
		info, err := os.Stat(appPath)
		if err == nil && info.IsDir() {
			return appPath, nil
	REDACTED
REDACTED
	return "", ErrChatGPTAppMissing
REDACTED

func readBundleIdentifier(ctx context.Context, appPath string) (string, error) {
	infoPlist := filepath.Join(appPath, "Contents", "Info.plist")
	output, err := exec.CommandContext(
		ctx,
		"/usr/bin/plutil",
		"-extract",
		"CFBundleIdentifier",
		"raw",
		infoPlist,
	).Output()
	if err != nil {
		return "", fmt.Errorf("%w: cannot read its bundle identifier", ErrChatGPTAppMissing)
REDACTED
	bundleID := strings.TrimSpace(string(output))
	if !strings.HasPrefix(bundleID, "com.openai.") {
		return "", errors.New("the installed ChatGPT app has an unexpected bundle identifier")
REDACTED
	return bundleID, nil
REDACTED

func (p *darwinProvider) readSignals(ctx context.Context) (deviceSignals, error) {
	const script = `ObjC.import("Foundation"); ObjC.import("AppKit");
const screen = $.NSScreen.mainScreen;
const frame = screen.frame;
JSON.stringify({
  locale: ObjC.unwrap($.NSLocale.currentLocale.localeIdentifier),
  languages: ObjC.deepUnwrap($.NSLocale.preferredLanguages),
  timezone: ObjC.unwrap($.NSTimeZone.localTimeZone.name),
  width: Number(frame.size.width),
  height: Number(frame.size.height),
  scale: Number(screen.backingScaleFactor)
REDACTED)`
	output, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", script).Output()
	if err != nil {
		return deviceSignals{REDACTED, fmt.Errorf("read macOS signals for Live attestation: %w", err)
REDACTED
	var values macOSSignals
	if err := json.Unmarshal(output, &values); err != nil {
		return deviceSignals{REDACTED, fmt.Errorf("decode macOS signals for Live attestation: %w", err)
REDACTED
	locale := truncateSignal(values.Locale, 64, "unknown")
	languages := values.Languages
	if len(languages) == 0 {
		languages = []string{localeREDACTED
REDACTED
	if len(languages) > 16 {
		languages = languages[:16]
REDACTED
	for index := range languages {
		languages[index] = truncateSignal(languages[index], 64, locale)
REDACTED
	scale := values.Scale
	if scale <= 0 {
		scale = 1
REDACTED
	return deviceSignals{
		SchemaVersion:      1,
		PreferredLanguages: languages,
		Locale:             locale,
		Timezone:           truncateSignal(values.Timezone, 64, "unknown"),
		ScreenSizeSum:      max(0, int(values.Width+values.Height+0.5)),
		ScreenScale:        scale,
		AppSessionID:       truncateSignal(p.appSessionID, 128, uuid.NewString()),
REDACTED, nil
REDACTED

func truncateSignal(value string, limit int, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
REDACTED
	if len(value) > limit {
		return value[:limit]
REDACTED
	return value
REDACTED

const deviceCheckScript = `
const addon = require(process.env.SUB2API_DEVICECHECK_MODULE);
const signals = JSON.parse(process.env.SUB2API_ATTESTATION_SIGNALS);
const bundleID = process.env.SUB2API_ATTESTATION_BUNDLE_ID;

function head(major, value) {
  if (value < 24) return Buffer.from([major + value]);
  if (value <= 255) return Buffer.from([major + 24, value]);
  if (value <= 65535) {
    const out = Buffer.allocUnsafe(3);
    out[0] = major + 25;
    out.writeUInt16BE(value, 1);
    return out;
  REDACTED
  const out = Buffer.allocUnsafe(5);
  out[0] = major + 26;
  out.writeUInt32BE(value, 1);
  return out;
REDACTED
function uint(value) { return head(0, value); REDACTED
function text(value) {
  const body = Buffer.from(value, "utf8");
  return Buffer.concat([head(96, body.length), body]);
REDACTED
function float(value) {
  if (Number.isSafeInteger(value) && value >= 0) return uint(value);
  const out = Buffer.allocUnsafe(9);
  out[0] = 251;
  out.writeDoubleBE(value, 1);
  return out;
REDACTED
function array(values) { return Buffer.concat([head(128, values.length), ...values]); REDACTED
function map(entries) {
  return Buffer.concat([head(160, entries.length), ...entries.flatMap(([key, value]) => [uint(key), value])]);
REDACTED
function field(key, value) { return Buffer.concat([text(key), text(value)]); REDACTED
function base64url(value) {
  return value.toString("base64").replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/u, "");
REDACTED

(async () => {
  const result = await addon.generateToken();
  if (!result || !result.supported) throw new Error("DeviceCheck is not supported on this Mac");
  if (!result.tokenBase64) throw new Error("DeviceCheck returned no token");
  const fingerprint = map([
    [0, uint(signals.schemaVersion)],
    [1, array(signals.preferredLanguages.map(text))],
    [2, text(signals.locale)],
    [3, text(signals.timezone)],
    [4, uint(signals.screenSizeSum)],
    [5, float(signals.screenScale)],
    [6, text(signals.appSessionId)]
  ]);
  const fields = [
    field("token", result.tokenBase64),
    field("bundle_id", bundleID),
    Buffer.concat([text("f"), head(64, fingerprint.length), fingerprint])
  ];
  if (result.latencyMs != null) {
    fields.push(Buffer.concat([text("t"), float(result.latencyMs)]));
  REDACTED
  const token = "v1." + base64url(Buffer.concat([Buffer.from([160 + fields.length]), ...fields]));
  process.stdout.write(JSON.stringify({v: 1, s: 0, t: tokenREDACTED));
REDACTED)().catch((error) => {
  process.stderr.write(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
REDACTED);`
