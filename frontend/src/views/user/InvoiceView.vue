<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="inline-flex w-full rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900 lg:w-auto">
            <button
              v-for="tab in tabs"
              :key="tab.value"
              type="button"
              :class="[
                'inline-flex flex-1 items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors lg:flex-none',
                activeTab === tab.value
                  ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-800 dark:text-primary-400'
                  : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100'
              ]"
              @click="switchTab(tab.value)"
            >
              <Icon :name="tab.icon" size="sm" />
              <span>{{ tab.label }}</span>
            </button>
          </div>

          <div class="flex flex-wrap justify-end gap-2">
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="activeLoading"
              :title="t('common.refresh')"
              @click="refreshActiveTab"
            >
              <Icon name="refresh" size="md" :class="activeLoading ? 'animate-spin' : ''" />
            </button>
            <button
              v-if="activeTab === 'profiles'"
              type="button"
              class="btn btn-primary"
              @click="openCreateProfile"
            >
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('invoice.profiles.create') }}
            </button>
            <button
              v-if="activeTab === 'orders'"
              type="button"
              class="btn btn-primary"
              :disabled="actionLoading || selectedOrderIds.size === 0 || !selectedProfileId"
              @click="submitInvoiceRequest"
            >
              <Icon name="check" size="md" class="mr-2" />
              {{ actionLoading ? t('common.processing') : t('invoice.actions.submitRequest') }}
            </button>
          </div>
        </div>
      </div>

      <section v-if="activeTab === 'requests'" class="space-y-4">
        <div class="card p-4">
          <div class="grid gap-3 md:grid-cols-[160px_1fr_1fr_auto] md:items-end">
            <div>
              <label class="input-label">{{ t('invoice.filters.status') }}</label>
              <Select
                v-model="requestFilters.status"
                :options="statusFilterOptions"
                class="mt-1"
                @change="handleRequestFilterChange"
              />
            </div>
            <div>
              <label class="input-label">{{ t('invoice.filters.startDate') }}</label>
              <input
                v-model="requestFilters.start_time"
                type="date"
                class="input mt-1 w-full"
                @change="handleRequestFilterChange"
              />
            </div>
            <div>
              <label class="input-label">{{ t('invoice.filters.endDate') }}</label>
              <input
                v-model="requestFilters.end_time"
                type="date"
                class="input mt-1 w-full"
                @change="handleRequestFilterChange"
              />
            </div>
            <button type="button" class="btn btn-secondary" @click="resetRequestFilters">
              {{ t('common.reset') }}
            </button>
          </div>
        </div>

        <div class="card overflow-hidden">
          <div v-if="requestsLoading" class="flex items-center justify-center py-16">
            <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
          </div>
          <div v-else-if="requests.length === 0" class="flex flex-col items-center justify-center gap-3 py-16 text-gray-500 dark:text-gray-400">
            <Icon name="document" size="xl" />
            <p class="text-sm">{{ t('invoice.empty.requests') }}</p>
          </div>
          <div v-else class="divide-y divide-gray-200 dark:divide-dark-700">
            <article v-for="request in requests" :key="request.id" class="p-4">
              <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div class="min-w-0 space-y-2">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ request.serial_no }}</span>
                    <span :class="statusBadgeClass(request.status)">
                      {{ t(`invoice.status.${request.status}`) }}
                    </span>
                  </div>
                  <div class="grid gap-x-8 gap-y-1 text-sm text-gray-600 dark:text-gray-300 md:grid-cols-2">
                    <div>
                      <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.title') }}:</span>
                      <span class="ml-1 font-medium text-gray-900 dark:text-white">{{ request.profile_snapshot.title }}</span>
                    </div>
                    <div>
                      <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.taxNumber') }}:</span>
                      <span class="ml-1 break-all font-mono">{{ request.profile_snapshot.tax_number }}</span>
                    </div>
                    <div>
                      <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.email') }}:</span>
                      <span class="ml-1 break-all">{{ request.profile_snapshot.email }}</span>
                    </div>
                    <div>
                      <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.createdAt') }}:</span>
                      <span class="ml-1">{{ formatDateTime(request.created_at) }}</span>
                    </div>
                  </div>
                </div>
                <div class="shrink-0 text-left lg:text-right">
                  <div class="text-lg font-semibold text-gray-900 dark:text-white">{{ formatCurrency(request.total_amount) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.requests.orderCount', { count: request.orders?.length || 0 }) }}</div>
                </div>
              </div>

              <div v-if="request.reject_reason" class="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
                {{ request.reject_reason }}
              </div>

              <div v-if="request.orders?.length" class="mt-3 flex flex-wrap gap-2">
                <span
                  v-for="order in request.orders"
                  :key="order.id"
                  class="rounded-md bg-gray-100 px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-300"
                >
                  {{ order.out_trade_no }}
                </span>
              </div>
            </article>
          </div>

          <Pagination
            v-if="requestPagination.total > 0"
            :page="requestPagination.page"
            :total="requestPagination.total"
            :page-size="requestPagination.page_size"
            @update:page="handleRequestPageChange"
            @update:pageSize="handleRequestPageSizeChange"
          />
        </div>
      </section>

      <section v-else-if="activeTab === 'profiles'" class="space-y-4">
        <div v-if="profilesLoading" class="card flex items-center justify-center py-16">
          <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
        </div>
        <div v-else-if="profiles.length === 0" class="card flex flex-col items-center justify-center gap-3 py-16 text-gray-500 dark:text-gray-400">
          <Icon name="clipboard" size="xl" />
          <p class="text-sm">{{ t('invoice.empty.profiles') }}</p>
          <button type="button" class="btn btn-primary" @click="openCreateProfile">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('invoice.profiles.create') }}
          </button>
        </div>
        <div v-else class="grid gap-4 lg:grid-cols-2">
          <article
            v-for="profile in profiles"
            :key="profile.id"
            class="card p-4"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ profile.title }}</h3>
                  <span v-if="profile.is_default" class="rounded-full bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
                    {{ t('invoice.profiles.default') }}
                  </span>
                </div>
                <p class="mt-1 break-all font-mono text-sm text-gray-600 dark:text-gray-300">{{ profile.tax_number }}</p>
              </div>
              <div class="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  class="rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
                  :title="t('common.edit')"
                  @click="openEditProfile(profile)"
                >
                  <Icon name="edit" size="sm" />
                </button>
                <button
                  type="button"
                  class="rounded-md p-2 text-red-500 hover:bg-red-50 hover:text-red-700 dark:text-red-400 dark:hover:bg-red-900/20"
                  :title="t('common.delete')"
                  @click="deleteTarget = profile"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>

            <dl class="mt-4 grid gap-3 text-sm sm:grid-cols-2">
              <div>
                <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.email') }}</dt>
                <dd class="mt-1 break-all text-gray-900 dark:text-white">{{ profile.email }}</dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.phone') }}</dt>
                <dd class="mt-1 text-gray-900 dark:text-white">{{ profile.phone || '-' }}</dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.bankName') }}</dt>
                <dd class="mt-1 text-gray-900 dark:text-white">{{ profile.bank_name || '-' }}</dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.bankAccount') }}</dt>
                <dd class="mt-1 break-all font-mono text-gray-900 dark:text-white">{{ profile.bank_account || '-' }}</dd>
              </div>
              <div class="sm:col-span-2">
                <dt class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.address') }}</dt>
                <dd class="mt-1 text-gray-900 dark:text-white">{{ profile.address || '-' }}</dd>
              </div>
            </dl>

            <div class="mt-4 flex justify-end">
              <button
                type="button"
                class="btn btn-secondary"
                :disabled="profile.is_default || actionLoading"
                @click="setDefaultProfile(profile.id)"
              >
                <Icon name="checkCircle" size="sm" class="mr-2" />
                {{ t('invoice.actions.setDefault') }}
              </button>
            </div>
          </article>
        </div>
      </section>

      <section v-else class="space-y-4">
        <div class="card p-4">
          <div class="grid gap-4 lg:grid-cols-[minmax(220px,320px)_1fr] lg:items-end">
            <div>
              <label class="input-label">{{ t('invoice.orders.invoiceProfile') }}</label>
              <Select
                :model-value="selectedProfileId"
                :options="profileSelectOptions"
                class="mt-1"
                :disabled="profiles.length === 0"
                @update:model-value="handleSelectedProfileChange"
              />
            </div>
            <div class="flex flex-wrap items-center justify-between gap-3 rounded-md bg-gray-50 px-4 py-3 dark:bg-dark-900">
              <div class="text-sm text-gray-600 dark:text-gray-300">
                {{ t('invoice.orders.selectedSummary', { count: selectedOrderIds.size, amount: formatCurrency(selectedTotal) }) }}
              </div>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="selectedOrderIds.size === 0"
                @click="clearSelection"
              >
                {{ t('invoice.actions.clearSelection') }}
              </button>
            </div>
          </div>
        </div>

        <div v-if="profiles.length === 0" class="card flex flex-col items-center justify-center gap-3 py-16 text-gray-500 dark:text-gray-400">
          <Icon name="clipboard" size="xl" />
          <p class="text-sm">{{ t('invoice.empty.profiles') }}</p>
          <button type="button" class="btn btn-primary" @click="openCreateProfile">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('invoice.profiles.create') }}
          </button>
        </div>
        <div v-else class="card overflow-hidden">
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="w-12 px-4 py-3 text-left">
                    <input
                      type="checkbox"
                      class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      :checked="currentPageAllSelected"
                      :disabled="ordersLoading || invoiceableOrders.length === 0"
                      @change="toggleCurrentPageSelection"
                    />
                  </th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('invoice.orders.orderNo') }}</th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('invoice.orders.amount') }}</th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('invoice.orders.type') }}</th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('invoice.orders.completedAt') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr v-if="ordersLoading">
                  <td colspan="5" class="px-4 py-16 text-center">
                    <Icon name="refresh" size="lg" class="mx-auto animate-spin text-gray-400" />
                  </td>
                </tr>
                <tr v-else-if="invoiceableOrders.length === 0">
                  <td colspan="5" class="px-4 py-16 text-center text-sm text-gray-500 dark:text-gray-400">
                    {{ t('invoice.empty.orders') }}
                  </td>
                </tr>
                <template v-else>
                  <tr v-for="order in invoiceableOrders" :key="order.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                    <td class="px-4 py-3">
                      <input
                        type="checkbox"
                        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                        :checked="selectedOrderIds.has(order.id)"
                        @change="toggleOrderSelection(order, $event)"
                      />
                    </td>
                    <td class="px-4 py-3">
                      <div class="font-mono text-sm text-gray-900 dark:text-white">{{ order.out_trade_no }}</div>
                      <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">#{{ order.id }}</div>
                    </td>
                    <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">{{ formatCurrency(order.pay_amount) }}</td>
                    <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ orderTypeLabel(order.order_type) }}</td>
                    <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ formatDateTime(order.completed_at || order.created_at) }}</td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <Pagination
            v-if="orderPagination.total > 0"
            :page="orderPagination.page"
            :total="orderPagination.total"
            :page-size="orderPagination.page_size"
            @update:page="handleOrderPageChange"
            @update:pageSize="handleOrderPageSizeChange"
          />
        </div>
      </section>
    </div>

    <BaseDialog
      :show="profileDialogOpen"
      :title="editingProfile ? t('invoice.profiles.edit') : t('invoice.profiles.create')"
      width="wide"
      @close="closeProfileDialog"
    >
      <form class="grid gap-4 md:grid-cols-2" @submit.prevent="submitProfile">
        <div>
          <label class="input-label">{{ t('invoice.fields.title') }}</label>
          <input v-model="profileForm.title" type="text" class="input mt-1 w-full" maxlength="255" required />
        </div>
        <div>
          <label class="input-label">{{ t('invoice.fields.taxNumber') }}</label>
          <input v-model="profileForm.tax_number" type="text" class="input mt-1 w-full" maxlength="64" required />
        </div>
        <div>
          <label class="input-label">{{ t('invoice.fields.email') }}</label>
          <input v-model="profileForm.email" type="email" class="input mt-1 w-full" maxlength="255" required />
        </div>
        <div>
          <label class="input-label">{{ t('invoice.fields.phone') }}</label>
          <input v-model="profileForm.phone" type="text" class="input mt-1 w-full" maxlength="64" />
        </div>
        <div>
          <label class="input-label">{{ t('invoice.fields.bankName') }}</label>
          <input v-model="profileForm.bank_name" type="text" class="input mt-1 w-full" maxlength="255" />
        </div>
        <div>
          <label class="input-label">{{ t('invoice.fields.bankAccount') }}</label>
          <input v-model="profileForm.bank_account" type="text" class="input mt-1 w-full" maxlength="128" />
        </div>
        <div class="md:col-span-2">
          <label class="input-label">{{ t('invoice.fields.address') }}</label>
          <input v-model="profileForm.address" type="text" class="input mt-1 w-full" maxlength="500" />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeProfileDialog">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="actionLoading" @click="submitProfile">
            {{ actionLoading ? t('common.processing') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="!!deleteTarget"
      :title="t('invoice.profiles.deleteTitle')"
      :message="t('invoice.profiles.deleteMessage', { name: deleteTarget?.title || '' })"
      :confirm-text="t('common.delete')"
      danger
      @confirm="confirmDeleteProfile"
      @cancel="deleteTarget = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { invoiceAPI } from '@/api/invoice'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatCurrency, formatDateTime } from '@/utils/format'
import type { SelectOption } from '@/types'
import type {
  InvoiceOrder,
  InvoiceProfile,
  InvoiceProfilePayload,
  InvoiceRequest,
  InvoiceRequestListParams,
  InvoiceStatus
} from '@/types/invoice'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

type InvoiceTab = 'requests' | 'profiles' | 'orders'
type TabIcon = 'document' | 'clipboard' | 'creditCard'

interface ProfileForm {
  title: string
  tax_number: string
  email: string
  address: string
  phone: string
  bank_name: string
  bank_account: string
}

const { t } = useI18n()
const appStore = useAppStore()

const activeTab = ref<InvoiceTab>('requests')
const requestsLoading = ref(false)
const profilesLoading = ref(false)
const ordersLoading = ref(false)
const actionLoading = ref(false)

const requests = ref<InvoiceRequest[]>([])
const profiles = ref<InvoiceProfile[]>([])
const invoiceableOrders = ref<InvoiceOrder[]>([])
const selectedOrderIds = ref<Set<number>>(new Set())
const selectedOrderMap = ref<Map<number, InvoiceOrder>>(new Map())
const selectedProfileId = ref<number | null>(null)

const profileDialogOpen = ref(false)
const editingProfile = ref<InvoiceProfile | null>(null)
const deleteTarget = ref<InvoiceProfile | null>(null)

const requestPagination = reactive({ page: 1, page_size: 20, total: 0 })
const orderPagination = reactive({ page: 1, page_size: 20, total: 0 })
const requestFilters = reactive<{
  status: InvoiceStatus | ''
  start_time: string
  end_time: string
}>({
  status: '',
  start_time: '',
  end_time: ''
})

const profileForm = reactive<ProfileForm>({
  title: '',
  tax_number: '',
  email: '',
  address: '',
  phone: '',
  bank_name: '',
  bank_account: ''
})

const tabs = computed<Array<{ value: InvoiceTab; label: string; icon: TabIcon }>>(() => [
  { value: 'requests', label: t('invoice.tabs.requests'), icon: 'document' },
  { value: 'profiles', label: t('invoice.tabs.profiles'), icon: 'clipboard' },
  { value: 'orders', label: t('invoice.tabs.orders'), icon: 'creditCard' }
])

const statusFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('common.all') },
  { value: 'pending', label: t('invoice.status.pending') },
  { value: 'completed', label: t('invoice.status.completed') },
  { value: 'rejected', label: t('invoice.status.rejected') }
])

