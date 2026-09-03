package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// 本文件实现两个启动阶段的一次性步骤，都围绕应用级密钥加密主密钥：
//
//  1. ensureSecretEncryptionKeyUsable —— 主密钥为空时，检查数据库里是否已经有
//     东西可丢。有则阻断启动并点名要设置的变量；没有则放行（全新安装不该为一把
//     它还用不到的密钥被卡住）。
//  2. reencryptPaymentProviderConfigs —— 把回归窗口内被明文写入的支付服务商配置
//     重新加密。这一步拿不到纯 SQL 上下文里的应用密钥，因此不能做成 migration。

const (
	// secretEncryptionBackfillLockID 是重加密回填的 PostgreSQL Advisory Lock ID。
	//
	// 刻意不复用 migrationsAdvisoryLockID：复用会把回填和数据库迁移串在同一把锁上，
	// 一个副本做回填时另一个副本连迁移都启动不了。而且「迁移先跑完并已释放锁」这个
	// 让复用不至于自死锁的前提，只是当前 InitEnt 调用顺序的巧合——独立 ID 让互斥
	// 不依赖调用顺序。任何稳定且不与同库其它锁冲突的 int64 都可以。
	secretEncryptionBackfillLockID int64 = 5217834190226610347

	secretEncryptionBackfillLockRetryInterval = 500 * time.Millisecond

	// paymentConfigBackfillMarkerKey 是回填的持久完成标记，存放在 settings 表。
	// 带 _v1 后缀：将来若需要再跑一轮不同语义的回填，用新键而不是删旧键。
	paymentConfigBackfillMarkerKey = "payment_provider_config_encryption_backfill_v1"
)

// secretEncryptionProbe 描述一条「已经有东西可丢」的证据。
type secretEncryptionProbe struct {
	// name 出现在启动错误信息里，让运维知道是哪个功能卡住了启动。
	name string
	// query 必须返回单个 bool。命中即代表该功能已经存过密文或已被启用。
	query string
}

// secretEncryptionProbes 中的每一条都直接对应一个已确认的 SecretEncryptor 调用点。
// 这些查询只在「主密钥为空」这条冷路径上执行，成本可以忽略。
var secretEncryptionProbes = []secretEncryptionProbe{
	// settings 里已存在的加密配置块（backup_service / image_storage_settings /
	// ollama_cloud_usage / securityaudit.prompt_config_store）。
	{
		name:  "backup S3 credentials (settings.backup_s3_config)",
		query: `SELECT EXISTS (SELECT 1 FROM settings WHERE key = 'backup_s3_config')`,
	},
	{
		name:  "S3 image storage credentials (settings.image_storage_config)",
		query: `SELECT EXISTS (SELECT 1 FROM settings WHERE key = 'image_storage_config')`,
	},
	{
		name:  "Ollama Cloud session (settings.ollama_cloud_usage_settings)",
		query: `SELECT EXISTS (SELECT 1 FROM settings WHERE key = 'ollama_cloud_usage_settings')`,
	},
	{
		name:  "prompt audit endpoint token (settings.prompt_audit_config)",
		query: `SELECT EXISTS (SELECT 1 FROM settings WHERE key = 'prompt_audit_config')`,
	},
	// 已启用的开关：功能开着就随时可能写入新密文。
	{
		name:  "channel monitor (settings.channel_monitor_enabled=true)",
		query: `SELECT EXISTS (SELECT 1 FROM settings WHERE key = 'channel_monitor_enabled' AND value = 'true')`,
	},
	{
		name:  "plugin management (settings.plugin_management_enabled=true)",
		query: `SELECT EXISTS (SELECT 1 FROM settings WHERE key = 'plugin_management_enabled' AND value = 'true')`,
	},
	{
		name:  "TOTP two-factor authentication (settings.totp_enabled=true)",
		query: `SELECT EXISTS (SELECT 1 FROM settings WHERE key = 'totp_enabled' AND value = 'true')`,
	},
	// 已经落库的密文行。
	{
		name:  "channel monitor upstream API keys (channel_monitors.api_key)",
		query: `SELECT EXISTS (SELECT 1 FROM channel_monitors WHERE api_key <> '')`,
	},
	{
		name:  "plugin configuration (sub2api_plugin_installations.config_encrypted)",
		query: `SELECT EXISTS (SELECT 1 FROM sub2api_plugin_installations WHERE config_encrypted <> '')`,
	},
	{
		name:  "users with 2FA enabled (users.totp_enabled)",
		query: `SELECT EXISTS (SELECT 1 FROM users WHERE totp_enabled = true)`,
	},
}

