<template>
  <BaseDialog :show="show" :title="t('admin.accounts.pools.title')" width="extra-wide" :close-on-escape="!saving" @close="close">
    <div class="space-y-5">
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.accounts.pools.description') }}</p>
      <p v-if="error" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" role="alert">{{ error }}</p>
      <div class="flex flex-wrap gap-2" role="tablist" :aria-label="t('admin.accounts.pools.title')">
        <button type="button" role="tab" :aria-selected="tab === 'pools'" :class="tab === 'pools' ? 'btn btn-primary' : 'btn btn-secondary'" @click="tab = 'pools'">{{ t('admin.accounts.pools.membersTab') }}</button>
        <button type="button" role="tab" :aria-selected="tab === 'policy'" :class="tab === 'policy' ? 'btn btn-primary' : 'btn btn-secondary'" @click="tab = 'policy'">{{ t('admin.accounts.pools.policyTab') }}</button>
        <button type="button" class="btn btn-secondary ml-auto" :disabled="loading || saving" @click="load">{{ t('common.refresh') }}</button>
      </div>

      <div v-if="loading" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
      <div v-else-if="tab === 'pools'" class="grid gap-5 md:grid-cols-[15rem_minmax(0,1fr)]">
        <div class="space-y-4">
          <label for="account-pool-list" class="input-label">{{ t('admin.accounts.pools.choosePool') }}</label>
          <select id="account-pool-list" v-model="selectedPool" class="input" :disabled="saving" @change="changePool">
            <option value="">{{ t('admin.accounts.pools.ungrouped') }}</option>
            <option v-for="pool in pools" :key="pool.id" :value="String(pool.id)">{{ pool.name }}</option>
          </select>
          <form class="space-y-3 rounded-xl border border-gray-200 p-3 dark:border-dark-700" @submit.prevent="savePool">
            <h4 class="text-sm font-medium">{{ currentPool ? t('admin.accounts.pools.editPool') : t('admin.accounts.pools.createPool') }}</h4>
            <div>
              <label for="account-pool-name" class="input-label">{{ t('common.name') }}</label>
              <input id="account-pool-name" v-model="poolName" class="input" maxlength="100" required :disabled="saving" />
            </div>
            <label class="flex items-center gap-2 text-sm">
              <input v-model="poolEnabled" type="checkbox" :disabled="saving" class="rounded border-gray-300" />
              {{ t('admin.accounts.pools.enabled') }}
            </label>
            <p class="text-xs text-gray-500">{{ t('admin.accounts.pools.enabledHint') }}</p>
            <div class="flex flex-wrap gap-2">
              <button type="submit" class="btn btn-primary" :disabled="saving || !poolName.trim()">{{ t('common.save') }}</button>
              <button v-if="currentPool" type="button" class="btn btn-secondary" :disabled="saving" @click="newPool">{{ t('admin.accounts.pools.createPool') }}</button>
              <button v-if="currentPool" type="button" class="btn btn-secondary text-red-600" :disabled="saving || members.length > 0" @click="deletePool">{{ t('common.delete') }}</button>
            </div>
            <p v-if="currentPool" class="text-xs text-gray-500">{{ t('admin.accounts.pools.deleteHint') }}</p>
          </form>
        </div>

        <div class="min-w-0 space-y-3">
          <div class="flex flex-wrap items-center gap-2">
            <input v-model="memberSearch" class="input min-w-0 flex-1" :placeholder="t('admin.accounts.searchAccounts')" :aria-label="t('admin.accounts.searchAccounts')" />
            <button v-if="currentPool" type="button" class="btn btn-primary" :disabled="saving || selectedAccountIds.length === 0" @click="addSelected">
              {{ t('admin.accounts.pools.addSelected', { count: selectedAccountIds.length }) }}
            </button>
          </div>
          <p class="text-xs text-gray-500">{{ t('admin.accounts.pools.selectionHint') }}</p>
          <p class="rounded-lg bg-gray-50 px-3 py-2 text-sm dark:bg-dark-700/50">{{ t('admin.accounts.pools.counts', { active: activeCount, total: members.length, limit: policy.max_active_accounts }) }}</p>
          <div class="max-h-[26rem] overflow-auto rounded-xl border border-gray-200 dark:border-dark-700">
            <table class="w-full text-left text-sm">
              <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800 dark:text-dark-300">
                <tr><th class="p-3">{{ t('common.name') }}</th><th class="p-3">{{ t('common.status') }}</th><th class="p-3">{{ t('admin.accounts.pools.exitGroup') }}</th><th class="p-3">{{ t('common.actions') }}</th></tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="member in visibleMembers" :key="member.account_id">
                  <td class="p-3"><div class="font-medium">{{ member.name }}</div><div class="text-xs text-gray-400">#{{ member.account_id }}</div></td>
                  <td class="p-3"><span>{{ stateLabel(member.state) }}</span><p v-if="member.reason" class="mt-1 max-w-56 break-words text-xs text-gray-500">{{ reasonLabel(member.reason) }}</p></td>
                  <td class="p-3 text-xs">{{ member.exit_group || t('admin.accounts.pools.defaultExit') }}</td>
                  <td class="p-3">
                    <div v-if="currentPool" class="flex gap-1">
                      <button type="button" class="btn btn-secondary px-2 py-1" :disabled="saving || memberSearch !== '' || members[0]?.account_id === member.account_id" :aria-label="t('admin.accounts.pools.moveUp')" @click="moveMember(member.account_id, -1)">↑</button>
                      <button type="button" class="btn btn-secondary px-2 py-1" :disabled="saving || memberSearch !== '' || members[members.length - 1]?.account_id === member.account_id" :aria-label="t('admin.accounts.pools.moveDown')" @click="moveMember(member.account_id, 1)">↓</button>
                      <button type="button" class="btn btn-secondary px-2 py-1" :disabled="saving" @click="removeMember(member.account_id)">{{ t('admin.accounts.pools.removeMember') }}</button>
                    </div>
                  </td>
                </tr>
                <tr v-if="visibleMembers.length === 0"><td colspan="4" class="p-6 text-center text-gray-500">{{ t('admin.accounts.pools.noMembers') }}</td></tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <form v-else class="space-y-4" @submit.prevent="savePolicy">
        <label class="flex items-center gap-2 text-sm font-medium">
          <input v-model="policy.enabled" type="checkbox" class="rounded border-gray-300" :disabled="saving" />
          {{ t('admin.accounts.pools.policyEnabled') }}
        </label>
        <p class="text-sm text-gray-500">{{ t('admin.accounts.pools.policyHint') }}</p>
        <div class="max-w-xs">
          <label for="pool-active-limit" class="input-label">{{ t('admin.accounts.pools.activeLimit') }}</label>
          <input id="pool-active-limit" v-model.number="policy.max_active_accounts" type="number" min="1" max="100" step="1" required class="input" :disabled="saving" />
        </div>
        <p class="rounded-lg bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-900/20 dark:text-blue-200">{{ t('admin.accounts.pools.rotationHint') }}</p>
        <div class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <h4 class="text-sm font-medium">{{ t('admin.accounts.pools.exitMapping') }}</h4>
          <p class="text-xs text-gray-500">{{ t('admin.accounts.pools.exitMappingHint') }}</p>
          <div v-for="exit in exitOptions" :key="exit.key" class="grid items-center gap-2 sm:grid-cols-2">
            <label :for="`pool-exit-${exit.key}`" class="text-sm">{{ exit.label }}</label>
            <input :id="`pool-exit-${exit.key}`" v-model="policy.proxy_exit_groups[exit.key]" class="input" maxlength="100" :placeholder="t('admin.accounts.pools.defaultExit')" :disabled="saving" />
          </div>
        </div>
        <button type="submit" class="btn btn-primary" :disabled="saving">{{ t('common.save') }}</button>
      </form>
    </div>
    <template #footer><button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.close') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { accountSchedulingPoolsAPI, type AccountSchedulingPool, type AccountSchedulingMember, type AccountSchedulingPolicy } from '@/api/admin/accountSchedulingPools'
