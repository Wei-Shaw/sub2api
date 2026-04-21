<template>
  <div class="space-y-4">
    <!-- Headers textarea -->
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.advanced.headers') REDACTEDREDACTED</label>
      <textarea
        v-model="headersText"
        rows="4"
        :placeholder="t('admin.channelMonitor.advanced.headersPlaceholder')"
        class="input font-mono text-xs"
        @blur="commitHeaders"
      />
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
import type { BodyOverrideMode REDACTED from '@/api/admin/channelMonitor'

const props = defineProps<{
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

// ---- Headers textarea (Key: Value per line) ----
const headersText = ref(serializeHeaders(props.extraHeaders))
const headersError = ref('')

watch(
  () => props.extraHeaders,
  (v) => {
    // 外部重置时（如切换平台 / 应用模板）同步文本
    headersText.value = serializeHeaders(v)
    headersError.value = ''
  REDACTED,
)

function commitHeaders() {
  const parsed = parseHeaders(headersText.value)
  if (parsed.error) {
    headersError.value = parsed.error
    return
  REDACTED
  headersError.value = ''
  emit('update:extraHeaders', parsed.headers)
REDACTED

function serializeHeaders(h: Record<string, string>): string {
  return Object.entries(h || {REDACTED)
    .map(([k, v]) => `${kREDACTED: ${vREDACTED`)
    .join('\n')
REDACTED

function parseHeaders(raw: string): { headers: Record<string, string>; error: string REDACTED {
  const result: Record<string, string> = {REDACTED
  const lines = raw.split(/\r?\n/).map((l) => l.trim()).filter(Boolean)
  for (const line of lines) {
    const idx = line.indexOf(':')
    if (idx <= 0) {
      return { headers: {REDACTED, error: t('admin.channelMonitor.advanced.headersParseError', { line REDACTED) REDACTED
    REDACTED
    const key = line.slice(0, idx).trim()
    const value = line.slice(idx + 1).trim()
    if (!key) {
      return { headers: {REDACTED, error: t('admin.channelMonitor.advanced.headersParseError', { line REDACTED) REDACTED
    REDACTED
    result[key] = value
  REDACTED
  return { headers: result, error: '' REDACTED
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
  if (props.bodyOverrideMode === 'merge') {
    return '{\n  "system": "You are Claude Code..."\nREDACTED'
  REDACTED
  return '{\n  "model": "claude-x",\n  "messages": [{"role":"user","content":"hi"REDACTED],\n  "max_tokens": 10\nREDACTED'
REDACTED)
</script>
