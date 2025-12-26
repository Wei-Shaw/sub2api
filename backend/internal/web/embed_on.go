//go:build embed

package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var frontendFS embed.FS

func ServeEmbeddedFrontend() gin.HandlerFunc {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		panic("failed to get dist subdirectory: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/v1/") ||
			strings.HasPrefix(path, "/v1beta/") ||
			strings.HasPrefix(path, "/setup/") ||
			path == "/health" ||
			path == "/responses" {
			c.Next()
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		if file, err := distFS.Open(cleanPath); err == nil {
			_ = file.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}

		serveIndexHTML(c, distFS)
	}
}

func serveIndexHTML(c *gin.Context, fsys fs.FS) {
	file, err := fsys.Open("index.html")
	if err != nil {
		c.String(http.StatusNotFound, "Frontend not found")
		c.Abort()
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to read index.html")
		c.Abort()
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	c.Abort()
}

func HasEmbeddedFrontend() bool {
	_, err := frontendFS.ReadFile("dist/index.html")
	return err == nil
}
