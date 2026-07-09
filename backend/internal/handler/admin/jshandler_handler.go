package admin

import (
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
)

type JSHandlerAdminHandler struct {
	svc *jshandler.Service
}

func NewJSHandlerAdminHandler(svc *jshandler.Service) *JSHandlerAdminHandler {
	return &JSHandlerAdminHandler{svc: svc}
}

func (h *JSHandlerAdminHandler) GetConfig(c *gin.Context) {
	raw, err := h.svc.ConfigJSON(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"config": string(raw)})
}

func (h *JSHandlerAdminHandler) UpdateConfig(c *gin.Context) {
	var cfg jshandler.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updated, err := h.svc.UpdateConfig(c.Request.Context(), cfg)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

func (h *JSHandlerAdminHandler) ListScripts(c *gin.Context) {
	scripts, err := h.svc.ListScripts()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"scripts": scripts})
}

func (h *JSHandlerAdminHandler) UploadScript(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "file is required")
		return
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".js") {
		response.BadRequest(c, "only .js files are allowed")
		return
	}
	f, err := file.Open()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, int64(jshandler.MaxScriptUploadBytes())+1))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if len(content) > jshandler.MaxScriptUploadBytes() {
		response.BadRequest(c, "script file too large")
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = strings.TrimSuffix(file.Filename, ".js")
	}
	entry, err := h.svc.AddScript(name, content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, entry)
}

func (h *JSHandlerAdminHandler) DeleteScript(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "id is required")
		return
	}
	if err := h.svc.DeleteScript(id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}