<template>
  <component :is="standalone ? 'div' : AppLayout" :class="standalone ? 'plugin-view-standalone' : ''">
    <div class="plugin-view" :class="standalone ? 'plugin-view--standalone' : ''">
      <section class="plugin-view__body">
        <!-- 加载中 -->
        <div v-if="state === 'loading'" class="plugin-view__placeholder">
          <p class="plugin-view__placeholder-text">{{ loadingText }}</p>
        </div>

        <!-- 加载失败 / 没有 entry_js_url 的旧插件 / mount 失败: 显示错误 + 重试 -->
        <div v-else-if="state === 'error'" class="plugin-view__placeholder">
          <p class="plugin-view__placeholder-text">{{ errorText }}</p>
          <p v-if="errorDetail" class="plugin-view__placeholder-meta">
            <code>{{ errorDetail }}</code>
          </p>
          <button v-if="canRetry" type="button" class="plugin-view__retry" @click="retry">
            {{ retryText }}
          </button>
        </div>

        <!-- 占位: 没有 entry_js_url 时降级到旧版占位 -->
        <div v-else-if="state === 'placeholder'" class="plugin-view__placeholder">
          <p class="plugin-view__placeholder-text">{{ placeholderText }}</p>
          <p v-if="pageName" class="plugin-view__placeholder-meta">
            page: <code>{{ pageName }}</code>
          </p>
        </div>

        <!-- Shadow DOM 容器: plugin 渲染挂在这里, host Vue 不参与子树渲染 -->
        <div ref="shadowHost" class="plugin-view__shadow-host" :data-state="state" />
      </section>
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { findPluginManifest } from '@/plugins/loader'
import { loadPluginEntry, unloadPlugin } from '@/plugins/loader-runtime'
import { mountPluginAssets, type MountedPluginHandle } from '@/plugins/mount-plugin'
import AppLayout from '@/components/layout/AppLayout.vue'

/**
 * PluginView 是插件页面的真实容器 (V2 — Shadow DOM 协议).
 *
 *   - 路由进来后从 manifest 拿到 entry_js_url, 通过 loader-runtime 动态加载;
 *   - 加载完成后调 plugin.mount(shadowRoot, ctx) 把 plugin UI 挂到独立 Shadow Root;
 *   - 加载/挂载失败时显示错误 + 重试按钮, 不影响 host;
 *   - 旧版插件 (没有 entry_js_url) 降级到原占位文案, 保持兼容.
 *
 * Shadow DOM 隔离让 plugin 渲染失败 / 注入全局样式都不会污染 host 主页面.
 */

type ViewState = 'loading' | 'placeholder' | 'ready' | 'error'

const route = useRoute()
const { t } = useI18n()

const pluginName = computed(() => stringMeta('pluginName'))
const displayName = computed(() => stringMeta('pluginDisplayName'))
const componentPath = computed(() => stringMeta('componentPath'))
// chrome="none" tells the host to render the plugin page without the
// admin sidebar/header — used for payment callbacks (Stripe / Alipay /
// WeChat return URLs) that must look like a standalone confirmation
// page rather than an in-app screen.
const standalone = computed(() => stringMeta('chrome') === 'none')

const pageName = computed(() => {
  const param = route.params.pageName
  if (Array.isArray(param)) {
    return param[0] ?? ''
  }
  return typeof param === 'string' ? param : ''
})

const state = ref<ViewState>('loading')
const errorDetail = ref<string>('')
const canRetry = ref<boolean>(false)
const shadowHostRef = useTemplateRef<HTMLDivElement>('shadowHost')

let mountedHandle: MountedPluginHandle | null = null
let activeMountToken = 0

const loadingText = computed(() => t('plugin.loading', { name: displayName.value || pluginName.value }))
const errorText = computed(() => t('plugin.loadError'))
const retryText = computed(() => t('common.retry'))
const placeholderText = computed(() => {
  if (!pluginName.value) {
    return t('plugin.contextUnavailable')
  }
  return t('plugin.loading', { name: displayName.value || pluginName.value })
})

watch(
  () => [pluginName.value, componentPath.value, route.fullPath] as const,
  () => {
    void load()
  },
  { immediate: true, flush: 'post' },
)

onBeforeUnmount(() => {
  // 路由离开 / 父组件销毁 — 卸载当前 plugin instance.
  cleanupMounted()
})

