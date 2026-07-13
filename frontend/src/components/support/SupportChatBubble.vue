<!--
  SupportChatBubble.vue

  右下角浮窗 FAB（floating action button）：
    - 圆形按钮、显示 admin 配置的图标（emoji 或图片 URL）。
    - 点击 → 切换 store.isOpen，由 SupportChatPanel 自行渲染气泡 / 面板。
    - 一直渲染（除非父级 SupportChatWidget 整体 hide 掉）。父级负责
      `support_chat_enabled` / 路由排除等可见性判断。

  设计取舍：
    - 故意不在 bubble 里写"路由判断"或"feature 开关"，单一职责，逻辑都
      落在 SupportChatWidget。
    - 图标支持两种格式：emoji（文本）或图片 URL（http(s)、站点相对路径、
      或带常见图片扩展名）。URL 走 `<img>`；emoji 走文本节点。
    - 视觉：仅展示 icon 本身，不带按钮背景/边框/ring；保留轻量 drop-shadow
      让头像在白底页面也能"浮"起来不被吞掉。hover 用 scale 而非阴影增强。
    - 默认头像：admin 未配置 support_chat_icon 时回退到内置 PNG 客服头像
      （由 Vite 处理为 hashed 静态资源）。过去默认是 "💬" emoji，体验偏
      简陋；现升级为可识别的卡通客服形象。
-->

<template>
  <button
    type="button"
    class="fixed bottom-[30%] right-6 z-[110] flex h-16 w-16 items-center justify-center rounded-full transition-transform duration-200 hover:scale-110 active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/60 focus-visible:ring-offset-2"
    :aria-label="t('support.chat.toggle_aria')"
    @click="store.toggleOpen()"
  >
    <img
      v-if="iconIsUrl"
      :src="iconText"
      :alt="t('support.chat.toggle_aria')"
      class="h-16 w-16 rounded-full object-cover drop-shadow-lg"
    />
    <span
      v-else
      class="select-none text-4xl leading-none drop-shadow-md"
    >{{ iconText }}</span>

    <!-- 浮窗有未读 / 错误时的红点（M1：仅 error 时亮）。 -->
    <span
      v-if="store.error"
      class="absolute right-0 top-0 h-3 w-3 rounded-full bg-red-500 ring-2 ring-white dark:ring-dark-900"
    />
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSupportChatStore } from '@/stores/supportChat'
import { useAppStore } from '@/stores/app'
import defaultAvatarUrl from '@/assets/support-chat-default-avatar.png'

const { t } = useI18n()
const store = useSupportChatStore()
const appStore = useAppStore()

/**
 * admin 在 setting 里配置的图标，已通过 PublicSettings 公开。
 * 空串时回退到内置默认头像（PNG）。
 */
const iconText = computed<string>(() => {
  const injected = (appStore.cachedPublicSettings as any)?.support_chat_icon
  return typeof injected === 'string' && injected.length > 0 ? injected : defaultAvatarUrl
})

/**
 * 把"应当用 <img> 渲染"的字符串和"emoji 文本"区分开。识别：
 *   - http(s):// 外链
 *   - 站点相对路径（/assets/xxx.png）—— Vite 处理 import 后形如此
 *   - 显式带常见图片扩展名（.png/.jpg/.jpeg/.svg/.webp/.gif）
 * 任意一种命中都走 <img>；否则按 emoji/文本渲染。
 */
const iconIsUrl = computed<boolean>(() => {
  const v = iconText.value
  return (
    /^https?:\/\//i.test(v) ||
    v.startsWith('/') ||
    v.startsWith('./') ||
    /\.(png|jpe?g|svg|webp|gif)(\?|$)/i.test(v)
  )
})
</script>
