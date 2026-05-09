<template>
  <BaseDialog
    :show="show"
    :title="t('keys.useKeyModal.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- No Group Assigned Warning -->
      <div v-if="!platform" class="flex items-start gap-3 p-4 rounded-lg bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800">
        <svg class="w-5 h-5 text-yellow-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
        </svg>
        <div>
          <p class="text-sm font-medium text-yellow-800 dark:text-yellow-200">
            {{ t('keys.useKeyModal.noGroupTitle') }}
          </p>
          <p class="text-sm text-yellow-700 dark:text-yellow-300 mt-1">
            {{ t('keys.useKeyModal.noGroupDescription') }}
          </p>
        </div>
      </div>

      <!-- Platform-specific content -->
      <template v-else>
        <!-- Description -->
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ platformDescription }}
        </p>

        <!-- Client Tabs -->
        <div v-if="clientTabs.length" class="border-b border-gray-200 dark:border-dark-700">
          <nav class="-mb-px flex space-x-6" aria-label="Client">
            <button
              v-for="tab in clientTabs"
              :key="tab.id"
              @click="activeClientTab = tab.id"
              :class="[
                'whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-sm transition-colors',
                activeClientTab === tab.id
                  ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
              ]"
            >
              <span class="flex items-center gap-2">
                <component :is="tab.icon" class="w-4 h-4" />
                {{ tab.label }}
              </span>
            </button>
          </nav>
        </div>

        <!-- OS/Shell Tabs -->
        <div v-if="showShellTabs" class="border-b border-gray-200 dark:border-dark-700">
          <nav class="-mb-px flex space-x-4" aria-label="Tabs">
            <button
              v-for="tab in currentTabs"
              :key="tab.id"
              @click="activeTab = tab.id"
              :class="[
                'whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-sm transition-colors',
                activeTab === tab.id
                  ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
              ]"
            >
              <span class="flex items-center gap-2">
                <component :is="tab.icon" class="w-4 h-4" />
                {{ tab.label }}
              </span>
            </button>
          </nav>
        </div>

        <!-- Code Blocks (Stacked for multi-file platforms) -->
        <div class="space-y-4">
          <div
            v-for="(file, index) in currentFiles"
            :key="index"
            class="relative"
          >
            <!-- File Hint (if exists) -->
            <p v-if="file.hint" class="text-xs text-amber-600 dark:text-amber-400 mb-1.5 flex items-center gap-1">
              <Icon name="exclamationCircle" size="sm" class="flex-shrink-0" />
              {{ file.hint }}
            </p>
            <div class="bg-gray-900 dark:bg-dark-900 rounded-xl overflow-hidden">
              <!-- Code Header -->
              <div class="flex items-center justify-between px-4 py-2 bg-gray-800 dark:bg-dark-800 border-b border-gray-700 dark:border-dark-700">
                <span class="text-xs text-gray-400 font-mono">{{ file.path }}</span>
                <button
                  @click="copyContent(file.content, index)"
                  class="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg transition-colors"
                  :class="copiedIndex === index
                    ? 'bg-green-500/20 text-green-400'
                    : 'bg-gray-700 hover:bg-gray-600 text-gray-300 hover:text-white'"
                >
                  <svg v-if="copiedIndex === index" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                  <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
                  </svg>
                  {{ copiedIndex === index ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
                </button>
              </div>
              <!-- Code Content -->
              <pre class="p-4 text-sm font-mono text-gray-100 overflow-x-auto"><code v-if="file.highlighted" v-html="file.highlighted"></code><code v-else v-text="file.content"></code></pre>
            </div>
          </div>
        </div>

        <!-- Usage Note -->
        <div v-if="showPlatformNote" class="flex items-start gap-3 p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-100 dark:border-blue-800">
          <Icon name="infoCircle" size="md" class="text-blue-500 flex-shrink-0 mt-0.5" />
          <p class="text-sm text-blue-700 dark:text-blue-300">
            {{ platformNote }}
          </p>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button
          @click="emit('close')"
          class="btn btn-secondary"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, h, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import type { GroupPlatform } from '@/types'
import { keysAPI, type OpenCodeOpenAIModel, type OpenCodeOpenAIModelsResponse } from '@/api/keys'

interface Props {
  show: boolean
  apiKey: string
  baseUrl: string
  platform: GroupPlatform | null
  allowMessagesDispatch?: boolean
}

interface Emits {
  (e: 'close'): void
}

interface TabConfig {
  id: string
  label: string
  icon: Component
}

interface FileConfig {
  path: string
  content: string
  hint?: string  // Optional hint message for this file
  highlighted?: string
}

interface OMPModelCost {
  input?: number
  output?: number
  cacheRead?: number
  cacheWrite?: number
}

interface OMPModelConfig {
  id: string
  name: string
  api: 'openai-responses'
  reasoning: boolean
  input: Array<'text' | 'image'>
  contextWindow?: number
  maxTokens?: number
  cost?: OMPModelCost
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const { copyToClipboard: clipboardCopy } = useClipboard()

const copiedIndex = ref<number | null>(null)
const activeTab = ref<string>('unix')
const activeClientTab = ref<string>('claude')
const openCodeModels = ref<Record<string, OpenCodeOpenAIModel> | null>(null)
const openCodeMetadata = ref<OpenCodeOpenAIModelsResponse | null>(null)
const openCodeLoading = ref(false)
const openCodeError = ref<string | null>(null)

// Reset tabs when platform changes
const defaultClientTab = computed(() => {
  switch (props.platform) {
    case 'openai':
      return 'codex'
    case 'gemini':
      return 'gemini'
    case 'antigravity':
      return 'claude'
    default:
      return 'claude'
  }
})

watch(() => props.platform, () => {
  activeTab.value = 'unix'
  activeClientTab.value = defaultClientTab.value
}, { immediate: true })

// Reset shell tab when client changes
watch(activeClientTab, () => {
  activeTab.value = 'unix'
})

const loadOpenCodeModels = async () => {
  if (props.platform !== 'openai') return
  if (openCodeLoading.value) return
  const hadOpenCodeModels = Boolean(openCodeModels.value)
  const hasOMPProviderToolsMetadata = Boolean(openCodeMetadata.value?.omp_openai_provider_tools)
  if (hadOpenCodeModels && (activeClientTab.value !== 'omp' || hasOMPProviderToolsMetadata)) return

  openCodeLoading.value = true
  openCodeError.value = null
  try {
    const resp = await keysAPI.getOpenCodeOpenAIModels()
    openCodeMetadata.value = resp
    openCodeModels.value = resp.models
  } catch (err) {
    openCodeMetadata.value = null
    if (!hadOpenCodeModels) {
      openCodeModels.value = null
    }
    console.error('Failed to load OpenCode OpenAI metadata:', err)
    openCodeError.value = 'Failed to load OpenCode OpenAI metadata'
  } finally {
    openCodeLoading.value = false
  }
}

watch(
  () => [props.show, props.platform, activeClientTab.value] as const,
  ([show, platform, client]) => {
    if (!show) return
    if (platform !== 'openai') return
    if (!['opencode', 'omp'].includes(client)) return
    void loadOpenCodeModels()
  },
  { immediate: true }
)

// Icon components
const AppleIcon = {
  render() {
    return h('svg', {
      fill: 'currentColor',
      viewBox: '0 0 24 24',
      class: 'w-4 h-4'
    }, [
      h('path', { d: 'M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z' })
    ])
  }
}

const WindowsIcon = {
  render() {
    return h('svg', {
      fill: 'currentColor',
      viewBox: '0 0 24 24',
      class: 'w-4 h-4'
    }, [
      h('path', { d: 'M3 12V6.75l6-1.32v6.48L3 12zm17-9v8.75l-10 .15V5.21L20 3zM3 13l6 .09v6.81l-6-1.15V13zm7 .25l10 .15V21l-10-1.91v-5.84z' })
    ])
  }
}

// Terminal icon for Claude Code
const TerminalIcon = {
  render() {
    return h('svg', {
      fill: 'none',
      stroke: 'currentColor',
      viewBox: '0 0 24 24',
      'stroke-width': '1.5',
      class: 'w-4 h-4'
    }, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'm6.75 7.5 3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0 0 21 17.25V6.75A2.25 2.25 0 0 0 18.75 4.5H5.25A2.25 2.25 0 0 0 3 6.75v10.5A2.25 2.25 0 0 0 5.25 20.25Z'
      })
    ])
  }
}

