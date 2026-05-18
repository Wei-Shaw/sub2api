<template>
  <div class="card border-l-4 border-l-primary-400">
    <div class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div>
        <span class="text-xs font-semibold tracking-wider text-primary-500 uppercase">{{ t('dashboard.quickstart') }}</span>
        <p class="mt-1 text-xs text-gray-400 dark:text-dark-300">{{ t('dashboard.quickstartDesc') }}</p>
      </div>
      <button @click="router.push('/guide')" class="text-xs font-medium text-primary-500 transition-colors hover:text-primary-600">
        {{ t('dashboard.viewFullGuide') }} →
      </button>
    </div>
    <div class="p-6">
      <!-- Tool Tabs -->
      <div class="mb-4 flex gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          @click="activeTab = tab.id"
          :class="[
            'flex-1 rounded-md px-3 py-2 text-xs font-medium transition-all',
            activeTab === tab.id
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-700 dark:text-dark-300 dark:hover:text-dark-200'
          ]"
        >{{ tab.label }}</button>
      </div>

      <!-- Codex CLI -->
      <div v-if="activeTab === 'codex'">
        <p class="mb-2 text-xs text-gray-500 dark:text-dark-300">{{ t('dashboard.codexHint') }}</p>
        <div class="relative rounded-lg bg-gray-900 p-4 font-mono text-xs leading-relaxed">
          <pre class="whitespace-pre-wrap text-gray-100"><span class="text-gray-500"># ~/.codex/config.toml</span>
<span class="text-emerald-400">model_provider</span> = <span class="text-amber-300">"willeai"</span>
<span class="text-emerald-400">model</span> = <span class="text-amber-300">"gpt-5.5"</span>

[<span class="text-emerald-400">model_providers.willeai</span>]
<span class="text-emerald-400">name</span> = <span class="text-amber-300">"WilleAI"</span>
<span class="text-emerald-400">base_url</span> = <span class="text-amber-300">"https://api.willeai.com"</span>
<span class="text-emerald-400">wire_api</span> = <span class="text-amber-300">"responses"</span>
<span class="text-emerald-400">experimental_bearer_token</span> = <span class="text-amber-300">"{{ t('dashboard.yourApiKey') }}"</span>
<span class="text-emerald-400">requires_openai_auth</span> = <span class="text-amber-300">true</span></pre>
          <button @click="copy('codex')" class="absolute top-3 right-3 rounded-md bg-gray-700 px-3 py-1 text-xs text-gray-300 transition-all hover:bg-gray-600 hover:text-white">
            {{ copiedTab === 'codex' ? '✓' : t('dashboard.copyConfig') }}
          </button>
        </div>
        <div class="mt-2 rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
          💡 <strong>gpt-image-2</strong>：直接用 <code class="rounded bg-amber-100 px-1 dark:bg-amber-900/40">codex --model gpt-image-2</code> 或在配置中修改 <code class="rounded bg-amber-100 px-1 dark:bg-amber-900/40">model</code> 字段即可。
        </div>
      </div>

      <!-- Hermes -->
      <div v-if="activeTab === 'hermes'">
        <p class="mb-2 text-xs text-gray-500 dark:text-dark-300">{{ t('dashboard.hermesHint') }}</p>
        <div class="relative rounded-lg bg-gray-900 p-4 font-mono text-xs leading-relaxed">
          <pre class="whitespace-pre-wrap text-gray-100"><span class="text-gray-500"># ~/.hermes/config.yaml</span>
