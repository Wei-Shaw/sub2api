<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.form.selectKeyTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <div class="space-y-3">
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.channelMonitor.form.selectKeyHint') }}
      </p>

      <div class="relative">
        <input
          v-model="search"
          type="text"
          class="input pl-9"
          :placeholder="t('keys.searchPlaceholder')"
        />
        <svg class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8" /><path d="m21 21-4.35-4.35" />
        </svg>
      </div>

      <div v-if="loading" class="py-6 text-center text-sm text-gray-500">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="filteredKeys.length === 0" class="py-6 text-center text-sm text-gray-500">
        {{ t('admin.channelMonitor.form.noActiveKey') }}
      </div>
      <div v-else class="max-h-96 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="w-full text-sm">
          <thead class="bg-gray-50 dark:bg-dark-800 sticky top-0 z-10">
            <tr class="text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
              <th class="px-3 py-2">{{ t('common.name') }}</th>
              <th class="px-3 py-2">{{ t('keys.apiKey') }}</th>
              <th class="px-3 py-2">{{ t('keys.group') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
            <tr
              v-for="k in filteredKeys"
              :key="k.id"
              class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700"
              @click="$emit('pick', k)"
            >
              <td class="px-3 py-2 font-medium text-gray-900 dark:text-white">{{ k.name }}</td>
              <td class="px-3 py-2 font-mono text-xs text-gray-500 dark:text-gray-400">{{ maskApiKey(k.key) }}</td>
              <td class="px-3 py-2">
                <GroupBadge
                  v-if="k.group"
                  :name="k.group.name"
                  :platform="k.group.platform"
                  :subscription-type="k.group.subscription_type"
                  :rate-multiplier="k.group.rate_multiplier"
                  :user-rate-multiplier="userGroupRates[k.group.id]"
                />
                <span v-else class="text-xs text-gray-400">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ApiKey } from '@/types'
import type { Provider } from '@/api/admin/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import { maskApiKey } from '@/utils/maskApiKey'

const props = withDefaults(defineProps<{
  show: boolean
  loading: boolean
  keys: ApiKey[]
  provider: Provider
  userGroupRates?: Record<number, number>
}>(), {
  userGroupRates: () => ({}),
})

defineEmits<{
  (e: 'close'): void
  (e: 'pick', key: ApiKey): void
}>()

const { t } = useI18n()

const search = ref('')

watch(() => props.show, (shown) => {
  if (!shown) search.value = ''
})

const filteredKeys = computed<ApiKey[]>(() => {
  const q = search.value.trim().toLowerCase()
  return props.keys.filter((k) => {
    if (k.group?.platform !== props.provider) return false
    if (!q) return true
    return (
      k.name.toLowerCase().includes(q) ||
      k.key.toLowerCase().includes(q) ||
      (k.group?.name || '').toLowerCase().includes(q)
    )
  })
})
</script>
