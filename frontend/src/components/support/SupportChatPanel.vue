<!--
  SupportChatPanel.vue

  浮窗对话主面板（~360×500，移动端全屏）：
    - 顶栏：title + 清空对话 + 关闭
    - 主体：FAQ quickbar（store.faqs 非空时） + 消息时间线
    - 底栏：输入框（Enter 发送 / Shift+Enter 换行 / 流式中按"停止"中断）
            + "提交工单"按钮 + 免责声明
    - 错误 banner：限流 / 网络 / api key 失效

  渲染契约：
    - shouldRender 由父级 SupportChatWidget 控制；本组件假设已"应该渲染"。
    - 是否展开则由 store.isOpen 控制，关闭时本组件不挂载（v-if）。

  样式取舍：
    - 用户消息纯文本，assistant 消息支持 Markdown（marked + DOMPurify）。
    - 流式光标用一个内联的脉冲字符 "▍"，与 streaming 状态绑定。
-->

<template>
  <div
    v-if="store.isOpen"
    class="fixed bottom-[calc(30%+2rem)] right-6 z-[111] flex h-[min(520px,85vh)] w-[420px] translate-y-1/2 flex-col overflow-hidden rounded-2xl bg-white shadow-2xl ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10 max-sm:inset-0 max-sm:bottom-0 max-sm:right-0 max-sm:top-0 max-sm:h-screen max-sm:max-h-screen max-sm:w-screen max-sm:translate-y-0 max-sm:rounded-none"
    role="dialog"
    :aria-label="t('support.chat.title')"
  >
    <!-- ============ Header ============ -->
    <header class="flex items-center justify-between border-b border-gray-100 bg-gradient-to-r from-primary-500/5 to-primary-600/10 px-4 py-3 dark:border-dark-700 dark:from-primary-500/10 dark:to-primary-600/20">
      <div class="flex items-center gap-2">
        <!--
          headerIcon 可能是 emoji 或图片 URL（admin 配置）。URL 走 <img>，
          否则按文本渲染——直接 v-text 会把 https://... 当字符串显示出来，
          这是 admin 在 SettingsView 把 support_chat_icon 改为图片 URL 后
          panel 顶部"显示的是 URL 而不是图片"的根因。
        -->
        <img
          v-if="headerIconIsUrl"
          :src="headerIcon"
          :alt="t('support.chat.title')"
          class="h-6 w-6 rounded-full object-cover"
        />
        <span v-else class="text-xl leading-none">{{ headerIcon }}</span>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('support.chat.title') }}
        </h3>
      </div>
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :title="t('support.chat.clear_session')"
          :disabled="!store.hasMessages || store.isLoading"
          @click="onClickClear"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6M1 7h22M9 7V4a2 2 0 012-2h2a2 2 0 012 2v3" />
          </svg>
        </button>
        <button
          type="button"
          class="rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :title="t('common.close')"
          @click="store.toggleOpen(false)"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </header>

    <!-- ============ Body ============ -->
    <div ref="bodyEl" class="flex-1 space-y-3 overflow-y-auto px-4 py-3">
      <!-- Welcome（消息为空时） -->
      <div
        v-if="!store.hasMessages"
        class="rounded-xl bg-gray-50 p-3 text-sm text-gray-700 dark:bg-dark-900/50 dark:text-gray-300"
      >
        {{ welcomeText }}
      </div>

      <!-- FAQ quickbar：仅在消息为空 + faqs 非空时展示 -->
      <div v-if="!store.hasMessages && store.faqs.length > 0" class="space-y-1.5">
        <div class="text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('support.chat.faq_section_title') }}
        </div>
        <button
          v-for="(faq, idx) in store.faqs"
          :key="idx"
          type="button"
          class="block w-full truncate rounded-lg border border-gray-200 bg-white px-3 py-2 text-left text-xs text-gray-700 transition-colors hover:border-primary-400 hover:bg-primary-50 dark:border-dark-600 dark:bg-dark-900/30 dark:text-gray-300 dark:hover:border-primary-500 dark:hover:bg-primary-900/20"
          @click="onClickFaq(faq)"
        >
          {{ faq.question }}
        </button>
      </div>

      <!-- 消息列表 -->
      <div v-for="m in store.messages" :key="m.id" :class="bubbleWrapClass(m.role)">
        <div :class="bubbleClass(m.role, m.errored)">
          <!-- assistant: Markdown 渲染（仅基础元素被 sanitize 过） -->
          <div
            v-if="m.role === 'assistant'"
            class="markdown-body prose prose-sm max-w-none dark:prose-invert"
            v-html="renderAssistant(m.content)"
          />
          <!-- user / system: 纯文本，保留换行 -->
          <div v-else class="whitespace-pre-wrap text-sm">{{ m.content }}</div>

          <!-- 流式光标 -->
          <span v-if="m.streaming" class="ml-0.5 inline-block animate-pulse">▍</span>
        </div>
      </div>

      <!-- 错误 banner -->
      <div
        v-if="store.error"
        class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300"
      >
        <div class="flex items-start justify-between gap-2">
          <span>{{ errorMessage }}</span>
          <button
            type="button"
            class="shrink-0 rounded px-2 py-0.5 text-xs font-medium text-red-700 underline-offset-2 hover:underline dark:text-red-300"
            @click="onClickRetry"
          >
            {{ t('support.chat.retry') }}
          </button>
        </div>
      </div>
    </div>

    <!-- ============ Footer ============ -->
    <footer class="space-y-2 border-t border-gray-100 bg-gray-50/50 px-3 py-2.5 dark:border-dark-700 dark:bg-dark-900/30">
      <!-- 未登录 + anonymous_llm=false 提示 -->
      <div
        v-if="loginRequired"
        class="flex items-center justify-between gap-2 rounded-lg bg-amber-50 px-3 py-1.5 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
      >
        <span>{{ t('support.chat.login_required') }}</span>
        <button
          type="button"
          class="rounded bg-amber-600 px-2 py-0.5 text-xs font-medium text-white hover:bg-amber-700"
          @click="onClickLogin"
        >
          {{ t('home.login') }}
        </button>
      </div>

      <div class="flex items-end gap-2">
        <textarea
          v-model="inputText"
          rows="1"
          :placeholder="t('support.chat.placeholder')"
          :disabled="loginRequired || store.isLoading"
          class="flex-1 resize-none rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-400 outline-none transition-all focus:border-primary-500 focus:ring-1 focus:ring-primary-500 disabled:opacity-60 dark:border-dark-600 dark:bg-dark-900/50 dark:text-white dark:placeholder-gray-500"
          @keydown="onInputKeydown"
        />
        <button
          v-if="store.isLoading"
          type="button"
          class="rounded-lg bg-red-500 px-3 py-2 text-xs font-medium text-white transition-colors hover:bg-red-600"
          @click="store.abortStream()"
        >
          {{ t('support.chat.stop') }}
        </button>
        <button
          v-else
          type="button"
          :disabled="!canSend"
          class="rounded-lg bg-primary-500 px-3 py-2 text-xs font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          @click="onClickSend"
        >
          {{ t('support.chat.send') }}
        </button>
      </div>

      <div class="flex items-center justify-between gap-2 text-[11px] text-gray-500 dark:text-gray-400">
        <button
          type="button"
          class="rounded text-primary-600 hover:underline disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-400"
          :disabled="!supportTicketEnabled"
          @click="onClickSubmitTicket"
        >
          {{ t('support.chat.submit_ticket') }}
        </button>
        <span class="truncate text-right">{{ t('support.chat.disclaimer') }}</span>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter, useRoute } from 'vue-router'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useSupportChatStore } from '@/stores/supportChat'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { SupportChatFAQ } from '@/api/supportChat'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const store = useSupportChatStore()
