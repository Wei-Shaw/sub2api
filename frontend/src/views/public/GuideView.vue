<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 bg-white/95 dark:border-dark-800 dark:bg-dark-900/95">
      <nav class="mx-auto flex max-w-6xl items-center justify-between px-4 py-4 sm:px-6 lg:px-8">
        <router-link to="/home" class="flex min-w-0 items-center gap-3">
          <span class="flex h-10 w-10 shrink-0 overflow-hidden rounded-lg bg-white shadow-sm">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <span class="truncate text-lg font-semibold">{{ siteName }}</span>
        </router-link>

        <div class="flex items-center gap-2">
          <router-link
            to="/home"
            class="inline-flex h-10 items-center gap-2 rounded-lg px-3 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            <Icon name="home" size="sm" />
            <span class="hidden sm:inline">首页</span>
          </router-link>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex h-10 items-center gap-2 rounded-lg bg-gray-900 px-4 text-sm font-medium text-white transition-colors hover:bg-gray-800 dark:bg-primary-600 dark:hover:bg-primary-500"
          >
            <Icon :name="isAuthenticated ? 'grid' : 'login'" size="sm" />
            <span>{{ isAuthenticated ? '控制台' : '登录' }}</span>
          </router-link>
        </div>
      </nav>
    </header>

    <main class="mx-auto max-w-6xl px-4 py-8 sm:px-6 lg:px-8">
      <section class="grid gap-6 lg:grid-cols-[1.15fr_0.85fr] lg:items-start">
        <div class="space-y-6">
          <div>
            <p class="mb-3 inline-flex items-center gap-2 rounded-lg bg-primary-50 px-3 py-1 text-sm font-medium text-primary-700 dark:bg-primary-900/25 dark:text-primary-300">
              <Icon name="lightbulb" size="sm" />
              TTAPI 新手教程
            </p>
            <h1 class="max-w-3xl text-3xl font-bold leading-tight text-gray-950 dark:text-white sm:text-4xl">
              3 分钟跑通第一次 API 调用
            </h1>
            <p class="mt-4 max-w-3xl text-base leading-7 text-gray-600 dark:text-dark-300">
              按下面顺序做：注册账号，兑换或充值额度，创建 API Key，先用站内在线聊天发一句短消息测通；
              确认没问题后，再去文档里选择 Codex、Cherry Studio 或其他客户端的配置教程。
            </p>
          </div>

          <div class="flex flex-col gap-3 sm:flex-row">
            <router-link
              :to="primaryAction.to"
              class="inline-flex h-11 items-center justify-center gap-2 rounded-lg bg-primary-600 px-5 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-primary-700"
            >
              <Icon :name="primaryAction.icon" size="sm" />
              {{ primaryAction.label }}
            </router-link>
            <router-link
              :to="authTarget('/chat')"
              class="inline-flex h-11 items-center justify-center gap-2 rounded-lg border border-primary-200 bg-white px-5 text-sm font-semibold text-primary-700 transition-colors hover:bg-primary-50 dark:border-primary-800 dark:bg-dark-900 dark:text-primary-300 dark:hover:bg-primary-900/20"
            >
              <Icon name="chat" size="sm" />
              在线聊天测试
            </router-link>
          </div>
        </div>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-4 flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-medium text-gray-500 dark:text-dark-400">统一 Base URL</p>
              <p class="mt-1 text-sm text-gray-600 dark:text-dark-300">所有 OpenAI 兼容客户端都填这个。</p>
            </div>
            <button
              type="button"
              class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg bg-gray-100 px-3 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700"
              @click="copyText('baseUrl', baseUrl)"
            >
              <Icon :name="copiedKey === 'baseUrl' ? 'check' : 'copy'" size="sm" />
              {{ copiedKey === 'baseUrl' ? '已复制' : '复制' }}
            </button>
          </div>
          <code class="block overflow-x-auto rounded-lg bg-gray-950 px-4 py-3 text-sm text-primary-200">{{ baseUrl }}</code>
          <div class="mt-4 rounded-lg bg-amber-50 p-4 text-sm leading-6 text-amber-900 dark:bg-amber-900/20 dark:text-amber-100">
            API Key 只在登录后显示。请不要把 Key 发给别人，也不要截图发到公开平台。
          </div>
        </section>
      </section>

      <section class="mt-8 grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
        <article
          v-for="step in quickSteps"
          :key="step.title"
          class="flex flex-col rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900"
        >
          <div class="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/25 dark:text-primary-300">
            <Icon :name="step.icon" size="md" />
          </div>
          <p class="text-xs font-semibold uppercase text-gray-400">{{ step.kicker }}</p>
          <h2 class="mt-1 text-base font-semibold">{{ step.title }}</h2>
          <p class="mt-2 flex-1 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ step.body }}</p>
          <component
            :is="step.external ? 'a' : 'router-link'"
            :to="step.external ? undefined : step.to"
            :href="step.external ? step.to : undefined"
            :target="step.external ? '_blank' : undefined"
            :rel="step.external ? 'noopener noreferrer' : undefined"
            class="mt-4 inline-flex h-9 items-center justify-center gap-1.5 rounded-lg bg-gray-900 px-3 text-sm font-semibold text-white transition-colors hover:bg-gray-800 dark:bg-primary-600 dark:hover:bg-primary-500"
          >
            <span>{{ step.actionLabel }}</span>
            <Icon :name="step.external ? 'externalLink' : 'arrowRight'" size="xs" :stroke-width="2" />
          </component>
        </article>
      </section>

      <section id="client-config" class="mt-8 scroll-mt-24 grid gap-6 lg:grid-cols-[0.95fr_1.05fr]">
        <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-5 flex items-center gap-2">
            <Icon name="terminal" size="md" class="text-primary-600 dark:text-primary-300" />
            <h2 class="text-lg font-semibold">Codex / CLI 配置</h2>
          </div>
          <p class="mb-4 text-sm leading-6 text-gray-600 dark:text-dark-300">
            客户端里选择 OpenAI 兼容接口，Base URL 填 TTAPI，API Key 填你在控制台创建的 Key。
          </p>
          <div class="mb-3 flex items-center justify-between gap-3">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">示例配置</span>
            <button
              type="button"
              class="inline-flex h-8 items-center gap-1.5 rounded-lg bg-gray-100 px-3 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700"
              @click="copyText('codex', codexSnippet)"
            >
              <Icon :name="copiedKey === 'codex' ? 'check' : 'copy'" size="xs" />
              {{ copiedKey === 'codex' ? '已复制' : '复制' }}
            </button>
          </div>
          <pre class="overflow-x-auto rounded-lg bg-gray-950 p-4 text-xs leading-6 text-gray-100"><code>{{ codexSnippet }}</code></pre>
        </div>

        <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-5 flex items-center gap-2">
            <Icon name="grid" size="md" class="text-primary-600 dark:text-primary-300" />
            <h2 class="text-lg font-semibold">常见客户端怎么填</h2>
          </div>
          <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-800">
            <div class="grid grid-cols-[0.9fr_1fr_1fr] bg-gray-50 text-xs font-semibold text-gray-500 dark:bg-dark-800 dark:text-dark-300">
              <div class="px-3 py-2">客户端</div>
              <div class="px-3 py-2">接口类型</div>
              <div class="px-3 py-2">模型示例</div>
            </div>
            <div
              v-for="client in clients"
              :key="client.name"
              class="grid grid-cols-[0.9fr_1fr_1fr] border-t border-gray-200 text-sm dark:border-dark-800"
            >
              <div class="px-3 py-3 font-medium">{{ client.name }}</div>
              <div class="px-3 py-3 text-gray-600 dark:text-dark-300">{{ client.provider }}</div>
              <div class="px-3 py-3 font-mono text-xs text-gray-700 dark:text-dark-200">{{ client.model }}</div>
            </div>
          </div>
          <p class="mt-4 text-sm leading-6 text-gray-600 dark:text-dark-300">
            如果客户端有 “OpenAI API Base” 或 “Custom Endpoint”，就填
            <span class="font-mono text-gray-900 dark:text-white">{{ baseUrl }}</span>。
          </p>
        </div>
      </section>

      <section class="mt-8 grid gap-6 lg:grid-cols-2">
        <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-5 flex items-center gap-2">
            <Icon name="clipboard" size="md" class="text-primary-600 dark:text-primary-300" />
            <h2 class="text-lg font-semibold">不消耗额度的连通性检查</h2>
          </div>
          <p class="mb-4 text-sm leading-6 text-gray-600 dark:text-dark-300">
            先请求模型列表，确认域名、Key 和客户端网络都正常，再开始正式对话。
          </p>
          <div class="mb-3 flex items-center justify-between gap-3">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">curl 示例</span>
            <button
              type="button"
              class="inline-flex h-8 items-center gap-1.5 rounded-lg bg-gray-100 px-3 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700"
              @click="copyText('curl', curlSnippet)"
            >
              <Icon :name="copiedKey === 'curl' ? 'check' : 'copy'" size="xs" />
              {{ copiedKey === 'curl' ? '已复制' : '复制' }}
            </button>
          </div>
          <pre class="overflow-x-auto rounded-lg bg-gray-950 p-4 text-xs leading-6 text-gray-100"><code>{{ curlSnippet }}</code></pre>
        </div>

        <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-5 flex items-center gap-2">
            <Icon name="questionCircle" size="md" class="text-primary-600 dark:text-primary-300" />
            <h2 class="text-lg font-semibold">遇到报错先看这里</h2>
          </div>
          <div class="space-y-3">
            <div
              v-for="item in troubleshooting"
              :key="item.title"
              class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800"
            >
              <h3 class="text-sm font-semibold">{{ item.title }}</h3>
              <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ item.body }}</p>
            </div>
          </div>
        </div>
      </section>

      <section class="mt-8 rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
        <div class="grid gap-5 md:grid-cols-[0.8fr_1.2fr] md:items-center">
          <div>
            <div class="mb-3 flex h-10 w-10 items-center justify-center rounded-lg bg-green-50 text-green-700 dark:bg-green-900/25 dark:text-green-300">
              <Icon name="shield" size="md" />
            </div>
            <h2 class="text-lg font-semibold">使用前的小约定</h2>
            <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">
              Key 只给自己用；异常失败、重复扣费或长时间不可用，请带截图和时间联系处理。
            </p>
          </div>
          <div class="grid gap-3 sm:grid-cols-3">
            <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
              <p class="text-sm font-semibold">失败扣费</p>
              <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">通常只按成功返回的上游用量计费。</p>
            </div>
            <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
              <p class="text-sm font-semibold">余额不足</p>
              <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">先兑换或充值，再重试请求。</p>
            </div>
            <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
              <p class="text-sm font-semibold">上下文太长</p>
              <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">compact 失败时重试一次，仍失败就新开会话。</p>
            </div>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAuthStore, useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'

