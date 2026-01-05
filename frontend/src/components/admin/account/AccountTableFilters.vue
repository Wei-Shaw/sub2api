<template>
  <div class="flex flex-wrap items-center gap-3">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.accounts.searchAccounts')"
      class="w-64"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />
    <Select v-model="filters.platform" class="w-40" :options="pOpts" @change="$emit('change')" />
    <Select v-model="filters.type" class="w-40" :options="tOpts" @change="$emit('change')" />
    <Select v-model="filters.status" class="w-40" :options="sOpts" @change="$emit('change')" />
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'; import { useI18n REDACTED from 'vue-i18n'; import Select from '@/components/common/Select.vue'; import SearchInput from '@/components/common/SearchInput.vue'
defineProps(['searchQuery', 'filters']); defineEmits(['update:searchQuery', 'change']); const { t REDACTED = useI18n()
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') REDACTED, { value: 'openai', label: 'OpenAI' REDACTED, { value: 'anthropic', label: 'Anthropic' REDACTED, { value: 'gemini', label: 'Gemini' REDACTED, { value: 'antigravity', label: 'Antigravity' REDACTED])
const tOpts = computed(() => [{ value: '', label: t('admin.accounts.allTypes') REDACTED, { value: 'oauth', label: t('admin.accounts.oauthType') REDACTED, { value: 'setup-token', label: t('admin.accounts.setupToken') REDACTED, { value: 'apikey', label: t('admin.accounts.apiKey') REDACTED])
const sOpts = computed(() => [{ value: '', label: t('admin.accounts.allStatus') REDACTED, { value: 'active', label: t('admin.accounts.status.active') REDACTED, { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED, { value: 'error', label: t('admin.accounts.status.error') REDACTED])
</script>