<span class="text-emerald-400">model</span>:
  <span class="text-emerald-400">default</span>: <span class="text-amber-300">gpt-5.5</span>
  <span class="text-emerald-400">provider</span>: <span class="text-amber-300">openai</span>
  <span class="text-emerald-400">base_url</span>: <span class="text-amber-300">https://api.willeai.com/v1</span>
  <span class="text-emerald-400">api_key</span>: <span class="text-amber-300">sk-your-api-key</span></pre>
          <button @click="copy('hermes')" class="absolute top-3 right-3 rounded-md bg-gray-700 px-3 py-1 text-xs text-gray-300 transition-all hover:bg-gray-600 hover:text-white">
            {{ copiedTab === 'hermes' ? '✓' : t('dashboard.copyConfig') }}
          </button>
        </div>
        <div class="mt-2 rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
          💡 <strong>gpt-image-2</strong>：修改 <code class="rounded bg-amber-100 px-1 dark:bg-amber-900/40">model.default</code> 为 <code class="rounded bg-amber-100 px-1 dark:bg-amber-900/40">gpt-image-2</code>，或对话中临时指定模型。
        </div>
      </div>

      <!-- OpenClaw -->
      <div v-if="activeTab === 'openclaw'">
        <p class="mb-2 text-xs text-gray-500 dark:text-dark-300">{{ t('dashboard.openclawHint') }}</p>
        <div class="relative rounded-lg bg-gray-900 p-4 font-mono text-xs leading-relaxed">
          <pre class="whitespace-pre-wrap text-gray-100"><span class="text-gray-500">// ~/.openclaw/openclaw.json</span>
{
  <span class="text-emerald-400">"models"</span>: {
    <span class="text-emerald-400">"mode"</span>: <span class="text-amber-300">"merge"</span>,
    <span class="text-emerald-400">"providers"</span>: {
      <span class="text-emerald-400">"willeai"</span>: {
        <span class="text-emerald-400">"baseUrl"</span>: <span class="text-amber-300">"https://api.willeai.com/v1"</span>,
        <span class="text-emerald-400">"apiKey"</span>: <span class="text-amber-300">"sk-your-api-key"</span>,
        <span class="text-emerald-400">"api"</span>: <span class="text-amber-300">"openai-responses"</span>,
        <span class="text-emerald-400">"models"</span>: [
          { <span class="text-emerald-400">"id"</span>: <span class="text-amber-300">"gpt-5.5"</span> },
          { <span class="text-emerald-400">"id"</span>: <span class="text-amber-300">"gpt-image-2"</span> }
        ]
      }
    }
  }
}</pre>
          <button @click="copy('openclaw')" class="absolute top-3 right-3 rounded-md bg-gray-700 px-3 py-1 text-xs text-gray-300 transition-all hover:bg-gray-600 hover:text-white">
            {{ copiedTab === 'openclaw' ? '✓' : t('dashboard.copyConfig') }}
          </button>
        </div>
        <div class="mt-2 rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
          💡 <strong>gpt-image-2</strong>：已在 providers.models 中添加，使用 <code class="rounded bg-amber-100 px-1 dark:bg-amber-900/40">willeai/gpt-image-2</code> 调用。
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

const router = useRouter()
const { t } = useI18n()
const activeTab = ref('codex')
const copiedTab = ref('')

const tabs = [
  { id: 'codex', label: 'Codex CLI' },
  { id: 'hermes', label: 'Hermes' },
  { id: 'openclaw', label: 'OpenClaw' },
]

const configs: Record<string, string> = {
  codex: `model_provider = "willeai"
model = "gpt-5.5"

[model_providers.willeai]
name = "WilleAI"
base_url = "https://api.willeai.com"
wire_api = "responses"
experimental_bearer_token = "sk-your-api-key"
requires_openai_auth = true`,
  hermes: `model:
  default: gpt-5.5
  provider: openai
  base_url: https://api.willeai.com/v1
  api_key: sk-your-api-key`,
  openclaw: `{
  "models": {
    "mode": "merge",
    "providers": {
      "willeai": {
        "baseUrl": "https://api.willeai.com/v1",
        "apiKey": "sk-your-api-key",
        "api": "openai-responses",
        "models": [
          { "id": "gpt-5.5" },
          { "id": "gpt-image-2" }
        ]
      }
    }
  }
}`,
}

const copy = async (tab: string) => {
  await navigator.clipboard.writeText(configs[tab])
  copiedTab.value = tab
  setTimeout(() => { copiedTab.value = '' }, 2000)
}
</script>
