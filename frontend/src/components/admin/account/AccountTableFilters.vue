<template>
  <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
    <div class="relative max-w-md flex-1"><input :value="searchQuery" type="text" :placeholder="t('admin.accounts.searchAccounts')" class="input" @input="$emit('update:searchQuery', ($event.target as HTMLInputElement).value)" /></div>
    <div class="flex gap-3">
      <Select v-model="filters.platform" :options="pOpts" @change="$emit('change')" />
      <Select v-model="filters.status" :options="sOpts" @change="$emit('change')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'; import { useI18n REDACTED from 'vue-i18n'; import Select from '@/components/common/Select.vue'
defineProps(['searchQuery', 'filters']); defineEmits(['update:searchQuery', 'change']); const { t REDACTED = useI18n()
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') REDACTED, { value: 'openai', label: 'OpenAI' REDACTED, { value: 'anthropic', label: 'Anthropic' REDACTED, { value: 'gemini', label: 'Gemini' REDACTED])
const sOpts = computed(() => [{ value: '', label: t('admin.accounts.allStatus') REDACTED, { value: 'active', label: t('admin.accounts.status.active') REDACTED, { value: 'error', label: t('admin.accounts.status.error') REDACTED])
</script>