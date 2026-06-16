// Package node 实现节点上报服务，定期向远程控制服务器发送心跳并同步账号状态快照。
package node

// xiugai 添加节点功能

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config 保存节点上报服务的配置信息，从当前目录的 .env 文件中加载。
// 节点内网 IP 不在此处配置，由上报服务启动时自动从系统网络接口检测。
type Config struct {
	// NodeName 节点唯一名称，用于在控制服务器上标识本节点。
	NodeName string
	// ControlAddr 控制服务器的基础 URL，例如 https://127.0.0.1:8443。
	ControlAddr string
	// NodePort 本节点的监听端口，在心跳请求中上报给控制服务器。
	NodePort int
	// TLSCert 客户端证书文件路径，启用 mTLS 双向认证时使用。
	TLSCert string
	// TLSKey 客户端私钥文件路径，与 TLSCert 配套使用。
	TLSKey string
	// TLSCA CA 证书文件路径，用于校验控制服务器的 TLS 证书。
	TLSCA string
}

// LoadConfig 从指定路径的 .env 文件读取并解析节点配置。
//
// 必填项：NODE_NAME（节点名称）、CONTROL_ADDR（控制服务器地址）。
// 可选项：NODE_PORT（本节点监听端口）、TLS_CERT（客户端证书）、
//
//	TLS_KEY（客户端私钥）、TLS_CA（CA 证书）。
//
// 节点内网 IP 不在此处读取，由上报服务启动时自动从系统网络接口检测。
//
// 参数：
//   - path：.env 文件路径，通常为当前工作目录下的 ".env"。
//
// 返回值：
//   - *Config：解析成功后的配置对象。
//   - error：文件不存在、读取失败或必填项缺失时返回对应错误。
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// 逐行解析 KEY=VALUE 格式，忽略空行和 # 开头的注释行。
	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// 去除可选的单引号或双引号包裹。
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		env[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	cfg := &Config{
		NodeName:    env["NODE_NAME"],
		ControlAddr: strings.TrimRight(env["CONTROL_ADDR"], "/"),
		TLSCert:     env["TLS_CERT"],
		TLSKey:      env["TLS_KEY"],
		TLSCA:       env["TLS_CA"],
	}

	if cfg.NodeName == "" {
		return nil, fmt.Errorf("NODE_NAME is required in %s", path)
	}
	if cfg.ControlAddr == "" {
		return nil, fmt.Errorf("CONTROL_ADDR is required in %s", path)
	}

	// NODE_PORT 为可选项，若填写则校验合法范围。
	if portStr := env["NODE_PORT"]; portStr != "" {
		port, convErr := strconv.Atoi(portStr)
		if convErr != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("NODE_PORT must be a valid port number (1-65535)")
		}
		cfg.NodePort = port
	}

	return cfg, nil
}

// end
