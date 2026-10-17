package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// gatewayStickySuccessModelDigest 把客户端请求模型摘要成定长片段，避免模型名
// 里的分隔符影响键结构，也避免超长模型名撑大键。
func gatewayStickySuccessModelDigest(model string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(model)))
	return hex.EncodeToString(sum[:8])
}

// gatewayStickySuccessCacheKey 是通用路径（Anthropic / Gemini / Antigravity /
// composite 等共享 GatewayService 的协议）成功粘性偏好的键。
//
// 布局 sticky_session:{groupID}:success:{sessionHash}:{sha(model)}：
//   - groupID 前缀沿用旧粘性键，保证分组隔离；
//   - model 摘要进键，保证一种模型的 fallback 不会污染另一种模型的可用偏好。
//
// 它与旧的 sticky_session:{groupID}:{sessionHash} 是两个独立键：旧键继续承载
// 「上一次用过的账号」，本键只承载「上一次真正成功的账号」。
func gatewayStickySuccessCacheKey(groupID int64, sessionHash, model string) string {
	return buildSessionKey(groupID, "success:"+sessionHash+":"+gatewayStickySuccessModelDigest(model))
}

func (c *gatewayCache) GetGatewayStickySuccess(ctx context.Context, groupID int64, sessionHash, model string) (service.GatewayStickySuccessBinding, error) {
	var binding service.GatewayStickySuccessBinding
	data, err := c.rdb.Get(ctx, gatewayStickySuccessCacheKey(groupID, sessionHash, model)).Bytes()
	if errors.Is(err, redis.Nil) {
		return binding, service.ErrStickySessionNotFound
	}
	if err != nil {
		return binding, err
	}
	err = json.Unmarshal(data, &binding)
	return binding, err
}

// compareAndSwapGatewayStickySuccess 整串比对当前值：只有本请求开始时观察到的
// 那一版偏好仍然在位才允许写入。晚完成的旧请求因此不会覆盖另一个请求已经提交
// 的更新偏好（ARGV[1] 为空串表示「要求该键当前不存在」）。
var compareAndSwapGatewayStickySuccess = redis.NewScript(`
local current = redis.call('GET', KEYS[1]) or ''
if current ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)

func (c *gatewayCache) CompareAndSwapGatewayStickySuccess(ctx context.Context, groupID int64, sessionHash, model string, expected, next service.GatewayStickySuccessBinding, ttl time.Duration) (bool, error) {
	previous := ""
	if expected.AccountID > 0 {
		data, err := json.Marshal(expected)
		if err != nil {
			return false, err
		}
		previous = string(data)
	}
	data, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	changed, err := compareAndSwapGatewayStickySuccess.Run(ctx, c.rdb, []string{gatewayStickySuccessCacheKey(groupID, sessionHash, model)}, previous, string(data), ttl.Milliseconds()).Int()
	return changed == 1, err
}
