<template>
  <div class="flex flex-wrap items-start gap-3">
    <div class="min-w-0 flex-1">
      <SearchInput
        :model-value="searchQuery"
        :placeholder="t('admin.accounts.searchAccounts')"
        @update:model-value="$emit('update:searchQuery', $event)"
        @search="$emit('change')"
      />
    </div>
    <div class="flex flex-wrap items-center gap-3">
      <Select :model-value="filters.platform" class="w-40 flex-shrink-0" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
      <Select :model-value="filters.status" class="w-40 flex-shrink-0" :options="sOpts" @update:model-value="updateStatus" @change="$emit('change')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'; import { useI18n REDACTED from 'vue-i18n'; import Select from '@/components/common/Select.vue'; import SearchInput from '@/components/common/SearchInput.vue'
const props = defineProps(['searchQuery', 'filters']); const emit = defineEmits(['update:searchQuery', 'update:filters', 'change']); const { t REDACTED = useI18n()
const updatePlatform = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, platform: value REDACTED) REDACTED
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value REDACTED) REDACTED
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') REDACTED, { value: 'openai', label: 'OpenAI' REDACTED, { value: 'anthropic', label: 'Anthropic' REDACTED, { value: 'gemini', label: 'Gemini' REDACTED])
const sOpts = computed(() => [{ value: '', label: t('admin.accounts.allStatus') REDACTED, { value: 'active', label: t('admin.accounts.status.active') REDACTED, { value: 'error', label: t('admin.accounts.status.error') REDACTED])
</script>
