export default {
  upstreamBalance: {
    title: '上游余额监控', description: '集中查看 Sub2API 与 New-API 上游的账户余额。', add: '添加上游', addTitle: '添加上游', editTitle: '编辑上游', refreshAll: '刷新全部', enabledOnly: '只看已启用', empty: '暂无上游监控配置',
    remaining: '剩余额度', used: '已用额度', requests: '请求次数', group: '所属分组', account: '上游账号', probe: '手动刷新', never: '尚未探测',
    channelStatus: '渠道状态', healthy: '健康', todayChanges: '今日倍率变动', changedToday: '今日有倍率调整', noChangesToday: '今日无变动', recentChanges: '最近倍率变动', totalGroups: '当前分组总数', groups: '当前分组与倍率',
    loadError: '加载上游余额失败', saveError: '保存上游配置失败', probeError: '探测上游失败', deleteError: '删除上游失败', deleteConfirm: '确定删除“{name}”吗？',
    form: { name: '站点名称', type: '上游类型', baseUrl: '站点地址', credentialMode: '登录方式', passwordMode: '账号密码（自动登录）', tokenMode: 'Token / Cookie', email: '登录邮箱', username: '登录用户名', password: '登录密码', passwordHelp: '账号密码将加密保存；启用 2FA 或登录验证码的账号无法自动登录。', accessToken: 'Access Token', accessTokenPlaceholder: '登录接口返回的 access_token', accessTokenHelp: '使用 Sub2API 登录接口返回的访问令牌，不是 sk- 开头的 API Key。', cookie: '浏览器 Cookie', cookieHelp: '登录 New-API 后，从浏览器开发者工具复制完整 Cookie 请求头。', userId: 'New-Api-User 用户 ID', userIdHelp: '可在 New-API 个人设置页查看。', interval: '轮询间隔（分钟）', order: '排列顺序', lowThreshold: '低余额阈值（美元）', enabled: '启用监控' },
    status: { ok: '正常', low: '余额低', failed: '探测失败', pending: '等待探测', disabled: '未启用' },
  },
}
