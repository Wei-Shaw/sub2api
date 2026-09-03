package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
)

// schemaMigrationsTableDDL 定义迁移记录表的 DDL。
// 该表用于跟踪已应用的迁移文件及其校验和。
// - filename: 迁移文件名，作为主键唯一标识每个迁移
// - checksum: 文件内容的 SHA256 哈希值，用于检测迁移文件是否被篡改
// - applied_at: 迁移应用时间戳
const schemaMigrationsTableDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	filename   TEXT PRIMARY KEY,
	checksum   TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

const atlasSchemaRevisionsTableDDL = `
CREATE TABLE IF NOT EXISTS atlas_schema_revisions (
	version TEXT PRIMARY KEY,
	description TEXT NOT NULL,
	type INTEGER NOT NULL,
	applied INTEGER NOT NULL DEFAULT 0,
	total INTEGER NOT NULL DEFAULT 0,
	executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	execution_time BIGINT NOT NULL DEFAULT 0,
	error TEXT NULL,
	error_stmt TEXT NULL,
	hash TEXT NOT NULL DEFAULT '',
	partial_hashes TEXT[] NULL,
	operator_version TEXT NULL
);
`

// migrationsAdvisoryLockID 是用于序列化迁移操作的 PostgreSQL Advisory Lock ID。
// 在多实例部署场景下，该锁确保同一时间只有一个实例执行迁移。
// 任何稳定的 int64 值都可以，只要不与同一数据库中的其他锁冲突即可。
const migrationsAdvisoryLockID int64 = 694208311321144027
const migrationsLockRetryInterval = 500 * time.Millisecond
const nonTransactionalMigrationSuffix = "_notx.sql"

// paymentOrdersOutTradeNoUniqueMigration 需要一次迁移专属的数据前置校验，因此仍按文件名识别。
// 其余 *_notx.sql 不再按文件名做特殊处理：INVALID 索引清理已改为从文件内容解析索引名。
const paymentOrdersOutTradeNoUniqueMigration = "120_enforce_payment_orders_out_trade_no_unique_notx.sql"

// 以下常量只被 tag 门控的测试（unit / integration）引用，golangci-lint 默认不带 build tag，
// 看不到那些文件，因此会误报 unused。
//
//nolint:unused // referenced from //go:build unit / integration test files
const (
	latestAPIKeyIPIndexMigration                 = "174_add_usage_logs_api_key_latest_ip_index_notx.sql"
	latestAPIKeyIPIndex                          = "idx_usage_logs_api_key_latest_ip"
	usageLogsUpstreamModelMismatchIndexMigration = "195_add_usage_log_upstream_model_mismatch_index_notx.sql"
	usageLogsUpstreamModelMismatchIndex          = "idx_usage_logs_upstream_model_mismatch_created_at"
	usageLogsEffectiveModelIndexesMigration      = "226_add_usage_log_effective_model_indexes_notx.sql"
	usageLogsEffectiveRequestedModelIndex        = "idx_usage_logs_effective_requested_model_created"
	usageLogsEffectiveUpstreamModelIndex         = "idx_usage_logs_effective_upstream_model_created"
)

type migrationChecksumCompatibilityRule struct {
	fileChecksum       string
	acceptedDBChecksum map[string]struct{}
	acceptedChecksums  map[string]struct{}
}

