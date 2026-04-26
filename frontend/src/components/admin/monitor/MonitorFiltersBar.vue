<template>
  <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
    <!-- Left: Search + Filters -->
    <div class="flex flex-1 flex-wrap items-center gap-3">
      <div class="relative w-full sm:w-64">
        <Icon
          name="search"
          size="md"
          class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
        />
        <input
          v-model="search"
          type="text"
          :placeholder="t('admin.channelMonitor.searchPlaceholder')"
          class="input pl-10"
          @input="$emit('search-input')"
        />
      </div>

      <Select
        v-model="provider"
        :options="providerFilterOptions"
        :placeholder="t('admin.channelMonitor.allProviders')"
        class="w-44"
        @change="$emit('reload')"
      />

      <Select
        v-model="enabled"
        :options="enabledFilterOptions"
        :placeholder="t('admin.channelMonitor.enabledFilter')"
        class="w-40"
        @change="$emit('reload')"
      />
    </div>

    <!-- Right: Actions -->
    <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
      <button
        @click="$emit('reload')"
        :disabled="loading"
        class="btn btn-secondary"
        :title="t('common.refresh')"
      >
        <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
      </button>
      <button
        @click="$emit('manage-templates')"
        class="btn btn-secondary"
        :title="t('admin.channelMonitor.template.manageButton')"
      >
        <Icon name="cog" size="md" class="mr-2" />
        {{ t('admin.channelMonitor.template.manageButton') }}
      </button>
      <button @click="$emit('create')" class="btn btn-primary">
        <Icon name="plus" size="md" class="mr-2" />
        {{ t('admin.channelMonitor.createButton') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Provider } from '@/api/admin/channelMonitor'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
} from '@/constants/channelMonitor'

defineProps<{
  loading: boolean
}>()

defineEmits<{
  (e: 'reload'): void
  (e: 'create'): void
  (e: 'manage-templates'): void
  (e: 'search-input'): void
}>()

const search = defineModel<string>('search', { required: true })
const provider = defineModel<Provider | ''>('provider', { required: true })
const enabled = defineModel<'' | 'true' | 'false'>('enabled', { required: true })

const { t } = useI18n()

const providerFilterOptions = computed(() => [
  { value: '', label: t('admin.channelMonitor.allProviders') },
  { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') },
  { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') },
  { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') },
])

const enabledFilterOptions = computed(() => [
  { value: '', label: t('admin.channelMonitor.allStatus') },
  { value: 'true', label: t('admin.channelMonitor.onlyEnabled') },
  { value: 'false', label: t('admin.channelMonitor.onlyDisabled') },
])
</script>
