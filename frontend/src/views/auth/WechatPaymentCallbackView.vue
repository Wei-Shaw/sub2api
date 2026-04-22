<template>
  <div class="min-h-screen bg-gray-50 px-4 py-10 dark:bg-dark-900">
    <div class="mx-auto max-w-2xl">
      <div class="card p-6">
        <h1 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ callbackTitleText REDACTEDREDACTED
        </h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ errorMessage || callbackProcessingText REDACTEDREDACTED
        </p>

        <div
          v-if="!errorMessage"
          class="mt-6 flex items-center justify-center py-10"
        >
          <div
            class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"
          ></div>
        </div>

        <div
          v-else
          class="mt-6 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/80"
        >
          <p class="text-sm text-gray-700 dark:text-gray-300">
            {{ errorMessage REDACTEDREDACTED
          </p>
          <button
            class="btn btn-primary mt-4"
            type="button"
            @click="goBackToPayment"
          >
            {{ backToPaymentText REDACTEDREDACTED
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useRoute, useRouter REDACTED from 'vue-router'
import { useAppStore REDACTED from '@/stores'

const { t REDACTED = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const errorMessage = ref('')

watch(errorMessage, (message) => {
  if (message) {
    appStore.showError(message)
  REDACTED
REDACTED)

const callbackProcessingText = computed(() => t('auth.wechatPayment.callbackProcessing'))
const callbackTitleText = computed(() => t('auth.wechatPayment.callbackTitle'))
const backToPaymentText = computed(() => t('auth.wechatPayment.backToPayment'))

function readQueryString(key: string): string {
  const value = route.query[key]
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  REDACTED
  return typeof value === 'string' ? value : ''
REDACTED

function parseFragmentParams(): URLSearchParams {
  const raw = typeof window !== 'undefined' ? window.location.hash : ''
  const hash = raw.startsWith('#') ? raw.slice(1) : raw
  return new URLSearchParams(hash)
REDACTED

function normalizeRedirectPath(path: string | null | undefined): string {
  const value = (path || '').trim()
  if (!value) return '/purchase'
  if (!value.startsWith('/')) return '/purchase'
  if (value.startsWith('//') || value.includes('://')) return '/purchase'
  if (value === '/payment') return '/purchase'
  if (value.startsWith('/payment?')) return '/purchase' + value.slice('/payment'.length)
  return value
REDACTED

function appendQueryParam(query: Record<string, string>, key: string, value: string) {
  if (value) {
    query[key] = value
  REDACTED
REDACTED

function goBackToPayment() {
  void router.replace('/purchase')
REDACTED

onMounted(async () => {
  const fragment = parseFragmentParams()
  const readParam = (key: string) => fragment.get(key) || readQueryString(key)

  const error = readParam('error') || readParam('err_msg') || readParam('errmsg')
  const errorDescription = readParam('error_description') || readParam('message')

  if (error) {
    errorMessage.value = errorDescription || error
    return
  REDACTED

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
  REDACTED

  const query: Record<string, string> = {
    ...Object.fromEntries(redirectURL.searchParams.entries()),
    wechat_resume: '1',
  REDACTED

  if (resumeToken) {
    query.wechat_resume_token = resumeToken
  REDACTED else {
    query.openid = openid
    appendQueryParam(query, 'state', state)
    appendQueryParam(query, 'scope', scope)
    appendQueryParam(query, 'payment_type', paymentType)
    appendQueryParam(query, 'amount', amount)
    appendQueryParam(query, 'order_type', orderType)
    appendQueryParam(query, 'plan_id', planId)
  REDACTED

  await router.replace({
    path: redirectURL.pathname,
    query,
  REDACTED)
REDACTED)
</script>
