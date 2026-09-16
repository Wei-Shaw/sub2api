package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/migrations"
)

func TestIsMigrationChecksumCompatible(t *testing.T) {
	t.Run("054历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"054_drop_legacy_cache_columns.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d",
		)
		require.True(t, ok)
	})

	t.Run("054在未知文件checksum下不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"054_drop_legacy_cache_columns.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"0000000000000000000000000000000000000000000000000000000000000000",
		)
		require.False(t, ok)
	})

	t.Run("061历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"061_add_usage_log_request_type.sql",
			"08a248652cbab7cfde147fc6ef8cda464f2477674e20b718312faa252e0481c0",
			"66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c",
		)
		require.True(t, ok)
	})

	t.Run("061第二个历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"061_add_usage_log_request_type.sql",
			"222b4a09c797c22e5922b6b172327c824f5463aaa8760e4f621bc5c22e2be0f3",
			"66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c",
		)
		require.True(t, ok)
	})

	t.Run("非白名单迁移不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"001_init.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d",
		)
		require.False(t, ok)
	})

	t.Run("109历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
		)
		require.True(t, ok)
	})

	t.Run("109当前checksum可兼容历史checksum", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
		)
		require.True(t, ok)
	})

	t.Run("109回滚到历史文件后仍兼容已应用的新checksum", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
		)
		require.True(t, ok)
	})

	t.Run("110历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"110_pending_auth_and_provider_default_grants.sql",
			"e3d1f433be2b564cfbdc549adf98fce13c5c7b363ebc20fd05b765d0563b0925",
			"32cf87ee787b1bb36b5c691367c96eee37518fa3eed6f3322cf68795e3745279",
		)
		require.True(t, ok)
	})

	t.Run("112历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"112_add_payment_order_provider_key_snapshot.sql",
			"ffd3e8a2c9295fa9cbefefd629a78268877e5b51bc970a82d9b3f46ec4ebd15e",
			"b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99",
		)
		require.True(t, ok)
	})

	t.Run("115历史checksum可兼容修复后的legacy external backfill", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"115_auth_identity_legacy_external_backfill.sql",
			"4cf39e508be9fd1a5aa41610cbbebeb80385c9adda45bf78a706de9db4f1385f",
			"022aadd97bb53e755f0cf7a3a957e0cb1a1353b0c39ec4de3234acd2871fd04f",
		)
		require.True(t, ok)
	})

	t.Run("116历史checksum可兼容修复后的legacy external safety reports", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"116_auth_identity_legacy_external_safety_reports.sql",
			"f7757bd929ac67ffb08ce69fa4cf20fad39dbff9d5a5085fb2adabb7607e5877",
			"07edb09fa8d04ffb172b0621e3c22f4d1757d20a24ae267b3b36b087ab72d488",
		)
		require.True(t, ok)
	})

	t.Run("119历史checksum可兼容占位文件", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"119_enforce_payment_orders_out_trade_no_unique.sql",
			"ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34",
			"0bbe809ae48a9d811dabda1ba1c74955bd71c4a9cc610f9128816818dfa6c11e",
		)
		require.True(t, ok)
	})

	t.Run("118多个历史checksum都可兼容当前版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"a38243ca0a72c3a01c0a92b7986423054d6133c0399441f853b99802852720fb",
			"e0cdf835d6c688d64100f483d31bc02ac9ebad414bf1837af239a84bf75b8227",
		} {
			ok := isMigrationChecksumCompatible(
				"118_wechat_dual_mode_and_auth_source_defaults.sql",
				dbChecksum,
				"b54194d7a3e4fbf710e0a3590d22a2fe7966804c487052a356e0b55f53ef96b0",
			)
			require.True(t, ok)
		}
	})

	t.Run("120多个历史checksum都可兼容新的notx修复版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61",
			"707431450603e70a43ce9fbd61e0c12fa67da4875158ccefabacea069587ab22",
			"04b082b5a239c525154fe9185d324ee2b05ff90da9297e10dba19f9be79aa59a",
		} {
			ok := isMigrationChecksumCompatible(
				"120_enforce_payment_orders_out_trade_no_unique_notx.sql",
				dbChecksum,
				"34aadc0db59a4e390f92a12b73bd74642d9724f33124f73638ae00089ea5e074",
			)
			require.True(t, ok)
		}
	})

	t.Run("119未知checksum不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"119_enforce_payment_orders_out_trade_no_unique.sql",
			"ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		)
		require.False(t, ok)
	})

	// 224 曾被就地补 'kiro' 并在部分环境应用，随后回滚为已发布版本，
	// 于是同一迁移在不同环境出现两个方向的 checksum 冲突，两向都需放行。
	t.Run("224补kiro版与已发布版双向兼容", func(t *testing.T) {
		const (
			released = "5227db3c1a6a1e2e422a9f9ba9d1f490c708b6c6dd91ce89f3c48115421a3e55"
			edited   = "4de3bf301cd838bbaf85613ce37dd47643165c0e3f36a1075341ff71aa37fae1"
			name     = "224_user_platform_quotas_add_cn_providers.sql"
		)
		// db=补kiro版 / file=已发布版（回滚后的环境，例如已应用改动版的 VPS）
		require.True(t, isMigrationChecksumCompatible(name, edited, released))
		// db=已发布版 / file=补kiro版（尚未回滚文件的环境）
		require.True(t, isMigrationChecksumCompatible(name, released, edited))
	})

	// 从官方镜像切到 fork：db 记录官方版 checksum，文件是补了 kiro/adobe 的 fork 版。
	t.Run("157/237/238官方版与fork版兼容", func(t *testing.T) {
		for _, tc := range []struct{ name, official, fork string }{
			{"157_user_platform_quotas_add_grok.sql", "5cace8fa32c6174a72721cd9b01f28f4545de1fd7bcd9ca196a4225056ec4fb8", "a918734da39c2e5a82e4a5e9511bac1f4cf7e310ceadd647df52692320633c1b"},
			{"237_add_minimax_platform.sql", "f4c73d2dbce114ca7ade1aac51998c3465490f4f3c9b3e868e53590f3fa8601b", "c754b29e15c10ef2a72887c4e2dd04a73a37c6c06218d1b2725836884450c03a"},
			{"238_opencode_go_platform.sql", "6f987e251519bd3759e60da44620a5d777494cceb333b6ce394aa0ea536ef5a2", "d310f134e119bd0b01c36e048841d04e1adc04a117c5c516ccdbbc8800742414"},
		} {
			require.True(t, isMigrationChecksumCompatible(tc.name, tc.official, tc.fork), tc.name)
			require.False(t, isMigrationChecksumCompatible(tc.name,
				"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", tc.fork), tc.name)
		}
	})

	// tw-01 现网库：2026-09-12 升级到 v0.2.4 时应用的是本 fork 自己的就地补丁版，
	// 与官方版、kiro 版都不同。不放行会让后续升级在启动时报 checksum mismatch。
	t.Run("237/238本fork历史就地补丁版兼容", func(t *testing.T) {
		for _, tc := range []struct{ name, forkLegacy, current string }{
			{"237_add_minimax_platform.sql", "f0ec18d80d35e280f848ecc44c82e476e44c6f4ff373f6f08093eb6a43d6d809", "c754b29e15c10ef2a72887c4e2dd04a73a37c6c06218d1b2725836884450c03a"},
			{"238_opencode_go_platform.sql", "79639d114ef8632b1fa49a813cf843e6b43fde00fe581f4e37e06007d340b97f", "d310f134e119bd0b01c36e048841d04e1adc04a117c5c516ccdbbc8800742414"},
		} {
			require.True(t, isMigrationChecksumCompatible(tc.name, tc.forkLegacy, tc.current), tc.name)
		}
	})

	t.Run("224未知checksum不兼容", func(t *testing.T) {
		const name = "224_user_platform_quotas_add_cn_providers.sql"
		unknown := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		require.False(t, isMigrationChecksumCompatible(name,
			"4de3bf301cd838bbaf85613ce37dd47643165c0e3f36a1075341ff71aa37fae1", unknown))
		require.False(t, isMigrationChecksumCompatible(name, unknown,
			"5227db3c1a6a1e2e422a9f9ba9d1f490c708b6c6dd91ce89f3c48115421a3e55"))
	})
}

