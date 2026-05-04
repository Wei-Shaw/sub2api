<template>
  <AppLayout>
    <div class="docs-page mx-auto max-w-7xl">
      <header class="docs-hero">
        <div class="min-w-0">
          <p class="docs-eyebrow">API Documentation</p>
          <h1>{{ t('usageDocs.title') }}</h1>
          <p class="docs-hero-copy">
            {{ t('usageDocs.description') }}
          </p>
        </div>
        <div class="docs-hero-status">
          <span class="docs-status-dot"></span>
          <span>OpenAI Compatible</span>
        </div>
      </header>

      <section class="docs-config-grid" aria-label="快速复制配置">
        <CopyableCodeBlock title="Base URL" :code="apiBaseUrl" compact />
        <CopyableCodeBlock title="API Key" code="sk-xxxxxxxxxxxxxxxx" compact />
        <CopyableCodeBlock title="Model" code="claude-sonnet-4 / gpt-4o / gemini-2.5-pro" compact />
      </section>

      <div class="docs-layout">
        <aside class="docs-toc-card">
          <div class="docs-toc-title">目录</div>
          <nav class="docs-toc">
            <a v-for="section in sections" :key="section.id" :href="`#${section.id}`">
              {{ section.title }}
            </a>
          </nav>
        </aside>

        <main class="docs-content">
          <section id="quick-start" class="docs-panel">
            <div class="docs-section-header">
              <span>01</span>
              <div>
                <h2>快速开始</h2>
                <p>从账户、密钥到模型调用的最短路径。</p>
              </div>
            </div>
            <p>登录后先完成充值或套餐开通，再创建 API Key。所有 OpenAI 兼容客户端统一使用当前站点的 API 地址。</p>
          </section>

          <section id="billing" class="docs-panel">
            <div class="docs-section-header">
              <span>02</span>
              <div>
                <h2>充值与套餐</h2>
                <p>余额、套餐和订单统一在充值订阅页面管理。</p>
              </div>
            </div>
            <p>充值、订阅和订单记录在充值订阅页面查看。本次 UI 调整不修改计费规则、倍率、套餐权益或余额逻辑。</p>
          </section>

          <section id="api-keys" class="docs-panel">
            <div class="docs-section-header">
              <span>03</span>
              <div>
                <h2>API Key 管理</h2>
                <p>按用途创建密钥，避免多个工具共用同一个 Key。</p>
              </div>
            </div>
            <p>在 API Key 页面创建密钥，并按用途分配给 Claude Code、Codex、Gemini、Curl 或第三方工具。密钥仅展示一次，请妥善保存。</p>
          </section>

          <section id="models" class="docs-panel">
            <div class="docs-section-header">
              <span>04</span>
              <div>
                <h2>渠道与模型</h2>
                <p>模型广场只展示当前可用配置。</p>
              </div>
            </div>
            <p>查看当前可用平台、模型与价格信息。</p>
            <p class="docs-alert">{{ t('usageDocs.textToImageUnsupported') }}</p>
          </section>

          <section id="common-config" class="docs-panel">
            <div class="docs-section-header">
              <span>05</span>
              <div>
                <h2>通用配置步骤</h2>
                <p>大多数客户端都只需要 Base URL、API Key 和模型名。</p>
              </div>
            </div>
            <ol class="docs-step-list">
              <li><span>1</span><p>复制当前站点 Base URL：<code>{{ apiBaseUrl }}</code></p></li>
              <li><span>2</span><p>创建或复制 API Key，并写入客户端环境变量。</p></li>
              <li><span>3</span><p>选择模型名称，发起一次最小请求验证连通性。</p></li>
            </ol>
            <div class="docs-code-stack">
              <CopyableCodeBlock title="Base URL" :code="apiBaseUrl" />
              <CopyableCodeBlock title="API Key" code="sk-xxxxxxxxxxxxxxxx" />
              <CopyableCodeBlock title="Model" code="claude-sonnet-4 / gpt-4o / gemini-2.5-pro" />
            </div>
            <CopyableCodeBlock
              title="Shell 环境变量"
              :code="`export OPENAI_BASE_URL=${apiBaseUrl}\nexport OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxx`"
            />
          </section>

          <section id="cli-config" class="docs-panel">
            <div class="docs-section-header">
              <span>06</span>
              <div>
                <h2>CLI 配置</h2>
                <p>常用命令行工具的环境变量示例。</p>
              </div>
            </div>
            <div class="docs-code-stack">
              <CopyableCodeBlock
                title="Claude Code"
                :code="`export ANTHROPIC_BASE_URL=${apiBaseUrl}\nexport ANTHROPIC_AUTH_TOKEN=sk-xxxxxxxxxxxxxxxx\nclaude`"
              />
              <CopyableCodeBlock
                title="Codex"
                :code="`export OPENAI_BASE_URL=${apiBaseUrl}\nexport OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxx\ncodex`"
              />
              <CopyableCodeBlock
                title="Gemini"
                :code="`export GEMINI_BASE_URL=${apiBaseUrl}\nexport GEMINI_API_KEY=sk-xxxxxxxxxxxxxxxx\ngemini`"
              />
            </div>
          </section>

          <section id="curl" class="docs-panel">
            <div class="docs-section-header">
              <span>07</span>
              <div>
                <h2>Curl 调用示例</h2>
                <p>用于验证 Key、模型名和网络连通性。</p>
              </div>
            </div>
            <CopyableCodeBlock
              title="OpenAI 兼容 Chat Completions"
              :code="curlExample"
            />
          </section>

          <section id="third-party" class="docs-panel">
            <div class="docs-section-header">
              <span>08</span>
              <div>
                <h2>第三方工具配置</h2>
                <p>把工具里的 OpenAI Base URL 指向当前站点。</p>
              </div>
            </div>
            <div class="docs-tool-grid">
              <ToolRow name="OpenCode" :base-url="apiBaseUrl" />
              <ToolRow name="Kilocode" :base-url="apiBaseUrl" />
              <ToolRow name="Zed" :base-url="apiBaseUrl" />
              <ToolRow name="Hermes Agent" :base-url="apiBaseUrl" />
              <ToolRow name="WSL" :base-url="apiBaseUrl" />
            </div>
          </section>

          <section id="faq" class="docs-panel">
            <div class="docs-section-header">
              <span>09</span>
              <div>
                <h2>常见问题</h2>
                <p>优先检查配置、余额和模型可用性。</p>
              </div>
            </div>
            <div class="docs-faq-list">
              <p><strong>请求失败怎么办？</strong> 先确认 Base URL、API Key、模型名称和账户余额，再查看用量记录。</p>
              <p><strong>价格在哪里看？</strong> 在模型广场查看当前前端可展示的渠道、模型与价格信息。</p>
              <p><strong>支持文生图吗？</strong> {{ t('usageDocs.textToImageUnsupported') }}</p>
            </div>
          </section>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'