// Sparkle icon for Gemini
const SparkleIcon = {
  render() {
    return h('svg', {
      fill: 'none',
      stroke: 'currentColor',
      viewBox: '0 0 24 24',
      'stroke-width': '1.5',
      class: 'w-4 h-4'
    }, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'M9.813 15.904 9 18.75l-.813-2.846a4.5 4.5 0 0 0-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 0 0 3.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 0 0 3.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 0 0-3.09 3.09ZM18.259 8.715 18 9.75l-.259-1.035a3.375 3.375 0 0 0-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 0 0 2.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 0 0 2.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 0 0-2.456 2.456ZM16.894 20.567 16.5 21.75l-.394-1.183a2.25 2.25 0 0 0-1.423-1.423L13.5 18.75l1.183-.394a2.25 2.25 0 0 0 1.423-1.423l.394-1.183.394 1.183a2.25 2.25 0 0 0 1.423 1.423l1.183.394-1.183.394a2.25 2.25 0 0 0-1.423 1.423Z'
      })
    ])
  }
}

const clientTabs = computed((): TabConfig[] => {
  if (!props.platform) return []
  switch (props.platform) {
    case 'openai': {
      const tabs: TabConfig[] = [
        { id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon },
        { id: 'codex-ws', label: t('keys.useKeyModal.cliTabs.codexCliWs'), icon: TerminalIcon },
      ]
      if (props.allowMessagesDispatch) {
        tabs.push({ id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon })
      }
      tabs.push({ id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon })
      tabs.push({ id: 'omp', label: t('keys.useKeyModal.cliTabs.omp'), icon: TerminalIcon })
      return tabs
    }
    case 'gemini':
      return [
        { id: 'gemini', label: t('keys.useKeyModal.cliTabs.geminiCli'), icon: SparkleIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ]
    case 'antigravity':
      return [
        { id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon },
        { id: 'gemini', label: t('keys.useKeyModal.cliTabs.geminiCli'), icon: SparkleIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ]
    default:
      return [{ id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon }]
  }
})

// Shell tabs (3 types for environment variable based configs)
const shellTabs: TabConfig[] = [
  { id: 'unix', label: 'macOS / Linux', icon: AppleIcon },
  { id: 'cmd', label: 'Windows CMD', icon: WindowsIcon },
  { id: 'powershell', label: 'PowerShell', icon: WindowsIcon }
]

// OpenAI tabs (2 OS types)
const openaiTabs: TabConfig[] = [
  { id: 'unix', label: 'macOS / Linux', icon: AppleIcon },
  { id: 'windows', label: 'Windows', icon: WindowsIcon }
]

const showShellTabs = computed(() => !['opencode', 'omp'].includes(activeClientTab.value))

const currentTabs = computed(() => {
  if (!showShellTabs.value) return []
  if (activeClientTab.value === 'codex' || activeClientTab.value === 'codex-ws') {
    return openaiTabs
  }
  return shellTabs
})

const platformDescription = computed(() => {
  switch (props.platform) {
    case 'openai':
      if (activeClientTab.value === 'omp') {
        return t('keys.useKeyModal.omp.description')
      }
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.description')
      }
      return t('keys.useKeyModal.openai.description')
    case 'gemini':
      return t('keys.useKeyModal.gemini.description')
    case 'antigravity':
      return t('keys.useKeyModal.antigravity.description')
    default:
      return t('keys.useKeyModal.description')
  }
})

const platformNote = computed(() => {
  switch (props.platform) {
    case 'openai':
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.note')
      }
      return activeTab.value === 'windows'
        ? t('keys.useKeyModal.openai.noteWindows')
        : t('keys.useKeyModal.openai.note')
    case 'gemini':
      return t('keys.useKeyModal.gemini.note')
    case 'antigravity':
      return activeClientTab.value === 'claude'
        ? t('keys.useKeyModal.antigravity.claudeNote')
        : t('keys.useKeyModal.antigravity.geminiNote')
    default:
      return t('keys.useKeyModal.note')
  }
})

