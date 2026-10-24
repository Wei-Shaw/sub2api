<template>
  <BaseDialog
    :show="show"
    :title="t('admin.subscriptions.resetAll.title')"
    width="normal"
    :close-on-escape="phase !== 'running'"
    :show-close-button="phase !== 'running'"
    @close="handleClose"
  >
    <!-- 1) 选择窗口 -->
    <div v-if="phase === 'setup'" class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.subscriptions.resetAll.intro') }}
      </p>
      <div class="flex flex-wrap gap-5">
        <label v-for="window in quotaWindows" :key="window" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input
            v-model="windows[window]"
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
          {{ t(`admin.subscriptions.${window}`) }}
        </label>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.resetAll.scopeHint') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    </div>

    <!-- 2) 二次确认 -->
    <div v-else-if="phase === 'confirm'" class="space-y-4">
      <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-900/20 dark:text-amber-200">
        <p class="font-semibold">{{ t('admin.subscriptions.resetAll.confirmTitle', { count: matched }) }}</p>
        <p class="mt-1">{{ t('admin.subscriptions.resetAll.confirmIrreversible') }}</p>
      </div>
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.subscriptions.resetAll.windows', { windows: windowLabel }) }}
      </p>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    </div>

    <!-- 3) 执行中：真实进度条 -->
    <div v-else-if="phase === 'running'" class="space-y-4">
      <p class="text-sm text-gray-700 dark:text-gray-300">
        {{ t('admin.subscriptions.resetAll.running', { processed: processed, total: matched }) }}
      </p>
      <div class="relative h-3 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
        <div
          class="absolute inset-y-0 left-0 rounded-full bg-primary-600 transition-all duration-300"
          :style="{ width: progressPercent + '%' }"
        ></div>
      </div>
    </div>

    <!-- 4) 结果汇报 -->
    <div v-else class="space-y-3">
      <p class="rounded-lg bg-gray-50 p-3 text-sm font-medium text-gray-900 dark:bg-dark-700 dark:text-gray-100">
        {{ t('admin.subscriptions.resetAll.result', { success: successCount, failed: failedCount }) }}
      </p>
      <div v-if="failures.length" class="space-y-2">
        <p class="text-sm font-medium text-red-600 dark:text-red-400">
          {{ t('admin.subscriptions.resetAll.failuresTitle') }}
        </p>
        <ul class="max-h-48 space-y-2 overflow-y-auto">
          <li
            v-for="item in failures"
            :key="item.subscription_id"
            class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300"
          >
            <p class="font-medium">#{{ item.subscription_id }}</p>
            <p class="mt-1 break-words">{{ item.error }}</p>
          </li>
        </ul>
      </div>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          v-if="phase === 'setup' || phase === 'confirm'"
          type="button"
          class="btn btn-secondary"
          @click="phase === 'confirm' ? (phase = 'setup') : handleClose()"
        >
          {{ phase === 'confirm' ? t('admin.subscriptions.resetAll.back') : t('common.cancel') }}
        </button>
        <button v-if="phase === 'setup'" type="button" class="btn btn-primary" :disabled="!anyWindow" @click="handlePreview">
          {{ t('common.next') }}
        </button>
        <button v-else-if="phase === 'confirm'" type="button" class="btn btn-danger" :disabled="matched === 0" @click="handleExecute">
          {{ t('admin.subscriptions.resetAll.confirm') }}
        </button>
        <button
          v-else-if="phase === 'done' && failures.length"
          type="button"
          class="btn btn-secondary"
          :disabled="retrying"
          @click="handleRetry"
        >
          {{ retrying ? t('common.processing') : t('admin.subscriptions.resetAll.retryFailures') }}
        </button>
        <button v-if="phase === 'done'" type="button" class="btn btn-primary" @click="handleClose">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: []; completed: [] }>()

const { t } = useI18n()
const appStore = useAppStore()

const quotaWindows = ['daily', 'weekly', 'monthly'] as const
// 默认不勾选：全站重置影响面大，强制操作者显式选择要清零的窗口
const windows = reactive({ daily: false, weekly: false, monthly: false })
const phase = ref<'setup' | 'confirm' | 'running' | 'done'>('setup')
const matched = ref(0)
const processed = ref(0)
const successCount = ref(0)
const failedCount = ref(0)
const failures = ref<{ subscription_id: number; error: string }[]>([])
const error = ref('')
const retrying = ref(false)

const anyWindow = computed(() => windows.daily || windows.weekly || windows.monthly)
const windowLabel = computed(() =>
  quotaWindows.filter((w) => windows[w]).map((w) => t(`admin.subscriptions.${w}`)).join(' + '),
)
const progressPercent = computed(() => {
  if (matched.value <= 0) return 0
  return Math.min(100, Math.round((processed.value / matched.value) * 100))
})

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      windows.daily = false
      windows.weekly = false
      windows.monthly = false
      phase.value = 'setup'
      matched.value = 0
      processed.value = 0
      successCount.value = 0
      failedCount.value = 0
      failures.value = []
      error.value = ''
    }
  },
)

function handleClose() {
  if (phase.value === 'running') return
  const done = phase.value === 'done'
  emit('close')
  if (done) emit('completed')
}

async function handlePreview() {
  error.value = ''
  try {
    matched.value = await adminAPI.subscriptions.resetAllQuotaPreview({
      daily: windows.daily,
      weekly: windows.weekly,
      monthly: windows.monthly,
    })
    phase.value = 'confirm'
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  }
}

async function handleExecute() {
  error.value = ''
  phase.value = 'running'
  processed.value = 0
  successCount.value = 0
  failedCount.value = 0
  failures.value = []
  try {
    // 循环分批直到 remaining=0（天然幂等，可中断续跑）
    for (let guard = 0; guard < 500; guard++) {
      const res = await adminAPI.subscriptions.resetAllQuota({
        daily: windows.daily,
        weekly: windows.weekly,
        monthly: windows.monthly,
        limit: 100,
      })
      successCount.value += res.success_count
      failedCount.value += res.failed_count
      if (res.failures?.length) failures.value.push(...res.failures)
      // 进度按服务端实际处理条数累计，不按「预估数 - 剩余数」推算
      processed.value += res.processed
      if (res.remaining <= 0) break
      if (res.processed === 0) break // 防御：无可处理项但 remaining>0，避免死循环
    }
    phase.value = 'done'
  } catch (e) {
    error.value = extractApiErrorMessage(e)
    phase.value = 'done'
  }
}

async function handleRetry() {
  if (!failures.value.length) return
  retrying.value = true
  try {
    const ids = failures.value.map((f) => f.subscription_id)
    const res = await adminAPI.subscriptions.resetAllQuotaRetry(ids, {
      daily: windows.daily,
      weekly: windows.weekly,
      monthly: windows.monthly,
    })
    // 重试结果覆盖失败清单
    failures.value = res.failures ?? []
    failedCount.value = failures.value.length
    if (failures.value.length === 0) {
      appStore.showSuccess(t('admin.subscriptions.resetAll.retryDone'))
    }
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  } finally {
    retrying.value = false
  }
}
</script>
