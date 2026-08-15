export default {
  organization: {
    console: '企业管理', accountId: '账号 ID', accountType: { label: '账号类型', personal: '个人账号', company: '企业账号' }, accountIdentity: { label: '账号身份', root: '主账号', iam: '子账号' }, iamUserId: 'IAM 成员 ID', principal: '登录主体', companyName: '企业名称', companyId: '公司 ID', companySize: '公司规模', role: '管理权限', policies: '权限策略', reviewReason: '审批意见',
    roleValue: { owner: '组织管理员', member: 'IAM 成员' },
    status: { pending: '待审批', approved: '已通过', rejected: '已拒绝', withdrawn: '已撤回', active: '正常', disabled: '已禁用', archived: '已归档', suspended: '已暂停' },
    tabs: { members: '成员', allocation: '资金划拨', finance: '财务管理', limits: '限额配置', dashboard: '仪表盘', subscriptions: '订阅套餐', usage: '使用记录', audit: '操作记录', settings: '功能设置' },
    audit: {
      title: '操作记录',
      description: '记录企业相关的充值、授权、划拨与限额配置操作。',
      categoryLabel: '类别',
      allCategories: '全部类别',
      categories: { recharge: '充值', authorize: '授权', allocate: '划拨', spend_limit: '限额配置', other: '其他' },
      time: '时间',
      actor: '操作人',
      subject: '操作对象',
      action: '动作',
      result: '结果',
      detail: '详情',
      resultValues: { success: '成功', denied: '被拒绝', failed: '失败' },
      actions: {
        organization_balance_company_deposit: '转入企业余额',
        organization_balance_company_withdraw: '转出企业余额',
        iam_policy_change: '权限策略变更',
        organization_balance_allocate: '余额划拨',
        organization_balance_reclaim: '余额回收',
        spend_limit_upsert: '配置成员限额',
        spend_limit_delete: '删除成员限额',
        organization_subscription_admin_assign: '分配订阅套餐',
        organization_subscription_admin_revoke: '回收订阅套餐'
      },
      empty: '暂无操作记录。',
      total: '共 {total} 条',
      previous: '上一页',
      next: '下一页'
    },
    authorization: { title: '授权 {name}', subtitle: '勾选要授予该用户的权限策略', empty: '暂无可用权限策略' },
    policyMeta: {
      CompanyFinanceReadOnly: { name: '企业财务只读', description: '查看主账号的可用、冻结与总余额，并可查看企业的仪表盘与使用记录。' },
      CompanySharedBalanceUse: { name: '共享余额使用', description: '可使用公司余额进行消费，但不可查看金额。' },
      CompanyFinanceManage: { name: '企业财务管理', description: '查看主账号余额、仪表盘与使用记录，并可划拨/回收成员余额、配置成员限额、购买或管理企业套餐。' },
      IAMUserManage: { name: 'IAM 用户管理', description: '创建 IAM 成员并重置其登录密码（不包含禁用与归档）。' }
    },
    login: { personal: '个人账号', iam: 'IAM 登录', title: 'IAM 成员登录', subtitle: '使用完整登录账号和密码登录', loginName: '登录名称', principal: '登录账号', genericError: '登录账号或密码错误' },
    upgrade: { title: '升级企业账户', backToProfile: '返回个人资料', feeLabel: '升级费用', feeNotice: '提交申请后，升级费用将从可用余额中冻结。审批通过后才会正式扣除；审批拒绝或中止申请时，冻结费用将退还至余额。', chargedFee: '费用快照', free: '免费', submit: '提交审批', withdraw: '中止', insufficientBalance: '余额不足，无法冻结升级费用', companySizePlaceholder: '请选择公司规模', companySizeInvalid: '请选择有效的公司规模', ineligible: { not_personal_root: '当前身份不能申请企业升级。', already_company_account: '当前账号已经是企业账户。', application_pending: '已有一项企业升级申请等待审批。', unknown: '当前账号不符合企业升级条件。' } },
	nameChange: { action: '申请更名', title: '申请企业更名', submit: '提交审批', pending: '更名申请已提交，审批通过前当前企业名称保持不变。' },
    password: { title: '修改初始密码', new: '新密码', confirm: '确认新密码', mismatch: '两次输入的密码不一致' },
	recovery: { title: 'IAM 恢复邮箱', code: '验证码', send: '发送验证码', verify: '验证邮箱', change: '更换邮箱', sent: '验证码已发送。', verified: '恢复邮箱已验证。' },
	members: { slots: 'IAM 成员 {used}/{limit}', create: '创建 IAM 成员', username: '用户名（可选）', usernamePlaceholder: '用于展示的用户名', password: '密码', generatePassword: '自动生成密码', showPassword: '显示密码', hidePassword: '隐藏密码', mustChangePassword: '强制该成员首次登录时修改密码', recoveryEmail: '恢复邮箱（可选）', allocateFunds: '划拨/收回资金', resetPassword: '重置密码', disable: '禁用', enable: '启用', archive: '归档', archiveConfirm: '确认归档 IAM 成员 {name}？归档后不能恢复。', authorize: '授权', oneTimeCredential: '一次性登录凭据', oneTimeWarning: '关闭后将无法再次查看该密码。', copied: '凭据已复制' },
    spendLimits: { configure: '配置成员限额', description: '仅统计共享企业余额和企业订阅消费，成员个人划拨余额不计入。成员专属规则优先于全员规则。', target: '限额粒度', allMembers: '全员限额', selectedMembers: '指定成员', noMembers: '暂无可配置成员', dailyLimit: '每日限额', monthlyLimit: '每月限额', enableAlert: '开启邮件告警', alertThreshold: '告警阈值', additionalRecipients: '指定接收人', recipientsPlaceholder: '输入邮箱或选择企业成员', recipientsHint: '支持按企业成员的恢复邮箱自动补全，也可手动添加邮箱。', removeRecipient: '移除接收人', rules: '已配置规则', alert: '告警', currentUsage: '当前成员用量', dailyUsage: '今日用量 / 限额', monthlyUsage: '本月用量 / 限额', unlimited: '未限额', noUsage: '暂无成员用量', deleteConfirm: '确认删除这条成员限额规则？' },
    allocation: { amount: '金额', allocate: '划拨', reclaim: '收回', rootAvailable: '公司账户当前可划拨余额：{amount}', targetAvailable: '目标账户当前可用余额' },
    finance: { available: '可用余额', frozen: '冻结余额', total: '总余额', companyBalance: '企业余额', company_available: '企业可用余额', company_frozen: '企业冻结余额', company_total: '企业总余额', noPermission: '无查看权限', transferAmount: '转入企业', deposit: '转入企业', withdraw: '转回个人', depositAvailable: '可转入', withdrawAvailable: '可转回', companyBalanceHint: '从个人余额转入企业余额，或从企业余额转回个人。企业 API 密钥将消耗企业余额。' },
    balanceSource: { label: '扣费来源', self: '主账号余额', allocated: '子账号划拨余额', shared: '历史共享余额', company: '企业余额', subscription: '企业订阅套餐' },
    subscriptions: { description: '为企业开通订阅套餐（分组），企业 API 密钥可绑定这些套餐使用其额度。', createTitle: '开通订阅套餐', group: '套餐分组', selectGroup: '请选择分组', createHint: '开通订阅套餐需付费下单，与个人订阅一致，费用由当前所有者支付，订阅将开通到公司主体。', empty: '暂无订阅套餐', rate: '倍率', status: '状态', usage: '用量', expiresAt: '到期时间', daily: '日', monthly: '月', cancel: '取消订阅', statuses: { active: '生效中', expired: '已过期', cancelled: '已取消' } },
    dashboard: { requests: '请求数', tokens: 'Token', cost: '实际费用', members: '公司成员' },
    usage: { member: '登录名', username: '用户名', allMembers: '全部成员', apiKey: 'API 密钥', apiKeyId: 'API 密钥 ID', searchApiKeyPlaceholder: '按名称搜索 API 密钥...', model: '模型', endpoint: '接口', tokens: 'Token', charge: '实际扣费', duration: '耗时（毫秒）', time: '请求时间', start: '开始时间', end: '结束时间', charged: '已扣费', refunded: '已退款', total: '共 {total} 条', previous: '上一页', next: '下一页', columnSettings: '展示列', statRequests: '总请求数', statInputTokens: '输入 Token', statOutputTokens: '输出 Token', statCost: '总消费', trendTitle: '每日用量趋势', trendEmpty: '暂无趋势数据' },
    admin: { title: '企业账户审批', applicant: '申请账号', similar: '相似企业名', approve: '通过', reject: '拒绝', upgrades: '账户升级', nameChanges: '企业更名', organizations: '企业组织', currentName: '当前名称', requestedName: '申请名称', audit: '审计记录', members: 'IAM 成员', suspend: '暂停', reactivate: '恢复' },
    settings: {
      title: '企业功能设置',
      description: '管理企业级功能开关。设置对本企业下所有成员生效。',
      autoSwitchSubscription: {
        label: '自动切换订阅套餐',
        description: '开启后，绑定了企业订阅的 API 密钥在当前套餐额度用完（或订阅失效）时，会自动尝试使用同平台下一个仍有额度的企业套餐，按套餐开通时间从早到晚查找。',
        on: '已开启',
        off: '已关闭'
      },
      save: '保存',
      saving: '保存中...',
      saved: '设置已保存',
      noPermission: '你没有权限修改企业功能设置。',
      fallback: {
        badge: '自动切换',
        badgeOff: '未启用自动切换',
        chainTitle: '套餐切换顺序',
        chainCurrent: '当前',
        chainNext: '候选 #{index}',
        chainEmpty: '暂无可切换的候选套餐（本企业下同平台无其它可用订阅）。',
        loading: '加载中...',
        loadFailed: '加载套餐顺序失败。',
        tooltipHelp: '查看自动切换说明与套餐切换顺序',
        tooltipIntro: '当前套餐额度用完（或订阅失效）时，将按下列顺序自动尝试同平台其它企业套餐。'
      }
    }
  }
}
