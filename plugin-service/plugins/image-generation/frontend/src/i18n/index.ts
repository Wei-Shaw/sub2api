import { createI18n } from 'vue-i18n'

const en = {
  newConversation: 'New conversation', historyTitle: 'History', selectKey: 'Available key',
  promptPlaceholder: 'Describe the image you want to generate', send: 'Send image prompt',
  stop: 'Stop generation', deleteHistory: 'Delete history', confirmDelete: 'Delete this history?',
  cancel: 'Cancel', confirm: 'Delete', emptyTitle: 'No conversation yet',
  emptyDescription: 'Send the first prompt and generated images will appear here.',
  generationWaiting: 'Generating image, please wait...', generationFailed: 'Image generation failed',
  emptyGenerationResult: 'Image generation returned no images', repeatGeneration: 'Generate again',
  useAsReference: 'Use as reference', refineFromImage: 'Refine prompt', cancelGeneration: 'Cancel generation',
  retryGeneration: 'Retry generation', model: 'Model', size: 'Size', referenceImage: 'Reference image',
  clearReference: 'Clear reference image',
}

const zh: typeof en = {
  newConversation: '新建会话', historyTitle: '历史记录', selectKey: '可用 Key',
  promptPlaceholder: '描述你想生成的图片内容', send: '发送图片提示词', stop: '停止生成',
  deleteHistory: '删除历史记录', confirmDelete: '确认删除这条历史记录吗？', cancel: '取消', confirm: '删除',
  emptyTitle: '还没有会话内容', emptyDescription: '发送第一条图片生成请求后，结果会出现在这里。',
  generationWaiting: '正在生成图片，请稍候...', generationFailed: '图片生成失败',
  emptyGenerationResult: '图片生成未返回可显示的图片', repeatGeneration: '再次生成',
  useAsReference: '设为参考图', refineFromImage: '基于此图优化', cancelGeneration: '取消生成',
  retryGeneration: '重新生成', model: '模型', size: '尺寸', referenceImage: '参考图',
  clearReference: '清除参考图',
}

export const messages = {
  en: { imageGeneration: en },
  zh: { imageGeneration: zh },
}

export function createImageGenerationI18n() {
  const locale = String(navigator.language || '').toLowerCase().startsWith('zh') ? 'zh' : 'en'
  return createI18n({ legacy: false, locale, fallbackLocale: 'en', messages })
}