// migrationChecksumCompatibilityRules 仅用于兼容历史上误修改过的迁移文件 checksum。
// 规则必须同时匹配「迁移名 + 数据库 checksum + 当前文件 checksum」且两者都落在该迁移的已知版本集合内才会放行，
// 避免放宽全局校验，也允许将误改的历史 migration 回滚为已发布版本而不要求人工修 checksum。
// 维护提醒：checksum 必须按运行器的算法计算，即 sha256(strings.TrimSpace(文件内容))，
// 而不是对原始文件字节做 sha256（`sha256sum <file>` 的输出会把结尾换行也算进去，二者永远不同）。
// 历史上 109/110/112/118/123/195/218/219/220 就是用 `sha256sum` 填的，规则因此完全失效；
// 115/116/120 则漏掉了中间版本。fileChecksum 必须等于当前文件的 checksum，
// acceptedDBChecksums 需要覆盖该文件出现过的所有历史版本，且只增不减。
// TestMigrationChecksumCompatibilityRulesMatchEmbeddedFiles 会守护这两条不变式。
var migrationChecksumCompatibilityRules = map[string]migrationChecksumCompatibilityRule{
	// 019/024/037 历史上误包含 `-- +goose Down` 块；迁移运行器不解析 goose 指令，
	// 该块会紧接着 Up 块在同一事务里执行，导致迁移自我回滚。
	// 现已删除 goose 标记（仅注释，不改变 SQL 语义），已安装的库保留历史 checksum，
	// 修复逻辑由 235/236/237 幂等迁移补齐。
	"019_migrate_wechat_to_attributes.sql": newMigrationChecksumCompatibilityRule("ee3ef7786d8d1a70fea3c96eada2199ab5dd1977f34bced36327c2ff38a4dff3", "d45e05b4bb722b287377790583c2677b8666dbf7e02b626c93468491d4ce8cf8"),
	"024_add_gemini_tier_id.sql":           newMigrationChecksumCompatibilityRule("63f0ecd8b51a66d63221b93b351fc93dc5a2d7e045886a26a23d4259881335bd", "b54de1b9a4423224f7aef5e644d1af115214d58dd61befd3c25db3e709b9163a"),
	// 027 的两条 request_id 归一化语句（全表 UPDATE + 对全部非空 request_id 做 ROW_NUMBER 窗口）
	// 现在被包在「唯一索引已存在则跳过」的 DO 块里。在已经跑过 027 的库上重放时，
	// 它们会在启动事务里全表扫描 + 排序落盘却更新 0 行，超时即前功尽弃；SQL 语义未变。
	"027_usage_billing_consistency.sql": newMigrationChecksumCompatibilityRule("900bd6d56f7cbec279f4aa04a7717c65b369d71efecdcd47229a05c3395b8722", "68df49831cadfb2c5d1f8e24b8b36068be454cca9fdf6df1141593ee20c98dd8"),
	// 036/041/044b/069/189 历史上使用裸 `ALTER TABLE ... ADD COLUMN`，在「列已存在但
	// schema_migrations 缺少对应行」的库（不含跟踪表的恢复、schema-only 克隆、手工补过 DDL）上
	// 会以 `column already exists` 直接中断启动，且只能人工修复。现已补上 IF NOT EXISTS
	// （纯幂等性修复，不改变列定义与默认值），已安装的库保留历史 checksum 继续放行。
	"036_ops_error_logs_add_is_count_tokens.sql":              newMigrationChecksumCompatibilityRule("c86571fad72cec209a3f9c1267f9e306585281ef42bfb6760fc594d35719215c", "ed9ee240c43ef259f18a7ca440d73f285988d2aa57af229e0b69344f11af3bf4"),
	"041_add_model_routing_enabled.sql":                       newMigrationChecksumCompatibilityRule("ed47b7db6af8d8b968e3554b9017bc792e4359775fc33c6f0095f3fc8db98caf", "5cee91bdfc5afe4815dba6b127755a76d33ab55a93419c5b51e57d8dfd9cba3a"),
	"044b_add_group_mcp_xml_inject.sql":                       newMigrationChecksumCompatibilityRule("ee6818ad95110ad842df5ac57b7573069f373e20eb4ef74ce5a472d3d9a274fc", "944e8ed8950dc9c2cf95ee62bcf342ea2e15a771895b43da87efec85db921c31"),
	"069_add_group_messages_dispatch.sql":                     newMigrationChecksumCompatibilityRule("594e258aadcfea83fd28258b8a35a1d4d2ae34b2b12944018bda9ee913c18aca", "c6d35422f140f97f5427f756eae929b243e3cbb362252806f179fbb7519b107f"),
	"189_add_group_allow_live.sql":                            newMigrationChecksumCompatibilityRule("d6d2e6ac7f201da0cebcc81bdc7b8a5ffff7f93abfb149f17d3dd609fa316ea6", "51172b10c160e7f560346dbaf736dc8e92feb793cd00169f5fb876c399460862"),
	"037_ops_alert_silences.sql":                              newMigrationChecksumCompatibilityRule("5bf2aba80d8501c9177494480a78668a236ee85249c84455ba908836681fe759", "72143a1ce3528ebc47472759c59011ec6993b25a3f22d50485538710047438c6"),
	"054_drop_legacy_cache_columns.sql":                       newMigrationChecksumCompatibilityRule("82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d", "182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4"),
	"061_add_usage_log_request_type.sql":                      newMigrationChecksumCompatibilityRule("66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c", "08a248652cbab7cfde147fc6ef8cda464f2477674e20b718312faa252e0481c0", "222b4a09c797c22e5922b6b172327c824f5463aaa8760e4f621bc5c22e2be0f3"),
	"109_auth_identity_compat_backfill.sql":                   newMigrationChecksumCompatibilityRule("2b380305e73ff0c13aa8c811e45897f2b36ca4a438f7b3e8f98e19ecb6bae0b3", "748ddcdc60f93a1ac562ce8a66ee870f64ee594bf6dbedad55ed8baf3c75b28c", "0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace", "551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee"),
	"110_pending_auth_and_provider_default_grants.sql":        newMigrationChecksumCompatibilityRule("57a196a9810fb478fa001dfff110f5c76a7d87fb04f15e12e513fcb75402d7a6", "301e90405b3424967b7d1931568b7a244902148fa82802f362c115ae4e2ae2ef", "32cf87ee787b1bb36b5c691367c96eee37518fa3eed6f3322cf68795e3745279", "e3d1f433be2b564cfbdc549adf98fce13c5c7b363ebc20fd05b765d0563b0925"),
	"112_add_payment_order_provider_key_snapshot.sql":         newMigrationChecksumCompatibilityRule("ab871fc02da1eabe0de6ca74a119ee3cea9c727caed30af2ae07a0cd1176d1b8", "d4476c67ceea871aa2d92ee2a603795a742d0379a58cf53938bb9aa559ff9caa", "b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99", "ffd3e8a2c9295fa9cbefefd629a78268877e5b51bc970a82d9b3f46ec4ebd15e"),
	"115_auth_identity_legacy_external_backfill.sql":          newMigrationChecksumCompatibilityRule("022aadd97bb53e755f0cf7a3a957e0cb1a1353b0c39ec4de3234acd2871fd04f", "4cf39e508be9fd1a5aa41610cbbebeb80385c9adda45bf78a706de9db4f1385f", "72f32dec60e352e652006b0a09ed8720b4c88e4afc177ecde22266a9803d7203"),
	"116_auth_identity_legacy_external_safety_reports.sql":    newMigrationChecksumCompatibilityRule("07edb09fa8d04ffb172b0621e3c22f4d1757d20a24ae267b3b36b087ab72d488", "f7757bd929ac67ffb08ce69fa4cf20fad39dbff9d5a5085fb2adabb7607e5877", "a4db306b0b987459590522ebb08ff9ce42ab1ff5d4f99ec4068c41a51f2236da"),
	"118_wechat_dual_mode_and_auth_source_defaults.sql":       newMigrationChecksumCompatibilityRule("ed272e0840730b6b8e7838513c4cc8817e8b5e488e27c88b5421adbece5e89c9", "b4a5b7a28f6a7ac67aad214645761e5a8486c83f0f2a1a874d7f67085f83159b", "6395ad255f2be2219ad85813b72db6fa7783c81d747e42e098847ef3594f1674", "b54194d7a3e4fbf710e0a3590d22a2fe7966804c487052a356e0b55f53ef96b0", "e0cdf835d6c688d64100f483d31bc02ac9ebad414bf1837af239a84bf75b8227", "a38243ca0a72c3a01c0a92b7986423054d6133c0399441f853b99802852720fb"),
	"119_enforce_payment_orders_out_trade_no_unique.sql":      newMigrationChecksumCompatibilityRule("0bbe809ae48a9d811dabda1ba1c74955bd71c4a9cc610f9128816818dfa6c11e", "ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34"),
	"120_enforce_payment_orders_out_trade_no_unique_notx.sql": newMigrationChecksumCompatibilityRule("34aadc0db59a4e390f92a12b73bd74642d9724f33124f73638ae00089ea5e074", "e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61", "79ea6127a22e61b3bad6ea29347a8cc3ff005f8b486ef4a51bd04fdda906f931", "707431450603e70a43ce9fbd61e0c12fa67da4875158ccefabacea069587ab22", "04b082b5a239c525154fe9185d324ee2b05ff90da9297e10dba19f9be79aa59a"),
	"123_fix_legacy_auth_source_grant_on_signup_defaults.sql": newMigrationChecksumCompatibilityRule("7faba5ef65051b7ecb215b7fd2351b0828b7c48153ec688ac089c1588d2cde41", "ac0d79ca6feb449674f54f593a5eac5f7cc06751047c664b586c1892e19c60d5", "ea17c2767b937f08274e091d212a93acb7e2d62521129179830f073a291fbd97", "2ce43c2cd89e9f9e1febd34a407ed9e84d177386c5544b6f02c1f58a21129f57", "6cd33422f215dcd1f486ab6f35c0ea5805d9ca69bb25906d94bc649156657145"),
	"159_batch_image_foundation.sql":                          newMigrationChecksumCompatibilityRule("d902b70982025ec519749faf058aab7631e82c3f48167b9a4ae4db718eb72cce", "82da85b5d98e67a0507647b873a40373e84538e4adafdeed6767c0ac8b6570b2"),
	"161_batch_image_pricing_snapshot.sql":                    newMigrationChecksumCompatibilityRule("4012af3e43636cb6af22e0176d59d1fcc70615c0f310194329461ae462c4fbd6", "96d915c9b7a6941ae99039e0ff3f1a61481eb9bddd933d11c6fadb2274554e87"),
	// 195 originally seeded mode=v2; flipped to v1 (safe default / opt-in v2). Existing DBs
	// that already applied the v2 seed keep their row and the historical checksum.
	"195_channel_monitor_mode.sql": newMigrationChecksumCompatibilityRule("73c39ac374c722253135041466108836845828a6065b499c60e7f27d6b92c21c", "f20366e106e3a54c73d4a67df3ba87734427ed859bc4ae42b0708e4cbcbacb56", "13f3792f3e3e53ee96e26415c884cf8062c77172824b54fcc9a8c0c2b1f185ec", "4c74fe33ef2274cc72e1bb49671e651274532c034b29f5b2982c2a4c88d101a6"),
	// 220 originally cleared video prices for all non-grok platforms (including composite);
	// composite is now preserved because it may route to Grok accounts.
	"220_clear_non_grok_video_generation_config.sql": newMigrationChecksumCompatibilityRule("cf4dbfa75ac27d93a30a6a14439fe7dccfc911c043358363d5ec47946aa0e28b", "353c8e8e1805f2a6fd61311e03118e7dd8388f264cfd9af9e0cabe2a696388c4", "3d08d905a7bca1f56f14b6d2a2a0dcb07480ff52c21393b4e2db1b3a3f83b3d0", "85e320b9ec64f2d3fcd8cf705b2b4e76a7b49f7a57140c14bff97f32691c818b", "3da48c8fdffe6390325f43d08b8e353e0a365df43d44a78dbbe655d0deb18402"),
	"219_group_search_price_per_1k.sql":              newMigrationChecksumCompatibilityRule("430c2e3595342fe22c59e9676e9b18ea376f076324b77174a21e6f181f57f4b5", "833578274d0eed24d39355298d5659b33e5484c869b331ffd815187c221552d2", "e86786ebcc3b14206fd2d321380a4e50e80cdadbfcf4962c639255e6a14008db", "df6ffd71b97e30ec2c8fe7b95e15783042dea58c553e32701ee7c42a5619af80"),
	"218_group_audio_voice_pricing.sql":              newMigrationChecksumCompatibilityRule("a99ade7d0d464c67bf56814570050cc363ffad64eae2cb1e1ed760065f0b3585", "343a955e52348ce92c35753e78ca3f8e5a76060c20af71061ca5e04c6ed84085", "40ee9f3a2af0e0a5e99dabc878fd0fe98be1011f26bcfcefcac7197f7081f0e7", "c2a5e5b4ffd6968ad1c10593289fbc11192cdea19fec3ed9bce3a84eff9a8351"),
}

