<template>
  <Modal
    :show="show"
    :title="t('keys.useKeyModal.title')"
    size="lg"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- Description -->
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('keys.useKeyModal.description') }}
      </p>

      <!-- OS Tabs -->
      <div class="border-b border-gray-200 dark:border-dark-700">
        <nav class="-mb-px flex space-x-4" aria-label="Tabs">
          <button
            v-for="tab in tabs"
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

      <!-- Code Block -->
      <div class="relative">
        <div class="bg-gray-900 dark:bg-dark-900 rounded-xl overflow-hidden">
          <!-- Code Header -->
          <div class="flex items-center justify-between px-4 py-2 bg-gray-800 dark:bg-dark-800 border-b border-gray-700 dark:border-dark-700">
            <span class="text-xs text-gray-400 font-mono">{{ activeTabConfig?.filename }}</span>
            <button
              @click="copyConfig"
              class="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg transition-colors"
              :class="copied
                ? 'bg-green-500/20 text-green-400'
                : 'bg-gray-700 hover:bg-gray-600 text-gray-300 hover:text-white'"
            >
              <svg v-if="copied" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
              </svg>
              <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
              </svg>
              {{ copied ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') }}
            </button>
          </div>
          <!-- Code Content -->
          <pre class="p-4 text-sm font-mono text-gray-100 overflow-x-auto"><code v-text="configCode"></code></pre>
        </div>
      </div>

      <!-- Usage Note -->
      <div class="flex items-start gap-3 p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-100 dark:border-blue-800">
        <svg class="w-5 h-5 text-blue-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
        </svg>
        <p class="text-sm text-blue-700 dark:text-blue-300">
          {{ t('keys.useKeyModal.note') }}
        </p>
      </div>
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
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, h } from 'vue'
import { useI18n } from 'vue-i18n'
import Modal from '@/components/common/Modal.vue'
import { useAppStore } from '@/stores/app'

interface Props {
  show: boolean
  apiKey: string
  baseUrl: string
}

interface Emits {
  (e: 'close'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const copied = ref(false)
const activeTab = ref<'unix' | 'cmd' | 'powershell'>('unix')

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

const tabs = [
  { id: 'unix' as const, label: 'macOS / Linux', icon: AppleIcon, filename: 'Terminal' },
  { id: 'cmd' as const, label: 'Windows CMD', icon: WindowsIcon, filename: 'Command Prompt' },
  { id: 'powershell' as const, label: 'PowerShell', icon: WindowsIcon, filename: 'PowerShell' }
]

const activeTabConfig = computed(() => tabs.find(tab => tab.id === activeTab.value))

// 仅输出纯文本，避免任何 HTML 注入风险。
const configCode = computed(() => {
  const baseUrl = props.baseUrl || window.location.origin
  const apiKey = props.apiKey

  switch (activeTab.value) {
    case 'unix':
      return `export ANTHROPIC_BASE_URL="${baseUrl}"
export ANTHROPIC_AUTH_TOKEN="${apiKey}"`
    case 'cmd':
      return `set ANTHROPIC_BASE_URL=${baseUrl}
set ANTHROPIC_AUTH_TOKEN=${apiKey}`
    case 'powershell':
      return `$env:ANTHROPIC_BASE_URL="${baseUrl}"
$env:ANTHROPIC_AUTH_TOKEN="${apiKey}"`
    default:
      return ''
  }
})

const copyConfig = async () => {
  try {
    await navigator.clipboard.writeText(configCode.value)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch (error) {
    appStore.showError(t('common.copyFailed'))
  }
}
</script>
