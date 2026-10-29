<template>
  <section class="card space-y-4 p-6" :aria-label="t('adminAccessKeys.title')">
    <h2 class="text-lg font-semibold">{{ t('adminAccessKeys.title') }}</h2>
    <p class="text-sm text-gray-500">{{ t('adminAccessKeys.authorizationHint') }}</p>
    <p class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">{{ t('adminAccessKeys.sharedAccountWarning') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <p v-if="notice" role="status" class="text-sm text-green-600">{{ notice }}</p>
    <div v-if="secret" class="space-y-2 rounded-lg border border-amber-300 p-4">
      <p>{{ t('adminAccessKeys.secretWarning') }}</p>
      <code class="block select-all break-all">{{ secret }}</code>
      <button type="button" class="btn btn-secondary btn-sm" @click="copySecret">{{ t('adminAccessKeys.copySecret') }}</button>
      <button type="button" data-test="dismiss-secret" class="btn btn-secondary btn-sm" @click="secret = ''">{{ t('adminAccessKeys.dismissSecret') }}</button>
    </div>
    <div class="flex gap-2">
      <button type="button" data-test="create" class="btn btn-primary" :disabled="busy || loading || !loaded" @click="edit()">{{ t('adminAccessKeys.create') }}</button>
      <button type="button" class="btn btn-secondary" :disabled="busy || loading" @click="load">{{ t('adminAccessKeys.refresh') }}</button>
    </div>
    <p v-if="loading">{{ t('adminAccessKeys.loading') }}</p>
    <div v-for="item in items" :key="item.id" class="space-y-2 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
      <p class="font-medium">{{ item.name }} <code>{{ item.key_prefix }}…</code> · {{ status(item) }}</p>
      <p class="text-sm text-gray-500">{{ item.description || t('adminAccessKeys.noDescription') }} · {{ t('adminAccessKeys.expires', { value: item.expires_at || t('adminAccessKeys.never') }) }}</p>
      <p class="text-sm text-gray-500">{{ t('adminAccessKeys.lastUsed', { value: item.last_used_at || t('adminAccessKeys.neverUsed') }) }} {{ item.last_used_ip }}</p>
      <div class="space-y-1 text-sm text-gray-500" :aria-label="t('adminAccessKeys.summaryLabel')">
        <p>{{ t('adminAccessKeys.summary', { globals: item.policy.global_permissions.length, rules: item.policy.rules.length }) }}</p>
        <p v-for="(rule, index) in item.policy.rules" :key="index">{{ t('adminAccessKeys.ruleLine', { index: index + 1, summary: ruleSummary(rule) }) }}</p>
      </div>
      <div v-if="!item.revoked_at" class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="edit(item)">{{ t('adminAccessKeys.editPermissions') }}</button>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="toggle(item)">{{ item.enabled ? t('adminAccessKeys.disable') : t('adminAccessKeys.enable') }}</button>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="rotateKey(item)">{{ t('adminAccessKeys.rotate') }}</button>
        <button type="button" class="btn btn-secondary btn-sm text-red-600" :disabled="busy" @click="revokeKey(item)">{{ t('adminAccessKeys.revoke') }}</button>
      </div>
      <div v-else>
        <button type="button" class="btn btn-secondary btn-sm text-red-600" :disabled="busy" @click="deleteRecord(item)">{{ t('adminAccessKeys.deleteRecord') }}</button>
      </div>
    </div>
    <div v-if="draft" class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
      <h3 class="font-semibold">{{ editingID ? t('adminAccessKeys.editTitle') : t('adminAccessKeys.createTitle') }}</h3>
      <label class="block">{{ t('adminAccessKeys.name') }}<input v-model="draft.name" data-test="name" class="input mt-1" maxlength="100" /></label>
      <label class="block">{{ t('adminAccessKeys.description') }}<textarea v-model="draft.description" class="input mt-1" maxlength="1000" /></label>
      <label class="block">{{ t('adminAccessKeys.expiration') }}<input v-model="expiration" type="datetime-local" class="input mt-1" /></label>
      <label class="block">{{ t('adminAccessKeys.allowedIps') }}<textarea v-model="ips" class="input mt-1" placeholder="192.0.2.1&#10;2001:db8::/32" /></label>
      <label class="flex gap-2"><input v-model="draft.enabled" type="checkbox" />{{ t('adminAccessKeys.enabled') }}</label>
      <fieldset v-for="(rule, index) in draft.policy.rules" :key="index" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
        <legend>{{ t('adminAccessKeys.ruleLegend', { index: index + 1 }) }}</legend>
        <label class="flex gap-2"><input v-model="rule.all_groups" type="checkbox" />{{ t('adminAccessKeys.allGroups') }}</label>
        <label class="block">{{ t('adminAccessKeys.groups') }}<select v-model="rule.group_ids" :data-test="`groups-${index}`" multiple class="input mt-1" :disabled="rule.all_groups"><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label>
        <label class="flex gap-2"><input v-model="rule.include_ungrouped" type="checkbox" />{{ t('adminAccessKeys.includeUngrouped') }}</label>
        <div>{{ t('adminAccessKeys.accountTypes') }}<div class="flex flex-wrap gap-4"><label v-for="type in accountTypes" :key="type" class="flex gap-2"><input v-model="rule.account_types" type="checkbox" :value="type" />{{ type }}</label></div></div>
        <label class="block">{{ t('adminAccessKeys.platforms') }}<select v-model="rule.platforms" multiple class="input mt-1"><option v-for="platform in CONCRETE_PLATFORM_OPTIONS" :key="platform.value" :value="platform.value">{{ platform.label }}</option></select></label>
        <div>{{ t('adminAccessKeys.accountOperations') }}<div class="grid gap-2 sm:grid-cols-2"><label v-for="permission in accountPermissions" :key="permission.id" class="flex gap-2"><input v-model="rule.permissions" type="checkbox" :value="permission.id" />{{ permission.label }} <span class="text-xs text-gray-500">{{ permission.id }}</span></label></div></div>
        <div v-if="groupPermissions.length" class="space-y-2 rounded border border-amber-300 p-3">
          <p>{{ t('adminAccessKeys.groupOperations') }}</p>
          <p class="text-sm text-amber-700 dark:text-amber-300">{{ t('adminAccessKeys.modelAllowlistHint') }}</p>
          <div class="grid gap-2 sm:grid-cols-2"><label v-for="permission in groupPermissions" :key="permission.id" class="flex gap-2"><input v-model="rule.permissions" type="checkbox" :value="permission.id" />{{ permission.label }} <span class="text-xs text-gray-500">{{ permission.id }}</span></label></div>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" @click="draft.policy.rules.splice(index, 1)">{{ t('adminAccessKeys.removeRule') }}</button>
      </fieldset>
      <button type="button" data-test="add-rule" class="btn btn-secondary btn-sm" @click="draft.policy.rules.push(emptyRule())">{{ t('adminAccessKeys.addRule') }}</button>
      <fieldset class="space-y-3 rounded-lg border border-amber-300 p-3">
        <legend>{{ t('adminAccessKeys.globalPermissions') }}</legend>
        <fieldset v-for="category in globalCategories" :key="category.name" class="space-y-2 rounded border border-gray-200 p-3 dark:border-dark-700">
          <legend class="text-sm font-medium">{{ category.name }}</legend>
          <label v-for="permission in category.permissions" :key="permission.id" class="flex gap-2"><input v-model="draft.policy.global_permissions" type="checkbox" :value="permission.id" />{{ permission.label }} <span class="text-xs text-gray-500">{{ permission.id }}</span></label>
        </fieldset>
      </fieldset>
      <div class="flex gap-2"><button type="button" data-test="save" class="btn btn-primary" :disabled="busy" @click="save">{{ busy ? t('adminAccessKeys.processing') : t('adminAccessKeys.save') }}</button><button type="button" class="btn btn-secondary" :disabled="busy" @click="draft = null">{{ t('adminAccessKeys.cancel') }}</button></div>
    </div>
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '@/api/admin/adminAccessKeys'
import type { AccessRule, AdminKey, AdminKeyInput, Permission } from '@/api/admin/adminAccessKeys'
import { getAllIncludingInactive } from '@/api/admin/groups'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'

