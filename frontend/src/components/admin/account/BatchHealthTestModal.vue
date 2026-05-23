<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.health.batchTitle')"
    width="extra-wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="grid gap-3 md:grid-cols-[1fr_120px_auto]">
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">
            {{ t('admin.accounts.health.model') }}
          </label>
          <input
            v-model="modelId"
            class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-100"
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">
            {{ t('admin.accounts.health.concurrency') }}
          </label>
          <input
            v-model.number="concurrency"
            type="number"
            min="1"
            max="50"
            class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-100"
          />
        </div>
        <div class="flex items-end gap-2">
          <button
            class="btn btn-primary"
            :disabled="selectedIds.length === 0 || starting"
            @click="startBatchTest"
          >
            <Icon v-if="starting" name="refresh" size="sm" class="mr-1.5 animate-spin" />
            {{ t('admin.accounts.health.startSelected', { count: selectedIds.length }) }}
          </button>
          <button class="btn btn-secondary" :disabled="loadingTasks" @click="loadTasks">
            <Icon name="refresh" size="sm" class="mr-1.5" />
            {{ t('common.refresh') }}
          </button>
        </div>
      </div>

      <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
        <input v-model="autoDisable" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        <span>{{ t('admin.accounts.health.autoDisable') }}</span>
      </label>

      <div class="grid gap-4 lg:grid-cols-[320px_1fr]">
        <div class="rounded-lg border border-gray-200 dark:border-dark-600">
          <div class="border-b border-gray-200 px-3 py-2 text-sm font-semibold text-gray-700 dark:border-dark-600 dark:text-gray-200">
            {{ t('admin.accounts.health.recentTasks') }}
          </div>
          <div class="max-h-[460px] overflow-y-auto">
            <button
              v-for="task in tasks"
              :key="task.id"
              class="block w-full border-b border-gray-100 px-3 py-3 text-left transition hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700"
              :class="{ 'bg-primary-50 dark:bg-primary-900/20': selectedTask?.id === task.id }"
              @click="selectTask(task.id)"
            >
              <div class="flex items-center justify-between gap-2">
                <span class="text-sm font-medium text-gray-900 dark:text-gray-100">#{{ task.id }}</span>
                <span :class="statusClass(task.status)">{{ statusLabel(task.status) }}</span>
              </div>
              <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ task.model_id }} · {{ task.completed_count }}/{{ task.total_count }}
              </div>
              <div class="mt-1 text-xs text-gray-400 dark:text-dark-400">
                {{ formatDate(task.created_at) }}
              </div>
            </button>
            <div v-if="tasks.length === 0" class="px-3 py-8 text-center text-sm text-gray-500">
              {{ t('admin.accounts.health.noTasks') }}
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 dark:border-dark-600">
          <div class="border-b border-gray-200 p-3 dark:border-dark-600">
            <div v-if="selectedTask" class="grid gap-2 sm:grid-cols-5">
              <Metric :label="t('admin.accounts.health.progress')" :value="`${selectedTask.completed_count}/${selectedTask.total_count}`" />
              <Metric :label="t('admin.accounts.health.success')" :value="String(selectedTask.success_count)" tone="success" />
              <Metric :label="t('admin.accounts.health.failed')" :value="String(selectedTask.failed_count)" tone="danger" />
              <Metric :label="t('admin.accounts.health.disabled')" :value="String(selectedTask.deactivated_count)" tone="warning" />
              <Metric :label="t('admin.accounts.health.status')" :value="statusLabel(selectedTask.status)" />
            </div>
            <div v-else class="text-sm text-gray-500">{{ t('admin.accounts.health.selectTask') }}</div>
          </div>

          <div class="max-h-[410px] overflow-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
              <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                <tr>
                  <th class="px-3 py-2 text-left">{{ t('admin.accounts.health.account') }}</th>
                  <th class="px-3 py-2 text-left">{{ t('admin.accounts.health.result') }}</th>
                  <th class="px-3 py-2 text-left">{{ t('admin.accounts.health.latency') }}</th>
                  <th class="px-3 py-2 text-left">{{ t('admin.accounts.health.failStreak') }}</th>
                  <th class="px-3 py-2 text-left">{{ t('admin.accounts.health.error') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="result in results" :key="result.id">
                  <td class="px-3 py-2 text-gray-900 dark:text-gray-100">
                    <div class="font-medium">{{ result.account_name || `#${result.account_id}` }}</div>
                    <div class="text-xs text-gray-500">{{ result.platform }} / {{ result.account_type }}</div>
                  </td>
                  <td class="px-3 py-2">
                    <span :class="resultClass(result.status)">{{ resultLabel(result.status) }}</span>
                    <span v-if="result.triggered_disabled" class="ml-1 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
                      {{ t('admin.accounts.health.disabled') }}
                    </span>
                  </td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ result.latency_ms }} ms</td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ result.fail_streak }}</td>
                  <td class="max-w-md px-3 py-2 text-xs text-gray-500 dark:text-gray-400">
                    <span class="line-clamp-2" :title="result.error_message">{{ result.error_message || result.response_text || '-' }}</span>
                  </td>
                </tr>
              </tbody>
            </table>
            <div v-if="selectedTask && results.length === 0" class="px-3 py-8 text-center text-sm text-gray-500">
              {{ t('admin.accounts.health.noResults') }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <button class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import type { AccountBatchTestResult, AccountBatchTestTask } from '@/types'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  show: boolean
  selectedIds: number[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'started'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const modelId = ref('gpt-5.4-mini')
const concurrency = ref(5)
const autoDisable = ref(false)
const starting = ref(false)
const loadingTasks = ref(false)
const tasks = ref<AccountBatchTestTask[]>([])
const selectedTask = ref<AccountBatchTestTask | null>(null)
const results = ref<AccountBatchTestResult[]>([])
let pollTimer: number | null = null

const isPollingTask = computed(() =>
  selectedTask.value?.status === 'pending' || selectedTask.value?.status === 'running'
)

const Metric = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    tone: { type: String, default: 'neutral' }
  },
  setup(metricProps) {
    return () => h('div', { class: 'rounded-md bg-gray-50 px-3 py-2 dark:bg-dark-700' }, [
      h('div', { class: 'text-xs text-gray-500 dark:text-gray-400' }, metricProps.label),
      h('div', {
        class: [
          'mt-0.5 text-lg font-semibold',
          metricProps.tone === 'success' ? 'text-emerald-600 dark:text-emerald-300' :
            metricProps.tone === 'danger' ? 'text-rose-600 dark:text-rose-300' :
              metricProps.tone === 'warning' ? 'text-amber-600 dark:text-amber-300' :
                'text-gray-900 dark:text-gray-100'
        ]
      }, metricProps.value)
    ])
  }
})