const showPlatformNote = computed(() => !['opencode', 'omp'].includes(activeClientTab.value))

const escapeHtml = (value: string) => value
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;')

const wrapToken = (className: string, value: string) =>
  `<span class="${className}">${escapeHtml(value)}</span>`

const keyword = (value: string) => wrapToken('text-emerald-300', value)
const variable = (value: string) => wrapToken('text-sky-200', value)
const operator = (value: string) => wrapToken('text-slate-400', value)
const string = (value: string) => wrapToken('text-amber-200', value)
const comment = (value: string) => wrapToken('text-slate-500', value)

// Syntax highlighting helpers
// Generate file configs based on platform and active tab
const currentFiles = computed((): FileConfig[] => {
  const baseUrl = props.baseUrl || window.location.origin
  const apiKey = props.apiKey
  const baseRoot = baseUrl.replace(/\/v1\/?$/, '').replace(/\/+$/, '')
  const ensureV1 = (value: string) => {
    const trimmed = value.replace(/\/+$/, '')
    return trimmed.endsWith('/v1') ? trimmed : `${trimmed}/v1`
  }
  const apiBase = ensureV1(baseRoot)
  const antigravityBase = ensureV1(`${baseRoot}/antigravity`)
  const antigravityGeminiBase = (() => {
    const trimmed = `${baseRoot}/antigravity`.replace(/\/+$/, '')
    return trimmed.endsWith('/v1beta') ? trimmed : `${trimmed}/v1beta`
  })()
  const geminiBase = (() => {
    const trimmed = baseRoot.replace(/\/+$/, '')
    return trimmed.endsWith('/v1beta') ? trimmed : `${trimmed}/v1beta`
  })()

  if (activeClientTab.value === 'opencode') {
    switch (props.platform) {
      case 'anthropic':
        return [generateOpenCodeConfig('anthropic', apiBase, apiKey)]
      case 'openai':
        if (openCodeLoading.value) {
          return [{ path: 'opencode.json', content: '{}', hint: 'Loading OpenCode OpenAI metadata...' }]
        }
        if (openCodeModels.value) {
          return [generateOpenCodeConfig('sub2api-openai', apiBase, apiKey, undefined, openCodeModels.value)]
        }
        if (openCodeError.value) {
          return [{ path: 'opencode.json', content: '{}', hint: openCodeError.value }]
        }
        return [{ path: 'opencode.json', content: '{}', hint: 'OpenCode OpenAI metadata unavailable' }]
      case 'gemini':
        return [generateOpenCodeConfig('gemini', geminiBase, apiKey)]
      case 'antigravity':
        return [
          generateOpenCodeConfig('antigravity-claude', antigravityBase, apiKey, 'opencode.json (Claude)'),
          generateOpenCodeConfig('antigravity-gemini', antigravityGeminiBase, apiKey, 'opencode.json (Gemini)')
        ]
      default:
        return []
    }
  }

  if (activeClientTab.value === 'omp') {
    const pluginMetadata = openCodeMetadata.value?.omp_openai_provider_tools
    const pluginVersion = pluginMetadata?.latest_version?.trim() ?? ''
    const requiredOMPModelIds = ['gpt-5.5', 'gpt-5.4-mini']
    if (openCodeLoading.value) {
      return [{ path: '1. Install OMP provider tools plugin', content: '# Loading OMP metadata...', hint: t('keys.useKeyModal.omp.loadingHint') }]
    }
    if (openCodeError.value || !openCodeModels.value) {
      return [{ path: '1. Install OMP provider tools plugin', content: '# OMP metadata unavailable', hint: openCodeError.value || t('keys.useKeyModal.omp.metadataErrorHint') }]
    }
    if (!pluginMetadata || !['ok', 'cached'].includes(pluginMetadata.status) || !pluginVersion) {
      return [{ path: '1. Install OMP provider tools plugin', content: '# OMP provider tools version unavailable', hint: t('keys.useKeyModal.omp.pluginVersionErrorHint') }]
    }
    const missingOMPModelIds = requiredOMPModelIds.filter((id) => !openCodeModels.value?.[id])
    if (missingOMPModelIds.length > 0) {
      return [{ path: '1. Install OMP provider tools plugin', content: `# OMP model metadata unavailable: ${missingOMPModelIds.join(', ')}`, hint: t('keys.useKeyModal.omp.metadataErrorHint') }]
    }
    return [
      generateOMPProviderToolsPluginInstructions(pluginVersion),
      generateOMPModelsConfig(apiBase, apiKey, openCodeModels.value, pluginVersion),
      generateOMPSettingsConfig()
    ]
  }

  switch (props.platform) {
    case 'openai':
      if (activeClientTab.value === 'claude') {
        return generateAnthropicFiles(baseUrl, apiKey)
      }
      if (activeClientTab.value === 'codex-ws') {
        return generateOpenAIWsFiles(baseUrl, apiKey)
      }
      return generateOpenAIFiles(baseUrl, apiKey)
    case 'gemini':
      return [generateGeminiCliContent(baseUrl, apiKey)]
    case 'antigravity':
      if (activeClientTab.value === 'gemini') {
        return [generateGeminiCliContent(`${baseUrl}/antigravity`, apiKey)]
      }
      return generateAnthropicFiles(`${baseUrl}/antigravity`, apiKey)
    default:
      return generateAnthropicFiles(baseUrl, apiKey)
  }
})