// ApplyMigrations 将嵌入的 SQL 迁移文件应用到指定的数据库。
//
// 该函数可以在每次应用启动时安全调用：
// - 已应用的迁移会被自动跳过（通过校验 filename 判断）
// - 如果迁移文件内容被修改（checksum 不匹配），会返回错误
// - 使用 PostgreSQL Advisory Lock 确保多实例并发安全
//
// 参数：
//   - ctx: 上下文，用于超时控制和取消
//   - db: 数据库连接
//
// 返回：
//   - error: 迁移过程中的任何错误
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	return applyMigrationsFS(ctx, db, migrations.FS)
}

// applyMigrationsFS 是迁移执行的核心实现。
// 它从指定的文件系统读取 SQL 迁移文件并按顺序应用。
//
// 迁移执行流程：
//  1. 获取 PostgreSQL Advisory Lock，防止多实例并发迁移
//  2. 确保 schema_migrations 表存在
//  3. 按文件名排序读取所有 .sql 文件
//  4. 对于每个迁移文件：
//     - 计算文件内容的 SHA256 校验和
//     - 检查该迁移是否已应用（通过 filename 查询）
//     - 如果已应用，验证校验和是否匹配
//     - 如果未应用，在事务中执行迁移并记录
//  5. 释放 Advisory Lock
//
// 参数：
//   - ctx: 上下文
//   - db: 数据库连接
//   - fsys: 包含迁移文件的文件系统（通常是 embed.FS）
func applyMigrationsFS(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	if db == nil {
		return errors.New("nil sql db")
	}

	// 获取分布式锁，确保多实例部署时只有一个实例执行迁移。
	// 这是 PostgreSQL 特有的 Advisory Lock 机制。
	lockConn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migrations lock connection: %w", err)
	}
	defer func() { _ = lockConn.Close() }()
	if err := pgAdvisoryLock(ctx, lockConn); err != nil {
		return err
	}
	defer func() {
		// 无论迁移是否成功，都要释放锁。
		// 独立超时确保原 ctx 取消后仍会尝试释放，但数据库链路异常不会
		// 无限阻塞进程退出。
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pgAdvisoryUnlock(unlockCtx, lockConn)
	}()

	// 创建迁移记录表（如果不存在）。
	// 该表记录所有已应用的迁移及其校验和。
	if _, err := lockConn.ExecContext(ctx, schemaMigrationsTableDDL); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// 自动对齐 Atlas 基线（如果检测到 legacy schema_migrations 且缺失 atlas_schema_revisions）。
	if err := ensureAtlasBaselineAligned(ctx, lockConn, fsys); err != nil {
		return err
	}

	// 获取所有 .sql 迁移文件并按文件名排序。
	// 命名规范：使用零填充数字前缀（如 001_init.sql, 002_add_users.sql）。
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files) // 确保按文件名顺序执行迁移

	for _, name := range files {
		// 读取迁移文件内容
		contentBytes, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		content := strings.TrimSpace(string(contentBytes))
		if content == "" {
			continue // 跳过空文件
		}

		// 计算文件内容的 SHA256 校验和，用于检测文件是否被修改。
		// 这是一种防篡改机制：如果有人修改了已应用的迁移文件，系统会拒绝启动。
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])

		// 检查该迁移是否已经应用
		var existing string
		rowErr := lockConn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename = $1", name).Scan(&existing)
		if rowErr == nil {
			// 迁移已应用，验证校验和是否匹配
			if existing != checksum {
				// 兼容特定历史误改场景（仅白名单规则），其余仍保持严格不可变约束。
				if isMigrationChecksumCompatible(name, existing, checksum) {
					continue
				}
				// 校验和不匹配意味着迁移文件在应用后被修改，这是危险的。
				// 正确的做法是创建新的迁移文件来进行变更。
				return fmt.Errorf(
					"migration %s checksum mismatch (db=%s file=%s)\n"+
						"This means the migration file was modified after being applied to the database.\n"+
						"Solutions:\n"+
						"  1. Revert to original: git log --oneline -- migrations/%s && git checkout <commit> -- migrations/%s\n"+
						"  2. For new changes, create a new migration file instead of modifying existing ones\n"+
						"Note: Modifying applied migrations breaks the immutability principle and can cause inconsistencies across environments",
					name, existing, checksum, name, name,
				)
			}
			continue // 迁移已应用且校验和匹配，跳过
		}
		if !errors.Is(rowErr, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", name, rowErr)
		}

		nonTx, err := validateMigrationExecutionMode(name, content)
		if err != nil {
			return fmt.Errorf("validate migration %s: %w", name, err)
		}

		if nonTx {
			if err := prepareNonTransactionalMigration(ctx, lockConn, name, content); err != nil {
				return fmt.Errorf("prepare migration %s: %w", name, err)
			}

			// *_notx.sql：用于 CREATE/DROP INDEX CONCURRENTLY 场景，必须非事务执行。
			// 逐条语句执行，避免将多条 CONCURRENTLY 语句放入同一个隐式事务块。
			statements := splitSQLStatements(content)
			for i, stmt := range statements {
				trimmed := strings.TrimSpace(stmt)
				if trimmed == "" {
					continue
				}
				if stripSQLLineComment(trimmed) == "" {
					continue
				}
				if _, err := lockConn.ExecContext(ctx, trimmed); err != nil {
					return fmt.Errorf("apply migration %s (non-tx statement %d): %w", name, i+1, err)
				}
			}
			if _, err := lockConn.ExecContext(ctx, "INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
				return fmt.Errorf("record migration %s (non-tx): %w", name, err)
			}
			continue
		}

		// 默认迁移在事务中执行，确保原子性：要么完全成功，要么完全回滚。
		tx, err := lockConn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}

		// 执行迁移 SQL
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		// 记录迁移已完成，保存文件名和校验和
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		// 提交事务
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
}

