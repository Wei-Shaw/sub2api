<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl pb-8">
      <header class="mb-6">
        <p class="text-xs font-semibold uppercase text-primary-600 dark:text-primary-400">
          {{ t('nav.securityAudit') }}
        </p>
        <h1 class="mt-1 text-2xl font-semibold text-gray-950 dark:text-white">
          {{ t('admin.userIsolationLookup.title') }}
        </h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-300">
          {{ t('admin.userIsolationLookup.description') }}
        </p>
      </header>

      <main class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <form class="grid gap-5 p-5 sm:p-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.25fr)_auto] lg:items-end" @submit.prevent="lookupUser">
          <div class="min-w-0">
            <label for="user-isolation-account" class="input-label">
              {{ t('admin.userIsolationLookup.account') }}
            </label>
            <Select
              id="user-isolation-account"
              v-model="selectedAccountID"
              :options="accountOptions"
              :placeholder="t('admin.userIsolationLookup.selectAccount')"
              :search-placeholder="t('admin.userIsolationLookup.searchAccount')"
              :empty-text="t('admin.userIsolationLookup.noEligibleAccounts')"
              :loading="accountsLoading"
              searchable
              remote
              clearable
              data-test="account-select"
              @search="searchAccounts"
            />
          </div>

          <div class="min-w-0">
            <label for="user-isolation-id" class="input-label">
              {{ t('admin.userIsolationLookup.isolationID') }}
            </label>
            <input
              id="user-isolation-id"
              v-model="isolationID"
              type="text"
              maxlength="46"
              autocomplete="off"
              spellcheck="false"
              class="input font-mono"
              :placeholder="t('admin.userIsolationLookup.isolationIDPlaceholder')"
              data-test="isolation-id"
            />
          </div>

          <button type="submit" class="btn btn-primary inline-flex h-10 items-center justify-center gap-2 whitespace-nowrap" :disabled="!canLookup || locating" data-test="lookup">
            <Icon name="search" size="sm" :class="locating ? 'animate-pulse' : ''" />
            {{ locating ? t('admin.userIsolationLookup.locating') : t('admin.userIsolationLookup.lookup') }}
          </button>
        </form>

        <div v-if="errorMessage" role="alert" class="border-t border-red-200 bg-red-50 px-5 py-4 text-sm text-red-700 dark:border-red-900/70 dark:bg-red-950/30 dark:text-red-300" data-test="error">
          {{ errorMessage }}
        </div>

        <section v-if="result" class="border-t border-gray-200 dark:border-dark-700" data-test="result">
          <div class="flex flex-wrap items-center justify-between gap-3 px-5 py-4 sm:px-6">
            <div class="flex min-w-0 items-center gap-3">
              <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
                <Icon name="user" size="sm" />
              </div>
              <div class="min-w-0">
                <h2 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.userIsolationLookup.result') }}</h2>
                <p class="truncate text-sm text-gray-500 dark:text-dark-300">{{ result.user.email }}</p>
                <p class="truncate text-xs text-gray-400 dark:text-dark-400">{{ result.account.name }} · {{ result.account.platform }} · #{{ result.account.id }}</p>
              </div>
            </div>
            <span class="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
              <Icon name="checkCircle" size="xs" />
              {{ t('admin.userIsolationLookup.exactMatch') }}
            </span>
          </div>

          <dl class="grid border-y border-gray-100 sm:grid-cols-2 lg:grid-cols-3 dark:border-dark-700">
            <div v-for="item in resultDetails" :key="item.label" class="min-w-0 border-b border-gray-100 px-5 py-4 last:border-b-0 sm:px-6 sm:[&:nth-last-child(-n+2)]:border-b-0 lg:[&:nth-last-child(-n+3)]:border-b-0 dark:border-dark-700">
              <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ item.label }}</dt>
              <dd class="mt-1 truncate text-sm font-medium text-gray-900 dark:text-white" :class="item.mono ? 'font-mono' : ''">{{ item.value }}</dd>
            </div>
          </dl>

          <div class="flex flex-wrap justify-end gap-2 px-5 py-4 sm:px-6">
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" data-test="view-user" @click="openUserManagement">
              <Icon name="users" size="sm" />
              {{ t('admin.userIsolationLookup.viewUser') }}
            </button>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" data-test="view-usage" @click="openUsageRecords">
              <Icon name="chart" size="sm" />
              {{ t('admin.userIsolationLookup.viewUsage') }}
            </button>
          </div>
        </section>
      </main>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI, type UserIsolationLookupResult } from '@/api/admin'
