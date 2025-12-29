<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex justify-end gap-3">
        <button
          @click="loadApiKeys"
          :disabled="loading"
          class="btn btn-secondary"
          :title="t('common.refresh')"
        >
          <svg
            :class="['h-5 w-5', loading ? 'animate-spin' : '']"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="1.5"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99"
            />
          </svg>
        </button>
        <button @click="showCreateModal = true" class="btn btn-primary">
          <svg
            class="mr-2 h-5 w-5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="1.5"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          {{ t('keys.createKey') REDACTEDREDACTED
        </button>
      </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="apiKeys" :loading="loading">
          <template #cell-key="{ value, row REDACTED">
            <div class="flex items-center gap-2">
              <code class="code text-xs">
                {{ maskKey(value) REDACTEDREDACTED
              </code>
              <button
                @click="copyToClipboard(value, row.id)"
                class="rounded-lg p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :class="
                  copiedKeyId === row.id
                    ? 'text-green-500'
                    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                "
                :title="copiedKeyId === row.id ? t('keys.copied') : t('keys.copyToClipboard')"
              >
                <svg
                  v-if="copiedKeyId === row.id"
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                <svg
                  v-else
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184"
                  />
                </svg>
              </button>
            </div>
          </template>

          <template #cell-name="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-group="{ row REDACTED">
            <div class="group/dropdown relative">
              <button
                :ref="(el) => setGroupButtonRef(row.id, el)"
                @click="openGroupSelector(row)"
                class="-mx-2 -my-1 flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1 transition-all duration-200 hover:bg-gray-100 dark:hover:bg-dark-700"
                :title="t('keys.clickToChangeGroup')"
              >
                <GroupBadge
                  v-if="row.group"
                  :name="row.group.name"
                  :platform="row.group.platform"
                  :subscription-type="row.group.subscription_type"
                  :rate-multiplier="row.group.rate_multiplier"
                />
                <span v-else class="text-sm text-gray-400 dark:text-dark-500">{{
                  t('keys.noGroup')
                REDACTEDREDACTED</span>
                <svg
                  class="h-3.5 w-3.5 text-gray-400 opacity-0 transition-opacity group-hover/dropdown:opacity-100"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9"
                  />
                </svg>
              </button>
            </div>
          </template>

          <template #cell-usage="{ row REDACTED">
            <div class="text-sm">
              <div class="flex items-center gap-1.5">
                <span class="text-gray-500 dark:text-gray-400">{{ t('keys.today') REDACTEDREDACTED:</span>
                <span class="font-medium text-gray-900 dark:text-white">
                  ${{ (usageStats[row.id]?.today_actual_cost ?? 0).toFixed(4) REDACTEDREDACTED
                </span>
              </div>
              <div class="mt-0.5 flex items-center gap-1.5">
                <span class="text-gray-500 dark:text-gray-400">{{ t('keys.total') REDACTEDREDACTED:</span>
                <span class="font-medium text-gray-900 dark:text-white">
                  ${{ (usageStats[row.id]?.total_actual_cost ?? 0).toFixed(4) REDACTEDREDACTED
                </span>
              </div>
            </div>
          </template>

          <template #cell-status="{ value REDACTED">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-gray']">
              {{ value REDACTEDREDACTED
            </span>
          </template>

          <template #cell-created_at="{ value REDACTED">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) REDACTEDREDACTED</span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <!-- Use Key Button -->
              <button
                @click="openUseKeyModal(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M6.75 7.5l3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0021 18V6a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 6v12a2.25 2.25 0 002.25 2.25z"
                  />
                </svg>
                <span class="text-xs">{{ t('keys.useKey') REDACTEDREDACTED</span>
              </button>
              <!-- Import to CC Switch Button -->
              <button
                @click="importToCcswitch(row.key)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5"
                  />
                </svg>
                <span class="text-xs">{{ t('keys.importToCcSwitch') REDACTEDREDACTED</span>
              </button>
              <!-- Toggle Status Button -->
              <button
                @click="toggleKeyStatus(row)"
                :class="[
                  'flex flex-col items-center gap-0.5 rounded-lg p-1.5 transition-colors',
                  row.status === 'active'
                    ? 'text-gray-500 hover:bg-yellow-50 hover:text-yellow-600 dark:hover:bg-yellow-900/20 dark:hover:text-yellow-400'
                    : 'text-gray-500 hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400'
                ]"
              >
                <svg
                  v-if="row.status === 'active'"
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636"
                  />
                </svg>
                <svg
                  v-else
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                <span class="text-xs">{{ row.status === 'active' ? t('keys.disable') : t('keys.enable') REDACTEDREDACTED</span>
              </button>
              <!-- Edit Button -->
              <button
                @click="editKey(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10"
                  />
                </svg>
                <span class="text-xs">{{ t('common.edit') REDACTEDREDACTED</span>
              </button>
              <!-- Delete Button -->
              <button
                @click="confirmDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0"
                  />
                </svg>
                <span class="text-xs">{{ t('common.delete') REDACTEDREDACTED</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('keys.noKeysYet')"
              :description="t('keys.createFirstKey')"
              :action-text="t('keys.createKey')"
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

    <!-- Create/Edit Modal -->
    <BaseDialog
      :show="showCreateModal || showEditModal"
      :title="showEditModal ? t('keys.editKey') : t('keys.createKey')"
      width="normal"
      @close="closeModals"
    >
      <form id="key-form" @submit.prevent="handleSubmit" class="space-y-5">
        <div>
          <label class="input-label">{{ t('keys.nameLabel') REDACTEDREDACTED</label>
          <input
            v-model="formData.name"
            type="text"
            required
            class="input"
            :placeholder="t('keys.namePlaceholder')"
          />
        </div>

        <div>
          <label class="input-label">{{ t('keys.groupLabel') REDACTEDREDACTED</label>
          <Select
            v-model="formData.group_id"
            :options="groupOptions"
            :placeholder="t('keys.selectGroup')"
          >
            <template #selected="{ option REDACTED">
              <GroupBadge
                v-if="option"
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
              />
              <span v-else class="text-gray-400">{{ t('keys.selectGroup') REDACTEDREDACTED</span>
            </template>
            <template #option="{ option REDACTED">
              <GroupBadge
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
              />
            </template>
          </Select>
        </div>

        <!-- Custom Key Section (only for create) -->
        <div v-if="!showEditModal" class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.customKeyLabel') REDACTEDREDACTED</label>
            <button
              type="button"
              @click="formData.use_custom_key = !formData.use_custom_key"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.use_custom_key ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.use_custom_key ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="formData.use_custom_key">
            <input
              v-model="formData.custom_key"
              type="text"
              class="input font-mono"
              :placeholder="t('keys.customKeyPlaceholder')"
              :class="{ 'border-red-500 dark:border-red-500': customKeyError REDACTED"
            />
            <p v-if="customKeyError" class="mt-1 text-sm text-red-500">{{ customKeyError REDACTEDREDACTED</p>
            <p v-else class="input-hint">{{ t('keys.customKeyHint') REDACTEDREDACTED</p>
          </div>
        </div>

        <div v-if="showEditModal">
          <label class="input-label">{{ t('keys.statusLabel') REDACTEDREDACTED</label>
          <Select
            v-model="formData.status"
            :options="statusOptions"
            :placeholder="t('keys.selectStatus')"
          />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeModals" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button form="key-form" type="submit" :disabled="submitting" class="btn btn-primary">
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
                ? t('keys.saving')
                : showEditModal
                  ? t('common.update')
                  : t('common.create')
            REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('keys.deleteKey')"
      :message="t('keys.deleteConfirmMessage', { name: selectedKey?.name REDACTED)"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Use Key Modal -->
    <UseKeyModal
      :show="showUseKeyModal"
      :api-key="selectedKey?.key || ''"
      :base-url="publicSettings?.api_base_url || ''"
      :platform="selectedKey?.group?.platform || null"
      @close="closeUseKeyModal"
    />

    <!-- Group Selector Dropdown (Teleported to body to avoid overflow clipping) -->
    <Teleport to="body">
      <div
        v-if="groupSelectorKeyId !== null && dropdownPosition"
        ref="dropdownRef"
        class="animate-in fade-in slide-in-from-top-2 fixed z-[9999] w-64 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 duration-200 dark:bg-dark-800 dark:ring-white/10"
        :style="{ top: dropdownPosition.top + 'px', left: dropdownPosition.left + 'px' REDACTED"
      >
        <div class="max-h-64 overflow-y-auto p-1.5">
          <button
            v-for="option in groupOptions"
            :key="option.value ?? 'null'"
            @click="changeGroup(selectedKeyForGroup!, option.value)"
            :class="[
              'flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm transition-colors',
              selectedKeyForGroup?.group_id === option.value ||
              (!selectedKeyForGroup?.group_id && option.value === null)
                ? 'bg-primary-50 dark:bg-primary-900/20'
                : 'hover:bg-gray-100 dark:hover:bg-dark-700'
            ]"
          >
            <GroupBadge
              :name="option.label"
              :platform="option.platform"
              :subscription-type="option.subscriptionType"
              :rate-multiplier="option.rate"
            />
            <svg
              v-if="
                selectedKeyForGroup?.group_id === option.value ||
                (!selectedKeyForGroup?.group_id && option.value === null)
              "
              class="h-4 w-4 shrink-0 text-primary-600 dark:text-primary-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              stroke-width="2"
            >
              <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
            </svg>
          </button>
        </div>
      </div>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, type ComponentPublicInstance REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { useClipboard REDACTED from '@/composables/useClipboard'

const { t REDACTED = useI18n()
import { keysAPI, authAPI, usageAPI, userGroupsAPI REDACTED from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import UseKeyModal from '@/components/keys/UseKeyModal.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import type { ApiKey, Group, PublicSettings, SubscriptionType, GroupPlatform REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import type { BatchApiKeyUsageStats REDACTED from '@/api/usage'
import { formatDateTime REDACTED from '@/utils/format'

interface GroupOption {
  value: number
  label: string
  rate: number
  subscriptionType: SubscriptionType
  platform: GroupPlatform
REDACTED

const appStore = useAppStore()
const { copyToClipboard: clipboardCopy REDACTED = useClipboard()

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('common.name'), sortable: true REDACTED,
  { key: 'key', label: t('keys.apiKey'), sortable: false REDACTED,
  { key: 'group', label: t('keys.group'), sortable: false REDACTED,
  { key: 'usage', label: t('keys.usage'), sortable: false REDACTED,
  { key: 'status', label: t('common.status'), sortable: true REDACTED,
  { key: 'created_at', label: t('keys.created'), sortable: true REDACTED,
  { key: 'actions', label: t('common.actions'), sortable: false REDACTED
])

const apiKeys = ref<ApiKey[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
const submitting = ref(false)
const usageStats = ref<Record<string, BatchApiKeyUsageStats>>({REDACTED)

const pagination = ref({
  page: 1,
  page_size: 10,
  total: 0,
  pages: 0
REDACTED)

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const showUseKeyModal = ref(false)
const selectedKey = ref<ApiKey | null>(null)
const copiedKeyId = ref<number | null>(null)
const groupSelectorKeyId = ref<number | null>(null)
const publicSettings = ref<PublicSettings | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<{ top: number; left: number REDACTED | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())
let abortController: AbortController | null = null

// Get the currently selected key for group change
const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
REDACTED)

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  REDACTED else {
    groupButtonRefs.value.delete(keyId)
  REDACTED
REDACTED

const formData = ref({
  name: '',
  group_id: null as number | null,
  status: 'active' as 'active' | 'inactive',
  use_custom_key: false,
  custom_key: ''
REDACTED)

// 自定义Key验证
const customKeyError = computed(() => {
  if (!formData.value.use_custom_key || !formData.value.custom_key) {
    return ''
  REDACTED
  const key = formData.value.custom_key
  if (key.length < 16) {
    return t('keys.customKeyTooShort')
  REDACTED
  // 检查字符：只允许字母、数字、下划线、连字符
  if (!/^[a-zA-Z0-9_-]+$/.test(key)) {
    return t('keys.customKeyInvalidChars')
  REDACTED
  return ''
REDACTED)

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') REDACTED,
  { value: 'inactive', label: t('common.inactive') REDACTED
])

// Convert groups to Select options format with rate multiplier and subscription type
const groupOptions = computed(() =>
  groups.value.map((group) => ({
    value: group.id,
    label: group.name,
    rate: group.rate_multiplier,
    subscriptionType: group.subscription_type,
    platform: group.platform
  REDACTED))
)

const maskKey = (key: string): string => {
  if (key.length <= 12) return key
  return `${key.slice(0, 8)REDACTED...${key.slice(-4)REDACTED`
REDACTED

const copyToClipboard = async (text: string, keyId: number) => {
  const success = await clipboardCopy(text, t('keys.copied'))
  if (success) {
    copiedKeyId.value = keyId
    setTimeout(() => {
      copiedKeyId.value = null
    REDACTED, 800)
  REDACTED
REDACTED

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const { name, code REDACTED = error as { name?: string; code?: string REDACTED
  return name === 'AbortError' || code === 'ERR_CANCELED'
REDACTED

const loadApiKeys = async () => {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  const { signal REDACTED = controller
  loading.value = true
  try {
    const response = await keysAPI.list(pagination.value.page, pagination.value.page_size, {
      signal
    REDACTED)
    if (signal.aborted) return
    apiKeys.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages

    // Load usage stats for all API keys in the list
    if (response.items.length > 0) {
      const keyIds = response.items.map((k) => k.id)
      try {
        const usageResponse = await usageAPI.getDashboardApiKeysUsage(keyIds, { signal REDACTED)
        if (signal.aborted) return
        usageStats.value = usageResponse.stats
      REDACTED catch (e) {
        if (!isAbortError(e)) {
          console.error('Failed to load usage stats:', e)
        REDACTED
      REDACTED
    REDACTED
  REDACTED catch (error) {
    if (isAbortError(error)) {
      return
    REDACTED
    appStore.showError(t('keys.failedToLoad'))
  REDACTED finally {
    if (abortController === controller) {
      loading.value = false
    REDACTED
  REDACTED
REDACTED

const loadGroups = async () => {
  try {
    groups.value = await userGroupsAPI.getAvailable()
  REDACTED catch (error) {
    console.error('Failed to load groups:', error)
  REDACTED
REDACTED

const loadPublicSettings = async () => {
  try {
    publicSettings.value = await authAPI.getPublicSettings()
  REDACTED catch (error) {
    console.error('Failed to load public settings:', error)
  REDACTED
REDACTED

const openUseKeyModal = (key: ApiKey) => {
  selectedKey.value = key
  showUseKeyModal.value = true
REDACTED

const closeUseKeyModal = () => {
  showUseKeyModal.value = false
  selectedKey.value = null
REDACTED

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadApiKeys()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadApiKeys()
REDACTED

const editKey = (key: ApiKey) => {
  selectedKey.value = key
  formData.value = {
    name: key.name,
    group_id: key.group_id,
    status: key.status,
    use_custom_key: false,
    custom_key: ''
  REDACTED
  showEditModal.value = true
REDACTED

const toggleKeyStatus = async (key: ApiKey) => {
  const newStatus = key.status === 'active' ? 'inactive' : 'active'
  try {
    await keysAPI.toggleStatus(key.id, newStatus)
    appStore.showSuccess(
      newStatus === 'active' ? t('keys.keyEnabledSuccess') : t('keys.keyDisabledSuccess')
    )
    loadApiKeys()
  REDACTED catch (error) {
    appStore.showError(t('keys.failedToUpdateStatus'))
  REDACTED
REDACTED

const openGroupSelector = (key: ApiKey) => {
  if (groupSelectorKeyId.value === key.id) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  REDACTED else {
    const buttonEl = groupButtonRefs.value.get(key.id)
    if (buttonEl) {
      const rect = buttonEl.getBoundingClientRect()
      dropdownPosition.value = {
        top: rect.bottom + 4,
        left: rect.left
      REDACTED
    REDACTED
    groupSelectorKeyId.value = key.id
  REDACTED
REDACTED

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
  if (key.group_id === newGroupId) return

  try {
    await keysAPI.update(key.id, { group_id: newGroupId REDACTED)
    appStore.showSuccess(t('keys.groupChangedSuccess'))
    loadApiKeys()
  REDACTED catch (error) {
    appStore.showError(t('keys.failedToChangeGroup'))
  REDACTED
REDACTED

const closeGroupSelector = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  // Check if click is inside the dropdown or the trigger button
  if (!target.closest('.group\\/dropdown') && !dropdownRef.value?.contains(target)) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  REDACTED
REDACTED

const confirmDelete = (key: ApiKey) => {
  selectedKey.value = key
  showDeleteDialog.value = true
REDACTED

const handleSubmit = async () => {
  // Validate group_id is required
  if (formData.value.group_id === null) {
    appStore.showError(t('keys.groupRequired'))
    return
  REDACTED

  // Validate custom key if enabled
  if (!showEditModal.value && formData.value.use_custom_key) {
    if (!formData.value.custom_key) {
      appStore.showError(t('keys.customKeyRequired'))
      return
    REDACTED
    if (customKeyError.value) {
      appStore.showError(customKeyError.value)
      return
    REDACTED
  REDACTED

  submitting.value = true
  try {
    if (showEditModal.value && selectedKey.value) {
      await keysAPI.update(selectedKey.value.id, formData.value)
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    REDACTED else {
      const customKey = formData.value.use_custom_key ? formData.value.custom_key : undefined
      await keysAPI.create(formData.value.name, formData.value.group_id, customKey)
      appStore.showSuccess(t('keys.keyCreatedSuccess'))
    REDACTED
    closeModals()
    loadApiKeys()
  REDACTED catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToSave')
    appStore.showError(errorMsg)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleDelete = async () => {
  if (!selectedKey.value) return

  try {
    await keysAPI.delete(selectedKey.value.id)
    appStore.showSuccess(t('keys.keyDeletedSuccess'))
    showDeleteDialog.value = false
    loadApiKeys()
  REDACTED catch (error) {
    appStore.showError(t('keys.failedToDelete'))
  REDACTED
REDACTED

const closeModals = () => {
  showCreateModal.value = false
  showEditModal.value = false
  selectedKey.value = null
  formData.value = {
    name: '',
    group_id: null,
    status: 'active',
    use_custom_key: false,
    custom_key: ''
  REDACTED
REDACTED

const importToCcswitch = (apiKey: string) => {
  const baseUrl = publicSettings.value?.api_base_url || window.location.origin
  const usageScript = `({
    request: {
      url: "{{baseUrlREDACTEDREDACTED/v1/usage",
      method: "GET",
      headers: { "Authorization": "Bearer {{apiKeyREDACTEDREDACTED" REDACTED
    REDACTED,
    extractor: function(response) {
      return {
        isValid: response.is_active || true,
        remaining: response.balance,
        unit: "USD"
      REDACTED;
    REDACTED
  REDACTED)`
  const params = new URLSearchParams({
    resource: 'provider',
    app: 'claude',
    name: 'sub2api',
    homepage: baseUrl,
    endpoint: baseUrl,
    apiKey: apiKey,
    configFormat: 'json',
    usageEnabled: 'true',
    usageScript: btoa(usageScript),
    usageAutoInterval: '30'
  REDACTED)
  const deeplink = `ccswitch://v1/import?${params.toString()REDACTED`

  try {
    window.open(deeplink, '_self')

    // Check if the protocol handler worked by detecting if we're still focused
    setTimeout(() => {
      if (document.hasFocus()) {
        // Still focused means the protocol handler likely failed
        appStore.showError(t('keys.ccSwitchNotInstalled'))
      REDACTED
    REDACTED, 100)
  REDACTED catch (error) {
    appStore.showError(t('keys.ccSwitchNotInstalled'))
  REDACTED
REDACTED

onMounted(() => {
  loadApiKeys()
  loadGroups()
  loadPublicSettings()
  document.addEventListener('click', closeGroupSelector)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', closeGroupSelector)
REDACTED)
</script>