type migrationConnection interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// prepareNonTransactionalMigration 在执行 *_notx.sql 之前做两件事：
//  1. 迁移专属的数据前置校验（目前只有 120 的 out_trade_no 重复检查）；
//  2. 清理上一次被中断的 CREATE INDEX CONCURRENTLY 留下的 INVALID 索引。
//
// 第 2 步的索引名直接从迁移文件内容里解析，因此覆盖全部 *_notx.sql。
// 历史实现只对 5 个文件名做 switch 白名单清理，而 062/072/148/151/155/190 等文件没有覆盖：
// CREATE INDEX CONCURRENTLY 被打断后会留下一个永久 INVALID 的索引，下次启动时语句里的
// IF NOT EXISTS 认为索引已存在而直接跳过，迁移被记为已应用，坏索引却永远留在库里
// （既不被使用，又要承担全部写放大）。白名单正是这个 bug 的成因，所以不再扩充白名单。
func prepareNonTransactionalMigration(ctx context.Context, db migrationConnection, name, content string) error {
	if name == paymentOrdersOutTradeNoUniqueMigration {
		if err := checkDuplicatePaymentOrderOutTradeNos(ctx, db); err != nil {
			return err
		}
	}

	for _, indexName := range concurrentlyCreatedIndexNames(content) {
		if err := dropInvalidIndexIfPresent(ctx, db, indexName); err != nil {
			return err
		}
	}
	return nil
}