const { t } = useI18n()

const siteOrigin = computed(() => (typeof window === 'undefined' ? 'https://your-domain.example' : window.location.origin))
const apiBaseUrl = computed(() => `${siteOrigin.value}/v1`)

const sections = [
  { id: 'quick-start', title: '快速开始' },
  { id: 'billing', title: '充值与套餐' },
  { id: 'api-keys', title: 'API Key 管理' },
  { id: 'models', title: '渠道与模型' },
  { id: 'common-config', title: '通用配置步骤' },
  { id: 'cli-config', title: 'CLI 配置' },
  { id: 'curl', title: 'Curl 调用示例' },
  { id: 'third-party', title: '第三方工具配置' },
  { id: 'faq', title: '常见问题' },
]

const curlExample = computed(
  () => `curl ${apiBaseUrl.value}/chat/completions \\
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-sonnet-4",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`,
)

async function copyText(value: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return
    } catch {
      // Fall through to textarea copy for insecure origins or denied permissions.
    }
  }

  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  try {
    textarea.select()
    const copied = document.execCommand('copy')
    if (!copied) {
      throw new Error('Fallback copy command returned false')
    }
  } finally {
    document.body.removeChild(textarea)
  }
}

const CopyableCodeBlock = defineComponent({
  props: {
    title: { type: String, required: true },
    code: { type: String, required: true },
    compact: { type: Boolean, default: false },
  },
  setup(props) {
    const copied = ref(false)
    const handleCopy = async () => {
      try {
        await copyText(props.code)
        copied.value = true
        window.setTimeout(() => {
          copied.value = false
        }, 1200)
      } catch (err) {
        console.error('Failed to copy documentation block:', err)
      }
    }

    return () =>
      h('div', { class: ['docs-code-block', props.compact && 'docs-code-block-compact'] }, [
        h('div', { class: 'docs-code-header' }, [
          h('span', props.title),
          h(
            'button',
            {
              type: 'button',
              class: 'docs-copy-button',
              'data-testid': 'copy-code-button',
              'aria-label': `复制 ${props.title}`,
              onClick: handleCopy,
            },
            copied.value ? '已复制' : '复制',
          ),
        ]),
        h('pre', [h('code', props.code)]),
      ])
  },
})

const ToolRow = defineComponent({
  props: {
    name: { type: String, required: true },
    baseUrl: { type: String, required: true },
  },
  setup(props) {
    return () =>
      h('div', { class: 'docs-tool-row' }, [
        h('strong', props.name),
        h('span', 'OpenAI Base URL'),
        h('code', props.baseUrl),
      ])
  },
})
</script>

