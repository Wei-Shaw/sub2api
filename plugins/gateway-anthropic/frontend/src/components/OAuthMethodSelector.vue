<template>
  <div class="space-y-3">
    <label class="input-label">{{ t('admin.accounts.addMethod') }}</label>
    <div class="flex gap-4">
      <label class="flex cursor-pointer items-center">
        <input :checked="addMethod === 'oauth'" type="radio" value="oauth"
          class="mr-2 text-primary-600 focus:ring-primary-500"
          @change="$emit('update:addMethod', 'oauth')" />
        <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.types.oauth') }}</span>
      </label>
      <label class="flex cursor-pointer items-center">
        <input :checked="addMethod === 'setup-token'" type="radio" value="setup-token"
          class="mr-2 text-primary-600 focus:ring-primary-500"
          @change="$emit('update:addMethod', 'setup-token')" />
        <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.setupTokenLongLived') }}</span>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{ addMethod: string }>()
defineEmits<{ 'update:addMethod': [value: string] }>()
</script>
