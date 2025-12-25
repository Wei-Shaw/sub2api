//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type SettingRepoSuite struct {
	suite.Suite
	ctx  context.Context
	db   *gorm.DB
	repo *settingRepository
REDACTED

func (s *SettingRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
	s.repo = NewSettingRepository(s.db).(*settingRepository)
REDACTED

func TestSettingRepoSuite(t *testing.T) {
	suite.Run(t, new(SettingRepoSuite))
REDACTED

func (s *SettingRepoSuite) TestSetAndGetValue() {
	s.Require().NoError(s.repo.Set(s.ctx, "k1", "v1"), "Set")
	got, err := s.repo.GetValue(s.ctx, "k1")
	s.Require().NoError(err, "GetValue")
	s.Require().Equal("v1", got, "GetValue mismatch")
REDACTED

func (s *SettingRepoSuite) TestSet_Upsert() {
	s.Require().NoError(s.repo.Set(s.ctx, "k1", "v1"), "Set")
	s.Require().NoError(s.repo.Set(s.ctx, "k1", "v2"), "Set upsert")
	got, err := s.repo.GetValue(s.ctx, "k1")
	s.Require().NoError(err, "GetValue after upsert")
	s.Require().Equal("v2", got, "upsert mismatch")
REDACTED

func (s *SettingRepoSuite) TestGetValue_Missing() {
	_, err := s.repo.GetValue(s.ctx, "nonexistent")
	s.Require().Error(err, "expected error for missing key")
	s.Require().ErrorIs(err, service.ErrSettingNotFound)
REDACTED

func (s *SettingRepoSuite) TestSetMultiple_AndGetMultiple() {
	s.Require().NoError(s.repo.SetMultiple(s.ctx, map[string]string{"k2": "v2", "k3": "v3"REDACTED), "SetMultiple")
	m, err := s.repo.GetMultiple(s.ctx, []string{"k2", "k3"REDACTED)
	s.Require().NoError(err, "GetMultiple")
	s.Require().Equal("v2", m["k2"])
	s.Require().Equal("v3", m["k3"])
REDACTED

func (s *SettingRepoSuite) TestGetMultiple_EmptyKeys() {
	m, err := s.repo.GetMultiple(s.ctx, []string{REDACTED)
	s.Require().NoError(err, "GetMultiple with empty keys")
	s.Require().Empty(m, "expected empty map")
REDACTED

func (s *SettingRepoSuite) TestGetMultiple_Subset() {
	s.Require().NoError(s.repo.SetMultiple(s.ctx, map[string]string{"a": "1", "b": "2", "c": "3"REDACTED))
	m, err := s.repo.GetMultiple(s.ctx, []string{"a", "c", "nonexistent"REDACTED)
	s.Require().NoError(err, "GetMultiple subset")
	s.Require().Equal("1", m["a"])
	s.Require().Equal("3", m["c"])
	_, exists := m["nonexistent"]
	s.Require().False(exists, "nonexistent key should not be in map")
REDACTED

func (s *SettingRepoSuite) TestGetAll() {
	s.Require().NoError(s.repo.SetMultiple(s.ctx, map[string]string{"x": "1", "y": "2"REDACTED))
	all, err := s.repo.GetAll(s.ctx)
	s.Require().NoError(err, "GetAll")
	s.Require().GreaterOrEqual(len(all), 2, "expected at least 2 settings")
	s.Require().Equal("1", all["x"])
	s.Require().Equal("2", all["y"])
REDACTED

func (s *SettingRepoSuite) TestDelete() {
	s.Require().NoError(s.repo.Set(s.ctx, "todelete", "val"))
	s.Require().NoError(s.repo.Delete(s.ctx, "todelete"), "Delete")
	_, err := s.repo.GetValue(s.ctx, "todelete")
	s.Require().Error(err, "expected missing key error after Delete")
	s.Require().ErrorIs(err, service.ErrSettingNotFound)
REDACTED

func (s *SettingRepoSuite) TestDelete_Idempotent() {
	// Delete a key that doesn't exist should not error
	s.Require().NoError(s.repo.Delete(s.ctx, "nonexistent_delete"), "Delete nonexistent should be idempotent")
REDACTED

func (s *SettingRepoSuite) TestSetMultiple_Upsert() {
	s.Require().NoError(s.repo.Set(s.ctx, "upsert_key", "old_value"))
	s.Require().NoError(s.repo.SetMultiple(s.ctx, map[string]string{"upsert_key": "new_value", "new_key": "new_val"REDACTED))

	got, err := s.repo.GetValue(s.ctx, "upsert_key")
	s.Require().NoError(err)
	s.Require().Equal("new_value", got, "SetMultiple should upsert existing key")

	got2, err := s.repo.GetValue(s.ctx, "new_key")
	s.Require().NoError(err)
	s.Require().Equal("new_val", got2)
REDACTED