// concurrentlyCreatedIndexNames 解析出内容里所有 `CREATE [UNIQUE] INDEX CONCURRENTLY` 创建的索引名。
// validateMigrationExecutionMode 已经保证 *_notx.sql 里的 CONCURRENTLY 语句必须是
// CREATE/DROP INDEX 且带 IF [NOT] EXISTS，并且 CREATE 语句必须能被这里解析出索引名，
// 因此该函数对合法的 *_notx.sql 是完备的（不会静默漏掉某个索引）。
func concurrentlyCreatedIndexNames(content string) []string {
	names := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, stmt := range splitSQLStatements(content) {
		indexName, ok := parseConcurrentlyCreatedIndexName(stripSQLLineComment(strings.TrimSpace(stmt)))
		if !ok {
			continue
		}
		if _, dup := seen[indexName]; dup {
			continue
		}
		seen[indexName] = struct{}{}
		names = append(names, indexName)
	}
	return names
}

// parseConcurrentlyCreatedIndexName 匹配 `CREATE [UNIQUE] INDEX CONCURRENTLY [IF NOT EXISTS] <name> ...`
// 并返回 <name>。只接受未加引号的普通标识符：解析结果会被拼进 DROP INDEX 语句，
// 无法参数化，所以形态不认识时宁可返回 false（退化为不清理），也不拼接任意文本。
func parseConcurrentlyCreatedIndexName(stmt string) (string, bool) {
	fields := strings.Fields(stmt)
	i := 0
	if i >= len(fields) || !strings.EqualFold(fields[i], "CREATE") {
		return "", false
	}
	i++
	if i < len(fields) && strings.EqualFold(fields[i], "UNIQUE") {
		i++
	}
	if i >= len(fields) || !strings.EqualFold(fields[i], "INDEX") {
		return "", false
	}
	i++
	if i >= len(fields) || !strings.EqualFold(fields[i], "CONCURRENTLY") {
		return "", false
	}
	i++
	if i+2 < len(fields) &&
		strings.EqualFold(fields[i], "IF") &&
		strings.EqualFold(fields[i+1], "NOT") &&
		strings.EqualFold(fields[i+2], "EXISTS") {
		i += 3
	}
	if i >= len(fields) {
		return "", false
	}
	name := fields[i]
	if !isPlainSQLIdentifier(name) {
		return "", false
	}
	return name, true
}