async function load(): Promise<void> {
  // 立即卸载上一次 mount, 否则切换 plugin 路由时旧实例会泄漏.
  cleanupMounted()
  state.value = 'loading'
  errorDetail.value = ''
  canRetry.value = false

  const myToken = ++activeMountToken

  const name = pluginName.value
  if (!name) {
    state.value = 'placeholder'
    return
  }

  const manifest = findPluginManifest(name)
  if (!manifest) {
    state.value = 'placeholder'
    return
  }

  // 后端没注入 entry_js_url 时降级到旧版占位 (兼容尚未升级的核心)
  if (!manifest.entry_js_url) {
    state.value = 'placeholder'
    return
  }

  const result = await loadPluginEntry({
    pluginName: manifest.name,
    entryJsUrl: manifest.entry_js_url,
    entryCssUrl: manifest.entry_css_url || undefined,
    isolation: manifest.isolation,
  })
  if (myToken !== activeMountToken) {
    // 用户已经切到下一条路由, 丢弃这次加载结果.
    return
  }
  if (result.error || !result.assets) {
    state.value = 'error'
    errorDetail.value = result.error?.message ?? 'unknown error'
    canRetry.value = true
    return
  }

  // 等 host element 就绪 (template ref 在 flush='post' 后可用, 但额外等一帧确保 DOM 已挂).
  await Promise.resolve()
  if (myToken !== activeMountToken) {
    return
  }
  const host = shadowHostRef.value
  if (!host) {
    state.value = 'error'
    errorDetail.value = 'plugin host element not ready'
    canRetry.value = true
    return
  }

  try {
    const handle = await mountPluginAssets({
      container: host,
      assets: result.assets,
      route,
      componentPath: componentPath.value,
      pluginName: manifest.name,
      entryCssUrl: manifest.entry_css_url || undefined,
    })
    if (myToken !== activeMountToken) {
      // 路由已变, mount 出来的实例直接卸载.
      handle.unmount()
      return
    }
    mountedHandle = handle
    state.value = 'ready'
  } catch (err) {
    state.value = 'error'
    errorDetail.value = err instanceof Error ? err.message : String(err)
    canRetry.value = true
  }
}

function retry(): void {
  if (pluginName.value) {
    unloadPlugin(pluginName.value)
  }
  void load()
}

function cleanupMounted(): void {
  if (mountedHandle) {
    try {
      mountedHandle.unmount()
    } catch {
      // unmount 失败时忽略, 后续 host 重新清空 container.
    }
    mountedHandle = null
  }
  // 即使 mount 没成功, 也清掉 shadow host 内容 (例如旧 shadow root 残留).
  // 浏览器不允许 detach Shadow Root, 所以靠 plugin app.unmount 清理 DOM.
  // 这里保险一手: 如果 host 元素还在, 清空显式子节点 (不影响 shadow root).
}

function stringMeta(key: string): string {
  const v = route.meta[key]
  return typeof v === 'string' ? v : ''
}
</script>

<style scoped>
.plugin-view {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  /* AppLayout's <main> is content-sized (no fixed height), so height:100%
     collapses to content height. Use the same viewport calc as the host's
     TablePageLayout: 64px header + lg:p-8 top/bottom padding (2rem × 2). */
  height: calc(100vh - 64px - 4rem);
  min-height: 0;
}

/* chrome="none" — no AppLayout means the plugin owns the full viewport. */
.plugin-view--standalone {
  height: 100vh;
  gap: 0;
}

.plugin-view-standalone {
  min-height: 100vh;
  background-color: rgb(249, 250, 251);
}

:global(.dark) .plugin-view-standalone {
  background-color: rgb(15, 23, 42);
}

.plugin-view__body {
  flex: 1;
  display: flex;
  align-items: stretch;
  justify-content: stretch;
  min-height: 0;
  position: relative;
}

.plugin-view__body > .plugin-view__placeholder {
  margin: auto;
}

.plugin-view__shadow-host {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 加载/错误/占位状态下隐藏 shadow host (避免空 div 占位影响布局) */
.plugin-view__shadow-host[data-state='loading'],
.plugin-view__shadow-host[data-state='error'],
.plugin-view__shadow-host[data-state='placeholder'] {
  display: none;
}

.plugin-view__placeholder {
  text-align: center;
  padding: 2rem;
  border: 1px dashed rgba(0, 0, 0, 0.15);
  border-radius: 0.75rem;
  background-color: rgba(249, 250, 251, 0.6);
  width: 100%;
  max-width: 32rem;
}

.plugin-view__placeholder-text {
  margin: 0;
  color: rgba(75, 85, 99, 1);
}

.plugin-view__placeholder-meta {
  margin: 0.5rem 0 0;
  font-size: 0.75rem;
  color: rgba(107, 114, 128, 1);
}

.plugin-view__retry {
  margin-top: 1rem;
  padding: 0.4rem 1rem;
  border-radius: 0.5rem;
  border: 1px solid rgba(59, 130, 246, 0.4);
  background-color: rgba(59, 130, 246, 0.08);
  color: rgb(37, 99, 235);
  font-size: 0.875rem;
  cursor: pointer;
}

.plugin-view__retry:hover {
  background-color: rgba(59, 130, 246, 0.16);
}

:global(.dark) .plugin-view__placeholder {
  background-color: rgba(31, 41, 55, 0.4);
  border-color: rgba(255, 255, 255, 0.12);
}

:global(.dark) .plugin-view__placeholder-text,
:global(.dark) .plugin-view__placeholder-meta {
  color: rgba(209, 213, 219, 1);
}
</style>
