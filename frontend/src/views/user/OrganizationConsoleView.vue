<template>
  <AppLayout>
    <div class="space-y-5">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ organization?.company_name }}</h2>
          <button v-if="isOwner" class="btn btn-ghost btn-sm" @click="showRename = true">
            {{ t('organization.nameChange.action') }}
          </button>
        </div>
        <p class="mt-1 font-mono text-xs text-gray-500">{{ organization?.account_id }}</p>
      </div>
      <div class="flex max-w-full overflow-x-auto rounded-md bg-gray-100 p-1 dark:bg-dark-800">
        <button
          v-for="tab in visibleTabs"
          :key="tab"
          class="whitespace-nowrap rounded px-3 py-2 text-sm"
          :class="activeTab === tab ? 'bg-white font-medium shadow-sm dark:bg-dark-700' : 'text-gray-600 dark:text-dark-300'"
          @click="activeTab = tab"
        >
          {{ t(`organization.tabs.${tab}`) }}
        </button>
      </div>
    </div>

    <p v-if="error" class="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </p>
    <div v-if="loading" class="py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>

    <section v-else-if="activeTab === 'members'" class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span class="text-sm text-gray-500">{{ t('organization.members.slots', { used: usedSlots, limit: memberLimit }) }}</span>
        <button class="btn btn-primary" :disabled="usedSlots >= memberLimit || operationKey !== ''" @click="showCreate = true">
          {{ t('organization.members.create') }}
        </button>
      </div>
      <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
        <table class="w-full min-w-[860px] text-sm">
          <thead class="bg-gray-50 text-left dark:bg-dark-800">
            <tr>
              <th class="p-3">{{ t('organization.login.loginName') }}</th>
              <th class="p-3">{{ t('organization.iamUserId') }}</th>
              <th class="p-3">{{ t('common.status') }}</th>
              <th class="p-3">{{ t('organization.policies') }}</th>
              <th class="p-3 text-right">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="member in members" :key="member.user_id" class="border-t border-gray-100 dark:border-dark-700">
              <td class="p-3">
                <div class="font-medium">{{ member.login_name }}</div>
                <div class="max-w-xs break-all font-mono text-xs text-gray-500">{{ member.principal }}</div>
              </td>
              <td class="p-3 font-mono text-xs">{{ member.external_user_id }}</td>
              <td class="p-3">{{ t(`organization.status.${member.status}`) }}</td>
              <td class="max-w-xs break-words p-3">{{ member.policy_names.join(', ') || '-' }}</td>
              <td class="p-3 text-right">
                <div class="flex flex-wrap justify-end gap-1">
                  <button class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="resetPassword(member)">{{ t('organization.members.resetPassword') }}</button>
                  <button v-if="member.status === 'active'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="setStatus(member, 'disabled')">{{ t('organization.members.disable') }}</button>
                  <button v-else-if="member.status === 'disabled'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="setStatus(member, 'active')">{{ t('organization.members.enable') }}</button>
                  <button v-if="member.status !== 'archived'" class="btn btn-ghost btn-sm text-red-600" :disabled="isBusy(member)" @click="archiveMember(member)">{{ t('organization.members.archive') }}</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section v-else-if="activeTab === 'authorization'" class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
      <table class="w-full min-w-[680px] text-sm">
        <thead class="bg-gray-50 text-left dark:bg-dark-800">
          <tr>
            <th class="p-3">{{ t('organization.login.loginName') }}</th>
            <th v-for="policy in policies" :key="policy.key" class="p-3">
              <div>{{ policy.display_name }}</div>
              <div class="max-w-xs text-xs font-normal text-gray-500">{{ policy.description }}</div>
              <div class="mt-1 text-xs font-normal text-gray-400">{{ policy.type }} v{{ policy.version }}</div>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="member in activeMembers" :key="member.user_id" class="border-t border-gray-100 dark:border-dark-700">
            <td class="p-3 font-medium">{{ member.login_name }}</td>
            <td v-for="policy in policies" :key="policy.key" class="p-3">
              <input
                type="checkbox"
                :aria-label="`${member.login_name}: ${policy.display_name}`"
                :checked="member.policy_names.includes(policy.key)"
                :disabled="isBusy(member)"
                @change="togglePolicy(member, policy.key, ($event.target as HTMLInputElement).checked)"
              >
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-else-if="activeTab === 'allocation'" class="space-y-3">
      <p class="text-sm text-gray-500">
        {{ t('organization.allocation.rootAvailable', { amount: finance?.available || '0' }) }}
      </p>
      <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
        <table class="w-full min-w-[720px] text-sm">
          <thead class="bg-gray-50 text-left dark:bg-dark-800">
            <tr><th class="p-3">{{ t('organization.login.loginName') }}</th><th class="p-3">{{ t('organization.finance.available') }}</th><th class="p-3">{{ t('organization.finance.frozen') }}</th><th class="p-3">{{ t('organization.allocation.amount') }}</th><th class="p-3">{{ t('common.actions') }}</th></tr>
          </thead>
          <tbody>
            <tr v-for="member in activeMembers" :key="member.user_id" class="border-t border-gray-100 dark:border-dark-700">
              <td class="p-3">{{ member.login_name }}</td>
              <td class="p-3 font-mono">{{ member.balance }}</td>
              <td class="p-3 font-mono">{{ member.frozen_balance }}</td>
              <td class="p-3"><input v-model.trim="amounts[member.user_id]" class="input w-36" type="number" min="0.00000001" step="0.00000001"></td>
              <td class="p-3">
                <div class="flex gap-1">
                  <button class="btn btn-secondary btn-sm" :disabled="!canAllocate(member) || isBusy(member)" @click="transfer(member, 'allocate')">{{ t('organization.allocation.allocate') }}</button>
                  <button class="btn btn-ghost btn-sm" :disabled="!canReclaim(member) || isBusy(member)" @click="transfer(member, 'reclaim')">{{ t('organization.allocation.reclaim') }}</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section v-else-if="activeTab === 'finance'" class="grid gap-4 sm:grid-cols-3">
      <div v-for="field in ['available', 'frozen', 'total'] as const" :key="field" class="rounded-md border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <div class="text-xs text-gray-500">{{ t(`organization.finance.${field}`) }}</div>
        <div class="mt-2 break-all font-mono text-xl font-semibold">{{ finance?.[field] ?? '-' }}</div>
      </div>
      <div v-if="!finance?.available" class="text-sm text-gray-500 sm:col-span-3">
        {{ t(`organization.balanceSource.${finance?.balance_source || 'allocated'}`) }}
      </div>
    </section>

    <section v-else class="space-y-4">
      <form class="grid gap-3 md:grid-cols-3 xl:grid-cols-4" @submit.prevent="loadUsage(1)">
        <select v-model="usageFilters.memberId" class="input">
          <option value="">{{ t('organization.usage.allMembers') }}</option>
          <option v-for="member in members" :key="member.user_id" :value="String(member.user_id)">{{ member.login_name }}</option>
        </select>
        <input v-model.trim="usageFilters.apiKeyId" class="input" type="number" min="1" :placeholder="t('organization.usage.apiKeyId')">
        <input v-model.trim="usageFilters.model" class="input" :placeholder="t('organization.usage.model')">
        <input v-model.trim="usageFilters.endpoint" class="input" :placeholder="t('organization.usage.endpoint')">
        <select v-model="usageFilters.status" class="input">
          <option value="">{{ t('common.all') }}</option>
          <option value="charged">{{ t('organization.usage.charged') }}</option>
          <option value="refunded">{{ t('organization.usage.refunded') }}</option>
        </select>
        <input v-model="usageFilters.start" class="input" type="datetime-local" :aria-label="t('organization.usage.start')">
        <input v-model="usageFilters.end" class="input" type="datetime-local" :aria-label="t('organization.usage.end')">
        <button class="btn btn-secondary" type="submit" :disabled="usageLoading">{{ t('common.search') }}</button>
      </form>
      <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
        <table class="w-full min-w-[1050px] text-sm">
          <thead class="bg-gray-50 text-left dark:bg-dark-800">
            <tr><th class="p-3">{{ t('organization.usage.member') }}</th><th class="p-3">{{ t('organization.usage.apiKey') }}</th><th class="p-3">{{ t('organization.usage.model') }}</th><th class="p-3">{{ t('organization.usage.endpoint') }}</th><th class="p-3">{{ t('common.status') }}</th><th class="p-3">{{ t('organization.usage.tokens') }}</th><th class="p-3">{{ t('organization.usage.charge') }}</th><th class="p-3">{{ t('organization.usage.duration') }}</th><th class="p-3">{{ t('organization.usage.time') }}</th></tr>
          </thead>
          <tbody>
            <tr v-for="row in usagePage.items" :key="row.id" class="border-t border-gray-100 dark:border-dark-700">
              <td class="p-3">{{ row.member_login }}</td><td class="p-3">{{ row.api_key_name || '-' }}</td><td class="p-3">{{ row.model }}</td><td class="max-w-xs break-all p-3">{{ row.endpoint || '-' }}</td><td class="p-3">{{ row.status }}</td><td class="p-3">{{ row.input_tokens + row.output_tokens }}</td><td class="p-3 font-mono">{{ row.actual_cost }}</td><td class="p-3">{{ row.duration_ms ?? '-' }}</td><td class="p-3 whitespace-nowrap">{{ new Date(row.created_at).toLocaleString() }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="flex items-center justify-between gap-3 text-sm">
        <span class="text-gray-500">{{ t('organization.usage.total', { total: usagePage.total }) }}</span>
        <div class="flex gap-2">
          <button class="btn btn-secondary btn-sm" :disabled="usageLoading || usagePage.page <= 1" @click="loadUsage(usagePage.page - 1)">{{ t('organization.usage.previous') }}</button>
          <button class="btn btn-secondary btn-sm" :disabled="usageLoading || usagePage.page >= usagePage.pages" @click="loadUsage(usagePage.page + 1)">{{ t('organization.usage.next') }}</button>
        </div>
      </div>
    </section>

    <div v-if="showCreate" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <form class="w-full max-w-lg space-y-4 rounded-md bg-white p-5 shadow-xl dark:bg-dark-800" @submit.prevent="createMember">
        <h3 class="font-semibold">{{ t('organization.members.create') }}</h3>
        <div>
          <label class="input-label" for="iam-member-login-name">{{ t('organization.login.loginName') }}</label>
          <div class="flex min-w-0 flex-col sm:flex-row">
            <input id="iam-member-login-name" v-model.trim="createForm.loginName" class="input min-w-0 flex-1 sm:rounded-r-none" required pattern="[A-Za-z0-9._-]{1,64}" autocomplete="off">
            <span data-testid="iam-principal-suffix" class="flex min-h-10 max-w-full items-center break-all rounded-md border border-gray-300 bg-gray-50 px-3 font-mono text-xs text-gray-600 sm:-ml-px sm:rounded-l-none sm:whitespace-nowrap dark:border-dark-600 dark:bg-dark-900 dark:text-dark-300">
              @{{ organization?.account_id }}.opentk.ai
            </span>
          </div>
        </div>
        <div>
          <label class="input-label" for="iam-member-password">{{ t('organization.members.password') }}</label>
          <div class="flex min-w-0 gap-2">
            <div class="relative min-w-0 flex-1">
              <input
                id="iam-member-password"
                v-model="createForm.password"
                class="input w-full pr-10 font-mono"
                :type="passwordVisible ? 'text' : 'password'"
                required
                minlength="8"
                maxlength="72"
                autocomplete="new-password"
              >
              <button
                type="button"
                class="absolute inset-y-0 right-0 grid w-10 place-items-center text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200"
                :title="t(passwordVisible ? 'organization.members.hidePassword' : 'organization.members.showPassword')"
                :aria-label="t(passwordVisible ? 'organization.members.hidePassword' : 'organization.members.showPassword')"
                @click="passwordVisible = !passwordVisible"
              >
                <Icon :name="passwordVisible ? 'eyeOff' : 'eye'" size="sm" />
              </button>
            </div>
            <button
              type="button"
              class="icon-btn shrink-0"
              data-testid="generate-iam-password"
              :title="t('organization.members.generatePassword')"
              :aria-label="t('organization.members.generatePassword')"
              @click="generatePassword"
            >
              <Icon name="refresh" size="sm" />
            </button>
          </div>
        </div>
        <label class="flex cursor-pointer items-start gap-2 text-sm text-gray-700 dark:text-dark-200">
          <input v-model="createForm.mustChangePassword" data-testid="must-change-password" class="mt-0.5 h-4 w-4" type="checkbox">
          <span>{{ t('organization.members.mustChangePassword') }}</span>
        </label>
        <input v-model.trim="createForm.recoveryEmail" class="input" type="email" :placeholder="t('organization.members.recoveryEmail')">
        <p v-if="modalError" class="text-sm text-red-600">{{ modalError }}</p>
        <div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" :disabled="operationKey !== ''" @click="closeCreate">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="operationKey !== ''">{{ t('common.create') }}</button></div>
      </form>
    </div>

    <div v-if="credential" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <div class="w-full max-w-lg rounded-md bg-white p-5 shadow-xl dark:bg-dark-800">
        <h3 class="font-semibold">{{ t('organization.members.oneTimeCredential') }}</h3>
        <div class="mt-4 flex items-start gap-2 rounded bg-gray-100 p-3 dark:bg-dark-900">
          <pre class="min-w-0 flex-1 whitespace-pre-wrap break-all font-mono text-sm">{{ credential.principal }}
{{ credential.password }}</pre>
          <button class="icon-btn shrink-0" :title="t('keys.copyToClipboard')" :aria-label="t('keys.copyToClipboard')" @click="copyCredential"><Icon name="copy" size="sm" /></button>
        </div>
        <p class="mt-3 text-xs text-amber-600">{{ t('organization.members.oneTimeWarning') }}</p>
        <button class="btn btn-primary mt-4" @click="credential = null">{{ t('common.confirm') }}</button>
      </div>
    </div>

    <div v-if="showRename" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <form class="w-full max-w-md space-y-4 rounded-md bg-white p-5 shadow-xl dark:bg-dark-800" @submit.prevent="requestNameChange">
        <h3 class="font-semibold">{{ t('organization.nameChange.title') }}</h3>
        <input v-model.trim="requestedName" class="input" required maxlength="255" :placeholder="t('organization.companyName')">
        <p v-if="renameMessage" class="text-sm text-green-600">{{ renameMessage }}</p>
        <p v-if="modalError" class="text-sm text-red-600">{{ modalError }}</p>
        <div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" :disabled="operationKey !== ''" @click="showRename = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="operationKey !== ''">{{ t('organization.nameChange.submit') }}</button></div>
      </form>
    </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { organizationAPI } from '@/api'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useClipboard } from '@/composables/useClipboard'
