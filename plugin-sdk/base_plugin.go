package pluginsdk

import (
	"io/fs"
	"log/slog"
	"path"
	"sync/atomic"
)

// BasePlugin provides the common lifecycle scaffolding shared by all
// gateway plugins: PluginContext storage, logger access, and frontend
// asset serving. Embed it in a plugin struct and call SetContext from
// Init; the host then gets working Logger/OpenFrontendFile methods for
// free.
//
// FrontendFS must be set during Init (typically with fs.Sub on an
// embed.FS) so OpenFrontendFile can resolve manifest-relative paths
// such as "dist/entry.js" without each plugin re-implementing path
// sanitisation.
type BasePlugin struct {
	ctx atomic.Pointer[PluginContext]

	// FrontendFS is the file system used by OpenFrontendFile. Plugins
	// should assign an fs.Sub of their embed.FS rooted where manifest
	// EntryJS / EntryCSS paths resolve (commonly the "frontend"
	// directory containing "dist/entry.js").
	FrontendFS fs.FS
}

// SetContext stores the SDK-supplied PluginContext for later access via
// Context / Logger. Call this once from Plugin.Init.
func (b *BasePlugin) SetContext(ctx PluginContext) {
	b.ctx.Store(&ctx)
}

// Context returns the live PluginContext or nil if Init has not run yet.
func (b *BasePlugin) Context() PluginContext {
	c := b.ctx.Load()
	if c == nil {
		return nil
	}
	return *c
}

// Logger returns the plugin logger or slog.Default when Init has not
// run yet (e.g. during early bootstrapping or in tests).
func (b *BasePlugin) Logger() *slog.Logger {
	if ctx := b.Context(); ctx != nil {
		return ctx.Logger()
	}
	return slog.Default()
}

// OpenFrontendFile implements pluginsdk.FrontendBundleProvider. It
// resolves rel against FrontendFS after stripping any leading slash and
// rejecting empty or "." paths so callers cannot read directory roots.
func (b *BasePlugin) OpenFrontendFile(rel string) ([]byte, error) {
	if b.FrontendFS == nil {
		return nil, fs.ErrNotExist
	}
	clean := path.Clean("/" + rel)
	if clean == "/" || clean == "/." {
		return nil, fs.ErrInvalid
	}
	clean = clean[1:] // strip leading "/"
	return fs.ReadFile(b.FrontendFS, clean)
}
