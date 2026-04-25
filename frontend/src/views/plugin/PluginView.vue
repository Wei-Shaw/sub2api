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
      <div class="plugin-view__placeholder">
        <p class="plugin-view__placeholder-text">
          {{ placeholderText }}
        </p>
        <p v-if="pageName" class="plugin-view__placeholder-meta">
          page: <code>{{ pageName }}</code>
        </p>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

/**
 * PluginView 是插件页面的占位容器。
 * 后续迭代会根据 meta.componentPath 加载真正的远程组件或 iframe;
 * 当前阶段仅显示插件名与占位提示,确保路由能跳转、菜单能高亮。
 */

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

const placeholderText = computed(() => {
  if (!pluginName.value) {
    return 'Plugin context unavailable.'
  }
  return `Plugin "${displayName.value || pluginName.value}" page is loading...`
})

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
  align-items: center;
  justify-content: center;
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
