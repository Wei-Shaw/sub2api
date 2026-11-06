package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Gin contexts belong to their request. Enforce the async boundary across all
// usage entry points, including rarely exercised provider and auxiliary routes.
func TestUsageRecordTasksDoNotCaptureGinContext(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(filename), "*.go"))
	require.NoError(t, err)
	files := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			contexts := make(map[string]bool)
			for _, param := range function.Type.Params.List {
				pointer, ok := param.Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				typeName, ok := pointer.X.(*ast.SelectorExpr)
				if !ok || typeName.Sel.Name != "Context" {
					continue
				}
				pkg, ok := typeName.X.(*ast.Ident)
				if !ok || pkg.Name != "gin" {
					continue
				}
				for _, name := range param.Names {
					contexts[name.Name] = true
				}
			}
			if len(contexts) == 0 {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				method, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch method.Sel.Name {
				case "submitUsageRecordTask", "submitMandatoryUsageRecordTask", "submitOpenAIUsageRecordTask":
				default:
					return true
				}
				for _, arg := range call.Args {
					task, ok := arg.(*ast.FuncLit)
					if !ok {
						continue
					}
					ast.Inspect(task.Body, func(node ast.Node) bool {
						if name, ok := node.(*ast.Ident); ok && contexts[name.Name] {
							t.Errorf("%s: usage task captures gin.Context %q; snapshot required fields before submitting", files.Position(name.Pos()), name.Name)
						}
						return true
					})
				}
				return true
			})
		}
	}
}
