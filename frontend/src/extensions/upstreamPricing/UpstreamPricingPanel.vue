<template>
  <section class="space-y-5" :aria-label="labels.tab">
    <p class="text-sm text-gray-500">{{ labels.note }}</p>
    <ActivityStatusPanel :account-id="accountId" :revision="state?.source.revision" />
    <p class="text-xs text-gray-500">{{ labels.baseUrlHint }}</p>
    <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>
    <p v-if="notice" role="status" class="text-sm text-green-700 dark:text-green-400">{{ notice }}</p>
    <p v-if="state?.account_changed" role="alert" class="text-sm text-amber-700">{{ labels.changedAccount }}</p>
    <fieldset :disabled="busy || !state" class="space-y-4 disabled:opacity-60">
      <div class="grid grid-cols-2 gap-4">
        <label class="space-y-1 text-sm"><span>{{ labels.provider }}</span>
          <select v-model="form.provider" class="input"><option value="newapi">New API</option><option value="sub2api">Sub2API</option></select>
        </label>
        <label class="space-y-1 text-sm"><span>{{ labels.baseUrl }}</span><input v-model.trim="form.base_url" class="input" type="url" placeholder="https://example.com" autocomplete="off" /></label>
        <label v-if="form.provider === 'newapi'" class="space-y-1 text-sm"><span>{{ labels.userId }}</span><input v-model.trim="form.user_id" class="input" inputmode="numeric" :placeholder="labels.userIdHint" autocomplete="off" /></label>
        <label class="space-y-1 text-sm"><span>{{ labels.token }}</span><input v-model="form.token" class="input" type="password" autocomplete="new-password" :placeholder="state?.source.has_credential ? labels.tokenSaved : ''" /></label>
      </div>
      <p class="text-xs text-gray-500">{{ labels.tokenHint }}</p>
      <BrowserImportPanel :account-id="accountId" :revision="state?.source.revision || 0" :config="form" @imported="receiveImport" />
      <p v-if="form.browser_import_id" role="status" class="text-sm text-green-700">面板凭证已验证并暂存，请在 5 分钟内预览价格；应用后加密保存。</p>
      <div class="flex items-end gap-3">
        <label class="flex-1 space-y-1 text-sm"><span>{{ labels.group }}</span>
          <select v-model="selectedGroup" class="input"><option value="">{{ labels.chooseGroup }}</option>
            <option v-for="g in groups" :key="`${g.id || ''}:${g.name}`" :value="groupKey(g)">{{ g.name }}{{ g.multiplier ? ` · ${g.multiplier}x` : '' }}</option>
          </select>
        </label>
        <button type="button" class="btn btn-secondary" @click="readGroups">{{ labels.readGroups }}</button>
      </div>
      <div class="flex flex-wrap items-end gap-4">
        <label class="flex items-center gap-2 py-2 text-sm"><input v-model="form.automatic" type="checkbox" />{{ labels.automatic }}</label>
        <label class="space-y-1 text-sm"><span>{{ labels.interval }}</span><select v-model.number="form.interval_minutes" class="input">
          <option v-for="n in [15, 60, 360, 1440]" :key="n" :value="n">{{ n }} {{ labels.minutes }}</option>
        </select></label>
        <button type="button" class="btn btn-secondary" :disabled="!state?.source.active_version || dirty" @click="saveSchedule">{{ labels.schedule }}</button>
      </div>
      <div class="flex flex-wrap gap-3">
        <button type="button" class="btn btn-primary" :disabled="!form.group" @click="makePreview">{{ labels.preview }}</button>
        <button type="button" class="btn btn-secondary" :disabled="!state?.source.active_version || dirty" @click="syncSaved">{{ labels.sync }}</button>
      </div>
    </fieldset>

    <p v-if="busy" role="status" class="text-sm text-gray-500">{{ labels.loading }}</p>
    <div v-if="state?.source.active_version" class="flex flex-wrap gap-x-5 gap-y-2 text-xs text-gray-500">
      <span>{{ labels.version }}: {{ state.source.active_version }}</span><span>{{ labels.lastSuccess }}: {{ date(state.source.last_success) }}</span>
      <span>{{ labels.nextSync }}: {{ state.source.next_sync ? date(state.source.next_sync) : labels.manualOnly }}</span>
    </div>
    <p v-if="state?.source.last_error" class="text-sm text-amber-700">{{ state.source.last_error }}</p>

    <div v-if="preview" class="space-y-2 rounded-lg border border-primary-200 p-3 dark:border-primary-800">
      <p class="text-sm">{{ labels.added }} {{ preview.added }} · {{ labels.changed }} {{ preview.changed }} · {{ labels.removed }} {{ preview.removed }}</p>
      <p class="text-xs text-gray-500">{{ labels.expires }}</p>
      <button type="button" class="btn btn-primary" :disabled="busy" @click="applyPreview">{{ labels.apply }}</button>
    </div>

    <div v-if="catalog" class="space-y-3">
      <p class="text-sm">{{ labels.supported }} {{ catalog.supported }} · {{ labels.unsupported }} {{ catalog.unsupported }} · {{ labels.multiplier }} {{ catalog.group.multiplier }}x</p>
      <p class="text-xs text-gray-500">{{ labels.unit }} · {{ labels.partial }}</p>
      <input v-model="search" class="input" :placeholder="labels.search" :aria-label="labels.search" />
      <div class="max-h-80 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="w-full text-left text-xs">
          <thead class="sticky top-0 bg-gray-50 dark:bg-dark-800"><tr><th class="p-2">{{ labels.model }}</th><th class="p-2">{{ labels.input }}</th><th class="p-2">{{ labels.output }}</th><th class="p-2">{{ labels.cacheRead }}</th><th class="p-2">{{ labels.cacheWrite }}</th><th class="p-2">{{ labels.mode }}</th></tr></thead>
          <tbody><tr v-for="p in matchingModels" :key="p.model" class="border-t border-gray-100 dark:border-dark-700">
            <td class="p-2"><button type="button" class="text-primary-600 underline" :disabled="busy" :title="labels.editManual" @click="editOverride(p)">{{ p.model }}</button><span v-if="p.manual" class="ml-1 text-amber-700">{{ labels.manual }}</span></td>
            <td class="p-2">{{ p.input ?? '—' }}</td><td class="p-2">{{ p.output ?? '—' }}</td><td class="p-2">{{ p.cache_read ?? '—' }}</td><td class="p-2">{{ p.cache_write ?? '—' }}</td>
            <td class="max-w-72 p-2">{{ p.mode === 'unavailable' ? p.reason : p.mode === 'per_request' ? `${labels.requestMode}: $${p.request}` : labels.tokenMode }}</td>
          </tr></tbody>
        </table>
      </div>
      <p class="text-xs text-gray-400">{{ labels.first100 }}</p>
    </div>
    <p v-else-if="!busy" class="text-sm text-gray-500">{{ labels.noCatalog }}</p>

    <details :open="editorOpen" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
      <summary class="cursor-pointer text-sm font-medium">{{ labels.manual }} ({{ form.overrides?.length || 0 }})</summary>
      <p class="text-xs text-gray-500">{{ labels.overrideHint }}</p>
      <fieldset :disabled="busy" class="space-y-3">
        <div class="grid grid-cols-2 gap-3">
          <label class="text-sm">{{ labels.model }}<input v-model.trim="editor.model" class="input" /></label>
          <label class="text-sm">{{ labels.mode }}<select v-model="editor.mode" class="input"><option value="token">{{ labels.tokenMode }}</option><option value="per_request">{{ labels.requestMode }}</option></select></label>
          <template v-if="editor.mode === 'token'"><label v-for="field in priceFields" :key="field.key" class="text-sm">{{ field.label }}<input :value="editor[field.key] ?? ''" class="input" inputmode="decimal" @input="setPrice(field.key, $event)" /></label></template>
          <label v-else class="text-sm">{{ labels.request }}<input :value="editor.request ?? ''" class="input" inputmode="decimal" @input="setPrice('request', $event)" /></label>
        </div>
        <div class="flex flex-wrap gap-2"><button type="button" class="btn btn-secondary" :disabled="!editor.model" @click="saveOverride">{{ labels.saveOverride }}</button><button type="button" class="btn btn-secondary" @click="clearEditor">{{ labels.clearEditor }}</button></div>
        <ul class="space-y-1 text-xs"><li v-for="p in form.overrides" :key="p.model" class="flex items-center gap-3"><button type="button" class="text-primary-600 underline" @click="editOverride(p)">{{ p.model }}</button><button type="button" class="text-red-600 underline" @click="removeOverride(p.model)">{{ labels.removeOverride }}</button></li></ul>
      </fieldset>
    </details>

    <details v-if="state?.runs.length"><summary class="cursor-pointer text-sm font-medium">{{ labels.history }}</summary>
      <ul class="mt-2 space-y-2 text-xs"><li v-for="run in state.runs" :key="run.id">{{ date(run.created_at) }} · {{ run.status === 'success' ? labels.success : labels.failed }} · {{ run.version_id ? `${labels.version} ${run.version_id}` : run.message }}</li></ul>
    </details>
    <details v-if="state?.versions.length"><summary class="cursor-pointer text-sm font-medium">{{ labels.versions }}</summary>
      <ul class="mt-2 space-y-2 text-xs"><li v-for="v in state.versions" :key="v.id" class="flex items-center gap-3"><span>#{{ v.id }} · {{ date(v.effective_from) }}</span><button v-if="v.id !== state.source.active_version" type="button" class="text-primary-600 underline" :disabled="busy || dirty" @click="rollback(v.id)">{{ labels.rollback }}</button></li></ul>
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { upstreamPricingAPI } from './api'
import ActivityStatusPanel from './ActivityStatusPanel.vue'
import BrowserImportPanel from './BrowserImportPanel.vue'
import type { BrowserImportStatus } from '.'
import { blankPrice, useUpstreamPricingLabels, type Catalog, type Price, type Preview, type PricingConfig, type PricingGroup, type PricingState } from '.'

