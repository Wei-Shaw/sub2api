<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="card flex min-h-0 flex-1 flex-col overflow-hidden">
      <IpGeoBatchToolbar :ips="rows.map((r) => r.client_ip)" @failed="emit('ipGeoBatchFailed')" />

      <DataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        clickable-rows
        server-side-sort
        default-sort-key="created_at"
        default-sort-order="desc"
        @sort="onSort"
        @rowClick="(row) => emit('openErrorDetail', row.id)"
      >
        <template #cell-created_at="{ row REDACTED">
          <span
            class="text-sm text-gray-600 dark:text-gray-400"
            :title="row.request_id || row.client_request_id"
          >{{ formatDateTime(row.created_at) REDACTEDREDACTED</span>
        </template>

        <template #cell-type="{ row REDACTED">
          <span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="getTypeBadge(row).className">
            {{ getTypeBadge(row).label REDACTEDREDACTED
          </span>
        </template>

        <template #cell-endpoint="{ row REDACTED">
          <div class="max-w-[320px] space-y-1 text-xs">
            <div class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('usage.inbound') REDACTEDREDACTED:</span>
              <span class="ml-1">{{ row.inbound_endpoint?.trim() || '-' REDACTEDREDACTED</span>
            </div>
            <div v-if="row.upstream_endpoint" class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('usage.upstream') REDACTEDREDACTED:</span>
              <span class="ml-1">{{ row.upstream_endpoint?.trim() || '-' REDACTEDREDACTED</span>
            </div>
          </div>
        </template>

        <template #cell-platform="{ row REDACTED">
          <span class="text-sm text-gray-900 dark:text-white">{{ row.platform || '-' REDACTEDREDACTED</span>
        </template>

        <template #cell-model="{ row REDACTED">
          <div v-if="hasModelMapping(row)" class="space-y-0.5 text-xs">
            <div class="break-all font-medium text-gray-900 dark:text-white">{{ row.requested_model REDACTEDREDACTED</div>
            <div class="break-all text-gray-500 dark:text-gray-400"><span class="mr-0.5">↳</span>{{ row.upstream_model REDACTEDREDACTED</div>
          </div>
          <span v-else-if="displayModel(row)" class="text-sm font-medium text-gray-900 dark:text-white">{{ displayModel(row) REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-group="{ row REDACTED">
          <span
            v-if="row.group_id"
            class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200"
            :title="t('admin.ops.errorLog.id') + ' ' + row.group_id"
          >
            {{ row.group_name || '#' + row.group_id REDACTEDREDACTED
          </span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-user="{ row REDACTED">
          <div v-if="row.user_id" class="text-sm">
            <button
              v-if="userClickable && row.user_email"
              class="font-medium text-primary-600 underline decoration-dashed underline-offset-2 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
              :title="t('admin.usage.clickToViewBalance')"
              @click.stop="emit('userClick', row.user_id, row.user_email)"
            >
              {{ row.user_email REDACTEDREDACTED
            </button>
            <span v-else class="font-medium text-gray-900 dark:text-white">{{ row.user_email || '-' REDACTEDREDACTED</span>
            <span class="ml-1 text-gray-500 dark:text-gray-400">#{{ row.user_id REDACTEDREDACTED</span>
          </div>
          <!-- 认证失败行 user_id 为空:回退显示已删除 KEY 所有者(归因快照,与详情弹窗一致) -->
          <div v-else-if="row.deleted_key_owner_user_id" class="text-sm">
            <button
              v-if="userClickable && row.deleted_key_owner_email"
              class="font-medium text-primary-600 underline decoration-dashed underline-offset-2 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
              :title="t('admin.usage.clickToViewBalance')"
              @click.stop="emit('userClick', row.deleted_key_owner_user_id, row.deleted_key_owner_email ?? undefined)"
            >
              {{ row.deleted_key_owner_email REDACTEDREDACTED
            </button>
            <span v-else class="font-medium text-gray-900 dark:text-white">{{ row.deleted_key_owner_email || '-' REDACTEDREDACTED</span>
            <span class="ml-1 text-gray-500 dark:text-gray-400">#{{ row.deleted_key_owner_user_id REDACTEDREDACTED</span>
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-api_key="{ row REDACTED">
          <div v-if="row.api_key_id || row.api_key_name" class="text-sm">
            <span class="text-gray-900 dark:text-white">{{ row.api_key_name || '#' + row.api_key_id REDACTEDREDACTED</span>
            <span
              v-if="row.api_key_deleted"
              class="ml-1 inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-100 text-rose-600 ring-1 ring-inset ring-rose-200 dark:bg-rose-500/20 dark:text-rose-400 dark:ring-rose-500/30"
            >{{ t('admin.ops.errorLog.keyDeletedBadge') REDACTEDREDACTED</span>
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-account="{ row REDACTED">
          <span
            v-if="row.account_id"
            class="text-sm text-gray-900 dark:text-white"
            :title="t('admin.ops.errorLog.accountId') + ' ' + row.account_id"
          >{{ row.account_name || '#' + row.account_id REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-category="{ row REDACTED">
          <span class="text-sm text-gray-900 dark:text-white">
            {{ t('usage.errors.categories.' + mapErrorCategory(row.phase, row.type)) REDACTEDREDACTED
          </span>
        </template>

        <template #cell-status="{ row REDACTED">
          <div class="flex items-center gap-1.5">
            <span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="getStatusClass(row.status_code)">
              {{ row.status_code REDACTEDREDACTED
            </span>
            <span
              v-if="row.severity"
              :class="['rounded px-1.5 py-0.5 text-[10px] font-medium', getSeverityClass(row.severity)]"
            >{{ row.severity REDACTEDREDACTED</span>
            <span
              v-if="row.request_type != null && row.request_type > 0"
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-200"
            >{{ formatRequestType(row.request_type) REDACTEDREDACTED</span>
          </div>
        </template>

        <template #cell-message="{ row REDACTED">
          <span
            v-if="row.message"
            class="block max-w-[280px] truncate text-sm text-gray-600 dark:text-gray-400"
            :title="row.message"
          >{{ formatSmartMessage(row.message) || '-' REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-user_agent="{ row REDACTED">
          <span
            v-if="row.user_agent"
            class="block max-w-[320px] truncate text-sm text-gray-600 dark:text-gray-400"
            :title="row.user_agent"
          >{{ row.user_agent REDACTEDREDACTED</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-client_ip="{ row REDACTED">
          <div @click.stop>
            <div v-if="row.client_ip">
              <span class="text-sm font-mono text-gray-600 dark:text-gray-400">{{ row.client_ip REDACTEDREDACTED</span>
              <IpGeoCell :ip="row.client_ip" />
            </div>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </div>
        </template>

        <template #cell-actions="{ row REDACTED">
          <button
            type="button"
            class="rounded p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-600 dark:hover:text-primary-400"
            :title="t('admin.ops.errorLog.details')"
            @click.stop="emit('openErrorDetail', row.id)"
          >
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" /></svg>
          </button>
        </template>

        <template #empty><EmptyState :message="t('admin.ops.errorLog.noErrors')" /></template>
      </DataTable>
    </div>

    <div class="flex-shrink-0">
      <Pagination
        v-if="total > 0"
        :total="total"
        :page="page"
        :page-size="pageSize"
        @update:page="emit('update:page', $event)"
        @update:pageSize="emit('update:pageSize', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import IpGeoCell from '@/components/common/IpGeoCell.vue'
import IpGeoBatchToolbar from '@/components/common/IpGeoBatchToolbar.vue'
import type { OpsErrorLog REDACTED from '@/api/admin/ops'
import type { Column REDACTED from '@/components/common/types'
import { getSeverityClass, formatDateTime REDACTED from '../utils/opsFormatters'
import { mapErrorCategory REDACTED from '@/utils/errorCategory'
import { mapErrorSortKey, statusCodeBadgeClass REDACTED from '@/utils/errorBadges'

const { t REDACTED = useI18n()

// 列序对齐管理端用量明细:身份(用户→Key→账号)→ 请求形态(平台→模型→端点→分组→类型)
// → 结果(状态→消息)→ 时间→UA→IP→操作
const allColumns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.ops.errorLog.user') REDACTED,
  { key: 'api_key', label: t('admin.ops.errorLog.apiKey') REDACTED,
  { key: 'account', label: t('admin.ops.errorLog.account') REDACTED,
  { key: 'platform', label: t('admin.ops.errorLog.platform') REDACTED,
  { key: 'model', label: t('admin.ops.errorLog.model'), sortable: true REDACTED,
  { key: 'endpoint', label: t('admin.ops.errorLog.endpoint') REDACTED,
  { key: 'group', label: t('admin.ops.errorLog.group') REDACTED,
  { key: 'type', label: t('admin.ops.errorLog.type') REDACTED,
  { key: 'category', label: t('usage.errors.category') REDACTED,
  { key: 'status', label: t('admin.ops.errorLog.status'), sortable: true REDACTED,
  { key: 'message', label: t('admin.ops.errorLog.message') REDACTED,
  { key: 'created_at', label: t('admin.ops.errorLog.time'), sortable: true REDACTED,
  { key: 'user_agent', label: t('usage.userAgent') REDACTED,
  { key: 'client_ip', label: t('admin.ops.errorLog.ip') REDACTED,
  { key: 'actions', label: t('admin.ops.errorLog.action') REDACTED,
])

// 传入 visibleColumnKeys 时按其过滤(列设置);未传则全量(Ops 弹窗等使用方)
const columns = computed<Column[]>(() =>
  props.visibleColumnKeys
    ? allColumns.value.filter((c) => props.visibleColumnKeys!.includes(c.key))
    : allColumns.value
)

function isUpstreamRow(log: OpsErrorLog): boolean {
  const phase = String(log.phase || '').toLowerCase()
  const owner = String(log.error_owner || '').toLowerCase()
  return phase === 'upstream' && owner === 'provider'
REDACTED

function hasModelMapping(log: OpsErrorLog): boolean {
  const requested = String(log.requested_model || '').trim()
  const upstream = String(log.upstream_model || '').trim()
  return !!requested && !!upstream && requested !== upstream
REDACTED

function displayModel(log: OpsErrorLog): string {
  const upstream = String(log.upstream_model || '').trim()
  if (upstream) return upstream
  const requested = String(log.requested_model || '').trim()
  if (requested) return requested
  return String(log.model || '').trim()
REDACTED

function formatRequestType(type: number | null | undefined): string {
  switch (type) {
    case 1: return t('admin.ops.errorLog.requestTypeSync')
    case 2: return t('admin.ops.errorLog.requestTypeStream')
    case 3: return t('admin.ops.errorLog.requestTypeWs')
    default: return ''
  REDACTED
REDACTED

// 徽章配色对齐用量明细(UsageTable)的 bg-X-100/text-X-800 体系
function getTypeBadge(log: OpsErrorLog): { label: string; className: string REDACTED {
  const phase = String(log.phase || '').toLowerCase()
  const owner = String(log.error_owner || '').toLowerCase()

  if (isUpstreamRow(log)) {
    return { label: t('admin.ops.errorLog.typeUpstream'), className: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200' REDACTED
  REDACTED
  if (phase === 'request' && owner === 'client') {
    return { label: t('admin.ops.errorLog.typeRequest'), className: 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200' REDACTED
  REDACTED
  if (phase === 'auth' && owner === 'client') {
    return { label: t('admin.ops.errorLog.typeAuth'), className: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200' REDACTED
  REDACTED
  if (phase === 'routing' && owner === 'platform') {
    return { label: t('admin.ops.errorLog.typeRouting'), className: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200' REDACTED
  REDACTED
  if (phase === 'internal' && owner === 'platform') {
    return { label: t('admin.ops.errorLog.typeInternal'), className: 'bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-200' REDACTED
  REDACTED

  const fallback = phase || owner || t('common.unknown')
  return { label: fallback, className: 'bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-200' REDACTED
REDACTED

interface Props {
  rows: OpsErrorLog[]
  total: number
  loading: boolean
  page: number
  pageSize: number
  /** 用户邮箱可点击(emit userClick),仅在有弹窗承接的使用方开启 */
  userClickable?: boolean
  /** 列设置:仅显示这些 key 的列;不传则全量 */
  visibleColumnKeys?: string[]
REDACTED

interface Emits {
  (e: 'openErrorDetail', id: number): void
  (e: 'update:page', value: number): void
  (e: 'update:pageSize', value: number): void
  (e: 'ipGeoBatchFailed'): void
  (e: 'sort', sortBy: string, sortOrder: 'asc' | 'desc'): void
  (e: 'userClick', userId: number, email?: string): void
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

function onSort(key: string, order: 'asc' | 'desc') {
  emit('sort', mapErrorSortKey(key), order)
REDACTED

const getStatusClass = statusCodeBadgeClass

function formatSmartMessage(msg: string): string {
  if (!msg) return ''

  if (msg.startsWith('{') || msg.startsWith('[')) {
    try {
      const obj = JSON.parse(msg)
      if (obj?.error?.message) return String(obj.error.message)
      if (obj?.message) return String(obj.message)
      if (obj?.detail) return String(obj.detail)
      if (typeof obj === 'object') return JSON.stringify(obj).substring(0, 150)
    REDACTED catch {
      // ignore parse error
    REDACTED
  REDACTED

  if (msg.includes('context deadline exceeded')) return t('admin.ops.errorLog.commonErrors.contextDeadlineExceeded')
  if (msg.includes('connection refused')) return t('admin.ops.errorLog.commonErrors.connectionRefused')
  if (msg.toLowerCase().includes('rate limit')) return t('admin.ops.errorLog.commonErrors.rateLimit')

  return msg.length > 200 ? msg.substring(0, 200) + '...' : msg
REDACTED
</script>
