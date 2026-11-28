<template>
  <div v-if="rows.length > 0" class="space-y-2">
    <div v-for="(row, index) in rows" :key="getHeaderOverrideRowKey(row)">
      <div class="flex items-center gap-2">
        <input
          v-model="row.name"
          type="text"
          class="input flex-1"
          :class="isNameError(rowErrors[index]) ? errorInputClass : ''"
          :placeholder="t('admin.accounts.headerOverride.namePlaceholder')"
          :aria-invalid="isNameError(rowErrors[index])"
          spellcheck="false"
          autocomplete="off"
        />
        <input
          v-model="row.value"
          type="text"
          class="input flex-1"
          :class="[
            isValueError(rowErrors[index]) ? errorInputClass : '',
            hasVariable(row) ? 'font-mono text-[13px]' : ''
          ]"
          :placeholder="t('admin.accounts.headerOverride.valuePlaceholder')"
          :aria-invalid="isValueError(rowErrors[index])"
          spellcheck="false"
          autocomplete="off"
          data-testid="header-override-value"
          @focus="rememberValueInput(row, $event)"
        />
        <button
          type="button"
          class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
          :aria-label="t('admin.accounts.headerOverride.removeRow')"
          :title="t('admin.accounts.headerOverride.removeRow')"
          @click="removeRow(index)"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
            />
          </svg>
        </button>
      </div>

      <p
        v-if="rowErrors[index]"
        class="mt-1 flex items-start gap-1 text-xs text-red-600 dark:text-red-400"
        data-testid="header-override-row-error"
      >
        <svg class="mt-0.5 h-3.5 w-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v3.75m0 3.75h.008M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <span>{{ t(`admin.accounts.headerOverride.${rowErrors[index]}`) }}</span>
      </p>
      <p
        v-else-if="rowPreviews[index]"
        class="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400"
        data-testid="header-override-row-preview"
      >
        <span class="flex-shrink-0">{{ t('admin.accounts.headerOverride.previewLabel') }}</span>
        <code class="truncate rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-700 dark:bg-dark-700 dark:text-gray-200">
          <span class="text-gray-500 dark:text-gray-400">{{ row.name.trim().toLowerCase() }}:</span>
          <template v-for="(part, partIndex) in rowPreviews[index]" :key="partIndex">
            <span
              v-if="part.variable"
              class="font-semibold text-primary-600 dark:text-primary-400"
              :title="t(`admin.accounts.headerOverride.variables.${part.variable}.label`)"
            >{{ part.text }}</span>
            <span v-else>{{ part.text }}</span>
          </template>
        </code>
      </p>
    </div>
  </div>

  <button
    type="button"
    class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
    @click="addRow"
  >
    <svg class="mr-1 inline h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
    </svg>
    {{ t('admin.accounts.headerOverride.addRow') }}
  </button>

  <!-- 身份上报变量 -->
  <div
    v-if="allowVariables"
    class="rounded-lg border border-gray-200 bg-gray-50/70 p-3 dark:border-dark-600 dark:bg-dark-700/40"
    data-testid="header-override-variables"
  >
    <div class="flex items-center gap-1.5">
      <svg class="h-4 w-4 text-primary-600 dark:text-primary-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7.5 3.75H6A2.25 2.25 0 003.75 6v1.5M16.5 3.75H18A2.25 2.25 0 0120.25 6v1.5m0 9V18A2.25 2.25 0 0118 20.25h-1.5m-9 0H6A2.25 2.25 0 013.75 18v-1.5M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
      </svg>
      <span class="text-sm font-medium text-gray-800 dark:text-gray-100">
        {{ t('admin.accounts.headerOverride.variablesTitle') }}
      </span>
    </div>
    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.headerOverride.variablesHint') }}
    </p>

    <div class="mt-2.5 grid grid-cols-1 gap-2 sm:grid-cols-2">
      <button
        v-for="variable in HEADER_OVERRIDE_VARIABLES"
        :key="variable"
        type="button"
        class="group flex items-start gap-2 rounded-lg border border-gray-200 bg-white px-2.5 py-2 text-left transition-colors hover:border-primary-400 hover:bg-primary-50/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-500 dark:bg-dark-800 dark:hover:border-primary-500 dark:hover:bg-primary-900/20"
        :data-testid="`header-override-variable-${variable}`"
        @mousedown.prevent
        @click="insertVariable(variable)"
      >
        <span class="min-w-0 flex-1">
          <span class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
            <code class="rounded bg-primary-50 px-1.5 py-0.5 font-mono text-[11px] font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">{{ headerOverrideVariableToken(variable) }}</code>
            <span class="text-xs font-medium text-gray-800 dark:text-gray-100">
              {{ t(`admin.accounts.headerOverride.variables.${variable}.label`) }}
            </span>
          </span>
          <span class="mt-0.5 block text-[11px] leading-4 text-gray-500 dark:text-gray-400">
            {{ t(`admin.accounts.headerOverride.variables.${variable}.desc`) }}
          </span>
        </span>
        <svg class="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-gray-300 transition-colors group-hover:text-primary-500 dark:text-dark-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
      </button>
    </div>

    <ul class="mt-2.5 space-y-1 text-[11px] leading-4 text-gray-500 dark:text-gray-400">
      <li v-for="note in VARIABLE_NOTES" :key="note" class="flex gap-1.5">
        <span class="mt-1.5 h-1 w-1 flex-shrink-0 rounded-full bg-gray-400 dark:bg-dark-300"></span>
        <span>{{ t(`admin.accounts.headerOverride.variableNotes.${note}`) }}</span>
      </li>
    </ul>
  </div>
  <p
    v-else
    class="flex items-start gap-1.5 rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:bg-dark-700/40 dark:text-gray-400"
    data-testid="header-override-variables-unavailable"
  >
    <svg class="mt-0.5 h-3.5 w-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
    </svg>
    <span>{{ t('admin.accounts.headerOverride.variablesUnavailable') }}</span>
  </p>

  <div class="flex flex-wrap gap-2">
    <HeaderOverrideJsonTools :rows="rows" @update:rows="emit('update:rows', $event)" />
  </div>

  <p class="text-xs text-gray-500 dark:text-gray-400">
    {{ t('admin.accounts.headerOverride.emptyValueHint') }}
  </p>
