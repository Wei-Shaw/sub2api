<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-2xl font-semibold">{{ t('relay.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500">{{ t('relay.description') }}</p>
        </div>
        <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('relay.refresh') }}</button>
      </div>
      <div class="rounded-xl bg-primary-50 p-4 text-sm dark:bg-dark-800">
        <p>{{ t('relay.privateNotice') }}</p>
        <p class="mt-2">{{ t('relay.publicNotice') }}</p>
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <div class="card p-5"><p class="text-sm text-gray-500">{{ t('relay.totalEarned') }}</p><p class="mt-2 text-2xl">{{ money(earnings.total_earned) }}</p></div>
        <div class="card p-5"><p class="text-sm text-gray-500">{{ t('relay.todayEarned') }}</p><p class="mt-2 text-2xl">{{ money(earnings.today_earned) }}</p></div>
      </div>
      <p class="text-sm text-gray-500">{{ t('relay.balanceNotice') }}</p>
      <p v-if="earnings.pending_earned > 0" class="text-sm text-amber-600">{{ t('relay.pendingNotice', { amount: money(earnings.pending_earned) }) }}</p>

      <section class="card p-5">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-lg font-semibold">{{ t('relay.privateGroups') }}</h2>
          <div class="flex gap-2">
            <RouterLink to="/keys" class="btn btn-secondary">{{ t('relay.createKey') }}</RouterLink>
            <button class="btn btn-primary" :disabled="loading" @click="openGroup()">{{ t('relay.addGroup') }}</button>
          </div>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <div v-for="group in privateGroups" :key="group.id" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex items-start justify-between gap-2">
              <div><p class="font-medium">{{ displayName(group) }}</p><p class="text-sm text-gray-500">{{ group.platform }} · #{{ group.id }}</p></div>
              <div class="flex gap-2">
                <button class="btn btn-secondary btn-sm" @click="openGroup(group)">{{ t('common.edit') }}</button>
                <button class="btn btn-secondary btn-sm" @click="pendingDelete = { kind: 'group', id: group.id }">{{ t('common.delete') }}</button>
              </div>
            </div>
            <p v-if="group.description" class="mt-2 text-sm text-gray-500">{{ group.description }}</p>
          </div>
        </div>
        <p class="mt-4 text-sm text-gray-500">{{ t('relay.createKeyHint') }}</p>
      </section>

      <section class="card p-5">
        <div class="mb-4 flex items-center justify-between gap-3">
          <h2 class="text-lg font-semibold">{{ t('relay.accounts') }}</h2>
          <button class="btn btn-primary" :disabled="loading" @click="openAccount()">{{ t('relay.addAccount') }}</button>
        </div>
        <p v-if="!accounts.length && !loading" class="py-8 text-center text-gray-500">{{ t('relay.emptyAccounts') }}</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead><tr class="border-b dark:border-dark-600"><th class="p-3">{{ t('relay.name') }}</th><th class="p-3">{{ t('relay.platform') }}</th><th class="p-3">{{ t('relay.groups') }}</th><th class="p-3">{{ t('relay.status') }}</th><th class="p-3">{{ t('common.actions') }}</th></tr></thead>
            <tbody>
              <tr v-for="account in accounts" :key="account.id" class="border-b dark:border-dark-600">
                <td class="p-3">{{ account.name }}<span class="ml-2 text-xs text-gray-500">#{{ account.id }}</span></td>
                <td class="p-3">{{ account.platform }}<br /><span class="text-xs text-gray-500">{{ account.type }}</span></td>
                <td class="p-3">
                  <p>{{ (account.group_ids || []).map(groupLabel).join(', ') }}</p>
                  <p class="text-xs text-gray-500">{{ isShared(account) ? t('relay.shared') : t('relay.privateOnly') }}</p>
                </td>
                <td class="p-3"><AccountStatusIndicator :account="account" /></td>
                <td class="p-3"><div class="flex flex-wrap gap-2">
                  <button class="btn btn-secondary btn-sm" @click="openAccount(account)">{{ t('common.edit') }}</button>
                  <button class="btn btn-secondary btn-sm" @click="openAction(account, 'test')">{{ t('admin.accounts.testConnection') }}</button>
                  <button class="btn btn-secondary btn-sm" @click="openAction(account, 'stats')">{{ t('admin.accounts.viewStats') }}</button>
                  <button v-if="account.type === 'oauth' || account.type === 'setup-token'" class="btn btn-secondary btn-sm" @click="openAction(account, 'reauth')">{{ t('admin.accounts.reAuthorizeAccount') }}</button>
                  <button v-if="account.type === 'oauth' || account.type === 'setup-token'" class="btn btn-secondary btn-sm" :disabled="refreshingID !== null" @click="refreshAccount(account)">{{ t('admin.accounts.refreshToken') }}</button>
                  <button class="btn btn-secondary btn-sm" @click="pendingDelete = { kind: 'account', id: account.id }">{{ t('common.delete') }}</button>
                </div></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="accountTotal > 20" class="mt-4 flex items-center justify-end gap-3">
          <button class="btn btn-secondary btn-sm" :disabled="accountPage <= 1 || loading" @click="accountPage--; load()">{{ t('relay.previous') }}</button>
          <span>{{ t('relay.page', { page: accountPage }) }}</span>
          <button class="btn btn-secondary btn-sm" :disabled="accountPage * 20 >= accountTotal || loading" @click="accountPage++; load()">{{ t('relay.next') }}</button>
        </div>
      </section>

      <section class="card p-5">
        <h2 class="mb-4 text-lg font-semibold">{{ t('relay.earnings') }}</h2>
        <p v-if="!earnings.items.length" class="py-8 text-center text-gray-500">{{ t('relay.emptyEarnings') }}</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead><tr class="border-b dark:border-dark-600"><th class="p-3">{{ t('relay.time') }}</th><th class="p-3">{{ t('relay.accountId') }}</th><th class="p-3">{{ t('relay.groupId') }}</th><th class="p-3">{{ t('relay.charged') }}</th><th class="p-3">{{ t('relay.earned') }}</th></tr></thead>
            <tbody><tr v-for="row in earnings.items" :key="row.id" class="border-b dark:border-dark-600"><td class="p-3">{{ new Date(row.created_at).toLocaleString() }}</td><td class="p-3">#{{ row.account_id }} <span v-if="row.self_use" class="text-xs text-gray-500">{{ t('relay.selfUse') }}</span></td><td class="p-3">{{ groupLabel(row.group_id) }}</td><td class="p-3">{{ money(row.charged_amount) }}</td><td class="p-3">{{ money(row.amount) }} <span v-if="!row.credited_at" class="text-xs text-amber-600">{{ t('relay.pending') }}</span> <span class="text-xs text-gray-500">({{ Math.round(row.supplier_rate * 100) }}%)</span></td></tr></tbody>
          </table>
        </div>
        <div v-if="earnings.total > 20" class="mt-4 flex items-center justify-end gap-3"><button class="btn btn-secondary btn-sm" :disabled="earningPage <= 1 || loading" @click="earningPage--; load()">{{ t('relay.previous') }}</button><span>{{ t('relay.page', { page: earningPage }) }}</span><button class="btn btn-secondary btn-sm" :disabled="earningPage * 20 >= earnings.total || loading" @click="earningPage++; load()">{{ t('relay.next') }}</button></div>
      </section>
    </div>

    <CreateAccountModal :show="showCreate" :proxies="[]" :groups="accountGroups" :initial-group-ids="privateGroups.slice(0, 1).map(group => group.id)" @close="showCreate = false" @created="load" />
    <EditAccountModal :show="showAccount" :account="editingAccount" :proxies="[]" :groups="accountGroups" @close="showAccount = false" @updated="load" />
    <ReAuthAccountModal :show="showReAuth" :account="actionAccount" @close="showReAuth = false" @reauthorized="load" />
    <AccountTestModal :show="showTest" :account="actionAccount" @close="showTest = false" />
    <AccountStatsModal :show="showStats" :account="actionAccount" @close="showStats = false" />

    <BaseDialog :show="showGroup" :title="editingGroup ? t('relay.editGroup') : t('relay.addGroup')" @close="showGroup = false">
      <form id="relay-group-form" class="space-y-4" @submit.prevent="saveGroup">
        <div><label for="relay-group-name" class="input-label">{{ t('relay.name') }}</label><input id="relay-group-name" v-model="groupForm.name" required maxlength="70" class="input" /></div>
        <div v-if="!editingGroup"><label for="relay-group-platform" class="input-label">{{ t('relay.platform') }}</label><select id="relay-group-platform" v-model="groupForm.platform" class="input"><option value="composite">Composite</option><option v-for="platform in platformOptions" :key="platform.value" :value="platform.value">{{ platform.label }}</option></select></div>
        <div v-if="editingGroup"><label for="relay-group-status" class="input-label">{{ t('relay.status') }}</label><select id="relay-group-status" v-model="groupForm.status" class="input"><option value="active">{{ t('relay.active') }}</option><option value="disabled">{{ t('relay.disabled') }}</option></select></div>
        <div v-if="editingGroup"><label for="relay-group-description" class="input-label">{{ t('relay.descriptionLabel') }}</label><textarea id="relay-group-description" v-model="groupForm.description" class="input" maxlength="1000" /></div>
      </form>
      <template #footer><button class="btn btn-secondary" @click="showGroup = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" type="submit" form="relay-group-form" :disabled="saving">{{ t('common.save') }}</button></template>
    </BaseDialog>
    <ConfirmDialog :show="!!pendingDelete" :title="t('relay.deleteTitle')" :message="t('relay.deleteMessage')" danger @cancel="pendingDelete = null" @confirm="remove" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, provide, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { relayApi, type RelayEarnings } from '@/api/relay'
