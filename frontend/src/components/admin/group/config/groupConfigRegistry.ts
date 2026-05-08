import { defineAsyncComponent, type Component } from 'vue'

const BUILTIN_CONFIGS: Record<string, () => Promise<Component>> = {
  anthropic: () => import('./AnthropicGroupConfig.vue'),
  openai: () => import('./OpenAIGroupConfig.vue'),
  antigravity: () => import('./AntigravityGroupConfig.vue'),
  gemini: () => import('./GeminiGroupConfig.vue'),
}

export function resolveGroupConfigComponent(platform: string): Component | null {
  const loader = BUILTIN_CONFIGS[platform]
  if (loader) return defineAsyncComponent(loader)
  return defineAsyncComponent(() => import('./PluginGroupConfig.vue'))
}
