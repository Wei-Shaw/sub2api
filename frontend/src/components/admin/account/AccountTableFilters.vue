<template>
  <div class="flex flex-wrap items-center gap-3">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.accounts.searchAccounts')"
      class="w-full sm:w-64"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />
    <Select :model-value="filters.platform" class="w-40" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
    <Select :model-value="filters.type" class="w-40" :options="tOpts" @update:model-value="updateType" @change="$emit('change')" />
    <Select :model-value="filters.status" class="w-40" :options="sOpts" @update:model-value="updateStatus" @change="$emit('change')" />
    <Select :model-value="filters.privacy_mode" class="w-40" :options="privacyOpts" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
    <Select :model-value="filters.group" class="w-40" :options="gOpts" @update:model-value="updateGroup" @change="$emit('change')" />
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'; import { useI18n REDACTED from 'vue-i18n'; import Select from '@/components/common/Select.vue'; import SearchInput from '@/components/common/SearchInput.vue'
import type { AdminGroup REDACTED from '@/types'
const props = defineProps<{ searchQuery: string; filters: Record<string, any>; groups?: AdminGroup[] REDACTED>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change']); const { t REDACTED = useI18n()
const updatePlatform = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, platform: value REDACTED) REDACTED
const updateType = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, type: value REDACTED) REDACTED
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value REDACTED) REDACTED
const updatePrivacyMode = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, privacy_mode: value REDACTED) REDACTED
const updateGroup = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, group: value REDACTED) REDACTED
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') REDACTED, { value: 'anthropic', label: 'Anthropic' REDACTED, { value: 'openai', label: 'OpenAI' REDACTED, { value: 'gemini', label: 'Gemini' REDACTED, { value: 'antigravity', label: 'Antigravity' REDACTED, { value: 'grok', label: 'Grok' REDACTED])
const tOpts = computed(() => [{ value: '', label: t('admin.accounts.allTypes') REDACTED, { value: 'oauth', label: t('admin.accounts.oauthType') REDACTED, { value: 'setup-token', label: t('admin.accounts.setupToken') REDACTED, { value: 'apikey', label: t('admin.accounts.apiKey') REDACTED, { value: 'bedrock', label: 'AWS Bedrock' REDACTED])
const sOpts = computed(() => [{ value: '', label: t('admin.accounts.allStatus') REDACTED, { value: 'active', label: t('admin.accounts.status.active') REDACTED, { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED, { value: 'error', label: t('admin.accounts.status.error') REDACTED, { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') REDACTED, { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') REDACTED, { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') REDACTED])
const privacyOpts = computed(() => [
  { value: '', label: t('admin.accounts.allPrivacyModes') REDACTED,
  { value: '__unset__', label: t('admin.accounts.privacyUnset') REDACTED,
  { value: 'training_off', label: 'Privacy' REDACTED,
  { value: 'training_set_cf_blocked', label: 'CF' REDACTED,
  { value: 'training_set_failed', label: 'Fail' REDACTED
])
const gOpts = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') REDACTED,
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') REDACTED,
  ...(props.groups || []).map(g => ({ value: String(g.id), label: g.name REDACTED))
])
</script>