function generateAnthropicFiles(baseUrl: string, apiKey: string): FileConfig[] {
  let path: string
  let content: string

  switch (activeTab.value) {
    case 'unix':
      path = 'Terminal'
      content = `export ANTHROPIC_BASE_URL="${baseUrl}"
export ANTHROPIC_AUTH_TOKEN="${apiKey}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
      break
    case 'cmd':
      path = 'Command Prompt'
      content = `set ANTHROPIC_BASE_URL=${baseUrl}
set ANTHROPIC_AUTH_TOKEN=${apiKey}
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
      break
    case 'powershell':
      path = 'PowerShell'
      content = `$env:ANTHROPIC_BASE_URL="${baseUrl}"
$env:ANTHROPIC_AUTH_TOKEN="${apiKey}"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
      break
    default:
      path = 'Terminal'
      content = ''
  }

  const vscodeSettingsPath = activeTab.value === 'unix'
    ? '~/.claude/settings.json'
    : '%userprofile%\\.claude\\settings.json'

  const vscodeContent = `{
  "env": {
    "ANTHROPIC_BASE_URL": "${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "${apiKey}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`

  return [
    { path, content },
    { path: vscodeSettingsPath, content: vscodeContent, hint: 'VSCode Claude Code' }
  ]
}

function generateGeminiCliContent(baseUrl: string, apiKey: string): FileConfig {
  const model = 'gemini-2.0-flash'
  const modelComment = t('keys.useKeyModal.gemini.modelComment')
  let path: string
  let content: string
  let highlighted: string

  switch (activeTab.value) {
    case 'unix':
      path = 'Terminal'
      content = `export GOOGLE_GEMINI_BASE_URL="${baseUrl}"
export GEMINI_API_KEY="${apiKey}"
export GEMINI_MODEL="${model}"  # ${modelComment}`
      highlighted = `${keyword('export')} ${variable('GOOGLE_GEMINI_BASE_URL')}${operator('=')}${string(`"${baseUrl}"`)}
${keyword('export')} ${variable('GEMINI_API_KEY')}${operator('=')}${string(`"${apiKey}"`)}
${keyword('export')} ${variable('GEMINI_MODEL')}${operator('=')}${string(`"${model}"`)}  ${comment(`# ${modelComment}`)}`
      break
    case 'cmd':
      path = 'Command Prompt'
      content = `set GOOGLE_GEMINI_BASE_URL=${baseUrl}
set GEMINI_API_KEY=${apiKey}
set GEMINI_MODEL=${model}`
      highlighted = `${keyword('set')} ${variable('GOOGLE_GEMINI_BASE_URL')}${operator('=')}${string(baseUrl)}
${keyword('set')} ${variable('GEMINI_API_KEY')}${operator('=')}${string(apiKey)}
${keyword('set')} ${variable('GEMINI_MODEL')}${operator('=')}${string(model)}
${comment(`REM ${modelComment}`)}`
      break
    case 'powershell':
      path = 'PowerShell'
      content = `$env:GOOGLE_GEMINI_BASE_URL="${baseUrl}"
$env:GEMINI_API_KEY="${apiKey}"
$env:GEMINI_MODEL="${model}"  # ${modelComment}`
      highlighted = `${keyword('$env:')}${variable('GOOGLE_GEMINI_BASE_URL')}${operator('=')}${string(`"${baseUrl}"`)}
${keyword('$env:')}${variable('GEMINI_API_KEY')}${operator('=')}${string(`"${apiKey}"`)}
${keyword('$env:')}${variable('GEMINI_MODEL')}${operator('=')}${string(`"${model}"`)}  ${comment(`# ${modelComment}`)}`
      break
    default:
      path = 'Terminal'
      content = ''
      highlighted = ''
  }

  return { path, content, highlighted }
}

function generateOpenAIFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'

  // config.toml content
  const configContent = `model_provider = "OpenAI"
model = "gpt-5.4"
review_model = "gpt-5.4"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"
requires_openai_auth = true`

  // auth.json content
  const authContent = `{
  "OPENAI_API_KEY": "${apiKey}"
}`

  return [
    {
      path: `${configDir}/config.toml`,
      content: configContent,
      hint: t('keys.useKeyModal.openai.configTomlHint')
    },
    {
      path: `${configDir}/auth.json`,
      content: authContent
    }
  ]
}

function generateOpenAIWsFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'

  // config.toml content with WebSocket v2
  const configContent = `model_provider = "OpenAI"
model = "gpt-5.4"
review_model = "gpt-5.4"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"
supports_websockets = true
requires_openai_auth = true

[features]
responses_websockets_v2 = true`

  // auth.json content
  const authContent = `{
  "OPENAI_API_KEY": "${apiKey}"
}`

  return [
    {
      path: `${configDir}/config.toml`,
      content: configContent,
      hint: t('keys.useKeyModal.openai.configTomlHint')
    },
    {
      path: `${configDir}/auth.json`,
      content: authContent
    }
  ]
}

