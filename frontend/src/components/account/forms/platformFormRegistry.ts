import { defineAsyncComponent, type Component } from 'vue'

const BUILTIN_FORMS: Record<string, () => Promise<{ default: Component }>> = {
  anthropic: () => import('./AnthropicForm.vue'),
  openai: () => import('./OpenAIForm.vue'),
  gemini: () => import('./GeminiForm.vue'),
  antigravity: () => import('./AntigravityForm.vue'),
}

export function resolvePlatformForm(platform: string): Component {
  const loader = BUILTIN_FORMS[platform]
  if (loader) return defineAsyncComponent(loader)
  return defineAsyncComponent(() => import('./PluginForm.vue'))
}
