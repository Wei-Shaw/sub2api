<template>
  <BaseDialog :show="show" :title="t('admin.proxyPools.bindAccountsTitle')" width="normal" @close="emit('close')">
    <div class="space-y-5">
      <div class="rounded bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:bg-dark-700 dark:text-gray-300">
        {{ t('admin.proxyPools.bindAccountsHint', { count: accountIds.length }) }}
      </div>
      <div>
        <label class="input-label">{{ t('admin.proxyPools.pool') }}</label>
        <Select
          v-model="selectedPoolId"
          :options="poolOptions"
          :placeholder="t('admin.proxyPools.selectPool')"
          :searchable="true"
          :empty-text="t('admin.proxyPools.noPools')"
        />
      </div>
      <div v-if="selectedPool" class="grid grid-cols-3 gap-3 border-y border-gray-200 py-4 dark:border-dark-600">
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.proxies') }}</div>
          <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ selectedPool.proxy_count }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.healthy') }}</div>
          <div class="mt-1 text-lg font-semibold text-emerald-600 dark:text-emerald-400">{{ selectedPool.healthy_proxy_count }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.boundAccounts') }}</div>
          <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ selectedPool.bound_account_count }}</div>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving || !selectedPoolId" @click="bindAccounts">
          <Icon name="link" size="sm" class="mr-2" />
          {{ saving ? t('admin.proxyPools.binding') : t('admin.proxyPools.bindAccounts') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { ProxyPoolWithStats } from '@/types'

const props = defineProps<{
  show: boolean
  accountIds: number[]
  pools: ProxyPoolWithStats[]
}>()

const emit = defineEmits<{
  close: []
  bound: []
}>()

const { t } = useI18n()
const appStore = useAppStore()
const selectedPoolId = ref<number | null>(null)
const saving = ref(false)

const activePools = computed(() => props.pools.filter((pool) => pool.status === 'active'))
const poolOptions = computed(() => activePools.value.map((pool) => ({ value: pool.id, label: pool.name })))
const selectedPool = computed(() => activePools.value.find((pool) => pool.id === selectedPoolId.value) ?? null)

watch(() => props.show, (visible) => {
  if (visible) selectedPoolId.value = activePools.value[0]?.id ?? null
})

async function bindAccounts() {
  if (!selectedPoolId.value || saving.value) return
  saving.value = true
  try {
    const result = await adminAPI.proxyPools.bindAccounts(selectedPoolId.value, props.accountIds)
    if (result.failed > 0) {
      appStore.showError(t('admin.proxyPools.bindPartial', { assigned: result.assigned, failed: result.failed }))
    } else {
      appStore.showSuccess(t('admin.proxyPools.bindSuccess', { count: result.assigned }))
    }
    emit('bound')
  } catch (error) {
    const message = (error as { message?: string } | null)?.message || t('admin.proxyPools.bindFailed')
    appStore.showError(message)
  } finally {
    saving.value = false
  }
}
</script>
