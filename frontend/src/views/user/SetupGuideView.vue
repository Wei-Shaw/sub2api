<template>
  <div class="guide-page min-h-screen bg-[#f6f2e9] text-[#17130f] dark:bg-dark-950 dark:text-white">
    <header class="sticky top-0 z-30 border-b border-[#e5ded2]/80 bg-[#f6f2e9]/90 backdrop-blur-xl dark:border-dark-800 dark:bg-dark-950/85">
      <div class="mx-auto flex max-w-7xl items-center justify-between px-5 py-4 sm:px-8">
        <router-link to="/home" class="flex items-center gap-3">
          <div class="flex h-9 w-9 items-center justify-center overflow-hidden rounded-xl border border-[#ded4c5] bg-[#fffaf2] dark:border-dark-700 dark:bg-dark-900">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <div>
            <p class="text-sm font-semibold leading-none text-[#17130f] dark:text-white">{{ siteName }}</p>
            <p class="mt-1 text-[11px] uppercase tracking-[0.22em] text-[#8a8173] dark:text-dark-300">AI Gateway Docs</p>
          </div>
        </router-link>
        <div class="flex items-center gap-2">
          <LocaleSwitcher />
          <router-link to="/login" class="rounded-full bg-[#17130f] px-4 py-2 text-sm font-medium text-white transition hover:-translate-y-0.5 hover:bg-black dark:bg-white dark:text-dark-950">
            登录使用
          </router-link>
        </div>
      </div>
    </header>

    <main class="mx-auto grid max-w-7xl grid-cols-1 gap-8 px-5 py-8 sm:px-8 lg:grid-cols-[240px_minmax(0,1fr)] xl:grid-cols-[240px_minmax(0,1fr)_180px]">
      <aside class="hidden lg:block">
        <nav class="sticky top-28 space-y-6 rounded-[24px] border border-[#e7ddcf] bg-[#fffaf2]/70 p-4 dark:border-dark-800 dark:bg-dark-900/70">
          <div>
            <p class="mb-3 text-[11px] font-semibold uppercase tracking-[0.22em] text-[#9a8f80]">开始之前</p>
            <a href="#overview" :class="navClass('overview')">总览</a>
            <a href="#pick" :class="navClass('pick')">怎么选工具</a>
          </div>
          <div>
            <p class="mb-3 text-[11px] font-semibold uppercase tracking-[0.22em] text-[#9a8f80]">配置教程</p>
            <a v-for="tool in tools" :key="tool.id" :href="'#' + tool.id" :class="navClass(tool.id)">
              <span>{{ tool.name }}</span>
              <span v-if="tool.badge" class="rounded-full bg-[#9fe870] px-1.5 py-0.5 text-[10px] font-semibold text-[#163300]">{{ tool.badge }}</span>
            </a>
          </div>
          <div>
            <p class="mb-3 text-[11px] font-semibold uppercase tracking-[0.22em] text-[#9a8f80]">排查</p>
            <a href="#faq" :class="navClass('faq')">常见问题</a>
            <a href="#errors" :class="navClass('errors')">错误码</a>
          </div>
        </nav>
      </aside>

      <article class="min-w-0 space-y-8">
        <section id="overview" class="relative overflow-hidden rounded-[32px] border border-[#e7ddcf] bg-[#fffaf2] p-7 sm:p-10 dark:border-dark-800 dark:bg-dark-900">
          <div class="absolute right-[-120px] top-[-160px] h-80 w-80 rounded-full bg-[#9fe870]/50 blur-3xl"></div>
          <div class="absolute bottom-[-140px] left-[10%] h-72 w-72 rounded-full bg-[#c96442]/15 blur-3xl"></div>
          <div class="relative max-w-3xl">
            <div class="mb-5 inline-flex items-center gap-2 rounded-full border border-[#ded4c5] bg-white/70 px-3 py-1.5 text-xs font-medium text-[#5f574c] dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200">
              <span class="h-2 w-2 rounded-full bg-[#9fe870]"></span>
              选对工具，复制配置，马上能跑
            </div>
            <h1 class="guide-title max-w-2xl text-4xl font-semibold tracking-[-0.04em] text-[#17130f] sm:text-6xl dark:text-white">
              卡卡AI 接入指南
            </h1>
            <p class="mt-5 max-w-2xl text-base leading-8 text-[#6f675b] sm:text-lg dark:text-dark-300">
              这里不堆概念，只告诉你三件事：用哪个地址、Key 放哪里、怎么验证。OpenAI 兼容工具统一走 <code>https://api.kaiaigo.com/v1</code>；Claude Code 单独走 <code>https://api.kaiaigo.com</code>。
            </p>
          </div>
          <div class="relative mt-8 grid gap-3 md:grid-cols-3">
            <div v-for="item in heroFacts" :key="item.label" class="rounded-2xl border border-[#eadfce] bg-white/75 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-[11px] uppercase tracking-[0.18em] text-[#9a8f80]">{{ item.label }}</p>
              <p class="mt-2 font-mono text-sm font-medium text-[#17130f] dark:text-white">{{ item.value }}</p>
              <p class="mt-2 text-xs leading-5 text-[#7a7165] dark:text-dark-300">{{ item.note }}</p>
            </div>
          </div>
        </section>

        <section id="pick" class="rounded-[28px] border border-[#e7ddcf] bg-white/70 p-6 dark:border-dark-800 dark:bg-dark-900/70">
          <div class="mb-5 flex items-end justify-between gap-4">
            <div>
              <p class="text-xs font-semibold uppercase tracking-[0.22em] text-[#c96442]">Pick your path</p>
              <h2 class="mt-2 text-2xl font-semibold tracking-[-0.03em] text-[#17130f] dark:text-white">不知道选哪个？看这里</h2>
            </div>
          </div>
          <div class="grid gap-3 md:grid-cols-3">
            <a v-for="card in pickCards" :key="card.title" :href="card.href" class="group rounded-2xl border border-[#e8dece] bg-[#fffaf2] p-5 transition hover:-translate-y-1 hover:border-[#c96442]/40 dark:border-dark-700 dark:bg-dark-800">
              <p class="text-lg font-semibold text-[#17130f] dark:text-white">{{ card.title }}</p>
              <p class="mt-2 text-sm leading-6 text-[#6f675b] dark:text-dark-300">{{ card.desc }}</p>
              <p class="mt-4 text-sm font-medium text-[#c96442]">去配置 →</p>
            </a>
          </div>
        </section>

        <section v-for="tool in tools" :id="tool.id" :key="tool.id" class="doc-section rounded-[28px] border border-[#e7ddcf] bg-[#fffaf2] p-6 dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-5 flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
            <div>
              <div class="mb-3 flex flex-wrap items-center gap-2">
                <span class="rounded-full bg-[#17130f] px-2.5 py-1 text-xs font-medium text-white dark:bg-white dark:text-dark-950">{{ tool.category }}</span>
                <span v-if="tool.badge" class="rounded-full bg-[#9fe870] px-2.5 py-1 text-xs font-semibold text-[#163300]">{{ tool.badge }}</span>
              </div>
              <h2 class="text-2xl font-semibold tracking-[-0.03em] text-[#17130f] dark:text-white">{{ tool.name }}</h2>
              <p class="mt-2 max-w-2xl text-sm leading-6 text-[#6f675b] dark:text-dark-300">{{ tool.desc }}</p>
            </div>
            <button @click="copy(tool.code)" class="shrink-0 rounded-full border border-[#ded4c5] bg-white px-4 py-2 text-sm font-medium text-[#17130f] transition hover:-translate-y-0.5 hover:border-[#9fe870] dark:border-dark-700 dark:bg-dark-800 dark:text-white">
              {{ copiedField === tool.code ? '已复制' : '复制配置' }}
            </button>
          </div>

          <div v-if="tool.tips?.length" class="mb-4 grid gap-2 md:grid-cols-2">
            <div v-for="tip in tool.tips" :key="tip" class="rounded-2xl border border-[#e9decd] bg-white/65 px-4 py-3 text-sm leading-6 text-[#6f675b] dark:border-dark-700 dark:bg-dark-800/70 dark:text-dark-300">
              {{ tip }}
            </div>
          </div>

          <div class="code-shell overflow-hidden rounded-2xl border border-[#272521] bg-[#151411] shadow-[0_24px_60px_-36px_rgba(20,20,19,0.8)]">
            <div class="flex items-center justify-between border-b border-white/10 px-4 py-3">
              <div class="flex items-center gap-2">
                <span class="h-2.5 w-2.5 rounded-full bg-[#ff6b5f]"></span>
                <span class="h-2.5 w-2.5 rounded-full bg-[#f7c948]"></span>
                <span class="h-2.5 w-2.5 rounded-full bg-[#9fe870]"></span>
              </div>
              <span class="font-mono text-[11px] uppercase tracking-[0.18em] text-[#b8b0a6]">{{ tool.file }}</span>
            </div>
            <pre class="overflow-x-auto p-5 text-sm leading-7 text-[#e8e1d8]"><code>{{ tool.code }}</code></pre>
          </div>

          <div v-if="tool.verify" class="mt-4 rounded-2xl border border-[#dfe8d3] bg-[#f3ffe9] p-4 dark:border-emerald-900/50 dark:bg-emerald-950/20">
            <p class="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-[#163300] dark:text-emerald-300">验证</p>
            <pre class="overflow-x-auto font-mono text-xs leading-6 text-[#163300] dark:text-emerald-200"><code>{{ tool.verify }}</code></pre>
          </div>
        </section>

        <section id="faq" class="rounded-[28px] border border-[#e7ddcf] bg-white/70 p-6 dark:border-dark-800 dark:bg-dark-900/70">
          <p class="text-xs font-semibold uppercase tracking-[0.22em] text-[#c96442]">FAQ</p>
          <h2 class="mt-2 text-2xl font-semibold tracking-[-0.03em] text-[#17130f] dark:text-white">最容易卡住的几个点</h2>
          <div class="mt-5 space-y-3">
            <details v-for="item in faqItems" :key="item.q" class="group rounded-2xl border border-[#e8dece] bg-[#fffaf2] p-4 dark:border-dark-700 dark:bg-dark-800">
              <summary class="flex cursor-pointer list-none items-center justify-between gap-4 text-sm font-semibold text-[#17130f] dark:text-white">
                {{ item.q }}
                <span class="text-[#c96442] transition group-open:rotate-45">＋</span>
              </summary>
              <p class="mt-3 text-sm leading-7 text-[#6f675b] dark:text-dark-300">{{ item.a }}</p>
            </details>
          </div>
        </section>

        <section id="errors" class="rounded-[28px] border border-[#e7ddcf] bg-[#17130f] p-6 text-white dark:border-dark-800">
          <p class="text-xs font-semibold uppercase tracking-[0.22em] text-[#9fe870]">Errors</p>
          <h2 class="mt-2 text-2xl font-semibold tracking-[-0.03em]">错误码不是给你背的，是给你定位问题的</h2>
          <div class="mt-5 overflow-hidden rounded-2xl border border-white/10">
            <table class="w-full text-left text-sm">
              <thead class="bg-white/5 text-xs uppercase tracking-[0.16em] text-[#b8b0a6]">
                <tr><th class="px-4 py-3">状态</th><th class="px-4 py-3">含义</th><th class="px-4 py-3">怎么处理</th></tr>
              </thead>
              <tbody class="divide-y divide-white/10">
                <tr v-for="err in errors" :key="err.code"><td class="px-4 py-3 font-mono text-[#9fe870]">{{ err.code }}</td><td class="px-4 py-3 text-[#e8e1d8]">{{ err.meaning }}</td><td class="px-4 py-3 text-[#b8b0a6]">{{ err.fix }}</td></tr>
              </tbody>
            </table>
          </div>
        </section>
      </article>

      <aside class="hidden xl:block">
        <nav class="sticky top-28 rounded-[24px] border border-[#e7ddcf] bg-white/60 p-4 dark:border-dark-800 dark:bg-dark-900/70">
          <p class="mb-3 text-[11px] font-semibold uppercase tracking-[0.22em] text-[#9a8f80]">On this page</p>
          <a v-for="item in tocItems" :key="item.id" :href="'#' + item.id" :class="['block py-1.5 text-sm transition', activeSection === item.id ? 'font-medium text-[#c96442]' : 'text-[#8a8173] hover:text-[#17130f] dark:text-dark-300 dark:hover:text-white']">{{ item.label }}</a>
        </nav>
      </aside>
    </main>

    <footer class="border-t border-[#e7ddcf] px-6 py-8 text-center text-sm text-[#8a8173] dark:border-dark-800 dark:text-dark-300">
      © {{ currentYear }} {{ siteName }} · <router-link to="/home" class="hover:text-[#17130f] dark:hover:text-white">返回首页</router-link>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const appStore = useAppStore()
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '卡卡AI')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const currentYear = computed(() => new Date().getFullYear())
const activeSection = ref('overview')
const copiedField = ref('')

