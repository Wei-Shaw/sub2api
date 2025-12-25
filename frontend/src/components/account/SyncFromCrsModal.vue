<template>
  <Modal
    :show="show"
    :title="t('admin.accounts.syncFromCrsTitle')"
    size="lg"
    close-on-click-outside
    @close="handleClose"
  >
    <div class="space-y-4">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.syncFromCrsDesc') }}
      </div>
      <div class="text-xs text-gray-500 dark:text-dark-400 bg-gray-50 dark:bg-dark-700/60 rounded-lg p-3">
        已有账号仅同步 CRS 返回的字段，缺失字段保持原值；凭据按键合并，不会清空未下发的键；未勾选"同步代理"时保留原有代理。
      </div>
      <div class="text-xs text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-3">
        {{ t('admin.accounts.crsVersionRequirement') }}
      </div>

      <div class="grid grid-cols-1 gap-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.crsBaseUrl') }}</label>
          <input
            v-model="form.base_url"
            type="text"
            class="input"
            :placeholder="t('admin.accounts.crsBaseUrlPlaceholder')"
          />
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.accounts.crsUsername') }}</label>
            <input
              v-model="form.username"
              type="text"
              class="input"
              autocomplete="username"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.crsPassword') }}</label>
            <input
              v-model="form.password"
              type="password"
              class="input"
              autocomplete="current-password"
            />
          </div>
        </div>

        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-300">
          <input v-model="form.sync_proxies" type="checkbox" class="rounded border-gray-300 dark:border-dark-600" />
          {{ t('admin.accounts.syncProxies') }}
        </label>
      </div>

      <div v-if="result" class="rounded-xl border border-gray-200 dark:border-dark-700 p-4 space-y-2">
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.syncResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.syncResultSummary', result) }}
        </div>

        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.syncErrors') }}
          </div>
          <div class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 dark:bg-dark-800 p-3 text-xs font-mono">
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind }} {{ item.crs_account_id }} — {{ item.action }}{{ item.error ? `: ${item.error}` : '' }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" :disabled="syncing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" :disabled="syncing" @click="handleSync">
          {{ syncing ? t('admin.accounts.syncing') : t('admin.accounts.syncNow') }}
        </button>
      </div>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Modal from '@/components/common/Modal.vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'synced'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const syncing = ref(false)
const result = ref<Awaited<ReturnType<typeof adminAPI.accounts.syncFromCrs>> | null>(null)

const form = reactive({
  base_url: '',
  username: '',
  password: '',
  sync_proxies: true
})

const errorItems = computed(() => {
  if (!result.value?.items) return []
  return result.value.items.filter((i) => i.action === 'failed' || i.action === 'skipped')
})

watch(
  () => props.show,
  (open) => {
    if (open) {
      result.value = null
    }
  }
)

const handleClose = () => {
  emit('close')
}

const handleSync = async () => {
  if (!form.base_url.trim() || !form.username.trim() || !form.password.trim()) {
    appStore.showError(t('admin.accounts.syncMissingFields'))
    return
  }

  syncing.value = true
  try {
    const res = await adminAPI.accounts.syncFromCrs({
      base_url: form.base_url.trim(),
      username: form.username.trim(),
      password: form.password,
      sync_proxies: form.sync_proxies
    })
    result.value = res

    if (res.failed > 0) {
      appStore.showError(t('admin.accounts.syncCompletedWithErrors', res))
    } else {
      appStore.showSuccess(t('admin.accounts.syncCompleted', res))
      emit('synced')
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.syncFailed'))
  } finally {
    syncing.value = false
  }
}
</script>
