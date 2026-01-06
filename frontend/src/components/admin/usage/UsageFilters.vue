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
              <span>{{ u.email REDACTEDREDACTED</span>
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
        <div class="w-full sm:w-auto sm:min-w-[220px]">
          <label class="input-label">{{ t('admin.usage.account') REDACTEDREDACTED</label>
          <Select v-model="filters.account_id" :options="accountOptions" searchable @change="emitChange" />
        </div>

        <!-- Stream Type Filter -->
        <div class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('usage.type') REDACTEDREDACTED</label>
          <Select v-model="filters.stream" :options="streamTypeOptions" @change="emitChange" />
        </div>

        <!-- Billing Type Filter -->
        <div class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('usage.billingType') REDACTEDREDACTED</label>
          <Select v-model="filters.billing_type" :options="billingTypeOptions" @change="emitChange" />
        </div>

        <!-- Group Filter -->
        <div class="w-full sm:w-auto sm:min-w-[200px]">
          <label class="input-label">{{ t('admin.usage.group') REDACTEDREDACTED</label>
          <Select v-model="filters.group_id" :options="groupOptions" searchable @change="emitChange" />
        </div>

        <!-- Date Range Filter -->
        <div class="w-full sm:w-auto [&_.date-picker-trigger]:w-full">
          <label class="input-label">{{ t('usage.timeRange') REDACTEDREDACTED</label>
          <DateRangePicker
            :start-date="startDate"
            :end-date="endDate"
            @update:startDate="updateStartDate"
            @update:endDate="updateEndDate"
            @change="emitChange"
          />
        </div>
      </div>

      <!-- Right: actions -->
      <div class="flex w-full flex-wrap items-center justify-end gap-3 sm:w-auto">
        <button type="button" @click="$emit('reset')" class="btn btn-secondary">
          {{ t('common.reset') REDACTEDREDACTED
        </button>
        <button type="button" @click="$emit('export')" :disabled="exporting" class="btn btn-primary">
          {{ t('usage.exportExcel') REDACTEDREDACTED
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, toRef, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { adminAPI REDACTED from '@/api/admin'
import Select, { type SelectOption REDACTED from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import type { SimpleApiKey, SimpleUser REDACTED from '@/api/admin/usage'

type ModelValue = Record<string, any>

interface Props {
  modelValue: ModelValue
  exporting: boolean
  startDate: string
  endDate: string
REDACTED

const props = defineProps<Props>()
const emit = defineEmits([
  'update:modelValue',
  'update:startDate',
  'update:endDate',
  'change',
  'reset',
  'export'
])

const { t REDACTED = useI18n()
const filters = toRef(props, 'modelValue')

const userSearchRef = ref<HTMLElement | null>(null)
const apiKeySearchRef = ref<HTMLElement | null>(null)

const userKeyword = ref('')
const userResults = ref<SimpleUser[]>([])
const showUserDropdown = ref(false)
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null

const apiKeyKeyword = ref('')
const apiKeyResults = ref<SimpleApiKey[]>([])
const showApiKeyDropdown = ref(false)
let apiKeySearchTimeout: ReturnType<typeof setTimeout> | null = null

const modelOptions = ref<SelectOption[]>([{ value: null, label: t('admin.usage.allModels') REDACTED])
const groupOptions = ref<SelectOption[]>([{ value: null, label: t('admin.usage.allGroups') REDACTED])
const accountOptions = ref<SelectOption[]>([{ value: null, label: t('admin.usage.allAccounts') REDACTED])

const streamTypeOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allTypes') REDACTED,
  { value: true, label: t('usage.stream') REDACTED,
  { value: false, label: t('usage.sync') REDACTED
])

const billingTypeOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allBillingTypes') REDACTED,
  { value: 1, label: t('usage.subscription') REDACTED,
  { value: 0, label: t('usage.balance') REDACTED
])

const emitChange = () => emit('change')

const updateStartDate = (value: string) => {
  emit('update:startDate', value)
  filters.value.start_date = value
REDACTED

const updateEndDate = (value: string) => {
  emit('update:endDate', value)
  filters.value.end_date = value
REDACTED

const debounceUserSearch = () => {
  if (userSearchTimeout) clearTimeout(userSearchTimeout)
  userSearchTimeout = setTimeout(async () => {
    if (!userKeyword.value) {
      userResults.value = []
      return
    REDACTED
    try {
      userResults.value = await adminAPI.usage.searchUsers(userKeyword.value)
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

  if (!clickedInsideUser) showUserDropdown.value = false
  if (!clickedInsideApiKey) showApiKeyDropdown.value = false
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

onMounted(async () => {
  document.addEventListener('click', onDocumentClick)

  try {
    const [gs, ms, as] = await Promise.all([
      adminAPI.groups.list(1, 1000),
      adminAPI.dashboard.getModelStats({ start_date: props.startDate, end_date: props.endDate REDACTED),
      adminAPI.accounts.list(1, 1000)
    ])

    groupOptions.value.push(...gs.items.map((g: any) => ({ value: g.id, label: g.name REDACTED)))

    accountOptions.value.push(...as.items.map((a: any) => ({ value: a.id, label: a.name REDACTED)))

    const uniqueModels = new Set<string>()
    ms.models?.forEach((s: any) => s.model && uniqueModels.add(s.model))
    modelOptions.value.push(
      ...Array.from(uniqueModels)
        .sort()
        .map((m) => ({ value: m, label: m REDACTED))
    )
  REDACTED catch {
    // Ignore filter option loading errors (page still usable)
  REDACTED
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
REDACTED)
</script>
