<template>
  <div class="space-y-4">
    <Button variant="outline" size="md" block :disabled="disabled" @click="startLogin">
      <template #icon>
        <!-- Squared, like every other mark in the system. The blue is DingTalk's. -->
        <svg
          class="h-4 w-4 shrink-0"
          viewBox="0 0 24 24"
          xmlns="http://www.w3.org/2000/svg"
          aria-hidden="true"
        >
          <rect width="24" height="24" fill="#1677FF" />
          <text
            x="12"
            y="17"
            font-family="sans-serif"
            font-size="14"
            font-weight="bold"
            fill="white"
            text-anchor="middle"
          >D</text>
        </svg>
      </template>
      {{ t('auth.dingtalk.signIn') }}
    </Button>

    <div v-if="showDivider" class="flex items-center gap-3">
      <span class="h-px flex-1 bg-line" aria-hidden="true"></span>
      <span class="text-2xs uppercase tracking-[0.08em] text-ink-tertiary">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <span class="h-px flex-1 bg-line" aria-hidden="true"></span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
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
const { t } = useI18n()

function startLogin(): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  storeOAuthAffiliateCode(resolveAffiliateReferralCode(props.affCode, route.query.aff, route.query.aff_code))
  emit('start', { provider: 'dingtalk', params: { redirect: redirectTo } })
}
</script>
