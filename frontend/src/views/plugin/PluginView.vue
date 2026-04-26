<template>
  <div class="plugin-view">
    <header class="plugin-view__header">
      <h1 class="plugin-view__title">
        {{ displayName || pluginName }}
      </h1>
      <p v-if="componentPath" class="plugin-view__subtitle">
        {{ componentPath }}
      </p>
    </header>

    <section class="plugin-view__body">
      <!-- 加载中 -->
      <div v-if="state === 'loading'" class="plugin-view__placeholder">
        <p class="plugin-view__placeholder-text">{{ loadingText }}</p>
      </div>

      <!-- 加载失败 / 没有 entry_js_url 的旧插件 / 找不到组件: 显示错误 + 重试 -->
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

      <!-- 真渲染插件页面 -->
      <keep-alive v-else-if="state === 'ready' && resolvedComponent">
        <component :is="resolvedComponent" />
      </keep-alive>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, shallowRef, watch, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { findPluginManifest } from '@/plugins/loader'
import { loadPluginEntry, resolvePluginComponent, unloadPlugin } from '@/plugins/loader-runtime'

/**
 * PluginView 是插件页面的真实容器。
 * - 路由进来后从 manifest 拿到 entry_js_url, 通过 loader-runtime 动态加载;
 * - 加载完成后按 meta.componentPath 查表渲染插件提供的组件;
 * - 加载失败时显示错误 + 重试按钮, 不影响 host;
 * - 旧版插件 (没有 entry_js_url) 降级到原占位文案, 保持兼容。
 */

type ViewState = 'loading' | 'placeholder' | 'ready' | 'error'

const route = useRoute()

const pluginName = computed(() => stringMeta('pluginName'))
const displayName = computed(() => stringMeta('pluginDisplayName'))
const componentPath = computed(() => stringMeta('componentPath'))

const pageName = computed(() => {
  const param = route.params.pageName
  if (Array.isArray(param)) {
    return param[0] ?? ''
  }
  return typeof param === 'string' ? param : ''
})

const state = ref<ViewState>('loading')
const resolvedComponent = shallowRef<Component | null>(null)
const errorDetail = ref<string>('')
const canRetry = ref<boolean>(false)

// 简单文案. host i18n 已经能覆盖菜单/按钮等通用区域, 这里直接给中文+英文兜底
const loadingText = computed(() => `Loading plugin "${displayName.value || pluginName.value}"...`)
const errorText = computed(() => 'Failed to load plugin page.')
const retryText = 'Retry'
const placeholderText = computed(() => {
  if (!pluginName.value) {
    return 'Plugin context unavailable.'
  }
  return `Plugin "${displayName.value || pluginName.value}" page is loading...`
})

watch(
  () => [pluginName.value, componentPath.value, route.fullPath] as const,
  () => {
    void load()
  },
  { immediate: true },
)

async function load(): Promise<void> {
  state.value = 'loading'
  resolvedComponent.value = null
  errorDetail.value = ''
  canRetry.value = false

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
  if (result.error || !result.assets) {
    state.value = 'error'
    errorDetail.value = result.error?.message ?? 'unknown error'
    canRetry.value = true
    return
  }

  const component = resolvePluginComponent(result.assets, componentPath.value)
  if (!component) {
    state.value = 'error'
    errorDetail.value = `component not found: ${componentPath.value || '(unspecified)'}`
    canRetry.value = false
    return
  }

  resolvedComponent.value = component
  state.value = 'ready'
}

function retry(): void {
  if (pluginName.value) {
    unloadPlugin(pluginName.value)
  }
  void load()
}

function stringMeta(key: string): string {
  const v = route.meta[key]
  return typeof v === 'string' ? v : ''
}
</script>

<style scoped>
.plugin-view {
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  height: 100%;
  min-height: 0;
}

.plugin-view__header {
  border-bottom: 1px solid rgba(0, 0, 0, 0.08);
  padding-bottom: 0.75rem;
}

.plugin-view__title {
  font-size: 1.5rem;
  font-weight: 600;
  margin: 0;
}

.plugin-view__subtitle {
  margin-top: 0.25rem;
  font-size: 0.875rem;
  color: rgba(107, 114, 128, 1);
}

.plugin-view__body {
  flex: 1;
  display: flex;
  align-items: stretch;
  justify-content: stretch;
  min-height: 0;
}

.plugin-view__body > :not(.plugin-view__placeholder) {
  flex: 1;
  min-height: 0;
}

.plugin-view__placeholder {
  text-align: center;
  padding: 2rem;
  border: 1px dashed rgba(0, 0, 0, 0.15);
  border-radius: 0.75rem;
  background-color: rgba(249, 250, 251, 0.6);
  width: 100%;
  max-width: 32rem;
  margin: auto;
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

:global(.dark) .plugin-view__subtitle,
:global(.dark) .plugin-view__placeholder-text,
:global(.dark) .plugin-view__placeholder-meta {
  color: rgba(209, 213, 219, 1);
}
</style>
