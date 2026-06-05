/**
 * Platform account form dynamic resolver.
 *
 * Resolution priority:
 *   1. Plugin-provided form components (plugin entry assets.formComponents)
 *   2. Built-in forms (BUILTIN_FORMS) -- migration-period compat, remove as platforms migrate
 *   3. Generic JSON schema form (PluginForm.vue) -- final fallback
 *
 * Primary API: resolveFormComponentAsync(platform) -- returns a Promise<Component | null>.
 * Callers store the result in a shallowRef and use <component :is="ref"> for rendering.
 * This avoids the defineAsyncComponent reactive cascade that caused blank rendering.
 */
import { type Component } from 'vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { findPluginManifest } from '@/plugins/loader'
import { loadPluginEntry } from '@/plugins/loader-runtime'
import type { PlatformDeclaration } from '@/api/admin/platforms'

/** Built-in form mapping -- migration compat. Remove entries as platforms move to plugins. */
const BUILTIN_FORMS: Record<string, () => Promise<{ default: Component }>> = {
}

/**
 * Resolve the account form component for a given platform.
 *
 * Returns the Component directly, or null on failure.
 * Never throws -- logs warnings on unexpected failures.
 *
 * Resolution: plugin -> builtin -> generic (PluginForm.vue).
 */
export async function resolveFormComponentAsync(
  platform: string,
): Promise<Component | null> {
  try {
    const pluginComponent = await resolveFromPlugin(platform)
    if (pluginComponent) return pluginComponent

    const builtinLoader = BUILTIN_FORMS[platform]
    if (builtinLoader) {
      const mod = await builtinLoader()
      return mod.default
    }

    const fallback = await import('./PluginForm.vue')
    return fallback.default
  } catch (err) {
    console.warn(`[platformFormRegistry] failed to resolve form for "${platform}"`, err)
    return null
  }
}

/**
 * Resolve form component from plugin entry assets.
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
    if (result.error || !result.assets?.formComponents) {
      return null
    }

    const formComponents = result.assets.formComponents

    const componentPath = findFormComponentPath(decl)
    if (componentPath && formComponents[componentPath]) {
      return formComponents[componentPath]
    }

    if (formComponents[platform]) {
      return formComponents[platform]
    }

    return null
  } catch {
    return null
  }
}

/**
 * Find first account type with a form_component_path in the platform declaration.
 */
function findFormComponentPath(decl: PlatformDeclaration): string | null {
  for (const at of decl.account_types) {
    if (at.form_component_path) {
      return at.form_component_path
    }
  }
  return null
}
