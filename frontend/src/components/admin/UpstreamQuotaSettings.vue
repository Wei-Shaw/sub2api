<template>
  <section class="card space-y-4 p-6" :aria-busy="loading">
    <div>
      <h3 class="text-lg font-medium text-gray-900 dark:text-white">{{ t('upstreamQuota.admin.title') }}</h3>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('upstreamQuota.admin.description') }}</p>
    </div>
    <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
      <input v-model="enabled" type="checkbox" :disabled="loading || !ready" class="rounded border-gray-300 text-primary-600" />
      {{ t('upstreamQuota.admin.enable') }}
    </label>
    <div>
      <label for="shared-quota-ids" class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('upstreamQuota.admin.accounts') }}</label>
      <input id="shared-quota-ids" v-model="idsText" type="text" class="input w-full" :disabled="loading || !ready" autocomplete="off" aria-describedby="shared-quota-hint" placeholder="1, 2, 3" />
      <p id="shared-quota-hint" class="mt-1 text-xs text-gray-500">{{ t('upstreamQuota.admin.hint') }}</p>
    </div>
    <p v-if="message" :role="failed ? 'alert' : 'status'" class="text-sm" :class="failed ? 'text-red-600' : 'text-emerald-600'">{{ t(message) }}</p>
    <button type="button" class="btn btn-primary" :disabled="loading || !ready" @click="save">{{ loading ? t('upstreamQuota.loading') : t('upstreamQuota.admin.save') }}</button>
  </section>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { upstreamQuotaAPI } from '@/api/upstreamQuota'
const { t } = useI18n()
const enabled = ref(false)
const idsText = ref('')
const loading = ref(true)
const ready = ref(false)
const failed = ref(false)
const message = ref('')
onMounted(async () => {
  try {
    const settings = await upstreamQuotaAPI.settings()
    enabled.value = settings.enabled
    idsText.value = settings.account_ids.join(', ')
    ready.value = true
  } catch { failed.value = true; message.value = 'upstreamQuota.admin.error' }
  finally { loading.value = false }
})
async function save() {
  const parts = idsText.value.trim() ? idsText.value.split(',').map(s => s.trim()) : []
  const ids = parts.map(Number)
  if (ids.length > 200 || parts.some(p => !/^\d+$/.test(p)) || ids.some(id => !Number.isSafeInteger(id) || id <= 0)) {
    failed.value = true; message.value = 'upstreamQuota.admin.invalid'; return
  }
  loading.value = true
  try {
    await upstreamQuotaAPI.save({ enabled: enabled.value, account_ids: [...new Set(ids)] })
    failed.value = false; message.value = 'upstreamQuota.admin.saved'
  } catch { failed.value = true; message.value = 'upstreamQuota.admin.error' }
  finally { loading.value = false }
}
</script>