function generateOpenCodeConfig(platform: string, baseUrl: string, apiKey: string, pathLabel?: string, openaiSource?: Record<string, OpenCodeOpenAIModel>): FileConfig {
  const provider: Record<string, any> = {
    [platform]: {
      options: {
        baseURL: baseUrl,
        apiKey
      }
    }
  }
  const buildReasoningVariants = (levels: string[]) =>
    Object.fromEntries(
      levels.map((level) => {
        const variant: Record<string, unknown> = {
          reasoningEffort: level,
          reasoningSummary: 'auto',
          include: ['reasoning.encrypted_content']
        }

        return [level, variant] as const
      })
    )

  const normalizeOpenCodeModelConfig = (model: OpenCodeOpenAIModel) => {
    const {
      id: _ignoredId,
      cost,
      experimental: _ignoredExperimental,
      provider: _ignoredProvider,
      tools: _ignoredTools,
      ...rest
    } = model as OpenCodeOpenAIModel & {
      experimental?: unknown
      provider?: unknown
      tools?: unknown
    }
    const normalizedCostEntries = Object.entries({
      input: cost?.input,
      output: cost?.output,
      cache_read: cost?.cache_read,
      cache_write: cost?.cache_write
    }).filter(([, value]) => typeof value === 'number')

    return {
      ...rest,
      ...(normalizedCostEntries.length > 0 ? { cost: Object.fromEntries(normalizedCostEntries) } : {})
    }
  }

  const openCodeBuiltinToolsForModel = (_id: string) => ({
    web_search: true
  })

  const openCodeImageGenerationBuiltinTools = () => ({
    web_search: true,
    image_generation: {
      enabled: true,
      model: 'gpt-image-2',
      output_format: 'png'
    }
  })

  const openCodeImageVariant = () => ({
    metadata: {
      builtin_tools: openCodeImageGenerationBuiltinTools()
    }
  })

  // Mirrors the upstream runtime-derived model set, not the UI custom-provider
  // form output.
  // upstream repo: anomalyco/opencode
  // file: packages/opencode/src/provider/provider.ts
  // function: fromModelsDevProvider()
  // commit: 7a6ce05
  // permalink / lines: https://github.com/anomalyco/opencode/blob/7a6ce05d0939826aa6c8e1c481489a713b2d633f/packages/opencode/src/provider/provider.ts#L1004-L1019
  // Local extensions layered after this mirror: `-Sys`, fast-id override for
  // gateway compatibility, and `options.metadata.builtin_tools`.
  const buildOpenCodeOpenAIBaseModels = (source: Record<string, OpenCodeOpenAIModel>) =>
    Object.fromEntries(
      Object.entries(source).map(([id, model]) => {
        const normalized = normalizeOpenCodeModelConfig(model)
        const normalizedOptions = normalized.options as Record<string, unknown> | undefined
        const normalizedMetadata =
          normalizedOptions?.metadata && typeof normalizedOptions.metadata === 'object' && !Array.isArray(normalizedOptions.metadata)
            ? normalizedOptions.metadata as Record<string, unknown>
            : undefined
        const finalId = id.endsWith('-fast') ? id.replace(/-fast$/, '') : id

        return [
          id,
          {
            ...normalized,
            id: finalId,
            options: {
              ...(normalizedOptions ?? {}),
              metadata: {
                ...(normalizedMetadata ?? {}),
                builtin_tools: openCodeBuiltinToolsForModel(id)
              },
              store: false
            },
            headers: normalized.headers,
            variants: buildReasoningVariants(reasoningLevels(id, model))
          }
        ]
      })
    )

  const withSysVariants = <T extends { id: string; name: string; options?: Record<string, unknown>; headers?: Record<string, unknown>; variants?: Record<string, unknown> }>(models: Record<string, T>) => {
    const expanded: Record<string, T> = {}

    for (const [id, config] of Object.entries(models)) {
      expanded[id] = config
      expanded[`${id}-Sys`] = {
        ...config,
        id: `${config.id}-Sys`,
        name: `${config.name} (Sys)`,
        ...(config.options ? { options: { ...config.options } } : {}),
        ...(config.headers ? { headers: { ...config.headers } } : {}),
        ...(config.variants ? { variants: { ...config.variants } } : {})
      }
    }

    return expanded
  }

  const reasoningLevels = (id: string, model: OpenCodeOpenAIModel) => {
    if (!model.reasoning) {
      return []
    }

    const lower = id.toLowerCase()
    if (lower === 'gpt-5-pro') {
      return []
    }

    if (['gpt-5-codex', 'gpt-5.1-codex', 'gpt-5.1-codex-max', 'gpt-5.1-codex-mini', 'codex-mini-latest'].includes(lower)) {
      return ['low', 'medium', 'high']
    }

    if (['gpt-5.3-codex-spark', 'gpt-5.3-codex', 'gpt-5.2-codex'].includes(lower)) {
      return ['low', 'medium', 'high', 'xhigh']
    }

    const levels = ['low', 'medium', 'high']
    if (lower.includes('gpt-5-') || lower === 'gpt-5') {
      levels.unshift('minimal')
    }
    if ((model.release_date ?? '') >= '2025-11-13') {
      levels.unshift('none')
    }
    if ((model.release_date ?? '') >= '2025-12-04') {
      levels.push('xhigh')
    }
    return levels
  }

  const openCodeOpenAIBaseModels = buildOpenCodeOpenAIBaseModels(openaiSource ?? {})
  const openaiModels = withSysVariants(openCodeOpenAIBaseModels)
  for (const [id, model] of Object.entries(openaiModels)) {
    if (id.toLowerCase().startsWith('gpt-5.5')) {
      model.variants = {
        ...(model.variants ?? {}),
        image: openCodeImageVariant()
      }
    }
  }
  const geminiModels = {
    'gemini-2.0-flash': {
      name: 'Gemini 2.0 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      }
    },
    'gemini-2.5-flash': {
      name: 'Gemini 2.5 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      }
    },
    'gemini-2.5-pro': {
      name: 'Gemini 2.5 Pro',
      limit: {
        context: 2097152,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3-flash-preview': {
      name: 'Gemini 3 Flash Preview',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      }
    },
    'gemini-3-pro-preview': {
      name: 'Gemini 3 Pro Preview',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.1-pro-preview': {
      name: 'Gemini 3.1 Pro Preview',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    }
  }

  const antigravityGeminiModels = {
    'gemini-2.5-flash': {
      name: 'Gemini 2.5 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'disable'
        }
      }
    },
    'gemini-2.5-flash-lite': {
      name: 'Gemini 2.5 Flash Lite',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-2.5-flash-thinking': {
      name: 'Gemini 2.5 Flash (Thinking)',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3-flash': {
      name: 'Gemini 3 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.1-pro-low': {
      name: 'Gemini 3.1 Pro Low',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.1-pro-high': {
      name: 'Gemini 3.1 Pro High',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-2.5-flash-image': {
      name: 'Gemini 2.5 Flash Image',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image'],
        output: ['image']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'gemini-3.1-flash-image': {
      name: 'Gemini 3.1 Flash Image',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image'],
        output: ['image']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    }
  }
  const claudeModels = {
    'claude-opus-4-6-thinking': {
      name: 'Claude 4.6 Opus (Thinking)',
      limit: {
        context: 200000,
        output: 128000
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    },
    'claude-sonnet-4-6': {
      name: 'Claude 4.6 Sonnet',
      limit: {
        context: 200000,
        output: 64000
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          budgetTokens: 24576,
          type: 'enabled'
        }
      }
    }
  }

  if (platform === 'gemini') {
    provider[platform].npm = '@ai-sdk/google'
    provider[platform].models = geminiModels
  } else if (platform === 'anthropic') {
    provider[platform].npm = '@ai-sdk/anthropic'
  } else if (platform === 'antigravity-claude') {
    provider[platform].npm = '@ai-sdk/anthropic'
    provider[platform].name = 'Antigravity (Claude)'
    provider[platform].models = claudeModels
  } else if (platform === 'antigravity-gemini') {
    provider[platform].npm = '@ai-sdk/google'
    provider[platform].name = 'Antigravity (Gemini)'
    provider[platform].models = antigravityGeminiModels
  } else if (platform === 'openai') {
    provider[platform].models = openaiModels
  } else if (platform === 'sub2api-openai') {
    provider[platform].npm = '@ai-sdk/openai'
    provider[platform].name = 'sub2api OpenAI'
    provider[platform].models = openaiModels
  }

  const agent =
    platform === 'openai' || platform === 'sub2api-openai'
      ? {
          build: {
            options: {
              store: false
            }
          },
          plan: {
            options: {
              store: false
            }
          },
          ...(platform === 'sub2api-openai'
            ? {
                image: {
                  mode: 'subagent',
                  description: 'Generate images with GPT-5.5 Image Fast (Sys)',
                  model: 'sub2api-openai/gpt-5.5-fast-Sys',
                  variant: 'image',
                  options: {
                    store: false
                  }
                }
              }
            : {})
        }
      : undefined

  const content = JSON.stringify(
    {
      provider,
      ...(agent ? { agent } : {}),
      $schema: 'https://opencode.ai/config.json'
    },
    null,
    2
  )

  return {
    path: pathLabel ?? 'opencode.json',
    content,
    hint: t('keys.useKeyModal.opencode.hint')
  }
}

