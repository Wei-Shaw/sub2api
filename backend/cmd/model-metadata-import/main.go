// model-metadata-import validates and atomically imports the Bestloong model catalog.
//
// Usage (from backend/):
//
//	go run ./cmd/model-metadata-import -input model-metadata.json
//	go run ./cmd/model-metadata-import -input model-metadata.json -allow-incomplete
//	go run ./cmd/model-metadata-import -input model-metadata.json -check-only
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func main() {
	inputPath := flag.String("input", "", "模型元数据 JSON 文件路径")
	checkOnly := flag.Bool("check-only", false, "只校验文件，不连接数据库或写入")
	allowIncomplete := flag.Bool("allow-incomplete", false, "临时导入不完整目录，缺失业务字段标记为待补充")
	flag.Parse()
	if strings.TrimSpace(*inputPath) == "" {
		fail("用法: model-metadata-import -input <文件.json> [-check-only]")
	}
	raw, err := os.ReadFile(*inputPath)
	if err != nil {
		fail("读取文件失败: %v", err)
	}
	var records []service.ModelMetadata
	if *allowIncomplete {
		records, err = service.ParseAndNormalizeIncompleteModelMetadata(raw)
	} else {
		records, err = service.ParseAndValidateModelMetadata(raw, nil)
	}
	if err != nil {
		fail("%v", err)
	}
	if *checkOnly {
		fmt.Printf("校验通过：%d 个模型，未写入数据库。\n", len(records))
		return
	}

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		fail("加载配置失败: %v", err)
	}
	client, db, err := repository.InitEnt(cfg)
	if err != nil {
		fail("连接数据库失败: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT name FROM groups WHERE deleted_at IS NULL`)
	if err != nil {
		fail("读取现网分组失败: %v", err)
	}
	knownGroups := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			fail("读取现网分组失败: %v", err)
		}
		knownGroups[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		fail("读取现网分组失败: %v", err)
	}
	if err := rows.Err(); err != nil {
		fail("读取现网分组失败: %v", err)
	}

	if *allowIncomplete {
		// Draft imports intentionally use the wildcard group marker. This keeps
		// the catalog visible until the business authorization mapping is supplied.
		if _, err := service.ParseAndNormalizeIncompleteModelMetadata(raw); err != nil {
			fail("%v", err)
		}
	} else {
		records, err = service.ParseAndValidateModelMetadata(raw, knownGroups)
		if err != nil {
			fail("%v", err)
		}
	}
	canonical, err := service.MarshalModelMetadata(records)
	if err != nil {
		fail("整理元数据失败: %v", err)
	}
	repo := repository.NewSettingRepository(client)
	if err := repo.Set(ctx, service.ModelMetadataSettingKey, string(canonical)); err != nil {
		fail("写入数据库失败: %v", err)
	}
	if *allowIncomplete {
		fmt.Printf("临时导入成功：%d 个模型。缺失业务字段已标记为「%s」，后续补齐后可重新正式导入。\n", len(records), service.IncompleteMetadataPlaceholder)
	} else {
		fmt.Printf("导入成功：%d 个模型。旧目录已整批替换。\n", len(records))
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