const props = defineProps<{ accountId: number; defaultBaseUrl?: string }>()
const labels = useUpstreamPricingLabels()
const state = ref<PricingState | null>(null)
const groups = ref<PricingGroup[]>([])
const preview = ref<Preview | null>(null)
const form = reactive<PricingConfig>({ provider: 'newapi', base_url: '', token: '', group: '', automatic: true, interval_minutes: 60, overrides: [] })
const busy = ref(false)
const error = ref('')
const notice = ref('')
const search = ref('')
const editor = reactive<Price>(blankPrice())
const editorOpen = ref(false)
const baseline = ref('')
const controller = new AbortController()
let alive = true
const catalog = computed<Catalog | null>(() => preview.value?.catalog || state.value?.catalog || null)
const matchingModels = computed(() => (catalog.value?.models || []).filter(p => p.model.toLowerCase().includes(search.value.toLowerCase())).slice(0, 100))
const connection = () => JSON.stringify([form.provider, form.base_url, form.user_id || '', form.group, form.group_id || '', form.overrides || [], form.token || '', form.browser_import_id || ''])
const dirty = computed(() => connection() !== baseline.value)
const groupKey = (g: PricingGroup) => `${g.id || ''}:${g.name}`
const selectedGroup = computed({ get: () => form.group ? groupKey({ id: form.group_id, name: form.group, multiplier: '' }) : '', set: (value: string) => {
  const group = groups.value.find(g => groupKey(g) === value)
  form.group = group?.name || ''; form.group_id = group?.id || ''
} })
const date = (value?: string | null) => value ? new Date(value).toLocaleString() : labels.value.never
type PriceField = 'input' | 'output' | 'cache_read' | 'cache_write' | 'cache_write_1h' | 'cache_write_30m' | 'cache_write_12h'
const priceFields = computed<{ key: PriceField; label: string }[]>(() => [
  { key: 'input', label: labels.value.input }, { key: 'output', label: labels.value.output }, { key: 'cache_read', label: labels.value.cacheRead },
  { key: 'cache_write', label: labels.value.cacheWrite }, { key: 'cache_write_1h', label: labels.value.cache1h }, { key: 'cache_write_30m', label: labels.value.cache30m }, { key: 'cache_write_12h', label: labels.value.cache12h },
])
function setPrice(key: PriceField | 'request', event: Event) { const value = (event.target as HTMLInputElement).value.trim(); editor[key] = value === '' ? null : value }
function clearEditor() { Object.assign(editor, blankPrice(), { cache_write_1h: null, cache_write_30m: null, cache_write_12h: null }) }
function editOverride(p: Price) { clearEditor(); Object.assign(editor, p, { mode: p.mode === 'unavailable' ? 'token' : p.mode }); editorOpen.value = true }
function saveOverride() {
  if (!editor.model) return
  const p = { ...editor, manual: true, reason: undefined }
  form.overrides = [...(form.overrides || []).filter(v => v.model !== p.model), p]
  clearEditor()
}
function removeOverride(model: string) { form.overrides = (form.overrides || []).filter(p => p.model !== model) }
watch(form, () => { preview.value = null; notice.value = '' }, { deep: true, flush: 'sync' })
watch(() => [form.provider, form.base_url, form.user_id, form.token], () => { form.browser_import_id = '' }, { flush: 'sync' })
function receiveImport(value: BrowserImportStatus) {
  form.token = ''; form.user_id = value.user_id
  form.browser_import_id = value.id
  groups.value = value.groups || []
  if (!groups.value.some(g => g.name === form.group && (g.id || '') === (form.group_id || ''))) { form.group = ''; form.group_id = '' }
}
watch(() => [form.provider, form.base_url, form.user_id, form.group, form.group_id], () => {
  form.overrides = []
  clearEditor()
}, { flush: 'sync' })