function generateOMPProviderToolsPluginInstructions(latestVersion: string): FileConfig {
  const content = `# 1. Install or upgrade provider-native tools plugin
omp plugin install npm:omp-openai-provider-tools@${latestVersion}

# 2. Check plugin health
omp plugin doctor

# 3. Preview the recommended image subagent template
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run

# 4. After reviewing the preview, write ~/.omp/agent/agents/image-generator.md
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys

# If image_generator already exists, the command refuses to overwrite it.
# Use --print to inspect and merge manually; use --force only when you intentionally replace it.
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --print`

  return {
    path: '1. Install OMP provider tools plugin',
    content,
    hint: t('keys.useKeyModal.omp.pluginHint')
  }
}

function normalizeOMPModelConfig(model: OpenCodeOpenAIModel): OMPModelConfig {
  const allowedInput = model.modalities?.input?.filter((item): item is 'text' | 'image' => item === 'text' || item === 'image') ?? []
  const input: Array<'text' | 'image'> = allowedInput.length > 0 ? allowedInput : ['text']
  const costEntries = Object.entries({
    input: model.cost?.input,
    output: model.cost?.output,
    cacheRead: model.cost?.cache_read,
    cacheWrite: model.cost?.cache_write
  }).filter((entry): entry is [keyof OMPModelCost, number] => typeof entry[1] === 'number')

  return {
    id: model.id,
    name: model.name,
    api: 'openai-responses',
    reasoning: model.reasoning,
    input,
    contextWindow: model.limit?.context,
    maxTokens: model.limit?.output,
    ...(costEntries.length > 0 ? { cost: Object.fromEntries(costEntries) as OMPModelCost } : {})
  }
}

