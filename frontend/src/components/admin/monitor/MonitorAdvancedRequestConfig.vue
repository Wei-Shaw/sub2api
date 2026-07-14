<template>
  <div class="space-y-4">
    <!-- Headers key-value rows -->
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.advanced.headers') REDACTEDREDACTED</label>
      <div class="space-y-1.5">
        <div
          v-for="(row, i) in headerRows"
          :key="i"
          class="flex items-center gap-2"
        >
          <input
            v-model="row.name"
            type="text"
            spellcheck="false"
            :placeholder="t('admin.channelMonitor.advanced.headerNamePlaceholder')"
            class="input w-52 flex-none font-mono text-xs"
            @blur="commitHeaders"
          />
          <input
            v-model="row.value"
            type="text"
            spellcheck="false"
            :placeholder="t('admin.channelMonitor.advanced.headerValuePlaceholder')"
            class="input flex-1 font-mono text-xs"
            @blur="commitHeaders"
          />
          <button
            type="button"
            class="flex-none rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-500/10 dark:hover:text-red-400"
            :title="t('common.delete')"
            @click="removeRow(i)"
          >
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded border border-dashed border-gray-300 px-2 py-1 text-xs text-gray-500 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-500 dark:hover:text-primary-400"
          @click="addRow"
        >
          <svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          {{ t('admin.channelMonitor.advanced.headerAddRow') REDACTEDREDACTED
        </button>
      </div>
      <p v-if="headersError" class="mt-1 text-xs text-red-500">{{ headersError REDACTEDREDACTED</p>
      <p v-else class="mt-1 text-xs text-gray-400">
        {{ t('admin.channelMonitor.advanced.headersHint') REDACTEDREDACTED
      </p>
    </div>

    <!-- Body mode radio -->
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.advanced.bodyMode') REDACTEDREDACTED</label>
      <div class="grid grid-cols-3 gap-3">
        <button
          v-for="opt in bodyModeOptions"
          :key="opt.value"
          type="button"
          class="rounded-lg border-2 px-3 py-2 text-sm font-medium transition-colors"
          :class="bodyModeButtonClass(opt.value)"
          @click="updateBodyMode(opt.value)"
        >
          {{ opt.label REDACTEDREDACTED
        </button>
      </div>
      <p class="mt-1 text-xs text-gray-400">
        {{ bodyModeHint REDACTEDREDACTED
      </p>
    </div>

    <!-- Body JSON (仅当 mode != off) -->
    <div v-if="bodyOverrideMode !== 'off'">
      <div class="mb-1 flex items-center justify-between">
        <label class="input-label !mb-0">{{ t('admin.channelMonitor.advanced.bodyJson') REDACTEDREDACTED</label>
        <button
          type="button"
          class="text-xs text-primary-600 hover:underline disabled:cursor-not-allowed disabled:text-gray-400 disabled:no-underline dark:text-primary-400"
          :disabled="!bodyText.trim()"
          @click="formatBody"
        >
          {{ t('admin.channelMonitor.advanced.bodyJsonFormat') REDACTEDREDACTED
        </button>
      </div>
      <textarea
        v-model="bodyText"
        rows="10"
        :placeholder="bodyPlaceholder"
        class="input font-mono text-xs"
        style="white-space: pre; overflow-wrap: normal; overflow-x: auto;"
        spellcheck="false"
        @blur="commitBody"
      />
      <p v-if="bodyError" class="mt-1 text-xs text-red-500">{{ bodyError REDACTEDREDACTED</p>
      <p v-else class="mt-1 text-xs text-gray-400">
        {{ t('admin.channelMonitor.advanced.bodyJsonHint') REDACTEDREDACTED
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { APIMode, BodyOverrideMode, Provider REDACTED from '@/api/admin/channelMonitor'
import {
  API_MODE_RESPONSES,
  DEFAULT_GROK_MODEL,
  PROVIDER_GROK,
  PROVIDER_OPENAI,
REDACTED from '@/constants/channelMonitor'

const props = defineProps<{
  provider?: Provider
  apiMode?: APIMode
  extraHeaders: Record<string, string>
  bodyOverrideMode: BodyOverrideMode
  bodyOverride: Record<string, unknown> | null
REDACTED>()

const emit = defineEmits<{
  (e: 'update:extraHeaders', value: Record<string, string>): void
  (e: 'update:bodyOverrideMode', value: BodyOverrideMode): void
  (e: 'update:bodyOverride', value: Record<string, unknown> | null): void
REDACTED>()

const { t REDACTED = useI18n()

// ---- Headers key-value rows ----
interface HeaderRow {
  name: string
  value: string
REDACTED

const headerRows = ref<HeaderRow[]>(toRows(props.extraHeaders))
const headersError = ref('')

watch(
  () => props.extraHeaders,
  (v) => {
    // 外部重置时（切换平台 / 应用模板）同步行。
    // 同值不回写，避免每次 commit 都把行重排。
    if (!isSameHeaderMap(toMap(headerRows.value), v)) {
      headerRows.value = toRows(v)
    REDACTED
    headersError.value = ''
  REDACTED,
)

function toRows(h: Record<string, string>): HeaderRow[] {
  const entries = Object.entries(h || {REDACTED)
  if (entries.length === 0) return [{ name: '', value: '' REDACTED]
  return entries.map(([name, value]) => ({ name, value REDACTED))
REDACTED

function toMap(rows: HeaderRow[]): Record<string, string> {
  const out: Record<string, string> = {REDACTED
  for (const row of rows) {
    const name = row.name.trim()
    if (name === '') continue
    out[name] = row.value
  REDACTED
  return out
REDACTED

function isSameHeaderMap(a: Record<string, string>, b: Record<string, string>): boolean {
  const ak = Object.keys(a)
  const bk = Object.keys(b || {REDACTED)
  if (ak.length !== bk.length) return false
  for (const k of ak) {
    if (a[k] !== b[k]) return false
  REDACTED
  return true
REDACTED

function commitHeaders() {
  // 空白 name + 空白 value 的行允许保留作为"占位新行"，不报错；
  // name 非空但 value 为空（或反之）都视为用户正在编辑，同样不报错。
  // 只在 name 里含冒号这种明显不合法时兜一下。
  for (const row of headerRows.value) {
    const name = row.name.trim()
    if (name === '') continue
    if (name.includes(':') || /\s/.test(name)) {
      headersError.value = t('admin.channelMonitor.advanced.headerNameInvalid', { name REDACTED)
      return
    REDACTED
  REDACTED
  headersError.value = ''
  emit('update:extraHeaders', toMap(headerRows.value))
REDACTED

function addRow() {
  headerRows.value.push({ name: '', value: '' REDACTED)
REDACTED

function removeRow(index: number) {
  headerRows.value.splice(index, 1)
  if (headerRows.value.length === 0) {
    headerRows.value.push({ name: '', value: '' REDACTED)
  REDACTED
  commitHeaders()
REDACTED

// ---- Body mode + JSON ----
const bodyText = ref(serializeBody(props.bodyOverride))
const bodyError = ref('')

watch(
  () => props.bodyOverride,
  (v) => {
    bodyText.value = serializeBody(v)
    bodyError.value = ''
  REDACTED,
)

function commitBody() {
  if (props.bodyOverrideMode === 'off') {
    return
  REDACTED
  const trimmed = bodyText.value.trim()
  if (trimmed === '') {
    emit('update:bodyOverride', null)
    bodyError.value = ''
    return
  REDACTED
  try {
    const parsed = JSON.parse(trimmed)
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
      bodyError.value = t('admin.channelMonitor.advanced.bodyJsonObjectError')
      return
    REDACTED
    emit('update:bodyOverride', parsed as Record<string, unknown>)
    bodyError.value = ''
  REDACTED catch (e) {
    bodyError.value =
      t('admin.channelMonitor.advanced.bodyJsonError') +
      ': ' +
      (e instanceof Error ? e.message : String(e))
  REDACTED
REDACTED

function formatBody() {
  const trimmed = bodyText.value.trim()
  if (trimmed === '') return
  try {
    const parsed = JSON.parse(trimmed)
    bodyText.value = JSON.stringify(parsed, null, 2)
    bodyError.value = ''
    // 同步把校验过的对象提交，避免格式化后焦点未移走时父组件读到旧值
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      emit('update:bodyOverride', parsed as Record<string, unknown>)
    REDACTED
  REDACTED catch (e) {
    bodyError.value =
      t('admin.channelMonitor.advanced.bodyJsonError') +
      ': ' +
      (e instanceof Error ? e.message : String(e))
  REDACTED
REDACTED

function serializeBody(body: Record<string, unknown> | null): string {
  if (!body || Object.keys(body).length === 0) return ''
  return JSON.stringify(body, null, 2)
REDACTED

function updateBodyMode(mode: BodyOverrideMode) {
  emit('update:bodyOverrideMode', mode)
  // 切换到 off 时清掉 body（提示用户）
  if (mode === 'off') {
    emit('update:bodyOverride', null)
  REDACTED
REDACTED

const bodyModeOptions = computed<{ value: BodyOverrideMode; label: string REDACTED[]>(() => [
  { value: 'off', label: t('admin.channelMonitor.advanced.bodyModeOff') REDACTED,
  { value: 'merge', label: t('admin.channelMonitor.advanced.bodyModeMerge') REDACTED,
  { value: 'replace', label: t('admin.channelMonitor.advanced.bodyModeReplace') REDACTED,
])

function bodyModeButtonClass(mode: BodyOverrideMode): string {
  const active = props.bodyOverrideMode === mode
  if (active) {
    return 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/15 dark:text-primary-300 dark:border-primary-400'
  REDACTED
  return 'border-gray-200 bg-white text-gray-600 hover:border-primary-300 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400'
REDACTED

const bodyModeHint = computed(() => {
  switch (props.bodyOverrideMode) {
    case 'merge':
      return t('admin.channelMonitor.advanced.bodyModeHintMerge')
    case 'replace':
      return t('admin.channelMonitor.advanced.bodyModeHintReplace')
    default:
      return t('admin.channelMonitor.advanced.bodyModeHintOff')
  REDACTED
REDACTED)

const bodyPlaceholder = computed(() => {
  if (props.provider === PROVIDER_OPENAI && props.apiMode === API_MODE_RESPONSES) {
    if (props.bodyOverrideMode === 'merge') {
      return '{\n  "max_output_tokens": 20\nREDACTED'
    REDACTED
    return '{\n  "model": "gpt-4o-mini",\n  "instructions": "You are a health check endpoint. Reply briefly.",\n  "input": "Reply with exactly: ok",\n  "max_output_tokens": 20,\n  "stream": false\nREDACTED'
  REDACTED
  if (props.provider === PROVIDER_OPENAI || props.provider === PROVIDER_GROK) {
    if (props.bodyOverrideMode === 'merge') {
      return '{\n  "max_tokens": 20\nREDACTED'
    REDACTED
    const model = props.provider === PROVIDER_GROK ? DEFAULT_GROK_MODEL : 'gpt-4o-mini'
    return `{\n  "model": "${modelREDACTED",\n  "messages": [{"role":"user","content":"Reply with exactly: ok"REDACTED],\n  "max_tokens": 20,\n  "stream": false\nREDACTED`
  REDACTED
  if (props.bodyOverrideMode === 'merge') {
    return '{\n  "system": "You are Claude Code..."\nREDACTED'
  REDACTED
  return '{\n  "model": "claude-x",\n  "messages": [{"role":"user","content":"hi"REDACTED],\n  "max_tokens": 10\nREDACTED'
REDACTED)
</script>
