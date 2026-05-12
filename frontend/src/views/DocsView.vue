<template>
  <div class="docs-shell min-h-screen text-slate-950">
    <header class="relative z-20 border-b border-slate-200/70 bg-white/75 backdrop-blur">
      <nav class="mx-auto flex max-w-7xl items-center justify-between px-5 py-4">
        <router-link :to="homePath" class="flex min-w-0 items-center gap-3">
          <span :class="['brand-mark', siteLogo ? 'brand-mark-image' : '']">
            <img
              v-if="siteLogo"
              :src="siteLogo"
              alt=""
              class="brand-logo-image"
            />
            <span v-else>{{ logoLetters }}</span>
          </span>
          <span class="min-w-0">
            <span class="block truncate text-base font-semibold leading-5 text-slate-700">
              {{ siteName }} Docs
            </span>
            <span class="block truncate text-sm leading-5 text-slate-500">
              {{ copy.headerSubtitle }}
            </span>
          </span>
        </router-link>

        <div class="flex items-center gap-3 sm:gap-5">
          <LocaleSwitcher />
          <router-link
            to="/agents"
            class="shrink-0 text-sm font-medium text-slate-500 transition hover:text-slate-900"
          >
            {{ copy.agentsHub }}
          </router-link>
          <router-link
            :to="homePath"
            class="hidden text-sm font-medium text-slate-500 transition hover:text-slate-900 sm:inline"
          >
            {{ copy.mainSite }}
          </router-link>
          <router-link to="/login" class="nav-button">
            {{ copy.signIn }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative z-10 mx-auto grid max-w-7xl gap-8 px-5 py-8 lg:grid-cols-[17rem_1fr]">
      <aside class="hidden lg:block">
        <nav class="sticky top-6 space-y-6 rounded-lg border border-slate-200/80 bg-white/72 p-4 shadow-sm">
          <div>
            <p class="px-2 text-xs font-bold uppercase tracking-[0.14em] text-slate-400">
              {{ copy.contents }}
            </p>
            <div class="mt-3 space-y-1">
              <a
                v-for="item in copy.nav"
                :key="item.href"
                :href="item.href"
                class="block rounded-md px-2 py-2 text-sm font-semibold text-slate-600 transition hover:bg-sky-50 hover:text-sky-800"
              >
                {{ item.label }}
              </a>
            </div>
          </div>

          <div class="rounded-lg bg-slate-950 p-4 text-white">
            <p class="text-xs font-semibold uppercase tracking-[0.14em] text-sky-200">
              Base URL
            </p>
            <p class="mt-2 break-words font-mono text-sm text-slate-200">{{ endpointBase }}</p>
          </div>
        </nav>
      </aside>

      <div class="min-w-0">
        <section id="overview" class="docs-hero">
          <div class="eyebrow-pill">
            <span class="h-2 w-2 rounded-full bg-sky-500"></span>
            {{ copy.eyebrow }}
          </div>
          <h1 class="mt-6 max-w-4xl text-4xl font-black leading-tight tracking-normal text-slate-950 sm:text-6xl">
            {{ copy.titleLead }}
            <span class="hero-highlight">{{ copy.titleHighlight }}</span>
          </h1>
          <p class="mt-6 max-w-3xl text-lg leading-8 text-slate-600">
            {{ copy.description }}
          </p>

          <div class="mt-8 flex flex-wrap gap-3">
            <a href="#quick-start" class="primary-action">{{ copy.quickStartAction }}</a>
            <a href="#api" class="secondary-action">{{ copy.apiAction }}</a>
          </div>

          <dl class="mt-9 grid gap-3 md:grid-cols-3">
            <div
              v-for="fact in copy.facts"
              :key="fact.label"
              class="fact-panel"
            >
              <dt class="text-xs font-bold uppercase tracking-[0.12em] text-slate-400">
                {{ fact.label }}
              </dt>
              <dd class="mt-2 break-words text-sm font-semibold text-slate-800">
                {{ fact.value }}
              </dd>
            </div>
          </dl>
        </section>

        <section class="docs-section">
          <h2>{{ copy.needTitle }}</h2>
          <div class="mt-5 grid gap-4 md:grid-cols-2">
            <a
              v-for="card in copy.needCards"
              :key="card.title"
              :href="card.href"
              class="guide-card"
            >
              <p class="text-sm font-bold text-slate-950">{{ card.title }}</p>
              <p class="mt-2 text-sm leading-6 text-slate-600">{{ card.description }}</p>
            </a>
          </div>
        </section>

        <section id="quick-start" class="docs-section">
          <h2>{{ copy.quickStartTitle }}</h2>
          <p class="section-lead">{{ copy.quickStartIntro }}</p>

          <div class="mt-6 grid gap-4 md:grid-cols-3">
            <article
              v-for="(step, index) in copy.steps"
              :key="step.title"
              class="step-panel"
            >
              <span class="step-number">{{ index + 1 }}</span>
              <h3>{{ step.title }}</h3>
              <p>{{ step.description }}</p>
            </article>
          </div>

          <div class="mt-8 space-y-5">
            <CodeSnippet
              v-for="block in quickStartBlocks"
              :key="block.key"
              :title="block.title"
              :code="block.code"
              :copied="copiedKey === block.key"
              :copy-label="copy.copy"
              :copied-label="copy.copied"
              @copy="copyCode(block.key, block.code)"
            />
          </div>
        </section>

        <section id="clients" class="docs-section">
          <h2>{{ copy.clientsTitle }}</h2>
          <p class="section-lead">{{ copy.clientsIntro }}</p>

          <div class="mt-6 space-y-5">
            <CodeSnippet
              v-for="block in clientBlocks"
              :key="block.key"
              :title="block.title"
              :description="block.description"
              :code="block.code"
              :copied="copiedKey === block.key"
              :copy-label="copy.copy"
              :copied-label="copy.copied"
              @copy="copyCode(block.key, block.code)"
            />
          </div>
        </section>

        <section id="api" class="docs-section">
          <h2>{{ copy.apiTitle }}</h2>
          <p class="section-lead">{{ copy.apiIntro }}</p>

          <div class="mt-6 grid gap-4 md:grid-cols-3">
            <article
              v-for="endpoint in copy.endpoints"
              :key="endpoint.path"
              class="endpoint-panel"
            >
              <p class="font-mono text-xs font-bold text-sky-700">{{ endpoint.method }}</p>
              <h3>{{ endpoint.path }}</h3>
              <p>{{ endpoint.description }}</p>
            </article>
          </div>

          <div class="mt-8 space-y-5">
            <CodeSnippet
              v-for="block in apiBlocks"
              :key="block.key"
              :title="block.title"
              :code="block.code"
              :copied="copiedKey === block.key"
              :copy-label="copy.copy"
              :copied-label="copy.copied"
              @copy="copyCode(block.key, block.code)"
            />
          </div>
        </section>

        <section id="errors" class="docs-section">
          <h2>{{ copy.errorsTitle }}</h2>
          <p class="section-lead">{{ copy.errorsIntro }}</p>
          <div class="mt-6 overflow-hidden rounded-lg border border-slate-200 bg-white/82">
            <div
              v-for="row in copy.errorRows"
              :key="row.code"
              class="grid gap-2 border-b border-slate-100 px-4 py-4 last:border-b-0 md:grid-cols-[7rem_1fr_1.4fr]"
            >
              <p class="font-mono text-sm font-bold text-slate-950">{{ row.code }}</p>
              <p class="text-sm font-semibold text-slate-700">{{ row.meaning }}</p>
              <p class="text-sm leading-6 text-slate-600">{{ row.fix }}</p>
            </div>
          </div>
        </section>

        <section id="faq" class="docs-section">
          <h2>{{ copy.faqTitle }}</h2>
          <div class="mt-6 space-y-3">
            <details
              v-for="item in copy.faq"
              :key="item.question"
              class="faq-item"
            >
              <summary>{{ item.question }}</summary>
              <p>{{ item.answer }}</p>
            </details>
          </div>
        </section>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { resolveHomePathForHost } from '@/utils/homeDomain'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import CodeSnippet from '@/components/common/CodeSnippet.vue'

type NavItem = { href: string; label: string }
type Fact = { label: string; value: string }
type GuideCard = { href: string; title: string; description: string }
type Step = { title: string; description: string }
type Endpoint = { method: string; path: string; description: string }
type ErrorRow = { code: string; meaning: string; fix: string }
type FaqItem = { question: string; answer: string }
type CodeBlock = { key: string; title: string; description?: string; code: string }

const appStore = useAppStore()
const { locale } = useI18n()
const copiedKey = ref('')

const isZh = computed(() => locale.value.toLowerCase().startsWith('zh'))
const homePath = computed(() => resolveHomePathForHost(appStore.cachedPublicSettings ?? window.__APP_CONFIG__))
const siteName = computed(() => {
  const configured = appStore.cachedPublicSettings?.site_name || appStore.siteName
  return configured?.trim() || 'OceanWay AI'
})
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const apiBaseUrl = computed(() => appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || '')

const logoLetters = computed(() => {
  const uppercaseLetters = siteName.value.match(/[A-Z]/g)?.slice(0, 2).join('')
  return uppercaseLetters || siteName.value.slice(0, 2).toUpperCase()
})

const endpointBase = computed(() => {
  const configured = apiBaseUrl.value.trim().replace(/\/+$/, '')
  if (configured) return configured
  const origin = getCurrentOrigin()
  return isLocalOrigin(origin) ? 'https://oceanway.site/v1' : `${origin}/v1`
})

const managedEndpointBase = computed(() => {
  const origin = getCurrentOrigin()
  return isLocalOrigin(origin) ? 'https://oceanwayai.site/v1' : `${origin}/v1`
})

const apiHost = computed(() => endpointBase.value.replace(/\/v1$/i, ''))
const copy = computed(() => isZh.value ? zhCopy.value : enCopy.value)

const quickStartBlocks = computed<CodeBlock[]>(() => [
  {
    key: 'curl',
    title: isZh.value ? '最小 curl 示例' : 'Minimal curl example',
    code: `curl ${endpointBase.value}/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer sk-your-token" \\
  -d '{
    "model": "your-model",
    "messages": [
      {"role": "user", "content": "Hello from ${siteName.value}"}
    ]
  }'`
  },
  {
    key: 'python',
    title: isZh.value ? 'Python SDK 示例' : 'Python SDK example',
    code: `from openai import OpenAI

client = OpenAI(
    api_key="sk-your-token",
    base_url="${endpointBase.value}"
)

response = client.chat.completions.create(
    model="your-model",
    messages=[{"role": "user", "content": "Hello from ${siteName.value}"}]
)

print(response.choices[0].message.content)`
  },
  {
    key: 'node',
    title: isZh.value ? 'Node.js SDK 示例' : 'Node.js SDK example',
    code: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk-your-token",
  baseURL: "${endpointBase.value}",
});

const response = await client.chat.completions.create({
  model: "your-model",
  messages: [{ role: "user", content: "Hello from ${siteName.value}" }],
});

console.log(response.choices[0].message.content);`
  }
])

const clientBlocks = computed<CodeBlock[]>(() => [
  {
    key: 'codex',
    title: 'Codex',
    description: isZh.value
      ? '适合 Codex CLI、桌面端和 Responses 风格 Provider。'
      : 'Works for Codex CLI, desktop, and Responses-style providers.',
    code: `disable_response_storage = true
model = "your-model"
model_provider = "${siteName.value}"
model_reasoning_effort = "high"

[model_providers."${siteName.value}"]
name = "${siteName.value}"
base_url = "${endpointBase.value}"
requires_openai_auth = true
wire_api = "responses"`
  },
  {
    key: 'auth-json',
    title: 'auth.json',
    description: isZh.value
      ? '鉴权文件里保持最小字段，减少读取异常。'
      : 'Keep the auth file minimal to avoid provider parsing issues.',
    code: `{
  "OPENAI_API_KEY": "sk-your-token"
}`
  },
  {
    key: 'claude-code',
    title: 'Claude Code',
    description: isZh.value
      ? '如果工具走 Anthropic 风格变量，Host 通常使用站点根域。'
      : 'For Anthropic-style variables, the host usually uses the site root.',
    code: `{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-your-token",
    "ANTHROPIC_BASE_URL": "${apiHost.value}",
    "ANTHROPIC_MODEL": "your-model",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "your-model",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "your-model",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "your-model"
  }
}`
  },
  {
    key: 'opencode',
    title: 'OpenCode',
    description: isZh.value
      ? '推荐用 OpenAI Compatible Provider，Provider ID 和模型前缀保持一致。'
      : 'Use an OpenAI-compatible provider and keep provider IDs consistent.',
    code: `{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "${siteName.value}": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "${siteName.value}",
      "options": {
        "baseURL": "${endpointBase.value}"
      },
      "models": {
        "your-model": {
          "model": "your-model",
          "name": "your-model"
        }
      }
    }
  },
  "model": "${siteName.value}/your-model"
}`
  }
])

const apiBlocks = computed<CodeBlock[]>(() => [
  {
    key: 'models',
    title: 'GET /v1/models',
    code: `curl ${endpointBase.value}/models \\
  -H "Authorization: Bearer sk-your-token"`
  },
  {
    key: 'responses',
    title: 'POST /v1/responses',
    code: `curl ${endpointBase.value}/responses \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer sk-your-token" \\
  -d '{
    "model": "your-model",
    "input": "Explain how to connect to ${siteName.value}.",
    "reasoning": { "effort": "medium" },
    "max_output_tokens": 800
  }'`
  },
  {
    key: 'managed',
    title: isZh.value ? '托管客户 Base URL' : 'Managed customer Base URL',
    code: `${managedEndpointBase.value}

Authorization: Bearer sk-managed-customer-key`
  }
])