const codexConfig = `# ~/.codex/config.toml
model = "gpt-5.5"
model_provider = "OpenAI"
disable_response_storage = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "https://api.kaiaigo.com/v1"
wire_api = "responses"
requires_openai_auth = true

# ~/.codex/auth.json
{
  "OPENAI_API_KEY": "sk-your-key"
}

# 使用
codex --model gpt-5.5`

const codexVerify = `curl https://api.kaiaigo.com/v1/models \\
  -H "Authorization: Bearer sk-your-key"`

const claudeConfig = `export ANTHROPIC_BASE_URL="https://api.kaiaigo.com"
export ANTHROPIC_API_KEY="sk-your-key"

claude --model claude-sonnet-4-6`

const claudeVerify = `curl https://api.kaiaigo.com/v1/messages \\
  -H "x-api-key: sk-your-key" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "content-type: application/json" \\
  -d '{"model":"claude-sonnet-4-6","max_tokens":20,"messages":[{"role":"user","content":"Hi"}]}'`

const pythonSdk = `from openai import OpenAI

client = OpenAI(
    api_key="sk-your-key",
    base_url="https://api.kaiaigo.com/v1",
)

response = client.chat.completions.create(
    model="gpt-5.4",
    messages=[{"role": "user", "content": "Hello"}],
)
print(response.choices[0].message.content)`

