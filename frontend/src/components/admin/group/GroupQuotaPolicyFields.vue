<template>
  <section class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-400">
    <button type="button" class="flex w-full items-center justify-between gap-3 text-left" data-testid="quota-toggle" :aria-expanded="expanded" @click="toggle">
      <span>
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('quotaPolicy.title') }}</span>
        <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('quotaPolicy.hint') }}</span>
      </span>
      <Icon :name="expanded ? 'chevronDown' : 'chevronRight'" size="sm" />
    </button>

    <div v-if="expanded" class="mt-4 space-y-4">
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('quotaPolicy.scope') }}</p>
      <p class="rounded-lg bg-primary-50 p-3 text-xs text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">{{ t('quotaPolicy.independentSave') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400" data-testid="quota-error">{{ error }}</p>
      <p v-if="notice" role="status" class="text-sm text-primary-600 dark:text-primary-300">{{ notice }}</p>
      <p v-if="busy && loaded" role="status" class="text-xs text-gray-500 dark:text-gray-400">{{ t('quotaPolicy.saving') }}</p>
      <div v-if="!loaded">
        <span v-if="busy" class="text-sm text-gray-500">{{ t('quotaPolicy.saving') }}</span>
        <button v-else type="button" class="btn btn-secondary" data-testid="quota-retry" @click="load">{{ t('quotaPolicy.retry') }}</button>
      </div>

      <fieldset v-else :disabled="busy" class="min-w-0 space-y-4">
        <p v-if="!draft.rules.length" class="rounded-lg border border-dashed border-gray-200 p-4 text-sm text-gray-500 dark:border-dark-500">{{ t('quotaPolicy.empty') }}</p>
        <div v-for="(rule, ruleIndex) in draft.rules" :key="rule.id" class="space-y-3 rounded-lg border border-gray-200 bg-gray-50/40 p-3 dark:border-dark-600 dark:bg-dark-800/40" data-testid="quota-rule">
          <div class="flex items-center justify-between gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ rule.name || `${t('quotaPolicy.name')} ${ruleIndex + 1}` }}</span>
            <div class="flex gap-3">
              <button type="button" class="text-xs text-primary-600 dark:text-primary-400" :disabled="draft.rules.length >= 20" @click="copyRule(rule)">{{ t('quotaPolicy.copy') }}</button>
              <button type="button" class="text-xs text-red-600 dark:text-red-400" @click="draft.rules.splice(ruleIndex, 1)">{{ t('quotaPolicy.remove') }}</button>
            </div>
          </div>
          <div class="grid gap-3 sm:grid-cols-3">
            <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.name') }}<input v-model="rule.name" maxlength="80" class="input mt-1 text-sm" data-testid="quota-name" /></label>
            <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.priority') }}<input v-model.number="rule.priority" type="number" min="-10000" max="10000" step="1" class="input mt-1 text-sm" /></label>
            <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.mode') }}<select v-model="rule.mode" class="input mt-1 text-sm" data-testid="quota-mode"><option value="disabled">{{ t('quotaPolicy.disabled') }}</option><option value="observe">{{ t('quotaPolicy.observe') }}</option><option value="enforce">{{ t('quotaPolicy.enforce') }}</option></select></label>
          </div>

          <div>
            <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.accounts') }}<input v-model="searches[rule.id]" class="input mt-1 text-sm" :placeholder="t('quotaPolicy.accountSearch')" /></label>
            <div class="mt-2 flex flex-wrap gap-1.5">
              <span v-for="id in rule.account_ids" :key="id" class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
                {{ accountLabel(id) }}<button type="button" :aria-label="`${t('quotaPolicy.remove')} ${accountLabel(id)}`" @click="rule.account_ids = rule.account_ids.filter(value => value !== id)"><Icon name="x" size="xs" /></button>
              </span>
            </div>
            <div class="mt-2 max-h-32 overflow-y-auto rounded-lg border border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-800">
              <p v-if="!accounts.length" class="text-xs text-gray-500">{{ t('quotaPolicy.noAccounts') }}</p>
              <label v-for="account in filteredAccounts(rule.id)" :key="account.id" class="flex cursor-pointer items-center gap-2 py-1 text-xs text-gray-700 dark:text-gray-300">
                <input v-model="rule.account_ids" type="checkbox" :value="account.id" :disabled="!rule.account_ids.includes(account.id) && rule.account_ids.length >= 50" :data-testid="`quota-account-${account.id}`" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                {{ account.name }} <span class="text-gray-400">#{{ account.id }}</span>
              </label>
            </div>
          </div>
          <div class="grid gap-3 sm:grid-cols-2">
            <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.accountMatch') }}<select v-model="rule.account_match" class="input mt-1 text-sm" data-testid="quota-account-match"><option value="any">{{ t('quotaPolicy.anyAccount') }}</option><option value="all">{{ t('quotaPolicy.allAccounts') }}</option></select></label>
            <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.windowMatch') }}<select v-model="rule.window_match" class="input mt-1 text-sm"><option value="any">{{ t('quotaPolicy.anyWindow') }}</option><option value="all">{{ t('quotaPolicy.allWindows') }}</option></select></label>
          </div>
          <div class="space-y-2">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.windows') }}</span>
            <div class="flex gap-4">
              <label v-for="window in quotaWindows" :key="window" class="flex items-center gap-2 text-xs text-gray-700 dark:text-gray-300"><input type="checkbox" :checked="rule.conditions.some(condition => condition.window === window)" :data-testid="`quota-window-${window}`" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" @change="toggleWindow(rule, window)" />{{ windowLabel(window) }}</label>
            </div>
            <div v-for="condition in rule.conditions" :key="condition.window" class="grid items-end gap-2 sm:grid-cols-[4rem_1fr_1fr]">
              <span class="pb-2 text-xs text-gray-500">{{ windowLabel(condition.window) }}</span>
              <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.trigger') }}<input v-model.number="condition.trigger_remaining" type="number" min="0" max="100" step="any" class="input mt-1 text-sm" data-testid="quota-trigger" /></label>
              <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.recover') }}<input v-model.number="condition.recover_remaining" type="number" min="0" max="100" step="any" class="input mt-1 text-sm" data-testid="quota-recover" /></label>
            </div>
          </div>
          <label class="block text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.recovery') }}<select v-model="rule.recovery" class="input mt-1 text-sm"><option value="auto">{{ t('quotaPolicy.auto') }}</option><option value="manual">{{ t('quotaPolicy.manual') }}</option></select></label>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('quotaPolicy.recoveryHint') }}</p>
          <div class="space-y-2">
            <div class="flex items-center justify-between gap-2">
              <span class="text-xs text-gray-600 dark:text-gray-300">{{ t('quotaPolicy.mappings') }}</span>
              <button type="button" class="text-xs text-primary-600 disabled:opacity-50 dark:text-primary-400" :disabled="modelsLoading" @click="loadModels">{{ t(modelsLoading ? 'quotaPolicy.modelsLoading' : 'quotaPolicy.refreshModels') }}</button>
            </div>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('quotaPolicy.modelCheckHint') }}</p>
            <p v-if="modelsError" class="text-xs text-amber-700 dark:text-amber-400" role="status">{{ modelsError }}</p>
            <p v-for="warning in modelCatalog?.warnings" :key="warning" class="text-xs text-amber-700 dark:text-amber-400">{{ warning }}</p>
            <GroupQuotaMappingRow v-for="(mapping, index) in rule.mappings" :key="mappingKey(mapping)" v-model:source="mapping.source" v-model:target="mapping.target" :group-id="groupId" :account-ids="rule.account_ids" :catalog="modelCatalog" :disabled="busy" :duplicate-source="rule.mappings.some((other, otherIndex) => otherIndex !== index && !!mapping.source && other.source === mapping.source)" @remove="rule.mappings.splice(index, 1)" />
            <button type="button" class="text-xs text-primary-600 dark:text-primary-400" :disabled="rule.mappings.length >= 50" @click="rule.mappings.push({ source: '', target: '' })">+ {{ t('quotaPolicy.addMapping') }}</button>
          </div>
        </div>
        <button type="button" class="btn btn-secondary text-sm" data-testid="quota-add" :disabled="draft.rules.length >= 20" @click="addRule">+ {{ t('quotaPolicy.add') }}</button>

        <div class="flex flex-wrap items-center gap-3 border-t border-gray-200 pt-3 dark:border-dark-600">
          <button type="button" class="btn btn-primary text-sm" data-testid="quota-save" :disabled="!dirty" @click="save">{{ t('quotaPolicy.save') }}</button>
          <span v-if="dirty" class="text-xs text-amber-600 dark:text-amber-400">{{ t('quotaPolicy.dirty') }}</span>
          <button v-if="conflicted" type="button" class="text-xs text-primary-600 dark:text-primary-400" data-testid="quota-reload" @click="load">{{ t('quotaPolicy.reload') }}</button>
        </div>
        <div class="space-y-2">
          <div class="flex gap-2"><input v-model="previewModel" class="input min-w-0 flex-1 text-sm" :placeholder="t('quotaPolicy.previewModel')" :aria-label="t('quotaPolicy.previewModel')" data-testid="quota-preview-model" /><button type="button" class="btn btn-secondary shrink-0 text-sm" data-testid="quota-preview" @click="preview">{{ t('quotaPolicy.preview') }}</button></div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('quotaPolicy.previewHint') }}</p>
          <p v-if="previewResult" role="status" class="rounded-lg bg-primary-50 p-3 text-xs text-primary-700 dark:bg-primary-900/20 dark:text-primary-300" data-testid="quota-preview-result">{{ previewResult.resolution.applied ? t('quotaPolicy.previewApplied', { name: previewResult.resolution.rule_name, source: previewResult.resolution.source_model, target: previewResult.resolution.target_model }) : t('quotaPolicy.previewUnchanged', { model: previewResult.resolution.source_model }) }}</p>
        </div>

        <div class="space-y-2 border-t border-gray-200 pt-3 dark:border-dark-600">
          <div class="flex items-center justify-between gap-2"><span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('quotaPolicy.runtime') }}</span><button type="button" class="btn btn-secondary text-xs" data-testid="quota-refresh" @click="refresh">{{ t('quotaPolicy.refresh') }}</button></div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('quotaPolicy.freshness') }}</p>
          <div class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead class="text-gray-500"><tr><th class="p-2 font-medium">{{ t('quotaPolicy.account') }}</th><th class="p-2 font-medium">{{ t('quotaPolicy.fiveHour') }}</th><th class="p-2 font-medium">{{ t('quotaPolicy.sevenDay') }}</th><th class="p-2 font-medium">{{ t('quotaPolicy.sampledAt') }}</th></tr></thead>
              <tbody class="text-gray-700 dark:text-gray-300"><tr v-for="sample in runtime.snapshots" :key="sample.account_id" class="border-t border-gray-100 dark:border-dark-600"><td class="p-2">{{ accountLabel(sample.account_id) }}<span v-if="sampleUnknown(sample)" class="block text-amber-600">{{ t('quotaPolicy.unknown') }}<span v-if="sample.error_code"> ({{ sample.error_code }})</span></span></td><td class="p-2">{{ percent(sample.five_hour?.remaining_percent) }}</td><td class="p-2">{{ percent(sample.seven_day?.remaining_percent) }}</td><td class="whitespace-nowrap p-2">{{ formatTime(sample.sampled_at) }}</td></tr></tbody>
            </table>
            <p v-if="!runtime.snapshots.length" class="p-2 text-xs text-gray-400">{{ t('quotaPolicy.noSnapshots') }}</p>
          </div>
          <div v-for="state in runtime.rules" :key="state.rule_id" class="flex items-center justify-between gap-3 rounded-lg bg-gray-50 p-2 text-xs dark:bg-dark-800">
            <span class="text-gray-700 dark:text-gray-300">{{ ruleLabel(state.rule_id) }} · {{ statusLabel(state.status) }}</span>
            <button v-if="state.active && !state.suppressed" type="button" class="text-primary-600 dark:text-primary-400" :disabled="dirty" :title="dirty ? t('quotaPolicy.releaseHint') : undefined" data-testid="quota-release" @click="release(state.rule_id)">{{ t('quotaPolicy.release') }}</button>
          </div>
        </div>
        <details class="text-xs text-gray-500 dark:text-gray-400">
          <summary class="cursor-pointer">{{ t('quotaPolicy.events') }}</summary>
          <p v-if="!runtime.events.length" class="mt-2">{{ t('quotaPolicy.noEvents') }}</p>
          <ul v-else class="mt-2 max-h-40 space-y-2 overflow-y-auto"><li v-for="(event, index) in runtime.events" :key="index"><span>{{ formatTime(event.at) }} · {{ ruleLabel(event.rule_id) }} · </span>{{ event.message }}</li></ul>
        </details>
      </fieldset>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GroupQuotaMappingRow from './GroupQuotaMappingRow.vue'
