<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title. The brand lockup lives in AuthLayout, so this is the page h1. -->
      <div>
        <h1 class="text-lg font-semibold text-ink">
          {{ t('auth.resetPasswordTitle') }}
        </h1>
        <p class="mt-1 text-sm text-ink-tertiary">
          {{ t('auth.resetPasswordHint') }}
        </p>
      </div>

      <!--
        Both terminal states — expired link and success — were centred pastel
        panels with a glyph in a 48px circle. They are now typographic status
        blocks: an eyebrow carrying the status tone, one line of ink, and a
        hairline. The tint is gone; the words are the state.
      -->
      <div v-if="isInvalidLink" class="space-y-4 border-t border-line pt-5">
        <div>
          <p class="text-2xs uppercase tracking-[0.08em] text-warn">
            {{ t('auth.invalidResetLink') }}
          </p>
          <p class="mt-2 text-sm text-ink">
            {{ t('auth.invalidResetLinkHint') }}
          </p>
        </div>

        <router-link
          to="/forgot-password"
          class="inline-block text-sm text-accent underline-offset-2 transition-colors duration-fast hover:text-accent-hover hover:underline"
        >
          {{ t('auth.requestNewResetLink') }}
        </router-link>
      </div>

      <!-- Success State -->
      <div v-else-if="isSuccess" class="space-y-5 border-t border-line pt-5">
        <div>
          <p class="text-2xs uppercase tracking-[0.08em] text-success">
            {{ t('auth.passwordResetSuccess') }}
          </p>
          <p class="mt-2 text-sm text-ink">
            {{ t('auth.passwordResetSuccessHint') }}
          </p>
        </div>

        <Button to="/login" tone="accent" variant="solid" size="md" block>
          {{ t('auth.signIn') }}
        </Button>
      </div>

      <!-- Form State -->
      <form v-else @submit.prevent="handleSubmit" class="space-y-4">
        <!--
          The leading mail/lock glyphs are gone: they labelled fields that
          already carry a text label. The readonly email no longer paints its
          own gray ground either — `.input:disabled` already sunkens it.
        -->
        <FormField id="email" :label="t('auth.emailLabel')">
          <template #default>
            <input id="email" :value="email" type="email" readonly disabled class="input" />
          </template>
        </FormField>

        <FormField id="password" :label="t('auth.newPassword')" :error="errors.password">
          <template #default="{ describedBy, invalid }">
            <div class="relative">
              <input
                id="password"
                v-model="formData.password"
                :type="showPassword ? 'text' : 'password'"
                required
                autocomplete="new-password"
                :disabled="isLoading"
                :aria-describedby="describedBy"
                :aria-invalid="invalid || undefined"
                class="input pr-10"
                :class="{ 'input-error': errors.password }"
                :placeholder="t('auth.newPasswordPlaceholder')"
              />
              <button
                type="button"
                @click="showPassword = !showPassword"
                :disabled="isLoading"
                :aria-label="showPassword ? t('common.hidePassword') : t('common.showPassword')"
                class="absolute inset-y-0 right-0 flex items-center px-3 text-ink-tertiary transition-colors duration-fast hover:text-ink"
              >
                <Icon v-if="showPassword" name="eyeOff" size="sm" />
                <Icon v-else name="eye" size="sm" />
              </button>
            </div>
          </template>
        </FormField>

        <FormField
          id="confirmPassword"
          :label="t('auth.confirmPassword')"
          :error="errors.confirmPassword"
        >
          <template #default="{ describedBy, invalid }">
            <div class="relative">
              <input
                id="confirmPassword"
                v-model="formData.confirmPassword"
                :type="showConfirmPassword ? 'text' : 'password'"
                required
                autocomplete="new-password"
                :disabled="isLoading"
                :aria-describedby="describedBy"
                :aria-invalid="invalid || undefined"
                class="input pr-10"
                :class="{ 'input-error': errors.confirmPassword }"
                :placeholder="t('auth.confirmPasswordPlaceholder')"
              />
              <button
                type="button"
                @click="showConfirmPassword = !showConfirmPassword"
                :disabled="isLoading"
                :aria-label="
                  showConfirmPassword ? t('common.hidePassword') : t('common.showPassword')
                "
                class="absolute inset-y-0 right-0 flex items-center px-3 text-ink-tertiary transition-colors duration-fast hover:text-ink"
              >
                <Icon v-if="showConfirmPassword" name="eyeOff" size="sm" />
                <Icon v-else name="eye" size="sm" />
              </button>
            </div>
          </template>
        </FormField>

        <!--
          The label stays put while loading. It used to swap to "Resetting…",
          which changed the button's width at exactly the moment the user was
          watching it; `Button` overlays the spinner on a reserved label box
          and sets `aria-busy` instead of hand-rolling an `<svg>` spinner.
        -->
        <Button
          type="submit"
          tone="accent"
          variant="solid"
          size="md"
          block
          :loading="isLoading"
          :disabled="isLoading"
          data-testid="reset-password-submit"
        >
          {{ t('auth.resetPassword') }}
        </Button>
      </form>
    </div>

    <!-- Footer -->
    <template #footer>
      <p class="text-sm text-ink-tertiary">
        {{ t('auth.rememberedPassword') }}
        <router-link
          to="/login"
          class="text-accent underline-offset-2 transition-colors duration-fast hover:text-accent-hover hover:underline"
        >
          {{ t('auth.signIn') }}
        </router-link>
      </p>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { resetPassword } from '@/api/auth'