async function load() {
  const value = await upstreamPricingAPI.get(props.accountId, controller.signal)
  if (!alive) return
  state.value = value
  Object.assign(form, { token: '', browser_import_id: '', user_id: '', group_id: '', overrides: [] }, value.source.config)
  form.token = ''
  if (!value.source.has_credential && !form.base_url) form.base_url = props.defaultBaseUrl || ''
  groups.value = form.group ? [{ name: form.group, id: form.group_id, multiplier: value.catalog?.group.multiplier || '' }] : []
  baseline.value = connection()
}
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await action() } catch (err) { if (alive) error.value = (err as { message?: string }).message || labels.value.error } finally { if (alive) busy.value = false }
}
function readGroups() { return run(async () => {
  const value = await upstreamPricingAPI.groups(props.accountId, state.value?.source.revision || 0, { ...form }, controller.signal)
  if (!alive) return
  groups.value = value
  if (!value.some(g => g.name === form.group && (g.id || '') === (form.group_id || ''))) { form.group = ''; form.group_id = '' }
}) }
function makePreview() { return run(async () => { const value = await upstreamPricingAPI.preview(props.accountId, state.value?.source.revision || 0, { ...form }, controller.signal); if (alive) preview.value = value }) }
function applyPreview() { return run(async () => {
  if (!preview.value) return
  await upstreamPricingAPI.apply(props.accountId, preview.value.id, controller.signal)
  await load(); if (alive) { preview.value = null; notice.value = labels.value.saved }
}) }
function syncSaved() { return run(async () => { await upstreamPricingAPI.sync(props.accountId, controller.signal); await load() }) }
function saveSchedule() { return run(async () => { await upstreamPricingAPI.schedule(props.accountId, form.automatic, form.interval_minutes, controller.signal); await load() }) }
function rollback(version: number) { return run(async () => { const value = await upstreamPricingAPI.rollback(props.accountId, version, controller.signal); if (alive) preview.value = value }) }
onMounted(() => { void run(load) })
onBeforeUnmount(() => { alive = false; controller.abort(); form.token = ''; form.browser_import_id = '' })
</script>
