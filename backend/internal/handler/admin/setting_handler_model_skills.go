package admin

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const maxModelSkillUploadBytes int64 = 512 * 1024

type modelSkillRequest struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

func (h *SettingHandler) ListModelSkills(c *gin.Context) {
	skills, err := h.settingService.GetModelSkills(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, skills)
}

func (h *SettingHandler) CreateModelSkill(c *gin.Context) {
	var req modelSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "SKILL 请求格式无效")
		return
	}
	skill, err := h.settingService.CreateModelSkill(c.Request.Context(), req.Name, req.Filename, req.Content)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, skill)
}

func (h *SettingHandler) UploadModelSkill(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxModelSkillUploadBytes+1024)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择有效的 SKILL 文件")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxModelSkillUploadBytes+1))
	if err != nil || int64(len(content)) > maxModelSkillUploadBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "SKILL 文件不能超过 512KB")
		return
	}
	filename := ""
	if header != nil {
		filename = filepath.Base(strings.TrimSpace(header.Filename))
	}
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	if override := strings.TrimSpace(c.PostForm("name")); override != "" {
		name = override
	}
	skill, err := h.settingService.CreateModelSkill(c.Request.Context(), name, filename, string(content))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, skill)
}

func (h *SettingHandler) UpdateModelSkill(c *gin.Context) {
	var req modelSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "SKILL 请求格式无效")
		return
	}
	skill, err := h.settingService.UpdateModelSkill(c.Request.Context(), c.Param("id"), req.Name, req.Filename, req.Content)
	if err != nil {
		if err == service.ErrSettingNotFound {
			response.NotFound(c, "SKILL 不存在")
			return
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, skill)
}

func (h *SettingHandler) DeleteModelSkill(c *gin.Context) {
	if err := h.settingService.DeleteModelSkill(c.Request.Context(), c.Param("id")); err != nil {
		if err == service.ErrSettingNotFound {
			response.NotFound(c, "SKILL 不存在")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "SKILL 已删除"})
}
