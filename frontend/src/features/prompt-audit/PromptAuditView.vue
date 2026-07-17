<template>
  <AppLayout>
    <div class="mx-auto max-w-[1600px]" :class="activeTab === 'config' && draft ? 'pb-28' : 'pb-8'">
      <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">{{ t('nav.securityAudit') REDACTEDREDACTED</p>
          <h1 class="mt-1 text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">{{ t('admin.promptAudit.title') REDACTEDREDACTED</h1>
          <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-300">{{ t('admin.promptAudit.description') REDACTEDREDACTED</p>
        </div>
        <div v-if="draft" class="text-right text-xs text-gray-500 dark:text-dark-400">
          <p>{{ t('admin.promptAudit.configVersion', { version: draft.config_version REDACTED) REDACTEDREDACTED</p>
          <p v-if="draft.updated_at" class="mt-1">{{ formatDate(draft.updated_at) REDACTEDREDACTED</p>
        </div>
      </header>

      <div v-if="loadErrors.config && !draft" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-5 dark:border-red-900 dark:bg-red-950/30">
        <p class="text-sm text-red-700 dark:text-red-300">{{ loadErrors.config REDACTEDREDACTED</p>
        <button type="button" class="btn btn-secondary btn-sm mt-3" @click="loadConfig">{{ t('admin.promptAudit.actions.retry') REDACTEDREDACTED</button>
      </div>

      <template v-else>
        <div class="mb-4" role="tablist" :aria-label="t('admin.promptAudit.title')">
          <div class="tabs inline-flex">
            <button
              v-for="tab in pageTabs"
              :key="tab.id"
              type="button"
              role="tab"
              class="tab"
              :class="{ 'tab-active': activeTab === tab.id REDACTED"
              :aria-selected="activeTab === tab.id"
              :data-test="`tab-${tab.idREDACTED`"
              @click="activeTab = tab.id"
            >
              {{ tab.label REDACTEDREDACTED
            </button>
          </div>
        </div>

        <main class="card px-4 sm:px-6 lg:px-8">
          <div v-show="activeTab === 'config'" data-test="tab-panel-config">
            <RuntimeOverview :runtime="runtime" :loading="loading.runtime" :error="loadErrors.runtime" @refresh="loadRuntime" />

            <template v-if="draft">
              <EndpointPool
                :endpoints="draft.endpoints"
                :probe-results="probeResults"
                :probing-ids="probingIds"
                @update:endpoints="updateEndpoints"
                @probe="runProbe"
              />
              <div v-if="loadErrors.groups" role="alert" class="mt-5 rounded-lg bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ loadErrors.groups REDACTEDREDACTED</div>
              <PolicyPanel :draft="draft" :groups="groups" @update:draft="replaceDraft" />
            </template>
          </div>

          <div v-show="activeTab === 'events'" data-test="tab-panel-events">
            <div
              v-if="draft?.enabled && !draft.store_pass_events"
              data-test="pass-events-disabled-notice"
              role="status"
              class="mt-6 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-900/70 dark:bg-amber-950/30 dark:text-amber-200"
            >
              <span>{{ t('admin.promptAudit.events.passEventsDisabled') REDACTEDREDACTED</span>
              <button type="button" class="btn btn-secondary btn-sm" @click="activeTab = 'config'">
                {{ t('admin.promptAudit.events.openConfiguration') REDACTEDREDACTED
              </button>
            </div>
            <EventWorkspace
              :events="events.items"
              :total="events.total"
              :page="events.page"
              :page-size="events.page_size"
              :filters="filters"
              :selected-ids="selectedEventIds"
              :loading="loading.events"
              :error="loadErrors.events"
              @filters-change="handleFiltersChanged"
              @search="applyEventFilters"
              @selection="selectedEventIds = $event"
              @page="changePage"
              @page-size="changePageSize"
              @view="openEvent"
              @delete="requestSingleDelete"
              @batch-delete="requestBatchDelete"
              @preview-delete="requestFilterDeletePreview"
            />
          </div>
        </main>
      </template>
    </div>

    <div v-if="draft && activeTab === 'config'" class="fixed inset-x-0 bottom-0 z-30 border-t border-gray-200 bg-white/95 px-4 py-3 shadow-[0_-12px_35px_rgba(15,23,42,0.08)] backdrop-blur dark:border-dark-700/80 dark:bg-dark-900/95 dark:shadow-[0_-12px_35px_rgba(0,0,0,0.35)] lg:left-64">
      <div class="mx-auto flex max-w-[1600px] flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap items-center gap-x-5 gap-y-2">
          <SaveToggle :label="t('admin.promptAudit.saveBar.enabled')" :model-value="draft.enabled" data-test="enabled-toggle" @update:model-value="setEnabled" />
          <SaveToggle :label="t('admin.promptAudit.saveBar.blocking')" :model-value="draft.blocking_enabled" :disabled="!draft.enabled" data-test="blocking-toggle" @update:model-value="setBlocking" />
          <SaveToggle :label="t('admin.promptAudit.saveBar.storePass')" :model-value="draft.store_pass_events" data-test="store-pass-toggle" @update:model-value="replaceDraft({ ...draft!, store_pass_events: $event REDACTED)" />
        </div>
        <div class="flex items-center gap-3">
          <span class="text-sm" :class="dirty ? 'text-amber-700 dark:text-amber-300' : 'text-gray-500 dark:text-dark-400'">
            {{ dirty ? t('admin.promptAudit.saveBar.dirty') : t('admin.promptAudit.saveBar.synced') REDACTEDREDACTED
          </span>
          <button type="button" class="btn btn-secondary" :disabled="!dirty || loading.saving" @click="resetDraft">{{ t('common.reset') REDACTEDREDACTED</button>
          <button type="button" class="btn btn-primary" :disabled="!dirty || loading.saving" data-test="save-config" @click="saveConfig">
            {{ loading.saving ? t('common.saving') : t('common.save') REDACTEDREDACTED
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :show="showBlockingConfirmation"
      :title="t('admin.promptAudit.blockingConfirm.title')"
      :message="t('admin.promptAudit.blockingConfirm.message')"
      :confirm-text="t('admin.promptAudit.blockingConfirm.confirm')"
      danger
      @confirm="confirmBlocking"
      @cancel="showBlockingConfirmation = false"
    />
    <ConfirmDialog
      :show="deleteRequest.mode !== ''"
      :title="t('admin.promptAudit.events.deleteConfirmTitle')"
      :message="t('admin.promptAudit.events.deleteConfirmMessage', { count: deleteRequest.ids.length REDACTED)"
      :confirm-text="t('common.delete')"
      danger
      @confirm="confirmIDDelete"
      @cancel="clearDeleteRequest"
    />
    <BaseDialog :show="Boolean(deletePreview)" :title="t('admin.promptAudit.events.filterDeleteTitle')" width="normal" @close="deletePreview = null">
      <div v-if="deletePreview" class="space-y-4 text-sm text-gray-600 dark:text-dark-300">
        <p>{{ t('admin.promptAudit.events.filterDeleteCount', { count: deletePreview.matched_count REDACTED) REDACTEDREDACTED</p>
        <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2">
          <dt>{{ t('admin.promptAudit.events.snapshotMax') REDACTEDREDACTED</dt><dd>{{ deletePreview.snapshot_max_id REDACTEDREDACTED</dd>
          <dt>Filter SHA-256</dt><dd class="break-all font-mono text-xs">{{ deletePreview.filter_hash REDACTEDREDACTED</dd>
          <dt>{{ t('admin.promptAudit.events.expiresAt') REDACTEDREDACTED</dt><dd>{{ formatDate(deletePreview.expires_at) REDACTEDREDACTED</dd>
        </dl>
        <p class="rounded-lg bg-amber-50 px-3 py-2 text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ t('admin.promptAudit.events.filterDeleteWarning') REDACTEDREDACTED</p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="deletePreview = null">{{ t('common.cancel') REDACTEDREDACTED</button>
          <button type="button" class="btn btn-danger" :disabled="loading.deleting" data-test="confirm-filter-delete" @click="confirmFilterDelete">{{ t('admin.promptAudit.events.confirmFilterDelete') REDACTEDREDACTED</button>
        </div>
      </template>
    </BaseDialog>
    <EventDetailDialog :show="showEventDetail" :event="activeEvent" :loading="loading.detail" @close="closeEventDetail" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useAppStore REDACTED from '@/stores/app'
import { extractApiErrorCode, extractApiErrorMessage REDACTED from '@/utils/apiError'
import RuntimeOverview from './components/RuntimeOverview.vue'
import EndpointPool from './components/EndpointPool.vue'
import PolicyPanel from './components/PolicyPanel.vue'
import EventWorkspace from './components/EventWorkspace.vue'
import EventDetailDialog from './components/EventDetailDialog.vue'
import promptAuditAPI from './api'
import type {
  PromptAuditDraft,
  PromptAuditEndpointDraft,
  PromptAuditEvent,
  PromptAuditGroup,
  PromptAuditRuntime,
  PromptDeletePreview,
  PromptEventFilters,
  PromptEventPage,
  PromptLoadErrors,
  PromptProbeResult,
REDACTED from './types'
import { buildUpdateRequest, cloneData, configToDraft, draftFingerprint, emptyEventFilters REDACTED from './viewModel'

const { t, locale REDACTED = useI18n()
const appStore = useAppStore()
type PromptAuditPageTab = 'config' | 'events'
const activeTab = ref<PromptAuditPageTab>('config')
const pageTabs = computed(() => [
  { id: 'events' as const, label: t('admin.promptAudit.tabs.events') REDACTED,
  { id: 'config' as const, label: t('admin.promptAudit.tabs.config') REDACTED,
])
const serverConfig = ref<PromptAuditDraft | null>(null)
const draft = ref<PromptAuditDraft | null>(null)
const runtime = ref<PromptAuditRuntime | null>(null)
const groups = ref<PromptAuditGroup[]>([])
const events = reactive<PromptEventPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 REDACTED)
const filters = ref<PromptEventFilters>(emptyEventFilters())
const appliedFilters = ref<PromptEventFilters>(emptyEventFilters())
const selectedEventIds = ref<number[]>([])
const activeEvent = ref<PromptAuditEvent | null>(null)
const showEventDetail = ref(false)
const probeResults = reactive<Record<string, PromptProbeResult>>({REDACTED)
const probingIds = ref<string[]>([])
const deletePreview = ref<PromptDeletePreview | null>(null)
const showBlockingConfirmation = ref(false)
const deleteRequest = reactive<{ mode: '' | 'single' | 'batch'; ids: number[] REDACTED>({ mode: '', ids: [] REDACTED)
const loading = reactive({ config: false, runtime: false, groups: false, events: false, saving: false, detail: false, deleting: false REDACTED)
const loadErrors = reactive<PromptLoadErrors>({ config: '', runtime: '', groups: '', events: '' REDACTED)
const dirty = computed(() => draftFingerprint(draft.value) !== draftFingerprint(serverConfig.value))

const SaveToggle = defineComponent({
  inheritAttrs: false,
  props: { label: { type: String, required: true REDACTED, modelValue: { type: Boolean, required: true REDACTED, disabled: { type: Boolean, default: false REDACTED REDACTED,
  emits: ['update:modelValue'],
  setup(props, { emit, attrs REDACTED) {
    return () => h('label', { class: ['flex items-center gap-2.5 text-sm', props.disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'] REDACTED, [
      h('button', {
        ...attrs,
        type: 'button',
        role: 'switch',
        'aria-checked': props.modelValue,
        'aria-label': props.label,
        disabled: props.disabled,
        class: [
          'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2',
          props.modelValue ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600',
          props.disabled ? 'cursor-not-allowed' : 'cursor-pointer',
        ],
        onClick: (event: MouseEvent) => {
          event.preventDefault()
          if (!props.disabled) emit('update:modelValue', !props.modelValue)
        REDACTED,
      REDACTED, [
        h('span', {
          class: [
            'pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transition-transform duration-200 ease-in-out',
            props.modelValue ? 'translate-x-5' : 'translate-x-0',
          ],
        REDACTED),
      ]),
      h('span', { class: 'select-none text-gray-700 dark:text-dark-200' REDACTED, props.label),
    ])
  REDACTED,
REDACTED)

function errorMessage(error: unknown, fallbackKey: string): string {
  const code = extractApiErrorCode(error)
  if (code) {
    const key = `admin.promptAudit.errors.${codeREDACTED`
    const translated = t(key)
    if (translated !== key) return translated
  REDACTED
  return extractApiErrorMessage(error, t(fallbackKey))
REDACTED

async function loadConfig() {
  loading.config = true
  loadErrors.config = ''
  try {
    const config = await promptAuditAPI.getConfig()
    serverConfig.value = configToDraft(config)
    draft.value = configToDraft(config)
  REDACTED catch (error) {
    loadErrors.config = errorMessage(error, 'admin.promptAudit.errors.loadConfig')
  REDACTED finally {
    loading.config = false
  REDACTED
REDACTED
async function loadRuntime() {
  loading.runtime = true
  loadErrors.runtime = ''
  try { runtime.value = await promptAuditAPI.getRuntime() REDACTED
  catch (error) { loadErrors.runtime = errorMessage(error, 'admin.promptAudit.errors.loadRuntime') REDACTED
  finally { loading.runtime = false REDACTED
REDACTED
async function loadGroups() {
  loading.groups = true
  loadErrors.groups = ''
  try { groups.value = await promptAuditAPI.listGroups() REDACTED
  catch (error) { loadErrors.groups = errorMessage(error, 'admin.promptAudit.errors.loadGroups') REDACTED
  finally { loading.groups = false REDACTED
REDACTED
async function loadEvents() {
  loading.events = true
  loadErrors.events = ''
  try {
    const result = await promptAuditAPI.listEvents(appliedFilters.value, events.page, events.page_size)
    Object.assign(events, result)
    selectedEventIds.value = []
  REDACTED catch (error) {
    loadErrors.events = errorMessage(error, 'admin.promptAudit.errors.loadEvents')
  REDACTED finally {
    loading.events = false
  REDACTED
REDACTED
async function loadInitial() {
  await Promise.allSettled([loadConfig(), loadRuntime(), loadGroups(), loadEvents()])
REDACTED

function replaceDraft(value: PromptAuditDraft) { draft.value = cloneData(value) REDACTED
function updateEndpoints(value: PromptAuditEndpointDraft[]) {
  if (!draft.value) return
  replaceDraft({ ...draft.value, endpoints: value REDACTED)
REDACTED
function setEnabled(value: boolean) {
  if (!draft.value) return
  replaceDraft({ ...draft.value, enabled: value, blocking_enabled: value ? draft.value.blocking_enabled : false REDACTED)
REDACTED
function setBlocking(value: boolean) {
  if (!draft.value || !draft.value.enabled) return
  if (value && !draft.value.blocking_enabled) { showBlockingConfirmation.value = true; return REDACTED
  replaceDraft({ ...draft.value, blocking_enabled: value REDACTED)
REDACTED
function confirmBlocking() {
  showBlockingConfirmation.value = false
  if (draft.value) replaceDraft({ ...draft.value, blocking_enabled: true REDACTED)
REDACTED
function resetDraft() {
  if (serverConfig.value) draft.value = cloneData(serverConfig.value)
REDACTED
async function saveConfig() {
  if (!draft.value || !dirty.value) return
  loading.saving = true
  try {
    const saved = await promptAuditAPI.updateConfig(buildUpdateRequest(draft.value))
    serverConfig.value = configToDraft(saved)
    draft.value = configToDraft(saved)
    appStore.showSuccess(t('admin.promptAudit.messages.saved'))
    await loadRuntime()
  REDACTED catch (error) {
    const code = extractApiErrorCode(error)
    appStore.showError(errorMessage(error, code === 'prompt_audit_config_conflict' ? 'admin.promptAudit.errors.prompt_audit_config_conflict' : 'admin.promptAudit.errors.saveConfig'))
  REDACTED finally {
    loading.saving = false
  REDACTED
REDACTED
async function runProbe(endpoint: PromptAuditEndpointDraft) {
  if (probingIds.value.includes(endpoint.id)) return
  probingIds.value = [...probingIds.value, endpoint.id]
  try {
    const result = await promptAuditAPI.probeEndpoint(endpoint)
    probeResults[endpoint.id] = result
    if (result.ok) appStore.showSuccess(t('admin.promptAudit.messages.probeSucceeded'))
    else appStore.showError(`${result.error_code || result.statusREDACTED: ${result.messageREDACTED`)
  REDACTED catch (error) {
    appStore.showError(errorMessage(error, 'admin.promptAudit.errors.probe'))
  REDACTED finally {
    probingIds.value = probingIds.value.filter((id) => id !== endpoint.id)
  REDACTED
REDACTED

function handleFiltersChanged(value: PromptEventFilters) {
  filters.value = cloneData(value)
  deletePreview.value = null
REDACTED
function applyEventFilters(value: PromptEventFilters) {
  filters.value = cloneData(value)
  appliedFilters.value = cloneData(value)
  events.page = 1
  deletePreview.value = null
  void loadEvents()
REDACTED
function changePage(value: number) { events.page = value; void loadEvents() REDACTED
function changePageSize(value: number) { events.page_size = value; events.page = 1; void loadEvents() REDACTED
async function openEvent(id: number) {
  showEventDetail.value = true
  loading.detail = true
  activeEvent.value = null
  try { activeEvent.value = await promptAuditAPI.getEvent(id) REDACTED
  catch (error) { appStore.showError(errorMessage(error, 'admin.promptAudit.errors.loadDetail')); showEventDetail.value = false REDACTED
  finally { loading.detail = false REDACTED
REDACTED
function closeEventDetail() { showEventDetail.value = false; activeEvent.value = null REDACTED
function requestSingleDelete(id: number) { deleteRequest.mode = 'single'; deleteRequest.ids = [id] REDACTED
function requestBatchDelete() { if (selectedEventIds.value.length) { deleteRequest.mode = 'batch'; deleteRequest.ids = [...selectedEventIds.value] REDACTED REDACTED
function clearDeleteRequest() { deleteRequest.mode = ''; deleteRequest.ids = [] REDACTED
async function confirmIDDelete() {
  const mode = deleteRequest.mode
  const ids = [...deleteRequest.ids]
  clearDeleteRequest()
  if (!mode || ids.length === 0) return
  loading.deleting = true
  try {
    const result = mode === 'single' ? await promptAuditAPI.deleteEvent(ids[0]) : await promptAuditAPI.batchDeleteEvents(ids)
    appStore.showSuccess(t('admin.promptAudit.messages.deleted', { count: result.deleted_events REDACTED))
    await Promise.allSettled([loadEvents(), loadRuntime()])
  REDACTED catch (error) { appStore.showError(errorMessage(error, 'admin.promptAudit.errors.delete')) REDACTED
  finally { loading.deleting = false REDACTED
REDACTED
async function requestFilterDeletePreview() {
  loading.deleting = true
  try { deletePreview.value = await promptAuditAPI.previewDelete(filters.value) REDACTED
  catch (error) { appStore.showError(errorMessage(error, 'admin.promptAudit.errors.previewDelete')) REDACTED
  finally { loading.deleting = false REDACTED
REDACTED
async function confirmFilterDelete() {
  if (!deletePreview.value) return
  const preview = deletePreview.value
  loading.deleting = true
  try {
    const result = await promptAuditAPI.deleteEventsByFilter(filters.value, preview)
    deletePreview.value = null
    appStore.showSuccess(t('admin.promptAudit.messages.deleted', { count: result.deleted_events REDACTED))
    await Promise.allSettled([loadEvents(), loadRuntime()])
  REDACTED catch (error) {
    deletePreview.value = null
    appStore.showError(errorMessage(error, 'admin.promptAudit.errors.deleteConfirmation'))
  REDACTED finally { loading.deleting = false REDACTED
REDACTED
function formatDate(value: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'medium' REDACTED).format(new Date(value))
REDACTED

onMounted(loadInitial)
</script>