const zhCopy = computed(() => ({
  headerSubtitle: 'OpenAI 兼容 API 接入文档',
  mainSite: '主站',
  agentsHub: 'Agents Hub',
  signIn: '登录',
  contents: '目录',
  eyebrow: `${siteName.value} Docs`,
  titleLead: '面向开发工具的',
  titleHighlight: 'AI 接入文档。',
  description:
    `${siteName.value} 提供 OpenAI 兼容 API 网关，适配 Codex、Claude Code、OpenCode 和通用 SDK。用统一的 API Key 与 Base URL 接入可用模型。`,
  quickStartAction: '快速开始',
  apiAction: '查看 API',
  facts: [
    { label: '入口', value: 'OpenAI Compatible' },
    { label: '默认 Base URL', value: endpointBase.value },
    { label: '文档路径', value: '/docs' }
  ] satisfies Fact[],
  nav: [
    { href: '#overview', label: '首页' },
    { href: '#quick-start', label: '快速接入' },
    { href: '#clients', label: '客户端配置' },
    { href: '#api', label: 'API 文档' },
    { href: '#errors', label: '错误排查' },
    { href: '#faq', label: 'Q&A' }
  ] satisfies NavItem[],
  needTitle: '你可能最先需要的内容',
  needCards: [
    { href: '#clients', title: 'Codex 配置', description: '优先推荐，直接配置 Base URL、Provider 和令牌即可。' },
    { href: '#api', title: 'Chat Completions', description: '兼容最广的接口，适合脚本、SDK 与通用 API 调用。' },
    { href: '#api', title: 'Responses', description: '适合新版 OpenAI 客户端、Codex 和更统一的响应结构。' },
    { href: '#errors', title: '常见问题', description: '接入失败、模型不可用或额度异常时，从这里开始排查。' }
  ] satisfies GuideCard[],
  quickStartTitle: '快速接入',
  quickStartIntro: '最快只需要一个 Base URL 和一个 API Key。先验证模型列表，再接入客户端。',
  steps: [
    { title: '登录控制台', description: '确认你已经有可用账户，并能创建可用 API Key。' },
    { title: '获取 API Key', description: '创建以 sk- 开头的令牌，并妥善保存。' },
    { title: '设置 Base URL', description: `将客户端指向 ${endpointBase.value}，不要漏掉 /v1。` }
  ] satisfies Step[],
  clientsTitle: '客户端配置',
  clientsIntro:
    '不同工具对 Base URL 的字段名不完全一致。OpenAI 兼容 SDK 通常填写 /v1 地址，Anthropic 风格工具通常填写根域。',
  apiTitle: 'API 文档',
  apiIntro: '最常用的是模型列表、Chat Completions 和 Responses。建议先测 /models，再开始正式调用。',
  endpoints: [
    { method: 'GET', path: '/v1/models', description: '查看当前令牌可见模型。' },
    { method: 'POST', path: '/v1/chat/completions', description: '兼容传统 OpenAI 对话接口。' },
    { method: 'POST', path: '/v1/responses', description: '适合新版客户端和推理任务。' }
  ] satisfies Endpoint[],
  errorsTitle: '错误排查',
  errorsIntro: '接入失败时，先看状态码和 error.code，再区分是客户端配置、令牌权限还是服务侧容量问题。',
  errorRows: [
    { code: '400', meaning: '请求参数错误', fix: '检查 JSON 结构、字段名和模型名。' },
    { code: '401', meaning: '认证失败', fix: '确认 Authorization 使用的是当前站点生成的 API Key。' },
    { code: '403', meaning: '权限不足', fix: '检查令牌是否有分组、订阅或模型权限。' },
    { code: '404', meaning: '路径不存在', fix: '确认 Base URL 包含 /v1，接口路径没有重复拼接。' },
    { code: '429', meaning: '频率超限', fix: '降低请求频率，或检查 Key / 用户 / 分组限流。' },
    { code: '503', meaning: '服务不可用', fix: '优先确认分组内是否有可调度账号和模型容量。' }
  ] satisfies ErrorRow[],
  faqTitle: 'Q&A',
  faq: [
    { question: 'Base URL 应该填裸域名还是 /v1？', answer: 'OpenAI 兼容 SDK 通常填写带 /v1 的地址。如果工具已经自动拼接 /v1，就不要重复追加。' },
    { question: '模型名应该怎么选？', answer: '先请求 GET /v1/models，把返回结果中的模型名写入客户端，避免凭记忆手写。' },
    { question: '内部客户和公共用户的文档是否不同？', answer: '调用协议一致，内部客户通常使用管理员交付的托管 API Key 和对应 Base URL。' }
  ] satisfies FaqItem[],
  copy: '复制',
  copied: '已复制'
}))

