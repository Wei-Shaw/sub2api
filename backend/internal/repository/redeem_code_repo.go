package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"gorm.io/gorm"
)

type redeemCodeRepository struct {
	db *gorm.DB
REDACTED

func NewRedeemCodeRepository(db *gorm.DB) service.RedeemCodeRepository {
	return &redeemCodeRepository{db: dbREDACTED
REDACTED

func (r *redeemCodeRepository) Create(ctx context.Context, code *service.RedeemCode) error {
	m := redeemCodeModelFromService(code)
	err := r.db.WithContext(ctx).Create(m).Error
	if err == nil {
		applyRedeemCodeModelToService(code, m)
REDACTED
	return err
REDACTED

func (r *redeemCodeRepository) CreateBatch(ctx context.Context, codes []service.RedeemCode) error {
	if len(codes) == 0 {
		return nil
REDACTED
	models := make([]redeemCodeModel, 0, len(codes))
	for i := range codes {
		m := redeemCodeModelFromService(&codes[i])
		if m != nil {
			models = append(models, *m)
	REDACTED
REDACTED
	return r.db.WithContext(ctx).Create(&models).Error
REDACTED

func (r *redeemCodeRepository) GetByID(ctx context.Context, id int64) (*service.RedeemCode, error) {
	var m redeemCodeModel
	err := r.db.WithContext(ctx).First(&m, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrRedeemCodeNotFound, nil)
REDACTED
	return redeemCodeModelToService(&m), nil
REDACTED

func (r *redeemCodeRepository) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	var m redeemCodeModel
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&m).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrRedeemCodeNotFound, nil)
REDACTED
	return redeemCodeModelToService(&m), nil
REDACTED

func (r *redeemCodeRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&redeemCodeModel{REDACTED, id).Error
REDACTED

func (r *redeemCodeRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
REDACTED

func (r *redeemCodeRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	var codes []redeemCodeModel
	var total int64

	db := r.db.WithContext(ctx).Model(&redeemCodeModel{REDACTED)

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

	outCodes := make([]service.RedeemCode, 0, len(codes))
	for i := range codes {
		outCodes = append(outCodes, *redeemCodeModelToService(&codes[i]))
REDACTED

	return outCodes, paginationResultFromTotal(total, params), nil
REDACTED

func (r *redeemCodeRepository) Update(ctx context.Context, code *service.RedeemCode) error {
	m := redeemCodeModelFromService(code)
	err := r.db.WithContext(ctx).Save(m).Error
	if err == nil {
		applyRedeemCodeModelToService(code, m)
REDACTED
	return err
REDACTED

func (r *redeemCodeRepository) Use(ctx context.Context, id, userID int64) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&redeemCodeModel{REDACTED).
		Where("id = ? AND status = ?", id, service.StatusUnused).
		Updates(map[string]any{
			"status":  service.StatusUsed,
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

func (r *redeemCodeRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if limit <= 0 {
		limit = 10
REDACTED

	var codes []redeemCodeModel
	err := r.db.WithContext(ctx).
		Preload("Group").
		Where("used_by = ?", userID).
		Order("used_at DESC").
		Limit(limit).
		Find(&codes).Error
	if err != nil {
		return nil, err
REDACTED

	outCodes := make([]service.RedeemCode, 0, len(codes))
	for i := range codes {
		outCodes = append(outCodes, *redeemCodeModelToService(&codes[i]))
REDACTED
	return outCodes, nil
REDACTED

type redeemCodeModel struct {
	ID        int64   `gorm:"primaryKey"`
	Code      string  `gorm:"uniqueIndex;size:32;not null"`
	Type      string  `gorm:"size:20;default:balance;not null"`
	Value     float64 `gorm:"type:decimal(20,8);not null"`
	Status    string  `gorm:"size:20;default:unused;not null"`
	UsedBy    *int64  `gorm:"index"`
	UsedAt    *time.Time
	Notes     string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"not null"`

	GroupID      *int64 `gorm:"index"`
	ValidityDays int    `gorm:"default:30"`

	User  *userModel  `gorm:"foreignKey:UsedBy"`
	Group *groupModel `gorm:"foreignKey:GroupID"`
REDACTED

func (redeemCodeModel) TableName() string { return "redeem_codes" REDACTED

func redeemCodeModelToService(m *redeemCodeModel) *service.RedeemCode {
	if m == nil {
		return nil
REDACTED
	return &service.RedeemCode{
		ID:           m.ID,
		Code:         m.Code,
		Type:         m.Type,
		Value:        m.Value,
		Status:       m.Status,
		UsedBy:       m.UsedBy,
		UsedAt:       m.UsedAt,
		Notes:        m.Notes,
		CreatedAt:    m.CreatedAt,
		GroupID:      m.GroupID,
		ValidityDays: m.ValidityDays,
		User:         userModelToService(m.User),
		Group:        groupModelToService(m.Group),
REDACTED
REDACTED

func redeemCodeModelFromService(r *service.RedeemCode) *redeemCodeModel {
	if r == nil {
		return nil
REDACTED
	return &redeemCodeModel{
		ID:           r.ID,
		Code:         r.Code,
		Type:         r.Type,
		Value:        r.Value,
		Status:       r.Status,
		UsedBy:       r.UsedBy,
		UsedAt:       r.UsedAt,
		Notes:        r.Notes,
		CreatedAt:    r.CreatedAt,
		GroupID:      r.GroupID,
		ValidityDays: r.ValidityDays,
REDACTED
REDACTED

func applyRedeemCodeModelToService(code *service.RedeemCode, m *redeemCodeModel) {
	if code == nil || m == nil {
		return
REDACTED
	code.ID = m.ID
	code.CreatedAt = m.CreatedAt
REDACTED