function withOMPSysVariants(models: Record<string, OMPModelConfig>): Record<string, OMPModelConfig> {
  const expanded: Record<string, OMPModelConfig> = {}
  for (const [id, model] of Object.entries(models)) {
    expanded[id] = {
      ...model,
      input: [...model.input],
      ...(model.cost ? { cost: { ...model.cost } } : {})
    }
    expanded[`${id}-Sys`] = {
      ...model,
      id: `${model.id}-Sys`,
      name: `${model.name} (Sys)`,
      input: [...model.input],
      ...(model.cost ? { cost: { ...model.cost } } : {})
    }
  }
  return expanded
}

function yamlScalar(value: string): string {
  return value
}

function renderOMPModelYaml(model: OMPModelConfig, indent = '      ', extraLines: string[] = []): string {
  const lines = [
    `${indent}- id: ${yamlScalar(model.id)}`,
    `${indent}  name: ${yamlScalar(model.name)}`,
    `${indent}  api: ${model.api}`,
    `${indent}  reasoning: ${model.reasoning ? 'true' : 'false'}`,
    `${indent}  input:`,
    ...model.input.map((item) => `${indent}    - ${item}`)
  ]
  if (model.contextWindow !== undefined) lines.push(`${indent}  contextWindow: ${model.contextWindow}`)
  if (model.maxTokens !== undefined) lines.push(`${indent}  maxTokens: ${model.maxTokens}`)
  const costEntries = Object.entries({
    input: model.cost?.input,
    output: model.cost?.output,
    cacheRead: model.cost?.cacheRead,
    cacheWrite: model.cost?.cacheWrite
  }).filter((entry): entry is [string, number] => typeof entry[1] === 'number')
  if (costEntries.length > 0) {
    lines.push(`${indent}  cost:`)
    for (const [key, value] of costEntries) {
      lines.push(`${indent}    ${key}: ${value}`)
    }
  }
  lines.push(...extraLines)
  return lines.join('\n')
}