func isPlainSQLIdentifier(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && (r >= '0' && r <= '9' || r == '$'):
		default:
			return false
		}
	}
	return true
}

func checkDuplicatePaymentOrderOutTradeNos(ctx context.Context, db migrationConnection) error {
	duplicates, err := findDuplicatePaymentOrderOutTradeNos(ctx, db)
	if err != nil {
		return fmt.Errorf("precheck duplicate out_trade_no: %w", err)
	}
	if len(duplicates) > 0 {
		return fmt.Errorf(
			"duplicate out_trade_no values block %s; remediate duplicates before retrying: %s",
			paymentOrdersOutTradeNoUniqueMigration,
			strings.Join(duplicates, ", "),
		)
	}
	return nil
}

func dropInvalidIndexIfPresent(ctx context.Context, db migrationConnection, indexName string) error {
	invalid, err := indexIsInvalid(ctx, db, indexName)
	if err != nil {
		return fmt.Errorf("check invalid index %s: %w", indexName, err)
	}
	if !invalid {
		return nil
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s", indexName)); err != nil {
		return fmt.Errorf("drop invalid index %s: %w", indexName, err)
	}
	return nil
}

func findDuplicatePaymentOrderOutTradeNos(ctx context.Context, db migrationConnection) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT out_trade_no, COUNT(*) AS duplicate_count
		FROM payment_orders
		WHERE out_trade_no <> ''
		GROUP BY out_trade_no
		HAVING COUNT(*) > 1
		ORDER BY duplicate_count DESC, out_trade_no
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	duplicates := make([]string, 0, 5)
	for rows.Next() {
		var outTradeNo string
		var duplicateCount int
		if err := rows.Scan(&outTradeNo, &duplicateCount); err != nil {
			return nil, err
		}
		duplicates = append(duplicates, fmt.Sprintf("%s (count=%d)", outTradeNo, duplicateCount))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return duplicates, nil
}

