<template>
  <div class="space-y-4">
    <button type="button" :disabled="disabled" class="flex min-h-11 w-full items-center gap-3 border-y border-border bg-surface px-3 py-2.5 text-left text-body font-semibold text-fg transition-colors hover:bg-accent-weak hover:text-accent-strong focus:outline-none focus-visible:bg-accent-weak disabled:cursor-not-allowed disabled:opacity-50" @click="startLogin">
      <svg class="shrink-0" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
        <circle cx="12" cy="12" r="12" fill="#229ED9" />
        <path
          d="M5.4 11.8 16.9 7.4c.5-.2 1 .1.8.9l-2 9.3c-.1.6-.5.8-1.1.5l-3-2.2-1.4 1.4c-.2.2-.3.3-.6.3l.2-3.1 5.6-5c.2-.2 0-.3-.4-.1l-6.9 4.3-3-.9c-.6-.2-.7-.6.3-1Z"
          fill="#fff"
        />
      </svg>
      {{ t('auth.telegram.signIn') }}
    </button>

    <div v-if="showDivider" class="flex items-center gap-3">
      <div class="h-px flex-1 bg-border"></div>
      <span class="text-meta text-fg-muted">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <div class="h-px flex-1 bg-border"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { OAuthLoginStart } from '@/api/auth'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'

const props = withDefaults(defineProps<{
  disabled?: boolean
  affCode?: string
  promoCode?: string
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
  const params: Record<string, string> = { redirect: redirectTo }
  const promoCode = props.promoCode?.trim()
  if (promoCode) {
    params.promo_code = promoCode
  }
  emit('start', { provider: 'telegram', params })
}
</script>