<style scoped>
.docs-page {
  padding: 1.5rem;
  color: rgb(17 24 39);
}

.docs-hero {
  align-items: flex-start;
  background: linear-gradient(135deg, rgb(255 255 255), rgb(248 250 252));
  border: 1px solid rgb(229 231 235);
  border-radius: 0.75rem;
  display: flex;
  gap: 1rem;
  justify-content: space-between;
  padding: 1.5rem;
}

.docs-eyebrow {
  color: rgb(37 99 235);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  margin-bottom: 0.5rem;
  text-transform: uppercase;
}

.docs-hero h1 {
  color: rgb(17 24 39);
  font-size: 1.875rem;
  font-weight: 700;
  letter-spacing: 0;
  line-height: 1.2;
}

.docs-hero-copy {
  color: rgb(75 85 99);
  font-size: 0.9375rem;
  line-height: 1.7;
  margin-top: 0.75rem;
  max-width: 48rem;
}

.docs-hero-status {
  align-items: center;
  background: rgb(239 246 255);
  border: 1px solid rgb(191 219 254);
  border-radius: 999px;
  color: rgb(30 64 175);
  display: inline-flex;
  flex-shrink: 0;
  font-size: 0.75rem;
  font-weight: 600;
  gap: 0.5rem;
  min-height: 2rem;
  padding: 0.375rem 0.75rem;
}

.docs-status-dot {
  background: rgb(34 197 94);
  border-radius: 999px;
  height: 0.5rem;
  width: 0.5rem;
}

.docs-config-grid {
  display: grid;
  gap: 0.75rem;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 1rem;
}

.docs-layout {
  display: grid;
  gap: 1.25rem;
  grid-template-columns: 240px minmax(0, 1fr);
  margin-top: 1.25rem;
}

.docs-toc-card {
  align-self: start;
  background: rgb(255 255 255);
  border: 1px solid rgb(229 231 235);
  border-radius: 0.75rem;
  padding: 0.75rem;
  position: sticky;
  top: 5rem;
}

.docs-toc-title {
  color: rgb(17 24 39);
  font-size: 0.8125rem;
  font-weight: 700;
  margin-bottom: 0.5rem;
  padding: 0 0.5rem;
}

.docs-toc {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.docs-toc a {
  border-radius: 0.5rem;
  color: rgb(75 85 99);
  font-size: 0.8125rem;
  line-height: 1.4;
  padding: 0.5rem;
}

.docs-toc a:hover {
  background: rgb(239 246 255);
  color: rgb(17 24 39);
}

.docs-content {
  display: grid;
  gap: 1rem;
  min-width: 0;
}

.docs-panel {
  background: rgb(255 255 255);
  border: 1px solid rgb(229 231 235);
  border-radius: 0.75rem;
  padding: 1.25rem;
}

.docs-section-header {
  align-items: flex-start;
  display: flex;
  gap: 0.875rem;
  margin-bottom: 0.875rem;
}

.docs-section-header > span {
  align-items: center;
  background: rgb(241 245 249);
  border-radius: 0.5rem;
  color: rgb(37 99 235);
  display: inline-flex;
  flex-shrink: 0;
  font-size: 0.75rem;
  font-weight: 700;
  height: 2rem;
  justify-content: center;
  width: 2rem;
}

.docs-section-header h2 {
  color: rgb(17 24 39);
  font-size: 1.125rem;
  font-weight: 700;
  line-height: 1.35;
}

.docs-section-header p {
  color: rgb(107 114 128);
  font-size: 0.8125rem;
  line-height: 1.5;
  margin-top: 0.125rem;
}

.docs-panel > p,
.docs-panel li {
  color: rgb(75 85 99);
  font-size: 0.875rem;
  line-height: 1.7;
}

.docs-step-list {
  display: grid;
  gap: 0.75rem;
  list-style: none;
  margin-bottom: 1rem;
  padding: 0;
}

.docs-step-list li {
  align-items: flex-start;
  background: rgb(248 250 252);
  border: 1px solid rgb(226 232 240);
  border-radius: 0.625rem;
  display: flex;
  gap: 0.75rem;
  padding: 0.75rem;
}

.docs-step-list li > span {
  align-items: center;
  background: rgb(37 99 235);
  border-radius: 999px;
  color: white;
  display: inline-flex;
  flex-shrink: 0;
  font-size: 0.75rem;
  font-weight: 700;
  height: 1.5rem;
  justify-content: center;
  width: 1.5rem;
}

.docs-step-list code {
  color: rgb(17 24 39);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  word-break: break-all;
}

.docs-code-stack {
  display: grid;
  gap: 0.75rem;
}

.docs-alert {
  align-items: center;
  background: rgb(255 251 235);
  border: 1px solid rgb(253 230 138);
  border-radius: 0.625rem;
  color: rgb(146 64 14) !important;
  display: flex;
  margin-top: 0.75rem;
  padding: 0.75rem;
}

.docs-code-block {
  border: 1px solid rgb(203 213 225);
  border-radius: 0.625rem;
  overflow: hidden;
}

.docs-code-block-compact {
  min-width: 0;
}

.docs-code-header {
  align-items: center;
  background: rgb(248 250 252);
  border-bottom: 1px solid rgb(226 232 240);
  color: rgb(51 65 85);
  display: flex;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.625rem 0.75rem;
}

.docs-code-header span {
  font-size: 0.75rem;
  font-weight: 700;
}

.docs-copy-button {
  border-radius: 0.375rem;
  background: rgb(37 99 235);
  color: rgb(255 255 255);
  font-size: 0.75rem;
  min-height: 1.75rem;
  padding: 0.25rem 0.5rem;
}

.docs-copy-button:hover {
  background: rgb(29 78 216);
}

.docs-code-block pre {
  background: rgb(15 23 42);
  color: rgb(226 232 240);
  overflow-x: auto;
  padding: 0.875rem 1rem;
}

.docs-code-block code {
  font-size: 0.8125rem;
  line-height: 1.6;
  white-space: pre;
}

.docs-tool-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
}

