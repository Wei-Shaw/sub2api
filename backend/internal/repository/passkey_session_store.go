package repository

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const passkeySessionPrefix = "passkey:session:"

type passkeySessionStore struct {
	redis *redis.Client
REDACTED

func NewPasskeySessionStore(redisClient *redis.Client) service.PasskeySessionStore {
	return &passkeySessionStore{redis: redisClientREDACTED
REDACTED

func (s *passkeySessionStore) Store(
	ctx context.Context,
	session *service.PasskeySession,
	ttl time.Duration,
) (string, error) {
	if session == nil || ttl <= 0 {
		return "", fmt.Errorf("invalid passkey session")
REDACTED
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate passkey session token: %w", err)
REDACTED
	token := base64.RawURLEncoding.EncodeToString(random)
	payload, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("encode passkey session: %w", err)
REDACTED
	if err = s.redis.Set(ctx, passkeySessionPrefix+token, payload, ttl).Err(); err != nil {
		return "", fmt.Errorf("store passkey session: %w", err)
REDACTED
	return token, nil
REDACTED

func (s *passkeySessionStore) Consume(
	ctx context.Context,
	token string,
) (*service.PasskeySession, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return nil, service.ErrPasskeySession
REDACTED
	payload, err := s.redis.GetDel(ctx, passkeySessionPrefix+token).Bytes()
	if err == redis.Nil {
		return nil, service.ErrPasskeySession
REDACTED
	if err != nil {
		return nil, fmt.Errorf("consume passkey session: %w", err)
REDACTED
	var session service.PasskeySession
	if err = json.Unmarshal(payload, &session); err != nil {
		return nil, service.ErrPasskeySession
REDACTED
	return &session, nil
REDACTED