import * as policyAPI from '@/api/admin/groupQuotaPolicy'
import type { QuotaCondition, QuotaModelCatalog, QuotaPolicyAccount, QuotaPolicyConfig, QuotaPolicyPreview, QuotaPolicyRule, QuotaPolicyRuntime, QuotaPolicySnapshot, QuotaPolicyView } from '@/api/admin/groupQuotaPolicy'

type DraftRule = Omit<QuotaPolicyRule, 'conditions'> & { conditions: { window: '5h' | '7d'; trigger_remaining: number | ''; recover_remaining: number | '' }[] }
const props = defineProps<{ groupId: number }>()
const { t } = useI18n()
const expanded = ref(false)
const loaded = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const conflicted = ref(false)
const revision = ref(0)
const draft = ref<{ rules: DraftRule[] }>({ rules: [] })
const savedJSON = ref('')
const accounts = ref<QuotaPolicyAccount[]>([])
const runtime = ref<QuotaPolicyRuntime>({ snapshots: [], rules: [], events: [] })
const savedNames = ref<Record<string, string>>({})
const searches = ref<Record<string, string>>({})
const previewModel = ref('')
const previewResult = ref<QuotaPolicyPreview | null>(null)
const modelCatalog = ref<QuotaModelCatalog | null>(null)
const modelsLoading = ref(false)
const modelsError = ref('')
let modelsController: AbortController | undefined
const mappingKeys = new WeakMap<object, number>()
let nextMappingKey = 0
function mappingKey(mapping: object) {
  if (!mappingKeys.has(mapping)) mappingKeys.set(mapping, ++nextMappingKey)
  return mappingKeys.get(mapping)!
}
const quotaWindows = ['5h', '7d'] as const
const dirty = computed(() => loaded.value && JSON.stringify(draft.value) !== savedJSON.value)
let generation = 0