const enCopy = computed(() => ({
  headerSubtitle: 'OpenAI-compatible API docs',
  mainSite: 'Main site',
  agentsHub: 'Agents Hub',
  signIn: 'Sign in',
  contents: 'Contents',
  eyebrow: `${siteName.value} Docs`,
  titleLead: 'AI integration docs for',
  titleHighlight: 'developer tools.',
  description:
    `${siteName.value} provides an OpenAI-compatible API gateway for Codex, Claude Code, OpenCode, and common SDKs. Use one API key and one Base URL to reach available models.`,
  quickStartAction: 'Quick start',
  apiAction: 'View API docs',
  facts: [
    { label: 'Entry', value: 'OpenAI Compatible' },
    { label: 'Default Base URL', value: endpointBase.value },
    { label: 'Docs path', value: '/docs' }
  ] satisfies Fact[],
  nav: [
    { href: '#overview', label: 'Home' },
    { href: '#quick-start', label: 'Quick start' },
    { href: '#clients', label: 'Client config' },
    { href: '#api', label: 'API docs' },
    { href: '#errors', label: 'Troubleshooting' },
    { href: '#faq', label: 'Q&A' }
  ] satisfies NavItem[],
  needTitle: 'What you may need first',
  needCards: [
    { href: '#clients', title: 'Codex config', description: 'Recommended first path: configure Base URL, provider, and token.' },
    { href: '#api', title: 'Chat Completions', description: 'The widest compatible API for scripts, SDKs, and general calls.' },
    { href: '#api', title: 'Responses', description: 'Better for newer OpenAI clients, Codex, and unified response shapes.' },
    { href: '#errors', title: 'Common issues', description: 'Start here for failed requests, unavailable models, or quota issues.' }
  ] satisfies GuideCard[],
  quickStartTitle: 'Quick start',
  quickStartIntro: 'You only need a Base URL and an API key. Check models first, then connect your client.',
  steps: [
    { title: 'Sign in', description: 'Make sure your account can create a usable API key.' },
    { title: 'Create an API key', description: 'Create a token starting with sk- and store it safely.' },
    { title: 'Set Base URL', description: `Point your client to ${endpointBase.value}. Do not omit /v1.` }
  ] satisfies Step[],
  clientsTitle: 'Client config',
  clientsIntro:
    'Tools name the Base URL field differently. OpenAI-compatible SDKs usually use the /v1 URL, while Anthropic-style tools often use the site root.',
  apiTitle: 'API docs',
  apiIntro: 'The most common APIs are model listing, Chat Completions, and Responses. Test /models first.',
  endpoints: [
    { method: 'GET', path: '/v1/models', description: 'List models visible to the current token.' },
    { method: 'POST', path: '/v1/chat/completions', description: 'Traditional OpenAI-compatible chat API.' },
    { method: 'POST', path: '/v1/responses', description: 'Useful for newer clients and reasoning tasks.' }
  ] satisfies Endpoint[],
  errorsTitle: 'Troubleshooting',
  errorsIntro: 'When calls fail, check the status code and error.code before separating client, token, and capacity issues.',
  errorRows: [
    { code: '400', meaning: 'Invalid request', fix: 'Check JSON shape, field names, and model name.' },
    { code: '401', meaning: 'Auth failed', fix: 'Use an API key generated by this site.' },
    { code: '403', meaning: 'No permission', fix: 'Check group, subscription, or model permissions.' },
    { code: '404', meaning: 'Wrong path', fix: 'Confirm the Base URL includes /v1 and paths are not duplicated.' },
    { code: '429', meaning: 'Rate limited', fix: 'Lower request rate or inspect key, user, and group limits.' },
    { code: '503', meaning: 'Unavailable', fix: 'Check whether the group has schedulable accounts and model capacity.' }
  ] satisfies ErrorRow[],
  faqTitle: 'Q&A',
  faq: [
    { question: 'Should Base URL include /v1?', answer: 'OpenAI-compatible SDKs usually need the /v1 URL. If your tool appends /v1 automatically, do not add it twice.' },
    { question: 'How should I choose a model name?', answer: 'Call GET /v1/models first and copy a returned model name into the client.' },
    { question: 'Are managed customers different from retail users?', answer: 'The calling protocol is the same. Managed customers usually receive an admin-issued API key and the matching Base URL.' }
  ] satisfies FaqItem[],
  copy: 'Copy',
  copied: 'Copied'
}))