function buildOMPEquivalenceOverrides(models: Record<string, OMPModelConfig>): string {
  const canonicalByModelId: Record<string, string> = {
    'gpt-5.5': 'gpt-5.5',
    'gpt-5.5-Sys': 'gpt-5.5-sys',
    'gpt-5.4-mini': 'gpt-5.4-mini',
    'gpt-5.4-mini-Sys': 'gpt-5.4-mini-sys'
  }
  const lines = Object.entries(canonicalByModelId)
    .filter(([id]) => models[id])
    .map(([id, canonical]) => `    sub2api-openai/${id}: ${canonical}`)
  lines.push('    sub2api-openai-image/gpt-5.5-Sys: gpt-5.5-image-sys')
  return lines.join('\n')
}

function generateOMPModelsConfig(baseUrl: string, apiKey: string, openaiSource: Record<string, OpenCodeOpenAIModel>, pluginVersion: string): FileConfig {
  const baseModels = Object.fromEntries(
    Object.entries(openaiSource).map(([id, model]) => [id, normalizeOMPModelConfig(model)])
  )
  const models = withOMPSysVariants(baseModels)
  const selectedIds = ['gpt-5.5', 'gpt-5.5-Sys', 'gpt-5.4-mini', 'gpt-5.4-mini-Sys'].filter((id) => models[id])
  const selectedModelYaml = selectedIds.map((id) => renderOMPModelYaml(models[id])).join('\n')
  const imageSource = models['gpt-5.5-Sys'] ?? normalizeOMPModelConfig(openaiSource['gpt-5.5'])
  const imageYaml = renderOMPModelYaml(
    { ...imageSource, id: 'gpt-5.5-Sys', name: 'GPT-5.5 Image (Sys)' },
    '      ',
    [
      '        compat:',
      '          openaiProviderTools:',
      '            imageGeneration: true'
    ]
  )

  const content = `# Image generation and provider-native web_search require this plugin:
#   omp plugin install npm:omp-openai-provider-tools@${pluginVersion}
#   omp plugin doctor
# Recommended image subagent command:
#   npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run
# Restart OMP after installing or upgrading the plugin.
providers:
  sub2api-openai:
    api: openai-responses
    baseUrl: ${baseUrl}
    apiKey: ${apiKey}
    compat:
      openaiProviderTools:
        enabled: true
    models:
${selectedModelYaml}

  sub2api-openai-image:
    api: openai-responses
    baseUrl: ${baseUrl}
    apiKey: ${apiKey}
    compat:
      openaiProviderTools:
        enabled: true
    models:
${imageYaml}

equivalence:
  overrides:
${buildOMPEquivalenceOverrides(models)}`

  return { path: '~/.omp/agent/models.yml', content, hint: t('keys.useKeyModal.omp.modelsHint') }
}

function generateOMPSettingsConfig(): FileConfig {
  const content = `defaultThinkingLevel: xhigh
serviceTier: priority

modelRoles:
  default: sub2api-openai/gpt-5.5-Sys
  slow: sub2api-openai/gpt-5.5-Sys
  smol: sub2api-openai/gpt-5.4-mini-Sys
  plan: sub2api-openai/gpt-5.5-Sys
  task: sub2api-openai/gpt-5.5-Sys:xhigh
  vision: sub2api-openai/gpt-5.5-Sys

task:
  agentModelOverrides:
    explore: sub2api-openai/gpt-5.4-mini-Sys:xhigh
    librarian: sub2api-openai/gpt-5.4-mini-Sys:xhigh
    reviewer: sub2api-openai/gpt-5.5-Sys:xhigh
    plan: sub2api-openai/gpt-5.5-Sys:xhigh`

  return { path: '~/.omp/agent/config.yml', content, hint: t('keys.useKeyModal.omp.configHint') }
}

const copyContent = async (content: string, index: number) => {
  const success = await clipboardCopy(content, t('keys.copied'))
  if (success) {
    copiedIndex.value = index
    setTimeout(() => {
      copiedIndex.value = null
    }, 2000)
  }
}
</script>