func indexIsInvalid(ctx context.Context, db migrationConnection, indexName string) (bool, error) {
	var invalid bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_class idx
			JOIN pg_namespace ns ON ns.oid = idx.relnamespace
			JOIN pg_index i ON i.indexrelid = idx.oid
			WHERE ns.nspname = 'public'
			  AND idx.relname = $1
			  AND NOT i.indisvalid
		)
	`, indexName).Scan(&invalid)
	return invalid, err
}

func ensureAtlasBaselineAligned(ctx context.Context, db migrationConnection, fsys fs.FS) error {
	hasLegacy, err := tableExists(ctx, db, "schema_migrations")
	if err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	if !hasLegacy {
		return nil
	}

	hasAtlas, err := tableExists(ctx, db, "atlas_schema_revisions")
	if err != nil {
		return fmt.Errorf("check atlas_schema_revisions: %w", err)
	}
	if !hasAtlas {
		if _, err := db.ExecContext(ctx, atlasSchemaRevisionsTableDDL); err != nil {
			return fmt.Errorf("create atlas_schema_revisions: %w", err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM atlas_schema_revisions").Scan(&count); err != nil {
		return fmt.Errorf("count atlas_schema_revisions: %w", err)
	}
	if count > 0 {
		return nil
	}

	version, description, hash, err := latestMigrationBaseline(fsys)
	if err != nil {
		return fmt.Errorf("atlas baseline version: %w", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO atlas_schema_revisions (version, description, type, applied, total, executed_at, execution_time, hash)
		VALUES ($1, $2, $3, 0, 0, NOW(), 0, $4)
	`, version, description, 1, hash); err != nil {
		return fmt.Errorf("insert atlas baseline: %w", err)
	}
	return nil
}

func tableExists(ctx context.Context, db migrationConnection, tableName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, tableName).Scan(&exists)
	return exists, err
}

func latestMigrationBaseline(fsys fs.FS) (string, string, string, error) {
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return "", "", "", err
	}
	if len(files) == 0 {
		return "baseline", "baseline", "", nil
	}
	sort.Strings(files)
	name := files[len(files)-1]
	contentBytes, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", "", "", err
	}
	content := strings.TrimSpace(string(contentBytes))
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	version := strings.TrimSuffix(name, ".sql")
	return version, version, hash, nil
}

func checksumSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func newMigrationChecksumCompatibilityRule(fileChecksum string, acceptedDBChecksums ...string) migrationChecksumCompatibilityRule {
	return migrationChecksumCompatibilityRule{
		fileChecksum:       fileChecksum,
		acceptedDBChecksum: checksumSet(acceptedDBChecksums...),
		acceptedChecksums:  checksumSet(append([]string{fileChecksum}, acceptedDBChecksums...)...),
	}
}

