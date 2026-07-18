package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/lib/pq"
)

const classifierVersion = "ingress-reject-v1"

type candidate struct {
	id         int64
	statusCode int
	message    string
	body       string
}

func main() {
	beforeRaw := flag.String("before", "", "required RFC3339 cutoff; only older rows are considered")
	execute := flag.Bool("execute", false, "delete matched rows (default is dry-run)")
	batchSize := flag.Int("batch-size", 5000, "scan/delete batch size (1-5000)")
	flag.Parse()

	if *beforeRaw == "" {
		log.Fatal("--before is required")
	}
	before, err := time.Parse(time.RFC3339, *beforeRaw)
	if err != nil {
		log.Fatalf("invalid --before: %v", err)
	}
	if *batchSize < 1 || *batchSize > 5000 {
		log.Fatal("--batch-size must be between 1 and 5000")
	}

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	client, db, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	counts, scanned, matched, deleted, err := cleanup(ctx, db, before, *batchSize, *execute)
	if err != nil {
		log.Fatalf("cleanup failed: %v", err)
	}

	digest := sha256.Sum256([]byte(classifierVersion))
	mode := "dry-run"
	if *execute {
		mode = "execute"
	}
	fmt.Printf("mode=%s before=%s classifier=%s scanned=%d matched=%d deleted=%d\n",
		mode, before.UTC().Format(time.RFC3339), hex.EncodeToString(digest[:]), scanned, matched, deleted)
	reasons := make([]string, 0, len(counts))
	for reason := range counts {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		fmt.Printf("reason=%s count=%d\n", reason, counts[reason])
	}
	if *execute && deleted > 0 {
		fmt.Println("cleanup complete; schedule VACUUM (ANALYZE) ops_error_logs during normal maintenance")
	}
}

func cleanup(ctx context.Context, db *sql.DB, before time.Time, batchSize int, execute bool) (map[string]int64, int64, int64, int64, error) {
	counts := make(map[string]int64)
	var cursor, scanned, matched, deleted int64
	for {
		rows, err := db.QueryContext(ctx, `
			SELECT id, COALESCE(status_code, 0), COALESCE(error_message, ''), COALESCE(error_body, '')
			FROM ops_error_logs
			WHERE id > $1
			  AND created_at < $2
			  AND error_phase = 'auth'
			  AND account_id IS NULL
			  AND upstream_status_code IS NULL
			  AND COALESCE(upstream_error_message, '') = ''
			  AND COALESCE(upstream_error_detail, '') = ''
			ORDER BY id ASC
			LIMIT $3`, cursor, before, batchSize)
		if err != nil {
			return nil, scanned, matched, deleted, err
		}

		batch := make([]candidate, 0, batchSize)
		for rows.Next() {
			var item candidate
			if err := rows.Scan(&item.id, &item.statusCode, &item.message, &item.body); err != nil {
				_ = rows.Close()
				return nil, scanned, matched, deleted, err
			}
			batch = append(batch, item)
			cursor = item.id
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, scanned, matched, deleted, err
		}
		_ = rows.Close()
		if len(batch) == 0 {
			break
		}

		ids := make([]int64, 0, len(batch))
		for _, item := range batch {
			scanned++
			if reason, ok := historicalIngressRejectReason(item); ok {
				matched++
				counts[reason]++
				ids = append(ids, item.id)
			}
		}
		if execute && len(ids) > 0 {
			result, err := db.ExecContext(ctx,
				`DELETE FROM ops_error_logs WHERE id = ANY($1) AND created_at < $2`, pq.Array(ids), before)
			if err != nil {
				return nil, scanned, matched, deleted, err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return nil, scanned, matched, deleted, err
			}
			deleted += n
		}
	}
	return counts, scanned, matched, deleted, nil
}

func historicalIngressRejectReason(item candidate) (string, bool) {
	code, message := parseErrorIdentity(item.body, item.message)
	switch code {
	case "API_KEY_REQUIRED":
		return "missing_key", true
	case "INVALID_API_KEY":
		return "invalid_key", true
	case "API_KEY_DISABLED":
		return "key_disabled", true
	case "USER_INACTIVE":
		return "user_inactive", true
	case "GROUP_DELETED":
		return "group_deleted", true
	case "GROUP_DISABLED":
		return "group_disabled", true
	case "GROUP_NOT_ALLOWED":
		return "group_forbidden", true
	case "ACCESS_DENIED":
		return "ip_acl_denied", true
	case "api_key_in_query_deprecated":
		return "query_key_deprecated", true
	}

	normalized := strings.TrimSpace(message)
	switch {
	case normalized == "API key is required":
		return "missing_key", true
	case normalized == "Invalid API key":
		return "invalid_key", true
	case normalized == "API key is disabled":
		return "key_disabled", true
	case normalized == "User account is not active":
		return "user_inactive", true
	case normalized == "API Key 所属分组已删除":
		return "group_deleted", true
	case normalized == "API Key 所属分组已停用":
		return "group_disabled", true
	case normalized == "API Key 所属专属分组不再允许当前用户使用":
		return "group_forbidden", true
	case normalized == "API Key is not assigned to any group and cannot be used. Please contact the administrator to assign it to a group.":
		return "group_unassigned", true
	case strings.HasPrefix(normalized, "Access denied. Your IP is "):
		return "ip_acl_denied", true
	case normalized == "Query parameter api_key is deprecated. Use Authorization header or key instead.":
		return "query_key_deprecated", true
	default:
		return "", false
	}
}

func parseErrorIdentity(body, fallbackMessage string) (string, string) {
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return "", fallbackMessage
	}
	message := payload.Message
	if message == "" {
		message = payload.Error.Message
	}
	if message == "" {
		message = fallbackMessage
	}
	return strings.TrimSpace(payload.Code), message
}
