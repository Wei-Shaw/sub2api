<template>
  <div class="space-y-4">
    <label v-if="apps.length > 1" class="block text-sm">
      {{ locale.startsWith('zh') ? '选择钉钉应用' : 'DingTalk application' }}
      <select v-model="selectedApp" class="input mt-1" :disabled="disabled"><option v-for="app in apps" :key="app.id" :value="app.id">{{ app.name }}</option></select>
    </label>
    <button type="button" :disabled="disabled || appsLoading" class="btn btn-secondary w-full" @click="startLogin">
      <svg
        class="icon mr-2"
        viewBox="0 0 24 24"
        xmlns="http://www.w3.org/2000/svg"
        width="20"
        height="20"
        aria-hidden="true"
        style="flex-shrink: 0"
      >
        <circle cx="12" cy="12" r="12" fill="#1677FF" />
        <text
          x="12"
          y="17"
          font-family="sans-serif"
          font-size="13"
          font-weight="bold"
          fill="white"
          text-anchor="middle"
        >D</text>
      </svg>
      {{ t('auth.dingtalk.signIn') }}
    </button>

    <div v-if="showDivider" class="flex items-center gap-3">
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
      <span class="text-xs text-gray-500 dark:text-dark-400">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { publicDingTalkApps } from '@/api/dingtalk'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { OAuthLoginStart } from '@/api/auth'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'

const props = withDefaults(defineProps<{
  disabled?: boolean
  affCode?: string
  showDivider?: boolean
}>(), {
  showDivider: true
})
const emit = defineEmits<{
  start: [request: OAuthLoginStart]
}>()

const route = useRoute()
const { t, locale } = useI18n()
const apps = ref<{ id: string; name: string }[]>([])
const selectedApp = ref('')
const appsLoading = ref(true)
onMounted(async () => {
  try { apps.value = await publicDingTalkApps(); selectedApp.value = apps.value[0]?.id || '' } catch { /* Keep the legacy default login available. */ } finally { appsLoading.value = false }
})

function startLogin(): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  storeOAuthAffiliateCode(resolveAffiliateReferralCode(props.affCode, route.query.aff, route.query.aff_code))
  const params: Record<string, string> = { redirect: redirectTo }
  if (selectedApp.value && selectedApp.value !== 'default') params.app_id = selectedApp.value
  emit('start', { provider: 'dingtalk', params })
}
</script>
