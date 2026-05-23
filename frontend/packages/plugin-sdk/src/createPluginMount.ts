/**
 * Shared plugin mount helper for V2 Shadow DOM protocol plugins.
 *
 * Wraps the boilerplate that channel-management and payment used to
 * duplicate: componentPath dispatch, fallback red-text rendering, shadow
 * root container lookup, and Vue app teardown.
 *
 * Plugin index.ts only needs to provide its own views map, plugin label
 * (with `plugin-` prefix) and SDK handle, e.g.:
 *
 *   return {
 *     mount: createPluginMount(sdk, VIEWS, 'plugin-channel-management'),
 *   }
 *
 * Error / fallback messages emit `[<label>] ...` verbatim, matching the
 * unified `[plugin-<name>]` prefix policy used across the project.
 */
import type { Component } from 'vue'

import type {
  HostSdk,
  PluginInstance,
  PluginRuntimeContext,
} from './host-sdk'

export type PluginMountFn = (
  shadowRoot: ShadowRoot,
  ctx: PluginRuntimeContext,
) => PluginInstance

export interface CreatePluginMountOptions {
  /**
   * Components registered globally on the plugin's Vue app. Callers should
   * pass `SDK_GLOBAL_COMPONENTS` (re-exported from the SDK entry) merged
   * with any plugin-local globals they need.
   */
  globalComponents: Record<string, Component>
}

export function createPluginMount(
  sdk: HostSdk,
  views: Record<string, Component>,
  pluginLabel: string,
  options: CreatePluginMountOptions,
): PluginMountFn {
  const components = options.globalComponents
  return (shadowRoot, ctx) => {
    const view = views[ctx.componentPath]
    if (!view) {
      const fallback = document.createElement('div')
      fallback.style.padding = '2rem'
      fallback.style.color = '#dc2626'
      fallback.textContent = `[${pluginLabel}] unknown component: ${ctx.componentPath || '(empty)'}`
      const root = shadowRoot.querySelector('.plugin-shadow-root') as HTMLElement | null
      ;(root || shadowRoot).appendChild(fallback)
      return { unmount: () => fallback.remove() }
    }

    const target = shadowRoot.querySelector('.plugin-shadow-root') as HTMLElement | null
    if (!target) {
      throw new Error(`[${pluginLabel}] plugin-shadow-root container not found in shadow root`)
    }

    const instance = sdk.runtime.createApp(view, target, { components })
    return { unmount: () => instance.unmount() }
  }
}