const statusLabel = (status: AccountBatchTestTask['status']) => t(`admin.accounts.health.taskStatus.${status}`)
const resultLabel = (status: AccountBatchTestResult['status']) => t(`admin.accounts.health.resultStatus.${status}`)

const statusClass = (status: AccountBatchTestTask['status']) => [
  'rounded px-1.5 py-0.5 text-[10px] font-medium',
  status === 'completed' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' :
    status === 'failed' ? 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300' :
      'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
]

const resultClass = (status: AccountBatchTestResult['status']) => [
  'rounded px-1.5 py-0.5 text-xs font-medium',
  status === 'success' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' :
    status === 'failed' ? 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300' :
      'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
]

const formatDate = (value: string | null) => {
  if (!value) return '-'
  return formatDateTime(new Date(value))
}

const loadTasks = async () => {
  loadingTasks.value = true
  try {
    const response = await adminAPI.accounts.listBatchTests({ limit: 30 })
    tasks.value = response.items || []
    if (!selectedTask.value && tasks.value.length > 0) {
      await selectTask(tasks.value[0].id)
    } else if (selectedTask.value) {
      const refreshed = tasks.value.find(task => task.id === selectedTask.value?.id)
      if (refreshed) selectedTask.value = refreshed
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.health.loadFailed'))
  } finally {
    loadingTasks.value = false
  }
}

const selectTask = async (id: number) => {
  try {
    const detail = await adminAPI.accounts.getBatchTest(id, { limit: 200 })
    selectedTask.value = detail.task
    results.value = detail.results || []
    updatePolling()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.health.loadFailed'))
  }
}

const startBatchTest = async () => {
  if (props.selectedIds.length === 0) return
  starting.value = true
  try {
    const task = await adminAPI.accounts.createBatchTest({
      account_ids: props.selectedIds,
      model_id: modelId.value,
      concurrency: concurrency.value,
      auto_disable: autoDisable.value
    })
    appStore.showSuccess(t('admin.accounts.health.started'))
    emit('started')
    await loadTasks()
    await selectTask(task.id)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.health.startFailed'))
  } finally {
    starting.value = false
  }
}

const clearPolling = () => {
  if (pollTimer != null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

const updatePolling = () => {
  clearPolling()
  if (!props.show || !isPollingTask.value || !selectedTask.value) return
  const taskID = selectedTask.value.id
  pollTimer = window.setInterval(async () => {
    await selectTask(taskID)
    await loadTasks()
  }, 3000)
}

watch(() => props.show, (visible) => {
  if (visible) {
    loadTasks()
  } else {
    clearPolling()
  }
})

watch(isPollingTask, updatePolling)

onUnmounted(clearPolling)
</script>
