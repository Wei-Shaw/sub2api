export default {
  qualityTest: {
    title: '质量检测',
    description: '同一提示词并排测试最多三个账号，对比输出质量',
    platform: '平台',
    reasoningEffort: '思考强度',
    effortDefault: '模型默认',
    slot: '账号 {n}',
    selectAccount: '选择账号',
    selectModel: '选择模型',
    prompt: '提示词',
    run: '开始检测',
    stop: '停止',
    duration: '耗时',
    chars: '{n} 字符',
    preview: '预览',
    source: '源码',
    loadAccountsFailed: '加载账号失败',
    loadModelsFailed: '加载模型列表失败',
    status: {
      idle: '空闲',
      running: '进行中',
      completed: '已完成',
      error: '失败',
      stopped: '已停止'
    }
  }
}
