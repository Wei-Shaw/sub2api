package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/tlsfingerprintprofile"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type tlsFingerprintProfileRepository struct {
	client *ent.Client
REDACTED

// NewTLSFingerprintProfileRepository 创建 TLS 指纹模板仓库
func NewTLSFingerprintProfileRepository(client *ent.Client) service.TLSFingerprintProfileRepository {
	return &tlsFingerprintProfileRepository{client: clientREDACTED
REDACTED

// List 获取所有模板
func (r *tlsFingerprintProfileRepository) List(ctx context.Context) ([]*model.TLSFingerprintProfile, error) {
	profiles, err := r.client.TLSFingerprintProfile.Query().
		Order(ent.Asc(tlsfingerprintprofile.FieldName)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	result := make([]*model.TLSFingerprintProfile, len(profiles))
	for i, p := range profiles {
		result[i] = r.toModel(p)
REDACTED
	return result, nil
REDACTED

// GetByID 根据 ID 获取模板
func (r *tlsFingerprintProfileRepository) GetByID(ctx context.Context, id int64) (*model.TLSFingerprintProfile, error) {
	p, err := r.client.TLSFingerprintProfile.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
	REDACTED
		return nil, err
REDACTED
	return r.toModel(p), nil
REDACTED

// Create 创建模板
func (r *tlsFingerprintProfileRepository) Create(ctx context.Context, p *model.TLSFingerprintProfile) (*model.TLSFingerprintProfile, error) {
	builder := r.client.TLSFingerprintProfile.Create().
		SetName(p.Name).
		SetEnableGrease(p.EnableGREASE)

	if p.Description != nil {
		builder.SetDescription(*p.Description)
REDACTED
	if len(p.CipherSuites) > 0 {
		builder.SetCipherSuites(p.CipherSuites)
REDACTED
	if len(p.Curves) > 0 {
		builder.SetCurves(p.Curves)
REDACTED
	if len(p.PointFormats) > 0 {
		builder.SetPointFormats(p.PointFormats)
REDACTED
	if len(p.SignatureAlgorithms) > 0 {
		builder.SetSignatureAlgorithms(p.SignatureAlgorithms)
REDACTED
	if len(p.ALPNProtocols) > 0 {
		builder.SetAlpnProtocols(p.ALPNProtocols)
REDACTED
	if len(p.SupportedVersions) > 0 {
		builder.SetSupportedVersions(p.SupportedVersions)
REDACTED
	if len(p.KeyShareGroups) > 0 {
		builder.SetKeyShareGroups(p.KeyShareGroups)
REDACTED
	if len(p.PSKModes) > 0 {
		builder.SetPskModes(p.PSKModes)
REDACTED
	if len(p.Extensions) > 0 {
		builder.SetExtensions(p.Extensions)
REDACTED

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.toModel(created), nil
REDACTED

// Update 更新模板
func (r *tlsFingerprintProfileRepository) Update(ctx context.Context, p *model.TLSFingerprintProfile) (*model.TLSFingerprintProfile, error) {
	builder := r.client.TLSFingerprintProfile.UpdateOneID(p.ID).
		SetName(p.Name).
		SetEnableGrease(p.EnableGREASE)

	if p.Description != nil {
		builder.SetDescription(*p.Description)
REDACTED else {
		builder.ClearDescription()
REDACTED

	if len(p.CipherSuites) > 0 {
		builder.SetCipherSuites(p.CipherSuites)
REDACTED else {
		builder.ClearCipherSuites()
REDACTED
	if len(p.Curves) > 0 {
		builder.SetCurves(p.Curves)
REDACTED else {
		builder.ClearCurves()
REDACTED
	if len(p.PointFormats) > 0 {
		builder.SetPointFormats(p.PointFormats)
REDACTED else {
		builder.ClearPointFormats()
REDACTED
	if len(p.SignatureAlgorithms) > 0 {
		builder.SetSignatureAlgorithms(p.SignatureAlgorithms)
REDACTED else {
		builder.ClearSignatureAlgorithms()
REDACTED
	if len(p.ALPNProtocols) > 0 {
		builder.SetAlpnProtocols(p.ALPNProtocols)
REDACTED else {
		builder.ClearAlpnProtocols()
REDACTED
	if len(p.SupportedVersions) > 0 {
		builder.SetSupportedVersions(p.SupportedVersions)
REDACTED else {
		builder.ClearSupportedVersions()
REDACTED
	if len(p.KeyShareGroups) > 0 {
		builder.SetKeyShareGroups(p.KeyShareGroups)
REDACTED else {
		builder.ClearKeyShareGroups()
REDACTED
	if len(p.PSKModes) > 0 {
		builder.SetPskModes(p.PSKModes)
REDACTED else {
		builder.ClearPskModes()
REDACTED
	if len(p.Extensions) > 0 {
		builder.SetExtensions(p.Extensions)
REDACTED else {
		builder.ClearExtensions()
REDACTED

	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.toModel(updated), nil
REDACTED

// Delete 删除模板
func (r *tlsFingerprintProfileRepository) Delete(ctx context.Context, id int64) error {
	return r.client.TLSFingerprintProfile.DeleteOneID(id).Exec(ctx)
REDACTED

// toModel 将 Ent 实体转换为服务模型
func (r *tlsFingerprintProfileRepository) toModel(e *ent.TLSFingerprintProfile) *model.TLSFingerprintProfile {
	p := &model.TLSFingerprintProfile{
		ID:                  e.ID,
		Name:                e.Name,
		Description:         e.Description,
		EnableGREASE:        e.EnableGrease,
		CipherSuites:        e.CipherSuites,
		Curves:              e.Curves,
		PointFormats:        e.PointFormats,
		SignatureAlgorithms: e.SignatureAlgorithms,
		ALPNProtocols:       e.AlpnProtocols,
		SupportedVersions:   e.SupportedVersions,
		KeyShareGroups:      e.KeyShareGroups,
		PSKModes:            e.PskModes,
		Extensions:          e.Extensions,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
REDACTED

	// 确保切片不为 nil
	if p.CipherSuites == nil {
		p.CipherSuites = []uint16{REDACTED
REDACTED
	if p.Curves == nil {
		p.Curves = []uint16{REDACTED
REDACTED
	if p.PointFormats == nil {
		p.PointFormats = []uint16{REDACTED
REDACTED
	if p.SignatureAlgorithms == nil {
		p.SignatureAlgorithms = []uint16{REDACTED
REDACTED
	if p.ALPNProtocols == nil {
		p.ALPNProtocols = []string{REDACTED
REDACTED
	if p.SupportedVersions == nil {
		p.SupportedVersions = []uint16{REDACTED
REDACTED
	if p.KeyShareGroups == nil {
		p.KeyShareGroups = []uint16{REDACTED
REDACTED
	if p.PSKModes == nil {
		p.PSKModes = []uint16{REDACTED
REDACTED
	if p.Extensions == nil {
		p.Extensions = []uint16{REDACTED
REDACTED

	return p
REDACTED
