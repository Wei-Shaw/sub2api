// Package redissession provides a multi-instance OAuth session backend.
package redissession

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNotConfigured = errors.New("redis session store not configured")

// Store persists JSON sessions and single-use markers under one namespace.
type Store struct {
	rdb    *redis.Client
	prefix string
	ttl    time.Duration
REDACTED

func New(rdb *redis.Client, prefix string, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 30 * time.Minute
REDACTED
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "oauth:session"
REDACTED
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
REDACTED
	return &Store{rdb: rdb, prefix: prefix, ttl: ttlREDACTED
REDACTED

func (s *Store) dataKey(id string) string { return s.prefix + strings.TrimSpace(id) REDACTED
func (s *Store) usedKey(id string) string { return s.prefix + "used:" + strings.TrimSpace(id) REDACTED

func (s *Store) Set(ctx context.Context, id string, value any) error {
	if s == nil || s.rdb == nil {
		return ErrNotConfigured
REDACTED
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	raw, err := json.Marshal(value)
	if err != nil {
		return err
REDACTED
	return s.rdb.Set(ctx, s.dataKey(id), raw, s.ttl).Err()
REDACTED

func (s *Store) Get(ctx context.Context, id string, dest any) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, ErrNotConfigured
REDACTED
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	raw, err := s.rdb.Get(ctx, s.dataKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil
REDACTED
	if err != nil {
		return false, err
REDACTED
	if err := json.Unmarshal(raw, dest); err != nil {
		return false, err
REDACTED
	return true, nil
REDACTED

func (s *Store) Delete(ctx context.Context, id string) error {
	if s == nil || s.rdb == nil {
		return ErrNotConfigured
REDACTED
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	return s.rdb.Del(ctx, s.dataKey(id), s.usedKey(id)).Err()
REDACTED

// TryConsume returns true only for the first claim while the session exists.
func (s *Store) TryConsume(ctx context.Context, id string) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, ErrNotConfigured
REDACTED
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	ttl := s.ttl
	if remaining, err := s.rdb.TTL(ctx, s.dataKey(id)).Result(); err == nil && remaining > 0 {
		ttl = remaining
REDACTED
	ok, err := s.rdb.SetNX(ctx, s.usedKey(id), "1", ttl).Result()
	if err != nil || !ok {
		return ok, err
REDACTED
	exists, err := s.rdb.Exists(ctx, s.dataKey(id)).Result()
	if err != nil {
		return false, err
REDACTED
	if exists == 0 {
		_ = s.rdb.Del(ctx, s.usedKey(id)).Err()
		return false, nil
REDACTED
	return true, nil
REDACTED
