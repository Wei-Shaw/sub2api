<!--
  SupportChatWidget.vue

  顶层容器：负责"是否渲染"决策 + 挂 bubble + 挂 panel。
  挂在 App.vue 顶层，全站可见（除被排除的路由）。

  shouldRender =
    public_settings.support_chat_enabled === true
    && current path NOT in hardcoded excluded
    && current path NOT in admin-configured excluded_routes（支持 `*` 后缀通配）

  硬编码排除：登录、注册、忘密、onboarding。这些页面浮窗会干扰首次体验。

  路由规则：
    - 完全相等 / startsWith 结尾的 `*` 后缀通配 → 命中。
    - 不写正则避免 admin 误配引入复杂度，简单 prefix 通配已覆盖典型场景。
-->

<template>
  <template v-if="shouldRender">
    <SupportChatBubble />
    <SupportChatPanel />
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import SupportChatBubble from './SupportChatBubble.vue'
import SupportChatPanel from './SupportChatPanel.vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const route = useRoute()

/** 硬编码排除路径：登录 / 注册 / 找回密码 / onboarding。 */
const HARDCODED_EXCLUDED: readonly string[] = [
  '/login',
  '/register',
  '/reset-password',
  '/forgot-password',
  '/setup',
  '/onboarding',
  '/onboarding/*',
]

/** 当前路径是否命中给定 patterns 之一。`/foo/*` 视为前缀匹配（prefix `/foo/`）。 */
function matchPath(current: string, patterns: readonly string[]): boolean {
  for (const raw of patterns) {
    const p = (raw || '').trim()
    if (!p) continue
    if (p.endsWith('/*')) {
      const prefix = p.slice(0, -1) // 包含尾部斜杠
      if (current === prefix.slice(0, -1) || current.startsWith(prefix)) return true
    } else if (p.endsWith('*')) {
      const prefix = p.slice(0, -1)
      if (current.startsWith(prefix)) return true
    } else {
      if (current === p) return true
    }
  }
  return false
}

const enabled = computed<boolean>(
  () => appStore.cachedPublicSettings?.support_chat_enabled === true
)

const adminExcluded = computed<readonly string[]>(() => {
  const list = appStore.cachedPublicSettings?.support_chat_excluded_routes
  return Array.isArray(list) ? list : []
})

const inExcludedRoute = computed<boolean>(() => {
  const path = route.path || '/'
  return (
    matchPath(path, HARDCODED_EXCLUDED) || matchPath(path, adminExcluded.value)
  )
})

const shouldRender = computed<boolean>(() => enabled.value && !inExcludedRoute.value)
</script>