async function copyCode(key: string, code: string) {
  try {
    await navigator.clipboard.writeText(code)
    copiedKey.value = key
    window.setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = ''
    }, 1600)
  } catch {
    copiedKey.value = ''
  }
}

function getCurrentOrigin() {
  if (typeof window === 'undefined') return ''
  return window.location.origin.replace(/\/+$/, '')
}

function isLocalOrigin(origin: string) {
  return /^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(?::\d+)?$/i.test(origin)
}

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.docs-shell {
  position: relative;
  background:
    linear-gradient(180deg, rgba(242, 248, 255, 0.96) 0%, rgba(250, 252, 255, 0.94) 38%, rgba(232, 246, 255, 0.98) 100%);
}

.docs-shell::before {
  position: fixed;
  inset: 0;
  pointer-events: none;
  content: '';
  background:
    linear-gradient(110deg, rgba(255, 255, 255, 0.86) 0%, rgba(255, 255, 255, 0) 44%),
    linear-gradient(180deg, rgba(196, 222, 250, 0) 55%, rgba(196, 222, 250, 0.38) 100%);
}

.brand-mark {
  display: inline-flex;
  width: 2.375rem;
  height: 2.375rem;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: linear-gradient(135deg, #002080 0%, #001040 78%);
  color: rgba(255, 255, 255, 0.78);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0;
  box-shadow: 0 10px 26px rgba(15, 23, 42, 0.18);
}

.brand-mark-image {
  overflow: hidden;
  border-radius: 0.65rem;
  background: #001040;
  box-shadow: none;
}

.brand-logo-image {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.nav-button,
.primary-action,
.secondary-action {
  display: inline-flex;
  min-height: 2.5rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  font-weight: 700;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    color 0.16s ease,
    transform 0.16s ease;
}

.nav-button,
.secondary-action {
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(255, 255, 255, 0.82);
  padding: 0.5rem 1rem;
  color: #111827;
}

.primary-action {
  background: #001040;
  padding: 0.72rem 1.05rem;
  color: white;
}

.primary-action:hover,
.secondary-action:hover,
.nav-button:hover {
  transform: translateY(-1px);
}

.docs-hero,
.docs-section {
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.72);
  box-shadow: 0 18px 38px rgba(15, 23, 42, 0.05);
}

.docs-hero {
  padding: 2rem;
}

.docs-section {
  margin-top: 1rem;
  padding: 1.5rem;
}

.docs-section h2 {
  color: #0f172a;
  font-size: 1.5rem;
  font-weight: 850;
  letter-spacing: 0;
}

.section-lead {
  margin-top: 0.75rem;
  max-width: 46rem;
  color: #475569;
  font-size: 0.95rem;
  line-height: 1.75;
}

.eyebrow-pill {
  display: inline-flex;
  min-height: 2.25rem;
  align-items: center;
  gap: 0.6rem;
  border: 1px solid rgba(0, 160, 255, 0.24);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.7);
  padding: 0.45rem 0.85rem;
  color: #005db8;
  font-size: 0.875rem;
  font-weight: 800;
}

