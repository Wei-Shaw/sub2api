<template>
  <section class="api-preview" :aria-label="t('home.macosHero.windowTitle')">
    <div class="api-preview__tabs">
      <div class="api-preview__tab-list" role="tablist" aria-label="API protocol examples">
        <button
          v-for="(example, index) in examples"
          :id="`api-tab-${example.id}`"
          :key="example.id"
          type="button"
          role="tab"
          class="api-preview__tab"
          :class="{ 'api-preview__tab--active': activeId === example.id }"
          :aria-selected="activeId === example.id"
          aria-controls="api-preview-panel"
          :tabindex="activeId === example.id ? 0 : -1"
          @click="activeId = example.id"
          @keydown="handleTabKeydown($event, index)"
        >
          {{ example.label }}
        </button>
      </div>
      <span class="api-preview__health">
        <span class="api-preview__health-dot" aria-hidden="true"></span>
        200 OK
      </span>
    </div>

    <div class="api-preview__endpoint" :title="fullEndpoint">
      <span class="api-preview__method">POST</span>
      <code>{{ activeExample.endpoint }}</code>
    </div>

    <Transition name="preview-swap" mode="out-in">
      <div
        id="api-preview-panel"
        :key="activeExample.id"
        class="api-preview__exchange"
        role="tabpanel"
        :aria-labelledby="`api-tab-${activeExample.id}`"
        tabindex="0"
      >
        <section class="api-preview__request">
          <div class="api-preview__section-heading">
            <h3>{{ t('home.macosHero.request') }}</h3>
            <span>{{ activeExample.requestFormat }}</span>
          </div>
          <pre><code>{{ activeExample.request }}</code></pre>
        </section>

        <section class="api-preview__response">
          <div class="api-preview__section-heading">
            <h3>{{ t('home.macosHero.response') }}</h3>
            <span class="api-preview__response-status">Success</span>
          </div>
          <pre><code>{{ activeExample.response }}</code></pre>
        </section>
      </div>
    </Transition>

    <footer class="api-preview__meta" aria-label="API preview metadata">
      <span>{{ activeExample.latency }} <i></i> {{ activeExample.tokens }} <i></i> DEMO</span>
      <span>{{ activeExample.transport }} <i></i> SSE</span>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

type ApiPreviewTab = 'chat' | 'responses' | 'claude' | 'gemini'

interface ApiExample {
  id: ApiPreviewTab
  label: string
  endpoint: string
  requestFormat: string
  request: string
  response: string
  latency: string
  tokens: string
  transport: string
}

const props = defineProps<{ apiBaseUrl: string }>()
const { t } = useI18n()

const examples: ApiExample[] = [
  {
    id: 'chat',
    label: 'Chat',
    endpoint: '/v1/chat/completions',
    requestFormat: 'OpenAI compatible',
    request: `curl -X POST "/v1/chat/completions" \\
  -H "Authorization: Bearer sk-••••" \\
  -d '{
    "model": "your-model",
    "messages": [
      { "role": "user", "content": "Hello" }
    ]
  }'`,
    response: `{
  "choices": [{
    "message": { "content": "Chat request routed." }
  }],
  "usage": { "total_tokens": 27 }
}`,
    latency: '142 MS',
    tokens: '27 TOKENS',
    transport: 'STREAM'
  },
  {
    id: 'responses',
    label: 'Responses',
    endpoint: '/v1/responses',
    requestFormat: 'OpenAI Responses',
    request: `curl -X POST "/v1/responses" \\
  -H "Authorization: Bearer sk-••••" \\
  -d '{
    "model": "your-model",
    "input": "Explain this API in one sentence."
  }'`,
    response: `{
  "status": "completed",
  "output": [{
    "type": "message", "role": "assistant"
  }]
}`,
    latency: '118 MS',
    tokens: '31 TOKENS',
    transport: 'EVENTS'
  },
  {
    id: 'claude',
    label: 'Claude',
    endpoint: '/v1/messages',
    requestFormat: 'Anthropic native',
    request: `curl -X POST "/v1/messages" \\
  -H "x-api-key: sk-••••" \\
  -H "anthropic-version: 2023-06-01" \\
  -d '{
    "model": "claude-sonnet-4",
    "messages": [{ "role": "user", "content": "Hello" }]
  }'`,
    response: `{
  "type": "message",
  "role": "assistant",
  "content": [{ "type": "text", "text": "Hello." }]
}`,
    latency: '164 MS',
    tokens: '24 TOKENS',
    transport: 'MESSAGES'
  },
  {
    id: 'gemini',
    label: 'Gemini',
    endpoint: '/v1beta/models/gemini-2.5-flash:generateContent',
    requestFormat: 'Google AI native',
    request: `curl -X POST "/v1beta/models/gemini-2.5-flash:generateContent" \\
  -H "x-goog-api-key: sk-••••" \\
  -d '{
    "contents": [{
      "parts": [{ "text": "Hello" }]
    }]
  }'`,
    response: `{
  "candidates": [{
    "content": { "parts": [{ "text": "Hello." }] }
  }]
}`,
    latency: '126 MS',
    tokens: '22 TOKENS',
    transport: 'CONTENT'
  }
]

