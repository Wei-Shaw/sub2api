<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.testAccountConnection')"
    width="normal"
    @close="handleClose"
  >
    <div class="space-y-4">
      <!-- Account Info Card -->
      <div
        v-if="account"
        class="flex items-center justify-between rounded-xl border border-gray-200 bg-gradient-to-r from-gray-50 to-gray-100 p-3 dark:border-dark-500 dark:from-dark-700 dark:to-dark-600"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-primary-500 to-primary-600"
          >
            <Icon name="play" size="md" class="text-white" :stroke-width="2" />
          </div>
          <div>
            <div class="font-semibold text-gray-900 dark:text-gray-100">{{ account.name }}</div>
            <div class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
              <span
                class="rounded bg-gray-200 px-1.5 py-0.5 text-[10px] font-medium uppercase dark:bg-dark-500"
              >
                {{ account.type }}
              </span>
              <span>{{ t('admin.accounts.account') }}</span>
            </div>
          </div>
        </div>
        <span
          :class="[
            'rounded-full px-2.5 py-1 text-xs font-semibold',
            account.status === 'active'
              ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
              : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
          ]"
        >
          {{ account.status }}
        </span>
      </div>

      <!-- Test Panel: plugin-provided or default fallback -->
      <component
        v-if="resolvedTestPanel"
        :is="resolvedTestPanel"
        ref="testPanelRef"
        :context="testContext"
      />
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.close') }}
        </button>
        <button
          @click="handleStartTest"
          :disabled="panelIsRunning"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            panelIsRunning
              ? 'cursor-not-allowed bg-primary-400 text-white'
              : 'bg-primary-500 text-white hover:bg-primary-600'
          ]"
        >
          <Icon
            v-if="panelIsRunning"
            name="refresh"
            size="sm"
            class="animate-spin"
            :stroke-width="2"
          />
          <Icon v-else name="play" size="sm" :stroke-width="2" />
          <span>
            {{ panelIsRunning ? t('admin.accounts.testing') : t('admin.accounts.startTest') }}
          </span>
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { type Component, computed, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog } from '@sub2api/plugin-sdk'
import type { AccountTestExposed, SdkTestContext } from '@sub2api/plugin-sdk'
import { Icon } from '@/components/icons'
import { adminAPI } from '@/api/admin'
import { resolveTestComponent } from './test/testComponentRegistry'
import type { Account, ClaudeModel } from '@/types'

const { t } = useI18n()

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

// Resolved test panel component (plugin or default fallback)
const resolvedTestPanel = shallowRef<Component | null>(null)
const testPanelRef = ref<AccountTestExposed | null>(null)

// Host-fetched data for test context
const availableModels = ref<ClaudeModel[]>([])

// Build test context for the panel component
const testContext = computed<SdkTestContext | null>(() => {
  if (!props.account) return null
  return {
    account: {
      id: props.account.id,
      name: props.account.name,
      platform: props.account.platform,
      type: props.account.type,
      credentials: props.account.credentials,
      extra: props.account.extra as Record<string, unknown> | undefined,
      proxy_id: props.account.proxy_id,
    },
    hostData: {
      availableModels: availableModels.value,
    },
  }
})

// Read panel state via template ref (Vue unwraps exposed refs)
const panelIsRunning = computed(() => testPanelRef.value?.isRunning ?? false)

const loadAvailableModels = async () => {
  if (!props.account) return
  try {
    availableModels.value = await adminAPI.accounts.getAvailableModels(props.account.id)
  } catch (error) {
    console.error('Failed to load available models:', error)
    availableModels.value = []
  }
}

// Resolve test panel + fetch models when modal opens
watch(
  () => props.show,
  async (newVal) => {
    if (!newVal || !props.account) {
      return
    }
    // Fetch models and resolve panel in parallel
    const [, panel] = await Promise.all([
      loadAvailableModels(),
      resolveTestComponent(props.account.platform),
    ])
    if (panel) {
      resolvedTestPanel.value = panel
    } else {
      // Fallback to built-in DefaultTestPanel
      const mod = await import('./test/DefaultTestPanel.vue')
      resolvedTestPanel.value = mod.default
    }
  },
)

const handleStartTest = () => {
  testPanelRef.value?.startTest()
}

const handleClose = () => {
  testPanelRef.value?.abort()
  emit('close')
}
</script>

<style>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
