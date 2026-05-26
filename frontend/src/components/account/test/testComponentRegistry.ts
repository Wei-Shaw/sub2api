/**
 * Test panel component dynamic resolver.
 *
 * Resolution priority:
 *   1. Plugin-provided test components (plugin entry assets.testComponents)
 *   2. null -- caller falls back to DefaultTestPanel
 *
 * Public API: resolveTestComponent(platform) returns Promise<Component | null>.
 * Callers store the result in a shallowRef and use <component :is="ref"> for rendering.
 */
import { type Component } from 'vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { findPluginManifest } from '@/plugins/loader'
import { loadPluginEntry } from '@/plugins/loader-runtime'

/**
 * Resolve the test panel component for a given platform.
 *
 * Returns the Component directly, or null when no plugin provides one.
 * Never throws -- logs warnings on unexpected failures.
 *
 * Resolution: plugin test_component_path -> platform key -> null (fallback to DefaultTestPanel).
 */
export async function resolveTestComponent(
  platform: string,
): Promise<Component | null> {
  try {
    return await resolveFromPlugin(platform)
  } catch (err) {
    console.warn(`[testComponentRegistry] failed to resolve test component for "${platform}"`, err)
    return null
  }
}

/**
 * Resolve test component from plugin entry assets.
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
    if (result.error || !result.assets?.testComponents) {
      return null
    }

    const testComponents = result.assets.testComponents

    // Match by test_config.test_component_path first
    const componentPath = decl.test_config?.test_component_path
    if (componentPath && testComponents[componentPath]) {
      return testComponents[componentPath]
    }

    // Fallback: try platform name as key
    if (testComponents[platform]) {
      return testComponents[platform]
    }

    return null
  } catch {
    return null
  }
}