const profileSelectOptions = computed<SelectOption[]>(() =>
  profiles.value.map((profile) => ({
    value: profile.id,
    label: profile.is_default
      ? `${profile.title} (${t('invoice.profiles.default')})`
      : profile.title
  }))
)

const activeLoading = computed(() => {
  if (activeTab.value === 'requests') return requestsLoading.value
  if (activeTab.value === 'profiles') return profilesLoading.value
  return ordersLoading.value
})

const selectedOrders = computed(() =>
  Array.from(selectedOrderMap.value.values()).filter((order) => selectedOrderIds.value.has(order.id))
)

const selectedTotal = computed(() =>
  selectedOrders.value.reduce((sum, order) => sum + (Number(order.pay_amount) || 0), 0)
)

const currentPageAllSelected = computed(() =>
  invoiceableOrders.value.length > 0 &&
  invoiceableOrders.value.every((order) => selectedOrderIds.value.has(order.id))
)

function switchTab(tab: InvoiceTab) {
  activeTab.value = tab
  if (tab === 'requests' && requests.value.length === 0) fetchRequests()
  if (tab === 'profiles' && profiles.value.length === 0) fetchProfiles()
  if (tab === 'orders') {
    if (profiles.value.length === 0) fetchProfiles()
    if (invoiceableOrders.value.length === 0) fetchInvoiceableOrders()
  }
}