const appStore = useAppStore()
const authStore = useAuthStore()

const inputText = ref<string>('')
const bodyEl = ref<HTMLElement | null>(null)

// ============ Computed: configuration shortcuts ============

/**
 * admin 配置的 anonymous LLM 开关。Backend 用此字段决定 anonymous 用户是否
 * 能拿到 LLM。前端复用同一字段做"输入框是否禁用"。
 */
const anonymousLlmEnabled = computed<boolean>(
  () => appStore.cachedPublicSettings?.support_chat_anonymous_llm === true
)

const supportTicketEnabled = computed<boolean>(
  () => appStore.cachedPublicSettings?.support_ticket_enabled === true
)

const loginRequired = computed<boolean>(
  () => !authStore.isAuthenticated && !anonymousLlmEnabled.value
)

const headerIcon = computed<string>(() => {
  const injected = (appStore.cachedPublicSettings as any)?.support_chat_icon
  return typeof injected === 'string' && injected.length > 0 ? injected : '💬'
})

/**
 * headerIcon 既可能是 emoji，也可能是 admin 在 SettingsView 配的图片 URL
 * （http(s) 外链 / 站点相对路径 / 带常见图片扩展名）。命中任意一种都走 <img>，
 * 否则按文本渲染。判定逻辑与 SupportChatBubble.iconIsUrl 完全一致——若以后
 * 需扩展 URL 形式，两处需同步修改。
 */
const headerIconIsUrl = computed<boolean>(() => {
  const v = headerIcon.value
  return (
    /^https?:\/\//i.test(v) ||
    v.startsWith('/') ||
    v.startsWith('./') ||
    /\.(png|jpe?g|svg|webp|gif)(\?|$)/i.test(v)
  )
})

