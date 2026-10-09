export default {
  relay: {
    keepStatus: '保持当前状态',
    pending: '待结算', pendingNotice: '待结算收益 {amount}：消费者补足欠费后，在刷新收益时自动入账。',
    title: '我的中转站', description: '接入自己的上游免费私用，也可以共享账号赚取站内余额。',
    privateNotice: '私有组仅供本人使用，本站不扣费；上游费用和额度照常消耗。',
    publicNotice: '公共组按组内价格扣费，并给实际提供账号的人分成；用到自己的账号也获得收益。公共请求会经过提供者的上游。',
    balanceNotice: '收益直接加入站内余额，仅用于消费，不支持提现。',
    totalEarned: '累计收益', todayEarned: '今日收益（UTC）', accounts: '我的上游账号', privateGroups: '我的私有组',
    earnings: '收益明细', addAccount: '接入上游', editAccount: '编辑上游', addGroup: '创建私有组', editGroup: '编辑私有组',
    emptyAccounts: '接入一个上游账号，即可开始使用自己的中转站。', emptyEarnings: '共享账号被调用后，收益会显示在这里。',
    name: '名称', platform: '平台', type: '凭证类型', apiKey: 'API Key', oauth: 'OAuth 凭证', baseUrl: '上游地址',
    baseUrlHint: '留空使用该平台默认地址；自建中转站填写公网 HTTP(S) 地址。',
    secret: '上游 API Key', secretHint: '编辑时留空保留原凭证。', concurrency: '账号最大并发',
    credentialJson: '其他凭证（JSON，可选）', credentialHint: 'OAuth 账号填写 access_token、refresh_token 等；可配置 model_mapping。',
    privateSelection: '私用分组', publicSelection: '共享到公共组', publicSelectionHint: '勾选后参与该组的公共调度；取消勾选可停止共享。私用和共享共用上游额度。',
    supplierRate: '提供者分成比例（0–1）', supplierRateHint: '仅对公共余额组内用户提供的账号生效，0.9 表示分成 90%。',
    shared: '已共享', privateOnly: '仅私用', groups: '所属分组', status: '状态', active: '启用', disabled: '停用',
    createKey: '管理调用密钥', createKeyHint: '在 API 密钥页面创建密钥，选择你的私有组，即可免费调用。',
    saved: '已保存', saveFailed: '保存失败', loadFailed: '加载失败', deleted: '已删除', deleteFailed: '删除失败',
    deleteTitle: '删除资源', deleteMessage: '删除后停止新请求，历史收益流水保留。确认删除？',
    invalidJson: '其他凭证必须是 JSON 对象', groupRequired: '至少选择一个私有组或公共组',
    accountId: '账号', groupId: '分组', charged: '消费额', earned: '收益', time: '时间', selfUse: '自己使用',
    previous: '上一页', next: '下一页', page: '第 {page} 页', defaultGroup: '默认私有组',
    descriptionLabel: '说明', refresh: '刷新'
  }
}
