<template>
  <div class="card p-6">
    <!-- Toolbar: left filters (multi-line) + right actions -->
    <div class="flex flex-wrap items-end justify-between gap-4">
      <!-- Left: filters (allowed to wrap to multiple rows) -->
      <div class="flex flex-1 flex-wrap items-end gap-4">
        <!-- User Search -->
        <div ref="userSearchRef" class="usage-filter-dropdown relative w-full sm:w-auto sm:min-w-[240px]">
          <label class="input-label">{{ t('admin.usage.userFilter') REDACTEDREDACTED</label>
          <input
            v-model="userKeyword"
            type="text"
            class="input pr-8"
            :placeholder="t('admin.usage.searchUserPlaceholder')"
            @input="debounceUserSearch"
            @focus="showUserDropdown = true"
          />
          <button
            v-if="filters.user_id"
            type="button"
            @click="clearUser"
            class="absolute right-2 top-9 text-gray-400"
            aria-label="Clear user filter"
          >
            ✕
          </button>
          <div
            v-if="showUserDropdown && (userResults.length > 0 || userKeyword)"
            class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:bg-gray-800"
          >
            <button
              v-for="u in userResults"
              :key="u.id"
              type="button"
              @click="selectUser(u)"
              class="w-full px-4 py-2 text-left hover:bg-gray-100 dark:hover:bg-gray-700"
            >
              <span>{{ u.email REDACTEDREDACTED<span v-if="u.deleted" class="ml-1 text-xs text-gray-400">（{{ t('admin.usage.userDeletedBadge') REDACTEDREDACTED）</span></span>
              <span class="ml-2 text-xs text-gray-400">#{{ u.id REDACTEDREDACTED</span>
            </button>
          </div>
        </div>

        <!-- API Key Search -->
        <div ref="apiKeySearchRef" class="usage-filter-dropdown relative w-full sm:w-auto sm:min-w-[240px]">
          <label class="input-label">{{ t('usage.apiKeyFilter') REDACTEDREDACTED</label>
          <input
            v-model="apiKeyKeyword"
            type="text"
            class="input pr-8"
            :placeholder="t('admin.usage.searchApiKeyPlaceholder')"
            @input="debounceApiKeySearch"
            @focus="onApiKeyFocus"
          />
          <button
            v-if="filters.api_key_id"
            type="button"
            @click="onClearApiKey"
            class="absolute right-2 top-9 text-gray-400"
            aria-label="Clear API key filter"
          >
            ✕
          </button>
          <div
            v-if="showApiKeyDropdown && apiKeyResults.length > 0"
            class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:bg-gray-800"
          >
            <button
              v-for="k in apiKeyResults"
              :key="k.id"
              type="button"
              @click="selectApiKey(k)"
              class="w-full px-4 py-2 text-left hover:bg-gray-100 dark:hover:bg-gray-700"
            >
              <span class="truncate">{{ k.name || `#${k.idREDACTED` REDACTEDREDACTED</span>
              <span class="ml-2 text-xs text-gray-400">#{{ k.id REDACTEDREDACTED</span>
            </button>
          </div>
        </div>

        <!-- Model Filter -->
        <div class="w-full sm:w-auto sm:min-w-[220px]">
          <label class="input-label">{{ t('usage.model') REDACTEDREDACTED</label>
          <Select v-model="filters.model" :options="modelOptions" searchable @change="emitChange" />
        </div>

        <!-- Account Filter -->
        <div ref="accountSearchRef" class="usage-filter-dropdown relative w-full sm:w-auto sm:min-w-[220px]">
          <label class="input-label">{{ t('admin.usage.account') REDACTEDREDACTED</label>
          <input
            v-model="accountKeyword"
            type="text"
            class="input pr-8"
            :placeholder="t('admin.usage.searchAccountPlaceholder')"
            @input="debounceAccountSearch"
            @focus="showAccountDropdown = true"
          />
          <button
            v-if="filters.account_id"
            type="button"
            @click="clearAccount"
            class="absolute right-2 top-9 text-gray-400"
            aria-label="Clear account filter"
          >
            ✕
          </button>
          <div
            v-if="showAccountDropdown && (accountResults.length > 0 || accountKeyword)"
            class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:bg-gray-800"
          >
            <button
              v-for="a in accountResults"
              :key="a.id"
              type="button"
              @click="selectAccount(a)"
              class="w-full px-4 py-2 text-left hover:bg-gray-100 dark:hover:bg-gray-700"
            >
              <span class="truncate">{{ a.name REDACTEDREDACTED</span>
              <span class="ml-2 text-xs text-gray-400">#{{ a.id REDACTEDREDACTED</span>
            </button>
          </div>
        </div>

        <!-- Request Type Filter (usage only) -->
        <div v-if="mode !== 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('usage.type') REDACTEDREDACTED</label>
          <Select v-model="filters.request_type" :options="requestTypeOptions" @change="emitChange" />
        </div>

        <!-- Billing Type Filter (usage only) -->
        <div v-if="mode !== 'errors'" class="w-full sm:w-auto sm:min-w-[200px]">
          <label class="input-label">{{ t('admin.usage.billingType') REDACTEDREDACTED</label>
          <Select v-model="filters.billing_type" :options="billingTypeOptions" @change="emitChange" />
        </div>

        <!-- Billing Mode Filter (usage only) -->
        <div v-if="mode !== 'errors'" class="w-full sm:w-auto sm:min-w-[200px]">
          <label class="input-label">{{ t('admin.usage.billingMode') REDACTEDREDACTED</label>
          <Select v-model="filters.billing_mode" :options="billingModeOptions" @change="emitChange" />
        </div>

        <!-- Error Phase Filter (errors only) -->
        <div v-if="mode === 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('admin.ops.errorLog.type') REDACTEDREDACTED</label>
          <Select v-model="filters.error_phase" :options="errorPhaseOptions" @change="emitChange" />
        </div>

        <!-- Error Category Filter (errors only) -->
        <div v-if="mode === 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('usage.errors.category') REDACTEDREDACTED</label>
          <Select v-model="filters.error_category" :options="errorCategoryOptions" @change="emitChange" />
        </div>

        <!-- Status Code Filter (errors only) -->
        <div v-if="mode === 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('admin.ops.errorLog.status') REDACTEDREDACTED</label>
          <Select v-model="filters.status_code" :options="statusCodeOptions" @change="emitChange" />
        </div>

        <!-- Group Filter -->
        <div class="w-full sm:w-auto sm:min-w-[200px]">
          <label class="input-label">{{ t('admin.usage.group') REDACTEDREDACTED</label>
          <Select v-model="filters.group_id" :options="groupOptions" searchable @change="emitChange" />
        </div>

      </div>

      <!-- Right: actions -->
      <div v-if="showActions" class="flex w-full flex-wrap items-center justify-end gap-3 sm:w-auto">
        <button type="button" @click="$emit('refresh')" class="btn btn-secondary">
          {{ t('common.refresh') REDACTEDREDACTED
        </button>
        <button type="button" @click="$emit('reset')" class="btn btn-secondary">
          {{ t('common.reset') REDACTEDREDACTED
        </button>
        <slot name="after-reset" />
        <template v-if="mode !== 'errors'">
          <button type="button" @click="$emit('cleanup')" class="btn btn-danger">
            {{ t('admin.usage.cleanup.button') REDACTEDREDACTED
          </button>
          <button type="button" @click="$emit('export')" :disabled="exporting" class="btn btn-primary">
            {{ t('usage.exportExcel') REDACTEDREDACTED
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, toRef, watch, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { adminAPI REDACTED from '@/api/admin'
import Select, { type SelectOption REDACTED from '@/components/common/Select.vue'
import { COMMON_ERROR_STATUS_CODES REDACTED from '@/utils/errorBadges'
import type { SimpleApiKey, SimpleUser REDACTED from '@/api/admin/usage'

type ModelValue = Record<string, any>

interface Props {
  modelValue: ModelValue
  exporting: boolean
  startDate: string
  endDate: string
  showActions?: boolean
  modelOptions?: string[]
  /** errors 模式:隐藏用量专属字段/按钮,显示错误类型+状态码(错误请求 tab 用) */
  mode?: 'usage' | 'errors'
REDACTED

const props = withDefaults(defineProps<Props>(), {
  showActions: true,
  mode: 'usage'
REDACTED)
const emit = defineEmits([
  'update:modelValue',
  'change',
  'refresh',
  'reset',
  'export',
  'cleanup'
])

const { t REDACTED = useI18n()
const filters = toRef(props, 'modelValue')

const userSearchRef = ref<HTMLElement | null>(null)
const apiKeySearchRef = ref<HTMLElement | null>(null)
const accountSearchRef = ref<HTMLElement | null>(null)

const userKeyword = ref('')
const userResults = ref<SimpleUser[]>([])
const showUserDropdown = ref(false)
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null

const apiKeyKeyword = ref('')
const apiKeyResults = ref<SimpleApiKey[]>([])
const showApiKeyDropdown = ref(false)
let apiKeySearchTimeout: ReturnType<typeof setTimeout> | null = null

interface SimpleAccount {
  id: number
  name: string
REDACTED
const accountKeyword = ref('')
const accountResults = ref<SimpleAccount[]>([])
const showAccountDropdown = ref(false)
let accountSearchTimeout: ReturnType<typeof setTimeout> | null = null

const modelOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allModels') REDACTED,
  ...(props.modelOptions ?? []).map((m) => ({ value: m, label: m REDACTED)),
])
const groupOptions = ref<SelectOption[]>([{ value: null, label: t('admin.usage.allGroups') REDACTED])

const requestTypeOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allTypes') REDACTED,
  { value: 'ws_v2', label: t('usage.ws') REDACTED,
  { value: 'stream', label: t('usage.stream') REDACTED,
  { value: 'sync', label: t('usage.sync') REDACTED,
  { value: 'cyber', label: t('usage.cyber') REDACTED
])

const billingTypeOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allBillingTypes') REDACTED,
  { value: 0, label: t('admin.usage.billingTypeBalance') REDACTED,
  { value: 1, label: t('admin.usage.billingTypeSubscription') REDACTED
])

// 错误类型对应后端 phase 参数(与错误表"类型"徽章同语义)
const errorPhaseOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allTypes') REDACTED,
  { value: 'upstream', label: t('admin.ops.errorLog.typeUpstream') REDACTED,
  { value: 'request', label: t('admin.ops.errorLog.typeRequest') REDACTED,
  { value: 'auth', label: t('admin.ops.errorLog.typeAuth') REDACTED,
  { value: 'routing', label: t('admin.ops.errorLog.typeRouting') REDACTED,
  { value: 'internal', label: t('admin.ops.errorLog.typeInternal') REDACTED,
])

// 分类码同用户端 /usage 错误筛选;"other" 无法反查为过滤条件,刻意不列
const errorCategoryCodes = ['auth', 'rate_limit', 'quota', 'invalid_request', 'service_unavailable', 'upstream', 'internal', 'cyber']

const errorCategoryOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('usage.errors.allCategories') REDACTED,
  ...errorCategoryCodes.map((c) => ({ value: c, label: t('usage.errors.categories.' + c) REDACTED)),
])

const statusCodeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('usage.errors.allStatuses') REDACTED,
  ...COMMON_ERROR_STATUS_CODES.map((c) => ({ value: c, label: String(c) REDACTED)),
])

const billingModeOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allBillingModes') REDACTED,
  { value: 'token', label: t('admin.usage.billingModeToken') REDACTED,
  { value: 'per_request', label: t('admin.usage.billingModePerRequest') REDACTED,
  { value: 'image', label: t('admin.usage.billingModeImage') REDACTED,
  { value: 'video', label: t('admin.usage.billingModeVideo') REDACTED
])

const emitChange = () => emit('change')

const debounceUserSearch = () => {
  if (userSearchTimeout) clearTimeout(userSearchTimeout)
  userSearchTimeout = setTimeout(async () => {
    if (!userKeyword.value) {
      userResults.value = []
      return
    REDACTED
    try {
      const results = await adminAPI.usage.searchUsers(userKeyword.value)
      userResults.value = results.sort((a, b) => Number(a.deleted) - Number(b.deleted))
    REDACTED catch {
      userResults.value = []
    REDACTED
  REDACTED, 300)
REDACTED

const debounceApiKeySearch = () => {
  if (apiKeySearchTimeout) clearTimeout(apiKeySearchTimeout)
  apiKeySearchTimeout = setTimeout(async () => {
    try {
      apiKeyResults.value = await adminAPI.usage.searchApiKeys(
        filters.value.user_id,
        apiKeyKeyword.value || ''
      )
    REDACTED catch {
      apiKeyResults.value = []
    REDACTED
  REDACTED, 300)
REDACTED

const selectUser = async (u: SimpleUser) => {
  userKeyword.value = u.email
  showUserDropdown.value = false
  filters.value.user_id = u.id
  clearApiKey()

  // Auto-load API keys for this user
  try {
    apiKeyResults.value = await adminAPI.usage.searchApiKeys(u.id, '')
  REDACTED catch {
    apiKeyResults.value = []
  REDACTED

  emitChange()
REDACTED

const clearUser = () => {
  userKeyword.value = ''
  userResults.value = []
  showUserDropdown.value = false
  filters.value.user_id = undefined
  clearApiKey()
  emitChange()
REDACTED

const selectApiKey = (k: SimpleApiKey) => {
  apiKeyKeyword.value = k.name || String(k.id)
  showApiKeyDropdown.value = false
  filters.value.api_key_id = k.id
  emitChange()
REDACTED

const clearApiKey = () => {
  apiKeyKeyword.value = ''
  apiKeyResults.value = []
  showApiKeyDropdown.value = false
  filters.value.api_key_id = undefined
REDACTED

const onClearApiKey = () => {
  clearApiKey()
  emitChange()
REDACTED

const debounceAccountSearch = () => {
  if (accountSearchTimeout) clearTimeout(accountSearchTimeout)
  accountSearchTimeout = setTimeout(async () => {
    if (!accountKeyword.value) {
      accountResults.value = []
      return
    REDACTED
    try {
      const res = await adminAPI.accounts.list(1, 20, { search: accountKeyword.value REDACTED)
      accountResults.value = res.items.map((a) => ({ id: a.id, name: a.name REDACTED))
    REDACTED catch {
      accountResults.value = []
    REDACTED
  REDACTED, 300)
REDACTED

const selectAccount = (a: SimpleAccount) => {
  accountKeyword.value = a.name
  showAccountDropdown.value = false
  filters.value.account_id = a.id
  emitChange()
REDACTED

const clearAccount = () => {
  accountKeyword.value = ''
  accountResults.value = []
  showAccountDropdown.value = false
  filters.value.account_id = undefined
  emitChange()
REDACTED

const onApiKeyFocus = () => {
  showApiKeyDropdown.value = true
  // Trigger search if no results yet
  if (apiKeyResults.value.length === 0) {
    debounceApiKeySearch()
  REDACTED
REDACTED

const onDocumentClick = (e: MouseEvent) => {
  const target = e.target as Node | null
  if (!target) return

  const clickedInsideUser = userSearchRef.value?.contains(target) ?? false
  const clickedInsideApiKey = apiKeySearchRef.value?.contains(target) ?? false
  const clickedInsideAccount = accountSearchRef.value?.contains(target) ?? false

  if (!clickedInsideUser) showUserDropdown.value = false
  if (!clickedInsideApiKey) showApiKeyDropdown.value = false
  if (!clickedInsideAccount) showAccountDropdown.value = false
REDACTED

watch(
  () => props.startDate,
  (value) => {
    filters.value.start_date = value
  REDACTED,
  { immediate: true REDACTED
)

watch(
  () => props.endDate,
  (value) => {
    filters.value.end_date = value
  REDACTED,
  { immediate: true REDACTED
)

watch(
  () => filters.value.user_id,
  (userId) => {
    if (!userId) {
      userKeyword.value = ''
      userResults.value = []
    REDACTED
  REDACTED
)

watch(
  () => filters.value.api_key_id,
  (apiKeyId) => {
    if (!apiKeyId) {
      apiKeyKeyword.value = ''
      apiKeyResults.value = []
    REDACTED
  REDACTED
)

watch(
  () => filters.value.account_id,
  (accountId) => {
    if (!accountId) {
      accountKeyword.value = ''
      accountResults.value = []
    REDACTED
  REDACTED
)

onMounted(async () => {
  document.addEventListener('click', onDocumentClick)
  try {
    const gs = await adminAPI.groups.list(1, 1000)
    groupOptions.value.push(...gs.items.map((g: any) => ({ value: g.id, label: g.name REDACTED)))
  REDACTED catch {
    // Ignore filter option loading errors (page still usable)
  REDACTED
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
REDACTED)
</script>
