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
            <Icon name="userCircle" size="md" class="text-white" :stroke-width="2" />
          </div>
          <div>
            <div class="font-semibold text-gray-900 dark:text-gray-100">{{ account.name REDACTEDREDACTED</div>
            <div class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
              <span
                class="rounded bg-gray-200 px-1.5 py-0.5 text-[10px] font-medium uppercase dark:bg-dark-500"
              >
                {{ account.type REDACTEDREDACTED
              </span>
              <span>{{ t('admin.accounts.account') REDACTEDREDACTED</span>
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
          {{ account.status REDACTEDREDACTED
        </span>
      </div>

      <div v-if="!isSoraAccount" class="space-y-1.5">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.selectTestModel') REDACTEDREDACTED
        </label>
        <Select
          v-model="selectedModelId"
          :options="availableModels"
          :disabled="loadingModels || status === 'connecting'"
          value-key="id"
          label-key="display_name"
          :placeholder="loadingModels ? t('common.loading') + '...' : t('admin.accounts.selectTestModel')"
        />
      </div>
      <div
        v-else
        class="rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-700 dark:border-blue-700 dark:bg-blue-900/20 dark:text-blue-300"
      >
        {{ t('admin.accounts.soraTestHint') REDACTEDREDACTED
      </div>

      <!-- Terminal Output -->
      <div class="group relative">
        <div
          ref="terminalRef"
          class="max-h-[240px] min-h-[120px] overflow-y-auto rounded-xl border border-gray-700 bg-gray-900 p-4 font-mono text-sm dark:border-gray-800 dark:bg-black"
        >
          <!-- Status Line -->
          <div v-if="status === 'idle'" class="flex items-center gap-2 text-gray-500">
            <Icon name="bolt" size="sm" :stroke-width="2" />
            <span>{{ t('admin.accounts.readyToTest') REDACTEDREDACTED</span>
          </div>
          <div v-else-if="status === 'connecting'" class="flex items-center gap-2 text-yellow-400">
            <svg class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            <span>{{ t('admin.accounts.connectingToApi') REDACTEDREDACTED</span>
          </div>

          <!-- Output Lines -->
          <div v-for="(line, index) in outputLines" :key="index" :class="line.class">
            {{ line.text REDACTEDREDACTED
          </div>

          <!-- Streaming Content -->
          <div v-if="streamingContent" class="text-green-400">
            {{ streamingContent REDACTEDREDACTED<span class="animate-pulse">_</span>
          </div>

          <!-- Result Status -->
          <div
            v-if="status === 'success'"
            class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-green-400"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <span>{{ t('admin.accounts.testCompleted') REDACTEDREDACTED</span>
          </div>
          <div
            v-else-if="status === 'error'"
            class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-red-400"
          >
            <Icon name="xCircle" size="sm" :stroke-width="2" />
            <span>{{ errorMessage REDACTEDREDACTED</span>
          </div>
        </div>

        <!-- Copy Button -->
        <button
          v-if="outputLines.length > 0"
          @click="copyOutput"
          class="absolute right-2 top-2 rounded-lg bg-gray-800/80 p-1.5 text-gray-400 opacity-0 transition-all hover:bg-gray-700 hover:text-white group-hover:opacity-100"
          :title="t('admin.accounts.copyOutput')"
        >
          <Icon name="copy" size="sm" :stroke-width="2" />
        </button>
      </div>

      <!-- Test Info -->
      <div class="flex items-center justify-between px-1 text-xs text-gray-500 dark:text-gray-400">
        <div class="flex items-center gap-3">
          <span class="flex items-center gap-1">
            <Icon name="cpu" size="sm" :stroke-width="2" />
            {{ isSoraAccount ? t('admin.accounts.soraTestTarget') : t('admin.accounts.testModel') REDACTEDREDACTED
          </span>
        </div>
        <span class="flex items-center gap-1">
          <Icon name="chatBubble" size="sm" :stroke-width="2" />
          {{ isSoraAccount ? t('admin.accounts.soraTestMode') : t('admin.accounts.testPrompt') REDACTEDREDACTED
        </span>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          :disabled="status === 'connecting'"
        >
          {{ t('common.close') REDACTEDREDACTED
        </button>
        <button
          @click="startTest"
          :disabled="status === 'connecting' || (!isSoraAccount && !selectedModelId)"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            status === 'connecting' || (!isSoraAccount && !selectedModelId)
              ? 'cursor-not-allowed bg-primary-400 text-white'
              : status === 'success'
                ? 'bg-green-500 text-white hover:bg-green-600'
                : status === 'error'
                  ? 'bg-orange-500 text-white hover:bg-orange-600'
                  : 'bg-primary-500 text-white hover:bg-primary-600'
          ]"
        >
          <svg
            v-if="status === 'connecting'"
            class="h-4 w-4 animate-spin"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          <svg
            v-else-if="status === 'idle'"
            class="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"
            />
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
          <svg v-else class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
            />
          </svg>
          <span>
            {{
              status === 'connecting'
                ? t('admin.accounts.testing')
                : status === 'idle'
                  ? t('admin.accounts.startTest')
                  : t('admin.accounts.retry')
            REDACTEDREDACTED
          </span>
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard REDACTED from '@/composables/useClipboard'
import { adminAPI REDACTED from '@/api/admin'
import type { Account, ClaudeModel REDACTED from '@/types'

const { t REDACTED = useI18n()
const { copyToClipboard REDACTED = useClipboard()

interface OutputLine {
  text: string
  class: string
REDACTED

const props = defineProps<{
  show: boolean
  account: Account | null
REDACTED>()

const emit = defineEmits<{
  (e: 'close'): void
REDACTED>()

const terminalRef = ref<HTMLElement | null>(null)
const status = ref<'idle' | 'connecting' | 'success' | 'error'>('idle')
const outputLines = ref<OutputLine[]>([])
const streamingContent = ref('')
const errorMessage = ref('')
const availableModels = ref<ClaudeModel[]>([])
const selectedModelId = ref('')
const loadingModels = ref(false)
let eventSource: EventSource | null = null
const isSoraAccount = computed(() => props.account?.platform === 'sora')

// Load available models when modal opens
watch(
  () => props.show,
  async (newVal) => {
    if (newVal && props.account) {
      resetState()
      await loadAvailableModels()
    REDACTED else {
      closeEventSource()
    REDACTED
  REDACTED
)

const loadAvailableModels = async () => {
  if (!props.account) return
  if (props.account.platform === 'sora') {
    availableModels.value = []
    selectedModelId.value = ''
    loadingModels.value = false
    return
  REDACTED

  loadingModels.value = true
  selectedModelId.value = '' // Reset selection before loading
  try {
    availableModels.value = await adminAPI.accounts.getAvailableModels(props.account.id)
    // Default selection by platform
    if (availableModels.value.length > 0) {
      if (props.account.platform === 'gemini') {
        const preferred =
          availableModels.value.find((m) => m.id === 'gemini-2.0-flash') ||
          availableModels.value.find((m) => m.id === 'gemini-2.5-flash') ||
          availableModels.value.find((m) => m.id === 'gemini-2.5-pro') ||
          availableModels.value.find((m) => m.id === 'gemini-3-flash-preview') ||
          availableModels.value.find((m) => m.id === 'gemini-3-pro-preview')
        selectedModelId.value = preferred?.id || availableModels.value[0].id
      REDACTED else {
        // Try to select Sonnet as default, otherwise use first model
        const sonnetModel = availableModels.value.find((m) => m.id.includes('sonnet'))
        selectedModelId.value = sonnetModel?.id || availableModels.value[0].id
      REDACTED
    REDACTED
  REDACTED catch (error) {
    console.error('Failed to load available models:', error)
    // Fallback to empty list
    availableModels.value = []
    selectedModelId.value = ''
  REDACTED finally {
    loadingModels.value = false
  REDACTED
REDACTED

const resetState = () => {
  status.value = 'idle'
  outputLines.value = []
  streamingContent.value = ''
  errorMessage.value = ''
REDACTED

const handleClose = () => {
  // 防止在连接测试进行中关闭对话框
  if (status.value === 'connecting') {
    return
  REDACTED
  closeEventSource()
  emit('close')
REDACTED

const closeEventSource = () => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  REDACTED
REDACTED

const addLine = (text: string, className: string = 'text-gray-300') => {
  outputLines.value.push({ text, class: className REDACTED)
  scrollToBottom()
REDACTED

const scrollToBottom = async () => {
  await nextTick()
  if (terminalRef.value) {
    terminalRef.value.scrollTop = terminalRef.value.scrollHeight
  REDACTED
REDACTED

const startTest = async () => {
  if (!props.account || (!isSoraAccount.value && !selectedModelId.value)) return

  resetState()
  status.value = 'connecting'
  addLine(t('admin.accounts.startingTestForAccount', { name: props.account.name REDACTED), 'text-blue-400')
  addLine(t('admin.accounts.testAccountTypeLabel', { type: props.account.type REDACTED), 'text-gray-400')
  addLine('', 'text-gray-300')

  closeEventSource()

  try {
    // Create EventSource for SSE
    const url = `/api/v1/admin/accounts/${props.account.idREDACTED/test`

    // Use fetch with streaming for SSE since EventSource doesn't support POST
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${localStorage.getItem('auth_token')REDACTED`,
        'Content-Type': 'application/json'
      REDACTED,
      body: JSON.stringify(
        isSoraAccount.value ? {REDACTED : { model_id: selectedModelId.value REDACTED
      )
    REDACTED)

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.statusREDACTED`)
    REDACTED

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error('No response body')
    REDACTED

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value REDACTED = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true REDACTED)
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const jsonStr = line.slice(6).trim()
          if (jsonStr) {
            try {
              const event = JSON.parse(jsonStr)
              handleEvent(event)
            REDACTED catch (e) {
              console.error('Failed to parse SSE event:', e)
            REDACTED
          REDACTED
        REDACTED
      REDACTED
    REDACTED
  REDACTED catch (error: any) {
    status.value = 'error'
    errorMessage.value = error.message || 'Unknown error'
    addLine(`Error: ${errorMessage.valueREDACTED`, 'text-red-400')
  REDACTED
REDACTED

const handleEvent = (event: {
  type: string
  text?: string
  model?: string
  success?: boolean
  error?: string
REDACTED) => {
  switch (event.type) {
    case 'test_start':
      addLine(t('admin.accounts.connectedToApi'), 'text-green-400')
      if (event.model) {
        addLine(t('admin.accounts.usingModel', { model: event.model REDACTED), 'text-cyan-400')
      REDACTED
      addLine(
        isSoraAccount.value ? t('admin.accounts.soraTestingFlow') : t('admin.accounts.sendingTestMessage'),
        'text-gray-400'
      )
      addLine('', 'text-gray-300')
      addLine(t('admin.accounts.response'), 'text-yellow-400')
      break

    case 'content':
      if (event.text) {
        streamingContent.value += event.text
        scrollToBottom()
      REDACTED
      break

    case 'test_complete':
      // Move streaming content to output lines
      if (streamingContent.value) {
        addLine(streamingContent.value, 'text-green-300')
        streamingContent.value = ''
      REDACTED
      if (event.success) {
        status.value = 'success'
      REDACTED else {
        status.value = 'error'
        errorMessage.value = event.error || 'Test failed'
      REDACTED
      break

    case 'error':
      status.value = 'error'
      errorMessage.value = event.error || 'Unknown error'
      if (streamingContent.value) {
        addLine(streamingContent.value, 'text-green-300')
        streamingContent.value = ''
      REDACTED
      break
  REDACTED
REDACTED

const copyOutput = () => {
  const text = outputLines.value.map((l) => l.text).join('\n')
  copyToClipboard(text, t('admin.accounts.outputCopied'))
REDACTED
</script>
