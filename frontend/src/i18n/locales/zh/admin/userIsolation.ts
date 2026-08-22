export default {
  userIsolationLookup: {
    title: '风险用户定位',
    description: '上游用户隔离标识反查',
    account: '上游账号（可选）',
    selectAccount: '全部已开启用户隔离的账号',
    searchAccount: '搜索账号名称',
    noEligibleAccounts: '没有匹配的用户隔离账号',
    isolationID: '上游用户标识',
    isolationIDPlaceholder: 'u1_...',
    lookup: '定位用户',
    locating: '定位中...',
    result: '定位结果',
    exactMatch: '精确匹配',
    userID: '用户 ID',
    email: '邮箱',
    username: '用户名',
    status: '状态',
    lastActiveAt: '最近活跃',
    lastUsedAt: '最近使用',
    viewUser: '用户管理',
    viewUsage: '用量记录',
    unknown: '-',
    errors: {
      INVALID_USER_ISOLATION_ID: '上游用户标识格式无效',
      USER_ISOLATION_NOT_ENABLED: '所选账号未开启用户隔离',
      USER_ISOLATION_USER_NOT_FOUND: '未找到匹配用户',
      USER_ISOLATION_ACCOUNTS_NOT_FOUND: '没有已开启用户隔离的账号',
      USER_ISOLATION_LOOKUP_BUSY: '另一个全局定位任务正在运行，请稍后重试',
      ACCOUNT_NOT_FOUND: '上游账号不存在',
      USER_ISOLATION_SECRET_UNAVAILABLE: '用户隔离密钥不可用',
      default: '风险用户定位失败'
    }
  }
}