// ensureSecretEncryptionKeyUsable 在主密钥缺失时决定是放行还是阻断启动。
//
// 密钥已配置：零查询返回。
//
// 密钥为空：逐条探测数据库。命中任意一条就返回错误——此时已经存在只有这把密钥才能
// 解开的数据，继续启动只会让运维在功能报错时才发现密钥丢了，或者更糟，在
// SecretEncryptor 曾经自动生成随机密钥的旧版本里根本发现不了。
//
// 一条都没命中：输出一次说明性告警后放行。全新安装在跑完 setup 之前必然没有任何
// 密文，此时强制失败等于让每个运维为一个尚不存在的风险付出确定的上手成本。这个宽松
// 分支只在它确实无害的那段时间成立：数据库里一出现第一份密文或第一个启用的相关功能，
// 上面的探测就会立刻开始拦截。
func ensureSecretEncryptionKeyUsable(ctx context.Context, db *sql.DB, cfg *config.Config) error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if strings.TrimSpace(cfg.Security.SecretEncryptionKey) != "" {
		return nil
	}
	if db == nil {
		return errors.New("nil sql db")
	}

	triggered := make([]string, 0, len(secretEncryptionProbes))
	for _, probe := range secretEncryptionProbes {
		hit, err := secretEncryptionProbeHit(ctx, db, probe.query)
		if err != nil {
			// 探测失败（表还不存在、被重命名、权限不足）一律按未命中处理。
			// 这一步的职责是防止静默数据损坏，不是给自己发明新的启动失败模式。
			slog.Debug("secret encryption preflight probe skipped",
				"probe", probe.name, "error", err)
			continue
		}
		if hit {
			triggered = append(triggered, probe.name)
		}
	}

	if len(triggered) == 0 {
		slog.Warn("no application secret encryption key is configured; "+
			"features that store secrets (backup S3, channel monitor, image storage, plugins, "+
			"Ollama Cloud, prompt audit endpoints, TOTP) will refuse to save until one is set",
			"set_env", config.SecretEncryptionKeyEnvVar,
			"generate_with", "openssl rand -hex 32")
		return nil
	}

	return fmt.Errorf(
		"refusing to start: no application secret encryption key is configured, but this database "+
			"already depends on one — %s.\n"+
			"Starting without the key would leave that data permanently undecryptable.\n"+
			"Set %s to the 64-hex-character key this installation was using (legacy alias: %s), then restart.\n"+
			"For a brand new key: openssl rand -hex 32",
		strings.Join(triggered, "; "),
		config.SecretEncryptionKeyEnvVar,
		config.LegacySecretEncryptionKeyEnvVar,
	)
}

func secretEncryptionProbeHit(ctx context.Context, db *sql.DB, query string) (bool, error) {
	var hit bool
	if err := db.QueryRowContext(ctx, query).Scan(&hit); err != nil {
		return false, err
	}
	return hit, nil
}

// isPlaintextPaymentProviderConfig 判定一个存储值是否为回归窗口写入的明文。
//
// 这是判定而不是猜测：两种格式在结构上互斥。
//   - 明文是 json.Marshal(map[string]string) 的输出，永远以 '{' 开头，且一定能
//     unmarshal 回 map[string]string。
//   - 密文是 fmt.Sprintf("%s:%s:%s", b64, b64, b64)，绝不可能是合法的 JSON 对象。
//
// 空串和 "null" 返回 false：没有任何秘密可保护，重加密它们只会给回滚增加噪音。
//
// 误判方向是安全的：一行被误判为密文只会被留在原地等下一轮，不会被错误地二次加密。
func isPlaintextPaymentProviderConfig(stored string) (map[string]string, bool) {
	if stored == "" {
		return nil, false
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(stored), &cfg); err != nil {
		return nil, false
	}
	if cfg == nil {
		return nil, false
	}
	return cfg, true
}

