package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type redeemCodeRepository struct {
	db *gorm.DB
REDACTED

func NewRedeemCodeRepository(db *gorm.DB) service.RedeemCodeRepository {
	return &redeemCodeRepository{db: dbREDACTED
REDACTED

func (r *redeemCodeRepository) Create(ctx context.Context, code *model.RedeemCode) error {
	return r.db.WithContext(ctx).Create(code).Error
REDACTED

func (r *redeemCodeRepository) CreateBatch(ctx context.Context, codes []model.RedeemCode) error {
	return r.db.WithContext(ctx).Create(&codes).Error
REDACTED

func (r *redeemCodeRepository) GetByID(ctx context.Context, id int64) (*model.RedeemCode, error) {
	var code model.RedeemCode
	err := r.db.WithContext(ctx).First(&code, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrRedeemCodeNotFound, nil)
REDACTED
	return &code, nil
REDACTED

func (r *redeemCodeRepository) GetByCode(ctx context.Context, code string) (*model.RedeemCode, error) {
	var redeemCode model.RedeemCode
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&redeemCode).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrRedeemCodeNotFound, nil)
REDACTED
	return &redeemCode, nil
REDACTED

func (r *redeemCodeRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.RedeemCode{REDACTED, id).Error
REDACTED

func (r *redeemCodeRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.RedeemCode, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
REDACTED

// ListWithFilters lists redeem codes with optional filtering by type, status, and search query
func (r *redeemCodeRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]model.RedeemCode, *pagination.PaginationResult, error) {
	var codes []model.RedeemCode
	var total int64

	db := r.db.WithContext(ctx).Model(&model.RedeemCode{REDACTED)

	// Apply filters
	if codeType != "" {
		db = db.Where("type = ?", codeType)
REDACTED
	if status != "" {
		db = db.Where("status = ?", status)
REDACTED
	if search != "" {
		searchPattern := "%" + search + "%"
		db = db.Where("code ILIKE ?", searchPattern)
REDACTED

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	if err := db.Preload("User").Preload("Group").Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&codes).Error; err != nil {
		return nil, nil, err
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return codes, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

func (r *redeemCodeRepository) Update(ctx context.Context, code *model.RedeemCode) error {
	return r.db.WithContext(ctx).Save(code).Error
REDACTED

func (r *redeemCodeRepository) Use(ctx context.Context, id, userID int64) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&model.RedeemCode{REDACTED).
		Where("id = ? AND status = ?", id, model.StatusUnused).
		Updates(map[string]any{
			"status":  model.StatusUsed,
			"used_by": userID,
			"used_at": now,
	REDACTED)
	if result.Error != nil {
		return result.Error
REDACTED
	if result.RowsAffected == 0 {
		return service.ErrRedeemCodeUsed.WithCause(gorm.ErrRecordNotFound)
REDACTED
	return nil
REDACTED

// ListByUser returns all redeem codes used by a specific user
func (r *redeemCodeRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]model.RedeemCode, error) {
	var codes []model.RedeemCode
	if limit <= 0 {
		limit = 10
REDACTED

	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("used_by = ?", userID).
		Order("used_at DESC").
		Limit(limit).
		Find(&codes).Error

	if err != nil {
		return nil, err
REDACTED
	return codes, nil
REDACTED
