<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
    <button type="button" @click="emit('update:expanded', !expanded)" class="flex w-full items-center justify-between">
      <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('payment.adminSettings.limitsTitle') }}
      </h4>
      <svg :class="['h-4 w-4 text-gray-400 transition-transform', expanded && 'rotate-180']" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" /></svg>
    </button>
    <div v-show="expanded" class="mt-3 space-y-3">
      <div
        v-for="lt in limitableTypes"
        :key="lt.value"
        class="rounded-lg border border-gray-100 p-3 dark:border-dark-700"
      >
        <p class="mb-2 text-xs font-medium text-gray-700 dark:text-gray-300">{{ lt.label }}</p>
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.adminSettings.limitSingleMin') }}</label>
            <input
              type="number"
              :value="getLimitVal(lt.value, 'singleMin')"
              @input="setLimitVal(lt.value, 'singleMin', ($event.target as HTMLInputElement).value)"
              class="input mt-0.5" min="1" step="0.01" :placeholder="limitPlaceholder(lt.value)"
            />
          </div>
          <div>
            <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.adminSettings.limitSingleMax') }}</label>
            <input
              type="number"
              :value="getLimitVal(lt.value, 'singleMax')"
              @input="setLimitVal(lt.value, 'singleMax', ($event.target as HTMLInputElement).value)"
              class="input mt-0.5" min="1" step="0.01" :placeholder="limitPlaceholder(lt.value)"
            />
          </div>
          <div>
            <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.adminSettings.limitDaily') }}</label>
            <input
              type="number"
              :value="getLimitVal(lt.value, 'dailyLimit')"
              @input="setLimitVal(lt.value, 'dailyLimit', ($event.target as HTMLInputElement).value)"
              class="input mt-0.5" min="1" step="0.01" :placeholder="limitPlaceholder(lt.value)"
            />
          </div>
        </div>
      </div>
      <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.adminSettings.limitsHint') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{
  limitableTypes: Array<{ value: string; label: string }>
  expanded: boolean
  getLimitVal: (paymentType: string, field: string) => string
  limitPlaceholder: (paymentType: string) => string
  setLimitVal: (paymentType: string, field: string, val: string) => void
}>()

const emit = defineEmits<{
  'update:expanded': [val: boolean]
}>()
</script>