watch(() => props.groupId, () => {
  generation++
  loaded.value = false
  busy.value = false
  draft.value = { rules: [] }
  error.value = ''
  notice.value = ''
  conflicted.value = false
  previewResult.value = null
  searches.value = {}
  modelsController?.abort()
  modelCatalog.value = null
  modelsLoading.value = false
  modelsError.value = ''
  if (expanded.value) void load()
})
watch([draft, previewModel], () => { previewResult.value = null; if (dirty.value) notice.value = '' }, { deep: true })
onBeforeUnmount(() => { generation++; modelsController?.abort() })

function acceptView(view: QuotaPolicyView, replaceDraft: boolean) {
  accounts.value = view.accounts ?? []
  runtime.value = { snapshots: view.runtime?.snapshots ?? [], rules: view.runtime?.rules ?? [], events: view.runtime?.events ?? [] }
  savedNames.value = Object.fromEntries((view.config.rules ?? []).map(rule => [rule.id, rule.name]))
  if (replaceDraft) {
    draft.value = JSON.parse(JSON.stringify({ rules: view.config.rules ?? [] }))
    savedJSON.value = JSON.stringify(draft.value)
    revision.value = view.revision
    loaded.value = true
    conflicted.value = false
  } else if (view.revision !== revision.value) {
    conflicted.value = true
    error.value = t('quotaPolicy.conflict')
  }
}

