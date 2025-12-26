package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type settingRepository struct {
	db *gorm.DB
REDACTED

func NewSettingRepository(db *gorm.DB) service.SettingRepository {
	return &settingRepository{db: dbREDACTED
REDACTED

func (r *settingRepository) Get(ctx context.Context, key string) (*service.Setting, error) {
	var m settingModel
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&m).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSettingNotFound, nil)
REDACTED
	return settingModelToService(&m), nil
REDACTED

func (r *settingRepository) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
REDACTED
	return setting.Value, nil
REDACTED

func (r *settingRepository) Set(ctx context.Context, key, value string) error {
	m := &settingModel{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(),
REDACTED

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"REDACTEDREDACTED,
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"REDACTED),
REDACTED).Create(m).Error
REDACTED

func (r *settingRepository) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	var settings []settingModel
	err := r.db.WithContext(ctx).Where("key IN ?", keys).Find(&settings).Error
	if err != nil {
		return nil, err
REDACTED

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
REDACTED
	return result, nil
REDACTED

func (r *settingRepository) SetMultiple(ctx context.Context, settings map[string]string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, value := range settings {
			m := &settingModel{
				Key:       key,
				Value:     value,
				UpdatedAt: time.Now(),
		REDACTED
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"REDACTEDREDACTED,
				DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"REDACTED),
		REDACTED).Create(m).Error; err != nil {
				return err
		REDACTED
	REDACTED
		return nil
REDACTED)
REDACTED

func (r *settingRepository) GetAll(ctx context.Context) (map[string]string, error) {
	var settings []settingModel
	err := r.db.WithContext(ctx).Find(&settings).Error
	if err != nil {
		return nil, err
REDACTED

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
REDACTED
	return result, nil
REDACTED

func (r *settingRepository) Delete(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).Where("key = ?", key).Delete(&settingModel{REDACTED).Error
REDACTED

type settingModel struct {
	ID        int64     `gorm:"primaryKey"`
	Key       string    `gorm:"uniqueIndex;size:100;not null"`
	Value     string    `gorm:"type:text;not null"`
	UpdatedAt time.Time `gorm:"not null"`
REDACTED

func (settingModel) TableName() string { return "settings" REDACTED

func settingModelToService(m *settingModel) *service.Setting {
	if m == nil {
		return nil
REDACTED
	return &service.Setting{
		ID:        m.ID,
		Key:       m.Key,
		Value:     m.Value,
		UpdatedAt: m.UpdatedAt,
REDACTED
REDACTED