import type { FinanceSummary, IAMMember, ManagedPolicy, OrganizationContext, OrganizationUsageParams, PaginatedOrganizationUsage } from '@/types'
import { useAuthStore } from '@/stores'

const { t } = useI18n()
const auth = useAuthStore()
const { copyToClipboard } = useClipboard()
type Tab = 'members' | 'authorization' | 'allocation' | 'finance' | 'usage'

const activeTab = ref<Tab>('finance')
const organization = ref<OrganizationContext>()
const members = ref<IAMMember[]>([])
const policies = ref<ManagedPolicy[]>([])
const finance = ref<FinanceSummary>()
const usagePage = ref<PaginatedOrganizationUsage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const memberLimit = ref(20)
const usedSlots = ref(0)
const showCreate = ref(false)
const showRename = ref(false)
const requestedName = ref('')
const renameMessage = ref('')
const credential = ref<{ principal: string; password: string } | null>(null)
const createForm = reactive({ loginName: '', password: '', mustChangePassword: true, recoveryEmail: '' })
const passwordVisible = ref(false)
const amounts = reactive<Record<number, string>>({})
const usageFilters = reactive({ memberId: '', apiKeyId: '', model: '', endpoint: '', status: '', start: '', end: '' })
const loading = ref(true)
const usageLoading = ref(false)
const operationKey = ref('')
const error = ref('')
const modalError = ref('')