</template>

<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import HeaderOverrideJsonTools from './HeaderOverrideJsonTools.vue'
import {
  HEADER_OVERRIDE_VARIABLES,
  HEADER_OVERRIDE_VARIABLE_SUGGESTED_NAMES,
  getHeaderOverrideRowErrors,
  headerOverrideHasVariable,
  headerOverrideVariableToken,
  parseHeaderOverrideTemplate,
  type HeaderOverrideRow,
  type HeaderOverrideRowError,
  type HeaderOverrideVariable
} from './credentialsBuilder'

const props = withDefaults(
  defineProps<{
    rows: HeaderOverrideRow[]
    /** 账号是否允许使用身份上报变量（仅 api_key 账号） */
    allowVariables?: boolean
  }>(),
  { allowVariables: true }
)

const emit = defineEmits<{
  (e: 'update:rows', rows: HeaderOverrideRow[]): void
}>()

const { t } = useI18n()

const errorInputClass =
  'border-red-400 focus:border-red-500 focus:ring-red-500/30 dark:border-red-500/70'

const VARIABLE_NOTES = ['unresolved', 'apiKeyOnly', 'websocket'] as const

/** 预览用的示例值：只为让管理员看清最终形态，不代表真实数据 */
const VARIABLE_SAMPLES: Record<HeaderOverrideVariable, string> = {
  user_id: '10086',
  api_key_id: '2048',
  group_id: '3',
  request_id: '6f1c2d3e-8a4b-4c5d-9e0f-a1b2c3d4e5f6'
}

