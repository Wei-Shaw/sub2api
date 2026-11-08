<template>
  <section
    class="rounded-2xl bg-white p-4 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
  >
    <!-- 折叠头 -->
    <button
      type="button"
      class="flex w-full items-center justify-between gap-3 text-left"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <div class="flex min-w-0 items-center gap-2.5">
        <span
          class="inline-flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-lg bg-amber-50 text-amber-500 dark:bg-amber-900/30 dark:text-amber-400"
        >
          <Icon name="bolt" size="sm" />
        </span>
        <div class="min-w-0">
          <div class="flex items-center gap-2 text-sm font-bold text-gray-900 dark:text-white">
            {{ t('admin.channelMonitor.tuning.title') }}
            <span
              v-if="dirty"
              class="rounded-md bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
            >
              {{ t('admin.channelMonitor.tuning.unsaved') }}
            </span>
          </div>
          <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.channelMonitor.tuning.subtitle') }}
          </p>
        </div>
      </div>
      <Icon
        name="chevronDown"
        size="sm"
        class="flex-shrink-0 text-gray-400 transition-transform duration-200"
        :class="expanded ? 'rotate-180' : ''"
      />
    </button>

    <!-- 展开内容 -->
    <div v-if="expanded" class="mt-4 space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700">
      <div v-if="loading" class="py-6 text-center text-xs text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>

      <template v-else>
        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <label
            v-for="field in fields"
            :key="field.key"
            class="flex flex-col gap-1 rounded-xl bg-gray-50/70 p-3 dark:bg-dark-900/40"
          >
            <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">
              {{ t(field.labelKey) }}
            </span>
            <div class="flex items-center gap-2">
              <input
                v-model.number="form[field.key]"
                type="number"
                :min="rangeOf(field.key)[0]"
                :max="rangeOf(field.key)[1]"
                class="w-24 rounded-lg border border-gray-200 bg-white px-2 py-1.5 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800 dark:text-white"
              />
              <span class="text-xs text-gray-400">{{ field.unit }}</span>
            </div>
            <span class="text-[11px] leading-snug text-gray-500 dark:text-gray-400">
              {{ t(field.hintKey) }}
            </span>
            <span class="text-[11px] text-gray-400">
              {{ t('admin.channelMonitor.tuning.range') }}:
              {{ rangeOf(field.key)[0] }} ~ {{ rangeOf(field.key)[1] }}
            </span>
          </label>
        </div>

        <!-- 警示：并发过高会污染质量数据 -->
        <div
          v-if="form.worker_concurrency > 10"
          class="flex items-start gap-2 rounded-xl bg-amber-50 p-3 text-xs leading-relaxed text-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
        >
          <Icon name="exclamationTriangle" size="sm" class="mt-0.5 flex-shrink-0" />
          <span>{{ t('admin.channelMonitor.tuning.highConcurrencyWarning') }}</span>
        </div>

        <div class="flex flex-wrap items-center gap-2 pt-1">
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="saving || !dirty"
            @click="save"
          >
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="saving || !dirty"
            @click="reset"
          >
            {{ t('common.reset') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="saving || loading"
            @click="load"
          >
            {{ t('common.refresh') }}
          </button>
          <span v-if="message" class="text-xs" :class="messageOk ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">
            {{ message }}
          </span>
        </div>

        <p class="text-[11px] leading-relaxed text-gray-500 dark:text-gray-400">
          {{ t('admin.channelMonitor.tuning.effectiveHint') }}
        </p>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { getTuning, updateTuning, type MonitorTuning } from '@/api/admin/channelMonitor'

const { t } = useI18n()

const expanded = ref(false)
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const messageOk = ref(true)

const defaults: MonitorTuning = {
  worker_concurrency: 5,
  response_header_timeout_seconds: 30,
  idle_conn_timeout_seconds: 1800,
  max_idle_conns_per_host: 16,
  request_timeout_seconds: 45,
}

const form = reactive<MonitorTuning>({ ...defaults })
const saved = ref<MonitorTuning>({ ...defaults })
const limits = ref<Record<string, [number, number]>>({})

const fields: Array<{
  key: keyof MonitorTuning
  labelKey: string
  hintKey: string
  unit: string
}> = [
  {
    key: 'worker_concurrency',
    labelKey: 'admin.channelMonitor.tuning.workerConcurrency',
    hintKey: 'admin.channelMonitor.tuning.workerConcurrencyHint',
    unit: '',
  },
  {
    key: 'response_header_timeout_seconds',
    labelKey: 'admin.channelMonitor.tuning.responseHeaderTimeout',
    hintKey: 'admin.channelMonitor.tuning.responseHeaderTimeoutHint',
    unit: 's',
  },
  {
    key: 'idle_conn_timeout_seconds',
    labelKey: 'admin.channelMonitor.tuning.idleConnTimeout',
    hintKey: 'admin.channelMonitor.tuning.idleConnTimeoutHint',
    unit: 's',
  },
  {
    key: 'max_idle_conns_per_host',
    labelKey: 'admin.channelMonitor.tuning.maxIdleConnsPerHost',
    hintKey: 'admin.channelMonitor.tuning.maxIdleConnsPerHostHint',
    unit: '',
  },
  {
    key: 'request_timeout_seconds',
    labelKey: 'admin.channelMonitor.tuning.requestTimeout',
    hintKey: 'admin.channelMonitor.tuning.requestTimeoutHint',
    unit: 's',
  },
]

const fallbackRanges: Record<string, [number, number]> = {
  worker_concurrency: [1, 50],
  response_header_timeout_seconds: [5, 300],
  idle_conn_timeout_seconds: [30, 7200],
  max_idle_conns_per_host: [1, 200],
  request_timeout_seconds: [10, 600],
}

function rangeOf(key: keyof MonitorTuning): [number, number] {
  return limits.value[key] ?? fallbackRanges[key] ?? [1, 9999]
}

const dirty = computed(() =>
  fields.some((f) => Number(form[f.key]) !== Number(saved.value[f.key]))
)

async function load() {
  loading.value = true
  message.value = ''
  try {
    const res = await getTuning()
    const next: MonitorTuning = {
      worker_concurrency: res.worker_concurrency ?? defaults.worker_concurrency,
      response_header_timeout_seconds:
        res.response_header_timeout_seconds ?? defaults.response_header_timeout_seconds,
      idle_conn_timeout_seconds:
        res.idle_conn_timeout_seconds ?? defaults.idle_conn_timeout_seconds,
      max_idle_conns_per_host: res.max_idle_conns_per_host ?? defaults.max_idle_conns_per_host,
      request_timeout_seconds: res.request_timeout_seconds ?? defaults.request_timeout_seconds,
    }
    Object.assign(form, next)
    saved.value = { ...next }
    if (res.limits) limits.value = res.limits
  } catch (e: unknown) {
    messageOk.value = false
    message.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  message.value = ''
  try {
    const res = await updateTuning({ ...form })
    const next: MonitorTuning = {
      worker_concurrency: res.worker_concurrency,
      response_header_timeout_seconds: res.response_header_timeout_seconds,
      idle_conn_timeout_seconds: res.idle_conn_timeout_seconds,
      max_idle_conns_per_host: res.max_idle_conns_per_host,
      request_timeout_seconds: res.request_timeout_seconds,
    }
    Object.assign(form, next)
    saved.value = { ...next }
    if (res.limits) limits.value = res.limits
    messageOk.value = true
    message.value = t('admin.channelMonitor.tuning.saveSuccess')
  } catch (e: unknown) {
    messageOk.value = false
    message.value = e instanceof Error ? e.message : String(e)
  } finally {
    saving.value = false
  }
}

function reset() {
  Object.assign(form, saved.value)
  message.value = ''
}

onMounted(load)
</script>