async function refreshActiveTab() {
  if (activeTab.value === 'requests') {
    await fetchRequests()
    return
  }
  if (activeTab.value === 'profiles') {
    await fetchProfiles()
    return
  }
  await Promise.all([fetchProfiles(), fetchInvoiceableOrders()])
}

async function fetchProfiles() {
  profilesLoading.value = true
  try {
    const res = await invoiceAPI.listProfiles()
    profiles.value = res.data || []
    ensureSelectedProfile()
  } catch (err: unknown) {
    showInvoiceError(err)
  } finally {
    profilesLoading.value = false
  }
}

async function fetchRequests() {
  requestsLoading.value = true
  try {
    const params: InvoiceRequestListParams = {
      page: requestPagination.page,
      page_size: requestPagination.page_size
    }
    if (requestFilters.status) params.status = requestFilters.status
    if (requestFilters.start_time) params.start_time = requestFilters.start_time
    if (requestFilters.end_time) params.end_time = requestFilters.end_time

    const res = await invoiceAPI.listRequests(params)
    requests.value = res.data.items || []
    requestPagination.total = res.data.total || 0
  } catch (err: unknown) {
    showInvoiceError(err)
  } finally {
    requestsLoading.value = false
  }
}

async function fetchInvoiceableOrders() {
  ordersLoading.value = true
  try {
    const res = await invoiceAPI.listInvoiceableOrders({
      page: orderPagination.page,
      page_size: orderPagination.page_size
    })
    invoiceableOrders.value = res.data.items || []
    orderPagination.total = res.data.total || 0
  } catch (err: unknown) {
    showInvoiceError(err)
  } finally {
    ordersLoading.value = false
  }
}