.docs-tool-row {
  background: rgb(248 250 252);
  border: 1px solid rgb(226 232 240);
  border-radius: 0.625rem;
  padding: 0.875rem;
}

.docs-tool-row strong {
  color: rgb(17 24 39);
  display: block;
  font-size: 0.9375rem;
  margin-bottom: 0.5rem;
}

.docs-tool-row span {
  color: rgb(100 116 139);
  display: block;
  font-size: 0.75rem;
  font-weight: 600;
  margin-bottom: 0.25rem;
}

.docs-tool-row code {
  color: rgb(30 41 59);
  font-size: 0.8125rem;
  word-break: break-all;
}

.docs-faq-list {
  display: grid;
  gap: 0.75rem;
}

.docs-faq-list p {
  background: rgb(248 250 252);
  border: 1px solid rgb(226 232 240);
  border-radius: 0.625rem;
  color: rgb(75 85 99);
  font-size: 0.875rem;
  line-height: 1.7;
  padding: 0.875rem;
}

.docs-faq-list strong {
  color: rgb(17 24 39);
}

:global(.dark) .docs-page {
  color: rgb(243 244 246);
}

:global(.dark) .docs-hero,
:global(.dark) .docs-toc-card,
:global(.dark) .docs-panel {
  background: rgb(17 24 39);
  border-color: rgb(51 65 85);
}

:global(.dark) .docs-hero h1,
:global(.dark) .docs-toc-title,
:global(.dark) .docs-section-header h2,
:global(.dark) .docs-tool-row strong,
:global(.dark) .docs-faq-list strong,
:global(.dark) .docs-step-list code {
  color: rgb(248 250 252);
}

:global(.dark) .docs-hero-copy,
:global(.dark) .docs-panel > p,
:global(.dark) .docs-panel li,
:global(.dark) .docs-section-header p,
:global(.dark) .docs-faq-list p {
  color: rgb(203 213 225);
}

:global(.dark) .docs-hero-status {
  background: rgb(30 41 59);
  border-color: rgb(51 65 85);
  color: rgb(191 219 254);
}

:global(.dark) .docs-toc,
:global(.dark) .docs-code-block,
:global(.dark) .docs-tool-row {
  border-color: rgb(55 65 81);
}

:global(.dark) .docs-toc a {
  color: rgb(209 213 219);
}

:global(.dark) .docs-toc a:hover {
  background: rgb(30 41 59);
  color: white;
}

:global(.dark) .docs-section-header > span,
:global(.dark) .docs-step-list li,
:global(.dark) .docs-code-header,
:global(.dark) .docs-tool-row,
:global(.dark) .docs-faq-list p {
  background: rgb(30 41 59);
  border-color: rgb(51 65 85);
}

:global(.dark) .docs-tool-row code {
  color: rgb(226 232 240);
}

@media (max-width: 1023px) {
  .docs-page {
    padding: 1rem;
  }

  .docs-hero {
    display: grid;
    padding: 1.25rem;
  }

  .docs-config-grid,
  .docs-layout {
    grid-template-columns: 1fr;
  }

  .docs-toc-card {
    position: static;
  }

  .docs-toc {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .docs-tool-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .docs-hero h1 {
    font-size: 1.5rem;
  }

  .docs-toc {
    grid-template-columns: 1fr;
  }

  .docs-panel {
    padding: 1rem;
  }
}
</style>
