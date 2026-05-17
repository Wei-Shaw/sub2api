/**
 * Platform account form dynamic resolver.
 *
 * Resolution priority:
 *   1. Plugin-provided form components (plugin entry assets.formComponents)
 *   2. Built-in forms (BUILTIN_FORMS) -- migration-period compat, remove as platforms migrate
 *   3. Generic JSON schema form (PluginForm.vue) -- final fallback
 *
 * External interface unchanged: resolvePlatformForm(platform) returns Component,
 * internally wraps async loading into defineAsyncComponent.
 */
import { defineAsyncComponent, type Component } from 'vue'
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
 * Returns defineAsyncComponent-wrapped Component for use with <component :is="...">.
 * Internally resolves: plugin -> builtin -> generic (three-tier fallback).
 */
export function resolvePlatformForm(platform: string): Component {
  return defineAsyncComponent(() => resolveFormComponent(platform))
}

async function resolveFormComponent(
  platform: string,
): Promise<{ default: Component }> {
  // 1. Try plugin entry assets
  const pluginComponent = await resolveFromPlugin(platform)
  if (pluginComponent) {
    return { default: pluginComponent }
  }

  // 2. Fallback to built-in form (migration compat)
  const builtinLoader = BUILTIN_FORMS[platform]
  if (builtinLoader) {
    return builtinLoader()
  }

  // 3. Final fallback: generic JSON schema form
  return import('./PluginForm.vue')
}

/**
 * Resolve form component from plugin entry assets.
 *
 * Flow:
 *   a. Get PlatformDeclaration from usePlatforms(), check for plugin_name
 *   b. Find manifest via plugin_name, get entry_js_url
 *   c. Load plugin entry (cached), look up form_component_path in assets.formComponents
 *
 * Returns null on any step failure, letting caller fall through to next tier.
 */
async function resolveFromPlugin(platform: string): Promise<Component | null> {
  const { getPlatformDecl, fetchPlatforms } = usePlatforms()

  // Ensure platform data is loaded before looking up plugin_name
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

    // Match by form_component_path (from account type declaration)
    const componentPath = findFormComponentPath(decl)
    if (componentPath && formComponents[componentPath]) {
      return formComponents[componentPath]
    }

    // No form_component_path: try platform name as key
    if (formComponents[platform]) {
      return formComponents[platform]
    }

    return null
  } catch {
    // Plugin load failure: silently fall through
    return null
  }
}

/**
 * Find first account type with a form_component_path in the platform declaration.
 *
 * Most platforms share one form component across all account types.
 * Future: pass accountType param to distinguish per-type forms.
 */
function findFormComponentPath(decl: PlatformDeclaration): string | null {
  for (const at of decl.account_types) {
    if (at.form_component_path) {
      return at.form_component_path
    }
  }
  return null
}