const isOwner = computed(() => organization.value?.role === 'owner')
const actions = computed(() => organization.value?.actions || [])
const visibleTabs = computed<Tab[]>(() => isOwner.value
  ? ['members', 'authorization', 'allocation', 'finance', 'usage']
  : (actions.value.includes('organization.finance.balance.read') ? ['finance'] : []))
const activeMembers = computed(() => members.value.filter(item => item.status === 'active'))

function errorMessage(cause: unknown): string {
  return (cause as { message?: string })?.message || t('common.error')
}

function isBusy(member: IAMMember): boolean {
  return operationKey.value.startsWith(`${member.user_id}:`)
}

function positiveAmount(member: IAMMember): number {
  const value = Number(amounts[member.user_id])
  return Number.isFinite(value) && value > 0 ? value : 0
}

function canAllocate(member: IAMMember): boolean {
  return positiveAmount(member) > 0 && positiveAmount(member) <= Number(finance.value?.available || 0)
}

function canReclaim(member: IAMMember): boolean {
  return positiveAmount(member) > 0 && positiveAmount(member) <= Number(member.balance)
}

function toISO(value: string): string | undefined {
  if (!value) return undefined
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString()
}

function organizationUsageParams(page: number): OrganizationUsageParams {
  return {
    page,
    page_size: usagePage.value.page_size || 20,
    member_id: usageFilters.memberId ? Number(usageFilters.memberId) : undefined,
    api_key_id: usageFilters.apiKeyId ? Number(usageFilters.apiKeyId) : undefined,
    model: usageFilters.model || undefined,
    endpoint: usageFilters.endpoint || undefined,
    status: usageFilters.status || undefined,
    start: toISO(usageFilters.start),
    end: toISO(usageFilters.end),
  }
}

