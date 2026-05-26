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
}

// NewTLSFingerprintProfileRepository 创建 TLS 指纹模板仓库
func NewTLSFingerprintProfileRepository(client *ent.Client) service.TLSFingerprintProfileRepository {
	return &tlsFingerprintProfileRepository{client: client}
}

// List 获取所有模板
func (r *tlsFingerprintProfileRepository) List(ctx context.Context) ([]*model.TLSFingerprintProfile, error) {
	profiles, err := r.client.TLSFingerprintProfile.Query().
		Order(ent.Asc(tlsfingerprintprofile.FieldName)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*model.TLSFingerprintProfile, len(profiles))
	for i, p := range profiles {
		result[i] = r.toModel(p)
	}
	return result, nil
}

// GetByID 根据 ID 获取模板
func (r *tlsFingerprintProfileRepository) GetByID(ctx context.Context, id int64) (*model.TLSFingerprintProfile, error) {
	p, err := r.client.TLSFingerprintProfile.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return r.toModel(p), nil
}

// Create 创建模板
func (r *tlsFingerprintProfileRepository) Create(ctx context.Context, p *model.TLSFingerprintProfile) (*model.TLSFingerprintProfile, error) {
	builder := r.client.TLSFingerprintProfile.Create().
		SetName(p.Name).
		SetEnableGrease(p.EnableGREASE)

	if p.Description != nil {
		builder.SetDescription(*p.Description)
	}
	if len(p.CipherSuites) > 0 {
		builder.SetCipherSuites(p.CipherSuites)
	}
	if len(p.Curves) > 0 {
		builder.SetCurves(p.Curves)
	}
	if len(p.PointFormats) > 0 {
		builder.SetPointFormats(p.PointFormats)
	}
	if len(p.SignatureAlgorithms) > 0 {
		builder.SetSignatureAlgorithms(p.SignatureAlgorithms)
	}
	if len(p.ALPNProtocols) > 0 {
		builder.SetAlpnProtocols(p.ALPNProtocols)
	}
	if len(p.SupportedVersions) > 0 {
		builder.SetSupportedVersions(p.SupportedVersions)
	}
	if len(p.KeyShareGroups) > 0 {
		builder.SetKeyShareGroups(p.KeyShareGroups)
	}
	if len(p.PSKModes) > 0 {
		builder.SetPskModes(p.PSKModes)
	}
	if len(p.Extensions) > 0 {
		builder.SetExtensions(p.Extensions)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return r.toModel(created), nil
}

// Update 更新模板
func (r *tlsFingerprintProfileRepository) Update(ctx context.Context, p *model.TLSFingerprintProfile) (*model.TLSFingerprintProfile, error) {
	builder := r.client.TLSFingerprintProfile.UpdateOneID(p.ID).
		SetName(p.Name).
		SetEnableGrease(p.EnableGREASE)

	if p.Description != nil {
		builder.SetDescription(*p.Description)
	} else {
		builder.ClearDescription()
	}

	if len(p.CipherSuites) > 0 {
		builder.SetCipherSuites(p.CipherSuites)
	} else {
		builder.ClearCipherSuites()
	}
	if len(p.Curves) > 0 {
		builder.SetCurves(p.Curves)
	} else {
		builder.ClearCurves()
	}
	if len(p.PointFormats) > 0 {
		builder.SetPointFormats(p.PointFormats)
	} else {
		builder.ClearPointFormats()
	}
	if len(p.SignatureAlgorithms) > 0 {
		builder.SetSignatureAlgorithms(p.SignatureAlgorithms)
	} else {
		builder.ClearSignatureAlgorithms()
	}
	if len(p.ALPNProtocols) > 0 {
		builder.SetAlpnProtocols(p.ALPNProtocols)
	} else {
		builder.ClearAlpnProtocols()
	}
	if len(p.SupportedVersions) > 0 {
		builder.SetSupportedVersions(p.SupportedVersions)
	} else {
		builder.ClearSupportedVersions()
	}
	if len(p.KeyShareGroups) > 0 {
		builder.SetKeyShareGroups(p.KeyShareGroups)
	} else {
		builder.ClearKeyShareGroups()
	}
	if len(p.PSKModes) > 0 {
		builder.SetPskModes(p.PSKModes)
	} else {
		builder.ClearPskModes()
	}
	if len(p.Extensions) > 0 {
		builder.SetExtensions(p.Extensions)
	} else {
		builder.ClearExtensions()
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return r.toModel(updated), nil
}

// Delete 删除模板
func (r *tlsFingerprintProfileRepository) Delete(ctx context.Context, id int64) error {
	return r.client.TLSFingerprintProfile.DeleteOneID(id).Exec(ctx)
}

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
	}

	// 确保切片不为 nil
	if p.CipherSuites == nil {
		p.CipherSuites = []uint16{}
	}
	if p.Curves == nil {
		p.Curves = []uint16{}
	}
	if p.PointFormats == nil {
		p.PointFormats = []uint16{}
	}
	if p.SignatureAlgorithms == nil {
		p.SignatureAlgorithms = []uint16{}
	}
	if p.ALPNProtocols == nil {
		p.ALPNProtocols = []string{}
	}
	if p.SupportedVersions == nil {
		p.SupportedVersions = []uint16{}
	}
	if p.KeyShareGroups == nil {
		p.KeyShareGroups = []uint16{}
	}
	if p.PSKModes == nil {
		p.PSKModes = []uint16{}
	}
	if p.Extensions == nil {
		p.Extensions = []uint16{}
	}

	return p
}