const baseUrl = 'https://ttapi123.xyz/v1'

const authStore = useAuthStore()
const appStore = useAppStore()

const copiedKey = ref('')
let copyTimer: ReturnType<typeof setTimeout> | undefined

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'TTAPI')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const clientDocsUrl = computed(() => docUrl.value || '/guide#client-config')
const authTarget = (path: string) => (
  isAuthenticated.value
    ? path
    : { path: '/login', query: { redirect: path } }
)
const primaryAction = computed(() => (
  isAuthenticated.value
    ? { to: '/keys', label: '去创建 API Key', icon: 'key' as const }
    : { to: '/register', label: '注册并领取额度', icon: 'userPlus' as const }
))

const quickSteps = computed(() => [
  {
    kicker: 'Step 1',
    title: '注册或登录',
    body: '用邮箱注册账号。首次使用建议先领取少量试用额度，确认客户端能正常请求。',
    icon: 'userPlus',
    to: isAuthenticated.value ? dashboardPath.value : '/register',
    actionLabel: isAuthenticated.value ? '去控制台' : '去注册',
    external: false,
  },
  {
    kicker: 'Step 2',
    title: '兑换或充值额度',
    body: '如果你拿到了兑换码，先去兑换额度。没有兑换码时，可以用控制台顶部的充值入口购买兑换码。',
    icon: 'gift',
    to: authTarget('/redeem'),
    actionLabel: '去兑换额度',
    external: false,
  },
  {
    kicker: 'Step 3',
    title: '创建 API Key',
    body: '进入控制台的 API Keys 页面，新建一个 Key，并保存好完整内容。之后页面通常不会再次完整展示。',
    icon: 'key',
    to: authTarget('/keys'),
    actionLabel: '去创建 Key',
    external: false,
  },
  {
    kicker: 'Step 4',
    title: '在线聊天测通',
    body: '打开站内在线聊天，选择 API Key 和模型，发一句短消息。能正常返回就说明账号、余额和上游链路都通。',
    icon: 'chat',
    to: authTarget('/chat'),
    actionLabel: '去测试',
    external: false,
  },
  {
    kicker: 'Step 5',
    title: '查看客户端文档',
    body: '不同客户端的入口和字段名不一样，详细配置教程统一放在文档里，按你使用的客户端照着填。',
    icon: 'document',
    to: clientDocsUrl.value,
    actionLabel: '去看文档',
    external: Boolean(docUrl.value),
  },
] as const)