// reencryptPaymentProviderConfigs 把 payment_provider_instances.config 中回归窗口内
// 被明文写入的行重新加密。
//
// 为什么不是 SQL migration：migrations/*.sql 由迁移运行器在纯 SQL 上下文中执行，
// 拿不到应用层的加密密钥。
//
// 为什么在启动路径上而不是后台 worker：回填必须在任何请求读到这些行之前完成，才能
// 保证「读到的一定是密文或已知的兼容明文」这个不变式。
//
// 幂等性由三层叠加保证，详见 openspec/changes/harden-app-secret-encryption/design.md：
//  1. settings 表里的持久完成标记；
//  2. 明文判别本身幂等（已加密的行不再匹配）；
//  3. 每行写回带 `AND config = <读到的原值>` 条件守卫。
func reencryptPaymentProviderConfigs(ctx context.Context, db *sql.DB, cfg *config.Config) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	if cfg == nil {
		return errors.New("nil config")
	}

	keyHex := strings.TrimSpace(cfg.Security.SecretEncryptionKey)
	if keyHex == "" {
		// 不写标记：配置好密钥后的下一次启动才真正执行回填。
		// 也不返回错误：「密钥缺失是否阻断启动」是 ensureSecretEncryptionKeyUsable
		// 的职责，这里不为同一件事发明第二种失败方式。
		slog.Warn("skipping payment provider config re-encryption: no secret encryption key configured",
			"set_env", config.SecretEncryptionKeyEnvVar)
		return nil
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != payment.AES256KeySize {
		// 走到这里说明密钥格式非法。NewAESEncryptor 已经会因此让启动失败，
		// 这里保持沉默跳过，避免同一个配置错误产生两条互相矛盾的报错。
		return nil
	}

	// 先查标记再取锁：绝大多数启动会在这里以一次查询结束。
	done, err := paymentConfigBackfillCompleted(ctx, db)
	if err != nil {
		return fmt.Errorf("check payment config backfill marker: %w", err)
	}
	if done {
		return nil
	}

	lockConn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire payment config backfill lock connection: %w", err)
	}
	defer func() { _ = lockConn.Close() }()

	if err := secretEncryptionBackfillLock(ctx, lockConn); err != nil {
		return err
	}
	defer func() {
		// 独立超时：原 ctx 取消后仍要尝试释放锁，但数据库链路异常不得无限阻塞进程退出。
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := lockConn.ExecContext(unlockCtx,
			"SELECT pg_advisory_unlock($1)", secretEncryptionBackfillLockID); err != nil {
			slog.Warn("failed to release payment config backfill lock", "error", err)
		}
	}()

	// 双重检查：在锁上等待期间，另一个副本可能已经跑完并写下了标记。
	done, err = paymentConfigBackfillCompleted(ctx, db)
	if err != nil {
		return fmt.Errorf("recheck payment config backfill marker: %w", err)
	}
	if done {
		return nil
	}

	rewritten, skipped, err := rewritePlaintextPaymentProviderConfigs(ctx, db, key)
	if err != nil {
		// 标记不写。已重加密的行和仍是明文的行都能被 decryptConfig 正常读取，
		// 下一次启动会接着处理剩下的行。宁可重跑一次廉价的扫描，
		// 也不要让半成品状态被标记成已完成。
		return fmt.Errorf("re-encrypt payment provider configs: %w", err)
	}

	if err := markPaymentConfigBackfillDone(ctx, db, rewritten); err != nil {
		return fmt.Errorf("record payment config backfill marker: %w", err)
	}

	if rewritten > 0 {
		slog.Warn("re-encrypted payment provider configs that were stored in plaintext",
			"rewritten", rewritten, "skipped", skipped,
			"note", "rotate the affected gateway credentials if this database was ever exposed")
	} else {
		slog.Info("payment provider config encryption backfill: nothing to do", "checked", skipped)
	}
	return nil
}

