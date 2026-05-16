/**
 * 分组配置表单动态解析器.
 *
 * 解析优先级:
 *   1. 插件提供的分组配置组件 (plugin entry assets.groupConfigComponents)
 *   2. 内置配置组件 (BUILTIN_CONFIGS) — 迁移期兼容
 *   3. 通用 JSON schema 配置表单 (PluginGroupConfig.vue) — 最终兜底
 *
 * 对外接口不变: resolveGroupConfigComponent(platform) 返回 Component | null.
 */
import { defineAsyncComponent, type Component } from 'vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { findPluginManifest } from '@/plugins/loader'
import { loadPluginEntry } from '@/plugins/loader-runtime'

/** 内置分组配置组件映射 — 迁移期兼容, 各平台迁入插件后可逐条移除. */
const BUILTIN_CONFIGS: Record<string, () => Promise<{ default: Component }>> = {
  anthropic: () => import('./AnthropicGroupConfig.vue'),
  openai: () => import('./OpenAIGroupConfig.vue'),
  antigravity: () => import('./AntigravityGroupConfig.vue'),
  gemini: () => import('./GeminiGroupConfig.vue'),
}

/**
 * 解析指定平台的分组配置组件.
 *
 * 返回 defineAsyncComponent 包装的 Component, 可直接用于 <component :is="..."/>.
 * 内部按 plugin -> builtin -> generic 三级降级.
 */
export function resolveGroupConfigComponent(platform: string): Component | null {
  return defineAsyncComponent(() => resolveConfigComponent(platform))
}

async function resolveConfigComponent(
  platform: string,
): Promise<{ default: Component }> {
  // 1. 尝试从插件 entry assets 取分组配置组件
  const pluginComponent = await resolveFromPlugin(platform)
  if (pluginComponent) {
    return { default: pluginComponent }
  }

  // 2. 降级到内置配置组件 (迁移期兼容)
  const builtinLoader = BUILTIN_CONFIGS[platform]
  if (builtinLoader) {
    return builtinLoader()
  }

  // 3. 最终兜底: 通用 JSON schema 配置表单
  return import('./PluginGroupConfig.vue')
}

/**
 * 从插件 entry assets 中解析分组配置组件.
 * 逻辑与 platformFormRegistry 中的 resolveFromPlugin 对称.
 */
async function resolveFromPlugin(platform: string): Promise<Component | null> {
  const { getPlatformDecl } = usePlatforms()
  const decl = getPlatformDecl(platform)
  if (!decl?.plugin_name) {
    return null
  }

  const manifest = findPluginManifest(decl.plugin_name)
  if (!manifest?.entry_js_url) {
    return null
  }

  try {
    const result = await loadPluginEntry({
      pluginName: manifest.name,
      entryJsUrl: manifest.entry_js_url,
      entryCssUrl: manifest.entry_css_url || undefined,
      isolation: manifest.isolation,
    })
    if (result.error || !result.assets?.groupConfigComponents) {
      return null
    }

    const groupComponents = result.assets.groupConfigComponents

    // 按 group_config.form_component_path 精确匹配
    const componentPath = decl.group_config?.form_component_path
    if (componentPath && groupComponents[componentPath]) {
      return groupComponents[componentPath]
    }

    // 没有 form_component_path 时, 尝试用 platform 名作为 key
    if (groupComponents[platform]) {
      return groupComponents[platform]
    }

    return null
  } catch {
    // 插件加载失败, 静默降级
    return null
  }
}
