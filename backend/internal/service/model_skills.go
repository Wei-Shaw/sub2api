package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SettingKeyModelSkills = "model_skills"

// ModelSkill 是管理员维护的可复用模型技能说明。正文按 UTF-8 文本保存，
// 分组只保存 ID，便于修改技能内容后所有引用自动生效。
type ModelSkill struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Filename  string    `json:"filename,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const maxModelSkillContentBytes = 512 * 1024

func (s *SettingService) GetModelSkills(ctx context.Context) ([]ModelSkill, error) {
	if s == nil || s.settingRepo == nil {
		return []ModelSkill{}, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyModelSkills)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return []ModelSkill{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []ModelSkill{}, nil
	}
	var skills []ModelSkill
	if err := json.Unmarshal([]byte(raw), &skills); err != nil {
		return nil, fmt.Errorf("decode model skills: %w", err)
	}
	if skills == nil {
		skills = []ModelSkill{}
	}
	return skills, nil
}

func (s *SettingService) saveModelSkills(ctx context.Context, skills []ModelSkill) error {
	data, err := json.Marshal(skills)
	if err != nil {
		return err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyModelSkills, string(data)); err != nil {
		return err
	}
	return nil
}

func newModelSkillID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func normalizeModelSkillInput(name, filename, content string) (string, string, string, error) {
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if name == "" {
		return "", "", "", errors.New("skill name is required")
	}
	if content == "" {
		return "", "", "", errors.New("skill content is required")
	}
	if len([]byte(content)) > maxModelSkillContentBytes {
		return "", "", "", fmt.Errorf("skill content cannot exceed %d KB", maxModelSkillContentBytes/1024)
	}
	return name, strings.TrimSpace(filename), content, nil
}

func (s *SettingService) CreateModelSkill(ctx context.Context, name, filename, content string) (*ModelSkill, error) {
	name, filename, content, err := normalizeModelSkillInput(name, filename, content)
	if err != nil {
		return nil, err
	}
	skills, err := s.GetModelSkills(ctx)
	if err != nil {
		return nil, err
	}
	id, err := newModelSkillID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	skill := ModelSkill{ID: id, Name: name, Filename: filename, Content: content, CreatedAt: now, UpdatedAt: now}
	skills = append(skills, skill)
	if err := s.saveModelSkills(ctx, skills); err != nil {
		return nil, err
	}
	return &skill, nil
}

func (s *SettingService) UpdateModelSkill(ctx context.Context, id, name, filename, content string) (*ModelSkill, error) {
	name, filename, content, err := normalizeModelSkillInput(name, filename, content)
	if err != nil {
		return nil, err
	}
	skills, err := s.GetModelSkills(ctx)
	if err != nil {
		return nil, err
	}
	for i := range skills {
		if skills[i].ID != id {
			continue
		}
		skills[i].Name, skills[i].Filename, skills[i].Content = name, filename, content
		skills[i].UpdatedAt = time.Now().UTC()
		if err := s.saveModelSkills(ctx, skills); err != nil {
			return nil, err
		}
		return &skills[i], nil
	}
	return nil, ErrSettingNotFound
}

func (s *SettingService) DeleteModelSkill(ctx context.Context, id string) error {
	skills, err := s.GetModelSkills(ctx)
	if err != nil {
		return err
	}
	filtered := skills[:0]
	found := false
	for _, skill := range skills {
		if skill.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, skill)
	}
	if !found {
		return ErrSettingNotFound
	}
	return s.saveModelSkills(ctx, filtered)
}