const activeId = ref<ApiPreviewTab>('chat')
const activeExample = computed(
  () => examples.find((example) => example.id === activeId.value) ?? examples[0]
)
const fullEndpoint = computed(
  () => `${props.apiBaseUrl.replace(/\/+$/, '')}${activeExample.value.endpoint}`
)

function handleTabKeydown(event: KeyboardEvent, index: number) {
  const keys: Partial<Record<string, number>> = {
    ArrowRight: (index + 1) % examples.length,
    ArrowLeft: (index - 1 + examples.length) % examples.length,
    Home: 0,
    End: examples.length - 1
  }
  const nextIndex = keys[event.key]
  if (nextIndex === undefined) return

  event.preventDefault()
  activeId.value = examples[nextIndex].id
  const tablist = (event.currentTarget as HTMLButtonElement).parentElement
  tablist?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[nextIndex]?.focus()
}
</script>

<style scoped>
.api-preview {
  width: min(100%, 560px);
  min-width: 0;
  overflow: hidden;
  color: var(--home-muted);
  background: var(--home-glass-strong);
  border: 1px solid var(--home-glass-border);
  border-radius: 16px;
  box-shadow:
    inset 0 1px 0 var(--home-glass-highlight),
    0 6px 8px rgb(7 16 36 / 10%);
  font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Helvetica Neue", Arial, sans-serif;
  transition: transform 220ms cubic-bezier(.22, 1, .36, 1), border-color 220ms ease;
  backdrop-filter: blur(20px) saturate(140%);
  -webkit-backdrop-filter: blur(20px) saturate(140%);
}

.api-preview:hover {
  border-color: var(--home-glass-border-strong);
  transform: translateY(-2px);
}

.api-preview__tabs {
  display: flex;
  min-height: 46px;
  align-items: stretch;
  justify-content: space-between;
  gap: 12px;
  padding: 0 12px;
  background: var(--home-glass-sheen);
  border-bottom: 1px solid var(--home-glass-divider);
}

.api-preview__tab-list {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 4px;
  padding: 6px 0;
  overflow-x: auto;
  scrollbar-width: none;
}

.api-preview__tab-list::-webkit-scrollbar { display: none; }

.api-preview__tab {
  position: relative;
  flex: 0 0 auto;
  min-width: 48px;
  min-height: 34px;
  padding: 0 10px;
  color: var(--home-muted);
  background: transparent;
  border: 0;
  border-radius: 9px;
  cursor: pointer;
  font: inherit;
  font-size: 12px;
  font-weight: 650;
  transition:
    color 180ms ease,
    background-color 180ms ease,
    box-shadow 180ms ease,
    transform 180ms cubic-bezier(.22, 1, .36, 1);
}

.api-preview__tab:hover { color: var(--home-text); background: var(--home-glass-hover); transform: translateY(-1px); }
.api-preview__tab--active {
  color: var(--home-accent);
  background: var(--home-glass-hover);
  box-shadow:
    inset 0 0 0 1px var(--home-glass-border-strong),
    inset 0 1px 0 var(--home-glass-highlight);
}
.api-preview__tab--active:active { transform: scale(.97); }
.api-preview__tab:focus-visible { outline: 2px solid var(--home-accent); outline-offset: -4px; border-radius: 8px; }

.api-preview__health {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  color: var(--home-muted);
  font: 10px/1 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  letter-spacing: .04em;
}