function ensureSelectedProfile() {
  if (profiles.value.length === 0) {
    selectedProfileId.value = null
    return
  }
  if (selectedProfileId.value && profiles.value.some((profile) => profile.id === selectedProfileId.value)) {
    return
  }
  const next = profiles.value.find((profile) => profile.is_default) || profiles.value[0]
  selectedProfileId.value = next.id
}

function handleRequestFilterChange() {
  requestPagination.page = 1
  fetchRequests()
}

function resetRequestFilters() {
  requestFilters.status = ''
  requestFilters.start_time = ''
  requestFilters.end_time = ''
  handleRequestFilterChange()
}

function handleRequestPageChange(page: number) {
  requestPagination.page = page
  fetchRequests()
}

function handleRequestPageSizeChange(size: number) {
  requestPagination.page_size = size
  requestPagination.page = 1
  fetchRequests()
}

function handleOrderPageChange(page: number) {
  orderPagination.page = page
  fetchInvoiceableOrders()
}

function handleOrderPageSizeChange(size: number) {
  orderPagination.page_size = size
  orderPagination.page = 1
  fetchInvoiceableOrders()
}

function handleSelectedProfileChange(value: string | number | boolean | null) {
  if (typeof value === 'boolean' || value === null) {
    selectedProfileId.value = null
    return
  }
  const id = typeof value === 'number' ? value : Number(value)
  selectedProfileId.value = Number.isFinite(id) && id > 0 ? id : null
}

