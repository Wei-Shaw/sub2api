<template>
  <div class="card p-6">
    <h2 class="font-semibold mb-4">{{ t('dashboard.quickActions') REDACTEDREDACTED</h2>
    <div class="space-y-2">
      <button @click="router.push('/keys')" class="w-full text-left p-3 hover:bg-gray-100 rounded-lg flex items-center gap-3"><div class="p-2 bg-blue-100 rounded-lg text-blue-600">🔑</div><span>{{ t('dashboard.createApiKey') REDACTEDREDACTED</span></button>
      <button @click="router.push('/usage')" class="w-full text-left p-3 hover:bg-gray-100 rounded-lg flex items-center gap-3"><div class="p-2 bg-green-100 rounded-lg text-green-600">📊</div><span>{{ t('dashboard.viewUsage') REDACTEDREDACTED</span></button>
      <button @click="router.push('/redeem')" class="w-full text-left p-3 hover:bg-gray-100 rounded-lg flex items-center gap-3"><div class="p-2 bg-amber-100 rounded-lg text-amber-600">🎁</div><span>{{ t('dashboard.redeemCode') REDACTEDREDACTED</span></button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter REDACTED from 'vue-router'; import { useI18n REDACTED from 'vue-i18n'
const router = useRouter(); const { t REDACTED = useI18n()
</script>