const welcomeText = computed<string>(() => {
  const injected = (appStore.cachedPublicSettings as any)?.support_chat_welcome
  return typeof injected === 'string' && injected.length > 0
    ? injected
    : t('support.chat.welcome')
})

const canSend = computed<boolean>(
  () => inputText.value.trim().length > 0 && !store.isLoading && !loginRequired.value
)

// ============ Markdown rendering（assistant 消息）============

marked.setOptions({ breaks: true, gfm: true })

function renderAssistant(content: string): string {
  if (!content) return ''
  try {
    const html = marked.parse(content) as string
    return DOMPurify.sanitize(html, {
      ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'a', 'code', 'pre', 'ul', 'ol', 'li', 'blockquote', 'h1', 'h2', 'h3', 'h4'],
      ALLOWED_ATTR: ['href', 'target', 'rel', 'class'],
    })
  } catch {
    return content.replace(/[<>&]/g, (c) => ({ '<': '&lt;', '>': '&gt;', '&': '&amp;' }[c]!))
  }
}

// ============ Error message mapping ============

const errorMessage = computed<string>(() => {
  const e = store.error
  if (!e) return ''
  if (e.type === 'rate_limited') {
    if (typeof e.retryAfter === 'number' && e.retryAfter > 0) {
      return t('support.chat.error_rate_limited_with_retry', { seconds: e.retryAfter })
    }
    return t('support.chat.error_rate_limited')
  }
  if (e.type === 'authentication_error') return t('support.chat.error_auth')
  if (e.type === 'config_error') return t('support.chat.error_key_invalid')
  if (e.type === 'network') return t('support.chat.error_network')
  return e.message || t('support.chat.error_unknown')
})

// ============ Bubble layout helpers ============

function bubbleWrapClass(role: string): string {
  return role === 'user' ? 'flex justify-end' : 'flex justify-start'
}
function bubbleClass(role: string, errored?: boolean): string {
  const base = 'max-w-[85%] rounded-2xl px-3 py-2 text-sm break-words'
  if (errored) {
    return `${base} bg-red-50 text-red-700 ring-1 ring-red-200 dark:bg-red-900/20 dark:text-red-200 dark:ring-red-900/40`
  }
  if (role === 'user') {
    return `${base} bg-primary-500 text-white`
  }
  return `${base} bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-gray-100`
}

// ============ Actions ============

function onInputKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (canSend.value) onClickSend()
  }
}

function onClickSend() {
  const text = inputText.value.trim()
  if (!text) return
  store.addUserMessage(text)
  inputText.value = ''
  store.streamAssistantReply()
}

function onClickClear() {
  if (!confirm(t('support.chat.clear_confirm'))) return
  store.clearSession()
}

function onClickFaq(faq: SupportChatFAQ) {
  store.appendFaqAsExchange(faq)
}

function onClickRetry() {
  // 重试 = 重新触发 streamAssistantReply（消息列表保持，仅清错误）
  store.error = null as any
  if (!store.isLoading) {
    // 把最后一条 streaming/errored assistant 消息移除再重试，避免叠加。
    const last = store.messages[store.messages.length - 1]
    if (last && last.role === 'assistant') {
      store.messages.pop()
    }
    store.streamAssistantReply()
  }
}

function onClickLogin() {
  router.push({ path: '/login', query: { redirect: route.fullPath } })
}

function onClickSubmitTicket() {
  if (!supportTicketEnabled.value) return
  // 强制把当前对话写入 localStorage，让工单新建页能读到。
  store.persistImmediately()
  const target = {
    path: '/support/tickets/new',
    query: { from: 'chat', session: 'support_chat_session_v1' },
  }
  if (!authStore.isAuthenticated) {
    const redirect = router.resolve(target).fullPath
    router.push({ path: '/login', query: { redirect } })
    return
  }
  router.push(target)
}

// ============ Effects ============

/** 打开浮窗时懒加载 FAQ；只加载一次。 */
watch(
  () => store.isOpen,
  (open) => {
    if (open) {
      store.loadFaqsLazy()
      // 滚动到底部
      nextTick(() => scrollToBottom())
    }
  }
)

/** 消息追加时自动滚动到底部。 */
watch(
  () => store.messages.length,
  () => nextTick(() => scrollToBottom())
)
/** 流式 chunk 追加：监听最后一条 assistant 内容长度。 */
watch(
  () => {
    const last = store.messages[store.messages.length - 1]
    return last?.streaming ? last.content.length : 0
  },
  () => nextTick(() => scrollToBottom())
)

function scrollToBottom() {
  const el = bodyEl.value
  if (!el) return
  el.scrollTop = el.scrollHeight
}

onMounted(() => {
  // 第一次打开前先恢复历史
  store.loadFromLocalStorage()
})
</script>