function toggleOrderSelection(order: InvoiceOrder, event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const nextIds = new Set(selectedOrderIds.value)
  const nextMap = new Map(selectedOrderMap.value)
  if (checked) {
    nextIds.add(order.id)
    nextMap.set(order.id, order)
  } else {
    nextIds.delete(order.id)
    nextMap.delete(order.id)
  }
  selectedOrderIds.value = nextIds
  selectedOrderMap.value = nextMap
}

function toggleCurrentPageSelection(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const nextIds = new Set(selectedOrderIds.value)
  const nextMap = new Map(selectedOrderMap.value)
  for (const order of invoiceableOrders.value) {
    if (checked) {
      nextIds.add(order.id)
      nextMap.set(order.id, order)
    } else {
      nextIds.delete(order.id)
      nextMap.delete(order.id)
    }
  }
  selectedOrderIds.value = nextIds
  selectedOrderMap.value = nextMap
}

function clearSelection() {
  selectedOrderIds.value = new Set()
  selectedOrderMap.value = new Map()
}

async function submitInvoiceRequest() {
  if (!selectedProfileId.value) {
    appStore.showError(t('invoice.messages.profileRequired'))
    return
  }
  const orderIds = Array.from(selectedOrderIds.value)
  if (orderIds.length === 0) {
    appStore.showError(t('invoice.messages.orderRequired'))
    return
  }

  actionLoading.value = true
  try {
    await invoiceAPI.createRequest({
      profile_id: selectedProfileId.value,
      order_ids: orderIds
    })
    appStore.showSuccess(t('invoice.messages.requestSubmitted'))
    clearSelection()
    activeTab.value = 'requests'
    requestPagination.page = 1
    await Promise.all([fetchRequests(), fetchInvoiceableOrders()])
  } catch (err: unknown) {
    showInvoiceError(err)
  } finally {
    actionLoading.value = false
  }
}