.hero-highlight {
  color: #006fd6;
}

.fact-panel,
.guide-card,
.step-panel,
.endpoint-panel,
.faq-item {
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.78);
  padding: 1rem;
}

.guide-card {
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    transform 0.16s ease;
}

.guide-card:hover {
  border-color: rgba(0, 160, 255, 0.42);
  background: rgba(232, 246, 255, 0.76);
  transform: translateY(-1px);
}

.step-number {
  display: inline-flex;
  width: 2rem;
  height: 2rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: #001040;
  color: white;
  font-size: 0.875rem;
  font-weight: 800;
}

.step-panel h3,
.endpoint-panel h3 {
  margin-top: 0.9rem;
  color: #0f172a;
  font-size: 1rem;
  font-weight: 800;
}

.step-panel p,
.endpoint-panel p {
  margin-top: 0.5rem;
  color: #475569;
  font-size: 0.875rem;
  line-height: 1.65;
}

:deep(.code-panel) {
  overflow: hidden;
  border: 1px solid rgba(203, 213, 225, 0.8);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.9);
}

:deep(.code-panel-header) {
  display: flex;
  min-height: 3.2rem;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.8rem 1rem;
}

:deep(.code-panel-title) {
  color: #0f172a;
  font-size: 0.9rem;
  font-weight: 800;
}

:deep(.code-panel-desc) {
  margin-top: 0.2rem;
  color: #64748b;
  font-size: 0.8rem;
  line-height: 1.5;
}

:deep(.copy-button) {
  flex: 0 0 auto;
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-radius: 0.5rem;
  background: white;
  padding: 0.42rem 0.7rem;
  color: #334155;
  font-size: 0.75rem;
  font-weight: 800;
}

:deep(.code-pre) {
  overflow-x: auto;
  background: #001040;
  padding: 1.1rem;
  color: #d1d5db;
  font-family:
    ui-monospace,
    SFMono-Regular,
    Menlo,
    Monaco,
    Consolas,
    'Liberation Mono',
    'Courier New',
    monospace;
  font-size: 0.8125rem;
  line-height: 1.75;
}

.faq-item summary {
  cursor: pointer;
  color: #0f172a;
  font-size: 0.95rem;
  font-weight: 800;
}

.faq-item p {
  margin-top: 0.75rem;
  color: #475569;
  font-size: 0.9rem;
  line-height: 1.7;
}

@media (max-width: 640px) {
  .docs-hero,
  .docs-section {
    padding: 1.1rem;
  }
}
</style>