import type { Account } from '@/types'
import { extractApiErrorCode, extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

interface AccountSelectOption extends SelectOption {
  account: Account
}

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const selectedAccountID = ref<number | null>(null)
const isolationID = ref('')
const accountOptions = ref<AccountSelectOption[]>([])
const accountsLoading = ref(false)
const locating = ref(false)
const result = ref<UserIsolationLookupResult | null>(null)
const errorMessage = ref('')

const canLookup = computed(() => selectedAccountID.value !== null && isolationID.value.trim().length > 0)
const statusLabel = computed(() => {
  if (!result.value) return '-'
  return result.value.user.status === 'active' ? t('common.active') : t('admin.users.disabled')
})
const resultDetails = computed(() => {
  if (!result.value) return []
  return [
    { label: t('admin.userIsolationLookup.userID'), value: `#${result.value.user.id}`, mono: true },
    { label: t('admin.userIsolationLookup.email'), value: result.value.user.email || t('admin.userIsolationLookup.unknown') },
    { label: t('admin.userIsolationLookup.username'), value: result.value.user.username || t('admin.userIsolationLookup.unknown') },
    { label: t('admin.userIsolationLookup.status'), value: statusLabel.value },
    { label: t('admin.userIsolationLookup.lastActiveAt'), value: displayDate(result.value.user.last_active_at) },
    { label: t('admin.userIsolationLookup.lastUsedAt'), value: displayDate(result.value.user.last_used_at) }
  ]
})

function accountOption(account: Account): AccountSelectOption {
  return {
    value: account.id,
    label: `${account.name} · ${account.platform} · #${account.id}`,
    account
  }
}

function isUserIsolationEnabled(account: Account): boolean {
  return account.extra?.user_isolation_enabled === true
}

async function searchAccounts(query: string): Promise<void> {
  accountsLoading.value = true
  try {
    const response = await adminAPI.accounts.list(1, 20, {
      search: query || undefined,
      lite: 'true'
    })
    const selected = accountOptions.value.find(option => option.value === selectedAccountID.value)
    const options = response.items.filter(isUserIsolationEnabled).map(accountOption)
    if (selected && !options.some(option => option.value === selected.value)) options.unshift(selected)
    accountOptions.value = options
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.userIsolationLookup.errors.default'))
  } finally {
    accountsLoading.value = false
  }
}

async function loadRouteAccount(): Promise<void> {
  const raw = Array.isArray(route.query.account_id) ? route.query.account_id[0] : route.query.account_id
  const accountID = typeof raw === 'string' ? Number(raw) : Number.NaN
  if (!Number.isSafeInteger(accountID) || accountID <= 0) {
    await searchAccounts('')
    return
  }
  try {
    const account = await adminAPI.accounts.getById(accountID)
    accountOptions.value = [accountOption(account)]
    selectedAccountID.value = account.id
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.userIsolationLookup.errors.default'))
  }
}

async function lookupUser(): Promise<void> {
  if (!canLookup.value || selectedAccountID.value === null) return
  locating.value = true
  result.value = null
  errorMessage.value = ''
  try {
    result.value = await adminAPI.userIsolation.lookup({
      account_id: selectedAccountID.value,
      isolation_id: isolationID.value.trim()
    })
  } catch (error) {
    const code = extractApiErrorCode(error)
    const key = code ? `admin.userIsolationLookup.errors.${code}` : ''
    const translated = key ? t(key) : ''
    errorMessage.value = translated && translated !== key
      ? translated
      : extractApiErrorMessage(error, t('admin.userIsolationLookup.errors.default'))
  } finally {
    locating.value = false
  }
}

function displayDate(value?: string | null): string {
  return value ? formatDateTime(value) || t('admin.userIsolationLookup.unknown') : t('admin.userIsolationLookup.unknown')
}

function openUserManagement(): void {
  if (!result.value) return
  void router.push({ path: '/admin/users', query: { search: result.value.user.email } })
}

function openUsageRecords(): void {
  if (!result.value) return
  void router.push({ path: '/admin/usage', query: { user_id: String(result.value.user.id) } })
}

watch([selectedAccountID, isolationID], () => {
  result.value = null
  errorMessage.value = ''
})

onMounted(loadRouteAccount)
</script>
