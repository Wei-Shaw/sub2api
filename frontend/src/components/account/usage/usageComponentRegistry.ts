/**
 * Usage display component dynamic resolver.
 *
 * Resolution priority:
 *   1. Plugin-provided usage components (plugin entry assets.usageComponents)
 *   2. null -- caller falls back to KeyAccountStats or generic display
 *
 * External interface: resolveUsageComponent(platform, accountType) returns
 * Component | null. Returns null when platforms haven't loaded yet or no plugin
 * is available, otherwise wraps in defineAsyncComponent.
 */
import { defineAsyncComponent, type Component } from 'vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { findPluginManifest, type PluginManifest } from '@/plugins/loader'
import { loadPluginEntry } from '@/plugins/loader-runtime'

/**
 * Resolve the usage display component for a platform:accountType pair.
 *
 * Returns defineAsyncComponent-wrapped Component, or null if platform data
 * is not yet loaded or no plugin is available. Callers use the null return
 * for conditional rendering (hasUsageComponent, quotaInfoComponent).
 */
export function resolveUsageComponent(platform: string, accountType: string): Component | null {
  const { getPlatformDecl, loaded } = usePlatforms()

  // When platforms aren't loaded yet, return null (callers fall back).
  // Once platforms load, the parent's reactive dependencies recompute.
  if (!loaded.value) return null

  const decl = getPlatformDecl(platform)
  if (!decl?.plugin_name) return null

  const manifest = findPluginManifest(decl.plugin_name)
  if (!manifest?.entry_js_url) return null

  return defineAsyncComponent(() => resolveComponent(platform, accountType, manifest))
}

async function resolveComponent(
  platform: string,
  accountType: string,
  manifest: PluginManifest,
): Promise<{ default: Component }> {
  const pluginComponent = await resolveFromPluginManifest(platform, accountType, manifest)
  if (pluginComponent) {
    return { default: pluginComponent }
  }

  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      name: 'EmptyUsageFallback',
      render() { return h('div', { class: 'text-xs text-gray-400' }, '-') },
    }),
  }
}

/**
 * Resolve usage component from a pre-validated plugin manifest.
 *
 * Flow:
 *   a. Load plugin entry (cached)
 *   b. Look up in assets.usageComponents matching:
 *      usage_display.component_path → platform:accountType → platform:* → platform
 */
async function resolveFromPluginManifest(
  platform: string,
  accountType: string,
  manifest: PluginManifest,
): Promise<Component | null> {
  try {
    const result = await loadPluginEntry({
      pluginName: manifest.name,
      entryJsUrl: manifest.entry_js_url,
      entryCssUrl: manifest.entry_css_url || undefined,
      isolation: manifest.isolation,
    })
    if (result.error || !result.assets?.usageComponents) {
      return null
    }

    const usageComponents = result.assets.usageComponents

    // Match by usage_display.component_path first
    const { getPlatformDecl } = usePlatforms()
    const decl = getPlatformDecl(platform)
    const componentPath = decl?.usage_display?.component_path
    if (componentPath && usageComponents[componentPath]) {
      return usageComponents[componentPath]
    }

    // Then try platform:accountType exact match
    const exactKey = `${platform}:${accountType}`
    if (usageComponents[exactKey]) {
      return usageComponents[exactKey]
    }

    // Then try platform:* wildcard
    const wildcardKey = `${platform}:*`
    if (usageComponents[wildcardKey]) {
      return usageComponents[wildcardKey]
    }

    // Then try platform name as key
    if (usageComponents[platform]) {
      return usageComponents[platform]
    }

    return null
  } catch {
    // Plugin load failure: silently fall through
    return null
  }
}