const nodeSdk = `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk-your-key",
  baseURL: "https://api.kaiaigo.com/v1",
});

const response = await client.chat.completions.create({
  model: "gpt-5.4",
  messages: [{ role: "user", content: "Hello" }],
});
console.log(response.choices[0].message.content);`

const hermesConfig = `# ~/.hermes/config.yaml
model:
  provider: openai
  default: gpt-5.5
  base_url: https://api.kaiaigo.com/v1
  api_key: sk-your-key

# 或用命令设置
hermes config set model.provider openai
hermes config set model.default gpt-5.5
hermes config set model.base_url https://api.kaiaigo.com/v1`

const openclawConfig = `// ~/.openclaw/openclaw.json
{
  "models": {
    "mode": "merge",
    "providers": {
      "kakaai": {
        "baseUrl": "https://api.kaiaigo.com/v1",
        "apiKey": "sk-your-key",
        "api": "openai-responses",
        "models": [
          { "id": "gpt-5.5" },
          { "id": "gpt-5.4" },
          { "id": "gpt-5.4-mini" }
        ]
      }
    }
  }
}`

const opencodeConfig = `// ~/.config/opencode/opencode.jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "kakaai": {
      "name": "卡卡AI",
      "options": {
        "baseURL": "https://api.kaiaigo.com/v1"
      },
      "models": {
        "gpt-5.5": { "name": "GPT-5.5" },
        "gpt-5.4": { "name": "GPT-5.4" },
        "gpt-5.4-mini": { "name": "GPT-5.4 Mini" }
      }
    }
  }
}

opencode auth login
# Provider 选 Other，provider id 填 kakaai，再粘贴 sk-your-key`

