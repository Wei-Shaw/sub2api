package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	dbsoraaccount "github.com/Wei-Shaw/sub2api/ent/soraaccount"
	dbsoracachefile "github.com/Wei-Shaw/sub2api/ent/soracachefile"
	dbsoratask "github.com/Wei-Shaw/sub2api/ent/soratask"
	dbsorausagestat "github.com/Wei-Shaw/sub2api/ent/sorausagestat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

// SoraAccount

type soraAccountRepository struct {
	client *ent.Client
REDACTED

func NewSoraAccountRepository(client *ent.Client) service.SoraAccountRepository {
	return &soraAccountRepository{client: clientREDACTED
REDACTED

func (r *soraAccountRepository) GetByAccountID(ctx context.Context, accountID int64) (*service.SoraAccount, error) {
	if accountID <= 0 {
		return nil, nil
REDACTED
	acc, err := r.client.SoraAccount.Query().Where(dbsoraaccount.AccountIDEQ(accountID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
	REDACTED
		return nil, err
REDACTED
	return mapSoraAccount(acc), nil
REDACTED

func (r *soraAccountRepository) GetByAccountIDs(ctx context.Context, accountIDs []int64) (map[int64]*service.SoraAccount, error) {
	if len(accountIDs) == 0 {
		return map[int64]*service.SoraAccount{REDACTED, nil
REDACTED
	records, err := r.client.SoraAccount.Query().Where(dbsoraaccount.AccountIDIn(accountIDs...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	result := make(map[int64]*service.SoraAccount, len(records))
	for _, record := range records {
		if record == nil {
			continue
	REDACTED
		result[record.AccountID] = mapSoraAccount(record)
REDACTED
	return result, nil
REDACTED

func (r *soraAccountRepository) Upsert(ctx context.Context, accountID int64, updates map[string]any) error {
	if accountID <= 0 {
		return errors.New("invalid account_id")
REDACTED
	acc, err := r.client.SoraAccount.Query().Where(dbsoraaccount.AccountIDEQ(accountID)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return err
REDACTED
	if acc == nil {
		builder := r.client.SoraAccount.Create().SetAccountID(accountID)
		applySoraAccountUpdates(builder.Mutation(), updates)
		return builder.Exec(ctx)
REDACTED
	updater := r.client.SoraAccount.UpdateOneID(acc.ID)
	applySoraAccountUpdates(updater.Mutation(), updates)
	return updater.Exec(ctx)
REDACTED

func applySoraAccountUpdates(m *ent.SoraAccountMutation, updates map[string]any) {
	if updates == nil {
		return
REDACTED
	for key, val := range updates {
		switch key {
		case "access_token":
			if v, ok := val.(string); ok {
				m.SetAccessToken(v)
		REDACTED
		case "session_token":
			if v, ok := val.(string); ok {
				m.SetSessionToken(v)
		REDACTED
		case "refresh_token":
			if v, ok := val.(string); ok {
				m.SetRefreshToken(v)
		REDACTED
		case "client_id":
			if v, ok := val.(string); ok {
				m.SetClientID(v)
		REDACTED
		case "email":
			if v, ok := val.(string); ok {
				m.SetEmail(v)
		REDACTED
		case "username":
			if v, ok := val.(string); ok {
				m.SetUsername(v)
		REDACTED
		case "remark":
			if v, ok := val.(string); ok {
				m.SetRemark(v)
		REDACTED
		case "plan_type":
			if v, ok := val.(string); ok {
				m.SetPlanType(v)
		REDACTED
		case "plan_title":
			if v, ok := val.(string); ok {
				m.SetPlanTitle(v)
		REDACTED
		case "subscription_end":
			if v, ok := val.(time.Time); ok {
				m.SetSubscriptionEnd(v)
		REDACTED
			if v, ok := val.(*time.Time); ok && v != nil {
				m.SetSubscriptionEnd(*v)
		REDACTED
		case "sora_supported":
			if v, ok := val.(bool); ok {
				m.SetSoraSupported(v)
		REDACTED
		case "sora_invite_code":
			if v, ok := val.(string); ok {
				m.SetSoraInviteCode(v)
		REDACTED
		case "sora_redeemed_count":
			if v, ok := val.(int); ok {
				m.SetSoraRedeemedCount(v)
		REDACTED
		case "sora_remaining_count":
			if v, ok := val.(int); ok {
				m.SetSoraRemainingCount(v)
		REDACTED
		case "sora_total_count":
			if v, ok := val.(int); ok {
				m.SetSoraTotalCount(v)
		REDACTED
		case "sora_cooldown_until":
			if v, ok := val.(time.Time); ok {
				m.SetSoraCooldownUntil(v)
		REDACTED
			if v, ok := val.(*time.Time); ok && v != nil {
				m.SetSoraCooldownUntil(*v)
		REDACTED
		case "cooled_until":
			if v, ok := val.(time.Time); ok {
				m.SetCooledUntil(v)
		REDACTED
			if v, ok := val.(*time.Time); ok && v != nil {
				m.SetCooledUntil(*v)
		REDACTED
		case "image_enabled":
			if v, ok := val.(bool); ok {
				m.SetImageEnabled(v)
		REDACTED
		case "video_enabled":
			if v, ok := val.(bool); ok {
				m.SetVideoEnabled(v)
		REDACTED
		case "image_concurrency":
			if v, ok := val.(int); ok {
				m.SetImageConcurrency(v)
		REDACTED
		case "video_concurrency":
			if v, ok := val.(int); ok {
				m.SetVideoConcurrency(v)
		REDACTED
		case "is_expired":
			if v, ok := val.(bool); ok {
				m.SetIsExpired(v)
		REDACTED
	REDACTED
REDACTED
REDACTED

func mapSoraAccount(acc *ent.SoraAccount) *service.SoraAccount {
	if acc == nil {
		return nil
REDACTED
	return &service.SoraAccount{
		AccountID:          acc.AccountID,
		AccessToken:        derefString(acc.AccessToken),
		SessionToken:       derefString(acc.SessionToken),
		RefreshToken:       derefString(acc.RefreshToken),
		ClientID:           derefString(acc.ClientID),
		Email:              derefString(acc.Email),
		Username:           derefString(acc.Username),
		Remark:             derefString(acc.Remark),
		UseCount:           acc.UseCount,
		PlanType:           derefString(acc.PlanType),
		PlanTitle:          derefString(acc.PlanTitle),
		SubscriptionEnd:    acc.SubscriptionEnd,
		SoraSupported:      acc.SoraSupported,
		SoraInviteCode:     derefString(acc.SoraInviteCode),
		SoraRedeemedCount:  acc.SoraRedeemedCount,
		SoraRemainingCount: acc.SoraRemainingCount,
		SoraTotalCount:     acc.SoraTotalCount,
		SoraCooldownUntil:  acc.SoraCooldownUntil,
		CooledUntil:        acc.CooledUntil,
		ImageEnabled:       acc.ImageEnabled,
		VideoEnabled:       acc.VideoEnabled,
		ImageConcurrency:   acc.ImageConcurrency,
		VideoConcurrency:   acc.VideoConcurrency,
		IsExpired:          acc.IsExpired,
		CreatedAt:          acc.CreatedAt,
		UpdatedAt:          acc.UpdatedAt,
REDACTED
REDACTED

func mapSoraUsageStat(stat *ent.SoraUsageStat) *service.SoraUsageStat {
	if stat == nil {
		return nil
REDACTED
	return &service.SoraUsageStat{
		AccountID:             stat.AccountID,
		ImageCount:            stat.ImageCount,
		VideoCount:            stat.VideoCount,
		ErrorCount:            stat.ErrorCount,
		LastErrorAt:           stat.LastErrorAt,
		TodayImageCount:       stat.TodayImageCount,
		TodayVideoCount:       stat.TodayVideoCount,
		TodayErrorCount:       stat.TodayErrorCount,
		TodayDate:             stat.TodayDate,
		ConsecutiveErrorCount: stat.ConsecutiveErrorCount,
		CreatedAt:             stat.CreatedAt,
		UpdatedAt:             stat.UpdatedAt,
REDACTED
REDACTED

func mapSoraCacheFile(file *ent.SoraCacheFile) *service.SoraCacheFile {
	if file == nil {
		return nil
REDACTED
	return &service.SoraCacheFile{
		ID:          int64(file.ID),
		TaskID:      derefString(file.TaskID),
		AccountID:   file.AccountID,
		UserID:      file.UserID,
		MediaType:   file.MediaType,
		OriginalURL: file.OriginalURL,
		CachePath:   file.CachePath,
		CacheURL:    file.CacheURL,
		SizeBytes:   file.SizeBytes,
		CreatedAt:   file.CreatedAt,
REDACTED
REDACTED

// SoraUsageStat

type soraUsageStatRepository struct {
	client *ent.Client
	sql    sqlExecutor
REDACTED

func NewSoraUsageStatRepository(client *ent.Client, sqlDB *sql.DB) service.SoraUsageStatRepository {
	return &soraUsageStatRepository{client: client, sql: sqlDBREDACTED
REDACTED

func (r *soraUsageStatRepository) RecordSuccess(ctx context.Context, accountID int64, isVideo bool) error {
	if accountID <= 0 {
		return nil
REDACTED
	field := "image_count"
	todayField := "today_image_count"
	if isVideo {
		field = "video_count"
		todayField = "today_video_count"
REDACTED
	today := time.Now().UTC().Truncate(24 * time.Hour)
	query := "INSERT INTO sora_usage_stats (account_id, " + field + ", " + todayField + ", today_date, consecutive_error_count, created_at, updated_at) " +
		"VALUES ($1, 1, 1, $2, 0, NOW(), NOW()) " +
		"ON CONFLICT (account_id) DO UPDATE SET " +
		field + " = sora_usage_stats." + field + " + 1, " +
		todayField + " = CASE WHEN sora_usage_stats.today_date = $2 THEN sora_usage_stats." + todayField + " + 1 ELSE 1 END, " +
		"today_date = $2, consecutive_error_count = 0, updated_at = NOW()"
	_, err := r.sql.ExecContext(ctx, query, accountID, today)
	return err
REDACTED

func (r *soraUsageStatRepository) RecordError(ctx context.Context, accountID int64) (int, error) {
	if accountID <= 0 {
		return 0, nil
REDACTED
	today := time.Now().UTC().Truncate(24 * time.Hour)
	query := "INSERT INTO sora_usage_stats (account_id, error_count, today_error_count, today_date, consecutive_error_count, last_error_at, created_at, updated_at) " +
		"VALUES ($1, 1, 1, $2, 1, NOW(), NOW(), NOW()) " +
		"ON CONFLICT (account_id) DO UPDATE SET " +
		"error_count = sora_usage_stats.error_count + 1, " +
		"today_error_count = CASE WHEN sora_usage_stats.today_date = $2 THEN sora_usage_stats.today_error_count + 1 ELSE 1 END, " +
		"today_date = $2, consecutive_error_count = sora_usage_stats.consecutive_error_count + 1, last_error_at = NOW(), updated_at = NOW() " +
		"RETURNING consecutive_error_count"
	var consecutive int
	err := scanSingleRow(ctx, r.sql, query, []any{accountID, todayREDACTED, &consecutive)
	if err != nil {
		return 0, err
REDACTED
	return consecutive, nil
REDACTED

func (r *soraUsageStatRepository) ResetConsecutiveErrors(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
REDACTED
	err := r.client.SoraUsageStat.Update().Where(dbsorausagestat.AccountIDEQ(accountID)).
		SetConsecutiveErrorCount(0).
		Exec(ctx)
	if ent.IsNotFound(err) {
		return nil
REDACTED
	return err
REDACTED

func (r *soraUsageStatRepository) GetByAccountID(ctx context.Context, accountID int64) (*service.SoraUsageStat, error) {
	if accountID <= 0 {
		return nil, nil
REDACTED
	stat, err := r.client.SoraUsageStat.Query().Where(dbsorausagestat.AccountIDEQ(accountID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
	REDACTED
		return nil, err
REDACTED
	return mapSoraUsageStat(stat), nil
REDACTED

func (r *soraUsageStatRepository) GetByAccountIDs(ctx context.Context, accountIDs []int64) (map[int64]*service.SoraUsageStat, error) {
	if len(accountIDs) == 0 {
		return map[int64]*service.SoraUsageStat{REDACTED, nil
REDACTED
	stats, err := r.client.SoraUsageStat.Query().Where(dbsorausagestat.AccountIDIn(accountIDs...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	result := make(map[int64]*service.SoraUsageStat, len(stats))
	for _, stat := range stats {
		if stat == nil {
			continue
	REDACTED
		result[stat.AccountID] = mapSoraUsageStat(stat)
REDACTED
	return result, nil
REDACTED

func (r *soraUsageStatRepository) List(ctx context.Context, params pagination.PaginationParams) ([]*service.SoraUsageStat, *pagination.PaginationResult, error) {
	query := r.client.SoraUsageStat.Query()
	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED
	stats, err := query.Order(ent.Desc(dbsorausagestat.FieldUpdatedAt)).
		Limit(params.Limit()).
		Offset(params.Offset()).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED
	result := make([]*service.SoraUsageStat, 0, len(stats))
	for _, stat := range stats {
		result = append(result, mapSoraUsageStat(stat))
REDACTED
	return result, paginationResultFromTotal(int64(total), params), nil
REDACTED

// SoraTask

type soraTaskRepository struct {
	client *ent.Client
REDACTED

func NewSoraTaskRepository(client *ent.Client) service.SoraTaskRepository {
	return &soraTaskRepository{client: clientREDACTED
REDACTED

func (r *soraTaskRepository) Create(ctx context.Context, task *service.SoraTask) error {
	if task == nil {
		return nil
REDACTED
	builder := r.client.SoraTask.Create().
		SetTaskID(task.TaskID).
		SetAccountID(task.AccountID).
		SetModel(task.Model).
		SetPrompt(task.Prompt).
		SetStatus(task.Status).
		SetProgress(task.Progress).
		SetRetryCount(task.RetryCount)
	if task.ResultURLs != "" {
		builder.SetResultUrls(task.ResultURLs)
REDACTED
	if task.ErrorMessage != "" {
		builder.SetErrorMessage(task.ErrorMessage)
REDACTED
	if task.CreatedAt.IsZero() {
		builder.SetCreatedAt(time.Now())
REDACTED else {
		builder.SetCreatedAt(task.CreatedAt)
REDACTED
	if task.CompletedAt != nil {
		builder.SetCompletedAt(*task.CompletedAt)
REDACTED
	return builder.Exec(ctx)
REDACTED

func (r *soraTaskRepository) UpdateStatus(ctx context.Context, taskID string, status string, progress float64, resultURLs string, errorMessage string, completedAt *time.Time) error {
	if taskID == "" {
		return nil
REDACTED
	builder := r.client.SoraTask.Update().Where(dbsoratask.TaskIDEQ(taskID)).
		SetStatus(status).
		SetProgress(progress)
	if resultURLs != "" {
		builder.SetResultUrls(resultURLs)
REDACTED
	if errorMessage != "" {
		builder.SetErrorMessage(errorMessage)
REDACTED
	if completedAt != nil {
		builder.SetCompletedAt(*completedAt)
REDACTED
	_, err := builder.Save(ctx)
	if ent.IsNotFound(err) {
		return nil
REDACTED
	return err
REDACTED

// SoraCacheFile

type soraCacheFileRepository struct {
	client *ent.Client
REDACTED

func NewSoraCacheFileRepository(client *ent.Client) service.SoraCacheFileRepository {
	return &soraCacheFileRepository{client: clientREDACTED
REDACTED

func (r *soraCacheFileRepository) Create(ctx context.Context, file *service.SoraCacheFile) error {
	if file == nil {
		return nil
REDACTED
	builder := r.client.SoraCacheFile.Create().
		SetAccountID(file.AccountID).
		SetUserID(file.UserID).
		SetMediaType(file.MediaType).
		SetOriginalURL(file.OriginalURL).
		SetCachePath(file.CachePath).
		SetCacheURL(file.CacheURL).
		SetSizeBytes(file.SizeBytes)
	if file.TaskID != "" {
		builder.SetTaskID(file.TaskID)
REDACTED
	if file.CreatedAt.IsZero() {
		builder.SetCreatedAt(time.Now())
REDACTED else {
		builder.SetCreatedAt(file.CreatedAt)
REDACTED
	return builder.Exec(ctx)
REDACTED

func (r *soraCacheFileRepository) ListOldest(ctx context.Context, limit int) ([]*service.SoraCacheFile, error) {
	if limit <= 0 {
		return []*service.SoraCacheFile{REDACTED, nil
REDACTED
	records, err := r.client.SoraCacheFile.Query().
		Order(dbsoracachefile.ByCreatedAt(entsql.OrderAsc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	result := make([]*service.SoraCacheFile, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
	REDACTED
		result = append(result, mapSoraCacheFile(record))
REDACTED
	return result, nil
REDACTED

func (r *soraCacheFileRepository) DeleteByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
REDACTED
	_, err := r.client.SoraCacheFile.Delete().Where(dbsoracachefile.IDIn(ids...)).Exec(ctx)
	return err
REDACTED
