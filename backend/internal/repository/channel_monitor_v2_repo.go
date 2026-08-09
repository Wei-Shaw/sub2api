package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type channelMonitorV2Repository struct{ db *sql.DB REDACTED

func NewChannelMonitorV2Repository(db *sql.DB) service.ChannelMonitorV2Repository {
	return &channelMonitorV2Repository{db: dbREDACTED
REDACTED

func (r *channelMonitorV2Repository) GetConfig(ctx context.Context) (*service.ChannelMonitorV2Config, error) {
	var cfg service.ChannelMonitorV2Config
	var platforms, thresholds []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT version, enabled, refresh_interval_seconds, platforms, group_ids,
		       COALESCE(ignored_error_categories, '{REDACTED'),
		       COALESCE(health_thresholds, '{REDACTED'::jsonb),
		       updated_at, updated_by
		FROM channel_monitor_v2_config WHERE id = 1`).Scan(
		&cfg.Version, &cfg.Enabled, &cfg.RefreshIntervalSeconds, &platforms,
		pq.Array(&cfg.GroupIDs), pq.Array(&cfg.IgnoredErrorCategories),
		&thresholds,
		&cfg.UpdatedAt, &cfg.UpdatedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("get channel monitor v2 config: %w", err)
REDACTED
	if err := json.Unmarshal(platforms, &cfg.Platforms); err != nil {
		return nil, fmt.Errorf("decode channel monitor v2 platforms: %w", err)
REDACTED
	if cfg.IgnoredErrorCategories == nil {
		cfg.IgnoredErrorCategories = []string{REDACTED
REDACTED
	cfg.HealthThresholds = service.DefaultChannelMonitorV2HealthThresholds()
	if len(thresholds) > 0 {
		_ = json.Unmarshal(thresholds, &cfg.HealthThresholds)
REDACTED
	cfg.HealthThresholds = service.NormalizeChannelMonitorV2HealthThresholds(cfg.HealthThresholds)
	return &cfg, nil
REDACTED

func (r *channelMonitorV2Repository) UpdateConfig(ctx context.Context, cfg service.ChannelMonitorV2Config, expectedVersion int) (*service.ChannelMonitorV2Config, error) {
	platforms, err := json.Marshal(cfg.Platforms)
	if err != nil {
		return nil, err
REDACTED
	if cfg.IgnoredErrorCategories == nil {
		cfg.IgnoredErrorCategories = []string{REDACTED
REDACTED
	cfg.HealthThresholds = service.NormalizeChannelMonitorV2HealthThresholds(cfg.HealthThresholds)
	thresholds, err := json.Marshal(cfg.HealthThresholds)
	if err != nil {
		return nil, err
REDACTED
	var updated service.ChannelMonitorV2Config
	var raw, rawThresholds []byte
	err = r.db.QueryRowContext(ctx, `
		UPDATE channel_monitor_v2_config
		SET version = version + 1, enabled = $1, refresh_interval_seconds = $2,
		    platforms = $3, group_ids = $4, ignored_error_categories = $5,
		    health_thresholds = $6, updated_by = $7, updated_at = NOW()
		WHERE id = 1 AND version = $8
		RETURNING version, enabled, refresh_interval_seconds, platforms, group_ids,
		          COALESCE(ignored_error_categories, '{REDACTED'),
		          COALESCE(health_thresholds, '{REDACTED'::jsonb),
		          updated_at, updated_by`,
		cfg.Enabled, cfg.RefreshIntervalSeconds, platforms, pq.Array(cfg.GroupIDs),
		pq.Array(cfg.IgnoredErrorCategories), thresholds, cfg.UpdatedBy, expectedVersion,
	).Scan(&updated.Version, &updated.Enabled, &updated.RefreshIntervalSeconds, &raw,
		pq.Array(&updated.GroupIDs), pq.Array(&updated.IgnoredErrorCategories),
		&rawThresholds,
		&updated.UpdatedAt, &updated.UpdatedBy)
	if err == sql.ErrNoRows {
		return nil, service.ErrChannelMonitorV2ConfigConflict
REDACTED
	if err != nil {
		return nil, fmt.Errorf("update channel monitor v2 config: %w", err)
REDACTED
	if err := json.Unmarshal(raw, &updated.Platforms); err != nil {
		return nil, err
REDACTED
	if updated.IgnoredErrorCategories == nil {
		updated.IgnoredErrorCategories = []string{REDACTED
REDACTED
	updated.HealthThresholds = service.DefaultChannelMonitorV2HealthThresholds()
	_ = json.Unmarshal(rawThresholds, &updated.HealthThresholds)
	updated.HealthThresholds = service.NormalizeChannelMonitorV2HealthThresholds(updated.HealthThresholds)
	return &updated, nil
REDACTED

type channelMonitorV2Fact struct {
	BucketStart, Platform, GroupName, Model             string
	GroupID                                             int64
	Success, Errors, UpstreamAffected, UpstreamAttempts int64
	Input, Output, CacheCreation, CacheRead             int64
	TTFTSum, TTFTCount, DurationSum, DurationCount      int64
REDACTED

type channelMonitorV2Histogram struct {
	BucketStart, Platform, Model, Metric string
	GroupID, UserID                      int64
	UpperBound                           int64
	Count                                int64
REDACTED

func (r *channelMonitorV2Repository) GetDimensions(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config) (*service.ChannelMonitorV2Dimensions, error) {
	coverage, err := r.loadCoverage(ctx, filter)
	if err != nil {
		return nil, err
REDACTED
	// Catalog dimensions must stay independent of the currently selected
	// platform/group/model multi-select filters so pickers keep their full option set.
	// Time/coverage still come from the request filter; config scope (enabled platforms,
	// group allow-list, display-model collapse) still applies.
	catalogFilter := channelMonitorV2CatalogFilter(channelMonitorV2CommonCoverageFilter(filter, *coverage))
	where, args, _ := channelMonitorV2WhereWithRollup(catalogFilter, cfg, "m")
	query := `SELECT m.platform, COALESCE(g.name, ''), lower(COALESCE(NULLIF(TRIM(g.platform), ''), 'unknown')), m.group_id, m.model,
	                 SUM(m.success_requests + m.error_requests)
	          FROM ` + channelMonitorV2MetricsTable(catalogFilter) + ` m
	          LEFT JOIN groups g ON g.id = NULLIF(m.group_id, 0) ` + where + `
	          GROUP BY m.platform, g.name, g.platform, m.group_id, m.model`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	platformCounts := map[string]int64{REDACTED
	type modelValue struct {
		platform string
		count    int64
REDACTED
	// Model dimensions are platform-scoped; identical names can represent
	// different upstream models and must not share counts.
	modelCounts := map[string]modelValue{REDACTED
	type groupValue struct {
		name     string
		platform string
		count    int64
REDACTED
	groupCounts := map[int64]groupValue{REDACTED
	for _, platform := range channelMonitorV2EnabledPlatforms(cfg) {
		platformCounts[platform] += 0
		for _, p := range cfg.Platforms {
			if p.Platform != platform || len(p.Models) == 0 {
				continue
		REDACTED
			for _, model := range p.Models {
				modelCounts[platform+"\x00"+model] = modelValue{platform: platform, count: 0REDACTED
		REDACTED
	REDACTED
REDACTED
	groupInfo, err := r.loadChannelMonitorV2GroupInfo(ctx, configuredChannelMonitorV2GroupIDs(catalogFilter, cfg))
	if err != nil {
		return nil, err
REDACTED
	for groupID, info := range groupInfo {
		groupCounts[groupID] = groupValue{name: info.name, platform: info.platform, count: 0REDACTED
REDACTED
	for rows.Next() {
		var platform, groupName, groupPlatform, model string
		var groupID, count int64
		if err := rows.Scan(&platform, &groupName, &groupPlatform, &groupID, &model, &count); err != nil {
			return nil, err
	REDACTED
		platformCounts[platform] += count
		displayModel := channelMonitorV2DisplayModel(cfg, platform, model)
		modelKey := platform + "\x00" + displayModel
		currentModel := modelCounts[modelKey]
		currentModel.count += count
		if currentModel.platform == "" {
			currentModel.platform = platform
	REDACTED
		modelCounts[modelKey] = currentModel
		if groupID > 0 {
			current := groupCounts[groupID]
			current.name = groupName
			current.platform = groupPlatform
			current.count += count
			groupCounts[groupID] = current
	REDACTED
REDACTED
	result := &service.ChannelMonitorV2Dimensions{
		Platforms: []service.ChannelMonitorV2Dimension{REDACTED,
		Groups:    []service.ChannelMonitorV2GroupDimension{REDACTED,
		Models:    []service.ChannelMonitorV2Dimension{REDACTED,
REDACTED
	for value, count := range platformCounts {
		result.Platforms = append(result.Platforms, service.ChannelMonitorV2Dimension{Value: value, Label: value, RequestCount: countREDACTED)
REDACTED
	for value, meta := range modelCounts {
		result.Models = append(result.Models, service.ChannelMonitorV2Dimension{Value: value, Label: channelMonitorV2ModelLabel(value), Platform: meta.platform, RequestCount: meta.countREDACTED)
REDACTED
	for id, value := range groupCounts {
		result.Groups = append(result.Groups, service.ChannelMonitorV2GroupDimension{ID: id, Name: value.name, Platform: value.platform, RequestCount: value.countREDACTED)
REDACTED
	sort.Slice(result.Platforms, func(i, j int) bool { return result.Platforms[i].RequestCount > result.Platforms[j].RequestCount REDACTED)
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].RequestCount > result.Models[j].RequestCount REDACTED)
	sort.Slice(result.Groups, func(i, j int) bool { return result.Groups[i].RequestCount > result.Groups[j].RequestCount REDACTED)
	return result, rows.Err()
REDACTED

func (r *channelMonitorV2Repository) GetSnapshot(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, admin bool) (*service.ChannelMonitorV2Snapshot, error) {
	coverage, err := r.loadCoverage(ctx, filter)
	if err != nil {
		return nil, err
REDACTED
	effectiveFilter := channelMonitorV2CommonCoverageFilter(filter, *coverage)
	facts, err := r.loadFacts(ctx, effectiveFilter, cfg, true)
	if err != nil {
		return nil, err
REDACTED
	histograms, err := r.loadHistograms(ctx, effectiveFilter, cfg, 0, true)
	if err != nil {
		return nil, err
REDACTED
	byBucket := map[string]*metricAccumulator{REDACTED
	total := newMetricAccumulator()
	for _, fact := range facts {
		if !channelMonitorV2ModelSelected(filter, cfg, fact.Platform, fact.Model) {
			continue
	REDACTED
		bucket := fact.BucketStart
		acc := byBucket[bucket]
		if acc == nil {
			acc = newMetricAccumulator()
			byBucket[bucket] = acc
	REDACTED
		acc.addFact(fact)
		total.addFact(fact)
REDACTED
	for _, hist := range histograms {
		if !channelMonitorV2ModelSelected(filter, cfg, hist.Platform, hist.Model) {
			continue
	REDACTED
		if acc := byBucket[hist.BucketStart]; acc != nil {
			acc.addHistogram(hist)
	REDACTED
		total.addHistogram(hist)
REDACTED
	minutes := channelMonitorV2CoveredMinutes(filter, *coverage)
	// Category-level ignored errors adjust error_rate without dropping volume.
	ignoredByBucket, ignoredTotal, err := r.loadIgnoredErrorCounts(ctx, effectiveFilter, cfg)
	if err != nil {
		return nil, fmt.Errorf("load ignored error counts: %w", err)
REDACTED
	metrics := total.metric(minutes, admin)
	applyIgnoredErrors(&metrics, ignoredTotal)
	if !admin {
		cfg.UpdatedBy = nil
REDACTED
	result := &service.ChannelMonitorV2Snapshot{Config: cfg, Coverage: *coverage, Metrics: metrics, Health: service.ChannelMonitorV2HealthForWithThresholds(metrics, cfg.HealthThresholds), Trend: []service.ChannelMonitorV2TrendPoint{REDACTEDREDACTED
	keys := make([]string, 0, len(byBucket))
	for key := range byBucket {
		keys = append(keys, key)
REDACTED
	sort.Strings(keys)
	for _, key := range keys {
		bucket, _ := time.Parse(time.RFC3339Nano, key)
		m := byBucket[key].metric(filter.Bucket.Minutes(), admin)
		applyIgnoredErrors(&m, ignoredByBucket[key])
		result.Trend = append(result.Trend, service.ChannelMonitorV2TrendPoint{BucketStart: bucket, Metrics: m, Health: service.ChannelMonitorV2HealthForWithThresholds(m, cfg.HealthThresholds)REDACTED)
REDACTED
	return result, nil
REDACTED

func (r *channelMonitorV2Repository) GetModels(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, admin bool) (*service.ChannelMonitorV2List[service.ChannelMonitorV2ModelRow], error) {
	coverage, err := r.loadCoverage(ctx, filter)
	if err != nil {
		return nil, err
REDACTED
	effectiveFilter := channelMonitorV2CommonCoverageFilter(filter, *coverage)
	facts, err := r.loadFacts(ctx, effectiveFilter, cfg, false)
	if err != nil {
		return nil, err
REDACTED
	hist, err := r.loadHistograms(ctx, effectiveFilter, cfg, 0, false)
	if err != nil {
		return nil, err
REDACTED
	accs := map[string]*metricAccumulator{REDACTED
	for _, platform := range channelMonitorV2EnabledPlatforms(cfg) {
		if len(filter.Platforms) > 0 && !containsString(filter.Platforms, platform) {
			continue
	REDACTED
		models := configuredChannelMonitorV2Models(cfg, platform, filter)
		for _, model := range models {
			if model == "" {
				continue
		REDACTED
			key := platform + "\x00" + model
			if accs[key] == nil {
				accs[key] = newMetricAccumulator()
		REDACTED
	REDACTED
REDACTED
	for _, fact := range facts {
		if !channelMonitorV2ModelSelected(filter, cfg, fact.Platform, fact.Model) {
			continue
	REDACTED
		model := channelMonitorV2DisplayModel(cfg, fact.Platform, fact.Model)
		key := fact.Platform + "\x00" + model
		if accs[key] == nil {
			accs[key] = newMetricAccumulator()
	REDACTED
		accs[key].addFact(fact)
REDACTED
	for _, h := range hist {
		if !channelMonitorV2ModelSelected(filter, cfg, h.Platform, h.Model) {
			continue
	REDACTED
		key := h.Platform + "\x00" + channelMonitorV2DisplayModel(cfg, h.Platform, h.Model)
		if accs[key] != nil {
			accs[key].addHistogram(h)
	REDACTED
REDACTED
	// Per platform+model ignored counts for error_rate adjustment.
	ignoredByPM, _, err := r.loadIgnoredErrorCountsByPlatformModel(ctx, effectiveFilter, cfg)
	if err != nil {
		return nil, fmt.Errorf("load ignored error counts by platform/model: %w", err)
REDACTED
	items := make([]service.ChannelMonitorV2ModelRow, 0, len(accs))
	minutes := channelMonitorV2CoveredMinutes(filter, *coverage)
	for key, acc := range accs {
		parts := strings.SplitN(key, "\x00", 2)
		metrics := acc.metric(minutes, admin)
		applyIgnoredErrors(&metrics, ignoredByPM[key])
		items = append(items, service.ChannelMonitorV2ModelRow{Platform: parts[0], Model: parts[1], Metrics: metrics, Health: service.ChannelMonitorV2HealthForWithThresholds(metrics, cfg.HealthThresholds)REDACTED)
REDACTED
	sort.Slice(items, func(i, j int) bool { return items[i].Metrics.RequestCount > items[j].Metrics.RequestCount REDACTED)
	return &service.ChannelMonitorV2List[service.ChannelMonitorV2ModelRow]{Coverage: *coverage, Items: itemsREDACTED, nil
REDACTED

type channelMonitorV2MatrixKey struct {
	platform, model string
	groupID         int64
REDACTED

type channelMonitorV2MatrixAccumulator struct {
	groupName string
	total     *metricAccumulator
	buckets   map[string]*metricAccumulator
REDACTED

func (r *channelMonitorV2Repository) GetMatrix(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, groupBy service.ChannelMonitorV2GroupBy, admin bool) (*service.ChannelMonitorV2Matrix, error) {
	coverage, err := r.loadCoverage(ctx, filter)
	if err != nil {
		return nil, err
REDACTED
	effectiveFilter := channelMonitorV2CommonCoverageFilter(filter, *coverage)
	facts, err := r.loadFacts(ctx, effectiveFilter, cfg, true)
	if err != nil {
		return nil, err
REDACTED
	histograms, err := r.loadHistograms(ctx, effectiveFilter, cfg, 0, true)
	if err != nil {
		return nil, err
REDACTED

	seedGroupIDs := configuredChannelMonitorV2GroupIDs(filter, cfg)
	// Empty config group list means all groups — load active groups so matrix seed
	// can materialize real platform/group rows (not bare platform placeholders).
	if len(seedGroupIDs) == 0 &&
		(groupBy == service.ChannelMonitorV2GroupByPlatformGroup || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel) {
		allIDs, loadErr := r.listActiveGroupIDs(ctx)
		if loadErr != nil {
			return nil, loadErr
	REDACTED
		seedGroupIDs = allIDs
REDACTED
	groupInfo, err := r.loadChannelMonitorV2GroupInfo(ctx, seedGroupIDs)
	if err != nil {
		return nil, err
REDACTED
	accs := seedChannelMonitorV2MatrixAccumulators(filter, cfg, groupBy, groupInfo)
	for _, fact := range facts {
		if !channelMonitorV2ModelSelected(filter, cfg, fact.Platform, fact.Model) {
			continue
	REDACTED
		// platform_group dimensions require a real group; skip group_id=0 traffic rows.
		if (groupBy == service.ChannelMonitorV2GroupByPlatformGroup || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel) &&
			fact.GroupID <= 0 {
			continue
	REDACTED
		key := channelMonitorV2MatrixDimensionKey(groupBy, cfg, fact.Platform, fact.GroupID, fact.Model)
		acc := accs[key]
		if acc == nil {
			acc = &channelMonitorV2MatrixAccumulator{total: newMetricAccumulator(), buckets: make(map[string]*metricAccumulator)REDACTED
			accs[key] = acc
	REDACTED
		if key.groupID > 0 {
			acc.groupName = fact.GroupName
	REDACTED
		bucket := acc.buckets[fact.BucketStart]
		if bucket == nil {
			bucket = newMetricAccumulator()
			acc.buckets[fact.BucketStart] = bucket
	REDACTED
		acc.total.addFact(fact)
		bucket.addFact(fact)
REDACTED
	for _, histogram := range histograms {
		if !channelMonitorV2ModelSelected(filter, cfg, histogram.Platform, histogram.Model) {
			continue
	REDACTED
		if (groupBy == service.ChannelMonitorV2GroupByPlatformGroup || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel) &&
			histogram.GroupID <= 0 {
			continue
	REDACTED
		key := channelMonitorV2MatrixDimensionKey(groupBy, cfg, histogram.Platform, histogram.GroupID, histogram.Model)
		acc := accs[key]
		if acc == nil {
			continue
	REDACTED
		acc.total.addHistogram(histogram)
		if bucket := acc.buckets[histogram.BucketStart]; bucket != nil {
			bucket.addHistogram(histogram)
	REDACTED
REDACTED

	result := &service.ChannelMonitorV2Matrix{GroupBy: groupBy, Coverage: *coverage, Items: make([]service.ChannelMonitorV2MatrixRow, 0, len(accs))REDACTED
	minutes := channelMonitorV2CoveredMinutes(filter, *coverage)
	ignoredByDimBucket, ignoredByDim, err := r.loadIgnoredErrorCountsByMatrixKey(ctx, effectiveFilter, cfg, groupBy)
	if err != nil {
		return nil, fmt.Errorf("load ignored error counts by matrix key: %w", err)
REDACTED
	for key, acc := range accs {
		// platform_group views only emit rows with a real group_id.
		if (groupBy == service.ChannelMonitorV2GroupByPlatformGroup || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel) &&
			key.groupID <= 0 {
			continue
	REDACTED
		metrics := acc.total.metric(minutes, admin)
		applyIgnoredErrors(&metrics, ignoredByDim[key])
		row := service.ChannelMonitorV2MatrixRow{Platform: key.platform, GroupName: acc.groupName, Model: key.model, Metrics: metrics, Health: service.ChannelMonitorV2HealthForWithThresholds(metrics, cfg.HealthThresholds), Buckets: []service.ChannelMonitorV2TrendPoint{REDACTEDREDACTED
		if key.groupID > 0 {
			groupID := key.groupID
			row.GroupID = &groupID
	REDACTED
		bucketKeys := make([]string, 0, len(acc.buckets))
		for bucket := range acc.buckets {
			bucketKeys = append(bucketKeys, bucket)
	REDACTED
		sort.Strings(bucketKeys)
		for _, bucketKey := range bucketKeys {
			bucketStart, parseErr := time.Parse(time.RFC3339Nano, bucketKey)
			if parseErr != nil {
				continue
		REDACTED
			bucketMetrics := acc.buckets[bucketKey].metric(filter.Bucket.Minutes(), admin)
			applyIgnoredErrors(&bucketMetrics, ignoredByDimBucket[key][bucketKey])
			row.Buckets = append(row.Buckets, service.ChannelMonitorV2TrendPoint{BucketStart: bucketStart, Metrics: bucketMetrics, Health: service.ChannelMonitorV2HealthForWithThresholds(bucketMetrics, cfg.HealthThresholds)REDACTED)
	REDACTED
		result.Items = append(result.Items, row)
REDACTED
	sort.Slice(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if a.Platform != b.Platform {
			return a.Platform < b.Platform
	REDACTED
		if a.GroupName != b.GroupName {
			return a.GroupName < b.GroupName
	REDACTED
		if a.GroupID != nil && b.GroupID != nil && *a.GroupID != *b.GroupID {
			return *a.GroupID < *b.GroupID
	REDACTED
		return a.Model < b.Model
REDACTED)
	return result, nil
REDACTED

func channelMonitorV2MatrixDimensionKey(groupBy service.ChannelMonitorV2GroupBy, cfg service.ChannelMonitorV2Config, platform string, groupID int64, model string) channelMonitorV2MatrixKey {
	key := channelMonitorV2MatrixKey{platform: platformREDACTED
	if groupBy == service.ChannelMonitorV2GroupByPlatformGroup || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel {
		key.groupID = groupID
REDACTED
	if groupBy == service.ChannelMonitorV2GroupByPlatformModel || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel {
		key.model = channelMonitorV2DisplayModel(cfg, platform, model)
REDACTED
	return key
REDACTED

func seedChannelMonitorV2MatrixAccumulators(filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, groupBy service.ChannelMonitorV2GroupBy, groupInfo map[int64]channelMonitorV2GroupInfo) map[channelMonitorV2MatrixKey]*channelMonitorV2MatrixAccumulator {
	accs := map[channelMonitorV2MatrixKey]*channelMonitorV2MatrixAccumulator{REDACTED
	platforms := channelMonitorV2EnabledPlatforms(cfg)
	if len(filter.Platforms) > 0 {
		platforms = intersectStrings(platforms, filter.Platforms)
REDACTED
	// Platform-only groupBy seeds groupID=0. When group is part of the dimension
	// (platform_group / platform_group_model), never seed a bare platform row —
	// only real group IDs (from config or discovered groupInfo).
	needsGroup := groupBy == service.ChannelMonitorV2GroupByPlatformGroup || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel
	groupIDs := []int64{0REDACTED
	if needsGroup {
		groupIDs = configuredChannelMonitorV2GroupIDs(filter, cfg)
		if len(groupIDs) == 0 {
			// Empty config group list means "all groups": use discovered active groups.
			for id := range groupInfo {
				groupIDs = append(groupIDs, id)
		REDACTED
			sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] REDACTED)
	REDACTED
		// Still empty → seed nothing; rows come only from fact traffic.
		if len(groupIDs) == 0 {
			return accs
	REDACTED
REDACTED
	for _, platform := range platforms {
		models := []string{""REDACTED
		if groupBy == service.ChannelMonitorV2GroupByPlatformModel || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel {
			models = configuredChannelMonitorV2Models(cfg, platform, filter)
			if len(models) == 0 {
				models = []string{""REDACTED
		REDACTED
	REDACTED
		for _, groupID := range groupIDs {
			info := groupInfo[groupID]
			if needsGroup {
				if groupID <= 0 {
					continue
			REDACTED
				if info.platform == "" || info.platform != platform {
					continue
			REDACTED
		REDACTED
			for _, model := range models {
				key := channelMonitorV2MatrixKey{platform: platformREDACTED
				if needsGroup {
					key.groupID = groupID
			REDACTED
				if groupBy == service.ChannelMonitorV2GroupByPlatformModel || groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel {
					key.model = model
			REDACTED
				if accs[key] == nil {
					accs[key] = &channelMonitorV2MatrixAccumulator{groupName: info.name, total: newMetricAccumulator(), buckets: make(map[string]*metricAccumulator)REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return accs
REDACTED

func configuredChannelMonitorV2Models(cfg service.ChannelMonitorV2Config, platform string, filter service.ChannelMonitorV2Filter) []string {
	models := []string{REDACTED
	for _, p := range cfg.Platforms {
		if p.Platform != platform {
			continue
	REDACTED
		models = append(models, p.Models...)
		break
REDACTED
	if len(filter.Models) > 0 {
		if len(models) == 0 {
			models = append(models, filter.Models...)
	REDACTED else {
			models = intersectStrings(models, filter.Models)
	REDACTED
REDACTED
	return models
REDACTED

// channelMonitorV2CatalogFilter clears multi-select dimensions so catalog endpoints
// return the full configured option set for the time window. Metrics queries must
// keep using the original filter.
func channelMonitorV2CatalogFilter(filter service.ChannelMonitorV2Filter) service.ChannelMonitorV2Filter {
	catalog := filter
	catalog.Platforms = nil
	catalog.GroupIDs = nil
	catalog.Models = nil
	return catalog
REDACTED

func configuredChannelMonitorV2GroupIDs(filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config) []int64 {
	groups := append([]int64(nil), cfg.GroupIDs...)
	if len(filter.GroupIDs) > 0 {
		if len(groups) > 0 {
			groups = intersectInt64(groups, filter.GroupIDs)
	REDACTED else {
			groups = append([]int64(nil), filter.GroupIDs...)
	REDACTED
REDACTED
	return groups
REDACTED

type channelMonitorV2GroupInfo struct {
	name     string
	platform string
REDACTED

func (r *channelMonitorV2Repository) listActiveGroupIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM groups WHERE deleted_at IS NULL AND status = 'active' ORDER BY id`)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	ids := []int64{REDACTED
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
	REDACTED
		ids = append(ids, id)
REDACTED
	return ids, rows.Err()
REDACTED

func (r *channelMonitorV2Repository) loadChannelMonitorV2GroupInfo(ctx context.Context, groupIDs []int64) (map[int64]channelMonitorV2GroupInfo, error) {
	out := map[int64]channelMonitorV2GroupInfo{REDACTED
	if len(groupIDs) == 0 {
		return out, nil
REDACTED
	rows, err := r.db.QueryContext(ctx, `SELECT id, COALESCE(name, ''), lower(COALESCE(NULLIF(TRIM(platform), ''), 'unknown')) FROM groups WHERE id = ANY($1) AND deleted_at IS NULL AND status = 'active'`, pq.Array(groupIDs))
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	for rows.Next() {
		var id int64
		var name, platform string
		if err := rows.Scan(&id, &name, &platform); err != nil {
			return nil, err
	REDACTED
		out[id] = channelMonitorV2GroupInfo{name: name, platform: platformREDACTED
REDACTED
	return out, rows.Err()
REDACTED

func (r *channelMonitorV2Repository) GetErrors(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, includeAdmin bool) (*service.ChannelMonitorV2List[service.ChannelMonitorV2ErrorRow], error) {
	coverage, err := r.loadCoverage(ctx, filter)
	if err != nil {
		return nil, err
REDACTED
	filter = channelMonitorV2CommonCoverageFilter(filter, *coverage)
	where, args, _ := channelMonitorV2WhereWithRollup(filter, cfg, "e")
	rows, err := r.db.QueryContext(ctx, `SELECT e.platform,e.model,e.error_category,SUM(e.error_requests) FROM `+channelMonitorV2ErrorMetricsTable(filter)+` e `+where+` AND e.taxonomy_version = `+fmt.Sprint(service.ChannelMonitorV2TaxonomyVersion)+` GROUP BY e.platform,e.model,e.error_category`, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	counts := map[string]int64{REDACTED
	var total int64
	ignoredSet := service.ChannelMonitorV2IgnoredCategorySet(cfg)
	for rows.Next() {
		var platform, model, category string
		var count int64
		if err := rows.Scan(&platform, &model, &category, &count); err != nil {
			return nil, err
	REDACTED
		if !channelMonitorV2ModelSelected(filter, cfg, platform, model) {
			continue
	REDACTED
		if category == "" {
			category = "other"
	REDACTED
		counts[category] += count
		total += count
REDACTED
	items := make([]service.ChannelMonitorV2ErrorRow, 0, len(counts))
	// Upstream message samples are admin-only: skip the raw ops_error_logs scan
	// for user-facing callers (privacy + avoids multi-million-row online scans).
	var details map[string][]service.ChannelMonitorV2ErrorDetail
	if includeAdmin {
		loaded, detailErr := r.loadErrorDetails(ctx, filter, cfg)
		if detailErr != nil {
			return nil, detailErr
	REDACTED
		details = loaded
REDACTED
	for category, count := range counts {
		rate := 0.0
		if total > 0 {
			rate = float64(count) / float64(total)
	REDACTED
		_, ignored := ignoredSet[category]
		row := service.ChannelMonitorV2ErrorRow{Category: category, Count: count, Rate: rate, Ignored: ignoredREDACTED
		if includeAdmin {
			row.Details = details[category]
	REDACTED
		items = append(items, row)
REDACTED
	// Stable: counted categories first (by count), then ignored (grey in UI).
	sort.Slice(items, func(i, j int) bool {
		if items[i].Ignored != items[j].Ignored {
			return !items[i].Ignored && items[j].Ignored
	REDACTED
		return items[i].Count > items[j].Count
REDACTED)
	return &service.ChannelMonitorV2List[service.ChannelMonitorV2ErrorRow]{Coverage: *coverage, Items: itemsREDACTED, rows.Err()
REDACTED

func (r *channelMonitorV2Repository) loadErrorDetails(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config) (map[string][]service.ChannelMonitorV2ErrorDetail, error) {
	conditions := []string{
		"current_error.created_at >= $1",
		"current_error.created_at < $2",
		"NOT current_error.is_count_tokens",
		"(COALESCE(current_error.status_code, 0) >= 400 OR current_error.error_type = 'cyber_policy')",
		`(NULLIF(current_error.request_id, '') IS NULL OR NOT EXISTS (
				SELECT 1 FROM ops_error_logs newer
				WHERE newer.request_id = current_error.request_id
				  AND NOT newer.is_count_tokens
				  AND (COALESCE(newer.status_code, 0) >= 400 OR newer.error_type = 'cyber_policy')
				  AND (newer.created_at, newer.id) > (current_error.created_at, current_error.id)
		))`,
REDACTED
	args := []any{filter.Start, filter.EndREDACTED
	platforms := channelMonitorV2EnabledPlatforms(cfg)
	if len(filter.Platforms) > 0 {
		platforms = intersectStrings(platforms, filter.Platforms)
REDACTED
	if len(platforms) > 0 {
		args = append(args, pq.Array(platforms))
		conditions = append(conditions, fmt.Sprintf("lower(COALESCE(NULLIF(TRIM(current_error.platform), ''), 'unknown')) = ANY($%d)", len(args)))
REDACTED else {
		conditions = append(conditions, "FALSE")
REDACTED
	groups := cfg.GroupIDs
	groupScopeEmpty := false
	if len(filter.GroupIDs) > 0 {
		if len(groups) > 0 {
			groups = intersectInt64(groups, filter.GroupIDs)
			groupScopeEmpty = len(groups) == 0
	REDACTED else {
			groups = filter.GroupIDs
	REDACTED
REDACTED
	if groupScopeEmpty {
		conditions = append(conditions, "FALSE")
REDACTED else if len(groups) > 0 {
		args = append(args, pq.Array(groups))
		conditions = append(conditions, fmt.Sprintf("COALESCE(current_error.group_id, 0) = ANY($%d)", len(args)))
REDACTED
	query := `SELECT
			lower(COALESCE(NULLIF(TRIM(current_error.platform), ''), 'unknown')) AS platform,
			COALESCE(current_error.group_id, 0) AS group_id,
			COALESCE(NULLIF(TRIM(current_error.requested_model), ''), NULLIF(TRIM(current_error.model), ''), 'unknown') AS model,
			COALESCE(current_error.error_type, '') AS error_type,
			COALESCE(current_error.error_owner, '') AS error_owner,
			COALESCE(current_error.error_source, '') AS error_source,
			COALESCE(current_error.status_code, 0) AS status_code,
			COALESCE(current_error.upstream_status_code, 0) AS upstream_status_code,
			LEFT(COALESCE(NULLIF(current_error.upstream_error_message, ''), NULLIF(current_error.error_message, ''), NULLIF(current_error.upstream_error_detail, ''), NULLIF(current_error.error_body, ''), current_error.error_type, ''), 600) AS message,
			COUNT(*) AS count
		FROM ops_error_logs current_error
		WHERE ` + strings.Join(conditions, " AND ") + `
		GROUP BY 1,2,3,4,5,6,7,8,9
		ORDER BY count DESC
		LIMIT 400`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	out := map[string][]service.ChannelMonitorV2ErrorDetail{REDACTED
	for rows.Next() {
		var platform, model, errorType, owner, source, message string
		var groupID int64
		var statusCode, upstreamStatusCode int
		var count int64
		if err := rows.Scan(&platform, &groupID, &model, &errorType, &owner, &source, &statusCode, &upstreamStatusCode, &message, &count); err != nil {
			return nil, err
	REDACTED
		if !channelMonitorV2ModelSelected(filter, cfg, platform, model) {
			continue
	REDACTED
		category := service.ClassifyChannelMonitorV2Error(service.ChannelMonitorV2ErrorInput{
			ErrorType:          errorType,
			ErrorOwner:         owner,
			ErrorSource:        source,
			StatusCode:         statusCode,
			UpstreamStatusCode: upstreamStatusCode,
			Message:            message,
	REDACTED)
		if len(out[category]) >= 5 {
			continue
	REDACTED
		out[category] = append(out[category], service.ChannelMonitorV2ErrorDetail{
			Platform:           platform,
			Model:              channelMonitorV2DisplayModel(cfg, platform, model),
			ErrorType:          errorType,
			StatusCode:         statusCode,
			UpstreamStatusCode: upstreamStatusCode,
			Message:            sanitizeChannelMonitorV2ErrorDetail(message),
			Count:              count,
	REDACTED)
REDACTED
	return out, rows.Err()
REDACTED

func sanitizeChannelMonitorV2ErrorDetail(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	replacements := []string{"Bearer ", "bearer ", "sk-", "sk-proj-"REDACTED
	for _, marker := range replacements {
		if idx := strings.Index(message, marker); idx >= 0 {
			end := idx + len(marker)
			for end < len(message) && message[end] != ' ' && message[end] != ',' && message[end] != '"' {
				end++
		REDACTED
			message = message[:idx] + marker + "****" + message[end:]
	REDACTED
REDACTED
	if len(message) > 240 {
		message = message[:240] + "…"
REDACTED
	return message
REDACTED

func (r *channelMonitorV2Repository) GetUsers(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, admin bool) (*service.ChannelMonitorV2List[service.ChannelMonitorV2UserRow], error) {
	coverage, err := r.loadCoverage(ctx, filter)
	if err != nil {
		return nil, err
REDACTED
	effectiveFilter := channelMonitorV2CommonCoverageFilter(filter, *coverage)
	filter = effectiveFilter
	where, args, _ := channelMonitorV2WhereWithRollup(filter, cfg, "m")
	query := `SELECT m.user_id,COALESCE(u.email,''),COALESCE(u.username,''),m.platform,m.model,
	SUM(m.success_requests),SUM(m.error_requests),SUM(m.input_tokens),SUM(m.output_tokens),SUM(m.cache_creation_tokens),SUM(m.cache_read_tokens),SUM(m.ttft_sum_ms),SUM(m.ttft_count),SUM(m.duration_sum_ms),SUM(m.duration_count)
	FROM ` + channelMonitorV2UserMetricsTable(filter) + ` m LEFT JOIN users u ON u.id=m.user_id ` + where + ` GROUP BY m.user_id,u.email,u.username,m.platform,m.model`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	type userMeta struct{ email, username string REDACTED
	meta := map[int64]userMeta{REDACTED
	accs := map[int64]*metricAccumulator{REDACTED
	for rows.Next() {
		var uid int64
		var email, username string
		var f channelMonitorV2Fact
		if err := rows.Scan(&uid, &email, &username, &f.Platform, &f.Model, &f.Success, &f.Errors, &f.Input, &f.Output, &f.CacheCreation, &f.CacheRead, &f.TTFTSum, &f.TTFTCount, &f.DurationSum, &f.DurationCount); err != nil {
			return nil, err
	REDACTED
		if !channelMonitorV2ModelSelected(filter, cfg, f.Platform, f.Model) {
			continue
	REDACTED
		if accs[uid] == nil {
			accs[uid] = newMetricAccumulator()
	REDACTED
		accs[uid].addFact(f)
		meta[uid] = userMeta{email, usernameREDACTED
REDACTED
	histograms, err := r.loadHistograms(ctx, filter, cfg, -1, false)
	if err != nil {
		return nil, err
REDACTED
	for _, histogram := range histograms {
		if !channelMonitorV2ModelSelected(filter, cfg, histogram.Platform, histogram.Model) {
			continue
	REDACTED
		if acc := accs[histogram.UserID]; acc != nil {
			acc.addHistogram(histogram)
	REDACTED
REDACTED
	// Error category facts are not per-user. Approximate ignored-error impact by
	// applying the window-level ignored/error ratio to each user's error count so
	// user-rank rates stay consistent with overview scoring.
	_, ignoredTotal, err := r.loadIgnoredErrorCounts(ctx, effectiveFilter, cfg)
	if err != nil {
		return nil, fmt.Errorf("load ignored error counts for users: %w", err)
REDACTED
	var windowErrors int64
	for _, acc := range accs {
		windowErrors += acc.errors
REDACTED
	ignoreRatio := 0.0
	if windowErrors > 0 && ignoredTotal > 0 {
		ignoreRatio = float64(ignoredTotal) / float64(windowErrors)
		if ignoreRatio > 1 {
			ignoreRatio = 1
	REDACTED
REDACTED

	items := make([]service.ChannelMonitorV2UserRow, 0, len(accs))
	minutes := channelMonitorV2CoveredMinutes(filter, *coverage)
	for uid, acc := range accs {
		id := uid
		m := meta[uid]
		label := m.username
		if label == "" {
			label = m.email
	REDACTED
		metrics := acc.metric(minutes, false)
		if ignoreRatio > 0 && metrics.ErrorRequests > 0 {
			approxIgnored := int64(float64(metrics.ErrorRequests)*ignoreRatio + 0.5)
			applyIgnoredErrors(&metrics, approxIgnored)
	REDACTED
		// Recompute health after rate adjustment.
		// (metric() does not attach health; callers that need it recompute.)
		items = append(items, service.ChannelMonitorV2UserRow{UserID: &id, Email: m.email, Username: m.username, DisplayLabel: label, CanDrilldown: admin, Metrics: metricsREDACTED)
REDACTED
	sort.Slice(items, func(i, j int) bool { return items[i].Metrics.RequestCount > items[j].Metrics.RequestCount REDACTED)
	return &service.ChannelMonitorV2List[service.ChannelMonitorV2UserRow]{Coverage: *coverage, Items: itemsREDACTED, rows.Err()
REDACTED

func (r *channelMonitorV2Repository) loadFacts(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, byBucket bool) ([]channelMonitorV2Fact, error) {
	where, args, bucketSeconds := channelMonitorV2WhereWithRollup(filter, cfg, "m")
	bucketExpr := "MIN(m.bucket_start)"
	group := "m.platform,m.group_id,g.name,m.model"
	if byBucket {
		if bucketSeconds > 0 {
			bucketExpr = "m.bucket_start"
			group = bucketExpr + "," + group
	REDACTED else {
			args = append([]any{fmt.Sprintf("%d seconds", int(filter.Bucket.Seconds()))REDACTED, args...)
			where = shiftSQLPlaceholders(where, 1)
			bucketExpr = "date_bin($1::interval,m.bucket_start,TIMESTAMPTZ '1970-01-01')"
			group = bucketExpr + "," + group
	REDACTED
REDACTED
	query := `SELECT ` + bucketExpr + `,m.platform,m.group_id,COALESCE(g.name,''),m.model,SUM(m.success_requests),SUM(m.error_requests),SUM(m.upstream_affected_requests),SUM(m.upstream_attempt_count),SUM(m.input_tokens),SUM(m.output_tokens),SUM(m.cache_creation_tokens),SUM(m.cache_read_tokens),SUM(m.ttft_sum_ms),SUM(m.ttft_count),SUM(m.duration_sum_ms),SUM(m.duration_count) FROM ` + channelMonitorV2MetricsTable(filter) + ` m LEFT JOIN groups g ON g.id=NULLIF(m.group_id,0) ` + where + ` GROUP BY ` + group
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	facts := []channelMonitorV2Fact{REDACTED
	for rows.Next() {
		var bucket time.Time
		var f channelMonitorV2Fact
		if err := rows.Scan(&bucket, &f.Platform, &f.GroupID, &f.GroupName, &f.Model, &f.Success, &f.Errors, &f.UpstreamAffected, &f.UpstreamAttempts, &f.Input, &f.Output, &f.CacheCreation, &f.CacheRead, &f.TTFTSum, &f.TTFTCount, &f.DurationSum, &f.DurationCount); err != nil {
			return nil, err
	REDACTED
		f.BucketStart = bucket.UTC().Format(time.RFC3339Nano)
		facts = append(facts, f)
REDACTED
	return facts, rows.Err()
REDACTED

func (r *channelMonitorV2Repository) loadHistograms(ctx context.Context, filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, userID int64, byBucket bool) ([]channelMonitorV2Histogram, error) {
	where, args, bucketSeconds := channelMonitorV2WhereWithRollup(filter, cfg, "h")
	if userID < 0 {
		where += " AND h.user_id > 0"
REDACTED else {
		args = append(args, userID)
		where += fmt.Sprintf(" AND h.user_id = $%d", len(args))
REDACTED
	bucketExpr := "MIN(h.bucket_start)"
	group := "h.platform,h.group_id,h.model,h.user_id,h.metric,h.upper_bound_ms"
	if byBucket {
		if bucketSeconds > 0 {
			bucketExpr = "h.bucket_start"
			group = bucketExpr + "," + group
	REDACTED else {
			oldArgs := args
			args = []any{fmt.Sprintf("%d seconds", int(filter.Bucket.Seconds()))REDACTED
			args = append(args, oldArgs...)
			where = shiftSQLPlaceholders(where, 1)
			bucketExpr = "date_bin($1::interval,h.bucket_start,TIMESTAMPTZ '1970-01-01')"
			group = bucketExpr + "," + group
	REDACTED
REDACTED
	rows, err := r.db.QueryContext(ctx, `SELECT `+bucketExpr+`,h.platform,h.group_id,h.model,h.user_id,h.metric,h.upper_bound_ms,SUM(h.sample_count) FROM `+channelMonitorV2HistogramTable(filter)+` h `+where+` GROUP BY `+group, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	out := []channelMonitorV2Histogram{REDACTED
	for rows.Next() {
		var bucket time.Time
		var h channelMonitorV2Histogram
		if err := rows.Scan(&bucket, &h.Platform, &h.GroupID, &h.Model, &h.UserID, &h.Metric, &h.UpperBound, &h.Count); err != nil {
			return nil, err
	REDACTED
		h.BucketStart = bucket.UTC().Format(time.RFC3339Nano)
		out = append(out, h)
REDACTED
	return out, rows.Err()
REDACTED

func (r *channelMonitorV2Repository) GetAggregationWatermark(ctx context.Context) (*service.ChannelMonitorV2AggregationWatermark, error) {
	var usageStart, errorStart, dataThrough, computed, backfill sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT usage_coverage_start, error_coverage_start, data_through, last_successful_at, backfill_cursor
		FROM channel_monitor_v2_watermarks WHERE id = 1`).Scan(&usageStart, &errorStart, &dataThrough, &computed, &backfill)
	if err == sql.ErrNoRows {
		return &service.ChannelMonitorV2AggregationWatermark{REDACTED, nil
REDACTED
	if err != nil {
		return nil, err
REDACTED
	out := &service.ChannelMonitorV2AggregationWatermark{HasData: dataThrough.ValidREDACTED
	if usageStart.Valid {
		out.UsageCoverageStart = usageStart.Time.UTC()
REDACTED
	if errorStart.Valid {
		out.ErrorCoverageStart = errorStart.Time.UTC()
REDACTED
	if dataThrough.Valid {
		out.DataThrough = dataThrough.Time.UTC()
REDACTED
	if computed.Valid {
		out.LastSuccessfulAt = computed.Time.UTC()
REDACTED
	if backfill.Valid {
		out.BackfillCursor = backfill.Time.UTC()
REDACTED
	return out, nil
REDACTED

func (r *channelMonitorV2Repository) loadCoverage(ctx context.Context, filter service.ChannelMonitorV2Filter) (*service.ChannelMonitorV2Coverage, error) {
	wm, err := r.GetAggregationWatermark(ctx)
	if err != nil {
		return nil, err
REDACTED
	if wm == nil {
		wm = &service.ChannelMonitorV2AggregationWatermark{REDACTED
REDACTED
	bootstrap := service.ChannelMonitorV2BootstrapProgress(time.Now().UTC(), wm.BackfillCursor, wm.HasData)
	if !wm.HasData || wm.DataThrough.IsZero() || wm.LastSuccessfulAt.IsZero() {
		return &service.ChannelMonitorV2Coverage{
			RequestedStart:        filter.Start,
			RequestedEnd:          filter.End,
			CoverageStart:         filter.End,
			DataThrough:           filter.Start,
			ComputedAt:            time.Time{REDACTED,
			AggregationLagSeconds: 0,
			CoverageComplete:      false,
			BucketSeconds:         int(filter.Bucket.Seconds()),
			Bootstrap:             bootstrap,
	REDACTED, nil
REDACTED
	coverageStart := filter.Start
	if !wm.UsageCoverageStart.IsZero() && wm.UsageCoverageStart.After(coverageStart) {
		coverageStart = wm.UsageCoverageStart
REDACTED
	if !wm.ErrorCoverageStart.IsZero() && wm.ErrorCoverageStart.After(coverageStart) {
		coverageStart = wm.ErrorCoverageStart
REDACTED
	through := filter.End
	if !wm.DataThrough.IsZero() && wm.DataThrough.Before(through) {
		through = wm.DataThrough
REDACTED
	computedAt := wm.LastSuccessfulAt
	lag := int64(0)
	if !through.IsZero() {
		lag = int64(time.Since(through).Seconds())
		if lag < 0 {
			lag = 0
	REDACTED
REDACTED
	// Complete = history depth for this range is filled (backfill reached
	// filter.Start). Do not require data_through >= filter.End: ParseFilter
	// aligns End to the next whole bucket (often in the future), so a healthy
	// minute-level lag would otherwise always show "partial historical coverage".
	return &service.ChannelMonitorV2Coverage{
		RequestedStart:        filter.Start,
		RequestedEnd:          filter.End,
		CoverageStart:         coverageStart,
		DataThrough:           through,
		ComputedAt:            computedAt,
		AggregationLagSeconds: lag,
		CoverageComplete:      channelMonitorV2HistoryCoverageComplete(coverageStart, filter.Start),
		BucketSeconds:         int(filter.Bucket.Seconds()),
		Bootstrap:             bootstrap,
REDACTED, nil
REDACTED

// channelMonitorV2HistoryCoverageComplete is true when aggregated history
// reaches the requested window start. Trailing freshness is reported via
// data_through / aggregation_lag_seconds, not coverage_complete.
func channelMonitorV2HistoryCoverageComplete(coverageStart, filterStart time.Time) bool {
	if coverageStart.IsZero() || filterStart.IsZero() {
		return false
REDACTED
	return !coverageStart.After(filterStart)
REDACTED

func channelMonitorV2FixedBucketSeconds(filter service.ChannelMonitorV2Filter) int {
	seconds := int(filter.Bucket.Seconds())
	switch seconds {
	case 300, 3600, 43200, 86400:
		return seconds
	default:
		return 0
REDACTED
REDACTED

func channelMonitorV2MetricsTable(filter service.ChannelMonitorV2Filter) string {
	if channelMonitorV2FixedBucketSeconds(filter) > 0 {
		return "channel_monitor_v2_metrics_rollup"
REDACTED
	return "channel_monitor_v2_metrics_1m"
REDACTED

func channelMonitorV2UserMetricsTable(filter service.ChannelMonitorV2Filter) string {
	if channelMonitorV2FixedBucketSeconds(filter) > 0 {
		return "channel_monitor_v2_user_metrics_rollup"
REDACTED
	return "channel_monitor_v2_user_metrics_1m"
REDACTED

func channelMonitorV2ErrorMetricsTable(filter service.ChannelMonitorV2Filter) string {
	if channelMonitorV2FixedBucketSeconds(filter) > 0 {
		return "channel_monitor_v2_error_metrics_rollup"
REDACTED
	return "channel_monitor_v2_error_metrics_1m"
REDACTED

func channelMonitorV2HistogramTable(filter service.ChannelMonitorV2Filter) string {
	if channelMonitorV2FixedBucketSeconds(filter) > 0 {
		return "channel_monitor_v2_latency_histograms_rollup"
REDACTED
	return "channel_monitor_v2_latency_histograms_1m"
REDACTED

func channelMonitorV2WhereWithRollup(filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, alias string) (string, []any, int) {
	where, args := channelMonitorV2Where(filter, cfg, alias)
	bucketSeconds := channelMonitorV2FixedBucketSeconds(filter)
	if bucketSeconds > 0 {
		args = append(args, bucketSeconds)
		where += fmt.Sprintf(" AND %s.bucket_seconds = $%d", alias, len(args))
REDACTED
	return where, args, bucketSeconds
REDACTED

func channelMonitorV2Where(filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, alias string) (string, []any) {
	conditions := []string{alias + ".bucket_start >= $1", alias + ".bucket_start < $2"REDACTED
	args := []any{filter.Start, filter.EndREDACTED
	platforms := channelMonitorV2EnabledPlatforms(cfg)
	if len(filter.Platforms) > 0 {
		platforms = intersectStrings(platforms, filter.Platforms)
REDACTED
	if len(platforms) > 0 {
		args = append(args, pq.Array(platforms))
		conditions = append(conditions, fmt.Sprintf("%s.platform = ANY($%d)", alias, len(args)))
REDACTED else {
		conditions = append(conditions, "FALSE")
REDACTED
	groups := cfg.GroupIDs
	groupScopeEmpty := false
	if len(filter.GroupIDs) > 0 {
		if len(groups) > 0 {
			groups = intersectInt64(groups, filter.GroupIDs)
			groupScopeEmpty = len(groups) == 0
	REDACTED else {
			groups = filter.GroupIDs
	REDACTED
REDACTED
	if groupScopeEmpty {
		conditions = append(conditions, "FALSE")
REDACTED else if len(groups) > 0 {
		args = append(args, pq.Array(groups))
		conditions = append(conditions, fmt.Sprintf("%s.group_id = ANY($%d)", alias, len(args)))
REDACTED
	return "WHERE " + strings.Join(conditions, " AND "), args
REDACTED

func channelMonitorV2EnabledPlatforms(cfg service.ChannelMonitorV2Config) []string {
	out := []string{REDACTED
	for _, p := range cfg.Platforms {
		if p.Enabled {
			out = append(out, p.Platform)
	REDACTED
REDACTED
	return out
REDACTED

// channelMonitorV2DisplayModel maps a raw model name for presentation.
// Semantics (parallel to empty group_ids = all groups):
//   - platform not in config / disabled → keep raw model (still collected)
//   - models list empty → show the real model name (no collapsing)
//   - models list non-empty → selected keep identity; everything else → __other__
func channelMonitorV2DisplayModel(cfg service.ChannelMonitorV2Config, platform, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return service.ChannelMonitorV2OtherModel
REDACTED
	for _, p := range cfg.Platforms {
		if p.Platform != platform {
			continue
	REDACTED
		// Empty allow-list: surface every real model instead of dumping into __other__.
		// Operators opt into the named + __other__ split only by listing models.
		if len(p.Models) == 0 {
			return model
	REDACTED
		for _, selected := range p.Models {
			if selected == model {
				return model
		REDACTED
	REDACTED
		return service.ChannelMonitorV2OtherModel
REDACTED
	// Platform not configured: still show the real model so traffic is visible.
	return model
REDACTED
func channelMonitorV2ModelSelected(filter service.ChannelMonitorV2Filter, cfg service.ChannelMonitorV2Config, platform, model string) bool {
	if len(filter.Models) == 0 {
		return true
REDACTED
	display := channelMonitorV2DisplayModel(cfg, platform, model)
	for _, selected := range filter.Models {
		if selected == display {
			return true
	REDACTED
REDACTED
	return false
REDACTED
func channelMonitorV2ModelLabel(model string) string {
	if model == service.ChannelMonitorV2OtherModel {
		return "Other models"
REDACTED
	return model
REDACTED

func channelMonitorV2CoveredMinutes(filter service.ChannelMonitorV2Filter, coverage service.ChannelMonitorV2Coverage) float64 {
	start := filter.Start
	if coverage.CoverageStart.After(start) {
		start = coverage.CoverageStart
REDACTED
	end := filter.End
	if !coverage.DataThrough.IsZero() && coverage.DataThrough.Before(end) {
		end = coverage.DataThrough
REDACTED
	if !start.Before(end) {
		return 1
REDACTED
	return end.Sub(start).Minutes()
REDACTED

// Usage and error sources can have different retention windows. Composite
// health metrics must use their common interval; otherwise error rates mix
// unlike periods and RPM/TPM use a denominator shorter than their numerator.
func channelMonitorV2CommonCoverageFilter(filter service.ChannelMonitorV2Filter, coverage service.ChannelMonitorV2Coverage) service.ChannelMonitorV2Filter {
	if coverage.CoverageStart.After(filter.Start) {
		filter.Start = coverage.CoverageStart
REDACTED
	if !coverage.DataThrough.IsZero() && coverage.DataThrough.Before(filter.End) {
		filter.End = coverage.DataThrough
REDACTED
	return filter
REDACTED
func intersectStrings(a, b []string) []string {
	set := map[string]struct{REDACTED{REDACTED
	for _, v := range b {
		set[v] = struct{REDACTED{REDACTED
REDACTED
	out := []string{REDACTED
	for _, v := range a {
		if _, ok := set[v]; ok {
			out = append(out, v)
	REDACTED
REDACTED
	return out
REDACTED

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func intersectInt64(a, b []int64) []int64 {
	set := map[int64]struct{REDACTED{REDACTED
	for _, v := range b {
		set[v] = struct{REDACTED{REDACTED
REDACTED
	out := []int64{REDACTED
	for _, v := range a {
		if _, ok := set[v]; ok {
			out = append(out, v)
	REDACTED
REDACTED
	return out
REDACTED

// shiftSQLPlaceholders shifts existing positional placeholders by offset.
func shiftSQLPlaceholders(query string, offset int) string {
	for i := 64; i >= 1; i-- {
		query = strings.ReplaceAll(query, fmt.Sprintf("$%d", i), fmt.Sprintf("$%d", i+offset))
REDACTED
	return query
REDACTED

type metricAccumulator struct {
	success, errors, upstreamAffected, upstreamAttempts, input, output, cacheCreation, cacheRead, ttftSum, ttftCount, durationSum, durationCount int64
	hist                                                                                                                                         map[string]map[int64]int64
REDACTED

func newMetricAccumulator() *metricAccumulator {
	return &metricAccumulator{hist: map[string]map[int64]int64{"ttft": {REDACTED, "duration": {REDACTEDREDACTEDREDACTED
REDACTED
func (a *metricAccumulator) addFact(f channelMonitorV2Fact) {
	a.success += f.Success
	a.errors += f.Errors
	a.upstreamAffected += f.UpstreamAffected
	a.upstreamAttempts += f.UpstreamAttempts
	a.input += f.Input
	a.output += f.Output
	a.cacheCreation += f.CacheCreation
	a.cacheRead += f.CacheRead
	a.ttftSum += f.TTFTSum
	a.ttftCount += f.TTFTCount
	a.durationSum += f.DurationSum
	a.durationCount += f.DurationCount
REDACTED
func (a *metricAccumulator) addHistogram(h channelMonitorV2Histogram) {
	if a.hist[h.Metric] == nil {
		a.hist[h.Metric] = map[int64]int64{REDACTED
REDACTED
	a.hist[h.Metric][h.UpperBound] += h.Count
REDACTED
func (a *metricAccumulator) metric(minutes float64, admin bool) service.ChannelMonitorV2Metric {
	requests := a.success + a.errors
	tokens := a.input + a.output + a.cacheCreation + a.cacheRead
	denom := a.input + a.cacheCreation + a.cacheRead
	if minutes <= 0 {
		minutes = 1
REDACTED
	m := service.ChannelMonitorV2Metric{SuccessRequests: a.success, ErrorRequests: a.errors, RequestCount: requests, InputTokens: a.input, OutputTokens: a.output, CacheCreationTokens: a.cacheCreation, CacheReadTokens: a.cacheRead, TokenCount: tokens, RPM: float64(requests) / minutes, TPM: float64(tokens) / minutes, CacheRateNumerator: a.cacheRead, CacheRateDenominator: denom, TTFT: latencyMetric(a.ttftSum, a.ttftCount, a.hist["ttft"]), Duration: latencyMetric(a.durationSum, a.durationCount, a.hist["duration"])REDACTED
	if requests > 0 {
		m.ErrorRate = float64(a.errors) / float64(requests)
		m.SuccessRate = float64(a.success) / float64(requests)
REDACTED
	if denom > 0 {
		m.CacheRate = float64(a.cacheRead) / float64(denom)
REDACTED
	if admin {
		affected, attempts := a.upstreamAffected, a.upstreamAttempts
		m.UpstreamAffectedRequests = &affected
		m.UpstreamAttemptCount = &attempts
REDACTED
	return m
REDACTED
func latencyMetric(sum, count int64, hist map[int64]int64) service.ChannelMonitorV2Latency {
	result := service.ChannelMonitorV2Latency{SampleCount: countREDACTED
	if count > 0 {
		avg := float64(sum) / float64(count)
		result.AvgMs = &avg
REDACTED
	result.P50Ms = histPercentile(hist, 0.50)
	result.P90Ms = histPercentile(hist, 0.90)
	result.P95Ms = histPercentile(hist, 0.95)
	return result
REDACTED
func histPercentile(hist map[int64]int64, p float64) *int64 {
	var total int64
	keys := make([]int64, 0, len(hist))
	for bound, count := range hist {
		keys = append(keys, bound)
		total += count
REDACTED
	if total == 0 {
		return nil
REDACTED
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] REDACTED)
	target := int64(float64(total)*p + 0.999999)
	var cumulative int64
	for _, bound := range keys {
		cumulative += hist[bound]
		if cumulative >= target {
			v := bound
			return &v
	REDACTED
REDACTED
	v := keys[len(keys)-1]
	return &v
REDACTED

// applyIgnoredErrors rewrites ErrorRate so ignored categories do not inflate the
// scored error rate used by health. Absolute ErrorRequests / RequestCount stay
// for volume/RPM. SuccessRate remains true success/request (ignored errors are
// not treated as successes).
func applyIgnoredErrors(m *service.ChannelMonitorV2Metric, ignoredCount int64) {
	if m == nil || ignoredCount <= 0 || m.RequestCount <= 0 {
		return
REDACTED
	if ignoredCount > m.ErrorRequests {
		ignoredCount = m.ErrorRequests
REDACTED
	countedErrors := m.ErrorRequests - ignoredCount
	if countedErrors < 0 {
		countedErrors = 0
REDACTED
	m.ErrorRate = float64(countedErrors) / float64(m.RequestCount)
	// Keep SuccessRate = SuccessRequests / RequestCount when absolute success is
	// known; fall back to residual only if SuccessRequests was never populated.
	if m.SuccessRequests > 0 || m.ErrorRequests >= m.RequestCount {
		m.SuccessRate = float64(m.SuccessRequests) / float64(m.RequestCount)
REDACTED
	if m.SuccessRate < 0 {
		m.SuccessRate = 0
REDACTED
	if m.SuccessRate > 1 {
		m.SuccessRate = 1
REDACTED
REDACTED

// loadIgnoredErrorCounts returns per-bucket and total ignored error request counts
// for categories listed in cfg.IgnoredErrorCategories. Buckets use the same
// date_bin alignment as loadFacts when filter.Bucket is set.
func (r *channelMonitorV2Repository) loadIgnoredErrorCounts(
	ctx context.Context,
	filter service.ChannelMonitorV2Filter,
	cfg service.ChannelMonitorV2Config,
) (byBucket map[string]int64, total int64, err error) {
	byBucket = map[string]int64{REDACTED
	if len(cfg.IgnoredErrorCategories) == 0 {
		return byBucket, 0, nil
REDACTED
	where, args, bucketSeconds := channelMonitorV2WhereWithRollup(filter, cfg, "e")
	bucketExpr := "e.bucket_start"
	groupBy := "e.bucket_start, e.platform, e.model"
	if filter.Bucket > 0 && bucketSeconds == 0 {
		args = append([]any{fmt.Sprintf("%d seconds", int(filter.Bucket.Seconds()))REDACTED, args...)
		where = shiftSQLPlaceholders(where, 1)
		bucketExpr = "date_bin($1::interval,e.bucket_start,TIMESTAMPTZ '1970-01-01')"
		groupBy = bucketExpr + ", e.platform, e.model"
REDACTED
	args = append(args, pq.Array(cfg.IgnoredErrorCategories), service.ChannelMonitorV2TaxonomyVersion)
	catIdx := len(args) - 1
	taxIdx := len(args)
	query := fmt.Sprintf(
		`SELECT %s, e.platform, e.model, SUM(e.error_requests)
		 FROM `+channelMonitorV2ErrorMetricsTable(filter)+` e %s
		 AND e.error_category = ANY($%d) AND e.taxonomy_version = $%d
		 GROUP BY %s`,
		bucketExpr, where, catIdx, taxIdx, groupBy,
	)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return byBucket, 0, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	for rows.Next() {
		var bucket time.Time
		var platform, model string
		var count int64
		if err := rows.Scan(&bucket, &platform, &model, &count); err != nil {
			return byBucket, 0, err
	REDACTED
		if !channelMonitorV2ModelSelected(filter, cfg, platform, model) {
			continue
	REDACTED
		key := bucket.UTC().Format(time.RFC3339Nano)
		byBucket[key] += count
		total += count
REDACTED
	return byBucket, total, rows.Err()
REDACTED

// loadIgnoredErrorCountsByPlatformModel keys are platform + "\x00" + display model.
func (r *channelMonitorV2Repository) loadIgnoredErrorCountsByPlatformModel(
	ctx context.Context,
	filter service.ChannelMonitorV2Filter,
	cfg service.ChannelMonitorV2Config,
) (byPM map[string]int64, total int64, err error) {
	byPM = map[string]int64{REDACTED
	if len(cfg.IgnoredErrorCategories) == 0 {
		return byPM, 0, nil
REDACTED
	where, args, _ := channelMonitorV2WhereWithRollup(filter, cfg, "e")
	args = append(args, pq.Array(cfg.IgnoredErrorCategories), service.ChannelMonitorV2TaxonomyVersion)
	catIdx := len(args) - 1
	taxIdx := len(args)
	query := fmt.Sprintf(
		`SELECT e.platform, e.model, SUM(e.error_requests)
		 FROM `+channelMonitorV2ErrorMetricsTable(filter)+` e %s
		 AND e.error_category = ANY($%d) AND e.taxonomy_version = $%d
		 GROUP BY e.platform, e.model`,
		where, catIdx, taxIdx,
	)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return byPM, 0, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	for rows.Next() {
		var platform, model string
		var count int64
		if err := rows.Scan(&platform, &model, &count); err != nil {
			return byPM, 0, err
	REDACTED
		if !channelMonitorV2ModelSelected(filter, cfg, platform, model) {
			continue
	REDACTED
		display := channelMonitorV2DisplayModel(cfg, platform, model)
		byPM[platform+"\x00"+display] += count
		total += count
REDACTED
	return byPM, total, rows.Err()
REDACTED

// loadIgnoredErrorCountsByMatrixKey returns ignored counts keyed by matrix dimension
// and by (dimension, bucket). Uses the same date_bin alignment as GetMatrix facts.
func (r *channelMonitorV2Repository) loadIgnoredErrorCountsByMatrixKey(
	ctx context.Context,
	filter service.ChannelMonitorV2Filter,
	cfg service.ChannelMonitorV2Config,
	groupBy service.ChannelMonitorV2GroupBy,
) (byDimBucket map[channelMonitorV2MatrixKey]map[string]int64, byDim map[channelMonitorV2MatrixKey]int64, err error) {
	byDimBucket = map[channelMonitorV2MatrixKey]map[string]int64{REDACTED
	byDim = map[channelMonitorV2MatrixKey]int64{REDACTED
	if len(cfg.IgnoredErrorCategories) == 0 {
		return byDimBucket, byDim, nil
REDACTED
	where, args, bucketSeconds := channelMonitorV2WhereWithRollup(filter, cfg, "e")
	bucketExpr := "e.bucket_start"
	groupSQL := "e.bucket_start, e.platform, e.group_id, e.model"
	if filter.Bucket > 0 && bucketSeconds == 0 {
		args = append([]any{fmt.Sprintf("%d seconds", int(filter.Bucket.Seconds()))REDACTED, args...)
		where = shiftSQLPlaceholders(where, 1)
		bucketExpr = "date_bin($1::interval,e.bucket_start,TIMESTAMPTZ '1970-01-01')"
		groupSQL = bucketExpr + ", e.platform, e.group_id, e.model"
REDACTED
	args = append(args, pq.Array(cfg.IgnoredErrorCategories), service.ChannelMonitorV2TaxonomyVersion)
	catIdx := len(args) - 1
	taxIdx := len(args)
	query := fmt.Sprintf(
		`SELECT %s, e.platform, e.group_id, e.model, SUM(e.error_requests)
		 FROM `+channelMonitorV2ErrorMetricsTable(filter)+` e %s
		 AND e.error_category = ANY($%d) AND e.taxonomy_version = $%d
		 GROUP BY %s`,
		bucketExpr, where, catIdx, taxIdx, groupSQL,
	)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return byDimBucket, byDim, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	for rows.Next() {
		var bucket time.Time
		var platform, model string
		var groupID, count int64
		if err := rows.Scan(&bucket, &platform, &groupID, &model, &count); err != nil {
			return byDimBucket, byDim, err
	REDACTED
		if !channelMonitorV2ModelSelected(filter, cfg, platform, model) {
			continue
	REDACTED
		bucketKey := bucket.UTC().Format(time.RFC3339Nano)
		key := channelMonitorV2MatrixDimensionKey(groupBy, cfg, platform, groupID, model)
		byDim[key] += count
		if byDimBucket[key] == nil {
			byDimBucket[key] = map[string]int64{REDACTED
	REDACTED
		byDimBucket[key][bucketKey] += count
REDACTED
	return byDimBucket, byDim, rows.Err()
REDACTED
