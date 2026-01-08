<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <!-- Top Toolbar: Left (search + filters) / Right (actions) -->
        <div class="flex flex-wrap items-start justify-between gap-4">
          <!-- Left: Fuzzy search + filters (wrap to multiple lines) -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <!-- Search -->
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.proxies.searchProxies')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <!-- Filters -->
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.protocol"
                :options="protocolOptions"
                :placeholder="t('admin.proxies.allProtocols')"
                @change="loadProxies"
              />
            </div>
            <div class="w-full sm:w-36">
              <Select
                v-model="filters.status"
                :options="statusOptions"
                :placeholder="t('admin.proxies.allStatus')"
                @change="loadProxies"
              />
            </div>
          </div>

          <!-- Right: Actions -->
          <div class="ml-auto flex flex-wrap items-center justify-end gap-3">
            <button
              @click="loadProxies"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="showCreateModal = true" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.proxies.createProxy') REDACTEDREDACTED
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="proxies" :loading="loading">
          <template #cell-name="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-protocol="{ value REDACTED">
            <span
              v-if="value"
              :class="['badge', value.startsWith('socks5') ? 'badge-primary' : 'badge-gray']"
            >
              {{ value.toUpperCase() REDACTEDREDACTED
            </span>
            <span v-else class="text-sm text-gray-400">-</span>
          </template>

          <template #cell-address="{ row REDACTED">
            <code class="code text-xs">{{ row.host REDACTEDREDACTED:{{ row.port REDACTEDREDACTED</code>
          </template>

          <template #cell-status="{ value REDACTED">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-danger']">
              {{ t('admin.accounts.status.' + value) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-account_count="{ value REDACTED">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ t('admin.groups.accountsCount', { count: value || 0 REDACTED) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <button
                @click="handleTestConnection(row)"
                :disabled="testingProxyIds.has(row.id)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-emerald-50 hover:text-emerald-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-emerald-900/20 dark:hover:text-emerald-400"
              >
                <svg
                  v-if="testingProxyIds.has(row.id)"
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
                <Icon v-else name="checkCircle" size="sm" />
                <span class="text-xs">{{ t('admin.proxies.testConnection') REDACTEDREDACTED</span>
              </button>
              <button
                @click="handleEdit(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') REDACTEDREDACTED</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete') REDACTEDREDACTED</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.proxies.noProxiesYet')"
              :description="t('admin.proxies.createFirstProxy')"
              :action-text="t('admin.proxies.createProxy')"
              @action="showCreateModal = true"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create Proxy Modal -->
    <BaseDialog
      :show="showCreateModal"
      :title="t('admin.proxies.createProxy')"
      width="normal"
      @close="closeCreateModal"
    >
      <!-- Tab Switch -->
      <div class="mb-6 flex border-b border-gray-200 dark:border-dark-600">
        <button
          type="button"
          @click="createMode = 'standard'"
          :class="[
            '-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors',
            createMode === 'standard'
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
          ]"
        >
          <Icon name="plus" size="sm" class="mr-1.5 inline" />
          {{ t('admin.proxies.standardAdd') REDACTEDREDACTED
        </button>
        <button
          type="button"
          @click="createMode = 'batch'"
          :class="[
            '-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors',
            createMode === 'batch'
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
          ]"
        >
          <svg
            class="mr-1.5 inline h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="1.5"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M3.75 12h16.5m-16.5 3.75h16.5M3.75 19.5h16.5M5.625 4.5h12.75a1.875 1.875 0 010 3.75H5.625a1.875 1.875 0 010-3.75z"
            />
          </svg>
          {{ t('admin.proxies.batchAdd') REDACTEDREDACTED
        </button>
      </div>

      <!-- Standard Add Form -->
      <form
        v-if="createMode === 'standard'"
        id="create-proxy-form"
        @submit.prevent="handleCreateProxy"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.proxies.name') REDACTEDREDACTED</label>
          <input
            v-model="createForm.name"
            type="text"
            required
            class="input"
            :placeholder="t('admin.proxies.enterProxyName')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.protocol') REDACTEDREDACTED</label>
          <Select v-model="createForm.protocol" :options="protocolSelectOptions" />
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.proxies.host') REDACTEDREDACTED</label>
            <input
              v-model="createForm.host"
              type="text"
              required
              :placeholder="t('admin.proxies.form.hostPlaceholder')"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.proxies.port') REDACTEDREDACTED</label>
            <input
              v-model.number="createForm.port"
              type="number"
              required
              min="1"
              max="65535"
              :placeholder="t('admin.proxies.form.portPlaceholder')"
              class="input"
            />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.username') REDACTEDREDACTED</label>
          <input
            v-model="createForm.username"
            type="text"
            class="input"
            :placeholder="t('admin.proxies.optionalAuth')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.password') REDACTEDREDACTED</label>
          <input
            v-model="createForm.password"
            type="password"
            class="input"
            :placeholder="t('admin.proxies.optionalAuth')"
          />
        </div>

      </form>

      <!-- Batch Add Form -->
      <div v-else class="space-y-5">
        <div>
          <label class="input-label">{{ t('admin.proxies.batchInput') REDACTEDREDACTED</label>
          <textarea
            v-model="batchInput"
            rows="10"
            class="input font-mono text-sm"
            :placeholder="t('admin.proxies.batchInputPlaceholder')"
            @input="parseBatchInput"
          ></textarea>
          <p class="input-hint mt-2">
            {{ t('admin.proxies.batchInputHint') REDACTEDREDACTED
          </p>
        </div>

        <!-- Parse Result -->
        <div v-if="batchParseResult.total > 0" class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
            <div class="flex items-center gap-4 text-sm">
              <div class="flex items-center gap-1.5">
              <Icon name="checkCircle" size="sm" :stroke-width="2" class="text-primary-500" />
              <span class="text-gray-700 dark:text-gray-300">
                {{ t('admin.proxies.parsedCount', { count: batchParseResult.valid REDACTED) REDACTEDREDACTED
              </span>
            </div>
            <div v-if="batchParseResult.invalid > 0" class="flex items-center gap-1.5">
              <Icon
                name="exclamationCircle"
                size="sm"
                :stroke-width="2"
                class="text-amber-500"
              />
              <span class="text-amber-600 dark:text-amber-400">
                {{ t('admin.proxies.invalidCount', { count: batchParseResult.invalid REDACTED) REDACTEDREDACTED
              </span>
            </div>
            <div v-if="batchParseResult.duplicate > 0" class="flex items-center gap-1.5">
              <svg
                class="h-4 w-4 text-gray-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="2"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M15.75 17.25v3.375c0 .621-.504 1.125-1.125 1.125h-9.75a1.125 1.125 0 01-1.125-1.125V7.875c0-.621.504-1.125 1.125-1.125H6.75a9.06 9.06 0 011.5.124m7.5 10.376h3.375c.621 0 1.125-.504 1.125-1.125V11.25c0-4.46-3.243-8.161-7.5-8.876a9.06 9.06 0 00-1.5-.124H9.375c-.621 0-1.125.504-1.125 1.125v3.5m7.5 10.375H9.375a1.125 1.125 0 01-1.125-1.125v-9.25m12 6.625v-1.875a3.375 3.375 0 00-3.375-3.375h-1.5a1.125 1.125 0 01-1.125-1.125v-1.5a3.375 3.375 0 00-3.375-3.375H9.75"
                />
              </svg>
              <span class="text-gray-500 dark:text-gray-400">
                {{ t('admin.proxies.duplicateCount', { count: batchParseResult.duplicate REDACTED) REDACTEDREDACTED
              </span>
            </div>
          </div>
        </div>

      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeCreateModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            v-if="createMode === 'standard'"
            type="submit"
            form="create-proxy-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
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
            {{ submitting ? t('admin.proxies.creating') : t('common.create') REDACTEDREDACTED
          </button>
          <button
            v-else
            @click="handleBatchCreate"
            type="button"
            :disabled="submitting || batchParseResult.valid === 0"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
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
            {{
              submitting
                ? t('admin.proxies.importing')
                : t('admin.proxies.importProxies', { count: batchParseResult.valid REDACTED)
            REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Edit Proxy Modal -->
    <BaseDialog
      :show="showEditModal"
      :title="t('admin.proxies.editProxy')"
      width="normal"
      @close="closeEditModal"
    >
      <form
        v-if="editingProxy"
        id="edit-proxy-form"
        @submit.prevent="handleUpdateProxy"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.proxies.name') REDACTEDREDACTED</label>
          <input v-model="editForm.name" type="text" required class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.protocol') REDACTEDREDACTED</label>
          <Select v-model="editForm.protocol" :options="protocolSelectOptions" />
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.proxies.host') REDACTEDREDACTED</label>
            <input v-model="editForm.host" type="text" required class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.proxies.port') REDACTEDREDACTED</label>
            <input
              v-model.number="editForm.port"
              type="number"
              required
              min="1"
              max="65535"
              class="input"
            />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.username') REDACTEDREDACTED</label>
          <input v-model="editForm.username" type="text" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.password') REDACTEDREDACTED</label>
          <input
            v-model="editForm.password"
            type="password"
            :placeholder="t('admin.proxies.leaveEmptyToKeep')"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxies.status') REDACTEDREDACTED</label>
          <Select v-model="editForm.status" :options="editStatusOptions" />
        </div>

      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeEditModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            v-if="editingProxy"
            type="submit"
            form="edit-proxy-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
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
            {{ submitting ? t('admin.proxies.updating') : t('common.update') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.proxies.deleteProxy')"
      :message="t('admin.proxies.deleteConfirm', { name: deletingProxy?.name REDACTED)"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { Proxy, ProxyProtocol REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t REDACTED = useI18n()
const appStore = useAppStore()

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.proxies.columns.name'), sortable: true REDACTED,
  { key: 'protocol', label: t('admin.proxies.columns.protocol'), sortable: true REDACTED,
  { key: 'address', label: t('admin.proxies.columns.address'), sortable: false REDACTED,
  { key: 'account_count', label: t('admin.proxies.columns.accounts'), sortable: true REDACTED,
  { key: 'status', label: t('admin.proxies.columns.status'), sortable: true REDACTED,
  { key: 'actions', label: t('admin.proxies.columns.actions'), sortable: false REDACTED
])

// Filter options
const protocolOptions = computed(() => [
  { value: '', label: t('admin.proxies.allProtocols') REDACTED,
  { value: 'http', label: 'HTTP' REDACTED,
  { value: 'https', label: 'HTTPS' REDACTED,
  { value: 'socks5', label: 'SOCKS5' REDACTED,
  { value: 'socks5h', label: 'SOCKS5H' REDACTED
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.proxies.allStatus') REDACTED,
  { value: 'active', label: t('admin.accounts.status.active') REDACTED,
  { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED
])

// Form options
const protocolSelectOptions = computed(() => [
  { value: 'http', label: t('admin.proxies.protocols.http') REDACTED,
  { value: 'https', label: t('admin.proxies.protocols.https') REDACTED,
  { value: 'socks5', label: t('admin.proxies.protocols.socks5') REDACTED,
  { value: 'socks5h', label: t('admin.proxies.protocols.socks5h') REDACTED
])

const editStatusOptions = computed(() => [
  { value: 'active', label: t('admin.accounts.status.active') REDACTED,
  { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED
])

const proxies = ref<Proxy[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({
  protocol: '',
  status: ''
REDACTED)
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
REDACTED)

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const submitting = ref(false)
const testingProxyIds = ref<Set<number>>(new Set())
const editingProxy = ref<Proxy | null>(null)
const deletingProxy = ref<Proxy | null>(null)

// Batch import state
const createMode = ref<'standard' | 'batch'>('standard')
const batchInput = ref('')
const batchParseResult = reactive({
  total: 0,
  valid: 0,
  invalid: 0,
  duplicate: 0,
  proxies: [] as Array<{
    protocol: ProxyProtocol
    host: string
    port: number
    username: string
    password: string
  REDACTED>
REDACTED)

const createForm = reactive({
  name: '',
  protocol: 'http' as ProxyProtocol,
  host: '',
  port: 8080,
  username: '',
  password: ''
REDACTED)

const editForm = reactive({
  name: '',
  protocol: 'http' as ProxyProtocol,
  host: '',
  port: 8080,
  username: '',
  password: '',
  status: 'active' as 'active' | 'inactive'
REDACTED)

let abortController: AbortController | null = null

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const maybeError = error as { name?: string; code?: string REDACTED
  return maybeError.name === 'AbortError' || maybeError.code === 'ERR_CANCELED'
REDACTED

const loadProxies = async () => {
  if (abortController) {
    abortController.abort()
  REDACTED
  const currentAbortController = new AbortController()
  abortController = currentAbortController
  loading.value = true
  try {
    const response = await adminAPI.proxies.list(pagination.page, pagination.page_size, {
      protocol: filters.protocol || undefined,
      status: filters.status as any,
      search: searchQuery.value || undefined
    REDACTED, { signal: currentAbortController.signal REDACTED)
    if (currentAbortController.signal.aborted || abortController !== currentAbortController) {
      return
    REDACTED
    proxies.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  REDACTED catch (error) {
    if (isAbortError(error)) {
      return
    REDACTED
    appStore.showError(t('admin.proxies.failedToLoad'))
    console.error('Error loading proxies:', error)
  REDACTED finally {
    if (abortController === currentAbortController) {
      loading.value = false
      abortController = null
    REDACTED
  REDACTED
REDACTED

let searchTimeout: ReturnType<typeof setTimeout>
const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadProxies()
  REDACTED, 300)
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadProxies()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadProxies()
REDACTED

const closeCreateModal = () => {
  showCreateModal.value = false
  createMode.value = 'standard'
  createForm.name = ''
  createForm.protocol = 'http'
  createForm.host = ''
  createForm.port = 8080
  createForm.username = ''
  createForm.password = ''
  batchInput.value = ''
  batchParseResult.total = 0
  batchParseResult.valid = 0
  batchParseResult.invalid = 0
  batchParseResult.duplicate = 0
  batchParseResult.proxies = []
REDACTED

// Parse proxy URL: protocol://user:pass@host:port or protocol://host:port
const parseProxyUrl = (
  line: string
): {
  protocol: ProxyProtocol
  host: string
  port: number
  username: string
  password: string
REDACTED | null => {
  const trimmed = line.trim()
  if (!trimmed) return null

  // Regex to parse proxy URL (supports http, https, socks5, socks5h)
  const regex = /^(https?|socks5h?):\/\/(?:([^:@]+):([^@]+)@)?([^:]+):(\d+)$/i
  const match = trimmed.match(regex)

  if (!match) return null

  const [, protocol, username, password, host, port] = match
  const portNum = parseInt(port, 10)

  if (portNum < 1 || portNum > 65535) return null

  return {
    protocol: protocol.toLowerCase() as ProxyProtocol,
    host: host.trim(),
    port: portNum,
    username: username?.trim() || '',
    password: password?.trim() || ''
  REDACTED
REDACTED

const parseBatchInput = () => {
  const lines = batchInput.value.split('\n').filter((l) => l.trim())
  const seen = new Set<string>()
  const proxies: typeof batchParseResult.proxies = []
  let invalid = 0
  let duplicate = 0

  for (const line of lines) {
    const parsed = parseProxyUrl(line)
    if (!parsed) {
      invalid++
      continue
    REDACTED

    // Check for duplicates (same host:port:username:password)
    const key = `${parsed.hostREDACTED:${parsed.portREDACTED:${parsed.usernameREDACTED:${parsed.passwordREDACTED`
    if (seen.has(key)) {
      duplicate++
      continue
    REDACTED
    seen.add(key)
    proxies.push(parsed)
  REDACTED

  batchParseResult.total = lines.length
  batchParseResult.valid = proxies.length
  batchParseResult.invalid = invalid
  batchParseResult.duplicate = duplicate
  batchParseResult.proxies = proxies
REDACTED

const handleBatchCreate = async () => {
  if (batchParseResult.valid === 0) return

  submitting.value = true
  try {
    const result = await adminAPI.proxies.batchCreate(batchParseResult.proxies)
    const created = result.created || 0
    const skipped = result.skipped || 0

    if (created > 0) {
      appStore.showSuccess(t('admin.proxies.batchImportSuccess', { created, skipped REDACTED))
    REDACTED else {
      appStore.showInfo(t('admin.proxies.batchImportAllSkipped', { skipped REDACTED))
    REDACTED

    closeCreateModal()
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToImport'))
    console.error('Error batch creating proxies:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleCreateProxy = async () => {
  if (!createForm.name.trim()) {
    appStore.showError(t('admin.proxies.nameRequired'))
    return
  REDACTED
  if (!createForm.host.trim()) {
    appStore.showError(t('admin.proxies.hostRequired'))
    return
  REDACTED
  if (createForm.port < 1 || createForm.port > 65535) {
    appStore.showError(t('admin.proxies.portInvalid'))
    return
  REDACTED
  submitting.value = true
  try {
    await adminAPI.proxies.create({
      name: createForm.name.trim(),
      protocol: createForm.protocol,
      host: createForm.host.trim(),
      port: createForm.port,
      username: createForm.username.trim() || null,
      password: createForm.password.trim() || null
    REDACTED)
    appStore.showSuccess(t('admin.proxies.proxyCreated'))
    closeCreateModal()
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToCreate'))
    console.error('Error creating proxy:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleEdit = (proxy: Proxy) => {
  editingProxy.value = proxy
  editForm.name = proxy.name
  editForm.protocol = proxy.protocol
  editForm.host = proxy.host
  editForm.port = proxy.port
  editForm.username = proxy.username || ''
  editForm.password = ''
  editForm.status = proxy.status
  showEditModal.value = true
REDACTED

const closeEditModal = () => {
  showEditModal.value = false
  editingProxy.value = null
REDACTED

const handleUpdateProxy = async () => {
  if (!editingProxy.value) return
  if (!editForm.name.trim()) {
    appStore.showError(t('admin.proxies.nameRequired'))
    return
  REDACTED
  if (!editForm.host.trim()) {
    appStore.showError(t('admin.proxies.hostRequired'))
    return
  REDACTED
  if (editForm.port < 1 || editForm.port > 65535) {
    appStore.showError(t('admin.proxies.portInvalid'))
    return
  REDACTED

  submitting.value = true
  try {
    const updateData: any = {
      name: editForm.name.trim(),
      protocol: editForm.protocol,
      host: editForm.host.trim(),
      port: editForm.port,
      username: editForm.username.trim() || null,
      status: editForm.status
    REDACTED

    // Only include password if it was changed
    const trimmedPassword = editForm.password.trim()
    if (trimmedPassword) {
      updateData.password = trimmedPassword
    REDACTED

    await adminAPI.proxies.update(editingProxy.value.id, updateData)
    appStore.showSuccess(t('admin.proxies.proxyUpdated'))
    closeEditModal()
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToUpdate'))
    console.error('Error updating proxy:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleTestConnection = async (proxy: Proxy) => {
  // Create new Set to trigger reactivity
  testingProxyIds.value = new Set([...testingProxyIds.value, proxy.id])
  try {
    const result = await adminAPI.proxies.testProxy(proxy.id)
    if (result.success) {
      const message = result.latency_ms
        ? t('admin.proxies.proxyWorkingWithLatency', { latency: result.latency_ms REDACTED)
        : t('admin.proxies.proxyWorking')
      appStore.showSuccess(message)
    REDACTED else {
      appStore.showError(result.message || t('admin.proxies.proxyTestFailed'))
    REDACTED
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToTest'))
    console.error('Error testing proxy:', error)
  REDACTED finally {
    // Create new Set without this proxy id to trigger reactivity
    const newSet = new Set(testingProxyIds.value)
    newSet.delete(proxy.id)
    testingProxyIds.value = newSet
  REDACTED
REDACTED

const handleDelete = (proxy: Proxy) => {
  deletingProxy.value = proxy
  showDeleteDialog.value = true
REDACTED

const confirmDelete = async () => {
  if (!deletingProxy.value) return

  try {
    await adminAPI.proxies.delete(deletingProxy.value.id)
    appStore.showSuccess(t('admin.proxies.proxyDeleted'))
    showDeleteDialog.value = false
    deletingProxy.value = null
    loadProxies()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.proxies.failedToDelete'))
    console.error('Error deleting proxy:', error)
  REDACTED
REDACTED

onMounted(() => {
  loadProxies()
REDACTED)
</script>