// rewritePlaintextPaymentProviderConfigs 逐行处理，返回 (重写行数, 跳过行数)。
//
// 刻意不用包住全部行的大事务：这一步跑在启动路径上并持有 advisory lock，
// 长事务会把其它副本的启动一起拖住。
func rewritePlaintextPaymentProviderConfigs(ctx context.Context, db *sql.DB, key []byte) (int, int, error) {
	type row struct {
		id     int64
		config string
	}

	rows, err := db.QueryContext(ctx, `SELECT id, config FROM payment_provider_instances ORDER BY id`)
	if err != nil {
		return 0, 0, fmt.Errorf("list payment provider instances: %w", err)
	}
	// 先把结果读完再逐行更新：在同一条连接上边遍历游标边执行 UPDATE
	// 会和 database/sql 的连接复用打架。实例数量是管理员手工配置的，规模很小。
	pending := make([]row, 0, 16)
	skipped := 0
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.config); err != nil {
			_ = rows.Close()
			return 0, skipped, fmt.Errorf("scan payment provider instance: %w", err)
		}
		if _, ok := isPlaintextPaymentProviderConfig(r.config); !ok {
			skipped++
			continue
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, skipped, fmt.Errorf("iterate payment provider instances: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, skipped, fmt.Errorf("close payment provider instances cursor: %w", err)
	}

	rewritten := 0
	for _, r := range pending {
		ciphertext, err := payment.Encrypt(r.config, key)
		if err != nil {
			return rewritten, skipped, fmt.Errorf("encrypt instance %d: %w", r.id, err)
		}

		// 条件守卫：如果管理员在读取和写回之间通过后台改了这一行，影响行数为 0，
		// 放弃该行。此时它的新值已经是密文（encryptConfig 已修好），放弃是正确的。
		res, err := db.ExecContext(ctx,
			`UPDATE payment_provider_instances SET config = $1 WHERE id = $2 AND config = $3`,
			ciphertext, r.id, r.config)
		if err != nil {
			return rewritten, skipped, fmt.Errorf("update instance %d: %w", r.id, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return rewritten, skipped, fmt.Errorf("update instance %d: %w", r.id, err)
		}
		if affected == 0 {
			slog.Info("payment provider config changed during backfill, leaving it alone", "instance_id", r.id)
			skipped++
			continue
		}
		rewritten++
	}
	return rewritten, skipped, nil
}

func paymentConfigBackfillCompleted(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM settings WHERE key = $1)`,
		paymentConfigBackfillMarkerKey).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func markPaymentConfigBackfillDone(ctx context.Context, db *sql.DB, rewritten int) error {
	value := fmt.Sprintf(`{"completed_at":%q,"rewritten":%d}`,
		time.Now().UTC().Format(time.RFC3339), rewritten)
	// ON CONFLICT DO NOTHING：另一个副本抢先写入标记不是错误。
	_, err := db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO NOTHING
	`, paymentConfigBackfillMarkerKey, value)
	return err
}

type backfillLockConnection interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// secretEncryptionBackfillLock 轮询获取 advisory lock，模式与 migrations_runner.go 一致，
// 但用的是独立的锁 ID（见 secretEncryptionBackfillLockID 的注释）。
func secretEncryptionBackfillLock(ctx context.Context, conn backfillLockConnection) error {
	ticker := time.NewTicker(secretEncryptionBackfillLockRetryInterval)
	defer ticker.Stop()

	for {
		var locked bool
		if err := conn.QueryRowContext(ctx,
			"SELECT pg_try_advisory_lock($1)", secretEncryptionBackfillLockID).Scan(&locked); err != nil {
			return fmt.Errorf("acquire payment config backfill lock: %w", err)
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire payment config backfill lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
