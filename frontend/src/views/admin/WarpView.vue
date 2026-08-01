<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3 w-full">
          <div class="flex flex-wrap items-center gap-3">
            <div class="flex items-center gap-2 text-sm">
              <span class="text-gray-500">{{ t('admin.warp.gateway') }}</span>
              <span :class="enabled ? 'badge badge-success' : 'badge badge-gray'">
                {{ enabled ? t('admin.warp.enabled') : t('admin.warp.disabled') }}
              </span>
            </div>
            <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
              <button class="btn btn-secondary" :disabled="loading" @click="refresh" :title="t('common.refresh')">
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <button class="btn btn-secondary" :disabled="!enabled || loading" @click="doSync">
                {{ t('admin.warp.sync') }}
              </button>
              <button class="btn btn-secondary" :disabled="!enabled || loading" @click="doHealthSync">
                {{ t('admin.warp.healthSync') }}
              </button>
              <button class="btn btn-secondary" :disabled="!enabled || loading" @click="showBind = true">
                {{ t('admin.warp.bindAccounts') }}
              </button>
              <button class="btn btn-secondary" :disabled="!enabled || loading" @click="showCreate = true">
                <Icon name="plus" size="md" class="mr-2" />
                {{ t('admin.warp.createPool') }}
              </button>
              <button class="btn btn-primary" :disabled="!enabled || loading" @click="showRegister = true">
                {{ t('admin.warp.registerPool') }}
              </button>
            </div>
          </div>
          <p v-if="!enabled" class="text-sm text-amber-600 dark:text-amber-400">
            {{ t('admin.warp.disabledHint') }}
          </p>
          <div v-if="lastResult" class="text-xs text-gray-500 dark:text-gray-400 flex flex-wrap gap-3">
            <span>{{ t('admin.warp.lastSync') }}:
              {{ t('admin.warp.created', { n: lastResult.created_proxies?.length || 0 }) }},
              {{ t('admin.warp.updated', { n: lastResult.updated_proxies?.length || 0 }) }},
              {{ t('admin.warp.members', { n: lastResult.member_ids?.length || 0 }) }}
            </span>
            <span v-if="lastResult.group">
              {{ t('admin.warp.group') }}: {{ lastResult.group.name }} (#{{ lastResult.group.id }})
            </span>
          </div>
          <div v-if="alerts.length" class="rounded-md border border-amber-300 bg-amber-50 dark:bg-amber-900/20 p-2 text-xs text-amber-800 dark:text-amber-200 space-y-1">
            <div v-for="(a, i) in alerts" :key="i">{{ a }}</div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="instances" :loading="loading">
          <template #cell-name="{ row }">
            <div class="font-medium text-gray-900 dark:text-white">{{ row.name }}</div>
            <div class="text-xs text-gray-500 font-mono">{{ row.id }}</div>
          </template>
          <template #cell-socks="{ row }">
            <code class="text-xs">socks5h://{{ row.listen_host || '127.0.0.1' }}:{{ row.listen_port }}</code>
          </template>
          <template #cell-status="{ row }">
            <span :class="statusClass(row.status)">{{ row.status }}</span>
          </template>
          <template #cell-exit="{ row }">
            <div>{{ row.exit_ip || '—' }}</div>
            <div v-if="row.exit_colo" class="text-xs text-gray-500">{{ row.exit_colo }}</div>
          </template>
          <template #cell-actions="{ row }">
            <button
              class="btn-icon"
              :disabled="!enabled || loading"
              :title="t('admin.warp.rotate')"
              @click="doRotate(row)"
            >
              <Icon name="refresh" size="sm" />
            </button>
          </template>
          <template #empty>
            <EmptyState
              :title="t('admin.warp.emptyTitle')"
              :description="t('admin.warp.emptyDesc')"
            />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="showCreate"
      :title="t('admin.warp.createPool')"
      width="normal"
      @close="showCreate = false"
    >
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.warp.namePrefix') }}</label>
          <input v-model="form.name_prefix" type="text" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.warp.count') }}</label>
          <input v-model.number="form.count" type="number" min="1" max="50" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.warp.groupName') }}</label>
          <input v-model="form.group_name" type="text" class="input" />
        </div>
        <label class="flex items-start gap-2 text-sm cursor-pointer">
          <input v-model="form.register" type="checkbox" class="mt-1" />
          <span>
            <span class="font-medium">{{ t('admin.warp.registerReal') }}</span>
            <span class="block text-xs text-gray-500">{{ t('admin.warp.registerRealHint') }}</span>
          </span>
        </label>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="showCreate = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="loading" @click="doCreatePool">
            {{ t('common.confirm') }}
          </button>
        </div>
      </div>
    </BaseDialog>

    <BaseDialog
      :show="showRegister"
      :title="t('admin.warp.registerPool')"
      width="normal"
      @close="showRegister = false"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.warp.registerRealHint') }}</p>
        <div>
          <label class="input-label">{{ t('admin.warp.namePrefix') }}</label>
          <input v-model="form.name_prefix" type="text" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.warp.count') }}</label>
          <input v-model.number="form.count" type="number" min="1" max="20" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.warp.groupName') }}</label>
          <input v-model="form.group_name" type="text" class="input" />
        </div>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="showRegister = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="loading" @click="doRegisterPool">
            {{ t('common.confirm') }}
          </button>
        </div>
      </div>
    </BaseDialog>

    <BaseDialog
      :show="showBind"
      :title="t('admin.warp.bindAccounts')"
      width="normal"
      @close="showBind = false"
    >
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.warp.groupName') }}</label>
          <input v-model="bindForm.group_name" type="text" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.warp.accountIds') }}</label>
          <input v-model="bindForm.account_ids_text" type="text" class="input" placeholder="1,2,3" />
        </div>
        <label class="flex items-center gap-2 text-sm cursor-pointer">
          <input v-model="bindForm.bind_all_active" type="checkbox" />
          <span>{{ t('admin.warp.bindAllActive') }}</span>
        </label>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="showBind = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="loading" @click="doBindAccounts">
            {{ t('common.confirm') }}
          </button>
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import warpAPI, { type WarpInstance, type WarpSyncResult } from '@/api/admin/warp'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const enabled = ref(false)
const instances = ref<WarpInstance[]>([])
const lastResult = ref<WarpSyncResult | null>(null)
const alerts = ref<string[]>([])
const showCreate = ref(false)
const showRegister = ref(false)
const showBind = ref(false)
const form = reactive({
  name_prefix: 'warp',
  count: 3,
  group_name: 'warp-pool',
  register: true
})
const bindForm = reactive({
  group_name: 'warp-pool',
  account_ids_text: '',
  bind_all_active: false
})

const columns = computed(() => [
  { key: 'name', label: t('admin.warp.colName') },
  { key: 'socks', label: t('admin.warp.colSocks') },
  { key: 'status', label: t('admin.warp.colStatus') },
  { key: 'exit', label: t('admin.warp.colExit') },
  { key: 'actions', label: t('common.actions'), width: '80px' }
])

function statusClass(s: string) {
  if (s === 'running') return 'badge badge-success'
  if (s === 'unhealthy' || s === 'error') return 'badge badge-danger'
  return 'badge badge-gray'
}

function applyResult(res: WarpSyncResult) {
  lastResult.value = res
  alerts.value = res.alerts || []
  if (res.snapshot?.instances) {
    instances.value = res.snapshot.instances
  }
}

async function refresh() {
  loading.value = true
  try {
    const st = await warpAPI.status()
    enabled.value = !!st.enabled
    if (!enabled.value) {
      instances.value = []
      return
    }
    instances.value = await warpAPI.listInstances()
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.warp.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function doSync() {
  loading.value = true
  try {
    const res = await warpAPI.sync(form.group_name)
    applyResult(res)
    await refresh()
    appStore.showSuccess(t('admin.warp.syncOk'))
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.warp.syncFailed'))
  } finally {
    loading.value = false
  }
}

async function doHealthSync() {
  loading.value = true
  try {
    const res = await warpAPI.healthSync(form.group_name)
    applyResult(res)
    await refresh()
    appStore.showSuccess(t('admin.warp.healthSyncOk'))
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.warp.syncFailed'))
  } finally {
    loading.value = false
  }
}

async function doCreatePool() {
  if (!form.count || form.count < 1) return
  loading.value = true
  try {
    const res = await warpAPI.createPool({
      name_prefix: form.name_prefix || 'warp',
      count: form.count,
      group_name: form.group_name || 'warp-pool',
      register: !!form.register
    })
    applyResult(res)
    showCreate.value = false
    await refresh()
    appStore.showSuccess(t('admin.warp.createOk'))
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.warp.createFailed'))
  } finally {
    loading.value = false
  }
}

async function doRegisterPool() {
  if (!form.count || form.count < 1) return
  loading.value = true
  try {
    const res = await warpAPI.registerPool({
      name_prefix: form.name_prefix || 'warp',
      count: form.count,
      group_name: form.group_name || 'warp-pool'
    })
    applyResult(res)
    showRegister.value = false
    await refresh()
    appStore.showSuccess(t('admin.warp.registerOk'))
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.warp.registerFailed'))
  } finally {
    loading.value = false
  }
}

async function doBindAccounts() {
  loading.value = true
  try {
    const ids = bindForm.account_ids_text
      .split(/[,\s]+/)
      .map((s) => s.trim())
      .filter(Boolean)
      .map((s) => Number(s))
      .filter((n) => Number.isFinite(n) && n > 0)
    const res = await warpAPI.bindAccounts({
      account_ids: ids,
      group_name: bindForm.group_name || 'warp-pool',
      bind_all_active: bindForm.bind_all_active
    })
    showBind.value = false
    appStore.showSuccess(
      `${t('admin.warp.bindOk')} (${res.updated_ids?.length || 0})`
    )
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.warp.bindFailed'))
  } finally {
    loading.value = false
  }
}

async function doRotate(row: WarpInstance) {
  loading.value = true
  try {
    const res = await warpAPI.rotate(row.id, form.group_name)
    applyResult(res)
    await refresh()
    appStore.showSuccess(t('admin.warp.rotateOk'))
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.warp.rotateFailed'))
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
</script>
