export default {
  organization: {
    console: '企业控制台', accountId: '账号 ID', accountType: { label: '账号类型', personal: '个人账号', company: '企业账号' }, accountIdentity: { label: '账号身份', root: '主账号', iam: '子账号' }, iamUserId: 'IAM 用户 ID', principal: '登录主体', companyName: '企业名称', role: '管理权限', policies: '权限策略', reviewReason: '审批意见',
    roleValue: { owner: '组织管理员', member: 'IAM 用户' },
    status: { pending: '待审批', approved: '已通过', rejected: '已拒绝', withdrawn: '已撤回', active: '正常', disabled: '已禁用', archived: '已归档', suspended: '已暂停' },
    tabs: { members: '成员', authorization: '授权', allocation: '资金划拨', finance: '财务', usage: '使用记录' },
    login: { personal: '个人账号', iam: 'IAM 登录', title: 'IAM 用户登录', subtitle: '使用登录名称、主账号 ID 和密码登录', loginName: '登录名称', genericError: '登录名称、账号 ID 或密码错误' },
    upgrade: { title: '升级企业账户', backToProfile: '返回个人资料', feeLabel: '升级费用', feeNotice: '提交申请后，升级费用将从可用余额中冻结。审批通过后才会正式扣除；审批拒绝或中止申请时，冻结费用将退还至余额。', chargedFee: '费用快照', submit: '提交审批', withdraw: '中止', insufficientBalance: '余额不足，无法冻结升级费用', ineligible: { not_personal_root: '当前身份不能申请企业升级。', already_company_account: '当前账号已经是企业账户。', application_pending: '已有一项企业升级申请等待审批。', unknown: '当前账号不符合企业升级条件。' } },
	nameChange: { action: '申请更名', title: '申请企业更名', submit: '提交审批', pending: '更名申请已提交，审批通过前当前企业名称保持不变。' },
    password: { title: '修改初始密码', new: '新密码', confirm: '确认新密码', mismatch: '两次输入的密码不一致' },
	recovery: { title: 'IAM 恢复邮箱', code: '验证码', send: '发送验证码', verify: '验证邮箱', change: '更换邮箱', sent: '验证码已发送。', verified: '恢复邮箱已验证。' },
	members: { slots: 'IAM 用户 {used}/{limit}', create: '创建 IAM 用户', recoveryEmail: '恢复邮箱（可选）', resetPassword: '重置密码', disable: '禁用', enable: '启用', archive: '归档', archiveConfirm: '确认归档 IAM 用户 {name}？归档后不能恢复。', oneTimeCredential: '一次性登录凭据', oneTimeWarning: '关闭后将无法再次查看该密码。', copied: '凭据已复制' },
    allocation: { amount: '金额', allocate: '划拨', reclaim: '收回', rootAvailable: '主账号当前可划拨余额：{amount}' },
    finance: { available: '可用余额', frozen: '冻结余额', total: '总余额' },
    balanceSource: { label: '扣费来源', self: '主账号余额', allocated: '子账号划拨余额', shared: '共享主账号余额' },
    usage: { member: '成员', allMembers: '全部成员', apiKey: 'API 密钥', apiKeyId: 'API 密钥 ID', model: '模型', endpoint: '接口', tokens: 'Token', charge: '实际扣费', duration: '耗时（毫秒）', time: '请求时间', start: '开始时间', end: '结束时间', charged: '已扣费', refunded: '已退款', total: '共 {total} 条', previous: '上一页', next: '下一页' },
    admin: { title: '企业账户审批', applicant: '申请账号', similar: '相似企业名', approve: '通过', reject: '拒绝', upgrades: '账户升级', nameChanges: '企业更名', organizations: '企业组织', currentName: '当前名称', requestedName: '申请名称', audit: '审计记录', members: 'IAM 成员', suspend: '暂停', reactivate: '恢复' }
  }
}
