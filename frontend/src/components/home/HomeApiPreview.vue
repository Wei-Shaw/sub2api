<template>
  <section class="api-preview" :aria-label="t('home.macosHero.windowTitle')">
    <div class="api-preview__tabs">
      <div class="api-preview__tab-list" aria-hidden="true">
        <span class="api-preview__tab api-preview__tab--active">Chat</span>
        <span class="api-preview__tab">Responses</span>
        <span class="api-preview__tab">Claude</span>
        <span class="api-preview__tab">Gemini</span>
      </div>
      <span class="api-preview__health">
        <span class="api-preview__health-dot" aria-hidden="true"></span>
        200 OK
      </span>
    </div>

    <div class="api-preview__endpoint" :title="fullEndpoint">
      <span class="api-preview__method">POST</span>
      <code>/v1/chat/completions</code>
    </div>

    <div class="api-preview__exchange">
      <section class="api-preview__request">
        <h3>{{ t('home.macosHero.request') }}</h3>
        <pre><code><span class="syntax-command">curl</span> <span class="syntax-option">-X POST</span> <span class="syntax-string">"/v1/chat/completions"</span> \
  <span class="syntax-option">-H</span> <span class="syntax-string">"Authorization: Bearer sk-••••"</span> \
  <span class="syntax-option">-d</span> <span class="syntax-brace">'{</span>
    <span class="syntax-key">"model"</span>: <span class="syntax-string">"your-model"</span>,
    <span class="syntax-key">"messages"</span>: <span class="syntax-brace">[</span>
      <span class="syntax-brace">{</span> <span class="syntax-key">"role"</span>: <span class="syntax-string">"user"</span>, <span class="syntax-key">"content"</span>: <span class="syntax-string">"..."</span> <span class="syntax-brace">}</span>
    <span class="syntax-brace">]</span>
  <span class="syntax-brace">}'</span></code></pre>
      </section>

      <section class="api-preview__response">
        <h3>{{ t('home.macosHero.response') }}</h3>
        <pre><code><span class="syntax-brace">{</span>
  <span class="syntax-key">"choices"</span>: <span class="syntax-brace">[{</span> <span class="syntax-key">"message"</span>: <span class="syntax-brace">{</span> <span class="syntax-key">"content"</span>: <span class="syntax-string">"Chat request routed."</span> <span class="syntax-brace">}</span> <span class="syntax-brace">}]</span>,
  <span class="syntax-key">"usage"</span>: <span class="syntax-brace">{</span> <span class="syntax-key">"total_tokens"</span>: <span class="syntax-number">27</span> <span class="syntax-brace">}</span>
<span class="syntax-brace">}</span></code></pre>
      </section>
    </div>

    <footer class="api-preview__meta" aria-label="API preview metadata">
      <span>142 MS <i></i> 27 TOKENS <i></i> COST $0.00081</span>
      <span>STREAM <i></i> SSE</span>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ apiBaseUrl: string }>()
const { t } = useI18n()
const fullEndpoint = computed(
  () => `${props.apiBaseUrl.replace(/\/+$/, '')}/v1/chat/completions`
)
</script>

<style scoped>
.api-preview {
  width: min(100%, 560px);
  min-width: 0;
  overflow: hidden;
  color: #667085;
  background: rgb(255 255 255 / 95%);
  border: 1px solid rgb(226 232 240 / 72%);
  border-radius: 28.8px;
  box-shadow: 0 20px 50px -25px rgb(15 23 42 / 18%);
  font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Helvetica Neue", Arial, sans-serif;
  transform: translateZ(0);
  transition: transform 260ms ease, box-shadow 260ms ease;
  backdrop-filter: blur(8px) saturate(120%);
}

.api-preview:hover {
  box-shadow: 0 24px 58px -25px rgb(15 23 42 / 24%);
  transform: translateY(-3px);
}

.api-preview__tabs {
  display: flex;
  min-height: 37px;
  align-items: stretch;
  justify-content: space-between;
  gap: 12px;
  padding: 0 12px;
  border-bottom: 1px solid rgb(226 232 240 / 72%);
}

