<template>
  <div :class="props.embedded ? '' : 'rounded border border-line bg-surface'">
    <div class="border-b border-line px-4 py-3">
      <h2 class="text-sm font-semibold text-ink">
        {{ t('profile.authBindings.title') }}
      </h2>
      <p class="mt-0.5 text-xs text-ink-tertiary">
        {{ t('profile.authBindings.description') }}
      </p>
    </div>

    <!--
      One hairline per provider. The rows used to be rounded-2xl cards nested
      inside a rounded card inside a rounded panel; the separator is the design
      now, and nothing here has a background of its own.
    -->
    <div class="divide-y divide-line-subtle">
      <div v-for="item in providerItems" :key="item.provider" class="px-4 py-3">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="flex min-w-0 flex-1 items-start gap-3">
            <!--
              A squared mark carrying the PROVIDER's own colour — the same
              decision LoginView made for its five OAuth sections. A pill tinted
              from the semantic palette would read as "this provider is
              healthy", which is not a claim a binding row makes. Email and a
              generic OIDC issuer have no brand colour we know, so they stay
              neutral rather than borrowing the accent.
            -->
            <span
              :class="[
                'flex h-6 w-6 shrink-0 items-center justify-center font-mono text-2xs font-semibold',
                providerMarkClass(item.provider),
              ]"
              aria-hidden="true"
            >
              <Icon v-if="item.provider === 'email'" name="mail" size="xs" />
              <template v-else>{{ providerInitial(item.provider) }}</template>
            </span>

            <div class="min-w-0 flex-1 space-y-2">
              <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                <h3 class="text-sm font-medium text-ink">
                  {{ item.label }}
                </h3>
                <!--
                  Bound state gets a 6px dot plus the word. Both labels stay in
                  tertiary ink: a screen listing five providers, most of them
                  connected, must not be five green words.
                -->
                <StatusDot
                  :data-testid="`profile-binding-${item.provider}-status`"
                  :tone="item.bound ? 'success' : 'neutral'"
                  :label="
                    item.bound
                      ? t('profile.authBindings.status.bound')
                      : t('profile.authBindings.status.notBound')
                  "
                  muted
                />
              </div>

              <p
                v-if="providerSummary(item.provider)"
                class="truncate text-sm text-ink-secondary"
              >
                {{ providerSummary(item.provider) }}
              </p>

              <div
                v-if="hasBindingDetails(item.provider, item.details)"
                class="grid gap-0.5 text-xs text-ink-tertiary"
              >
                <p
                  v-if="item.provider !== 'email' && item.details?.display_name"
                  class="font-medium text-ink-secondary"
                >
                  {{ item.details.display_name }}
                </p>
                <p
                  v-if="item.provider !== 'email' && item.details?.subject_hint"
                  class="font-mono tabular-nums"
                >
                  {{ item.details.subject_hint }}
                </p>
                <p v-if="bindingCountLabel(item.details)">
                  {{ bindingCountLabel(item.details) }}
                </p>
                <p v-if="bindingNote(item.details)">
                  {{ bindingNote(item.details) }}
                </p>
              </div>

              <!--
                Every field is wrapped in FormField now. These four validations
                ("email required", "invalid email", "code required", "password
                too short") previously existed ONLY as a toast: the offending
                control was never marked and the message expired on a timer.
              -->
              <div
                v-if="item.provider === 'email' && showEmailForm"
                data-testid="profile-binding-email-form"
                class="max-w-md space-y-1 pt-1"
              >
                <FormField :label="t('auth.emailLabel')" :error="emailErrors.email">
                  <template #default="{ id, describedBy, invalid }">
                    <div class="flex items-start gap-2">
                      <input
                        :id="id"
                        v-model.trim="emailBindingForm.email"
                        data-testid="profile-binding-email-input"
                        type="email"
                        autocomplete="email"
                        class="input min-w-0 flex-1"
                        :class="{ 'input-error': emailErrors.email }"
                        :aria-describedby="describedBy"
                        :aria-invalid="invalid || undefined"
                        :placeholder="t('profile.authBindings.emailPlaceholder')"
                        :disabled="isSendingEmailCode || isBindingEmail"
                      />
                      <Button
                        data-testid="profile-binding-email-send-code"
                        size="md"
                        class="h-9 shrink-0"
                        :loading="isSendingEmailCode"
                        :disabled="isBindingEmail"
                        @click="sendEmailCode"
                      >
                        {{ t('profile.authBindings.sendCodeAction') }}
                      </Button>
                    </div>
                  </template>
                </FormField>

                <FormField
                  :label="t('auth.verificationCode')"
                  :error="emailErrors.verifyCode"
                >
                  <template #default="{ id, describedBy, invalid }">
                    <input
                      :id="id"
                      v-model.trim="emailBindingForm.verifyCode"
                      data-testid="profile-binding-email-code-input"
                      type="text"
                      inputmode="numeric"
                      autocomplete="one-time-code"
                      maxlength="6"
                      class="input font-mono tabular-nums"
                      :class="{ 'input-error': emailErrors.verifyCode }"
                      :aria-describedby="describedBy"
                      :aria-invalid="invalid || undefined"
                      :placeholder="t('profile.authBindings.codePlaceholder')"
                      :disabled="isBindingEmail"
                    />
                  </template>
                </FormField>

                <FormField :label="emailPasswordLabel" :error="emailErrors.password">
                  <template #default="{ id, describedBy, invalid }">
                    <input
                      :id="id"
                      v-model="emailBindingForm.password"
                      data-testid="profile-binding-email-password-input"
                      type="password"
                      :autocomplete="emailBound ? 'current-password' : 'new-password'"
                      class="input"
                      :class="{ 'input-error': emailErrors.password }"
                      :aria-describedby="describedBy"
                      :aria-invalid="invalid || undefined"
                      :placeholder="emailPasswordPlaceholder"
                      :disabled="isBindingEmail"
                    />
                  </template>
                </FormField>

                <Button
                  data-testid="profile-binding-email-submit"
                  tone="accent"
                  variant="solid"
                  size="md"
                  block
                  :loading="isBindingEmail"
                  @click="bindEmail"
                >
                  {{ emailSubmitActionLabel }}
                </Button>
              </div>
            </div>
          </div>

          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <Button
              v-if="item.provider === 'email' && compact"
              data-testid="profile-binding-email-toggle"
              @click="toggleEmailForm"
            >
              {{
                showEmailForm
                  ? t('profile.authBindings.hideEmailFormAction')
                  : t('profile.authBindings.manageEmailAction')
              }}
            </Button>
            <Button
              v-if="item.canBind"
              :data-testid="`profile-binding-${item.provider}-action`"
              tone="accent"
              variant="solid"
              @click="startBinding(item.provider)"
            >
              {{ t('profile.authBindings.bindAction', { providerName: item.label }) }}
            </Button>
            <!--
              The label no longer swaps to "loading…" while the request is in
              flight; `Button` overlays a spinner on the reserved label box, so
              the control keeps its width and gains `aria-busy`.
            -->
            <Button
              v-if="item.canUnbind"
              :data-testid="`profile-binding-${item.provider}-unbind`"
              :loading="unbindingProvider === item.provider"
              @click="handleUnbindForItem(item.provider, item.label)"
            >
              {{ t('profile.authBindings.unbindAction') }}
            </Button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import {
  hasExplicitWeChatOAuthCapabilities,
  resolveWeChatOAuthStartStrict,
  type WeChatOAuthPublicSettings,
} from '@/api/auth'
import {
  bindEmailIdentity,
  sendEmailBindingCode,
  startOAuthBinding,
  unbindAuthIdentity,
} from '@/api/user'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import type { User, UserAuthBindingStatus, UserAuthProvider } from '@/types'

type BindableProvider = Exclude<UserAuthProvider, 'email'>

const props = withDefaults(
  defineProps<{
    user: User | null
    linuxdoEnabled?: boolean
    dingtalkEnabled?: boolean
    oidcEnabled?: boolean
    oidcProviderName?: string
    wechatEnabled?: boolean
    wechatOpenEnabled?: boolean
    wechatMpEnabled?: boolean
    embedded?: boolean
    compact?: boolean
  }>(),
  {
    linuxdoEnabled: false,
    dingtalkEnabled: false,
    oidcEnabled: false,
    oidcProviderName: 'OIDC',
    wechatEnabled: false,
    wechatOpenEnabled: undefined,
    wechatMpEnabled: undefined,
    embedded: false,
    compact: false,
  }
)

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()

const localUser = ref<User | null>(null)
const isSendingEmailCode = ref(false)
const isBindingEmail = ref(false)
const isEmailFormExpanded = ref(!props.compact)
const unbindingProvider = ref<BindableProvider | null>(null)
const emailBindingForm = reactive({
  email: '',
  verifyCode: '',
  password: '',
})
/**
 * Inline counterparts to the four validation toasts below. The toast is kept —
 * this form can be scrolled out of view inside the profile page — but the
 * message now also lands on the control that is actually wrong.
 */
const emailErrors = reactive({
  email: '',
  verifyCode: '',
  password: '',
})

function clearEmailErrors(): void {
  emailErrors.email = ''
  emailErrors.verifyCode = ''
  emailErrors.password = ''
}

watch(
  () => props.user,
  (user) => {
    localUser.value = null
    if (!user) {
      return
    }
    if (typeof user.email === 'string' && !user.email.endsWith('.invalid')) {
      emailBindingForm.email = user.email
    }
  },
  { immediate: true }
)

watch(
  () => props.compact,
  (value) => {
    if (!value) {
      isEmailFormExpanded.value = true
    }
  },
  { immediate: true }
)

const currentUser = computed(() => localUser.value ?? props.user)
const compact = computed(() => props.compact)
const emailBound = computed(() => getBindingStatus('email'))
const showEmailForm = computed(() => !compact.value || isEmailFormExpanded.value)
const emailPasswordPlaceholder = computed(() =>
  emailBound.value
    ? t('profile.authBindings.replaceEmailPasswordPlaceholder')
    : t('profile.authBindings.passwordPlaceholder')
)
/** Replacing a bound address asks for the CURRENT password; a first bind sets one. */
const emailPasswordLabel = computed(() =>
  emailBound.value ? t('profile.currentPassword') : t('auth.newPassword')
)
const emailSubmitActionLabel = computed(() =>
  emailBound.value
    ? t('profile.authBindings.confirmEmailReplaceAction')
    : t('profile.authBindings.confirmEmailBindAction')
)
const legacyBindingNoteKeys: Record<string, string> = {
  'Primary account email is managed from the profile form.':
    'profile.authBindings.notes.emailManagedFromProfile',
  'You can unbind this sign-in method.': 'profile.authBindings.notes.canUnbind',
  'Bind another sign-in method before unbinding.':
    'profile.authBindings.notes.bindAnotherBeforeUnbind',
}

function resolveLegacyCompatibleWeChatSettings(
  settings: WeChatOAuthPublicSettings | null | undefined
): (WeChatOAuthPublicSettings & {
  wechat_oauth_open_enabled: boolean
  wechat_oauth_mp_enabled: boolean
}) | null {
  if (!settings) {
    return null
  }

  if (hasExplicitWeChatOAuthCapabilities(settings)) {
    return settings
  }

  if (typeof settings.wechat_oauth_enabled !== 'boolean') {
    return null
  }

  return {
    ...settings,
    wechat_oauth_open_enabled: settings.wechat_oauth_enabled,
    wechat_oauth_mp_enabled: settings.wechat_oauth_enabled,
  }
}

const wechatOAuthSettings = computed<WeChatOAuthPublicSettings | null>(() => {
  const cachedSettings = resolveLegacyCompatibleWeChatSettings(appStore.cachedPublicSettings)
  if (cachedSettings) {
    return cachedSettings
  }

  return resolveLegacyCompatibleWeChatSettings({
    wechat_oauth_enabled: props.wechatEnabled,
    wechat_oauth_open_enabled: props.wechatOpenEnabled,
    wechat_oauth_mp_enabled: props.wechatMpEnabled,
  })
})

const resolvedWeChatBinding = computed(() => resolveWeChatOAuthStartStrict(wechatOAuthSettings.value))

function normalizeBindingStatus(binding: boolean | UserAuthBindingStatus | undefined): boolean | null {
  if (typeof binding === 'boolean') {
    return binding
  }
  if (!binding) {
    return null
  }
  if (typeof binding.bound === 'boolean') {
    return binding.bound
  }
  return Boolean(binding.provider_subject || binding.issuer || binding.provider_key)
}

function getBindingStatus(provider: UserAuthProvider): boolean {
  return getBindingStatusForUser(currentUser.value, provider)
}

function getBindingStatusForUser(user: User | null | undefined, provider: UserAuthProvider): boolean {
  if (provider === 'email') {
    if (typeof user?.email_bound === 'boolean') {
      return user.email_bound
    }
    const nested = user?.auth_bindings?.email ?? user?.identity_bindings?.email
    const normalized = normalizeBindingStatus(nested)
    return normalized ?? false
  }

  const directFlag = user?.[`${provider}_bound` as keyof User]
  if (typeof directFlag === 'boolean') {
    return directFlag
  }

  const nested = user?.auth_bindings?.[provider] ?? user?.identity_bindings?.[provider]
  const normalized = normalizeBindingStatus(nested)
  return normalized ?? false
}

function getBindingDetails(provider: UserAuthProvider): UserAuthBindingStatus | null {
  const binding = currentUser.value?.auth_bindings?.[provider] ?? currentUser.value?.identity_bindings?.[provider]
  if (!binding || typeof binding === 'boolean') {
    return null
  }
  return binding
}

function getDisplayableEmail(user: User | null | undefined): string {
  const email = user?.email?.trim() || ''
  if (!email) {
    return ''
  }
  if (email.endsWith('.invalid') && !getBindingStatusForUser(user, 'email')) {
    return ''
  }
  return email
}

function isProviderEnabledForBinding(provider: BindableProvider): boolean {
  if (provider === 'linuxdo') {
    return props.linuxdoEnabled
  }
  if (provider === 'dingtalk') {
    return props.dingtalkEnabled
  }
  if (provider === 'oidc') {
    return props.oidcEnabled
  }
  return resolvedWeChatBinding.value.mode !== null
}

const providerItems = computed(() => [
  {
    provider: 'email' as const,
    label: t('profile.authBindings.providers.email'),
    bound: getBindingStatus('email'),
    canBind: false,
    canUnbind: false,
    details: getBindingDetails('email'),
  },
  {
    provider: 'linuxdo' as const,
    label: t('profile.authBindings.providers.linuxdo'),
    bound: getBindingStatus('linuxdo'),
    canBind:
      !getBindingStatus('linuxdo') &&
      isProviderEnabledForBinding('linuxdo') &&
      (getBindingDetails('linuxdo')?.can_bind ?? true),
    canUnbind: Boolean(getBindingStatus('linuxdo') && getBindingDetails('linuxdo')?.can_unbind),
    details: getBindingDetails('linuxdo'),
  },
  {
    provider: 'dingtalk' as const,
    label: t('profile.authBindings.providers.dingtalk'),
    bound: getBindingStatus('dingtalk'),
    canBind:
      !getBindingStatus('dingtalk') &&
      isProviderEnabledForBinding('dingtalk') &&
      (getBindingDetails('dingtalk')?.can_bind ?? true),
    canUnbind: Boolean(getBindingStatus('dingtalk') && getBindingDetails('dingtalk')?.can_unbind),
    details: getBindingDetails('dingtalk'),
  },
  {
    provider: 'oidc' as const,
    label: t('profile.authBindings.providers.oidc', { providerName: props.oidcProviderName }),
    bound: getBindingStatus('oidc'),
    canBind:
      !getBindingStatus('oidc') &&
      isProviderEnabledForBinding('oidc') &&
      (getBindingDetails('oidc')?.can_bind ?? true),
    canUnbind: Boolean(getBindingStatus('oidc') && getBindingDetails('oidc')?.can_unbind),
    details: getBindingDetails('oidc'),
  },
  {
    provider: 'wechat' as const,
    label: t('profile.authBindings.providers.wechat'),
    bound: getBindingStatus('wechat'),
    canBind:
      !getBindingStatus('wechat') &&
      isProviderEnabledForBinding('wechat') &&
      (getBindingDetails('wechat')?.can_bind ?? true),
    canUnbind: Boolean(getBindingStatus('wechat') && getBindingDetails('wechat')?.can_unbind),
    details: getBindingDetails('wechat'),
  },
])

function providerInitial(provider: UserAuthProvider): string {
  if (provider === 'linuxdo') {
    return 'L'
  }
  if (provider === 'dingtalk') {
    return 'D'
  }
  if (provider === 'wechat') {
    return 'W'
  }
  if (provider === 'oidc') {
    return 'O'
  }
  return 'E'
}

/**
 * Provider marks carry the PROVIDER's colour, not a tone from the semantic
 * palette — the same call LoginView makes for its OAuth buttons. The two
 * providers with no brand of their own (the account's own email, and an OIDC
 * issuer whose identity is configured per-deployment) stay neutral: the accent
 * means interactive or selected and cannot be spent on branding.
 */
function providerMarkClass(provider: UserAuthProvider): string {
  if (provider === 'linuxdo') {
    return 'bg-[#FEB005] text-[#1D1D1F]'
  }
  if (provider === 'dingtalk') {
    return 'bg-[#1677FF] text-white'
  }
  if (provider === 'wechat') {
    return 'bg-[#07C160] text-white'
  }
  return 'border border-line bg-surface text-ink-secondary'
}

function providerSummary(provider: UserAuthProvider): string {
  if (provider === 'email') {
    return getDisplayableEmail(currentUser.value)
  }
  return ''
}

function bindingCountLabel(details: UserAuthBindingStatus | null): string {
  if (!details || typeof details.bound_count !== 'number' || details.bound_count <= 1) {
    return ''
  }
  return t('profile.authBindings.boundCount', { count: details.bound_count })
}

function bindingNote(details: UserAuthBindingStatus | null): string {
  if (!details) {
    return ''
  }

  const noteKey = details.note_key?.trim() || legacyBindingNoteKeys[details.note?.trim() || ''] || ''
  if (noteKey) {
    const translated = t(noteKey)
    if (translated !== noteKey) {
      return translated
    }
  }

  return details.note?.trim() || ''
}

function hasBindingDetails(
  provider: UserAuthProvider,
  details: UserAuthBindingStatus | null
): boolean {
  if (!details) {
    return false
  }

  const showsProviderIdentityDetails =
    provider !== 'email' && Boolean(details.display_name || details.subject_hint)

  return Boolean(showsProviderIdentityDetails || bindingCountLabel(details) || bindingNote(details))
}

function toggleEmailForm(): void {
  isEmailFormExpanded.value = !isEmailFormExpanded.value
}

function startBinding(provider: UserAuthProvider): void {
  if (provider === 'email') {
    return
  }
  startOAuthBinding(provider, {
    redirectTo: route.fullPath || '/profile',
    wechatOAuthSettings: provider === 'wechat' ? wechatOAuthSettings.value : null,
  })
}

function applyUpdatedUser(user: User): void {
  localUser.value = user
  authStore.user = user
}

async function handleUnbind(provider: BindableProvider, providerLabel: string): Promise<void> {
  unbindingProvider.value = provider
  try {
    const user = await unbindAuthIdentity(provider)
    applyUpdatedUser(user)
    appStore.showSuccess(t('profile.authBindings.unbindSuccess', { providerName: providerLabel }))
  } catch (error) {
    appStore.showError((error as { message?: string }).message || t('common.tryAgain'))
  } finally {
    unbindingProvider.value = null
  }
}

function handleUnbindForItem(provider: UserAuthProvider, providerLabel: string): void {
  if (provider === 'email') {
    return
  }
  void handleUnbind(provider, providerLabel)
}

function failEmailValidation(field: keyof typeof emailErrors, message: string): false {
  emailErrors[field] = message
  appStore.showError(message)
  return false
}

function validateEmailBindingForm(requireCode: boolean): boolean {
  clearEmailErrors()

  if (!emailBindingForm.email) {
    return failEmailValidation('email', t('auth.emailRequired'))
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailBindingForm.email)) {
    return failEmailValidation('email', t('auth.invalidEmail'))
  }
  if (requireCode && !emailBindingForm.verifyCode) {
    return failEmailValidation('verifyCode', t('auth.codeRequired'))
  }
  if (requireCode && !emailBindingForm.password) {
    return failEmailValidation('password', t('auth.passwordRequired'))
  }
  if (requireCode && !emailBound.value && emailBindingForm.password.length < 6) {
    return failEmailValidation('password', t('auth.passwordMinLength'))
  }
  return true
}

