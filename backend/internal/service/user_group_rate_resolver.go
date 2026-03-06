package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

type userGroupRateResolver struct {
	repo         UserGroupRateRepository
	cache        *gocache.Cache
	cacheTTL     time.Duration
	sf           *singleflight.Group
	logComponent string
REDACTED

func newUserGroupRateResolver(repo UserGroupRateRepository, cache *gocache.Cache, cacheTTL time.Duration, sf *singleflight.Group, logComponent string) *userGroupRateResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserGroupRateCacheTTL
REDACTED
	if cache == nil {
		cache = gocache.New(cacheTTL, time.Minute)
REDACTED
	if logComponent == "" {
		logComponent = "service.gateway"
REDACTED
	if sf == nil {
		sf = &singleflight.Group{REDACTED
REDACTED

	return &userGroupRateResolver{
		repo:         repo,
		cache:        cache,
		cacheTTL:     cacheTTL,
		sf:           sf,
		logComponent: logComponent,
REDACTED
REDACTED

func (r *userGroupRateResolver) Resolve(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if r == nil || userID <= 0 || groupID <= 0 {
		return groupDefaultMultiplier
REDACTED

	key := fmt.Sprintf("%d:%d", userID, groupID)
	if r.cache != nil {
		if cached, ok := r.cache.Get(key); ok {
			if multiplier, castOK := cached.(float64); castOK {
				userGroupRateCacheHitTotal.Add(1)
				return multiplier
		REDACTED
	REDACTED
REDACTED
	if r.repo == nil {
		return groupDefaultMultiplier
REDACTED
	userGroupRateCacheMissTotal.Add(1)

	value, err, shared := r.sf.Do(key, func() (any, error) {
		if r.cache != nil {
			if cached, ok := r.cache.Get(key); ok {
				if multiplier, castOK := cached.(float64); castOK {
					userGroupRateCacheHitTotal.Add(1)
					return multiplier, nil
			REDACTED
		REDACTED
	REDACTED

		userGroupRateCacheLoadTotal.Add(1)
		userRate, repoErr := r.repo.GetByUserAndGroup(ctx, userID, groupID)
		if repoErr != nil {
			return nil, repoErr
	REDACTED

		multiplier := groupDefaultMultiplier
		if userRate != nil {
			multiplier = *userRate
	REDACTED
		if r.cache != nil {
			r.cache.Set(key, multiplier, r.cacheTTL)
	REDACTED
		return multiplier, nil
REDACTED)
	if shared {
		userGroupRateCacheSFSharedTotal.Add(1)
REDACTED
	if err != nil {
		userGroupRateCacheFallbackTotal.Add(1)
		logger.LegacyPrintf(r.logComponent, "get user group rate failed, fallback to group default: user=%d group=%d err=%v", userID, groupID, err)
		return groupDefaultMultiplier
REDACTED

	multiplier, ok := value.(float64)
	if !ok {
		userGroupRateCacheFallbackTotal.Add(1)
		return groupDefaultMultiplier
REDACTED
	return multiplier
REDACTED
