package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/errorpassthroughrule"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type errorPassthroughRepository struct {
	client *ent.Client
REDACTED

// NewErrorPassthroughRepository 创建错误透传规则仓库
func NewErrorPassthroughRepository(client *ent.Client) service.ErrorPassthroughRepository {
	return &errorPassthroughRepository{client: clientREDACTED
REDACTED

// List 获取所有规则
func (r *errorPassthroughRepository) List(ctx context.Context) ([]*model.ErrorPassthroughRule, error) {
	rules, err := r.client.ErrorPassthroughRule.Query().
		Order(ent.Asc(errorpassthroughrule.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	result := make([]*model.ErrorPassthroughRule, len(rules))
	for i, rule := range rules {
		result[i] = r.toModel(rule)
REDACTED
	return result, nil
REDACTED

// GetByID 根据 ID 获取规则
func (r *errorPassthroughRepository) GetByID(ctx context.Context, id int64) (*model.ErrorPassthroughRule, error) {
	rule, err := r.client.ErrorPassthroughRule.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
	REDACTED
		return nil, err
REDACTED
	return r.toModel(rule), nil
REDACTED

// Create 创建规则
func (r *errorPassthroughRepository) Create(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	builder := r.client.ErrorPassthroughRule.Create().
		SetName(rule.Name).
		SetEnabled(rule.Enabled).
		SetPriority(rule.Priority).
		SetMatchMode(rule.MatchMode).
		SetPassthroughCode(rule.PassthroughCode).
		SetPassthroughBody(rule.PassthroughBody).
		SetSkipMonitoring(rule.SkipMonitoring)

	if len(rule.ErrorCodes) > 0 {
		builder.SetErrorCodes(rule.ErrorCodes)
REDACTED
	if len(rule.Keywords) > 0 {
		builder.SetKeywords(rule.Keywords)
REDACTED
	if len(rule.Platforms) > 0 {
		builder.SetPlatforms(rule.Platforms)
REDACTED
	if rule.ResponseCode != nil {
		builder.SetResponseCode(*rule.ResponseCode)
REDACTED
	if rule.CustomMessage != nil {
		builder.SetCustomMessage(*rule.CustomMessage)
REDACTED
	if rule.Description != nil {
		builder.SetDescription(*rule.Description)
REDACTED

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.toModel(created), nil
REDACTED

// Update 更新规则
func (r *errorPassthroughRepository) Update(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	builder := r.client.ErrorPassthroughRule.UpdateOneID(rule.ID).
		SetName(rule.Name).
		SetEnabled(rule.Enabled).
		SetPriority(rule.Priority).
		SetMatchMode(rule.MatchMode).
		SetPassthroughCode(rule.PassthroughCode).
		SetPassthroughBody(rule.PassthroughBody).
		SetSkipMonitoring(rule.SkipMonitoring)

	// 处理可选字段
	if len(rule.ErrorCodes) > 0 {
		builder.SetErrorCodes(rule.ErrorCodes)
REDACTED else {
		builder.ClearErrorCodes()
REDACTED
	if len(rule.Keywords) > 0 {
		builder.SetKeywords(rule.Keywords)
REDACTED else {
		builder.ClearKeywords()
REDACTED
	if len(rule.Platforms) > 0 {
		builder.SetPlatforms(rule.Platforms)
REDACTED else {
		builder.ClearPlatforms()
REDACTED
	if rule.ResponseCode != nil {
		builder.SetResponseCode(*rule.ResponseCode)
REDACTED else {
		builder.ClearResponseCode()
REDACTED
	if rule.CustomMessage != nil {
		builder.SetCustomMessage(*rule.CustomMessage)
REDACTED else {
		builder.ClearCustomMessage()
REDACTED
	if rule.Description != nil {
		builder.SetDescription(*rule.Description)
REDACTED else {
		builder.ClearDescription()
REDACTED

	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.toModel(updated), nil
REDACTED

// Delete 删除规则
func (r *errorPassthroughRepository) Delete(ctx context.Context, id int64) error {
	return r.client.ErrorPassthroughRule.DeleteOneID(id).Exec(ctx)
REDACTED

// toModel 将 Ent 实体转换为服务模型
func (r *errorPassthroughRepository) toModel(e *ent.ErrorPassthroughRule) *model.ErrorPassthroughRule {
	rule := &model.ErrorPassthroughRule{
		ID:              int64(e.ID),
		Name:            e.Name,
		Enabled:         e.Enabled,
		Priority:        e.Priority,
		ErrorCodes:      e.ErrorCodes,
		Keywords:        e.Keywords,
		MatchMode:       e.MatchMode,
		Platforms:       e.Platforms,
		PassthroughCode: e.PassthroughCode,
		PassthroughBody: e.PassthroughBody,
		SkipMonitoring:  e.SkipMonitoring,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
REDACTED

	if e.ResponseCode != nil {
		rule.ResponseCode = e.ResponseCode
REDACTED
	if e.CustomMessage != nil {
		rule.CustomMessage = e.CustomMessage
REDACTED
	if e.Description != nil {
		rule.Description = e.Description
REDACTED

	// 确保切片不为 nil
	if rule.ErrorCodes == nil {
		rule.ErrorCodes = []int{REDACTED
REDACTED
	if rule.Keywords == nil {
		rule.Keywords = []string{REDACTED
REDACTED
	if rule.Platforms == nil {
		rule.Platforms = []string{REDACTED
REDACTED

	return rule
REDACTED