const heroFacts = [
  { label: 'OpenAI Base URL', value: 'https://api.kaiaigo.com/v1', note: 'Codex、Cursor、OpenAI SDK、OpenCode 都用这个。' },
  { label: 'Claude Base URL', value: 'https://api.kaiaigo.com', note: 'Claude Code 不要加 /v1，环境变量用 ANTHROPIC_BASE_URL。' },
  { label: 'API Key', value: 'sk-your-key', note: '登录后在 API Keys 页面创建，复制到配置里即可。' },
]

const pickCards = [
  { title: '我用命令行写代码', desc: '优先看 Codex CLI。配置最稳，适合日常编码和自动化任务。', href: '#codex' },
  { title: '我用 Claude Code', desc: '只记住一点：Base URL 不带 /v1，其它按环境变量走。', href: '#claude' },
  { title: '我在项目里调 API', desc: '看 OpenAI SDK，Python / Node.js 都是标准 OpenAI 兼容写法。', href: '#sdk' },
]

const tools = [
  { id: 'codex', name: 'Codex CLI', category: '推荐', badge: '最稳', file: '~/.codex/config.toml + auth.json', desc: '实测稳定方案是 config.toml + auth.json 两段式。不要使用旧版 bearer token 实验配置。', code: codexConfig, verify: codexVerify, tips: ['OpenAI 兼容地址必须带 /v1。', 'Key 放 auth.json，config.toml 只写 provider 和 base_url。'] },
  { id: 'claude', name: 'Claude Code', category: 'Anthropic', file: 'Shell env', desc: 'Claude Code 使用 Anthropic 协议，Base URL 写根地址，不要加 /v1。', code: claudeConfig, verify: claudeVerify, tips: ['ANTHROPIC_BASE_URL=https://api.kaiaigo.com', '如果报模型不存在，先用 /v1/models 看当前可用模型。'] },
  { id: 'sdk', name: 'OpenAI SDK', category: '开发调用', file: 'Python / Node.js', desc: '在自己的项目里调用最简单：base_url / baseURL 指向 OpenAI 兼容地址。', code: `${pythonSdk}\n\n# ---------- Node.js ----------\n${nodeSdk}`, verify: codexVerify, tips: ['Python 参数名是 base_url。', 'Node.js 参数名是 baseURL。'] },
  { id: 'hermes', name: 'Hermes Agent', category: 'Agent', file: '~/.hermes/config.yaml', desc: 'Hermes 可以按 OpenAI 兼容 provider 接入，适合多工具、多模型的 Agent 工作流。', code: hermesConfig, tips: ['如果 Hermes 版本较新，也可以用 hermes config set 写入配置。'] },
  { id: 'openclaw', name: 'OpenClaw', category: 'Agent', file: '~/.openclaw/openclaw.json', desc: '通过 models.providers 添加自定义 provider，api 推荐使用 openai-responses。', code: openclawConfig, tips: ['provider id 建议固定为 kakaai，后续切模型更清楚。'] },
  { id: 'opencode', name: 'OpenCode', category: 'CLI', file: '~/.config/opencode/opencode.jsonc', desc: 'OpenCode 通过 JSONC 添加 provider，再用 auth login 保存 API Key。', code: opencodeConfig, tips: ['配置文件可以是 opencode.jsonc 或 opencode.json。'] },
]

