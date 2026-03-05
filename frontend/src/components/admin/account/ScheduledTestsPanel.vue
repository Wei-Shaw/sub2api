<template>
  <BaseDialog
    :show="show"
    :title="t('admin.scheduledTests.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- Add Plan Button -->
      <div class="flex items-center justify-between">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.scheduledTests.title') REDACTEDREDACTED
        </p>
        <button
          @click="showAddForm = !showAddForm"
          class="btn btn-primary flex items-center gap-1.5 text-sm"
        >
          <Icon name="plus" size="sm" :stroke-width="2" />
          {{ t('admin.scheduledTests.addPlan') REDACTEDREDACTED
        </button>
      </div>

      <!-- Add Plan Form -->
      <div
        v-if="showAddForm"
        class="rounded-xl border border-primary-200 bg-primary-50/50 p-4 dark:border-primary-800 dark:bg-primary-900/20"
      >
        <div class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.scheduledTests.addPlan') REDACTEDREDACTED
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.model') REDACTEDREDACTED
            </label>
            <Select
              v-model="newPlan.model_id"
              :options="modelOptions"
              :placeholder="t('admin.scheduledTests.model')"
              :searchable="modelOptions.length > 5"
            />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.cronExpression') REDACTEDREDACTED
            </label>
            <Input
              v-model="newPlan.cron_expression"
              :placeholder="'*/30 * * * *'"
              :hint="t('admin.scheduledTests.cronHelp')"
            />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.maxResults') REDACTEDREDACTED
            </label>
            <Input
              v-model="newPlan.max_results"
              type="number"
              placeholder="100"
            />
          </div>
          <div class="flex items-end">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <Toggle v-model="newPlan.enabled" />
              {{ t('admin.scheduledTests.enabled') REDACTEDREDACTED
            </label>
          </div>
        </div>
        <div class="mt-3 flex justify-end gap-2">
          <button
            @click="showAddForm = false; resetNewPlan()"
            class="rounded-lg bg-gray-100 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          >
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            @click="handleCreate"
            :disabled="!newPlan.model_id || !newPlan.cron_expression || creating"
            class="flex items-center gap-1.5 rounded-lg bg-primary-500 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Icon v-if="creating" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
            {{ t('common.save') REDACTEDREDACTED
          </button>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-8">
        <Icon name="refresh" size="md" class="animate-spin text-gray-400" :stroke-width="2" />
        <span class="ml-2 text-sm text-gray-500">{{ t('common.loading') REDACTEDREDACTED...</span>
      </div>

      <!-- Empty State -->
      <div
        v-else-if="plans.length === 0"
        class="rounded-xl border border-dashed border-gray-300 py-10 text-center dark:border-dark-600"
      >
        <Icon name="calendar" size="lg" class="mx-auto mb-2 text-gray-400" :stroke-width="1.5" />
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.scheduledTests.noPlans') REDACTEDREDACTED
        </p>
      </div>

      <!-- Plans List -->
      <div v-else class="space-y-3">
        <div
          v-for="plan in plans"
          :key="plan.id"
          class="rounded-xl border border-gray-200 bg-white transition-all dark:border-dark-600 dark:bg-dark-800"
        >
          <!-- Plan Header -->
          <div
            class="flex cursor-pointer items-center justify-between px-4 py-3"
            @click="toggleExpand(plan.id)"
          >
            <div class="flex flex-1 items-center gap-4">
              <!-- Model -->
              <div class="min-w-0">
                <div class="text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ plan.model_id REDACTEDREDACTED
                </div>
                <div class="mt-0.5 font-mono text-xs text-gray-500 dark:text-gray-400">
                  {{ plan.cron_expression REDACTEDREDACTED
                </div>
              </div>

              <!-- Enabled Toggle -->
              <div class="flex items-center gap-1.5" @click.stop>
                <Toggle
                  :model-value="plan.enabled"
                  @update:model-value="(val: boolean) => handleToggleEnabled(plan, val)"
                />
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ plan.enabled ? t('admin.scheduledTests.enabled') : '' REDACTEDREDACTED
                </span>
              </div>
            </div>

            <div class="flex items-center gap-3">
              <!-- Last Run -->
              <div v-if="plan.last_run_at" class="hidden text-right text-xs text-gray-500 dark:text-gray-400 sm:block">
                <div>{{ t('admin.scheduledTests.lastRun') REDACTEDREDACTED</div>
                <div>{{ formatDateTime(plan.last_run_at) REDACTEDREDACTED</div>
              </div>

              <!-- Next Run -->
              <div v-if="plan.next_run_at" class="hidden text-right text-xs text-gray-500 dark:text-gray-400 sm:block">
                <div>{{ t('admin.scheduledTests.nextRun') REDACTEDREDACTED</div>
                <div>{{ formatDateTime(plan.next_run_at) REDACTEDREDACTED</div>
              </div>

              <!-- Actions -->
              <div class="flex items-center gap-1" @click.stop>
                <button
                  @click="startEdit(plan)"
                  class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-500 dark:hover:bg-blue-900/20"
                  :title="t('admin.scheduledTests.editPlan')"
                >
                  <Icon name="edit" size="sm" :stroke-width="2" />
                </button>
                <button
                  @click="confirmDeletePlan(plan)"
                  class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                  :title="t('admin.scheduledTests.deletePlan')"
                >
                  <Icon name="trash" size="sm" :stroke-width="2" />
                </button>
              </div>

              <!-- Expand indicator -->
              <Icon
                name="chevronDown"
                size="sm"
                :class="[
                  'text-gray-400 transition-transform duration-200',
                  expandedPlanId === plan.id ? 'rotate-180' : ''
                ]"
              />
            </div>
          </div>

          <!-- Edit Form -->
          <div
            v-if="editingPlanId === plan.id"
            class="border-t border-blue-100 bg-blue-50/50 px-4 py-3 dark:border-blue-900 dark:bg-blue-900/10"
            @click.stop
          >
            <div class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.editPlan') REDACTEDREDACTED
            </div>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.model') REDACTEDREDACTED
                </label>
                <Select
                  v-model="editForm.model_id"
                  :options="modelOptions"
                  :placeholder="t('admin.scheduledTests.model')"
                  :searchable="modelOptions.length > 5"
                />
              </div>
              <div>
                <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.cronExpression') REDACTEDREDACTED
                </label>
                <Input
                  v-model="editForm.cron_expression"
                  :placeholder="'*/30 * * * *'"
                  :hint="t('admin.scheduledTests.cronHelp')"
                />
              </div>
              <div>
                <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                  {{ t('admin.scheduledTests.maxResults') REDACTEDREDACTED
                </label>
                <Input
                  v-model="editForm.max_results"
                  type="number"
                  placeholder="100"
                />
              </div>
              <div class="flex items-end">
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <Toggle v-model="editForm.enabled" />
                  {{ t('admin.scheduledTests.enabled') REDACTEDREDACTED
                </label>
              </div>
            </div>
            <div class="mt-3 flex justify-end gap-2">
              <button
                @click="cancelEdit"
                class="rounded-lg bg-gray-100 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
              >
                {{ t('common.cancel') REDACTEDREDACTED
              </button>
              <button
                @click="handleEdit"
                :disabled="!editForm.model_id || !editForm.cron_expression || updating"
                class="flex items-center gap-1.5 rounded-lg bg-primary-500 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon v-if="updating" name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
                {{ t('common.save') REDACTEDREDACTED
              </button>
            </div>
          </div>

          <!-- Expanded Results Section -->
          <div
            v-if="expandedPlanId === plan.id"
            class="border-t border-gray-100 px-4 py-3 dark:border-dark-700"
          >
            <div class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ t('admin.scheduledTests.results') REDACTEDREDACTED
            </div>

            <!-- Results Loading -->
            <div v-if="loadingResults" class="flex items-center justify-center py-4">
              <Icon name="refresh" size="sm" class="animate-spin text-gray-400" :stroke-width="2" />
              <span class="ml-2 text-xs text-gray-500">{{ t('common.loading') REDACTEDREDACTED...</span>
            </div>

            <!-- No Results -->
            <div
              v-else-if="results.length === 0"
              class="py-4 text-center text-xs text-gray-500 dark:text-gray-400"
            >
              {{ t('admin.scheduledTests.noResults') REDACTEDREDACTED
            </div>

            <!-- Results List -->
            <div v-else class="max-h-64 space-y-2 overflow-y-auto">
              <div
                v-for="result in results"
                :key="result.id"
                class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900"
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <!-- Status Badge -->
                    <span
                      :class="[
                        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                        result.status === 'success'
                          ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
                          : result.status === 'running'
                            ? 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-400'
                            : 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400'
                      ]"
                    >
                      {{
                        result.status === 'success'
                          ? t('admin.scheduledTests.success')
                          : result.status === 'running'
                            ? t('admin.scheduledTests.running')
                            : t('admin.scheduledTests.failed')
                      REDACTEDREDACTED
                    </span>

                    <!-- Latency -->
                    <span v-if="result.latency_ms > 0" class="text-xs text-gray-500 dark:text-gray-400">
                      {{ result.latency_ms REDACTEDREDACTEDms
                    </span>
                  </div>

                  <!-- Started At -->
                  <span class="text-xs text-gray-400">
                    {{ formatDateTime(result.started_at) REDACTEDREDACTED
                  </span>
                </div>

                <!-- Response / Error (collapsible) -->
                <div v-if="result.error_message" class="mt-2">
                  <div
                    class="cursor-pointer text-xs font-medium text-red-600 dark:text-red-400"
                    @click="toggleResultDetail(result.id)"
                  >
                    {{ t('admin.scheduledTests.errorMessage') REDACTEDREDACTED
                    <Icon
                      name="chevronDown"
                      size="sm"
                      :class="[
                        'inline transition-transform duration-200',
                        expandedResultIds.has(result.id) ? 'rotate-180' : ''
                      ]"
                    />
                  </div>
                  <pre
                    v-if="expandedResultIds.has(result.id)"
                    class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded bg-red-50 p-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300"
                  >{{ result.error_message REDACTEDREDACTED</pre>
                </div>
                <div v-else-if="result.response_text" class="mt-2">
                  <div
                    class="cursor-pointer text-xs font-medium text-gray-600 dark:text-gray-400"
                    @click="toggleResultDetail(result.id)"
                  >
                    {{ t('admin.scheduledTests.responseText') REDACTEDREDACTED
                    <Icon
                      name="chevronDown"
                      size="sm"
                      :class="[
                        'inline transition-transform duration-200',
                        expandedResultIds.has(result.id) ? 'rotate-180' : ''
                      ]"
                    />
                  </div>
                  <pre
                    v-if="expandedResultIds.has(result.id)"
                    class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded bg-gray-100 p-2 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300"
                  >{{ result.response_text REDACTEDREDACTED</pre>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.scheduledTests.deletePlan')"
      :message="t('admin.scheduledTests.confirmDelete')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select, { type SelectOption REDACTED from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'
import { Icon REDACTED from '@/components/icons'
import { adminAPI REDACTED from '@/api/admin'
import { useAppStore REDACTED from '@/stores/app'
import { formatDateTime REDACTED from '@/utils/format'
import type { ScheduledTestPlan, ScheduledTestResult REDACTED from '@/types'

const { t REDACTED = useI18n()
const appStore = useAppStore()

const props = defineProps<{
  show: boolean
  accountId: number | null
  modelOptions: SelectOption[]
REDACTED>()

const emit = defineEmits<{
  (e: 'close'): void
REDACTED>()

// State
const loading = ref(false)
const creating = ref(false)
const loadingResults = ref(false)
const plans = ref<ScheduledTestPlan[]>([])
const results = ref<ScheduledTestResult[]>([])
const expandedPlanId = ref<number | null>(null)
const expandedResultIds = reactive(new Set<number>())
const showAddForm = ref(false)
const showDeleteConfirm = ref(false)
const deletingPlan = ref<ScheduledTestPlan | null>(null)
const editingPlanId = ref<number | null>(null)
const updating = ref(false)
const editForm = reactive({
  model_id: '' as string,
  cron_expression: '' as string,
  max_results: '100' as string,
  enabled: true
REDACTED)

const newPlan = reactive({
  model_id: '' as string,
  cron_expression: '' as string,
  max_results: '100' as string,
  enabled: true
REDACTED)

const resetNewPlan = () => {
  newPlan.model_id = ''
  newPlan.cron_expression = ''
  newPlan.max_results = '100'
  newPlan.enabled = true
REDACTED

// Load plans when dialog opens
watch(
  () => props.show,
  async (visible) => {
    if (visible && props.accountId) {
      await loadPlans()
    REDACTED else {
      plans.value = []
      results.value = []
      expandedPlanId.value = null
      expandedResultIds.clear()
      showAddForm.value = false
      showDeleteConfirm.value = false
    REDACTED
  REDACTED
)

const loadPlans = async () => {
  if (!props.accountId) return
  loading.value = true
  try {
    plans.value = await adminAPI.scheduledTests.listByAccount(props.accountId)
  REDACTED catch (error: any) {
    appStore.showError(error?.message || 'Failed to load plans')
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const handleCreate = async () => {
  if (!props.accountId || !newPlan.model_id || !newPlan.cron_expression) return
  creating.value = true
  try {
    const maxResults = Number(newPlan.max_results) || 100
    await adminAPI.scheduledTests.create({
      account_id: props.accountId,
      model_id: String(newPlan.model_id),
      cron_expression: String(newPlan.cron_expression),
      enabled: newPlan.enabled,
      max_results: maxResults
    REDACTED)
    appStore.showSuccess(t('admin.scheduledTests.createSuccess'))
    showAddForm.value = false
    resetNewPlan()
    await loadPlans()
  REDACTED catch (error: any) {
    appStore.showError(error?.message || 'Failed to create plan')
  REDACTED finally {
    creating.value = false
  REDACTED
REDACTED

const handleToggleEnabled = async (plan: ScheduledTestPlan, enabled: boolean) => {
  try {
    const updated = await adminAPI.scheduledTests.update(plan.id, { enabled REDACTED)
    const index = plans.value.findIndex((p) => p.id === plan.id)
    if (index !== -1) {
      plans.value[index] = updated
    REDACTED
    appStore.showSuccess(t('admin.scheduledTests.updateSuccess'))
  REDACTED catch (error: any) {
    appStore.showError(error?.message || 'Failed to update plan')
  REDACTED
REDACTED

const startEdit = (plan: ScheduledTestPlan) => {
  editingPlanId.value = plan.id
  editForm.model_id = plan.model_id
  editForm.cron_expression = plan.cron_expression
  editForm.max_results = String(plan.max_results)
  editForm.enabled = plan.enabled
REDACTED

const cancelEdit = () => {
  editingPlanId.value = null
REDACTED

const handleEdit = async () => {
  if (!editingPlanId.value || !editForm.model_id || !editForm.cron_expression) return
  updating.value = true
  try {
    const updated = await adminAPI.scheduledTests.update(editingPlanId.value, {
      model_id: String(editForm.model_id),
      cron_expression: String(editForm.cron_expression),
      max_results: Number(editForm.max_results) || 100,
      enabled: editForm.enabled
    REDACTED)
    const index = plans.value.findIndex((p) => p.id === editingPlanId.value)
    if (index !== -1) {
      plans.value[index] = updated
    REDACTED
    appStore.showSuccess(t('admin.scheduledTests.updateSuccess'))
    editingPlanId.value = null
  REDACTED catch (error: any) {
    appStore.showError(error?.message || 'Failed to update plan')
  REDACTED finally {
    updating.value = false
  REDACTED
REDACTED

const confirmDeletePlan = (plan: ScheduledTestPlan) => {
  deletingPlan.value = plan
  showDeleteConfirm.value = true
REDACTED

const handleDelete = async () => {
  if (!deletingPlan.value) return
  try {
    await adminAPI.scheduledTests.delete(deletingPlan.value.id)
    appStore.showSuccess(t('admin.scheduledTests.deleteSuccess'))
    plans.value = plans.value.filter((p) => p.id !== deletingPlan.value!.id)
    if (expandedPlanId.value === deletingPlan.value.id) {
      expandedPlanId.value = null
      results.value = []
    REDACTED
  REDACTED catch (error: any) {
    appStore.showError(error?.message || 'Failed to delete plan')
  REDACTED finally {
    showDeleteConfirm.value = false
    deletingPlan.value = null
  REDACTED
REDACTED

const toggleExpand = async (planId: number) => {
  if (expandedPlanId.value === planId) {
    expandedPlanId.value = null
    results.value = []
    expandedResultIds.clear()
    return
  REDACTED

  expandedPlanId.value = planId
  expandedResultIds.clear()
  loadingResults.value = true
  try {
    results.value = await adminAPI.scheduledTests.listResults(planId, 20)
  REDACTED catch (error: any) {
    appStore.showError(error?.message || 'Failed to load results')
    results.value = []
  REDACTED finally {
    loadingResults.value = false
  REDACTED
REDACTED

const toggleResultDetail = (resultId: number) => {
  if (expandedResultIds.has(resultId)) {
    expandedResultIds.delete(resultId)
  REDACTED else {
    expandedResultIds.add(resultId)
  REDACTED
REDACTED
</script>
