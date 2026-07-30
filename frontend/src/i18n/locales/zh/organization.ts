export default {
  organization: {
    console: '企业控制台', accountId: '账号 ID', accountType: { label: '账号类型', personal: '个人账号', company: '企业账号' }, accountIdentity: { label: '账号身份', root: '主账号', iam: '子账号' }, iamUserId: 'IAM 用户 ID', principal: '登录主体', companyName: '企业名称', companyId: '公司 ID', companySize: '公司规模', role: '管理权限', policies: '权限策略', reviewReason: '审批意见',
    roleValue: { owner: '组织管理员', member: 'IAM 用户' },
    status: { pending: '待审批', approved: '已通过', rejected: '已拒绝', withdrawn: '已撤回', active: '正常', disabled: '已禁用', archived: '已归档', suspended: '已暂停' },
    tabs: { members: '成员', allocation: '资金划拨', finance: '财务', subscriptions: '订阅套餐', usage: '使用记录' },
    authorization: { title: '授权 {name}', subtitle: '勾选要授予该用户的权限策略', empty: '暂无可用权限策略' },
    policyMeta: {
      CompanyFinanceReadOnly: { name: '企业财务只读', description: '查看主账号的可用、冻结与总余额。' },
      CompanySharedBalanceUse: { name: '共享余额使用', description: '使用主账号余额进行 API 消费，但不可查看其金额。' }
    },
    login: { personal: '个人账号', iam: 'IAM 登录', title: 'IAM 用户登录', subtitle: '使用完整登录账号和密码登录', loginName: '登录名称', principal: '登录账号', genericError: '登录账号或密码错误' },
    upgrade: { title: '升级企业账户', backToProfile: '返回个人资料', feeLabel: '升级费用', feeNotice: '提交申请后，升级费用将从可用余额中冻结。审批通过后才会正式扣除；审批拒绝或中止申请时，冻结费用将退还至余额。', chargedFee: '费用快照', free: '免费', submit: '提交审批', withdraw: '中止', insufficientBalance: '余额不足，无法冻结升级费用', companySizePlaceholder: '请选择公司规模', companySizeInvalid: '请选择有效的公司规模', ineligible: { not_personal_root: '当前身份不能申请企业升级。', already_company_account: '当前账号已经是企业账户。', application_pending: '已有一项企业升级申请等待审批。', unknown: '当前账号不符合企业升级条件。' } },
	nameChange: { action: '申请更名', title: '申请企业更名', submit: '提交审批', pending: '更名申请已提交，审批通过前当前企业名称保持不变。' },
    password: { title: '修改初始密码', new: '新密码', confirm: '确认新密码', mismatch: '两次输入的密码不一致' },
	recovery: { title: 'IAM 恢复邮箱', code: '验证码', send: '发送验证码', verify: '验证邮箱', change: '更换邮箱', sent: '验证码已发送。', verified: '恢复邮箱已验证。' },
	members: { slots: 'IAM 用户 {used}/{limit}', create: '创建 IAM 用户', password: '密码', generatePassword: '自动生成密码', showPassword: '显示密码', hidePassword: '隐藏密码', mustChangePassword: '强制该用户首次登录时修改密码', recoveryEmail: '恢复邮箱（可选）', allocateFunds: '划拨资金', resetPassword: '重置密码', disable: '禁用', enable: '启用', archive: '归档', archiveConfirm: '确认归档 IAM 用户 {name}？归档后不能恢复。', authorize: '授权', oneTimeCredential: '一次性登录凭据', oneTimeWarning: '关闭后将无法再次查看该密码。', copied: '凭据已复制' },
    allocation: { amount: '金额', allocate: '划拨', reclaim: '收回', rootAvailable: '主账号当前可划拨余额：{amount}', targetAvailable: '目标账户当前可用余额' },
    finance: { available: '可用余额', frozen: '冻结余额', total: '总余额', companyBalance: '企业余额', company_available: '企业可用余额', company_frozen: '企业冻结余额', company_total: '企业总余额', transferAmount: '转入企业', deposit: '转入企业', withdraw: '转回个人', depositAvailable: '可转入', withdrawAvailable: '可转回', companyBalanceHint: '从个人余额转入企业余额，或从企业余额转回个人。企业 API 密钥将消耗企业余额。' },
    balanceSource: { label: '扣费来源', self: '主账号余额', allocated: '子账号划拨余额', shared: '共享主账号余额' },
    subscriptions: { description: '为企业开通订阅套餐（分组），企业 API 密钥可绑定这些套餐使用其额度。', createTitle: '开通订阅套餐', group: '套餐分组', selectGroup: '请选择分组', create: '开通', createHint: '有效期由订阅套餐自身决定。仅可选择当前账号有权使用的订阅分组。', empty: '暂无订阅套餐', status: '状态', usage: '用量', expiresAt: '到期时间', daily: '日', monthly: '月', cancel: '取消订阅', statuses: { active: '生效中', expired: '已过期', cancelled: '已取消' } },
    usage: { member: '成员', allMembers: '全部成员', apiKey: 'API 密钥', apiKeyId: 'API 密钥 ID', model: '模型', endpoint: '接口', tokens: 'Token', charge: '实际扣费', duration: '耗时（毫秒）', time: '请求时间', start: '开始时间', end: '结束时间', charged: '已扣费', refunded: '已退款', total: '共 {total} 条', previous: '上一页', next: '下一页', statRequests: '总请求数', statInputTokens: '输入 Token', statOutputTokens: '输出 Token', statCost: '总消费', trendTitle: '每日用量趋势', trendEmpty: '暂无趋势数据' },
    admin: { title: '企业账户审批', applicant: '申请账号', similar: '相似企业名', approve: '通过', reject: '拒绝', upgrades: '账户升级', nameChanges: '企业更名', organizations: '企业组织', currentName: '当前名称', requestedName: '申请名称', audit: '审计记录', members: 'IAM 成员', suspend: '暂停', reactivate: '恢复' }
  }
}
