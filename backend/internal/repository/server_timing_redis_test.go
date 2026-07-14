package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/redis/go-redis/v9"
)

func TestServerTimingRedisHookRecordsCommands(t *testing.T) {
	collector := servertiming.New(time.Now())
	ctx := servertiming.WithCollector(context.Background(), collector)
	hook := serverTimingRedisHook{REDACTED

	process := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		time.Sleep(time.Millisecond)
		return errors.New("redis failure")
REDACTED)
	if err := process(ctx, redis.NewStringCmd(ctx, "get", "sensitive-key")); err == nil {
		t.Fatal("ProcessHook did not return the underlying error")
REDACTED

	pipeline := hook.ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
		time.Sleep(time.Millisecond)
		return nil
REDACTED)
	commands := []redis.Cmder{
		redis.NewStringCmd(ctx, "get", "first-secret"),
		redis.NewStringCmd(ctx, "get", "second-secret"),
		redis.NewStatusCmd(ctx, "set", "third-secret", "value"),
REDACTED
	if err := pipeline(ctx, commands); err != nil {
		t.Fatal(err)
REDACTED

	header := collector.HeaderValue(time.Now(), "bypass")
	if !strings.Contains(header, `commands=4`) {
		t.Fatalf("header %q does not report one command and a three-command pipeline", header)
REDACTED
	if strings.Contains(header, "secret") || strings.Contains(header, "get") {
		t.Fatalf("Redis command details leaked into header: %q", header)
REDACTED
REDACTED

func TestServerTimingRedisHookSkipsInactiveContext(t *testing.T) {
	called := false
	hook := serverTimingRedisHook{REDACTED
	process := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		called = true
		return nil
REDACTED)
	ctx := context.Background()
	if err := process(ctx, redis.NewStringCmd(ctx, "ping")); err != nil {
		t.Fatal(err)
REDACTED
	if !called {
		t.Fatal("inactive Redis command did not reach the next hook")
REDACTED
REDACTED