function openCreateProfile() {
  editingProfile.value = null
  resetProfileForm()
  profileDialogOpen.value = true
}

function openEditProfile(profile: InvoiceProfile) {
  editingProfile.value = profile
  profileForm.title = profile.title
  profileForm.tax_number = profile.tax_number
  profileForm.email = profile.email
  profileForm.address = profile.address || ''
  profileForm.phone = profile.phone || ''
  profileForm.bank_name = profile.bank_name || ''
  profileForm.bank_account = profile.bank_account || ''
  profileDialogOpen.value = true
}

function closeProfileDialog() {
  profileDialogOpen.value = false
  editingProfile.value = null
  resetProfileForm()
}

function resetProfileForm() {
  profileForm.title = ''
  profileForm.tax_number = ''
  profileForm.email = ''
  profileForm.address = ''
  profileForm.phone = ''
  profileForm.bank_name = ''
  profileForm.bank_account = ''
}

async function submitProfile() {
  const payload = buildProfilePayload()
  if (!payload) return

  actionLoading.value = true
  try {
    if (editingProfile.value) {
      await invoiceAPI.updateProfile(editingProfile.value.id, payload)
    } else {
      await invoiceAPI.createProfile(payload)
    }
    appStore.showSuccess(t('common.saved'))
    closeProfileDialog()
    await fetchProfiles()
  } catch (err: unknown) {
    showInvoiceError(err)
  } finally {
    actionLoading.value = false
  }
}

async function confirmDeleteProfile() {
  if (!deleteTarget.value) return
  actionLoading.value = true
  try {
    await invoiceAPI.deleteProfile(deleteTarget.value.id)
    appStore.showSuccess(t('common.deleted'))
    deleteTarget.value = null
    await fetchProfiles()
  } catch (err: unknown) {
    showInvoiceError(err)
  } finally {
    actionLoading.value = false
  }
}

async function setDefaultProfile(id: number) {
  actionLoading.value = true
  try {
    await invoiceAPI.setDefaultProfile(id)
    appStore.showSuccess(t('invoice.messages.defaultUpdated'))
    await fetchProfiles()
  } catch (err: unknown) {
    showInvoiceError(err)
  } finally {
    actionLoading.value = false
  }
}

function buildProfilePayload(): InvoiceProfilePayload | null {
  const title = profileForm.title.trim()
  const taxNumber = profileForm.tax_number.trim()
  const email = profileForm.email.trim()
  if (!title || !taxNumber || !email) {
    appStore.showError(t('invoice.messages.requiredFields'))
    return null
  }
  return {
    title,
    tax_number: taxNumber,
    email,
    address: trimOrNull(profileForm.address),
    phone: trimOrNull(profileForm.phone),
    bank_name: trimOrNull(profileForm.bank_name),
    bank_account: trimOrNull(profileForm.bank_account)
  }
}

function trimOrNull(value: string): string | null {
  const trimmed = value.trim()
  return trimmed ? trimmed : null
}

function statusBadgeClass(status: InvoiceStatus): string {
  const base = 'rounded-full px-2 py-0.5 text-xs font-medium'
  switch (status) {
    case 'pending':
      return `${base} bg-yellow-50 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300`
    case 'completed':
      return `${base} bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300`
    case 'rejected':
      return `${base} bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300`
    default:
      return `${base} bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300`
  }
}

function orderTypeLabel(type: string): string {
  const key = `invoice.orderTypes.${type}`
  const label = t(key)
  return label === key ? type : label
}

function showInvoiceError(err: unknown) {
  appStore.showError(extractI18nErrorMessage(err, t, 'invoice.errors', t('common.error')))
}

onMounted(() => {
  fetchProfiles()
  fetchRequests()
})
</script>