const { t } = useI18n()
const stepUp = useStepUp()
const items = ref<AdminKey[]>([])
const permissions = ref<Permission[]>([])
const groups = ref<{ id: number; name: string }[]>([])
const loading = ref(false)
const loaded = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const secret = ref('')
const draft = ref<AdminKeyInput | null>(null)
const editingID = ref('')
const expiration = ref('')
const initialExpiration = ref('')
const ips = ref('')
const accountTypes = ['oauth', 'setup-token', 'apikey', 'upstream', 'bedrock', 'service_account']
const accountPermissions = computed(() => permissions.value.filter(p => p.scope === 'account'))
const groupPermissions = computed(() => permissions.value.filter(p => p.scope === 'group'))
const globalCategories = computed(() => {
  const categories = new Map<string, Permission[]>()
  for (const permission of permissions.value.filter(p => p.scope === 'global')) {
    const name = permission.category || t('adminAccessKeys.other')
    categories.set(name, [...(categories.get(name) || []), permission])
  }
  return Array.from(categories, ([name, permissions]) => ({ name, permissions }))
})
// Go 的空切片可能序列化为 null；统一为空数组且不增加任何授权。
function normalizeItem(item: AdminKey): AdminKey {
  return { ...item, allowed_ips: item.allowed_ips ?? [], policy: {
    global_permissions: item.policy?.global_permissions ?? [],
    rules: (item.policy?.rules ?? []).map(rule => ({ ...rule,
      permissions: rule.permissions ?? [], group_ids: rule.group_ids ?? [],
      account_types: rule.account_types ?? [], platforms: rule.platforms ?? []
    }))
  } }
}
function ruleSummary(rule: AccessRule): string {
  const selectedGroups = rule.all_groups ? t('adminAccessKeys.allGroupsSummary') : rule.group_ids.map(id => groups.value.find(group => group.id === id)?.name || t('adminAccessKeys.groupFallback', { id })).join('、') || t('adminAccessKeys.noGroups')
  const groupLabel = selectedGroups + (rule.include_ungrouped ? t('adminAccessKeys.ungroupedSummary') : '')
  const types = rule.account_types.join('、') || t('adminAccessKeys.noAccountTypes')
  const platforms = rule.platforms.join('、') || t('adminAccessKeys.anyPlatform')
  return t('adminAccessKeys.ruleSummary', { groups: groupLabel, types, platforms, count: rule.permissions.length })
}
function emptyRule(): AccessRule { return { permissions: [], group_ids: [], all_groups: false, include_ungrouped: false, account_types: [], platforms: [] } }
function report(e: unknown) { if (!isStepUpCancelled(e)) error.value = (e as { message?: string })?.message || t('adminAccessKeys.operationFailed') }
async function load() {
  loading.value = true
  error.value = ''
  try {
    const [result, allGroups] = await Promise.all([api.list(), getAllIncludingInactive()])
    items.value = (result.items ?? []).map(normalizeItem)
    permissions.value = result.permissions ?? []
    groups.value = allGroups ?? []
    loaded.value = true
  } catch (e) { report(e) } finally { loading.value = false }
}
function edit(item?: AdminKey) {
  error.value = ''; notice.value = ''; secret.value = ''
  editingID.value = item?.id || ''
  draft.value = item ? JSON.parse(JSON.stringify({ name: item.name, description: item.description, enabled: item.enabled, expires_at: item.expires_at, allowed_ips: item.allowed_ips, policy: item.policy })) : { name: '', description: '', enabled: true, expires_at: null, allowed_ips: [], policy: { rules: [emptyRule()], global_permissions: [] } }
  const date = item?.expires_at ? new Date(item.expires_at) : null
  expiration.value = date ? new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ''
  initialExpiration.value = expiration.value
  ips.value = item?.allowed_ips.join('\n') || ''
}
async function mutate(action: () => Promise<void>) {
  busy.value = true; error.value = ''; notice.value = ''; secret.value = ''
  try { await stepUp.run(action) } catch (e) { report(e) } finally { busy.value = false }
}
function replace(item: AdminKey) { items.value = [normalizeItem(item), ...items.value.filter(key => key.id !== item.id)] }
async function save() {
  if (!draft.value) return
  if (!draft.value.name.trim()) { error.value = t('adminAccessKeys.nameRequired'); return }
  const date = expiration.value ? new Date(expiration.value) : null
  if (date && !Number.isFinite(date.getTime())) { error.value = t('adminAccessKeys.expirationInvalid'); return }
  const input: AdminKeyInput = JSON.parse(JSON.stringify(draft.value))
  input.name = input.name.trim()
  // 未修改日期时保留服务端原值，避免丢失秒数、微秒或时区精度。
  if (expiration.value !== initialExpiration.value) input.expires_at = date?.toISOString() || null
  input.allowed_ips = ips.value.split(/[\n,]/).map(ip => ip.trim()).filter(Boolean)
  await mutate(async () => {
    if (editingID.value) replace(await api.update(editingID.value, input))
    else { const result = await api.create(input); replace(result.item); secret.value = result.key }
    draft.value = null
    notice.value = t('adminAccessKeys.saved')
  })
}
async function toggle(item: AdminKey) { await mutate(async () => { replace(await api.update(item.id, { name: item.name, description: item.description, enabled: !item.enabled, expires_at: item.expires_at, allowed_ips: item.allowed_ips, policy: item.policy })); notice.value = t('adminAccessKeys.statusUpdated') }) }
async function rotateKey(item: AdminKey) {
  if (!confirm(t('adminAccessKeys.rotateConfirm', { name: item.name }))) return
  await mutate(async () => { const result = await api.rotate(item.id); replace(result.item); secret.value = result.key; notice.value = t('adminAccessKeys.rotated') })
}
async function revokeKey(item: AdminKey) {
  if (!confirm(t('adminAccessKeys.revokeConfirm', { name: item.name }))) return
  await mutate(async () => { await api.revoke(item.id); replace({ ...item, enabled: false, revoked_at: new Date().toISOString() }); if (editingID.value === item.id) draft.value = null; notice.value = t('adminAccessKeys.revoked') })
}
async function deleteRecord(item: AdminKey) {
  if (busy.value || !item.revoked_at) return
  if (!confirm(t('adminAccessKeys.deleteConfirm', { name: item.name }))) return
  await mutate(async () => {
    await api.deleteRecord(item.id)
    items.value = items.value.filter(key => key.id !== item.id)
    if (editingID.value === item.id) draft.value = null
    notice.value = t('adminAccessKeys.removed')
  })
}
async function copySecret() { try { await navigator.clipboard.writeText(secret.value); notice.value = t('adminAccessKeys.copied') } catch { error.value = t('adminAccessKeys.copyFailed') } }
function status(item: AdminKey) { return item.revoked_at ? t('adminAccessKeys.statusRevoked') : !item.enabled ? t('adminAccessKeys.statusDisabled') : item.expires_at && new Date(item.expires_at).getTime() <= Date.now() ? t('adminAccessKeys.statusExpired') : t('adminAccessKeys.statusEnabled') }
onMounted(load)
</script>
