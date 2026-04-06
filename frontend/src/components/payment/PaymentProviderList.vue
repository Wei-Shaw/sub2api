<template>
  <div class="card">
    <!-- Header -->
    <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.payment.providerManagement') }}
          </h2>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.payment.providerManagementDesc') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <button
            @click="emit('refresh')"
            :disabled="loading"
            class="btn btn-secondary btn-sm"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button
            @click="emit('create')"
            :disabled="!canCreate"
            :class="canCreate
              ? 'btn btn-primary btn-sm'
              : 'btn btn-secondary btn-sm cursor-not-allowed opacity-50'"
          >
            {{ t('admin.settings.payment.createProvider') }}
          </button>
        </div>
      </div>
    </div>

    <!-- List -->
    <div class="p-4">
      <!-- Loading -->
      <div v-if="loading && !providers.length" class="flex items-center justify-center py-6">
        <div class="h-5 w-5 animate-spin rounded-full border-2 border-primary-500 border-t-transparent" />
      </div>

      <!-- Provider cards -->
      <div v-else-if="providers.length" class="space-y-3">
        <ProviderCard
          v-for="p in providers"
          :key="p.id"
          :provider="p"
          :enabled="isEnabled(p.provider_key)"
          :available-types="getTypes(p.provider_key)"
          @toggle-field="(field) => emit('toggleField', p, field)"
          @toggle-type="(type) => emit('toggleType', p, type)"
          @edit="emit('edit', p)"
          @delete="emit('delete', p)"
        />
      </div>

      <!-- Empty -->
      <div v-else class="py-6 text-center">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ canCreate
            ? t('admin.settings.payment.noProviders')
            : t('admin.settings.payment.enableTypesFirst') }}
        </p>
        <button
          v-if="canCreate"
          @click="emit('create')"
          class="btn btn-primary btn-sm mt-2"
        >
          {{ t('admin.settings.payment.createProvider') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ProviderCard from './ProviderCard.vue'
import type { ProviderInstance } from '@/types/payment'
import type { TypeOption } from './providerConfig'
import { getAvailableTypes, parseTypes } from './providerConfig'

const props = defineProps<{
  providers: ProviderInstance[]
  loading: boolean
  canCreate: boolean
  enabledPaymentTypes: string
  allPaymentTypes: TypeOption[]
  redirectLabel: string
}>()

const emit = defineEmits<{
  refresh: []
  create: []
  edit: [provider: ProviderInstance]
  delete: [provider: ProviderInstance]
  toggleField: [provider: ProviderInstance, field: 'enabled' | 'refundEnabled']
  toggleType: [provider: ProviderInstance, type: string]
}>()

const { t } = useI18n()

function isEnabled(providerKey: string): boolean {
  const enabled = parseTypes(props.enabledPaymentTypes)
  if (providerKey === 'easypay') {
    return enabled.includes('easypay') || enabled.includes('alipay') || enabled.includes('wxpay')
  }
  if (providerKey === 'alipay') return enabled.includes('alipay')
  if (providerKey === 'wxpay') return enabled.includes('wxpay')
  return enabled.includes(providerKey)
}

function getTypes(providerKey: string): TypeOption[] {
  return getAvailableTypes(providerKey, props.allPaymentTypes, props.redirectLabel)
}
</script>
