import { computed, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { getPublicSettings } from '@/api/auth'
import type { PublicSettings } from '@/types'

type ReviewSettings = Pick<PublicSettings, 'registration_review_instructions' | 'registration_reason_max_length'>

export function useRegistrationReview() {
  const appStore = useAppStore()
  const { t } = useI18n()
  const registrationReviewSettings = shallowRef<Partial<ReviewSettings> | null>(null)
  const settings = computed(() => registrationReviewSettings.value ?? appStore.cachedPublicSettings)
  const registrationReviewInstructions = computed(() => settings.value?.registration_review_instructions?.trim() || '')
  const registrationReasonMaxLength = computed(() => {
    const limit = settings.value?.registration_reason_max_length
    return typeof limit === 'number' && Number.isInteger(limit) && limit >= 1 && limit <= 10000 ? limit : 500
  })
  async function loadRegistrationReviewSettings() {
    if (registrationReviewSettings.value) return
    try {
      registrationReviewSettings.value = await getPublicSettings()
    } catch {
      // Keep the default prompt; the backend remains authoritative for validation.
    }
  }
  function registrationReasonError(reason: string): string {
    if (!reason.trim()) return t('auth.registrationReasonRequired')
    if (Array.from(reason.trim()).length > registrationReasonMaxLength.value) {
      return t('auth.registrationReasonTooLong', { max: registrationReasonMaxLength.value })
    }
    return ''
  }
  return { loadRegistrationReviewSettings, registrationReviewSettings, registrationReviewInstructions, registrationReasonMaxLength, registrationReasonError }
}
