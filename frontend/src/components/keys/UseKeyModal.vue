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

        <!-- Code Blocks -->
        <div class="space-y-4">
          <div
            v-for="(file, index) in currentFiles"
            :key="index"
            class="relative"
          >
            <p v-if="file.hint" class="text-xs text-amber-600 dark:text-amber-400 mb-1.5 flex items-center gap-1">
              <Icon name="exclamationCircle" size="sm" class="flex-shrink-0" />
              {{ file.hint }}
            </p>
            <div class="bg-gray-900 dark:bg-dark-900 rounded-xl overflow-hidden">
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
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog } from '@sub2api/plugin-sdk'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { getPlatformCliConfig } from './platformCliConfigs'
import { usePlatforms } from '@/composables/usePlatforms'
import { getPlatformMeta } from '@/utils/platformFrontendMeta'
import {
  generateClaudeCodeFiles,
  generateGeminiCliFile,
  generateCodexFiles,
  generateOpencodeFiles,
  resolveBaseRoot,
  type FileConfig
} from './cliSnippetGenerators'
import { AppleIcon, WindowsIcon, TerminalIcon, SparkleIcon, type TabConfig } from './cliIcons'
import type { GroupPlatform } from '@/types'

interface Props {
  show: boolean
  apiKey: string
  baseUrl: string
  platform: GroupPlatform | null
  allowMessagesDispatch?: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{ (e: 'close'): void }>()

const { t } = useI18n()
const { copyToClipboard: clipboardCopy } = useClipboard()

const copiedIndex = ref<number | null>(null)
const activeTab = ref('unix')
const activeClientTab = ref('claude')

const { getPlatformDecl: getUseKeyPlatformDecl } = usePlatforms()

const cliConfig = computed(() => {
  if (!props.platform) return null
  const decl = getUseKeyPlatformDecl(props.platform)
  const meta = getPlatformMeta(decl)
  return getPlatformCliConfig(props.platform, meta.cli_config)
})

// Reset tabs when platform changes
watch(() => props.platform, () => {
  activeTab.value = 'unix'
  activeClientTab.value = cliConfig.value?.defaultClientTab ?? 'claude'
}, { immediate: true })

watch(activeClientTab, () => { activeTab.value = 'unix' })

// ---------- client tabs ----------

const clientTabs = computed((): TabConfig[] => {
  const cfg = cliConfig.value
  if (!cfg) return []
  const tabs: TabConfig[] = []
  if (cfg.codexCliSupported) {
    tabs.push({ id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon })
  }
  if (cfg.codexWsSupported) {
    tabs.push({ id: 'codex-ws', label: t('keys.useKeyModal.cliTabs.codexCliWs'), icon: TerminalIcon })
  }
  if (cfg.claudeCodeSupported && (!cfg.claudeCodeRequiresDispatch || props.allowMessagesDispatch)) {
    tabs.push({ id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon })
  }
  if (cfg.geminiCliSupported) {
    tabs.push({ id: 'gemini', label: t('keys.useKeyModal.cliTabs.geminiCli'), icon: SparkleIcon })
  }
  if (cfg.opencodeSupported) {
    tabs.push({ id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon })
  }
  return tabs
})

// ---------- shell tabs ----------

const shellTabs: TabConfig[] = [
  { id: 'unix', label: 'macOS / Linux', icon: AppleIcon },
  { id: 'cmd', label: 'Windows CMD', icon: WindowsIcon },
  { id: 'powershell', label: 'PowerShell', icon: WindowsIcon }
]
const openaiTabs: TabConfig[] = [
  { id: 'unix', label: 'macOS / Linux', icon: AppleIcon },
  { id: 'windows', label: 'Windows', icon: WindowsIcon }
]

const showShellTabs = computed(() => activeClientTab.value !== 'opencode')
const currentTabs = computed(() => {
  if (!showShellTabs.value) return []
  if (activeClientTab.value === 'codex' || activeClientTab.value === 'codex-ws') return openaiTabs
  return shellTabs
})

// ---------- description / note ----------

const platformDescription = computed(() => {
  const cfg = cliConfig.value
  if (!cfg) return ''
  const override = cfg.descriptionKeyOverrides?.[activeClientTab.value]
  return t(override ?? cfg.descriptionKey)
})

const platformNote = computed(() => {
  const cfg = cliConfig.value
  if (!cfg) return ''
  const clientOverride = cfg.noteKeyOverrides?.[activeClientTab.value]
  if (clientOverride) return t(clientOverride)
  const shellOverride = cfg.noteShellOverrides?.[activeTab.value]
  if (shellOverride) return t(shellOverride)
  return t(cfg.noteKey)
})

const showPlatformNote = computed(() => activeClientTab.value !== 'opencode')

// ---------- file generation ----------

const currentFiles = computed((): FileConfig[] => {
  const cfg = cliConfig.value
  if (!cfg) return []
  const baseUrl = props.baseUrl || window.location.origin
  const apiKey = props.apiKey
  const baseRoot = resolveBaseRoot(baseUrl)
  const tab = activeClientTab.value

  if (tab === 'opencode') {
    return generateOpencodeFiles(cfg.opencodeConfigs, baseRoot, apiKey, t('keys.useKeyModal.opencode.hint'))
  }
  if (tab === 'claude') {
    return generateClaudeCodeFiles(cfg, baseUrl, apiKey, activeTab.value)
  }
  if (tab === 'gemini') {
    return [generateGeminiCliFile(cfg, baseUrl, apiKey, activeTab.value, t('keys.useKeyModal.gemini.modelComment'))]
  }
  if (tab === 'codex' || tab === 'codex-ws') {
    const model = cfg.codexModel ?? 'gpt-5.4'
    const hint = t('keys.useKeyModal.openai.configTomlHint')
    return generateCodexFiles(baseUrl, apiKey, activeTab.value, model, tab === 'codex-ws', hint)
  }
  return []
})

// ---------- clipboard ----------

const copyContent = async (content: string, index: number) => {
  const success = await clipboardCopy(content, t('keys.copied'))
  if (success) {
    copiedIndex.value = index
    setTimeout(() => { copiedIndex.value = null }, 2000)
  }
}
</script>
