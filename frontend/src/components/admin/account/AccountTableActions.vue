<template>
  <div class="account-table-actions flex flex-shrink-0 flex-nowrap items-center gap-1.5">
    <slot name="before"></slot>
    <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary h-8 w-8 justify-center px-0" :title="t('common.refresh')">
      <Icon name="refresh" size="sm" :class="[loading ? 'animate-spin' : '']" />
    </button>
    <slot name="after"></slot>
    <slot name="beforeCreate"></slot>
    <button @click="$emit('create')" class="btn btn-primary h-8 whitespace-nowrap px-3 text-sm">{{ t('admin.accounts.createAccount') }}</button>
    <slot name="afterCreate"></slot>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps(['loading'])
defineEmits(['refresh', 'create'])

const { t } = useI18n()
</script>

<style scoped>
.account-table-actions :deep(.btn) {
  min-height: 2rem;
  padding-top: 0.25rem;
  padding-bottom: 0.25rem;
}
</style>