// knownStaleCompatibilityRules 是已知的「死规则」：其迁移文件在规则写下之后又被改动，
// 但规则的 fileChecksum 未同步更新，导致 isMigrationChecksumCompatible 永不命中
// （该函数要求 db 与 file 两侧都落在接受集内）。这些环境升级时仍会撞 checksum mismatch。
//
// 逐条修复需要审阅各自 diff 判断新旧版语义是否互认，不在单次改动的范围内，先登记冻结。
// 修好一条就从此表移除 —— 下面的用例会强制这一点，避免白名单腐化。
var knownStaleCompatibilityRules = map[string]struct{}{
	"109_auth_identity_compat_backfill.sql":                   {},
	"110_pending_auth_and_provider_default_grants.sql":        {},
	"112_add_payment_order_provider_key_snapshot.sql":         {},
	"118_wechat_dual_mode_and_auth_source_defaults.sql":       {},
	"123_fix_legacy_auth_source_grant_on_signup_defaults.sql": {},
	"195_channel_monitor_mode.sql":                            {},
	"218_group_audio_voice_pricing.sql":                       {},
	"219_group_search_price_per_1k.sql":                       {},
	"220_clear_non_grok_video_generation_config.sql":          {},
}

// TestMigrationChecksumCompatibilityRules_NotStale 防漂移：兼容规则里的 checksum 是
// 硬编码值，若对应迁移文件之后又被改动，规则会静默失效（启动报 checksum mismatch，
// 且错误信息不会提示"规则已过期"，极难定位）。此用例按 ApplyMigrations 的算法
// （TrimSpace 后 SHA256）重算嵌入内容，确保每条规则对当前文件仍然可命中。
func TestMigrationChecksumCompatibilityRules_NotStale(t *testing.T) {
	for name, rule := range migrationChecksumCompatibilityRules {
		content, err := migrations.FS.ReadFile(name)
		if err != nil {
			continue // 规则对应的文件可能已删除/改名，不在本用例职责内
		}
		sum := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
		current := hex.EncodeToString(sum[:])
		_, live := rule.acceptedChecksums[current]

		if _, known := knownStaleCompatibilityRules[name]; known {
			require.Falsef(t, live,
				"%s 的兼容规则已修好（接受集已含当前 checksum %s），"+
					"请从 knownStaleCompatibilityRules 移除该条目", name, current)
			continue
		}
		require.Truef(t, live,
			"%s 的兼容规则未包含当前嵌入文件的 checksum(%s) → 规则永不命中（死规则）；"+
				"改动迁移文件时必须同步更新规则，命中它的升级路径否则会在启动时报 checksum mismatch",
			name, current)
	}
}
