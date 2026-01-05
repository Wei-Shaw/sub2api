<template>
  <Teleport to="body">
    <div v-if="show && position" class="action-menu-content fixed z-[9999] w-52 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 dark:bg-dark-800" :style="{ top: position.top + 'px', left: position.left + 'px' REDACTED">
      <div class="py-1">
        <template v-if="account">
          <button @click="$emit('test', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100"><span class="text-green-500">▶</span> {{ t('admin.accounts.testConnection') REDACTEDREDACTED</button>
          <button @click="$emit('stats', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100"><span class="text-indigo-500">📊</span> {{ t('admin.accounts.viewStats') REDACTEDREDACTED</button>
          <template v-if="account.type === 'oauth' || account.type === 'setup-token'">
            <button @click="$emit('reauth', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 text-blue-600">🔗 {{ t('admin.accounts.reAuthorize') REDACTEDREDACTED</button>
            <button @click="$emit('refresh-token', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 text-purple-600">🔄 {{ t('admin.accounts.refreshToken') REDACTEDREDACTED</button>
          </template>
        </template>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
defineProps(['show', 'account', 'position']); defineEmits(['close', 'test', 'stats', 'reauth', 'refresh-token']); const { t REDACTED = useI18n()
</script>