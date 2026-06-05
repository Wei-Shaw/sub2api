<template>
  <div class="space-y-5">
    <div>
      <label class="input-label">
        {{ t('admin.channels.form.name', 'Name') }}
        <span class="input-required">*</span>
      </label>
      <input
        :value="form.name"
        @input="emit('update-field', 'name', ($event.target as HTMLInputElement).value)"
        type="text"
        required
        class="input"
        :placeholder="t('admin.channels.form.namePlaceholder', 'Enter channel name')"
      />
    </div>

    <div>
      <label class="input-label">{{ t('admin.channels.form.description', 'Description') }}</label>
      <textarea
        :value="form.description"
        @input="emit('update-field', 'description', ($event.target as HTMLTextAreaElement).value)"
        rows="2"
        class="input"
        :placeholder="t('admin.channels.form.descriptionPlaceholder', 'Optional description')"
      ></textarea>
    </div>

    <div v-if="isEdit">
      <label class="input-label">{{ t('admin.channels.form.status', 'Status') }}</label>
      <Select
        :modelValue="form.status"
        :options="statusEditOptions"
        @update:modelValue="emit('update-field', 'status', String($event))"
      />
    </div>

    <div>
      <label class="flex items-center gap-2 cursor-pointer">
        <input
          type="checkbox"
          :checked="form.restrict_models"
          @change="emit('update-field', 'restrict_models', ($event.target as HTMLInputElement).checked)"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        />
        <span class="input-label mb-0">{{ t('admin.channels.form.restrictModels', 'Restrict Models') }}</span>
      </label>
      <p class="mt-1 ml-6 text-xs text-gray-400">
        {{ t('admin.channels.form.restrictModelsHint', 'When enabled, only models in the pricing list are allowed. Others will be rejected.') }}
      </p>
    </div>

    <div>
      <label class="input-label">{{ t('admin.channels.form.billingModelSource', 'Billing Basis') }}</label>
      <Select
        :modelValue="form.billing_model_source"
        :options="billingModelSourceOptions"
        @update:modelValue="emit('update-field', 'billing_model_source', String($event))"
      />
      <p class="mt-1 text-xs text-gray-400">
        {{ t('admin.channels.form.billingModelSourceHint', 'Controls which model name is used for pricing lookup') }}
      </p>
    </div>

    <div class="space-y-3">
      <label class="input-label mb-0">{{ t('admin.channels.form.platformConfig', 'Platform Configuration') }}</label>
      <div class="flex flex-wrap gap-2">
        <label
          v-for="p in PLATFORM_ORDER"
          :key="p"
          class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors"
          :class="isPlatformActive(p)
            ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700'
            : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
        >
          <input
            type="checkbox"
            :checked="isPlatformActive(p)"
            class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            @change="emit('toggle-platform', p)"
          />
          <PlatformIcon :platform="p" size="xs" :class="platformTextClass(p)" />
          <span :class="platformTextClass(p)">{{ t('admin.groups.platforms.' + p, p) }}</span>
        </label>
      </div>
    </div>

    <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.channels.form.applyPricingToAccountStats') }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.applyPricingToAccountStatsDesc') }}
          </p>
        </div>
        <Toggle
          :modelValue="form.apply_pricing_to_account_stats"
          @update:modelValue="emit('update-field', 'apply_pricing_to_account_stats', $event)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * ChannelBasicSettings — Channel 编辑对话框 "基础设置" tab (T9 拆出).
 * 通过 update-field event 把单字段变更回传给父对话框 (ChannelDialog),
 * 避免双向绑定整个 form (封装原则).
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { PlatformIcon, Select, Toggle } from '@sub2api/plugin-sdk'
import { PLATFORM_ORDER, type ChannelFormState, type GroupPlatform } from './types'
import { platformTextClass } from '@sub2api/plugin-sdk'

const props = defineProps<{
  form: ChannelFormState
  isEdit: boolean
}>()

const emit = defineEmits<{
  (e: 'update-field', key: keyof ChannelFormState, value: unknown): void
  (e: 'toggle-platform', platform: GroupPlatform): void
}>()

const { t } = useI18n()

const statusEditOptions = computed(() => [
  { value: 'active', label: t('admin.channels.statusActive', 'Active') },
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') },
])

const billingModelSourceOptions = computed(() => [
  { value: 'channel_mapped', label: t('admin.channels.form.billingModelSourceChannelMapped', 'Bill by channel-mapped model') },
  { value: 'requested', label: t('admin.channels.form.billingModelSourceRequested', 'Bill by requested model') },
  { value: 'upstream', label: t('admin.channels.form.billingModelSourceUpstream', 'Bill by final upstream model') },
])

function isPlatformActive(p: GroupPlatform): boolean {
  return props.form.platforms.some((s) => s.enabled && s.platform === p)
}
</script>
