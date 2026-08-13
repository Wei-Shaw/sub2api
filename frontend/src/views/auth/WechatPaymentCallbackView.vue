<template>
  <!--
    Not an OAuth login screen and not `AuthLayout`'s column — this is a payment
    resume hop. It shares the callback shell and nothing else.
  -->
  <div class="min-h-screen bg-canvas px-4 py-10">
    <div class="mx-auto max-w-2xl border border-line bg-surface p-6">
      <!--
        The failing state used to say the same sentence twice: once in the
        subtitle and again inside a tinted box below it. One statement, carrying
        its own tone, and the recovery action under a rule.

        The centred 32px ring spinner is gone with it. A callback that is still
        working now says so in words; the spinner rides along in the eyebrow as
        the redundant channel.
      -->
      <CallbackStatusPanel
        :status="errorMessage ? 'error' : 'working'"
        :status-label="errorMessage ? t('common.error') : t('common.processing')"
        :title="callbackTitleText"
        :description="errorMessage || callbackProcessingText"
      >
        <div v-if="errorMessage" class="border-t border-line pt-5">
          <Button
            type="button"
            tone="accent"
            variant="solid"
            size="md"
            @click="goBackToPayment"
          >
            {{ backToPaymentText }}
          </Button>
        </div>
      </CallbackStatusPanel>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
// By path, not through `components/common/index.ts`: the barrel re-exports
// LocaleSwitcher, which pulls `createI18n` into the graph and breaks the specs
// that mock `vue-i18n` with a partial factory.
import Button from '@/components/common/Button.vue'
import CallbackStatusPanel from '@/components/auth/CallbackStatusPanel.vue'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const errorMessage = ref('')

watch(errorMessage, (message) => {
  if (message) {
    appStore.showError(message)
  }
})

const callbackProcessingText = computed(() => t('auth.wechatPayment.callbackProcessing'))
const callbackTitleText = computed(() => t('auth.wechatPayment.callbackTitle'))
const backToPaymentText = computed(() => t('auth.wechatPayment.backToPayment'))

function readQueryString(key: string): string {
  const value = route.query[key]
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  }
  return typeof value === 'string' ? value : ''
}

function parseFragmentParams(): URLSearchParams {
  const raw = typeof window !== 'undefined' ? window.location.hash : ''
  const hash = raw.startsWith('#') ? raw.slice(1) : raw
  return new URLSearchParams(hash)
}

function normalizeRedirectPath(path: string | null | undefined): string {
  const value = (path || '').trim()
  if (!value) return '/purchase'
  if (!value.startsWith('/')) return '/purchase'
  if (value.startsWith('//') || value.includes('://')) return '/purchase'
  if (value === '/payment') return '/purchase'
  if (value.startsWith('/payment?')) return '/purchase' + value.slice('/payment'.length)
  return value
}

function appendQueryParam(query: Record<string, string>, key: string, value: string) {
  if (value) {
    query[key] = value
  }
}

function goBackToPayment() {
  void router.replace('/purchase')
}

onMounted(async () => {
  const fragment = parseFragmentParams()
  const readParam = (key: string) => fragment.get(key) || readQueryString(key)

  const error = readParam('error') || readParam('err_msg') || readParam('errmsg')
  const errorDescription = readParam('error_description') || readParam('message')

  if (error) {
    errorMessage.value = errorDescription || error
    return
  }

  const resumeToken = readParam('wechat_resume_token')
  const openid = readParam('openid')
  const state = readParam('state')
  const scope = readParam('scope')
  const paymentType = readParam('payment_type')
  const amount = readParam('amount')
  const orderType = readParam('order_type')
  const planId = readParam('plan_id')
  const redirectURL = new URL(
    normalizeRedirectPath(readParam('redirect')),
    window.location.origin,
  )

  if (!resumeToken && !openid) {
    errorMessage.value = t('auth.wechatPayment.callbackMissingResumeToken')
    return
  }

  const query: Record<string, string> = {
    ...Object.fromEntries(redirectURL.searchParams.entries()),
    wechat_resume: '1',
  }

  if (resumeToken) {
    query.wechat_resume_token = resumeToken
  } else {
    query.openid = openid
    appendQueryParam(query, 'state', state)
    appendQueryParam(query, 'scope', scope)
    appendQueryParam(query, 'payment_type', paymentType)
    appendQueryParam(query, 'amount', amount)
    appendQueryParam(query, 'order_type', orderType)
    appendQueryParam(query, 'plan_id', planId)
  }

  await router.replace({
    path: redirectURL.pathname,
    query,
  })
})
</script>
