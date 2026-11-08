export default {
  intelligenceCheck: {
    title: '智力检测',
    description: '让账号跑一次「鹈鹕骑自行车」绘图题，看看模型的真实智力水平。',
    subtitle: '已开启智力检测的账号各跑一次「鹈鹕骑自行车」绘图题，产出可直接预览的动态 HTML 作品。跑测只负责产出作品，通过与否由管理员人工评审；卡片数量等于已开启的账号数量，每张卡显示该账号最近一次的结果。',
    refresh: '刷新',
    runAll: '全部重跑',
    running: '跑测中…',
    rerun: '重跑',
    empty: '还没有账号。请先到「账号管理」添加账号。',
    noEnabledAccounts: '还没有账号启用智力检测。请到「账号管理」→ 编辑账号 → 智力检测，为账号打开开关。',
    previewNotice: '作品由 AI 生成，脚本在隔离沙箱中运行，可安全预览。',
    accountLabel: '账号 {index}',
    badge: {
      pass: '智力通过',
      fail: '未通过',
      unknown: '待评审',
      runFailed: '执行失败',
      running: '跑测中',
      untested: '未跑测'
    },
    card: {
      noModel: '未记录模型',
      accountMeta: '#{id} · {model}',
      accountMetaWithEffort: '#{id} · {model} · {effort}',
      publicMetaWithEffort: '{model} · 智力等级 {effort}',
      detail: '{latency} · {time}',
      noArtifact: '暂无作品',
      pendingThumbnail: '作品加载中…',
      generating: '作品生成中…',
      untestedHint: '还没有跑测记录',
      approve: '通过',
      reject: '不通过',
      reviewHint: '待评审：请人工查看作品后判定',
      reviewed: '评审于 {time}',
      reviewedNoTime: '已完成评审'
    },
    reviewSubmitted: '已提交评审：{verdict}',
    reviewSubmittedSynced: '已提交评审：{verdict}（已联动账号状态）',
    error: {
      load: '加载智力检测数据失败',
      trigger: '提交跑测任务失败',
      review: '提交评审失败'
    }
  }
}