.api-preview__health-dot { width: 7px; height: 7px; background: #22c58b; border-radius: 50%; box-shadow: 0 0 0 3px rgb(34 197 139 / 12%); }

.api-preview__endpoint {
  display: flex;
  min-height: 48px;
  align-items: center;
  gap: 10px;
  box-sizing: border-box;
  padding: 12px 20px;
  border-bottom: 1px solid var(--home-glass-divider);
  font: 12px/1.4 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.api-preview__endpoint code { overflow: hidden; color: var(--home-code-text); text-overflow: ellipsis; white-space: nowrap; }
.api-preview__method { padding: 4px 7px; color: var(--home-success); background: var(--home-success-soft); border-radius: 6px; font-size: 10px; font-weight: 760; }

.api-preview__exchange { display: grid; grid-template-rows: 235px 165px; min-width: 0; height: 400px; outline: none; }
.api-preview__exchange:focus-visible { box-shadow: inset 0 0 0 2px var(--home-accent); }
.api-preview__request, .api-preview__response { min-width: 0; overflow: hidden; box-sizing: border-box; padding: 16px 20px; }
.api-preview__response { background: var(--home-glass-sheen); border-top: 1px solid var(--home-glass-divider); }

.api-preview__section-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 14px; }
.api-preview h3 { margin: 0; color: var(--home-muted); font-size: 10px; font-weight: 760; letter-spacing: .12em; }
.api-preview__section-heading > span { color: var(--home-faint); font-size: 10px; font-weight: 600; }
.api-preview__response-status { color: var(--home-success) !important; }
.api-preview pre { max-width: 100%; margin: 0; overflow-x: auto; color: var(--home-code-text); font: 12px/1.58 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; scrollbar-color: var(--home-glass-divider) transparent; scrollbar-width: thin; white-space: pre; }

.api-preview__meta {
  display: flex;
  min-height: 38px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  box-sizing: border-box;
  padding: 10px 20px;
  color: var(--home-faint);
  background: var(--home-glass-sheen);
  border-top: 1px solid var(--home-glass-divider);
  font: 9px/1 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  letter-spacing: .03em;
}

.api-preview__meta span { display: inline-flex; align-items: center; gap: 7px; white-space: nowrap; }
.api-preview__meta i { width: 3px; height: 3px; background: currentColor; border-radius: 50%; opacity: .55; }

.preview-swap-enter-active,
.preview-swap-leave-active { transition: opacity 180ms ease, transform 180ms cubic-bezier(.22, 1, .36, 1); }
.preview-swap-enter-from { opacity: 0; transform: translateY(5px); }
.preview-swap-leave-to { opacity: 0; transform: translateY(-3px); }

@media (max-width: 640px) {
  .api-preview { border-radius: 14px; }
  .api-preview__tabs { padding-inline: 8px; }
  .api-preview__tab { padding-inline: 7px; }
  .api-preview__exchange { grid-template-rows: 230px 158px; height: 388px; }
  .api-preview__request, .api-preview__response { padding: 15px 16px; }
  .api-preview__endpoint, .api-preview__meta { padding-inline: 16px; }
  .api-preview pre { font-size: 10.5px; }
  .api-preview__section-heading > span { display: none; }
  .api-preview__meta { overflow-x: auto; }
}

@media (prefers-reduced-motion: reduce) {
  .api-preview,
  .api-preview__tab,
  .preview-swap-enter-active,
  .preview-swap-leave-active { transition: none; }
  .api-preview__tab:hover,
  .api-preview:hover { transform: none; }
}
</style>

<style>
.mac-home--dark .api-preview {
  color: #aab7ca;
  background: rgb(5 10 18 / 90%);
  border-color: rgb(255 255 255 / 11%);
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 8%),
    0 6px 8px rgb(0 0 0 / 28%);
}

.mac-home--dark .api-preview__tabs,
.mac-home--dark .api-preview__response,
.mac-home--dark .api-preview__meta {
  background: rgb(255 255 255 / 3%);
}

.mac-home--dark .api-preview__tabs,
.mac-home--dark .api-preview__endpoint,
.mac-home--dark .api-preview__response,
.mac-home--dark .api-preview__meta {
  border-color: rgb(255 255 255 / 7%);
}

.mac-home--dark .api-preview__endpoint code,
.mac-home--dark .api-preview pre {
  color: #c9d6e8;
}
</style>
