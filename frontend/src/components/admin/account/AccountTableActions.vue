<template>
  <div class="flex justify-end gap-3">
    <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary"><svg :class="['h-5 w-5', loading ? 'animate-spin' : '']" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" /></svg></button>
    <button @click="$emit('sync')" class="btn btn-secondary">{{ t('admin.accounts.syncFromCrs') REDACTEDREDACTED</button>
    <button @click="$emit('create')" class="btn btn-primary">{{ t('admin.accounts.createAccount') REDACTEDREDACTED</button>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'; defineProps(['loading']); defineEmits(['refresh', 'sync', 'create']); const { t REDACTED = useI18n()
</script>