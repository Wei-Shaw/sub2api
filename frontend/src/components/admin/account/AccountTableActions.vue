<template>
  <div class="flex flex-wrap items-center gap-3">
    <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary">
      <Icon name="refresh" size="md" :class="[loading ? 'animate-spin' : '']" />
    </button>
    <button @click="$emit('sync')" class="btn btn-secondary">{{ t('admin.accounts.syncFromCrs') REDACTEDREDACTED</button>
    <button @click="$emit('create')" class="btn btn-primary">{{ t('admin.accounts.createAccount') REDACTEDREDACTED</button>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps(['loading'])
defineEmits(['refresh', 'sync', 'create'])

const { t REDACTED = useI18n()
</script>