const { t } = useI18n()

// ==================== Router & Stores ====================

const route = useRoute()
const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const isSuccess = ref<boolean>(false)
const errorMessage = ref<string>('')
const showPassword = ref<boolean>(false)
const showConfirmPassword = ref<boolean>(false)

// URL parameters
const email = ref<string>('')
const token = ref<string>('')

const formData = reactive({
  password: '',
  confirmPassword: ''
})

const errors = reactive({
  password: '',
  confirmPassword: ''
})

const validationToastMessage = computed(
  () => errors.password || errors.confirmPassword || ''
)

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

// Check if the reset link is valid (has email and token)
const isInvalidLink = computed(() => !email.value || !token.value)

// ==================== Lifecycle ====================

onMounted(() => {
  // Get email and token from URL query parameters
  email.value = (route.query.email as string) || ''
  token.value = (route.query.token as string) || ''

  if (!email.value || !token.value) {
    appStore.showError(t('auth.invalidResetLink'))
  }
})

// ==================== Validation ====================

function validateForm(): boolean {
  errors.password = ''
  errors.confirmPassword = ''

  let isValid = true

  // Password validation
  if (!formData.password) {
    errors.password = t('auth.passwordRequired')
    isValid = false
  } else if (formData.password.length < 6) {
    errors.password = t('auth.passwordMinLength')
    isValid = false
  }

  // Confirm password validation
  if (!formData.confirmPassword) {
    errors.confirmPassword = t('auth.confirmPasswordRequired')
    isValid = false
  } else if (formData.password !== formData.confirmPassword) {
    errors.confirmPassword = t('auth.passwordsDoNotMatch')
    isValid = false
  }

  return isValid
}

// ==================== Form Handlers ====================

async function handleSubmit(): Promise<void> {
  errorMessage.value = ''

  if (!validateForm()) {
    return
  }

  isLoading.value = true

  try {
    await resetPassword({
      email: email.value,
      token: token.value,
      new_password: formData.password
    })

    isSuccess.value = true
    appStore.showSuccess(t('auth.passwordResetSuccess'))
  } catch (error: unknown) {
    const err = error as { message?: string; response?: { data?: { detail?: string; code?: string } } }

    // Check for invalid/expired token error
    if (err.response?.data?.code === 'INVALID_RESET_TOKEN') {
      errorMessage.value = t('auth.invalidOrExpiredToken')
    } else if (err.response?.data?.detail) {
      errorMessage.value = err.response.data.detail
    } else if (err.message) {
      errorMessage.value = err.message
    } else {
      errorMessage.value = t('auth.resetPasswordFailed')
    }

    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
  }
}
</script>

<!--
  The `<style scoped>` block held `.fade-*` classes for a `<transition
  name="fade">` this template never had — dead CSS carrying a banned
  `transition: all`.
-->