async function sendEmailCode(): Promise<void> {
  if (!validateEmailBindingForm(false)) {
    return
  }

  isSendingEmailCode.value = true
  try {
    await sendEmailBindingCode(emailBindingForm.email)
    appStore.showSuccess(t('profile.authBindings.codeSentTo', { email: emailBindingForm.email }))
  } catch (error) {
    appStore.showError((error as { message?: string }).message || t('auth.sendCodeFailed'))
  } finally {
    isSendingEmailCode.value = false
  }
}

async function bindEmail(): Promise<void> {
  if (!validateEmailBindingForm(true)) {
    return
  }

  isBindingEmail.value = true
  try {
    const user = await bindEmailIdentity({
      email: emailBindingForm.email,
      verify_code: emailBindingForm.verifyCode,
      password: emailBindingForm.password,
    })
    const replacingBoundEmail = emailBound.value
    applyUpdatedUser(user)
    emailBindingForm.verifyCode = ''
    emailBindingForm.password = ''
    clearEmailErrors()
    if (compact.value) {
      isEmailFormExpanded.value = false
    }
    appStore.showSuccess(
      replacingBoundEmail
        ? t('profile.authBindings.replaceSuccess')
        : t('profile.authBindings.bindSuccess')
    )
  } catch (error) {
    appStore.showError((error as { message?: string }).message || t('common.tryAgain'))
  } finally {
    isBindingEmail.value = false
  }
}
</script>
