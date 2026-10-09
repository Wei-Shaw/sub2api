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
                <td class="p-3"><p>{{ account.status === 'active' ? t('relay.active') : t('relay.disabled') }}</p><p v-if="account.error_message" class="max-w-xs break-words text-xs text-red-500">{{ account.error_message }}</p></td>
                <td class="p-3"><div class="flex gap-2"><button class="btn btn-secondary btn-sm" @click="openAccount(account)">{{ t('common.edit') }}</button><button class="btn btn-secondary btn-sm" @click="pendingDelete = { kind: 'account', id: account.id }">{{ t('common.delete') }}</button></div></td>
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

    <BaseDialog :show="showAccount" :title="editingAccount ? t('relay.editAccount') : t('relay.addAccount')" @close="showAccount = false">
      <form id="relay-account-form" class="space-y-4" @submit.prevent="saveAccount">
        <div><label for="relay-account-name" class="input-label">{{ t('relay.name') }}</label><input id="relay-account-name" v-model="accountForm.name" class="input" required maxlength="100" /></div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div><label for="relay-platform" class="input-label">{{ t('relay.platform') }}</label><select id="relay-platform" v-model="accountForm.platform" class="input" :disabled="!!editingAccount"><option v-for="platform in platformOptions" :key="platform.value" :value="platform.value">{{ platform.label }}</option></select></div>
          <div><label for="relay-type" class="input-label">{{ t('relay.type') }}</label><select id="relay-type" v-model="accountForm.type" class="input" :disabled="!!editingAccount"><option value="apikey">{{ t('relay.apiKey') }}</option><option value="oauth">{{ t('relay.oauth') }}</option></select></div>
        </div>
        <div><label for="relay-base-url" class="input-label">{{ t('relay.baseUrl') }}</label><input id="relay-base-url" v-model="accountForm.baseUrl" class="input" type="url" placeholder="https://api.example.com" /><p class="input-hint">{{ t('relay.baseUrlHint') }}</p></div>
        <div v-if="accountForm.type === 'apikey'"><label for="relay-secret" class="input-label">{{ t('relay.secret') }}</label><input id="relay-secret" v-model="accountForm.secret" type="password" class="input" autocomplete="new-password" :required="!editingAccount" /><p class="input-hint">{{ t('relay.secretHint') }}</p></div>
        <div><label for="relay-credentials" class="input-label">{{ t('relay.credentialJson') }}</label><textarea id="relay-credentials" v-model="accountForm.json" class="input font-mono" rows="4" spellcheck="false" placeholder="{}" /><p class="input-hint">{{ t('relay.credentialHint') }}</p></div>
        <div class="grid gap-4 sm:grid-cols-2"><div><label for="relay-concurrency" class="input-label">{{ t('relay.concurrency') }}</label><input id="relay-concurrency" v-model.number="accountForm.concurrency" type="number" min="1" max="100" required class="input" /></div><div><label for="relay-status" class="input-label">{{ t('relay.status') }}</label><select id="relay-status" v-model="accountForm.status" class="input"><option v-if="editingAccount && !['active', 'disabled'].includes(String(editingAccount.status))" :value="editingAccount.status">{{ t('relay.keepStatus') }}</option><option value="active">{{ t('relay.active') }}</option><option value="disabled">{{ t('relay.disabled') }}</option></select></div></div>
        <fieldset><legend class="input-label">{{ t('relay.privateSelection') }}</legend><label v-for="group in eligiblePrivateGroups" :key="group.id" class="mb-2 flex items-center gap-2"><input v-model="accountForm.groupIds" type="checkbox" :value="group.id" /><span>{{ displayName(group) }}</span></label></fieldset>
        <fieldset><legend class="input-label">{{ t('relay.publicSelection') }}</legend><p class="input-hint mb-3">{{ t('relay.publicSelectionHint') }}</p><label v-for="group in eligiblePublicGroups" :key="group.id" class="mb-2 flex items-center gap-2"><input v-model="accountForm.groupIds" type="checkbox" :value="group.id" /><span>{{ group.name }} · {{ Math.round((group.supplier_rate ?? 0.9) * 100) }}%</span></label></fieldset>
      </form>
      <template #footer><button class="btn btn-secondary" @click="showAccount = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" type="submit" form="relay-account-form" :disabled="saving">{{ t('common.save') }}</button></template>
    </BaseDialog>

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
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { relayApi, type RelayEarnings } from '@/api/relay'
import { useAppStore } from '@/stores/app'
import type { Account, Group } from '@/types'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'

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
const showGroup = ref(false)
const pendingDelete = ref<{ kind: 'account' | 'group'; id: number } | null>(null)
const accountForm = reactive({ name: '', platform: 'openai', type: 'apikey', baseUrl: '', secret: '', json: '', concurrency: 3, status: 'active', groupIds: [] as number[] })
const groupForm = reactive({ name: '', platform: 'composite', description: '', status: 'active' })
const platformOptions = CONCRETE_PLATFORM_OPTIONS
const compatible = (group: Group) => group.platform === 'composite' || group.platform === accountForm.platform
const eligiblePrivateGroups = computed(() => privateGroups.value.filter(compatible))
const eligiblePublicGroups = computed(() => publicGroups.value.filter(group => compatible(group) && (!group.require_oauth_only || accountForm.type === 'oauth')))
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
  editingAccount.value = account || null
  const editable = new Set(['api_base_urls', 'model_mapping', 'account_mode', 'upstream_protocol', 'auth_mode', 'organization_id', 'organization_uuid', 'project_id', 'scope', 'email', 'expires_at', 'token_type'])
  const extra = Object.fromEntries(Object.entries(account?.credentials || {}).filter(([key]) => editable.has(key)))

  Object.assign(accountForm, { name: account?.name || '', platform: account?.platform || 'openai', type: account?.type || 'apikey',
    baseUrl: String(account?.credentials?.base_url || ''), secret: '', json: Object.keys(extra).length ? JSON.stringify(extra, null, 2) : '', concurrency: account?.concurrency || 3,
    status: account ? String(account.status) : 'active', groupIds: account ? [...(account.group_ids || [])] : privateGroups.value.slice(0, 1).map(group => group.id) })
  showAccount.value = true
}

async function saveAccount() {
  let credentials: Record<string, unknown>
  try {
    const extra: unknown = JSON.parse(accountForm.json.trim() || '{}')
    if (!extra || typeof extra !== 'object' || Array.isArray(extra)) throw new Error('invalid')
    credentials = { ...extra as Record<string, unknown>, base_url: accountForm.baseUrl.trim() }
    if (accountForm.secret.trim()) credentials.api_key = accountForm.secret.trim()
  } catch { app.showError(t('relay.invalidJson')); return }
  if (!accountForm.groupIds.length) { app.showError(t('relay.groupRequired')); return }
  saving.value = true
  try {
    const input = { name: accountForm.name.trim(), platform: accountForm.platform, type: accountForm.type,
      credentials, concurrency: accountForm.concurrency, status: accountForm.status, group_ids: accountForm.groupIds }
    if (editingAccount.value) await relayApi.updateAccount(editingAccount.value.id, input)
    else await relayApi.createAccount(input)
    accountForm.secret = ''; accountForm.json = ''; showAccount.value = false
    app.showSuccess(t('relay.saved')); await load()
  } catch (error) { app.showError((error as { message?: string }).message || t('relay.saveFailed')) }
  finally { saving.value = false }
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