async function loadUsage(page = 1) {
  if (!isOwner.value) return
  usageLoading.value = true
  error.value = ''
  try {
    usagePage.value = await organizationAPI.getUsage(organizationUsageParams(page))
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    usageLoading.value = false
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const context = await organizationAPI.getContext()
    organization.value = context.organization
    finance.value = context.finance
    if (isOwner.value) {
      const [memberData, policyData] = await Promise.all([organizationAPI.listMembers(), organizationAPI.listPolicies()])
      members.value = memberData.items
      memberLimit.value = memberData.member_limit
      usedSlots.value = memberData.used_slots
      policies.value = policyData
      if (!visibleTabs.value.includes(activeTab.value)) activeTab.value = 'members'
      await loadUsage(usagePage.value.page || 1)
    } else {
      activeTab.value = 'finance'
    }
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    loading.value = false
  }
}

async function createMember() {
  operationKey.value = 'create'
  modalError.value = ''
  try {
    const result = await organizationAPI.createMember(
      createForm.loginName,
      createForm.password,
      createForm.mustChangePassword,
      createForm.recoveryEmail || undefined,
    )
    credential.value = { principal: result.member.principal, password: result.initial_password }
    closeCreate()
    await load()
  } catch (cause) {
    modalError.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

function closeCreate() {
  showCreate.value = false
  createForm.loginName = ''
  createForm.password = ''
  createForm.mustChangePassword = true
  createForm.recoveryEmail = ''
  passwordVisible.value = false
  modalError.value = ''
}

function generatePassword() {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  const random = new Uint8Array(24)
  globalThis.crypto.getRandomValues(random)
  createForm.password = Array.from(random, value => alphabet[value & 63]).join('')
  passwordVisible.value = true
}

async function setStatus(member: IAMMember, status: IAMMember['status']) {
  operationKey.value = `${member.user_id}:status`
  error.value = ''
  try {
    await organizationAPI.setMemberStatus(member.user_id, status)
    await load()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function archiveMember(member: IAMMember) {
  if (!window.confirm(t('organization.members.archiveConfirm', { name: member.login_name }))) return
  await setStatus(member, 'archived')
}

async function resetPassword(member: IAMMember) {
  operationKey.value = `${member.user_id}:reset`
  error.value = ''
  try {
    const result = await organizationAPI.resetMemberPassword(member.user_id)
    credential.value = { principal: member.principal, password: result.initial_password }
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function togglePolicy(member: IAMMember, key: string, attached: boolean) {
  operationKey.value = `${member.user_id}:policy`
  error.value = ''
  try {
    await organizationAPI.setPolicy(member.user_id, key, attached)
    await load()
    await auth.refreshUser()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function transfer(member: IAMMember, operation: 'allocate' | 'reclaim') {
  const amount = amounts[member.user_id]
  if (!amount || (operation === 'allocate' ? !canAllocate(member) : !canReclaim(member))) return
  operationKey.value = `${member.user_id}:balance`
  error.value = ''
  try {
    await organizationAPI.transferBalance(member.user_id, amount, operation)
    amounts[member.user_id] = ''
    await load()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function requestNameChange() {
  if (!requestedName.value) return
  operationKey.value = 'rename'
  modalError.value = ''
  try {
    await organizationAPI.requestNameChange(requestedName.value)
    renameMessage.value = t('organization.nameChange.pending')
    requestedName.value = ''
  } catch (cause) {
    modalError.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function copyCredential() {
  if (!credential.value) return
  await copyToClipboard(`${credential.value.principal}\n${credential.value.password}`, t('organization.members.copied'))
}

onMounted(load)
</script>
