export default {
  officialPricing: {
    title: '官方价格管理',
    description: '集中维护按量模型的官方基础价，并在应用同步候选前检查每一项差异。',
    eyebrow: 'Token 官价工作台',
    check: '检查官方价格',
    checking: '正在检查',
    refresh: '刷新模型',
    tokenOnly: '这里只管理按 Token 计费模型的官方价；按次价格由管理员单独定价，不参与官价同步。',
    isolationNotice: '官方基础价仅用于用户价格页展示与倍率计算，不会直接修改渠道成本、分组定价或实际扣费。',
    loadFailed: '加载官方价格失败',
    search: '搜索模型或厂商',
    modelCount: '{count} 个按量模型',
    providerCount: '{count} 个厂商',
    providerModels: '{count} 个模型',
    noModels: '暂无匹配的按量模型',
    manual: {
      title: '手动制定官价',
      hint: '直接编辑四个官方价字段。留空表示该价格项暂无官价；手动保存后来源会标记为“管理员手动维护”，展示倍率、备注及其他计费配置保持不变。',
      save: '保存官价',
      saving: '保存中',
      saved: '模型 {model} 的官方价已保存',
      saveFailed: '保存模型官方价失败',
      invalid: '官方价必须为空或大于等于 0'
    },
    fields: {
      model: '模型',
      input: '输入',
      output: '输出',
      cacheWrite: '缓存写入',
      cacheRead: '缓存读取',
      source: '来源与时间',
      actions: '操作',
      unit: '每 100 万 Token'
    },
    metadata: {
      neverSynced: '尚未同步',
      syncedAt: '同步于 {time}',
      sourceLink: '查看来源',
      sourceUnknown: '未标注来源'
    },
    sync: {
      title: '官价同步差异',
      hint: '同步只生成候选，不会自动改价。检查来源、时间和差异后，勾选需要应用的模型。',
      fetchedAt: '检查时间：{time}',
      candidates: '{count} 条候选',
      changed: '{count} 条有变化',
      applicable: '{count} 条可应用',
      selected: '{count} 条已选择',
      current: '当前官价',
      proposed: '候选官价',
      evidence: '来源证据',
      status: '状态',
      applySelected: '应用所选',
      applying: '正在应用',
      applied: '已应用 {count} 个模型的官方价',
      noSelection: '请至少选择一条可应用的价格候选',
      empty: '未发现可展示的官价候选',
      checkFailed: '检查官方价格失败',
      applyFailed: '应用官方价格失败；候选可能已过期，请重新检查',
      selectAll: '选择全部可应用项',
      changedStatus: '价格有变化',
      noChange: '价格无变化',
      applicableStatus: '可以应用',
      notApplicable: '不可应用',
      reasonFallback: '来源数据不完整或模型版本不匹配',
      sourceUpdatedAt: '来源更新：{time}',
      reference: '官方参考链接',
      unverifiedWarning: '聚合来源只作为候选线索，不能视为厂商已核验官价。请核对参考链接后再应用。'
    },
    source: {
      herohao_aggregate: '聚合候选 · 未核验',
      provider_official: '厂商官方来源',
      manual: '管理员手动维护',
      unknown: '未知来源'
    },
    confidence: {
      high: '高可信',
      medium: '中等可信',
      low: '低可信',
      unverified: '未核验',
      unknown: '可信度未知'
    },
    reason: {
      unsupported_billing_mode: '该模型不是按 Token 计费，不参与官方价格同步',
      currency_mismatch: '候选币种与当前模型展示币种不一致',
      candidate_not_found: '聚合来源中没有找到完全匹配的模型',
      provider_mismatch: '模型厂商与候选来源不一致',
      candidate_disabled: '来源中的该模型当前已停用',
      candidate_price_missing: '来源没有提供可应用的官方价格字段'
    }
  }
}