import { useAppStore } from '@/stores/app'
import type { Account, Group } from '@/types'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import CreateAccountModal from '@/components/account/CreateAccountModal.vue'
import EditAccountModal from '@/components/account/EditAccountModal.vue'
import ReAuthAccountModal from '@/components/admin/account/ReAuthAccountModal.vue'
import AccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import AccountStatsModal from '@/components/admin/account/AccountStatsModal.vue'
import AccountStatusIndicator from '@/components/account/AccountStatusIndicator.vue'
import { accountWorkspaceKey } from '@/composables/useAccountWorkspace'
import { relayAccountWorkspace } from '@/api/relayAccountWorkspace'

provide(accountWorkspaceKey, relayAccountWorkspace)

const { t } = useI18n()
const app = useAppStore()
const loading = ref(false)
const saving = ref(false)
const accounts = ref<Account[]>([])
const privateGroups = ref<Group[]>([])
const publicGroups = ref<Group[]>([])
const accountPage = ref(1)
const accountTotal = ref(0)
const earningPage = ref(1)
const earnings = ref<RelayEarnings>({ items: [], total: 0, total_earned: 0, today_earned: 0, pending_earned: 0 })
const editingAccount = ref<Account | null>(null)
const editingGroup = ref<Group | null>(null)
const showAccount = ref(false)
const showCreate = ref(false)
const showReAuth = ref(false)
const showTest = ref(false)
const showStats = ref(false)
const actionAccount = ref<Account | null>(null)
const refreshingID = ref<number | null>(null)
const accountGroups = computed(() => [...privateGroups.value, ...publicGroups.value])
const showGroup = ref(false)
const pendingDelete = ref<{ kind: 'account' | 'group'; id: number } | null>(null)
const groupForm = reactive({ name: '', platform: 'composite', description: '', status: 'active' })
const platformOptions = CONCRETE_PLATFORM_OPTIONS
const money = (value: number) => `$${value.toFixed(8).replace(/0+$/, '').replace(/\.$/, '.00')}`
const displayName = (group: Group) => {
  const name = group.name.replace(/^u\d+: /, '')
  return name === 'My relay' ? t('relay.defaultGroup') : name
}
const groupLabel = (id: number) => {
  const group = [...privateGroups.value, ...publicGroups.value].find(group => group.id === id)
  return group ? displayName(group) : `#${id}`
}
const isShared = (account: Account) => (account.group_ids || []).some(id => publicGroups.value.some(group => group.id === id))

