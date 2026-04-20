<template>
  <AppLayout>
    <div class="space-y-6">
      <section class="grid gap-4 md:grid-cols-3">
        <div class="card p-5">
          <p class="text-sm font-medium text-gray-500 dark:text-dark-400">
            {{ copy.total REDACTEDREDACTED
          </p>
          <p data-test="summary-total" class="mt-2 text-3xl font-semibold text-gray-900 dark:text-gray-100">
            {{ summary.total REDACTEDREDACTED
          </p>
        </div>
        <div class="card p-5">
          <p class="text-sm font-medium text-gray-500 dark:text-dark-400">
            {{ copy.open REDACTEDREDACTED
          </p>
          <p data-test="summary-open" class="mt-2 text-3xl font-semibold text-amber-600 dark:text-amber-400">
            {{ summary.open_total REDACTEDREDACTED
          </p>
        </div>
        <div class="card p-5">
          <p class="text-sm font-medium text-gray-500 dark:text-dark-400">
            {{ copy.resolved REDACTEDREDACTED
          </p>
          <p data-test="summary-resolved" class="mt-2 text-3xl font-semibold text-emerald-600 dark:text-emerald-400">
            {{ summary.resolved_total REDACTEDREDACTED
          </p>
        </div>
      </section>

      <TablePageLayout>
        <template #actions>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">
                {{ copy.title REDACTEDREDACTED
              </h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                {{ copy.subtitle REDACTEDREDACTED
              </p>
            </div>
            <button type="button" class="btn btn-secondary" :disabled="loading || resolving" @click="refreshAll">
              <Icon name="refresh" size="md" :class="loading || summaryLoading ? 'animate-spin' : ''" />
            </button>
          </div>
        </template>

        <template #filters>
          <div class="flex flex-wrap items-center gap-3">
            <div class="w-full sm:w-80">
              <label class="input-label" for="report-type-filter">{{ copy.reportType REDACTEDREDACTED</label>
              <select
                id="report-type-filter"
                v-model="filters.reportType"
                data-test="report-type-filter"
                class="input"
                @change="handleReportTypeChange"
              >
                <option value="">{{ copy.allReportTypes REDACTEDREDACTED</option>
                <option
                  v-for="option in reportTypeOptions"
                  :key="option.value"
                  :value="option.value"
                >
                  {{ option.label REDACTEDREDACTED
                </option>
              </select>
            </div>
          </div>
        </template>

        <template #table>
          <DataTable :columns="columns" :data="reports" :loading="loading">
            <template #cell-status="{ row REDACTED">
              <span :class="['badge', row.resolved_at ? 'badge-success' : 'badge-warning']">
                {{ row.resolved_at ? copy.resolvedBadge : copy.openBadge REDACTEDREDACTED
              </span>
            </template>

            <template #cell-report_type="{ value REDACTED">
              <span class="font-mono text-xs text-gray-600 dark:text-dark-300">{{ value REDACTEDREDACTED</span>
            </template>

            <template #cell-report_key="{ value REDACTED">
              <span class="font-medium text-gray-900 dark:text-gray-100">{{ value REDACTEDREDACTED</span>
            </template>

            <template #cell-details_preview="{ row REDACTED">
              <div class="flex flex-wrap gap-2">
                <span
                  v-for="entry in getDetailHighlights(row.details)"
                  :key="entry.key"
                  class="rounded-full bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-200"
                >
                  {{ entry.key REDACTEDREDACTED: {{ entry.value REDACTEDREDACTED
                </span>
              </div>
            </template>

            <template #cell-created_at="{ value REDACTED">
              <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) REDACTEDREDACTED</span>
            </template>

            <template #cell-resolved_at="{ value REDACTED">
              <span class="text-sm text-gray-500 dark:text-dark-400">
                {{ value ? formatDateTime(value) : copy.notResolved REDACTEDREDACTED
              </span>
            </template>

            <template #cell-actions="{ row REDACTED">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :data-test="`select-report-${row.idREDACTED`"
                @click="selectReport(row)"
              >
                {{ copy.viewDetails REDACTEDREDACTED
              </button>
            </template>
          </DataTable>
        </template>

        <template #pagination>
          <Pagination
            v-if="pagination.total > 0"
            :page="pagination.page"
            :page-size="pagination.pageSize"
            :total="pagination.total"
            @update:page="handlePageChange"
            @update:pageSize="handlePageSizeChange"
          />
        </template>
      </TablePageLayout>

      <section class="grid gap-6 xl:grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)]">
        <div class="card p-6">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
                {{ copy.detailTitle REDACTEDREDACTED
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                {{ selectedReport ? selectedReport.report_key : copy.selectPrompt REDACTEDREDACTED
              </p>
            </div>
            <span
              v-if="selectedReport"
              :class="['badge', selectedReport.resolved_at ? 'badge-success' : 'badge-warning']"
            >
              {{ selectedReport.resolved_at ? copy.resolvedBadge : copy.openBadge REDACTEDREDACTED
            </span>
          </div>

          <div v-if="selectedReport" class="mt-6 space-y-5">
            <dl class="grid gap-4 sm:grid-cols-2">
              <div>
                <dt class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ copy.reportType REDACTEDREDACTED</dt>
                <dd class="mt-1 break-all font-mono text-sm text-gray-900 dark:text-gray-100">{{ selectedReport.report_type REDACTEDREDACTED</dd>
              </div>
              <div>
                <dt class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ copy.reportKey REDACTEDREDACTED</dt>
                <dd class="mt-1 break-all text-sm text-gray-900 dark:text-gray-100">{{ selectedReport.report_key REDACTEDREDACTED</dd>
              </div>
              <div>
                <dt class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ copy.createdAt REDACTEDREDACTED</dt>
                <dd class="mt-1 text-sm text-gray-900 dark:text-gray-100">{{ formatDateTime(selectedReport.created_at) REDACTEDREDACTED</dd>
              </div>
              <div>
                <dt class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ copy.resolvedAt REDACTEDREDACTED</dt>
                <dd class="mt-1 text-sm text-gray-900 dark:text-gray-100">
                  {{ selectedReport.resolved_at ? formatDateTime(selectedReport.resolved_at) : copy.notResolved REDACTEDREDACTED
                </dd>
              </div>
              <div>
                <dt class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ copy.resolvedBy REDACTEDREDACTED</dt>
                <dd class="mt-1 text-sm text-gray-900 dark:text-gray-100">{{ selectedReport.resolved_by_user_id ?? '-' REDACTEDREDACTED</dd>
              </div>
              <div>
                <dt class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ copy.resolutionNote REDACTEDREDACTED</dt>
                <dd class="mt-1 whitespace-pre-wrap text-sm text-gray-900 dark:text-gray-100">
                  {{ selectedReport.resolution_note || copy.emptyResolutionNote REDACTEDREDACTED
                </dd>
              </div>
            </dl>

            <div>
              <h3 class="text-sm font-medium text-gray-700 dark:text-dark-300">{{ copy.keyFields REDACTEDREDACTED</h3>
              <div class="mt-3 flex flex-wrap gap-2">
                <span
                  v-for="entry in getDetailHighlights(selectedReport.details)"
                  :key="entry.key"
                  class="rounded-full bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-200"
                >
                  {{ entry.key REDACTEDREDACTED: {{ entry.value REDACTEDREDACTED
                </span>
              </div>
            </div>

            <div>
              <h3 class="text-sm font-medium text-gray-700 dark:text-dark-300">{{ copy.rawDetails REDACTEDREDACTED</h3>
              <pre class="mt-3 max-h-96 overflow-auto rounded-xl bg-gray-950 p-4 text-xs text-gray-100">{{ formatDetailsJson(selectedReport.details) REDACTEDREDACTED</pre>
            </div>
          </div>

          <div v-else class="mt-6 rounded-2xl border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
            {{ copy.selectPrompt REDACTEDREDACTED
          </div>
        </div>

        <div class="card p-6">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {{ copy.resolveTitle REDACTEDREDACTED
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
            {{ copy.resolveSubtitle REDACTEDREDACTED
          </p>

          <div class="mt-6 space-y-4">
            <div>
              <label class="input-label" for="resolution-note">{{ copy.resolutionNote REDACTEDREDACTED</label>
              <textarea
                id="resolution-note"
                v-model="resolutionNote"
                data-test="resolution-note"
                class="input min-h-40"
                :disabled="!selectedReport || Boolean(selectedReport.resolved_at) || resolving"
                :placeholder="copy.resolvePlaceholder"
              ></textarea>
            </div>

            <button
              type="button"
              class="btn btn-primary w-full"
              data-test="resolve-submit"
              :disabled="!canResolve"
              @click="submitResolve"
            >
              {{ resolving ? copy.resolving : copy.resolveAction REDACTEDREDACTED
            </button>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { adminAPI REDACTED from '@/api/admin'
import type {
  AuthIdentityMigrationReport,
  AuthIdentityMigrationReportSummary,
REDACTED from '@/api/admin/users'
import type { Column REDACTED from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore REDACTED from '@/stores/app'
import { formatDateTime REDACTED from '@/utils/format'

const { locale REDACTED = useI18n()
const appStore = useAppStore()

const isZh = computed(() => locale.value.toLowerCase().startsWith('zh'))
const text = (zh: string, en: string) => (isZh.value ? zh : en)

const copy = computed(() => ({
  title: text('Auth Identity Migration Reports', 'Auth Identity Migration Reports'),
  subtitle: text('处理 auth identity 迁移过程中需要人工收口的异常记录。', 'Review and resolve auth identity migration records that require manual follow-up.'),
  total: text('总报告数', 'Total reports'),
  open: text('待处理', 'Open'),
  resolved: text('已解决', 'Resolved'),
  reportType: text('报告类型', 'Report type'),
  allReportTypes: text('全部类型', 'All report types'),
  resolvedBadge: text('已解决', 'Resolved'),
  openBadge: text('待处理', 'Open'),
  notResolved: text('未解决', 'Not resolved'),
  viewDetails: text('查看', 'View'),
  detailTitle: text('报告详情', 'Report details'),
  selectPrompt: text('从列表中选择一条报告以查看详情和处理意见。', 'Select a report from the list to inspect details and submit a resolution note.'),
  reportKey: text('报告键', 'Report key'),
  createdAt: text('创建时间', 'Created at'),
  resolvedAt: text('解决时间', 'Resolved at'),
  resolvedBy: text('处理人 ID', 'Resolved by'),
  resolutionNote: text('处理备注', 'Resolution note'),
  emptyResolutionNote: text('暂无处理备注', 'No resolution note'),
  keyFields: text('关键字段', 'Key fields'),
  rawDetails: text('原始详情', 'Raw details'),
  resolveTitle: text('提交处理结果', 'Submit resolution'),
  resolveSubtitle: text('填写运营备注后提交 resolve，后端会记录处理人和处理时间。', 'Submit an operational note to resolve the selected report. The backend will record the resolver and timestamp.'),
  resolvePlaceholder: text('填写本次处理动作、用户沟通结果或后续追踪信息。', 'Describe the action taken, user communication, or follow-up context.'),
  resolveAction: text('提交 Resolve', 'Submit resolve'),
  resolving: text('提交中...', 'Submitting...'),
REDACTED))

const summary = ref<AuthIdentityMigrationReportSummary>({
  total: 0,
  open_total: 0,
  resolved_total: 0,
  by_type: {REDACTED,
REDACTED)
const reports = ref<AuthIdentityMigrationReport[]>([])
const selectedReport = ref<AuthIdentityMigrationReport | null>(null)
const resolutionNote = ref('')
const loading = ref(false)
const summaryLoading = ref(false)
const resolving = ref(false)

const filters = reactive({
  reportType: '',
REDACTED)

const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
REDACTED)

const columns: Column[] = [
  { key: 'status', label: text('状态', 'Status') REDACTED,
  { key: 'report_type', label: text('报告类型', 'Report type') REDACTED,
  { key: 'report_key', label: text('报告键', 'Report key') REDACTED,
  { key: 'details_preview', label: text('关键字段', 'Key fields') REDACTED,
  { key: 'created_at', label: text('创建时间', 'Created at') REDACTED,
  { key: 'resolved_at', label: text('解决时间', 'Resolved at') REDACTED,
  { key: 'actions', label: text('操作', 'Actions') REDACTED,
]

const reportTypeOptions = computed(() =>
  Object.entries(summary.value.by_type)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([value, count]) => ({
      value,
      label: `${valueREDACTED (${countREDACTED)`,
    REDACTED))
)

const canResolve = computed(() =>
  Boolean(
    selectedReport.value &&
    !selectedReport.value.resolved_at &&
    resolutionNote.value.trim() &&
    !resolving.value
  )
)

const loadSummary = async () => {
  summaryLoading.value = true
  try {
    summary.value = await adminAPI.users.getAuthIdentityMigrationReportSummary()
  REDACTED catch (error) {
    console.error('Failed to load auth identity migration report summary:', error)
    appStore.showError(text('加载 migration reports 汇总失败', 'Failed to load migration report summary'))
  REDACTED finally {
    summaryLoading.value = false
  REDACTED
REDACTED

const loadReports = async () => {
  loading.value = true
  try {
    const response = await adminAPI.users.listAuthIdentityMigrationReports({
      page: pagination.page,
      pageSize: pagination.pageSize,
      reportType: filters.reportType,
    REDACTED)

    reports.value = response.items
    pagination.total = response.total

    if (selectedReport.value) {
      const refreshed = response.items.find((report) => report.id === selectedReport.value?.id) ?? null
      selectedReport.value = refreshed
      resolutionNote.value = refreshed?.resolved_at
        ? refreshed.resolution_note ?? ''
        : resolutionNote.value
    REDACTED
  REDACTED catch (error) {
    console.error('Failed to load auth identity migration reports:', error)
    appStore.showError(text('加载 migration reports 列表失败', 'Failed to load migration reports'))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const refreshAll = async () => {
  await Promise.all([loadSummary(), loadReports()])
REDACTED

const handleReportTypeChange = async () => {
  pagination.page = 1
  await loadReports()
REDACTED

const handlePageChange = async (page: number) => {
  pagination.page = page
  await loadReports()
REDACTED

const handlePageSizeChange = async (pageSize: number) => {
  pagination.page = 1
  pagination.pageSize = pageSize
  await loadReports()
REDACTED

const selectReport = (report: AuthIdentityMigrationReport) => {
  selectedReport.value = report
  resolutionNote.value = report.resolution_note ?? ''
REDACTED

const formatDetailsJson = (details: Record<string, unknown>) => JSON.stringify(details ?? {REDACTED, null, 2)

const isDisplayableValue = (value: unknown) =>
  ['string', 'number', 'boolean'].includes(typeof value)

const getDetailHighlights = (details: Record<string, unknown>) => {
  const preferredKeys = [
    'user_id',
    'legacy_email',
    'provider_key',
    'provider_subject',
    'email',
    'subject',
  ]

  const entries = preferredKeys
    .filter((key) => key in details && isDisplayableValue(details[key]))
    .map((key) => ({ key, value: String(details[key]) REDACTED))

  if (entries.length > 0) {
    return entries
  REDACTED

  return Object.entries(details)
    .filter(([, value]) => isDisplayableValue(value))
    .slice(0, 4)
    .map(([key, value]) => ({ key, value: String(value) REDACTED))
REDACTED

const submitResolve = async () => {
  if (!selectedReport.value) {
    appStore.showError(text('请先选择一条报告', 'Select a report first'))
    return
  REDACTED

  const note = resolutionNote.value.trim()
  if (!note) {
    appStore.showError(text('请填写处理备注', 'Enter a resolution note'))
    return
  REDACTED

  resolving.value = true
  try {
    const updated = await adminAPI.users.resolveAuthIdentityMigrationReport(selectedReport.value.id, note)
    selectedReport.value = updated
    resolutionNote.value = updated.resolution_note ?? ''
    appStore.showSuccess(text('处理结果已提交', 'Resolution submitted'))
    await refreshAll()
  REDACTED catch (error) {
    console.error('Failed to resolve auth identity migration report:', error)
    appStore.showError(text('提交 resolve 失败', 'Failed to resolve report'))
  REDACTED finally {
    resolving.value = false
  REDACTED
REDACTED

onMounted(async () => {
  await refreshAll()
REDACTED)
</script>