import { useAppStore } from '@/stores/app'

const props = withDefaults(defineProps<{ show: boolean; selectedAccountIds?: number[]; proxies?: { id: number; name: string }[] }>(), { selectedAccountIds: () => [], proxies: () => [] })
const emit = defineEmits<{ (e: 'close'): void; (e: 'updated'): void }>()
const { t, te } = useI18n()
const appStore = useAppStore()
const tab = ref<'pools' | 'policy'>('pools')
const pools = ref<AccountSchedulingPool[]>([])
const members = ref<AccountSchedulingMember[]>([])
const selectedPool = ref('')
const poolName = ref('')
const poolEnabled = ref(false)
const memberSearch = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const policy = ref<AccountSchedulingPolicy>({ enabled: false, max_active_accounts: 3, proxy_exit_groups: {} })
const currentPool = computed(() => pools.value.find(pool => String(pool.id) === selectedPool.value))
const activeCount = computed(() => members.value.filter(member => member.state === 'active' || member.state === 'draining').length)
const visibleMembers = computed(() => members.value.filter(member => `${member.name} ${member.account_id}`.toLowerCase().includes(memberSearch.value.trim().toLowerCase())))
const exitOptions = computed(() => [
  { key: 'direct', label: t('admin.accounts.pools.direct') },
  ...props.proxies.map(proxy => ({ key: String(proxy.id), label: proxy.name })),
  ...Object.keys(policy.value.proxy_exit_groups).filter(key => key !== 'direct' && !props.proxies.some(proxy => String(proxy.id) === key)).map(key => ({ key, label: `#${key}` }))
])