async function run<T>(operation: () => Promise<T>, accept: (result: T) => void, loading = false) {
  if (busy.value) return
  const current = generation
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await operation()
    if (current === generation) accept(result)
  } catch (cause) {
    if (current !== generation) return
    const failure = cause as { response?: { status?: number }; status?: number; message?: string }
    if ((failure.response?.status ?? failure.status) === 409) {
      conflicted.value = true
      error.value = t('quotaPolicy.conflict')
    } else {
      error.value = t(loading ? 'quotaPolicy.loadFailed' : 'quotaPolicy.operationFailed')
      if (!loading && failure.status === 400 && failure.message) error.value += ` ${failure.message}`
    }
  } finally {
    if (current === generation) busy.value = false
  }
}

function load() { return run(() => policyAPI.get(props.groupId), view => { acceptView(view, true); void loadModels() }, true) }
async function loadModels() {
  if (modelsLoading.value) return
  const current = generation
  modelsController = new AbortController()
  modelsLoading.value = true
  modelsError.value = ''
  try {
    const catalog = await policyAPI.getModels(props.groupId, modelsController.signal)
    if (current === generation) modelCatalog.value = catalog
  } catch {
    if (current === generation) modelsError.value = t('quotaPolicy.modelsFailed')
  } finally {
    if (current === generation) modelsLoading.value = false
  }
}
function toggle() { expanded.value = !expanded.value; if (expanded.value && !loaded.value) void load() }
function addRule() {
  draft.value.rules.push({ id: crypto.randomUUID(), name: '', priority: 0, mode: 'disabled', account_ids: [], account_match: 'any', window_match: 'any', conditions: [], recovery: 'auto', mappings: [{ source: '', target: '' }] })
}
function copyRule(rule: DraftRule) { draft.value.rules.push({ ...JSON.parse(JSON.stringify(rule)), id: crypto.randomUUID(), mode: 'disabled' }) }
function toggleWindow(rule: DraftRule, window: QuotaCondition['window']) {
  const index = rule.conditions.findIndex(condition => condition.window === window)
  if (index >= 0) rule.conditions.splice(index, 1)
  else rule.conditions.push({ window, trigger_remaining: '', recover_remaining: '' })
}
function validation(): string {
  for (const rule of draft.value.rules) {
    if (!rule.name.trim() || rule.name.trim().length > 80) return t('quotaPolicy.validationName')
    if (!Number.isInteger(rule.priority) || rule.priority < -10000 || rule.priority > 10000) return t('quotaPolicy.validationPriority')
    if (!rule.account_ids.length || (rule.mode !== 'disabled' && rule.account_ids.some(id => !accounts.value.some(account => account.id === id)))) return t('quotaPolicy.validationAccounts')
    if (!rule.conditions.length) return t('quotaPolicy.validationWindows')
    if (rule.conditions.some(condition => typeof condition.trigger_remaining !== 'number' || !Number.isFinite(condition.trigger_remaining) || typeof condition.recover_remaining !== 'number' || !Number.isFinite(condition.recover_remaining) || condition.trigger_remaining < 0 || condition.recover_remaining > 100 || condition.trigger_remaining >= condition.recover_remaining)) return t('quotaPolicy.validationThreshold')
    const sources = new Set<string>()
    for (const mapping of rule.mappings) {
      if (!mapping.source || !mapping.target || /[\s*]/.test(mapping.source + mapping.target) || mapping.source.length > 200 || mapping.target.length > 200 || mapping.source === mapping.target || sources.has(mapping.source)) return t('quotaPolicy.validationMappings')
      sources.add(mapping.source)
    }
    if (!rule.mappings.length) return t('quotaPolicy.validationMappings')
  }
  return ''
}
function config(): QuotaPolicyConfig {
  return { rules: draft.value.rules.map(rule => ({ ...JSON.parse(JSON.stringify(rule)), name: rule.name.trim(), conditions: rule.conditions.map(condition => ({ ...condition, trigger_remaining: Number(condition.trigger_remaining), recover_remaining: Number(condition.recover_remaining) })) })) }
}
function save() {
  if (!loaded.value || !dirty.value) return
  error.value = validation()
  if (error.value) return
  return run(() => policyAPI.save(props.groupId, revision.value, config()), view => { acceptView(view, true); notice.value = t('quotaPolicy.saved') })
}
function preview() {
  error.value = validation() || (!previewModel.value.trim() ? t('quotaPolicy.validationPreview') : '')
  if (error.value) return
  return run(() => policyAPI.preview(props.groupId, config(), previewModel.value.trim()), result => { previewResult.value = result })
}
function refresh() { return run(() => policyAPI.refresh(props.groupId), view => acceptView(view, false)) }
function release(ruleId: string) {
  if (dirty.value) return
  return run(() => policyAPI.release(props.groupId, revision.value, ruleId), view => acceptView(view, true))
}
function filteredAccounts(ruleId: string) {
  const query = (searches.value[ruleId] ?? '').trim().toLowerCase()
  return accounts.value.filter(account => `${account.name} ${account.id}`.toLowerCase().includes(query))
}
function accountLabel(id: number) { return accounts.value.find(account => account.id === id)?.name ?? `${t('quotaPolicy.missingAccount')} #${id}` }
function ruleLabel(id: string) { return savedNames.value[id] || id }
function windowLabel(window: string) { return t(window === '5h' ? 'quotaPolicy.fiveHour' : 'quotaPolicy.sevenDay') }
function statusLabel(status: string) { return ['disabled', 'unknown', 'normal', 'active', 'observing', 'suppressed'].includes(status) ? t(`quotaPolicy.${status}`) : t('quotaPolicy.unknown') }
function percent(value?: number) { return typeof value === 'number' && Number.isFinite(value) ? `${value.toFixed(1)}%` : t('quotaPolicy.unknown') }
function sampleUnknown(sample: QuotaPolicySnapshot) { return !!sample.error_code || !sample.sampled_at || Date.now() / 1000 - sample.sampled_at > 180 }
function formatTime(seconds?: number) { return seconds ? new Date(seconds * 1000).toLocaleString() : '—' }
</script>