.api-preview__tab-list { display: flex; min-width: 0; gap: 6px; overflow-x: auto; scrollbar-width: none; }
.api-preview__tab-list::-webkit-scrollbar { display: none; }
.api-preview__tab { display: inline-flex; flex: 0 0 auto; align-items: center; padding: 0 9px; color: #a1a1aa; font-size: 12px; font-weight: 600; }
.api-preview__tab--active { color: #10a36f; box-shadow: inset 0 -1.5px #10b981; }
.api-preview__health { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 8px; color: #a1a1aa; font: 10px/1 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; letter-spacing: .04em; }
.api-preview__health-dot { width: 7px; height: 7px; background: #10b981; border-radius: 50%; box-shadow: 0 0 0 3px rgb(16 185 129 / 10%); }

.api-preview__endpoint {
  display: flex;
  min-height: 44px;
  align-items: center;
  gap: 10px;
  box-sizing: border-box;
  padding: 12px 20px;
  border-bottom: 1px solid rgb(226 232 240 / 65%);
  font: 12px/1.4 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.api-preview__endpoint code { overflow: hidden; color: #71717a; text-overflow: ellipsis; white-space: nowrap; }
.api-preview__method { padding: 4px 7px; color: #0f9f6e; background: #dff8ed; border-radius: 6px; font-size: 10px; font-weight: 750; }

.api-preview__exchange { display: grid; grid-template-rows: 235px 165px; min-width: 0; }
.api-preview__request, .api-preview__response { min-width: 0; overflow: hidden; box-sizing: border-box; padding: 16px 20px; }
.api-preview__response { background: rgb(248 250 252 / 62%); border-top: 1px solid rgb(226 232 240 / 65%); }
.api-preview h3 { margin: 0 0 14px; color: #b0b3ba; font-size: 10px; font-weight: 750; letter-spacing: .2em; text-transform: uppercase; }
.api-preview pre { max-width: 100%; margin: 0; overflow-x: auto; color: #8b8f97; font: 12.5px/1.55 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; scrollbar-color: rgb(148 163 184 / 30%) transparent; scrollbar-width: thin; white-space: pre; }
.syntax-command, .syntax-string { color: #15a875; }
.syntax-option, .syntax-key { color: #3b82f6; }
.syntax-brace { color: #9ca3af; }
.syntax-number { color: #8b5cf6; }

.api-preview__meta { display: flex; min-height: 36px; align-items: center; justify-content: space-between; gap: 12px; box-sizing: border-box; padding: 10px 20px; color: #b0b3ba; background: rgb(248 250 252 / 75%); border-top: 1px solid rgb(226 232 240 / 65%); font: 9px/1 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; letter-spacing: .03em; }
.api-preview__meta span { display: inline-flex; align-items: center; gap: 7px; white-space: nowrap; }
.api-preview__meta i { width: 3px; height: 3px; background: #d4d4d8; border-radius: 50%; }

@media (max-width: 640px) {
  .api-preview { border-radius: 22px; }
  .api-preview__exchange { grid-template-rows: auto auto; }
  .api-preview__request, .api-preview__response { min-height: 170px; padding: 15px 16px; }
  .api-preview__endpoint, .api-preview__meta { padding-inline: 16px; }
  .api-preview pre { font-size: 11px; }
  .api-preview__meta { overflow-x: auto; }
}

@media (prefers-reduced-motion: reduce) {
  .api-preview { transition: none; }
  .api-preview:hover { transform: none; }
}
</style>

<style>
.mac-home--dark .api-preview {
  color: #aab3c2;
  background: rgb(11 15 23 / 94%);
  border-color: rgb(255 255 255 / 8%);
  box-shadow: 0 24px 62px -24px rgb(0 0 0 / 65%);
}

.mac-home--dark .api-preview__tabs,
.mac-home--dark .api-preview__endpoint,
.mac-home--dark .api-preview__response,
.mac-home--dark .api-preview__meta {
  border-color: rgb(255 255 255 / 6%);
}

.mac-home--dark .api-preview__response,
.mac-home--dark .api-preview__meta {
  background: rgb(255 255 255 / 2%);
}

.mac-home--dark .api-preview__endpoint code {
  color: #c1c8d4;
}
</style>