func isMigrationChecksumCompatible(name, dbChecksum, fileChecksum string) bool {
	rule, ok := migrationChecksumCompatibilityRules[name]
	if !ok {
		return false
	}
	_, dbOK := rule.acceptedChecksums[dbChecksum]
	if !dbOK {
		return false
	}
	_, fileOK := rule.acceptedChecksums[fileChecksum]
	return fileOK
}

func validateMigrationExecutionMode(name, content string) (bool, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	upperContent := strings.ToUpper(content)
	nonTx := strings.HasSuffix(normalizedName, nonTransactionalMigrationSuffix)

	if !nonTx {
		if strings.Contains(upperContent, "CONCURRENTLY") {
			return false, errors.New("CONCURRENTLY statements must be placed in *_notx.sql migrations")
		}
		return false, nil
	}

	if strings.Contains(upperContent, "BEGIN") || strings.Contains(upperContent, "COMMIT") || strings.Contains(upperContent, "ROLLBACK") {
		return false, errors.New("*_notx.sql must not contain transaction control statements (BEGIN/COMMIT/ROLLBACK)")
	}

	statements := splitSQLStatements(content)
	for _, stmt := range statements {
		cleanedStmt := stripSQLLineComment(strings.TrimSpace(stmt))
		normalizedStmt := strings.ToUpper(cleanedStmt)
		if normalizedStmt == "" {
			continue
		}

		if strings.Contains(normalizedStmt, "CONCURRENTLY") {
			isCreateIndex := strings.Contains(normalizedStmt, "CREATE") && strings.Contains(normalizedStmt, "INDEX")
			isDropIndex := strings.Contains(normalizedStmt, "DROP") && strings.Contains(normalizedStmt, "INDEX")
			if !isCreateIndex && !isDropIndex {
				return false, errors.New("*_notx.sql currently only supports CREATE/DROP INDEX CONCURRENTLY statements")
			}
			if isCreateIndex && !strings.Contains(normalizedStmt, "IF NOT EXISTS") {
				return false, errors.New("CREATE INDEX CONCURRENTLY in *_notx.sql must include IF NOT EXISTS for idempotency")
			}
			if isDropIndex && !strings.Contains(normalizedStmt, "IF EXISTS") {
				return false, errors.New("DROP INDEX CONCURRENTLY in *_notx.sql must include IF EXISTS for idempotency")
			}
			// 索引名必须可解析：prepareNonTransactionalMigration 依赖它清理上一次中断留下的
			// INVALID 索引。若解析不出来，这个文件就会静默失去清理能力（正是 M6 的成因），
			// 所以宁可在启动时直接报错，也不放行一个"看起来正常"的迁移。
			if isCreateIndex {
				if _, ok := parseConcurrentlyCreatedIndexName(cleanedStmt); !ok {
					return false, errors.New("CREATE INDEX CONCURRENTLY in *_notx.sql must use the form " +
						"`CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS <unquoted_index_name> ON ...` " +
						"so the runner can drop the INVALID index left behind by an interrupted build")
				}
			}
			continue
		}

		return false, errors.New("*_notx.sql must not mix non-CONCURRENTLY SQL statements")
	}

	return true, nil
}

func splitSQLStatements(content string) []string {
	parts := strings.Split(content, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func stripSQLLineComment(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// pgAdvisoryLock 获取 PostgreSQL Advisory Lock。
// Advisory Lock 是一种轻量级的锁机制，不与任何特定的数据库对象关联。
// 它非常适合用于应用层面的分布式锁场景，如迁移序列化。
type advisoryLockConnection interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func pgAdvisoryLock(ctx context.Context, db advisoryLockConnection) error {
	ticker := time.NewTicker(migrationsLockRetryInterval)
	defer ticker.Stop()

	for {
		var locked bool
		if err := db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", migrationsAdvisoryLockID).Scan(&locked); err != nil {
			return fmt.Errorf("acquire migrations lock: %w", err)
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire migrations lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// pgAdvisoryUnlock 释放 PostgreSQL Advisory Lock。
// 必须在获取锁后确保释放，否则会阻塞其他实例的迁移操作。
func pgAdvisoryUnlock(ctx context.Context, db advisoryLockConnection) error {
	_, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationsAdvisoryLockID)
	if err != nil {
		return fmt.Errorf("release migrations lock: %w", err)
	}
	return nil
}