const faqItems = [
  { q: '购买套餐后第一步做什么？', a: '先登录，进入 API Keys 页面创建一个 Key。然后回到本页，按你使用的工具复制配置，把 sk-your-key 替换成自己的 Key。' },
  { q: '为什么 Codex 要写 /v1，Claude Code 不写 /v1？', a: 'Codex / SDK 走 OpenAI 兼容接口，路径入口是 /v1；Claude Code 走 Anthropic 协议，客户端会自己拼接请求路径，所以 Base URL 写根地址。' },
  { q: '赠送余额能买套餐吗？', a: '不能。赠送余额只用于 API 按量计费；订阅套餐只使用自己充值的余额。余额页会区分赠送余额和充值余额。' },
  { q: '每天额度在哪里看？', a: '登录后进入「我的订阅」页面，可以看到套餐到期时间、今日已用额度、今日剩余额度和重置倒计时。' },
  { q: '商务合作、采购或开发票怎么联系？', a: '商务合作、企业采购、开票请联系 QQ 591719412（银河）。' },
]

const errors = [
  { code: '401', meaning: '认证失败', fix: '检查 API Key 是否复制完整，是否被禁用。' },
  { code: '403', meaning: '无权访问', fix: '当前 Key 没有访问该模型或接口的权限。' },
  { code: '429', meaning: '请求频率超限', fix: '稍后重试，系统会自动做账号池调度。' },
  { code: '502', meaning: '上游服务异常', fix: '通常是上游临时不可用，持续出现再联系管理员。' },
]

const tocItems = computed(() => [
  { id: 'overview', label: '总览' },
  { id: 'pick', label: '工具选择' },
  ...tools.map(tool => ({ id: tool.id, label: tool.name })),
  { id: 'faq', label: 'FAQ' },
  { id: 'errors', label: '错误码' },
])

function navClass(id: string) {
  return [
    'mb-1 flex items-center justify-between gap-2 rounded-xl px-3 py-2 text-sm transition',
    activeSection.value === id
      ? 'bg-[#17130f] font-medium text-white dark:bg-white dark:text-dark-950'
      : 'text-[#6f675b] hover:bg-[#efe5d7] hover:text-[#17130f] dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
  ]
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedField.value = text
    setTimeout(() => { copiedField.value = '' }, 1600)
  } catch {
    copiedField.value = ''
  }
}

let observer: IntersectionObserver | null = null
onMounted(() => {
  const sections = document.querySelectorAll('section[id]')
  observer = new IntersectionObserver((entries) => {
    const visible = entries.filter(entry => entry.isIntersecting).sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0]
    if (visible?.target?.id) activeSection.value = visible.target.id
  }, { rootMargin: '-18% 0px -65% 0px', threshold: [0.1, 0.25, 0.5] })
  sections.forEach(section => observer?.observe(section))
})

onUnmounted(() => observer?.disconnect())
</script>

<style scoped>
.guide-page {
  font-feature-settings: "ss01", "calt";
}
.guide-title {
  font-family: Georgia, 'Times New Roman', serif;
  line-height: 0.98;
}
.doc-section {
  scroll-margin-top: 112px;
}
section[id] {
  scroll-margin-top: 112px;
}
code {
  border-radius: 7px;
  background: rgba(23, 19, 15, 0.07);
  padding: 0.12rem 0.34rem;
  font-size: 0.9em;
}
.code-shell code {
  background: transparent;
  padding: 0;
}
</style>