const getHeaderOverrideRowKey = createStableObjectKeyResolver<HeaderOverrideRow>(
  'header-override-row'
)

// 行内只提示已填写名称的行：先填值后填名是正常的输入顺序，不该中途报错
// （保存时的整体校验仍会拦截"有值无名"）。
const rowErrors = computed(() =>
  getHeaderOverrideRowErrors(props.rows, { allowVariables: props.allowVariables }).map(
    (error, index) => (props.rows[index].name.trim() ? error : null)
  )
)

const NAME_ERRORS: ReadonlySet<HeaderOverrideRowError> = new Set([
  'invalidName',
  'blockedName',
  'duplicateName'
])

const isNameError = (error: HeaderOverrideRowError | null): boolean =>
  error !== null && NAME_ERRORS.has(error)
const isValueError = (error: HeaderOverrideRowError | null): boolean =>
  error !== null && !NAME_ERRORS.has(error)

const hasVariable = (row: HeaderOverrideRow): boolean => headerOverrideHasVariable(row.value)

interface PreviewPart {
  text: string
  variable?: HeaderOverrideVariable
}

/** 含变量的行给出"发送示例"，让替换结果一目了然 */
const rowPreviews = computed<Array<PreviewPart[] | null>>(() =>
  props.rows.map((row) => {
    const value = row.value.trim()
    if (!row.name.trim() || !headerOverrideHasVariable(value)) return null
    const parts = parseHeaderOverrideTemplate(value)
    if (!parts) return null
    const preview: PreviewPart[] = [{ text: ' ' }]
    for (const part of parts) {
      preview.push(
        part.kind === 'variable'
          ? { text: VARIABLE_SAMPLES[part.name], variable: part.name }
          : { text: part.text }
      )
    }
    return preview
  })
)

// 当前聚焦的值输入框。变量按钮用 mousedown.prevent 保住焦点，
// 因此"点击时焦点仍在某个值输入框"就等价于"用户想插到这里"。
let focusedValueRow: HeaderOverrideRow | null = null
let focusedValueInput: HTMLInputElement | null = null

const rememberValueInput = (row: HeaderOverrideRow, event: FocusEvent) => {
  focusedValueRow = row
  focusedValueInput = event.target as HTMLInputElement
}

const insertVariable = async (variable: HeaderOverrideVariable) => {
  const token = headerOverrideVariableToken(variable)
  const row = focusedValueRow
  const input = focusedValueInput
  if (!row || !input || document.activeElement !== input || !props.rows.includes(row)) {
    // 没有正在编辑的值输入框：新增一行，带上建议的请求头名。
    // 建议名已被占用时：同值视为已添加（连点不产生重名行），否则留空名称由管理员自行命名。
    const suggested = HEADER_OVERRIDE_VARIABLE_SUGGESTED_NAMES[variable]
    const existing = props.rows.find((item) => item.name.trim().toLowerCase() === suggested)
    if (existing?.value.trim() === token) return
    emit('update:rows', [...props.rows, { name: existing ? '' : suggested, value: token }])
    return
  }
  const current = row.value
  const start = Math.min(input.selectionStart ?? current.length, current.length)
  const end = Math.min(input.selectionEnd ?? current.length, current.length)
  row.value = current.slice(0, start) + token + current.slice(end)
  if (!row.name.trim()) row.name = HEADER_OVERRIDE_VARIABLE_SUGGESTED_NAMES[variable]
  const caret = start + token.length
  await nextTick()
  if (input.isConnected) input.setSelectionRange(caret, caret)
}

const addRow = () => {
  emit('update:rows', [...props.rows, { name: '', value: '' }])
}

const removeRow = (index: number) => {
  emit('update:rows', props.rows.filter((_, i) => i !== index))
}
</script>
