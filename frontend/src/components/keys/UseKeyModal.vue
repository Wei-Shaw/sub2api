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
        <div v-if="clientTabs.length" class="overflow-x-auto border-b border-gray-200 dark:border-dark-700">
          <nav class="-mb-px flex min-w-max gap-4 sm:gap-6" role="tablist" :aria-label="t('keys.useKeyModal.setup.clientSelector')">
            <button
              v-for="tab in clientTabs"
              :key="tab.id"
              type="button"
              role="tab"
              :aria-selected="activeClientTab === tab.id"
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

        <!-- Codex Authentication Mode -->
        <div
          v-if="showCodexAuthMode"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
        >
          <div class="mb-2">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('keys.useKeyModal.openai.authModeTitle') }}
            </p>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('keys.useKeyModal.openai.authModeDescription') }}
            </p>
          </div>
          <div
            class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-700"
            role="radiogroup"
            :aria-label="t('keys.useKeyModal.openai.authModeTitle')"
          >
            <button
              type="button"
              role="radio"
              data-testid="codex-auth-mode-legacy"
              :aria-checked="codexAuthMode === 'legacy'"
              :class="[
                'rounded-md px-3 py-2 text-sm font-medium transition-colors',
                codexAuthMode === 'legacy'
                  ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300'
                  : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
              ]"
              @click="codexAuthMode = 'legacy'"
            >
              {{ t('keys.useKeyModal.openai.authModeLegacy') }}
            </button>
            <button
              type="button"
              role="radio"
              data-testid="codex-auth-mode-api-key"
              :aria-checked="codexAuthMode === 'api-key'"
              :class="[
                'rounded-md px-3 py-2 text-sm font-medium transition-colors',
                codexAuthMode === 'api-key'
                  ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300'
                  : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
              ]"
              @click="codexAuthMode = 'api-key'"
            >
              {{ t('keys.useKeyModal.openai.authModeApiKey') }}
            </button>
          </div>
          <p
            data-testid="codex-auth-mode-help"
            class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400"
          >
            {{ codexAuthMode === 'api-key'
              ? t('keys.useKeyModal.openai.authModeApiKeyHelp')
              : t('keys.useKeyModal.openai.authModeLegacyHelp') }}
          </p>
          <div
            v-if="codexAuthMode === 'api-key'"
            data-testid="codex-api-key-restart-notice"
            class="mt-3 flex items-start gap-2 border-l-2 border-amber-400 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:border-amber-500 dark:bg-amber-950/30 dark:text-amber-200"
          >
            <Icon name="exclamationCircle" size="sm" class="mt-0.5 flex-shrink-0" />
            <p>{{ t('keys.useKeyModal.openai.authModeApiKeyRestartNotice') }}</p>
          </div>
        </div>

        <!-- OS/Shell Tabs -->
        <div v-if="showShellTabs" class="overflow-x-auto border-b border-gray-200 dark:border-dark-700">
          <nav class="-mb-px flex min-w-max gap-4" role="tablist" :aria-label="t('keys.useKeyModal.setup.environmentSelector')">
            <button
              v-for="tab in currentTabs"
              :key="tab.id"
              type="button"
              role="tab"
              :aria-selected="activeTab === tab.id"
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

        <!-- Guided install step -->
        <section
          v-if="guidedSetup"
          data-testid="setup-step-install"
          class="rounded-xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-800/40"
        >
          <div class="flex items-start gap-3">
            <span
              class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-primary-600 text-sm font-semibold text-white"
              aria-hidden="true"
            >1</span>
            <div class="min-w-0 flex-1">
              <div class="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('keys.useKeyModal.setup.installTitle', { client: guidedSetup.clientName }) }}
                </h3>
                <a
                  :href="guidedSetup.docsUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex w-fit items-center gap-1 text-xs font-medium text-primary-600 hover:underline dark:text-primary-400"
                >
                  {{ t('keys.useKeyModal.setup.officialGuide') }}
                  <span aria-hidden="true">↗</span>
                </a>
              </div>
              <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-400">
                {{ t('keys.useKeyModal.setup.installDescription', { shell: guidedSetup.installShell }) }}
              </p>
            </div>
          </div>

          <div class="mt-3 overflow-hidden rounded-lg bg-gray-950 dark:bg-black/40">
            <div class="flex items-center justify-between border-b border-gray-800 px-3 py-2">
              <span class="text-xs font-medium text-gray-400">{{ guidedSetup.installShell }}</span>
              <button
                type="button"
                data-testid="copy-install-command"
                :aria-label="t('keys.useKeyModal.setup.copyInstallCommand')"
                class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium transition-colors"
                :class="copiedCommandID === 'install'
                  ? 'bg-green-500/20 text-green-400'
                  : 'bg-gray-800 text-gray-300 hover:bg-gray-700 hover:text-white'"
                @click="copyCommand(guidedSetup.installCommand, 'install')"
              >
                <Icon :name="copiedCommandID === 'install' ? 'check' : 'clipboard'" size="sm" />
                {{ copiedCommandID === 'install' ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
              </button>
            </div>
            <pre class="overflow-x-auto p-3 text-sm font-mono text-gray-100"><code v-text="guidedSetup.installCommand"></code></pre>
          </div>
        </section>

        <!-- Configuration step / raw blocks -->
        <section
          :data-testid="guidedSetup ? 'setup-step-configure' : undefined"
          :class="guidedSetup
            ? 'rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800/20'
            : ''"
        >
          <div v-if="guidedSetup" class="mb-4 flex items-start gap-3">
            <span
              class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-primary-600 text-sm font-semibold text-white"
              aria-hidden="true"
            >2</span>
            <div class="min-w-0 flex-1">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('keys.useKeyModal.setup.configureTitle') }}
              </h3>
              <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-400">
                {{ guidedSetup.configureDescription }}
              </p>
              <p class="mt-2 flex items-start gap-1.5 text-xs leading-5 text-amber-700 dark:text-amber-300">
                <Icon name="shield" size="sm" class="mt-0.5 flex-shrink-0" />
                {{ t('keys.useKeyModal.setup.secretWarning') }}
              </p>
            </div>
          </div>

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
                <div class="flex items-center justify-between gap-3 px-3 py-2 sm:px-4 bg-gray-800 dark:bg-dark-800 border-b border-gray-700 dark:border-dark-700">
                  <span class="min-w-0 truncate text-xs text-gray-400 font-mono" :title="file.path">{{ file.path }}</span>
                  <button
                    type="button"
                    @click="copyContent(file.content, index)"
                    class="flex flex-shrink-0 items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg transition-colors"
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
                <pre class="p-3 sm:p-4 text-sm font-mono text-gray-100 overflow-x-auto"><code v-if="file.highlighted" v-html="file.highlighted"></code><code v-else v-text="file.content"></code></pre>
              </div>
            </div>
          </div>
        </section>

        <!-- Run and verify step -->
        <section
          v-if="guidedSetup"
          data-testid="setup-step-run"
          class="rounded-xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-800/40"
        >
          <div class="flex items-start gap-3">
            <span
              class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-primary-600 text-sm font-semibold text-white"
              aria-hidden="true"
            >3</span>
            <div class="min-w-0 flex-1">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('keys.useKeyModal.setup.runTitle') }}
              </h3>
              <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-400">
                {{ t('keys.useKeyModal.setup.runDescription', { client: guidedSetup.clientName }) }}
              </p>
            </div>
          </div>

          <div class="mt-3 overflow-hidden rounded-lg bg-gray-950 dark:bg-black/40">
            <div class="flex items-center justify-between border-b border-gray-800 px-3 py-2">
              <span class="text-xs font-medium text-gray-400">{{ guidedSetup.installShell }}</span>
              <button
                type="button"
                data-testid="copy-run-command"
                :aria-label="t('keys.useKeyModal.setup.copyRunCommand')"
                class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium transition-colors"
                :class="copiedCommandID === 'run'
                  ? 'bg-green-500/20 text-green-400'
                  : 'bg-gray-800 text-gray-300 hover:bg-gray-700 hover:text-white'"
                @click="copyCommand(guidedSetup.runCommand, 'run')"
              >
                <Icon :name="copiedCommandID === 'run' ? 'check' : 'clipboard'" size="sm" />
                {{ copiedCommandID === 'run' ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
              </button>
            </div>
            <pre class="overflow-x-auto p-3 text-sm font-mono text-gray-100"><code v-text="guidedSetup.runCommand"></code></pre>
          </div>
        </section>

        <!-- Usage Note -->
        <div v-if="showPlatformNote" class="flex items-start gap-3 p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-100 dark:border-blue-800">
          <Icon name="infoCircle" size="md" class="text-blue-500 flex-shrink-0 mt-0.5" />
          <p class="text-sm text-blue-700 dark:text-blue-300">
            {{ platformNote }}
          </p>
        </div>
      </template>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, h, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import type { GroupPlatform } from '@/types'

interface Props {
  show: boolean
  apiKey: string
  baseUrl: string
  platform: GroupPlatform | null
  allowMessagesDispatch?: boolean
  platformProfileRegistry?: string
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

interface GuidedSetup {
  clientName: string
  docsUrl: string
  installCommand: string
  installShell: string
  configureDescription: string
  runCommand: string
}

interface PlatformGuideCopyBlock {
  id?: string
  client_id: string
  os?: string
  path: string
  hint?: string
  language?: string
  content_template: string
}

interface PlatformGuideClient {
  id: string
  label: string
  os?: string[]
}

interface PlatformGuideMetadata {
  profile_id?: string
  title?: string
  description?: string
  note?: string
  default_client?: string
  clients?: PlatformGuideClient[]
  copy_blocks?: PlatformGuideCopyBlock[]
}

interface PlatformProfile {
  platform: string
  guide?: PlatformGuideMetadata
}

interface PlatformProfileRegistry {
  version?: number
  profiles?: PlatformProfile[]
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const { copyToClipboard: clipboardCopy } = useClipboard()

const copiedIndex = ref<number | null>(null)
const copiedCommandID = ref<'install' | 'run' | null>(null)
const activeTab = ref<string>('unix')
const activeClientTab = ref<string>('claude')
type CodexAuthMode = 'legacy' | 'api-key'
const codexAuthMode = ref<CodexAuthMode>('legacy')
const CURRENT_CODEX_MODEL = 'gpt-5.6-sol'
const CODEX_ACTOR_AUTH_HEADER = 'x-openai-actor-authorization'
const CODEX_ACTOR_AUTH_VALUE = 'local-image-extension'
const CODEX_DOCS_URL = 'https://learn.chatgpt.com/docs/codex/cli'
const CLAUDE_CODE_DOCS_URL = 'https://docs.anthropic.com/en/docs/claude-code/setup'

const platformProfileRegistry = computed<PlatformProfileRegistry | null>(() => {
  const raw = props.platformProfileRegistry?.trim()
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as PlatformProfileRegistry
    return Array.isArray(parsed.profiles) ? parsed : null
  } catch (_error) {
    return null
  }
})

const activePlatformProfile = computed<PlatformProfile | null>(() => {
  if (!props.platform) return null
  const platform = props.platform.toLowerCase()
  return platformProfileRegistry.value?.profiles?.find((profile) =>
    profile.platform?.toLowerCase() === platform
  ) || null
})

const activePlatformGuide = computed<PlatformGuideMetadata | null>(() =>
  activePlatformProfile.value?.guide || null
)

// Reset tabs when platform/registry changes.
const defaultClientTab = computed(() => {
  const registryDefault = activePlatformGuide.value?.default_client?.trim()
  if (registryDefault) return registryDefault
  switch (props.platform) {
    case 'openai':
      return 'codex'
    case 'grok':
      return 'grok'
    case 'gemini':
      return 'gemini'
    case 'antigravity':
      return 'claude'
    default:
      return 'claude'
  }
})

watch([() => props.platform, () => props.platformProfileRegistry], () => {
  activeTab.value = 'unix'
  activeClientTab.value = defaultClientTab.value
  codexAuthMode.value = 'legacy'
}, { immediate: true })

watch(() => props.show, (show) => {
  if (show) {
    codexAuthMode.value = 'legacy'
  }
})

// Reset shell tab when client changes
watch(activeClientTab, () => {
  activeTab.value = 'unix'
})

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

const iconForClient = (clientID: string): Component =>
  clientID === 'gemini' ? SparkleIcon : TerminalIcon

const clientTabs = computed((): TabConfig[] => {
  const registryClients = activePlatformGuide.value?.clients
  if (Array.isArray(registryClients) && registryClients.length > 0) {
    return registryClients
      .filter((client) => client.id?.trim() && client.label?.trim())
      .map((client) => ({
        id: client.id,
        label: client.label,
        icon: iconForClient(client.id),
      }))
  }
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
    case 'grok':
      return [
        { id: 'grok', label: t('keys.useKeyModal.cliTabs.grokCli'), icon: TerminalIcon },
        { id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon },
        { id: 'codex', label: t('keys.useKeyModal.cliTabs.codexCli'), icon: TerminalIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ]
    default:
      return [
        { id: 'claude', label: t('keys.useKeyModal.cliTabs.claudeCode'), icon: TerminalIcon },
        { id: 'opencode', label: t('keys.useKeyModal.cliTabs.opencode'), icon: TerminalIcon }
      ]
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

const showShellTabs = computed(() => activeClientTab.value !== 'opencode')

const showCodexAuthMode = computed(() =>
  props.platform === 'openai' &&
  (activeClientTab.value === 'codex' || activeClientTab.value === 'codex-ws')
)

const currentTabs = computed(() => {
  if (!showShellTabs.value) return []
  if (activeClientTab.value === 'codex' || activeClientTab.value === 'codex-ws' || activeClientTab.value === 'grok') {
    return openaiTabs
  }
  return shellTabs
})

const platformDescription = computed(() => {
  const registryDescription = activePlatformGuide.value?.description?.trim()
  if (registryDescription) return registryDescription
  switch (props.platform) {
    case 'openai':
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.description')
      }
      return t('keys.useKeyModal.openai.description')
    case 'gemini':
      return t('keys.useKeyModal.gemini.description')
    case 'antigravity':
      return t('keys.useKeyModal.antigravity.description')
    case 'grok':
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.grok.claudeDescription')
      }
      if (activeClientTab.value === 'codex') {
        return t('keys.useKeyModal.grok.codexDescription')
      }
      return t('keys.useKeyModal.grok.description')
    default:
      return t('keys.useKeyModal.description')
  }
})

const platformNote = computed(() => {
  const registryNote = activePlatformGuide.value?.note?.trim()
  if (registryNote) return registryNote
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
    case 'grok':
      if (activeClientTab.value === 'claude') {
        return t('keys.useKeyModal.grok.claudeNote')
      }
      if (activeClientTab.value === 'codex') {
        return activeTab.value === 'windows'
          ? t('keys.useKeyModal.grok.codexNoteWindows')
          : t('keys.useKeyModal.grok.codexNote')
      }
      return activeTab.value === 'windows'
        ? t('keys.useKeyModal.grok.noteWindows')
        : t('keys.useKeyModal.grok.note')
    default:
      return t('keys.useKeyModal.note')
  }
})

const showPlatformNote = computed(() => activeClientTab.value !== 'opencode')

const guidedSetup = computed<GuidedSetup | null>(() => {
  const clientName = clientTabs.value.find((tab) => tab.id === activeClientTab.value)?.label

  if (activeClientTab.value === 'codex' || activeClientTab.value === 'codex-ws') {
    const isWindows = activeTab.value === 'windows'
    return {
      clientName: clientName || 'Codex CLI',
      docsUrl: CODEX_DOCS_URL,
      installCommand: isWindows
        ? 'irm https://chatgpt.com/codex/install.ps1 | iex'
        : 'curl -fsSL https://chatgpt.com/codex/install.sh | sh',
      installShell: isWindows
        ? t('keys.useKeyModal.setup.powerShell')
        : t('keys.useKeyModal.setup.terminal'),
      configureDescription: t('keys.useKeyModal.setup.configureCodex'),
      runCommand: 'codex --version\ncodex',
    }
  }

  if (activeClientTab.value === 'claude') {
    const installCommand = activeTab.value === 'unix'
      ? 'curl -fsSL https://claude.ai/install.sh | bash'
      : activeTab.value === 'cmd'
        ? 'powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://claude.ai/install.ps1 | iex"'
        : 'irm https://claude.ai/install.ps1 | iex'

    return {
      clientName: clientName || 'Claude Code',
      docsUrl: CLAUDE_CODE_DOCS_URL,
      installCommand,
      installShell: activeTab.value === 'unix'
        ? t('keys.useKeyModal.setup.terminal')
        : activeTab.value === 'cmd'
          ? t('keys.useKeyModal.setup.commandPrompt')
          : t('keys.useKeyModal.setup.powerShell'),
      configureDescription: t('keys.useKeyModal.setup.configureClaude'),
      runCommand: 'claude --version\nclaude',
    }
  }

  return null
})

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

const renderGuideTemplate = (template: string, values: Record<string, string>) =>
  template.replace(/\{\{\s*([a-zA-Z0-9_]+)\s*\}\}/g, (_match, key: string) =>
    values[key] ?? ''
  )

function applyCodexAuthModeToRegistryContent(content: string): string {
  if (!showCodexAuthMode.value) return content

  const lines = content.split('\n')
  const sectionStart = lines.findIndex((line) => /^\s*\[model_providers\.[^\]]+\]\s*$/.test(line))
  if (sectionStart < 0) return content

  const nextSectionOffset = lines
    .slice(sectionStart + 1)
    .findIndex((line) => /^\s*\[[^\]]+\]\s*$/.test(line))
  const sectionEnd = nextSectionOffset < 0
    ? lines.length
    : sectionStart + 1 + nextSectionOffset
  const providerLines = lines.slice(sectionStart + 1, sectionEnd)
    .filter((line) => !/^\s*requires_openai_auth\s*=/.test(line))

  const headerIndex = providerLines.findIndex((line) => /^\s*http_headers\s*=\s*\{.*\}\s*$/.test(line))
  if (headerIndex >= 0) {
    const inlineTable = providerLines[headerIndex]
      .match(/^(\s*http_headers\s*=\s*\{)(.*)(\}\s*)$/)
    if (inlineTable) {
      const entries = inlineTable[2]
        .match(/(?:"[^"]+"|[A-Za-z0-9_-]+)\s*=\s*"[^"]*"/g) || []
      const preservedEntries = entries.filter((entry) =>
        !entry.toLowerCase().startsWith(`"${CODEX_ACTOR_AUTH_HEADER}"`)
      )
      if (codexAuthMode.value === 'api-key') {
        preservedEntries.push(`"${CODEX_ACTOR_AUTH_HEADER}" = "${CODEX_ACTOR_AUTH_VALUE}"`)
      }
      if (preservedEntries.length === 0) {
        providerLines.splice(headerIndex, 1)
      } else {
        providerLines[headerIndex] = `${inlineTable[1]} ${preservedEntries.join(', ')} ${inlineTable[3]}`
      }
    }
  }

  const wireApiIndex = providerLines.findIndex((line) => /^\s*wire_api\s*=/.test(line))
  const insertAt = wireApiIndex >= 0 ? wireApiIndex + 1 : providerLines.length
  const authLines = [
    `requires_openai_auth = ${codexAuthMode.value === 'legacy' ? 'true' : 'false'}`,
  ]
  if (codexAuthMode.value === 'api-key' && headerIndex < 0) {
    authLines.push(`http_headers = { "${CODEX_ACTOR_AUTH_HEADER}" = "${CODEX_ACTOR_AUTH_VALUE}" }`)
  }
  providerLines.splice(insertAt, 0, ...authLines)

  return [
    ...lines.slice(0, sectionStart + 1),
    ...providerLines,
    ...lines.slice(sectionEnd),
  ].join('\n')
}

function registryBlocksForActiveTab(baseUrl: string, apiKey: string): FileConfig[] {
  const guide = activePlatformGuide.value
  const blocks = guide?.copy_blocks
  if (!Array.isArray(blocks) || blocks.length === 0) return []

  const baseRoot = baseUrl.replace(/\/v1\/?$/, '').replace(/\/+$/, '')
  const ensureV1 = (value: string) => {
    const trimmed = value.replace(/\/+$/, '')
    return trimmed.endsWith('/v1') ? trimmed : `${trimmed}/v1`
  }
  const ensureV1Beta = (value: string) => {
    const trimmed = value.replace(/\/+$/, '')
    return trimmed.endsWith('/v1beta') ? trimmed : `${trimmed}/v1beta`
  }
  const values: Record<string, string> = {
    base_url: baseUrl,
    base_root: baseRoot,
    api_base_url: ensureV1(baseRoot),
    api_key: apiKey,
    openai_model: CURRENT_CODEX_MODEL,
    gemini_model: 'gemini-2.0-flash',
    gemini_base_url: ensureV1Beta(baseRoot),
    antigravity_base_url: ensureV1(`${baseRoot}/antigravity`),
    antigravity_gemini_base_url: ensureV1Beta(`${baseRoot}/antigravity`),
  }

  return blocks
    .filter((block) => {
      if (block.client_id !== activeClientTab.value) return false
      if (!block.os) return true
      return block.os === activeTab.value
    })
    .map((block) => {
      const renderedContent = renderGuideTemplate(block.content_template, values)
      return {
        path: block.path,
        hint: block.hint,
        content: applyCodexAuthModeToRegistryContent(renderedContent),
      }
    })
}

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
  const registryFiles = registryBlocksForActiveTab(baseUrl, apiKey)
  if (registryFiles.length > 0) {
    return registryFiles
  }

  if (activeClientTab.value === 'opencode') {
    switch (props.platform) {
      case 'anthropic':
        return [generateOpenCodeConfig('anthropic', apiBase, apiKey)]
      case 'openai':
        return [generateOpenCodeConfig('openai', apiBase, apiKey)]
      case 'gemini':
        return [generateOpenCodeConfig('gemini', geminiBase, apiKey)]
      case 'antigravity':
        return [
          generateOpenCodeConfig('antigravity-claude', antigravityBase, apiKey, 'opencode.json (Claude)'),
          generateOpenCodeConfig('antigravity-gemini', antigravityGeminiBase, apiKey, 'opencode.json (Gemini)')
        ]
      case 'grok':
        return [generateOpenCodeConfig('grok', apiBase, apiKey)]
      default:
        return [generateOpenCodeConfig('openai', apiBase, apiKey)]
    }
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
    case 'grok':
      if (activeClientTab.value === 'claude') {
        return generateGrokClaudeFiles(baseRoot, apiKey)
      }
      if (activeClientTab.value === 'codex') {
        return generateGrokCodexFiles(apiBase, apiKey)
      }
      return generateGrokFiles(apiBase, apiKey)
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
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export CLAUDE_CODE_ATTRIBUTION_HEADER=0`
      break
    case 'cmd':
      path = 'Command Prompt'
      content = `set ANTHROPIC_BASE_URL=${baseUrl}
set ANTHROPIC_AUTH_TOKEN=${apiKey}
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
set CLAUDE_CODE_ATTRIBUTION_HEADER=0`
      break
    case 'powershell':
      path = 'PowerShell'
      content = `$env:ANTHROPIC_BASE_URL="${baseUrl}"
$env:ANTHROPIC_AUTH_TOKEN="${apiKey}"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
$env:CLAUDE_CODE_ATTRIBUTION_HEADER=0`
      break
    default:
      path = 'Terminal'
      content = ''
  }

  const vscodeSettingsPath = activeTab.value === 'unix'
    ? '~/.claude/settings.json'
    : '%USERPROFILE%\\.claude\\settings.json'

  const vscodeContent = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "env": {
    "ANTHROPIC_BASE_URL": "${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "${apiKey}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`

  return [
    { path, content },
    {
      path: vscodeSettingsPath,
      content: vscodeContent,
      hint: t('keys.useKeyModal.claudeSettingsHint')
    }
  ]
}

function generateGrokClaudeFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const environment = {
    ANTHROPIC_BASE_URL: baseUrl,
    ANTHROPIC_AUTH_TOKEN: apiKey,
    ANTHROPIC_MODEL: 'grok-4.5',
    ANTHROPIC_DEFAULT_OPUS_MODEL: 'grok-4.5',
    ANTHROPIC_DEFAULT_SONNET_MODEL: 'grok-4.5',
    ANTHROPIC_DEFAULT_HAIKU_MODEL: 'grok-4.5',
    ANTHROPIC_DEFAULT_FABLE_MODEL: 'grok-4.5',
    CLAUDE_CODE_SUBAGENT_MODEL: 'grok-4.5',
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
    CLAUDE_CODE_ATTRIBUTION_HEADER: '0'
  }
  let path: string
  let content: string

  switch (activeTab.value) {
    case 'unix':
      path = 'Terminal'
      content = Object.entries(environment)
        .map(([name, value]) => `export ${name}="${value}"`)
        .join('\n')
      break
    case 'cmd':
      path = 'Command Prompt'
      content = Object.entries(environment)
        .map(([name, value]) => `set ${name}=${value}`)
        .join('\n')
      break
    case 'powershell':
      path = 'PowerShell'
      content = Object.entries(environment)
        .map(([name, value]) => `$env:${name}="${value}"`)
        .join('\n')
      break
    default:
      path = 'Terminal'
      content = ''
  }

  const settingsPath = activeTab.value === 'unix'
    ? '~/.claude/settings.json'
    : '%USERPROFILE%\\.claude\\settings.json'

  return [
    { path, content },
    {
      path: settingsPath,
      content: JSON.stringify({
        $schema: 'https://json.schemastore.org/claude-code-settings.json',
        env: environment
      }, null, 2),
      hint: t('keys.useKeyModal.claudeSettingsHint')
    }
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

function configPath(configDir: string, fileName: string): string {
  const separator = configDir.includes('\\') ? '\\' : '/'
  return `${configDir}${separator}${fileName}`
}

function generateOpenAIFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'

  // config.toml content
  const configContent = `model_provider = "OpenAI"
model = "${CURRENT_CODEX_MODEL}"
review_model = "${CURRENT_CODEX_MODEL}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"
${generateCodexProviderAuthConfig()}

[features]
goals = true`

  // auth.json content
  const authContent = `{
  "OPENAI_API_KEY": "${apiKey}"
}`

  return [
    {
      path: configPath(configDir, 'config.toml'),
      content: configContent,
      hint: t('keys.useKeyModal.openai.configTomlHint')
    },
    {
      path: configPath(configDir, 'auth.json'),
      content: authContent
    }
  ]
}

function generateCodexProviderAuthConfig(): string {
  if (codexAuthMode.value === 'api-key') {
    return `requires_openai_auth = false
http_headers = { "x-openai-actor-authorization" = "local-image-extension" }`
  }

  return 'requires_openai_auth = true'
}

function generateGrokFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.grok' : '~/.grok'
  const configContent = `[models]
default = "grok"
web_search = "grok"

[model."grok"]
model = "grok-4.5"
base_url = "${baseUrl}"
name = "Grok 4.5"
api_key = "${apiKey}"
api_backend = "responses"
context_window = 1000000
supports_backend_search = true`

  return [{
    path: configPath(configDir, 'config.toml'),
    content: configContent,
    hint: t('keys.useKeyModal.grok.configTomlHint')
  }]
}

function generateGrokCodexFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configPath = isWindows
    ? '%USERPROFILE%\\.codex\\config.toml'
    : '~/.codex/config.toml'
  const configContent = `model_provider = "sub2api_grok"
model = "grok-4.5"
review_model = "grok-4.5"
model_reasoning_effort = "xhigh"
model_context_window = 1000000

[model_providers.sub2api_grok]
name = "Sub2API Grok"
base_url = "${baseUrl}"
env_key = "SUB2API_API_KEY"
wire_api = "responses"
supports_websockets = true

[features]
responses_websockets_v2 = true`
  const environmentContent = isWindows
    ? `$env:SUB2API_API_KEY="${apiKey}"`
    : `export SUB2API_API_KEY="${apiKey}"`

  return [
    {
      path: configPath,
      content: configContent,
      hint: t('keys.useKeyModal.grok.codexConfigTomlHint')
    },
    {
      path: isWindows ? 'PowerShell' : 'Terminal',
      content: environmentContent
    }
  ]
}

function generateOpenAIWsFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'

  // config.toml content with WebSocket v2
  const configContent = `model_provider = "OpenAI"
model = "${CURRENT_CODEX_MODEL}"
review_model = "${CURRENT_CODEX_MODEL}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"
supports_websockets = true
${generateCodexProviderAuthConfig()}

[features]
responses_websockets_v2 = true
goals = true`

  // auth.json content
  const authContent = `{
  "OPENAI_API_KEY": "${apiKey}"
}`

  return [
    {
      path: configPath(configDir, 'config.toml'),
      content: configContent,
      hint: t('keys.useKeyModal.openai.configTomlHint')
    },
    {
      path: configPath(configDir, 'auth.json'),
      content: authContent
    }
  ]
}

function generateOpenCodeConfig(platform: string, baseUrl: string, apiKey: string, pathLabel?: string): FileConfig {
  const provider: Record<string, any> = {
    [platform]: {
      options: {
        baseURL: baseUrl,
        apiKey
      }
    }
  }
  const openaiModels = {
    'gpt-5.2': {
      name: 'GPT-5.2',
      limit: {
        context: 400000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.6': {
      name: 'GPT-5.6 (Sol)',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.6-sol': {
      name: 'GPT-5.6 Sol',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.6-terra': {
      name: 'GPT-5.6 Terra',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.6-luna': {
      name: 'GPT-5.6 Luna',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
    },
    'gpt-5.5': {
      name: 'GPT-5.5',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.4': {
      name: 'GPT-5.4',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.4-mini': {
      name: 'GPT-5.4 Mini',
      limit: {
        context: 400000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.3-codex-spark': {
      name: 'GPT-5.3 Codex Spark',
      limit: {
        context: 128000,
        output: 32000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'codex-mini-latest': {
      name: 'Codex Mini',
      limit: {
        context: 200000,
        output: 100000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {}
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
    'gemini-3.5-flash': {
      name: 'Gemini 3.5 Flash',
      limit: {
        context: 1048576,
        output: 65536
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
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
    'claude-fable-5': {
      name: 'Claude Fable 5',
      limit: {
        context: 1048576,
        output: 128000
      },
      modalities: {
        input: ['text', 'image', 'pdf'],
        output: ['text']
      },
      options: {
        thinking: {
          type: 'adaptive'
        }
      }
    },
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
  const grokModels = {
    'grok-4.5': {
      name: 'Grok 4.5',
      limit: { context: 1000000, output: 128000 }
    },
    'grok-4.3': {
      name: 'Grok 4.3',
      limit: { context: 1000000, output: 128000 }
    },
    'grok-build-0.1': {
      name: 'Grok Build 0.1',
      limit: { context: 256000, output: 128000 }
    },
    'grok-composer-2.5-fast': {
      name: 'Grok Composer 2.5 Fast',
      limit: { context: 500000, output: 128000 }
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
  } else if (platform === 'grok') {
    provider[platform].npm = '@ai-sdk/openai'
    provider[platform].name = 'Grok'
    provider[platform].models = grokModels
  }

  const agent =
    platform === 'openai'
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
          }
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

const copyCommand = async (content: string, commandID: 'install' | 'run') => {
  const success = await clipboardCopy(content, t('keys.copied'))
  if (success) {
    copiedCommandID.value = commandID
    setTimeout(() => {
      if (copiedCommandID.value === commandID) {
        copiedCommandID.value = null
      }
    }, 2000)
  }
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