async function load() {
  loading.value = true
  try {
    const [groups, rows, income] = await Promise.all([relayApi.groups(), relayApi.accounts(accountPage.value), relayApi.earnings(earningPage.value)])
    privateGroups.value = groups.private_groups
    publicGroups.value = groups.public_groups
    accounts.value = rows.items
    accountTotal.value = rows.total
    earnings.value = income
  } catch { app.showError(t('relay.loadFailed')) }
  finally { loading.value = false }
}

function openAccount(account?: Account) {
  if (!account) { showCreate.value = true; return }
  editingAccount.value = account
  showAccount.value = true
}

function openAction(account: Account, action: 'test' | 'stats' | 'reauth') {
  actionAccount.value = account
  if (action === 'test') showTest.value = true
  else if (action === 'stats') showStats.value = true
  else showReAuth.value = true
}

async function refreshAccount(account: Account) {
  if (refreshingID.value !== null) return
  refreshingID.value = account.id
  try {
    await relayAccountWorkspace.api.accounts.refreshCredentials(account.id)
    app.showSuccess(t('relay.saved'))
    await load()
  } catch (error) {
    app.showError((error as { message?: string }).message || t('relay.saveFailed'))
  } finally { refreshingID.value = null }
}

function openGroup(group?: Group) {
  editingGroup.value = group || null
  Object.assign(groupForm, { name: group ? displayName(group) : '', platform: group?.platform || 'composite', description: group?.description || '', status: group?.status === 'active' || !group ? 'active' : 'disabled' })
  showGroup.value = true
}

async function saveGroup() {
  saving.value = true
  try {
    if (editingGroup.value) await relayApi.updateGroup(editingGroup.value, groupForm.name.trim(), groupForm.description, groupForm.status)
    else await relayApi.createGroup(groupForm.name.trim(), groupForm.platform)
    showGroup.value = false; app.showSuccess(t('relay.saved')); await load()
  } catch (error) { app.showError((error as { message?: string }).message || t('relay.saveFailed')) }
  finally { saving.value = false }
}

async function remove() {
  const target = pendingDelete.value
  if (!target || saving.value) return
  saving.value = true
  try {
    if (target.kind === 'account') await relayApi.deleteAccount(target.id)
    else await relayApi.deleteGroup(target.id)
    pendingDelete.value = null; app.showSuccess(t('relay.deleted')); await load()
  } catch (error) { app.showError((error as { message?: string }).message || t('relay.deleteFailed')) }
  finally { saving.value = false }
}

let poll: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void load()
  poll = setInterval(() => {
    if (!document.hidden && !loading.value && !saving.value) {
      void relayApi.earnings(earningPage.value).then(income => { earnings.value = income }).catch(() => {})
    }
  }, 10000)
})
onUnmounted(() => { if (poll) clearInterval(poll) })
</script>
