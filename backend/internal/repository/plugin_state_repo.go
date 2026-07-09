package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/pluginstate"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
	"github.com/redis/go-redis/v9"
)

const (
	// pluginStateChannel 是插件启停状态变更的跨实例广播频道。
	pluginStateChannel = "plugin:state:changed"
	// pluginStateReconcileInterval 是定时对账周期：广播丢失时最迟经一个周期收敛。
	pluginStateReconcileInterval = 60 * time.Second
	// pluginStateOpTimeout 是后台加载/对账单次 DB 操作的超时。
	pluginStateOpTimeout = 30 * time.Second
)

// pluginStateMessage 是启停变更的广播负载。
// Origin 标记发送方实例：本实例的变更在 SetEnabled 中已同步生效并回调，
// 订阅侧据此跳过自己发出的广播，保证同一变更至多触发一次本地回调。
type pluginStateMessage struct {
	Origin  string `json:"origin"`
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// PluginStateStore 是 pluginkit.StateStore 的持久化实现：
// DB（plugin_states 表）为唯一事实源，内存快照供热路径 O(1) 读，
// Redis Pub/Sub 跨实例广播，定时全量对账兜底广播丢失。
type PluginStateStore struct {
	client     *ent.Client
	rdb        *redis.Client
	instanceID string

	// writeMu 串行化「DB 写 → 内存快照写」复合操作（SetEnabled）以及
	// reconcile 的「全量读 → 快照替换」，保证内存写序与 DB 提交序一致。
	// 否则并发 toggle 下本实例内存可与 DB 漂移，最迟要等一个对账周期才收敛。
	// 订阅者回调始终在 writeMu 之外交付（非持锁上下文契约不受影响）。
	writeMu sync.Mutex

	// publishMu 让广播序跟随 DB 提交序：SetEnabled 在持 writeMu 时先取
	// publishMu 再释放 writeMu（锁交接），Publish 的网络调用因此发生在
	// writeMu 之外——Redis 抖动时慢广播只串行化广播本身，不再阻塞后续
	// toggle 的 DB 写与 reconcile 对账。乱序广播不可接受：远端实例会以
	// 过期状态运行至多一个对账周期。
	publishMu sync.Mutex

	mu      sync.RWMutex
	enabled map[pluginkit.ID]bool

	subMu       sync.Mutex
	subscribers []func(pluginkit.StateChange)

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

var _ pluginkit.StateStore = (*PluginStateStore)(nil)

// NewPluginStateStore 创建启停状态机：全量加载 DB 状态到内存快照，
// 并启动广播订阅与定时对账两个后台循环。加载或订阅失败时返回错误（拒绝启动）。
func NewPluginStateStore(client *ent.Client, rdb *redis.Client) (*PluginStateStore, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &PluginStateStore{
		client:     client,
		rdb:        rdb,
		instanceID: newPluginStateInstanceID(),
		enabled:    make(map[pluginkit.ID]bool),
		ctx:        ctx,
		cancel:     cancel,
	}

	loadCtx, loadCancel := context.WithTimeout(ctx, pluginStateOpTimeout)
	defer loadCancel()
	snapshot, err := s.loadAll(loadCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("plugin state store: initial load: %w", err)
	}
	s.enabled = snapshot

	pubsub := rdb.Subscribe(ctx, pluginStateChannel)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		cancel()
		return nil, fmt.Errorf("plugin state store: subscribe %s: %w", pluginStateChannel, err)
	}

	s.wg.Add(2)
	go s.subscribeLoop(pubsub)
	go s.reconcileLoop()

	return s, nil
}

// Enabled 只读内存快照，无 IO；未知 ID 返回 false（无记录 = disabled）。
func (s *PluginStateStore) Enabled(id pluginkit.ID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled[id]
}

// Lookup 除启用态外报告是否存在显式记录。内存快照对每一行（无论 true/false）
// 都有条目，故 comma-ok 即存在性判定。
func (s *PluginStateStore) Lookup(id pluginkit.ID) (bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	enabled, exists := s.enabled[id]
	return enabled, exists
}

// SetEnabled 按契约顺序生效：DB upsert → 本地内存 → Redis 广播 → 本地订阅者回调。
// 广播失败不回滚本地生效（其他实例由对账周期兜底收敛）。
func (s *PluginStateStore) SetEnabled(ctx context.Context, id pluginkit.ID, enabled bool, updatedBy string) error {
	if err := id.Validate(); err != nil {
		return err
	}

	s.writeMu.Lock()
	err := s.client.PluginState.Create().
		SetID(string(id)).
		SetEnabled(enabled).
		SetUpdatedBy(updatedBy).
		SetUpdatedAt(time.Now()).
		OnConflictColumns(pluginstate.FieldID).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		s.writeMu.Unlock()
		return fmt.Errorf("plugin state store: upsert %s: %w", id, err)
	}

	s.mu.Lock()
	s.enabled[id] = enabled
	s.mu.Unlock()

	// 锁交接：先取 publishMu 再放 writeMu，广播序保持 DB 提交序，
	// 而 Publish 网络调用不占用 writeMu（见字段注释）。
	s.publishMu.Lock()
	s.writeMu.Unlock()
	if payload, err := json.Marshal(pluginStateMessage{Origin: s.instanceID, ID: string(id), Enabled: enabled}); err != nil {
		slog.Warn("plugin_state_broadcast_marshal_failed", "plugin_id", string(id), "error", err)
		// 广播脱离请求 ctx 的取消：DB 已提交、本地已生效，admin 断连不该把
		// 跨实例收敛降级到对账周期（最迟 60s 的状态漂移窗口）。
	} else if err := s.rdb.Publish(context.WithoutCancel(ctx), pluginStateChannel, payload).Err(); err != nil {
		slog.Warn("plugin_state_broadcast_failed", "plugin_id", string(id), "error", err)
	}
	s.publishMu.Unlock()

	s.notify(pluginkit.StateChange{ID: id, Enabled: enabled})
	return nil
}

