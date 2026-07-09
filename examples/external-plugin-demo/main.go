// external-plugin-demo 是 sub2api 外部插件的最小可用示例：
//   - 经 SDK 在宿主指定的 unix socket 上起 HTTP 服务（/healthz 由 SDK 内置）；
//   - GET /user/info 返回 JSON（对外路径 /api/v1/plugins/demo-external/api/info）；
//   - GET /user/stream 发 3 条 SSE 后关闭（验证宿主反代的流式透传）；
//   - 启动时经能力 API 做一次 KV 写读（boot_count 自增）并写一条宿主日志。
//
// 打包见 build.sh，清单见 manifest.json。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pluginhost/sdk"
)

// demoConfig 是本插件的私有配置（与 manifest.json 的 config_schema 对应）。
type demoConfig struct {
	Greeting string `json:"greeting"`
}

func main() {
	var cfg demoConfig
	if err := sdk.Config(&cfg); err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.Greeting == "" {
		cfg.Greeting = "hello from external demo plugin"
	}

	bootCount, err := recordBoot()
	if err != nil {
		// 能力面异常不阻塞插件启动：核心服务是 HTTP 面，能力演练失败仅记日志。
		log.Printf("capability warm-up failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /user/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plugin":     "demo-external",
			"greeting":   cfg.Greeting,
			"boot_count": bootCount,
		})
	})
	mux.HandleFunc("GET /user/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		for seq := 1; seq <= 3; seq++ {
			if _, err := fmt.Fprintf(w, "data: {\"seq\":%d}\n\n", seq); err != nil {
				return
			}
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	})

	if err := sdk.Serve(mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// recordBoot 演练能力面：KV 读写一次（boot_count 自增）+ 写一条宿主日志。
func recordBoot() (int, error) {
	client, err := sdk.NewClient()
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const key = "boot_count"
	count := 0
	if value, found, err := client.KVGet(ctx, key); err != nil {
		return 0, err
	} else if found {
		if count, err = strconv.Atoi(value); err != nil {
			count = 0
		}
	}
	count++
	if err := client.KVSet(ctx, key, strconv.Itoa(count)); err != nil {
		return 0, err
	}
	if err := client.Log(ctx, "info", "demo external plugin started",
		map[string]any{"boot_count": count}); err != nil {
		return count, err
	}
	return count, nil
}
