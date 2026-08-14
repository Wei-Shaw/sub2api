<template>
  <div class="space-y-4">
    <Button variant="outline" size="md" block :disabled="disabled" @click="startLogin">
      <template #icon>
        <!--
          Squared and neutral. The accent is reserved for interaction and
          selection in this system, so it cannot also brand an identity
          provider whose real colours we do not know.
        -->
        <span
          class="inline-flex h-4 w-4 items-center justify-center border border-line font-mono text-2xs text-ink-secondary"
          aria-hidden="true"
        >
          {{ providerInitial }}
        </span>
      </template>
      {{ t('auth.oidc.signIn', { providerName: normalizedProviderName }) }}
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
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import type { OAuthLoginStart } from '@/api/auth'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'

const props = withDefaults(defineProps<{
  disabled?: boolean
  affCode?: string
  providerName?: string
  showDivider?: boolean
}>(), {
  providerName: 'OIDC',
  showDivider: true
})
const emit = defineEmits<{
  start: [request: OAuthLoginStart]
}>()

const route = useRoute()
const { t } = useI18n()

const normalizedProviderName = computed(() => {
  const name = props.providerName?.trim()
  return name || 'OIDC'
})

const providerInitial = computed(() => normalizedProviderName.value.charAt(0).toUpperCase() || 'O')

function startLogin(): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  storeOAuthAffiliateCode(resolveAffiliateReferralCode(props.affCode, route.query.aff, route.query.aff_code))
  emit('start', { provider: 'oidc', params: { redirect: redirectTo } })
}
</script>
