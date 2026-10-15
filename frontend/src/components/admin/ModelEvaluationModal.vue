<template>
  <BaseDialog :show="true" :title="t('admin.modelEvaluation.title')" width="wide" @close="close">
    <div class="space-y-4">
      <div class="sticky top-0 z-10 space-y-2 bg-white pb-3 dark:bg-dark-800">
        <p class="font-medium">{{ targetLabel }}</p>
        <div v-if="error" role="alert" class="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-800 dark:bg-red-950 dark:text-red-200">
          {{ error }}
        </div>
        <div v-if="historyError" role="alert" class="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-800 dark:bg-red-950 dark:text-red-200">
          {{ historyError }}
          <button type="button" class="ml-2 underline" :disabled="historyLoading" @click="loadHistory(historyPage)">{{ t('admin.modelEvaluation.refresh') }}</button>
        </div>
        <div v-if="running || state" role="status" aria-live="polite" class="rounded-lg bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-950 dark:text-blue-200">
          {{ running ? t(current ? 'admin.modelEvaluation.progress' : 'admin.modelEvaluation.creating', { current, total: batch?.rounds || rounds }) : state }}
        </div>
      </div>

      <details ref="historyPanel" class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-600" data-testid="evaluation-history">
        <summary class="cursor-pointer font-medium">{{ t('admin.modelEvaluation.history') }} ({{ historyTotal }})</summary>
        <p class="my-2 text-xs text-gray-500">{{ t('admin.modelEvaluation.historyHint') }}</p>
        <div class="mb-2 flex items-center justify-between gap-2">
          <span>{{ t('admin.modelEvaluation.historyPage', { page: historyPage }) }}</span>
          <div class="flex gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="running || historyLoading" @click="loadHistory(historyPage)">{{ t('admin.modelEvaluation.refresh') }}</button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="running || historyLoading || historyPage <= 1" @click="loadHistory(historyPage - 1)">{{ t('admin.modelEvaluation.previous') }}</button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="running || historyLoading || historyPage * 20 >= historyTotal" @click="loadHistory(historyPage + 1)">{{ t('admin.modelEvaluation.next') }}</button>
          </div>
        </div>
        <p v-if="historyLoading" class="text-gray-500">{{ t('admin.modelEvaluation.loadingHistory') }}</p>
        <p v-else-if="!history.length" class="text-gray-500">{{ t('admin.modelEvaluation.emptyHistory') }}</p>
        <ul v-else class="max-h-56 divide-y divide-gray-100 overflow-y-auto dark:divide-dark-700">
          <li v-for="item in history" :key="item.id" class="flex items-center justify-between gap-3 py-2">
            <div class="min-w-0 break-words">
              <p>{{ new Date(item.created_at).toLocaleString() }} · {{ item.model }} · {{ item.effort }}</p>
              <p class="text-xs text-gray-500">{{ t('admin.modelEvaluation.historySummary', { completed: item.completed, total: item.rounds, correct: item.correct, graded: item.graded }) }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="running || detailLoading" :data-testid="`evaluation-report-${item.id}`" @click="openReport(item.id)">{{ t('admin.modelEvaluation.viewReport') }}</button>
          </li>
        </ul>
      </details>

      <p class="text-sm text-gray-500">{{ t(target.type === 'group' ? 'admin.modelEvaluation.groupHint' : 'admin.modelEvaluation.accountHint') }}</p>
      <p class="text-sm text-gray-500">{{ t('admin.modelEvaluation.hint') }}</p>
      <div class="grid gap-3 sm:grid-cols-3">
        <label class="space-y-1 text-sm">
          <span>{{ t('admin.modelEvaluation.model') }}</span>
          <input v-model="model" list="evaluation-models" class="input w-full" maxlength="200" :disabled="running" data-testid="evaluation-model" :placeholder="t('admin.modelEvaluation.modelPlaceholder')" />
          <datalist id="evaluation-models"><option v-for="item in models" :key="item" :value="item" /></datalist>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('admin.modelEvaluation.effort') }}</span>
          <select v-model="effort" class="input w-full" :disabled="running" data-testid="evaluation-effort">
            <option v-for="item in efforts" :key="item" :value="item">{{ item }}</option>
          </select>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('admin.modelEvaluation.rounds') }}</span>
          <input v-model.number="rounds" type="number" class="input w-full" min="1" max="20" step="1" :disabled="running" data-testid="evaluation-rounds" />
        </label>
      </div>
      <p v-if="modelWarning" role="alert" class="text-sm text-amber-700 dark:text-amber-300">{{ modelWarning }}</p>
      <p class="text-xs text-gray-500">{{ t('admin.modelEvaluation.costHint') }}</p>
      <p v-if="batch" class="break-all text-xs text-gray-500">{{ t('admin.modelEvaluation.reportId') }}: {{ batch.id }} · {{ t('admin.modelEvaluation.privateReport') }}</p>

      <div v-if="rows.length" ref="resultPanel" class="scroll-mt-32 space-y-3">
        <div class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-700">
          <p class="font-medium">{{ t('admin.modelEvaluation.summary', { correct, graded, total: rows.length, errors, ungraded }) }}</p>
          <p>{{ t('admin.modelEvaluation.accuracy') }}: {{ graded ? `${(correct / graded * 100).toFixed(1)}%` : t('admin.modelEvaluation.noData') }}</p>
          <p class="mt-1">{{ t(incorrect ? 'admin.modelEvaluation.checkAgain' : 'admin.modelEvaluation.noConclusion') }}</p>
          <p v-if="batch" class="mt-1 text-xs">{{ batch.model }} · {{ batch.effort }} · {{ batch.benchmark }}</p>
          <p v-if="pending" class="mt-1 text-amber-700 dark:text-amber-300">{{ t('admin.modelEvaluation.pendingHint', { count: pending }) }}</p>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead><tr class="border-b border-gray-200 dark:border-dark-600">
              <th class="p-2">{{ t('admin.modelEvaluation.round') }}</th>
              <th class="p-2">{{ t('admin.modelEvaluation.account') }}</th>
              <th class="p-2">{{ t('admin.modelEvaluation.result') }}</th>
              <th class="p-2">{{ t('admin.modelEvaluation.tokens') }}</th>
              <th class="p-2">{{ t('admin.modelEvaluation.time') }}</th>
            </tr></thead>
            <tbody><tr v-for="row in rows" :key="row.round" class="border-b border-gray-100 dark:border-dark-700">
              <td class="p-2">{{ row.round }}</td>
              <td class="p-2">{{ row.result?.account_id ? `${row.result.account_name} #${row.result.account_id}` : '—' }}</td>
              <td class="p-2">{{ t(`admin.modelEvaluation.status.${row.result?.status || 'error'}`) }}</td>
              <td class="whitespace-nowrap p-2">{{ token(row.result?.input_tokens) }} / {{ token(row.result?.output_tokens) }} / {{ token(row.result?.reasoning_tokens) }}</td>
              <td class="p-2">{{ row.result ? `${(row.result.duration_ms / 1000).toFixed(1)}s` : '—' }}</td>
            </tr></tbody>
          </table>
        </div>
        <p class="text-xs text-gray-500">{{ t('admin.modelEvaluation.missingTokens') }}</p>
        <details v-for="row in rows" :key="row.round" class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-600">
          <summary class="cursor-pointer">{{ t('admin.modelEvaluation.detail', { round: row.round }) }} · {{ t(`admin.modelEvaluation.status.${row.result?.status || 'error'}`) }}</summary>
          <div v-if="row.result" class="mt-2 space-y-1 break-words text-xs text-gray-500">
            <p>{{ t('admin.modelEvaluation.mapping') }}: {{ row.result.requested_model }} → {{ row.result.upstream_model || '—' }}</p>
            <p>{{ t('admin.modelEvaluation.reportedModel') }}: {{ row.result.reported_model || '—' }}</p>
            <p>{{ t('admin.modelEvaluation.effort') }}: {{ row.result.requested_effort }} → {{ row.result.effective_effort || '—' }}</p>
            <p>{{ row.result.endpoint }} · {{ row.result.request_id }}</p>
          </div>
          <p v-if="row.error || row.result?.error" class="mt-2 text-red-600 dark:text-red-300">{{ row.error || row.result?.error }}</p>
          <p v-if="row.save_error" class="mt-2 text-red-600 dark:text-red-300">{{ row.save_error }}</p>
          <pre v-if="row.result?.text" class="mt-2 max-h-72 overflow-y-auto whitespace-pre-wrap break-words font-sans text-sm">{{ row.result.text }}</pre>
        </details>
      </div>
    </div>
    <template #footer>
      <div class="flex flex-wrap justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="!rows.length" @click="download">{{ t('admin.modelEvaluation.export') }}</button>
        <button v-if="running" type="button" class="btn btn-secondary" data-testid="evaluation-cancel" @click="cancel">{{ t('admin.modelEvaluation.cancel') }}</button>
        <button v-else type="button" class="btn btn-primary" :disabled="detailLoading" data-testid="evaluation-start" @click="start">{{ t('admin.modelEvaluation.start') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getAvailableModels } from '@/api/admin/accounts'
import { getModelAllowlistCandidates } from '@/api/admin/groups'
import { createEvaluation, getEvaluation, listEvaluations, runEvaluation } from '@/api/admin/modelEvaluation'
import type { EvaluationReport, EvaluationRound, EvaluationTarget } from '@/api/admin/modelEvaluation'

const props = defineProps<{ target: EvaluationTarget }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const efforts = ['low', 'medium', 'high', 'xhigh', 'max']
const model = ref('')
const models = ref<string[]>([])
const effort = ref('high')
const rounds = ref(5)
const running = ref(false)
const current = ref(0)
const error = ref('')
const modelWarning = ref('')
const state = ref('')
const rows = ref<EvaluationRound[]>([])
const batch = ref<EvaluationReport | null>(null)
const history = ref<EvaluationReport[]>([])
const historyPage = ref(1)
const historyTotal = ref(0)
const historyLoading = ref(false)
const detailLoading = ref(false)
const historyError = ref('')
const historyPanel = ref<HTMLDetailsElement | null>(null)
const resultPanel = ref<HTMLElement | null>(null)
const life = new AbortController()
let run: AbortController | null = null
const targetLabel = computed(() => `${t(`admin.modelEvaluation.${props.target.type}`)} · ${props.target.name} #${props.target.id}`)
const correct = computed(() => rows.value.filter(row => row.result?.status === 'correct').length)
const incorrect = computed(() => rows.value.filter(row => row.result?.status === 'incorrect').length)
const graded = computed(() => correct.value + incorrect.value)
const ungraded = computed(() => rows.value.filter(row => row.result?.status === 'ungraded').length)
const errors = computed(() => rows.value.filter(row => !row.result || row.result.status === 'error').length)
const pending = computed(() => rows.value.filter(row => row.result?.status === 'running' || row.result?.status === 'interrupted').length)
const token = (value?: number | null) => value == null ? '—' : String(value)
const message = (err: unknown) => (err as { message?: string })?.message || t('admin.modelEvaluation.retryHint')

async function init() {
  try {
    const catalog = props.target.type === 'account'
      ? await getAvailableModels(props.target.id).then(items => items.map(item => item.id))
      : await getModelAllowlistCandidates(props.target.id, 'openai')
    if (life.signal.aborted) return
    models.value = catalog.filter(item => !/image|audio|realtime|embedding|whisper|tts|sora|dall-e/i.test(item))
  } catch {
    if (!life.signal.aborted) modelWarning.value = `${targetLabel.value}：${t('admin.modelEvaluation.modelsFailed')}`
  }
}

async function loadHistory(page = 1) {
  if (life.signal.aborted || historyLoading.value) return
  historyLoading.value = true
  historyError.value = ''
  try {
    const data = await listEvaluations(props.target, page, life.signal)
    if (life.signal.aborted) return
    history.value = data.items
    historyPage.value = data.page
    historyTotal.value = data.total
  } catch (err) {
    if (!life.signal.aborted) historyError.value = `${targetLabel.value}：${t('admin.modelEvaluation.historyFailed')} ${message(err)}`
  } finally {
    historyLoading.value = false
  }
}

async function openReport(id: string) {
  if (running.value || detailLoading.value) return
  detailLoading.value = true
  historyError.value = ''
  try {
    const report = await getEvaluation(id, life.signal)
    if (life.signal.aborted) return
    batch.value = report
    rows.value = report.results || []
    error.value = ''
    state.value = t('admin.modelEvaluation.loadedReport', { id, completed: report.completed, total: report.rounds })
    if (historyPanel.value) historyPanel.value.open = false
    await nextTick()
    resultPanel.value?.scrollIntoView?.({ block: 'nearest' })
  } catch (err) {
    if (!life.signal.aborted) historyError.value = `${t('admin.modelEvaluation.reportId')} ${id}：${t('admin.modelEvaluation.historyFailed')} ${message(err)}`
  } finally {
    detailLoading.value = false
  }
}

async function start() {
  if (running.value || detailLoading.value || life.signal.aborted) return
  if (!model.value.trim() || !Number.isInteger(rounds.value) || rounds.value < 1 || rounds.value > 20) {
    error.value = `${targetLabel.value}：${t('admin.modelEvaluation.invalid')}`
    return
  }
  const controller = new AbortController()
  run = controller
  const selected = { model: model.value.trim(), effort: effort.value, rounds: rounds.value }
  batch.value = null
  current.value = 0
  rows.value = []
  error.value = ''
  state.value = ''
  running.value = true
  try {
    const report = await createEvaluation({ target_type: props.target.type, target_id: props.target.id, ...selected }, controller.signal)
    if (controller.signal.aborted) return
    batch.value = report
    for (let index = 1; index <= selected.rounds; index++) {
      if (controller.signal.aborted) break
      current.value = index
      try {
        const row = await runEvaluation(report.id, index, controller.signal)
        if (controller.signal.aborted) break
        rows.value.push(row)
        if (!row.saved) {
          error.value = row.save_error || t('admin.modelEvaluation.saveFailed', { id: report.id, round: index })
          break
        }
        const result = row.result
        if (!result) break
        if (result.status === 'error' || result.status === 'ungraded' || result.status === 'incorrect') {
          const object = result.account_id ? `${result.account_name} #${result.account_id}` : targetLabel.value
          const detail = result.error || t(result.status === 'ungraded' ? 'admin.modelEvaluation.ungradedHint' : 'admin.modelEvaluation.incorrectHint')
          error.value = t('admin.modelEvaluation.roundFailed', { target: object, round: index, detail })
        }
      } catch (err) {
        if (controller.signal.aborted) break
        const detail = `${message(err)} ${t('admin.modelEvaluation.retryHint')}`
        rows.value.push({ round: index, error: detail, saved: false })
        error.value = t('admin.modelEvaluation.roundFailed', { target: targetLabel.value, round: index, detail })
        break
      }
    }
  } catch (err) {
    if (!controller.signal.aborted) error.value = `${targetLabel.value}：${t('admin.modelEvaluation.createFailed')} ${message(err)}`
  } finally {
    if (!life.signal.aborted) {
      running.value = false
      state.value = t(controller.signal.aborted ? 'admin.modelEvaluation.canceled' : 'admin.modelEvaluation.finished', { completed: rows.value.length, total: selected.rounds })
    }
    if (run === controller) run = null
    if (!life.signal.aborted) void loadHistory(1)
  }
}

function cancel() { run?.abort() }
function close() { cancel(); emit('close') }
function download() {
  const report = { version: 2, id: batch.value?.id, benchmark: batch.value?.benchmark, prompt: batch.value?.prompt, source: 'https://github.com/haowang02/codex-candy-eval', target: batch.value ? { type: batch.value.target_type, id: batch.value.target_id, name: batch.value.target_name } : props.target, config: batch.value ? { model: batch.value.model, effort: batch.value.effort, rounds: batch.value.rounds, created_at: batch.value.created_at } : null, recorded: rows.value.length, correct: correct.value, graded: graded.value, ungraded: ungraded.value, errors: errors.value, pending: pending.value, results: rows.value }
  saveAs(new Blob([JSON.stringify(report, null, 2)], { type: 'application/json;charset=utf-8' }), `model-evaluation-${props.target.type}-${props.target.id}-${Date.now()}.json`)
}

onMounted(init)
onMounted(() => loadHistory())
onUnmounted(() => { life.abort(); cancel() })
</script>