const clients = [
  { name: 'Codex', provider: 'OpenAI compatible', model: 'gpt-5.5' },
  { name: 'Cherry Studio', provider: 'OpenAI', model: 'gpt-5.4-mini' },
  { name: 'Chatbox', provider: 'OpenAI API', model: 'gpt-5.4' },
  { name: 'Open WebUI', provider: 'OpenAI compatible', model: 'gpt-5.5' },
] as const

const troubleshooting = [
  {
    title: '401 / API key required',
    body: 'Key 没填、填错，或者复制时多了空格。重新复制完整 Key，确认请求头是 Authorization: Bearer sk-...',
  },
  {
    title: '402 / 余额不足',
    body: '账号额度不够了。充值、兑换额度后再请求；如果刚充值仍失败，刷新页面后再试。',
  },
  {
    title: '429 / 账号繁忙',
    body: '当前上游账号并发满了，等几十秒重试，或换一个更轻的模型。长上下文任务更容易占住并发。',
  },
  {
    title: '502 / compact 失败',
    body: '常见原因是上游链路中断、账号限额或上下文太长。先重试一次，连续失败就新开会话或联系处理。',
  },
] as const

const codexSnippet = `model = "gpt-5.5"
review_model = "gpt-5.5"
disable_response_storage = true
network_access = "enabled"

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
env_key = "OPENAI_API_KEY"`

const curlSnippet = `curl ${baseUrl}/models \\
  -H "Authorization: Bearer sk-你的APIKey"`

async function copyText(key: string, text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedKey.value = key
    if (copyTimer) {
      clearTimeout(copyTimer)
    }
    copyTimer = setTimeout(() => {
      copiedKey.value = ''
    }, 1600)
  } catch {
    copiedKey.value = ''
  }
}

onMounted(() => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>
