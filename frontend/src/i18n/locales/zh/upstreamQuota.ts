export default {
  title: '上游额度',
  description: '只读查看管理员公开的账号额度快照。',
  refresh: '刷新', loading: '加载中…', disabledPage: '尚未启用上游额度公开展示。',
  modelNote: '反重力显示模型额度快照。同类模型中额度和重置时间相同的数据合并显示，不同的数据分别编号；不据此推断 5 小时或 7 天窗口。',
  platforms: '平台', cacheNote: '额度快照共享缓存 3 分钟，刷新不会强制查询上游。页面不自动轮询。',
  loadError: '暂时无法读取额度，请稍后重试。', empty: '此平台暂无公开账号。',
  unknown: '暂无额度数据', unavailable: '额度不可用', concurrency: '当前请求',
  remaining: '剩余', resets: '重置于', sampleTime: '额度采样于',
  status: { available: '可用', rate_limited: '已限流', disabled: '已停用', error: '异常' },
  windows: {
    five_hour: '5 小时', seven_day: '7 天',
    gemini_shared_minute: '共享池 / 分钟', gemini_pro_minute: 'Pro / 分钟', gemini_flash_minute: 'Flash / 分钟',
    gemini_shared_daily: '共享池 / 日', gemini_pro_daily: 'Pro / 日', gemini_flash_daily: 'Flash / 日',
    gemini_model: 'Gemini 模型额度', claude_model: 'Claude 模型额度', gpt_model: 'GPT 模型额度'
  },
  admin: {
    title: '用户端上游额度', description: '默认关闭。所选账号会对全部已登录用户展示，不受用户所属分组限制。仅公开匿名编号、额度、重置时间和并发。',
    enable: '启用额度公开展示', accounts: '公开的账号 ID', hint: '从账号管理页获取 ID，以英文逗号分隔，最多 200 个。不会公开账号名称或凭证。',
    save: '保存额度公开设置', saved: '已保存，重新加载用户页面后更新导航。', error: '无法读取或保存额度公开设置。', invalid: '请输入最多 200 个正整数账号 ID。'
  }
}