function stateLabel(state: string) {
  const key = `admin.accounts.pools.states.${state}`
  return te(key) ? t(key) : state
}
function reasonLabel(reason: string) {
  const key = `admin.accounts.pools.reasons.${reason}`
  return te(key) ? t(key) : reason
}
function close() { if (!saving.value) emit('close') }
function setError(err: unknown) { error.value = err instanceof Error ? err.message : t('admin.accounts.pools.saveFailed') }
function resetPoolForm() { poolName.value = currentPool.value?.name ?? ''; poolEnabled.value = currentPool.value?.enabled ?? false }
async function loadMembers() {
  resetPoolForm()
  const id = currentPool.value?.id ?? null
  const result = await accountSchedulingPoolsAPI.members(id)
  members.value = id === null ? result.filter(member => member.pool_id == null) : result
}
async function changePool() {
  loading.value = true; error.value = ''
  try { await loadMembers() } catch (err) { setError(err) }
  finally { loading.value = false }
}
async function load() {
  loading.value = true; error.value = ''
  try {
    const [poolResult, policyResult] = await Promise.all([accountSchedulingPoolsAPI.list(), accountSchedulingPoolsAPI.policy()])
    pools.value = poolResult
    policy.value = { ...policyResult, proxy_exit_groups: { ...policyResult.proxy_exit_groups } }
    if (selectedPool.value && !currentPool.value) selectedPool.value = ''
    await loadMembers()
  } catch (err) { setError(err) }
  finally { loading.value = false }
}
async function mutate(action: () => Promise<void>) {
  saving.value = true; error.value = ''
  try { await action(); await load(); emit('updated'); appStore.showSuccess(t('admin.accounts.pools.saved')) }
  catch (err) { setError(err) }
  finally { saving.value = false }
}
function newPool() { selectedPool.value = ''; memberSearch.value = ''; void changePool() }
async function savePool() {
  const id = currentPool.value?.id
  await mutate(async () => {
    const input = { name: poolName.value.trim(), enabled: poolEnabled.value }
    const pool = id ? await accountSchedulingPoolsAPI.update(id, input) : await accountSchedulingPoolsAPI.create(input)
    selectedPool.value = String(pool.id)
  })
}
async function deletePool() {
  if (!currentPool.value || members.value.length > 0) return
  const id = currentPool.value.id
  await mutate(async () => { await accountSchedulingPoolsAPI.remove(id); selectedPool.value = '' })
}
async function addSelected() {
  if (!currentPool.value || props.selectedAccountIds.length === 0) return
  const id = currentPool.value.id
  await mutate(() => accountSchedulingPoolsAPI.assign(id, [...props.selectedAccountIds]))
}
async function removeMember(id: number) { await mutate(() => accountSchedulingPoolsAPI.assign(null, [id])) }
async function moveMember(id: number, direction: number) {
  if (!currentPool.value || memberSearch.value) return
  const ids = members.value.map(member => member.account_id)
  const index = ids.indexOf(id); const target = index + direction
  if (index < 0 || target < 0 || target >= ids.length) return
  ;[ids[index], ids[target]] = [ids[target]!, ids[index]!]
  const poolId = currentPool.value.id
  await mutate(() => accountSchedulingPoolsAPI.assign(poolId, ids))
}
async function savePolicy() {
  const value = policy.value
  const payload = { ...value, proxy_exit_groups: Object.fromEntries(Object.entries(value.proxy_exit_groups).map(([key, group]) => [key, group.trim()]).filter(([, group]) => group)) }
  await mutate(() => accountSchedulingPoolsAPI.updatePolicy(payload))
}
watch(() => props.show, open => { if (open) void load() }, { immediate: true })
</script>
