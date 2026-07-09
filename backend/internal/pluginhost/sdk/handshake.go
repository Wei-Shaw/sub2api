package sdk

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// handshake 是宿主拉起插件进程时经 stdin 注入的一行 JSON：
//
//	{"token":"<能力面 Bearer token>","config":{...插件私有配置...}}
//
// 机密走 stdin 而非环境变量：同 OS 用户下任何进程都能读 /proc/<pid>/environ，
// env 携带 token/config 会击穿插件间按 ID 硬隔离；stdin 只有父子进程可见。
// 宿主写完一行即关闭写端，插件读到 EOF 不影响已缓存的握手内容。
type handshake struct {
	Token  string          `json:"token"`
	Config json.RawMessage `json:"config"`
}

var (
	handshakeOnce sync.Once
	handshakeVal  handshake
	handshakeErr  error
)

// readHandshake 读取并缓存宿主握手（进程生命周期内只读一次；
// 配置变更由宿主重启进程重新注入）。
func readHandshake() (handshake, error) {
	handshakeOnce.Do(func() {
		line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
		if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
			handshakeErr = fmt.Errorf("sdk: read host handshake from stdin: %w (must be launched by sub2api plugin host)", err)
			return
		}
		if err := json.Unmarshal(line, &handshakeVal); err != nil {
			handshakeErr = fmt.Errorf("sdk: decode host handshake: %w", err)
			return
		}
		if handshakeVal.Token == "" {
			handshakeErr = errors.New("sdk: host handshake has no token (must be launched by sub2api plugin host)")
		}
	})
	return handshakeVal, handshakeErr
}
