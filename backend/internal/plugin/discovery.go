package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DiscoveredPlugin 代表磁盘上扫描到的一个插件,尚未启动。
type DiscoveredPlugin struct {
	Name       string // 插件名称(目录名)
	BinaryPath string // 可执行文件绝对路径
}

// DiscoverPlugins 扫描 pluginsDir 下所有子目录,寻找与目录同名的可执行文件。
// 命名规则:
//   - 目录: {pluginsDir}/{name}/
//   - 二进制(Windows): {name}.exe
//   - 二进制(其他平台): {name}
//
// 不存在或不可执行的二进制会被跳过(扫描过程不返回错误,由上层在启动时再次校验)。
func DiscoverPlugins(pluginsDir string) ([]DiscoveredPlugin, error) {
	if pluginsDir == "" {
		return nil, errors.New("plugins directory is empty")
	}
	info, err := os.Stat(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat plugins dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("plugins path is not a directory: %s", pluginsDir)
	}

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}

	out := make([]DiscoveredPlugin, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 忽略隐藏目录与临时目录,避免误启动备份文件。
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}

		binPath := filepath.Join(pluginsDir, name, pluginBinaryName(name))
		st, err := os.Stat(binPath)
		if err != nil || st.IsDir() {
			continue
		}
		out = append(out, DiscoveredPlugin{
			Name:       name,
			BinaryPath: binPath,
		})
	}
	return out, nil
}

// pluginBinaryName 根据当前操作系统返回插件二进制文件名。
func pluginBinaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
