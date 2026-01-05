<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: fuzzy search + filters (can wrap to multiple lines) -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.groups.searchGroups')"
                class="input pl-10"
              />
            </div>
          <Select
            v-model="filters.platform"
            :options="platformFilterOptions"
            :placeholder="t('admin.groups.allPlatforms')"
            class="w-44"
            @change="loadGroups"
          />
          <Select
            v-model="filters.status"
            :options="statusOptions"
            :placeholder="t('admin.groups.allStatus')"
            class="w-40"
            @change="loadGroups"
          />
          <Select
            v-model="filters.is_exclusive"
            :options="exclusiveOptions"
            :placeholder="t('admin.groups.allGroups')"
            class="w-44"
            @change="loadGroups"
          />
          </div>

          <!-- Right: actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadGroups"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button
              @click="showCreateModal = true"
              class="btn btn-primary"
              data-tour="groups-create-btn"
            >
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.groups.createGroup') REDACTEDREDACTED
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="displayedGroups" :loading="loading">
          <template #cell-name="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-platform="{ value REDACTED">
            <span
              :class="[
                'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
                value === 'anthropic'
                  ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                  : value === 'openai'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : value === 'antigravity'
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                      : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
              ]"
            >
              <PlatformIcon :platform="value" size="xs" />
              {{ t('admin.groups.platforms.' + value) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-billing_type="{ row REDACTED">
            <div class="space-y-1">
              <!-- Type Badge -->
              <span
                :class="[
                  'inline-block rounded-full px-2 py-0.5 text-xs font-medium',
                  row.subscription_type === 'subscription'
                    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
                ]"
              >
                {{
                  row.subscription_type === 'subscription'
                    ? t('admin.groups.subscription.subscription')
                    : t('admin.groups.subscription.standard')
                REDACTEDREDACTED
              </span>
              <!-- Subscription Limits - compact single line -->
              <div
                v-if="row.subscription_type === 'subscription'"
                class="text-xs text-gray-500 dark:text-gray-400"
              >
                <template
                  v-if="row.daily_limit_usd || row.weekly_limit_usd || row.monthly_limit_usd"
                >
                  <span v-if="row.daily_limit_usd"
                    >${{ row.daily_limit_usd REDACTEDREDACTED/{{ t('admin.groups.limitDay') REDACTEDREDACTED</span
                  >
                  <span
                    v-if="row.daily_limit_usd && (row.weekly_limit_usd || row.monthly_limit_usd)"
                    class="mx-1 text-gray-300 dark:text-gray-600"
                    >·</span
                  >
                  <span v-if="row.weekly_limit_usd"
                    >${{ row.weekly_limit_usd REDACTEDREDACTED/{{ t('admin.groups.limitWeek') REDACTEDREDACTED</span
                  >
                  <span
                    v-if="row.weekly_limit_usd && row.monthly_limit_usd"
                    class="mx-1 text-gray-300 dark:text-gray-600"
                    >·</span
                  >
                  <span v-if="row.monthly_limit_usd"
                    >${{ row.monthly_limit_usd REDACTEDREDACTED/{{ t('admin.groups.limitMonth') REDACTEDREDACTED</span
                  >
                </template>
                <span v-else class="text-gray-400 dark:text-gray-500">{{
                  t('admin.groups.subscription.noLimit')
                REDACTEDREDACTED</span>
              </div>
            </div>
          </template>

          <template #cell-rate_multiplier="{ value REDACTED">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value REDACTEDREDACTEDx</span>
          </template>

          <template #cell-is_exclusive="{ value REDACTED">
            <span :class="['badge', value ? 'badge-primary' : 'badge-gray']">
              {{ value ? t('admin.groups.exclusive') : t('admin.groups.public') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-account_count="{ value REDACTED">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ t('admin.groups.accountsCount', { count: value || 0 REDACTED) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-status="{ value REDACTED">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-danger']">
              {{ t('admin.accounts.status.' + value) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
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
              :title="t('admin.groups.noGroupsYet')"
              :description="t('admin.groups.createFirstGroup')"
              :action-text="t('admin.groups.createGroup')"
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

    <!-- Create Group Modal -->
    <BaseDialog
      :show="showCreateModal"
      :title="t('admin.groups.createGroup')"
      width="normal"
      @close="closeCreateModal"
    >
      <form id="create-group-form" @submit.prevent="handleCreateGroup" class="space-y-5">
        <div>
          <label class="input-label">{{ t('admin.groups.form.name') REDACTEDREDACTED</label>
          <input
            v-model="createForm.name"
            type="text"
            required
            class="input"
            :placeholder="t('admin.groups.enterGroupName')"
            data-tour="group-form-name"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.description') REDACTEDREDACTED</label>
          <textarea
            v-model="createForm.description"
            rows="3"
            class="input"
            :placeholder="t('admin.groups.optionalDescription')"
          ></textarea>
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.platform') REDACTEDREDACTED</label>
          <Select
            v-model="createForm.platform"
            :options="platformOptions"
            data-tour="group-form-platform"
          />
          <p class="input-hint">{{ t('admin.groups.platformHint') REDACTEDREDACTED</p>
        </div>
        <div v-if="createForm.subscription_type !== 'subscription'">
          <label class="input-label">{{ t('admin.groups.form.rateMultiplier') REDACTEDREDACTED</label>
          <input
            v-model.number="createForm.rate_multiplier"
            type="number"
            step="0.001"
            min="0.001"
            required
            class="input"
            data-tour="group-form-multiplier"
          />
          <p class="input-hint">{{ t('admin.groups.rateMultiplierHint') REDACTEDREDACTED</p>
        </div>
        <div v-if="createForm.subscription_type !== 'subscription'" data-tour="group-form-exclusive">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.form.exclusive') REDACTEDREDACTED
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <!-- Tooltip Popover -->
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="mb-2 text-xs font-medium">{{ t('admin.groups.exclusiveTooltip.title') REDACTEDREDACTED</p>
                  <p class="mb-2 text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.exclusiveTooltip.description') REDACTEDREDACTED
                  </p>
                  <div class="rounded bg-gray-800 p-2 dark:bg-gray-700">
                    <p class="text-xs leading-relaxed text-gray-300">
                      <span class="inline-flex items-center gap-1 text-primary-400"><Icon name="lightbulb" size="xs" /> {{ t('admin.groups.exclusiveTooltip.example') REDACTEDREDACTED</span>
                      {{ t('admin.groups.exclusiveTooltip.exampleContent') REDACTEDREDACTED
                    </p>
                  </div>
                  <!-- Arrow -->
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="createForm.is_exclusive = !createForm.is_exclusive"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                createForm.is_exclusive ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  createForm.is_exclusive ? 'translate-x-6' : 'translate-x-1'
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ createForm.is_exclusive ? t('admin.groups.exclusive') : t('admin.groups.public') REDACTEDREDACTED
            </span>
          </div>
        </div>

        <!-- Subscription Configuration -->
        <div class="mt-4 border-t pt-4">
          <div>
            <label class="input-label">{{ t('admin.groups.subscription.type') REDACTEDREDACTED</label>
            <Select v-model="createForm.subscription_type" :options="subscriptionTypeOptions" />
            <p class="input-hint">{{ t('admin.groups.subscription.typeHint') REDACTEDREDACTED</p>
          </div>

          <!-- Subscription limits (only show when subscription type is selected) -->
          <div
            v-if="createForm.subscription_type === 'subscription'"
            class="space-y-4 border-l-2 border-primary-200 pl-4 dark:border-primary-800"
          >
            <div>
              <label class="input-label">{{ t('admin.groups.subscription.dailyLimit') REDACTEDREDACTED</label>
              <input
                v-model.number="createForm.daily_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.groups.subscription.weeklyLimit') REDACTEDREDACTED</label>
              <input
                v-model.number="createForm.weekly_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.groups.subscription.monthlyLimit') REDACTEDREDACTED</label>
              <input
                v-model.number="createForm.monthly_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
          </div>
        </div>

      </form>

      <template #footer>
        <div class="flex justify-end gap-3 pt-4">
          <button @click="closeCreateModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            form="create-group-form"
            :disabled="submitting"
            class="btn btn-primary"
            data-tour="group-form-submit"
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
            {{ submitting ? t('admin.groups.creating') : t('common.create') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Edit Group Modal -->
    <BaseDialog
      :show="showEditModal"
      :title="t('admin.groups.editGroup')"
      width="normal"
      @close="closeEditModal"
    >
      <form
        v-if="editingGroup"
        id="edit-group-form"
        @submit.prevent="handleUpdateGroup"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.groups.form.name') REDACTEDREDACTED</label>
          <input
            v-model="editForm.name"
            type="text"
            required
            class="input"
            data-tour="edit-group-form-name"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.description') REDACTEDREDACTED</label>
          <textarea v-model="editForm.description" rows="3" class="input"></textarea>
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.platform') REDACTEDREDACTED</label>
          <Select
            v-model="editForm.platform"
            :options="platformOptions"
            :disabled="true"
            data-tour="group-form-platform"
          />
          <p class="input-hint">{{ t('admin.groups.platformNotEditable') REDACTEDREDACTED</p>
        </div>
        <div v-if="editForm.subscription_type !== 'subscription'">
          <label class="input-label">{{ t('admin.groups.form.rateMultiplier') REDACTEDREDACTED</label>
          <input
            v-model.number="editForm.rate_multiplier"
            type="number"
            step="0.001"
            min="0.001"
            required
            class="input"
            data-tour="group-form-multiplier"
          />
        </div>
        <div v-if="editForm.subscription_type !== 'subscription'">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.form.exclusive') REDACTEDREDACTED
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <!-- Tooltip Popover -->
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="mb-2 text-xs font-medium">{{ t('admin.groups.exclusiveTooltip.title') REDACTEDREDACTED</p>
                  <p class="mb-2 text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.exclusiveTooltip.description') REDACTEDREDACTED
                  </p>
                  <div class="rounded bg-gray-800 p-2 dark:bg-gray-700">
                    <p class="text-xs leading-relaxed text-gray-300">
                      <span class="inline-flex items-center gap-1 text-primary-400"><Icon name="lightbulb" size="xs" /> {{ t('admin.groups.exclusiveTooltip.example') REDACTEDREDACTED</span>
                      {{ t('admin.groups.exclusiveTooltip.exampleContent') REDACTEDREDACTED
                    </p>
                  </div>
                  <!-- Arrow -->
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="editForm.is_exclusive = !editForm.is_exclusive"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                editForm.is_exclusive ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  editForm.is_exclusive ? 'translate-x-6' : 'translate-x-1'
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ editForm.is_exclusive ? t('admin.groups.exclusive') : t('admin.groups.public') REDACTEDREDACTED
            </span>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.status') REDACTEDREDACTED</label>
          <Select v-model="editForm.status" :options="editStatusOptions" />
        </div>

        <!-- Subscription Configuration -->
        <div class="mt-4 border-t pt-4">
          <div>
            <label class="input-label">{{ t('admin.groups.subscription.type') REDACTEDREDACTED</label>
            <Select
              v-model="editForm.subscription_type"
              :options="subscriptionTypeOptions"
              :disabled="true"
            />
            <p class="input-hint">{{ t('admin.groups.subscription.typeNotEditable') REDACTEDREDACTED</p>
          </div>

          <!-- Subscription limits (only show when subscription type is selected) -->
          <div
            v-if="editForm.subscription_type === 'subscription'"
            class="space-y-4 border-l-2 border-primary-200 pl-4 dark:border-primary-800"
          >
            <div>
              <label class="input-label">{{ t('admin.groups.subscription.dailyLimit') REDACTEDREDACTED</label>
              <input
                v-model.number="editForm.daily_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.groups.subscription.weeklyLimit') REDACTEDREDACTED</label>
              <input
                v-model.number="editForm.weekly_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.groups.subscription.monthlyLimit') REDACTEDREDACTED</label>
              <input
                v-model.number="editForm.monthly_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
          </div>
        </div>

      </form>

      <template #footer>
        <div class="flex justify-end gap-3 pt-4">
          <button @click="closeEditModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            form="edit-group-form"
            :disabled="submitting"
            class="btn btn-primary"
            data-tour="group-form-submit"
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
            {{ submitting ? t('admin.groups.updating') : t('common.update') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.groups.deleteGroup')"
      :message="deleteConfirmMessage"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { useOnboardingStore REDACTED from '@/stores/onboarding'
import { adminAPI REDACTED from '@/api/admin'
import type { Group, GroupPlatform, SubscriptionType REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const onboardingStore = useOnboardingStore()

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.groups.columns.name'), sortable: true REDACTED,
  { key: 'platform', label: t('admin.groups.columns.platform'), sortable: true REDACTED,
  { key: 'billing_type', label: t('admin.groups.columns.billingType'), sortable: true REDACTED,
  { key: 'rate_multiplier', label: t('admin.groups.columns.rateMultiplier'), sortable: true REDACTED,
  { key: 'is_exclusive', label: t('admin.groups.columns.type'), sortable: true REDACTED,
  { key: 'account_count', label: t('admin.groups.columns.accounts'), sortable: true REDACTED,
  { key: 'status', label: t('admin.groups.columns.status'), sortable: true REDACTED,
  { key: 'actions', label: t('admin.groups.columns.actions'), sortable: false REDACTED
])

// Filter options
const statusOptions = computed(() => [
  { value: '', label: t('admin.groups.allStatus') REDACTED,
  { value: 'active', label: t('admin.accounts.status.active') REDACTED,
  { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED
])

const exclusiveOptions = computed(() => [
  { value: '', label: t('admin.groups.allGroups') REDACTED,
  { value: 'true', label: t('admin.groups.exclusive') REDACTED,
  { value: 'false', label: t('admin.groups.nonExclusive') REDACTED
])

const platformOptions = computed(() => [
  { value: 'anthropic', label: 'Anthropic' REDACTED,
  { value: 'openai', label: 'OpenAI' REDACTED,
  { value: 'gemini', label: 'Gemini' REDACTED,
  { value: 'antigravity', label: 'Antigravity' REDACTED
])

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.groups.allPlatforms') REDACTED,
  { value: 'anthropic', label: 'Anthropic' REDACTED,
  { value: 'openai', label: 'OpenAI' REDACTED,
  { value: 'gemini', label: 'Gemini' REDACTED,
  { value: 'antigravity', label: 'Antigravity' REDACTED
])

const editStatusOptions = computed(() => [
  { value: 'active', label: t('admin.accounts.status.active') REDACTED,
  { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED
])

const subscriptionTypeOptions = computed(() => [
  { value: 'standard', label: t('admin.groups.subscription.standard') REDACTED,
  { value: 'subscription', label: t('admin.groups.subscription.subscription') REDACTED
])

const groups = ref<Group[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({
  platform: '',
  status: '',
  is_exclusive: ''
REDACTED)
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
REDACTED)

let abortController: AbortController | null = null

const displayedGroups = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return groups.value
  return groups.value.filter((group) => {
    const name = group.name?.toLowerCase?.() ?? ''
    const description = group.description?.toLowerCase?.() ?? ''
    return name.includes(q) || description.includes(q)
  REDACTED)
REDACTED)

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const submitting = ref(false)
const editingGroup = ref<Group | null>(null)
const deletingGroup = ref<Group | null>(null)

const createForm = reactive({
  name: '',
  description: '',
  platform: 'anthropic' as GroupPlatform,
  rate_multiplier: 1.0,
  is_exclusive: false,
  subscription_type: 'standard' as SubscriptionType,
  daily_limit_usd: null as number | null,
  weekly_limit_usd: null as number | null,
  monthly_limit_usd: null as number | null
REDACTED)

const editForm = reactive({
  name: '',
  description: '',
  platform: 'anthropic' as GroupPlatform,
  rate_multiplier: 1.0,
  is_exclusive: false,
  status: 'active' as 'active' | 'inactive',
  subscription_type: 'standard' as SubscriptionType,
  daily_limit_usd: null as number | null,
  weekly_limit_usd: null as number | null,
  monthly_limit_usd: null as number | null
REDACTED)

// 根据分组类型返回不同的删除确认消息
const deleteConfirmMessage = computed(() => {
  if (!deletingGroup.value) {
    return ''
  REDACTED
  if (deletingGroup.value.subscription_type === 'subscription') {
    return t('admin.groups.deleteConfirmSubscription', { name: deletingGroup.value.name REDACTED)
  REDACTED
  return t('admin.groups.deleteConfirm', { name: deletingGroup.value.name REDACTED)
REDACTED)

const loadGroups = async () => {
  if (abortController) {
    abortController.abort()
  REDACTED
  const currentController = new AbortController()
  abortController = currentController
  const { signal REDACTED = currentController
  loading.value = true
  try {
    const response = await adminAPI.groups.list(pagination.page, pagination.page_size, {
      platform: (filters.platform as GroupPlatform) || undefined,
      status: filters.status as any,
      is_exclusive: filters.is_exclusive ? filters.is_exclusive === 'true' : undefined
    REDACTED, { signal REDACTED)
    if (signal.aborted) return
    groups.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  REDACTED catch (error: any) {
    if (signal.aborted || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') {
      return
    REDACTED
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error('Error loading groups:', error)
  REDACTED finally {
    if (abortController === currentController && !signal.aborted) {
      loading.value = false
    REDACTED
  REDACTED
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadGroups()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadGroups()
REDACTED

const closeCreateModal = () => {
  showCreateModal.value = false
  createForm.name = ''
  createForm.description = ''
  createForm.platform = 'anthropic'
  createForm.rate_multiplier = 1.0
  createForm.is_exclusive = false
  createForm.subscription_type = 'standard'
  createForm.daily_limit_usd = null
  createForm.weekly_limit_usd = null
  createForm.monthly_limit_usd = null
REDACTED

const handleCreateGroup = async () => {
  if (!createForm.name.trim()) {
    appStore.showError(t('admin.groups.nameRequired'))
    return
  REDACTED
  submitting.value = true
  try {
    await adminAPI.groups.create(createForm)
    appStore.showSuccess(t('admin.groups.groupCreated'))
    closeCreateModal()
    loadGroups()
    // Only advance tour if active, on submit step, and creation succeeded
    if (onboardingStore.isCurrentStep('[data-tour="group-form-submit"]')) {
      onboardingStore.nextStep(500)
    REDACTED
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.failedToCreate'))
    console.error('Error creating group:', error)
    // Don't advance tour on error
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleEdit = (group: Group) => {
  editingGroup.value = group
  editForm.name = group.name
  editForm.description = group.description || ''
  editForm.platform = group.platform
  editForm.rate_multiplier = group.rate_multiplier
  editForm.is_exclusive = group.is_exclusive
  editForm.status = group.status
  editForm.subscription_type = group.subscription_type || 'standard'
  editForm.daily_limit_usd = group.daily_limit_usd
  editForm.weekly_limit_usd = group.weekly_limit_usd
  editForm.monthly_limit_usd = group.monthly_limit_usd
  showEditModal.value = true
REDACTED

const closeEditModal = () => {
  showEditModal.value = false
  editingGroup.value = null
REDACTED

const handleUpdateGroup = async () => {
  if (!editingGroup.value) return
  if (!editForm.name.trim()) {
    appStore.showError(t('admin.groups.nameRequired'))
    return
  REDACTED

  submitting.value = true
  try {
    await adminAPI.groups.update(editingGroup.value.id, editForm)
    appStore.showSuccess(t('admin.groups.groupUpdated'))
    closeEditModal()
    loadGroups()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.failedToUpdate'))
    console.error('Error updating group:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleDelete = (group: Group) => {
  deletingGroup.value = group
  showDeleteDialog.value = true
REDACTED

const confirmDelete = async () => {
  if (!deletingGroup.value) return

  try {
    await adminAPI.groups.delete(deletingGroup.value.id)
    appStore.showSuccess(t('admin.groups.groupDeleted'))
    showDeleteDialog.value = false
    deletingGroup.value = null
    loadGroups()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.failedToDelete'))
    console.error('Error deleting group:', error)
  REDACTED
REDACTED

// 监听 subscription_type 变化，订阅模式时重置 rate_multiplier 为 1，is_exclusive 为 true
watch(
  () => createForm.subscription_type,
  (newVal) => {
    if (newVal === 'subscription') {
      createForm.rate_multiplier = 1.0
      createForm.is_exclusive = true
    REDACTED
  REDACTED
)

onMounted(() => {
  loadGroups()
REDACTED)
</script>