// Subscribe 注册状态变更回调；本实例与跨实例变更均触发，回调在非持锁上下文中调用。
func (s *PluginStateStore) Subscribe(fn func(pluginkit.StateChange)) {
	s.subMu.Lock()
	s.subscribers = append(s.subscribers, fn)
	s.subMu.Unlock()
}

// Close 停止广播订阅与对账循环并等待其退出，幂等。
func (s *PluginStateStore) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
	})
	return nil
}

// notify 在非持锁上下文中依次调用订阅者回调（允许回调内再读 Enabled）。
func (s *PluginStateStore) notify(change pluginkit.StateChange) {
	s.subMu.Lock()
	subs := make([]func(pluginkit.StateChange), len(s.subscribers))
	copy(subs, s.subscribers)
	s.subMu.Unlock()

	for _, fn := range subs {
		fn(change)
	}
}

// subscribeLoop 消费跨实例广播，直到 Close 或订阅通道关闭。
func (s *PluginStateStore) subscribeLoop(pubsub *redis.PubSub) {
	defer s.wg.Done()
	defer func() {
		if err := pubsub.Close(); err != nil {
			slog.Warn("plugin_state_pubsub_close_failed", "error", err)
		}
	}()

	ch := pubsub.Channel()
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				slog.Warn("plugin_state_subscriber_stopped", "reason", "channel_closed")
				return
			}
			s.handleBroadcast(msg.Payload)
		}
	}
}

// handleBroadcast 应用一条跨实例变更：更新内存快照并触发回调；
// 本实例发出的广播直接跳过（SetEnabled 已同步生效并回调）。
func (s *PluginStateStore) handleBroadcast(payload string) {
	var msg pluginStateMessage
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		slog.Warn("plugin_state_broadcast_malformed", "error", err)
		return
	}
	if msg.Origin == s.instanceID {
		return
	}
	// 广播来自共享 Redis 频道，非本包写入者也可发布：非法 ID 不得进内存
	// 快照（SetEnabled/DB 侧均有校验，快照必须维持同一不变量）。
	id := pluginkit.ID(msg.ID)
	if err := id.Validate(); err != nil {
		slog.Warn("plugin_state_broadcast_invalid_id", "plugin_id", msg.ID, "error", err)
		return
	}

	s.mu.Lock()
	s.enabled[id] = msg.Enabled
	s.mu.Unlock()

	s.notify(pluginkit.StateChange{ID: id, Enabled: msg.Enabled})
}

// reconcileLoop 定时全量对账，兜底广播丢失导致的状态漂移。
func (s *PluginStateStore) reconcileLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(pluginStateReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.reconcile(s.ctx); err != nil {
				slog.Warn("plugin_state_reconcile_failed", "error", err)
			}
		}
	}
}

// reconcile 从 DB 全量重载并替换内存快照，对存在差异的插件补发回调。
// DB 行被删除等价于 disabled。「全量读 → 快照替换」持 writeMu 与 SetEnabled 互斥，
// 避免用旧 DB 快照覆盖穿插到位的新写入。
func (s *PluginStateStore) reconcile(ctx context.Context) error {
	changes, err := s.reconcileSnapshot(ctx)
	if err != nil {
		return err
	}
	for _, change := range changes {
		s.notify(change)
	}
	return nil
}

// reconcileSnapshot 在 writeMu 保护下完成「全量读 → 差异计算 → 快照替换」，
// 返回需补发的变更（回调由 reconcile 在锁外交付）。
func (s *PluginStateStore) reconcileSnapshot(ctx context.Context) ([]pluginkit.StateChange, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	opCtx, cancel := context.WithTimeout(ctx, pluginStateOpTimeout)
	defer cancel()
	fresh, err := s.loadAll(opCtx)
	if err != nil {
		return nil, err
	}

	var changes []pluginkit.StateChange
	s.mu.Lock()
	for id, enabled := range fresh {
		if s.enabled[id] != enabled {
			changes = append(changes, pluginkit.StateChange{ID: id, Enabled: enabled})
		}
	}
	for id, enabled := range s.enabled {
		if _, ok := fresh[id]; !ok && enabled {
			changes = append(changes, pluginkit.StateChange{ID: id, Enabled: false})
		}
	}
	s.enabled = fresh
	s.mu.Unlock()
	return changes, nil
}

// loadAll 全量读取 plugin_states 表，构建 ID → enabled 快照。
func (s *PluginStateStore) loadAll(ctx context.Context) (map[pluginkit.ID]bool, error) {
	rows, err := s.client.PluginState.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugin state store: load all: %w", err)
	}
	snapshot := make(map[pluginkit.ID]bool, len(rows))
	for _, row := range rows {
		snapshot[pluginkit.ID(row.ID)] = row.Enabled
	}
	return snapshot, nil
}

// newPluginStateInstanceID 生成实例标识，用于跳过自己发出的广播。
func newPluginStateInstanceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 不可用时退化为时间戳，仅影响自广播去重的极小概率碰撞。
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
