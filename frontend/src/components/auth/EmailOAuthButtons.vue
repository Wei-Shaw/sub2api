<template>
  <div v-if="hasProviders" class="space-y-4">
    <div v-if="showDivider" class="flex items-center gap-3">
      <span class="h-px flex-1 bg-line" aria-hidden="true"></span>
      <span class="text-2xs uppercase tracking-[0.08em] text-ink-tertiary">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <span class="h-px flex-1 bg-line" aria-hidden="true"></span>
    </div>

    <div :class="providerGridClass">
      <!--
        These were 48px tall — 12px taller than every other control on the page,
        including the primary action they sit under. A federated sign-in is not
        more important than signing in.
      -->
      <Button
        v-for="provider in visibleProviders"
        :key="provider"
        variant="outline"
        size="md"
        block
        :disabled="disabled"
        @click="startLogin(provider)"
      >
        <template #icon>
          <GitHubMark v-if="provider === 'github'" class="h-4 w-4 text-ink" />
          <GoogleMark v-else class="h-4 w-4" />
        </template>
        {{ providerLabel(provider) }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import GitHubMark from './GitHubMark.vue'
import GoogleMark from './GoogleMark.vue'
import Button from '@/components/common/Button.vue'
import type { OAuthLoginStart } from '@/api/auth'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'

type EmailOAuthProvider = 'github' | 'google'
const EMAIL_OAUTH_PENDING_PROVIDER_KEY = 'email_oauth_pending_provider'

const props = withDefaults(defineProps<{
  disabled?: boolean
  affCode?: string
  githubEnabled?: boolean
  googleEnabled?: boolean
  showDivider?: boolean
}>(), {
  showDivider: true
})
const emit = defineEmits<{
  start: [request: OAuthLoginStart]
}>()

const route = useRoute()
const { t } = useI18n()

const visibleProviders = computed<EmailOAuthProvider[]>(() => {
  const providers: EmailOAuthProvider[] = []
  if (props.githubEnabled) providers.push('github')
  if (props.googleEnabled) providers.push('google')
  return providers
})

const hasProviders = computed(() => visibleProviders.value.length > 0)
const hasMultipleProviders = computed(() => visibleProviders.value.length > 1)
const providerGridClass = computed(() => [
  'grid',
  'grid-cols-1',
  'gap-3',
  hasMultipleProviders.value ? 'sm:grid-cols-2' : ''
])

function providerLabel(provider: EmailOAuthProvider): string {
  const name = provider === 'github' ? 'GitHub' : 'Google'
  return hasMultipleProviders.value ? name : t('auth.emailOAuth.signIn', { providerName: name })
}

function startLogin(provider: EmailOAuthProvider): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  const affiliateCode = resolveAffiliateReferralCode(props.affCode, route.query.aff, route.query.aff_code)
  storeOAuthAffiliateCode(affiliateCode)
  window.sessionStorage.setItem(EMAIL_OAUTH_PENDING_PROVIDER_KEY, provider)
  const params: Record<string, string> = { redirect: redirectTo }
  if (affiliateCode) {
    params.aff_code = affiliateCode
  }
  emit('start', { provider, params })
}
</script>
