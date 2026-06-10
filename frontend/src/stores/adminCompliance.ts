import { defineStore REDACTED from 'pinia'
import { computed, ref REDACTED from 'vue'
import adminComplianceAPI, { type AdminComplianceStatus REDACTED from '@/api/admin/compliance'
import { getLocale REDACTED from '@/i18n'

const FALLBACK_ZH_PHRASE = '我已阅读、理解并同意 Sub2API 部署与运营合规承诺'
const FALLBACK_EN_PHRASE = 'I have read, understood, and agree to the Sub2API Deployment and Operation Compliance Commitment'

export const useAdminComplianceStore = defineStore('adminCompliance', () => {
  const status = ref<AdminComplianceStatus | null>(null)
  const loading = ref(false)
  const submitting = ref(false)
  const initialized = ref(false)
  const forceVisible = ref(false)

  const required = computed(() => status.value?.required === true)
  const shouldShow = computed(() => required.value || forceVisible.value)
  const currentLocale = computed(() => getLocale())
  const expectedPhrase = computed(() => {
    if (currentLocale.value === 'zh') {
      return status.value?.ack_phrase_zh || FALLBACK_ZH_PHRASE
    REDACTED
    return status.value?.ack_phrase_en || FALLBACK_EN_PHRASE
  REDACTED)

  async function fetchStatus(): Promise<AdminComplianceStatus> {
    loading.value = true
    try {
      const nextStatus = await adminComplianceAPI.getStatus()
      status.value = nextStatus
      initialized.value = true
      forceVisible.value = nextStatus.required
      return nextStatus
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  async function accept(phrase: string): Promise<AdminComplianceStatus> {
    submitting.value = true
    try {
      const nextStatus = await adminComplianceAPI.accept({
        phrase,
        language: currentLocale.value
      REDACTED)
      status.value = nextStatus
      forceVisible.value = nextStatus.required
      return nextStatus
    REDACTED finally {
      submitting.value = false
    REDACTED
  REDACTED

  function requireAcknowledgement(partialStatus?: Partial<AdminComplianceStatus>): void {
    status.value = {
      required: true,
      version: partialStatus?.version || status.value?.version || 'v2026.06.10',
      document_path_zh: partialStatus?.document_path_zh || status.value?.document_path_zh || 'docs/legal/admin-compliance.zh.md',
      document_path_en: partialStatus?.document_path_en || status.value?.document_path_en || 'docs/legal/admin-compliance.en.md',
      document_url_zh: partialStatus?.document_url_zh || status.value?.document_url_zh || 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/legal/admin-compliance.zh.md',
      document_url_en: partialStatus?.document_url_en || status.value?.document_url_en || 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/legal/admin-compliance.en.md',
      ack_phrase_zh: partialStatus?.ack_phrase_zh || status.value?.ack_phrase_zh || FALLBACK_ZH_PHRASE,
      ack_phrase_en: partialStatus?.ack_phrase_en || status.value?.ack_phrase_en || FALLBACK_EN_PHRASE,
      acknowledgement: status.value?.acknowledgement
    REDACTED
    initialized.value = true
    forceVisible.value = true
  REDACTED

  function reset(): void {
    status.value = null
    loading.value = false
    submitting.value = false
    initialized.value = false
    forceVisible.value = false
  REDACTED

  return {
    status,
    loading,
    submitting,
    initialized,
    required,
    shouldShow,
    expectedPhrase,
    fetchStatus,
    accept,
    requireAcknowledgement,
    reset
  REDACTED
REDACTED)
