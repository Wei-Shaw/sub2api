<template>
  <div v-if="selectedIds.length > 0" class="mb-4 flex items-center justify-between p-3 bg-primary-50 rounded-lg">
    <span class="text-sm font-medium">{{ t('admin.accounts.bulkActions.selected', { count: selectedIds.length REDACTED) REDACTEDREDACTED</span>
    <div class="flex gap-2">
      <button @click="$emit('delete')" class="btn btn-danger btn-sm">{{ t('admin.accounts.bulkActions.delete') REDACTEDREDACTED</button>
      <button @click="$emit('edit')" class="btn btn-primary btn-sm">{{ t('admin.accounts.bulkActions.edit') REDACTEDREDACTED</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
defineProps(['selectedIds']); defineEmits(['delete', 'edit']); const { t REDACTED = useI18n()
</script>