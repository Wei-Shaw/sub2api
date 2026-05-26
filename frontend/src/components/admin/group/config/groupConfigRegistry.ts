/**
 * Group config form dynamic resolver.
 *
 * Resolution priority:
 *   1. Plugin-provided group config components (plugin entry assets.groupConfigComponents)
 *   2. Generic JSON schema config form (PluginGroupConfig.vue) -- final fallback
 *
 * Public API unchanged: resolveGroupConfigComponent(platform) returns Component | null.
 */
import { defineAsyncComponent, type Component } from 'vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { findPluginManifest } from '@/plugins/loader'
import { loadPluginEntry } from '@/plugins/loader-runtime'

/** All builtin group config components have been migrated to plugins. */
const BUILTIN_CONFIGS: Record<string, () => Promise<{ default: Component }>> = {}

/**
 * Resolve the group config component for a given platform.
 *
 * Returns a defineAsyncComponent-wrapped Component, usable directly with
 * <component :is="..."/>. Internally falls back from plugin -> builtin -> generic.
 */
export function resolveGroupConfigComponent(platform: string): Component | null {
  return defineAsyncComponent(() => resolveConfigComponent(platform))
}

async function resolveConfigComponent(
  platform: string,
): Promise<{ default: Component }> {
  // 1. Try plugin entry assets for group config component
  const pluginComponent = await resolveFromPlugin(platform)
  if (pluginComponent) {
    return { default: pluginComponent }
  }

  // 2. Fall back to builtin config component (migration compat -- currently empty)
  const builtinLoader = BUILTIN_CONFIGS[platform]
  if (builtinLoader) {
    return builtinLoader()
  }

  // 3. Final fallback: generic JSON schema config form
  return import('./PluginGroupConfig.vue')
}

/**
 * Resolve group config component from plugin entry assets.
 * Symmetric with platformFormRegistry's resolveFromPlugin.
 */
async function resolveFromPlugin(platform: string): Promise<Component | null> {
  const { getPlatformDecl, fetchPlatforms } = usePlatforms()

  await fetchPlatforms()

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

    // Match by group_config.form_component_path
    const componentPath = decl.group_config?.form_component_path
    if (componentPath && groupComponents[componentPath]) {
      return groupComponents[componentPath]
    }

    // Fallback: try platform name as key
    if (groupComponents[platform]) {
      return groupComponents[platform]
    }

    return null
  } catch {
    // Plugin load failure -- silent fallback
    return null
  }
}
