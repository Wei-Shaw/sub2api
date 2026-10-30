<template>
  <div>
    <label :for="id" class="input-label">{{ t('admin.accounts.pools.importTarget') }}</label>
    <select :id="id" class="input" :value="modelValue ?? ''" :disabled="disabled || loading" @change="change">
      <option value="">{{ t('admin.accounts.pools.noPool') }}</option>
      <option v-for="pool in pools" :key="pool.id" :value="pool.id">{{ pool.name }}</option>
    </select>
    <p v-if="error" class="mt-1 text-xs text-red-600" role="alert">{{ error }}</p>
    <p v-else class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.pools.importHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { accountSchedulingPoolsAPI, type AccountSchedulingPool } from '@/api/admin/accountSchedulingPools'

withDefaults(defineProps<{ modelValue: number | null; id: string; disabled?: boolean }>(), { disabled: false })
const emit = defineEmits<{ (e: 'update:modelValue', value: number | null): void }>()
const { t } = useI18n()
const pools = ref<AccountSchedulingPool[]>([])
const loading = ref(false)
const error = ref('')
function change(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  emit('update:modelValue', value === '' ? null : Number(value))
}
onMounted(async () => {
  loading.value = true
  try { pools.value = await accountSchedulingPoolsAPI.list() }
  catch { error.value = t('admin.accounts.pools.loadFailed') }
  finally { loading.value = false }
})
</script>
