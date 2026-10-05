export default {
  tutorial: {
    title: '帮助文档',
    description: 'API 接入教程与常见问题解答',
    tabs: {
      guide: '配置指南',
      faq: '常见问题',
    },

    guide: {
      model: '模型',
      client: '客户端',
      copy: '复制',
      copied: '已复制',
      tabCurl: 'curl',
      tabPython: 'Python',
      tabSdk: 'OpenAI SDK',
      configNote: '提示:可将 ANTHROPIC_DEFAULT_HAIKU_MODEL 改为对应家族的 *-flash 型号,后台轻量任务计费更低;四个模型字段也可以全部填同一个模型。',
    },

    faq: {
      searchPlaceholder: '搜索问题关键词,如:倍率、429',
      resultCount: '共 {count} 个问题',
      noResult: '没有找到相关问题,换个关键词试试',
    },
  },
}
