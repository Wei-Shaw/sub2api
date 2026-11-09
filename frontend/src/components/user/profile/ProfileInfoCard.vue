<template>
  <div class="space-y-6">
    <section
      data-testid="profile-overview-hero"
      class="card overflow-hidden"
    >
      <div class="flex flex-col gap-5 p-5 lg:flex-row lg:items-center lg:justify-between">
        <div class="flex min-w-0 items-center gap-4">
          <div
            class="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-full bg-primary-600 text-lg font-semibold text-white"
          >
            <img
              v-if="avatarUrl"
              :src="avatarUrl"
              :alt="displayName"
              class="h-full w-full object-cover"
            >
            <span v-else>{{ avatarInitial }}</span>
          </div>

          <div class="min-w-0 space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="truncate text-lg font-semibold tracking-tight text-gray-900 dark:text-white">
                {{ displayName }}
              </h2>
              <span :class="['badge', user?.role === 'admin' ? 'badge-primary' : 'badge-gray']">
                {{ user?.role === 'admin' ? t('profile.administrator') : t('profile.user') }}
              </span>
              <span
                :class="['badge', user?.status === 'active' ? 'badge-success' : 'badge-danger']"
              >
                {{
                  user?.status === 'active'
                    ? t('common.active')
                    : t('common.disabled')
                }}
              </span>
            </div>
            <p class="truncate text-sm text-gray-500 dark:text-dark-400">
              {{ primaryEmailDisplay }}
            </p>
            <div
              v-if="sourceHints.length"
              class="flex flex-wrap gap-1.5 pt-1"
            >
              <span
                v-for="hint in sourceHints"
                :key="hint.key"
                class="badge badge-gray"
              >
                <Icon name="link" size="xs" />
                {{ hint.text }}
              </span>
            </div>
          </div>
        </div>

        <dl
          class="grid grid-cols-3 divide-x divide-gray-200/70 rounded-lg border border-gray-200/70 dark:divide-dark-800 dark:border-dark-800 lg:w-[26rem] lg:flex-shrink-0"
        >
          <div
            data-testid="profile-overview-metric-balance"
            class="min-w-0 px-3 py-3 sm:px-4"
          >
            <dt class="truncate text-xs text-gray-500 dark:text-dark-400">
              {{ t('profile.accountBalance') }}
            </dt>
            <dd class="mt-1 truncate text-[13px] font-semibold tabular-nums text-gray-900 dark:text-white sm:text-base">
              {{ formatCurrency(user?.balance || 0) }}
            </dd>
          </div>
          <div
            data-testid="profile-overview-metric-concurrency"
            class="min-w-0 px-3 py-3 sm:px-4"
          >
            <dt class="truncate text-xs text-gray-500 dark:text-dark-400">
              {{ t('profile.concurrencyLimit') }}
            </dt>
            <dd class="mt-1 truncate text-[13px] font-semibold tabular-nums text-gray-900 dark:text-white sm:text-base">
              {{ user?.concurrency || 0 }}
            </dd>
          </div>
          <div
            data-testid="profile-overview-metric-member-since"
            class="min-w-0 px-3 py-3 sm:px-4"
          >
            <dt class="truncate text-xs text-gray-500 dark:text-dark-400">
              {{ t('profile.memberSince') }}
            </dt>
            <dd class="mt-1 text-[13px] font-semibold tabular-nums text-gray-900 dark:text-white sm:text-base">
              {{ memberSinceLabel }}
            </dd>
          </div>
        </dl>
      </div>
    </section>

    <div class="space-y-6">
      <div data-testid="profile-main-column" class="space-y-6">
        <section
          data-testid="profile-basics-panel"
          class="card p-5"
        >
          <div class="mb-4 flex items-start justify-between gap-4">
            <div>
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('profile.basicsTitle') }}
              </h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                {{ t('profile.basicsDescription') }}
              </p>
            </div>
          </div>

          <div class="grid gap-4 md:grid-cols-2">
            <div class="rounded-lg border border-gray-200/70 p-4 dark:border-dark-800">
              <ProfileAvatarCard
                :user="user"
                embedded
              />
            </div>

            <div class="rounded-lg border border-gray-200/70 p-4 dark:border-dark-800">
              <ProfileEditForm
                :initial-username="user?.username || ''"
                embedded
              />
            </div>
          </div>
        </section>

        <section
          data-testid="profile-auth-bindings-panel"
          class="card p-5"
        >
          <ProfileIdentityBindingsSection
            :user="user"
            :linuxdo-enabled="linuxdoEnabled"
            :dingtalk-enabled="dingtalkEnabled"
            :oidc-enabled="oidcEnabled"
            :oidc-provider-name="oidcProviderName"
            :wechat-enabled="wechatEnabled"
            :wechat-open-enabled="wechatOpenEnabled"
            :wechat-mp-enabled="wechatMpEnabled"
            embedded
            compact
          />
        </section>
      </div>

      <div data-testid="profile-side-column" class="space-y-6">
        <section
          v-if="sourceHints.length"
          class="card p-5"
        >
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('profile.linkedProfileSources') }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('profile.linkedProfileSourcesDescription') }}
          </p>

          <div class="mt-4 grid gap-2">
            <div
              v-for="hint in sourceHints"
              :key="hint.key"
              class="flex items-start gap-3 rounded-lg border border-gray-200/70 px-3.5 py-2.5 text-sm text-gray-600 dark:border-dark-800 dark:text-dark-300"
            >
              <Icon name="link" size="sm" class="mt-0.5 text-gray-400 dark:text-gray-500" />
              <span>{{ hint.text }}</span>
            </div>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ProfileAvatarCard from '@/components/user/profile/ProfileAvatarCard.vue'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import type { User, UserAuthBindingStatus, UserAuthProvider, UserProfileSourceContext } from '@/types'

const props = withDefaults(defineProps<{
  user: User | null
  linuxdoEnabled?: boolean
  dingtalkEnabled?: boolean
  oidcEnabled?: boolean
  oidcProviderName?: string
  wechatEnabled?: boolean
  wechatOpenEnabled?: boolean
  wechatMpEnabled?: boolean
}>(), {
  linuxdoEnabled: false,
  dingtalkEnabled: false,
  oidcEnabled: false,
  oidcProviderName: 'OIDC',
  wechatEnabled: false,
  wechatOpenEnabled: undefined,
  wechatMpEnabled: undefined,
})

const { t } = useI18n()

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

function isEmailBound(user: User | null | undefined): boolean {
  if (typeof user?.email_bound === 'boolean') {
    return user.email_bound
  }

  const nested = user?.auth_bindings?.email ?? user?.identity_bindings?.email
  const normalized = normalizeBindingStatus(nested)
  return normalized ?? false
}

const avatarUrl = computed(() => props.user?.avatar_url?.trim() || '')
const displayName = computed(() => props.user?.username?.trim() || props.user?.email?.trim() || t('profile.user'))
const primaryEmailDisplay = computed(() => {
  const email = props.user?.email?.trim() || ''
  if (!email) {
    return ''
  }
  if (email.endsWith('.invalid') && !isEmailBound(props.user)) {
    return ''
  }
  return email
})
const avatarInitial = computed(() => displayName.value.charAt(0).toUpperCase() || 'U')
const memberSinceLabel = computed(() => {
  const raw = props.user?.created_at?.trim()
  if (!raw) {
    return '-'
  }

  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
  }).format(date)
})

const providerLabels = computed<Record<UserAuthProvider, string>>(() => ({
  email: t('profile.authBindings.providers.email'),
  linuxdo: t('profile.authBindings.providers.linuxdo'),
  dingtalk: t('profile.authBindings.providers.dingtalk'),
  oidc: t('profile.authBindings.providers.oidc', { providerName: props.oidcProviderName }),
  wechat: t('profile.authBindings.providers.wechat'),
  github: 'GitHub',
  google: 'Google'
}))

function formatCurrency(value: number): string {
  return `$${value.toFixed(2)}`
}

function normalizeProvider(value: string): UserAuthProvider | null {
  const normalized = value.trim().toLowerCase()
  if (
    normalized === 'email' ||
    normalized === 'linuxdo' ||
    normalized === 'dingtalk' ||
    normalized === 'wechat' ||
    normalized === 'github' ||
    normalized === 'google'
  ) {
    return normalized
  }
  if (normalized === 'oidc' || normalized.startsWith('oidc:') || normalized.startsWith('oidc/')) {
    return 'oidc'
  }
  return null
}

function readObjectString(source: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return ''
}

function resolveThirdPartySource(
  rawSource: string | UserProfileSourceContext | null | undefined
): { provider: UserAuthProvider; label: string } | null {
  if (!rawSource) {
    return null
  }

  if (typeof rawSource === 'string') {
    const provider = normalizeProvider(rawSource)
    if (!provider || provider === 'email') {
      return null
    }
    return {
      provider,
      label: providerLabels.value[provider]
    }
  }

  const sourceRecord = rawSource as Record<string, unknown>
  const provider = normalizeProvider(
    readObjectString(sourceRecord, 'provider', 'source', 'provider_type', 'auth_provider')
  )
  if (!provider || provider === 'email') {
    return null
  }

  const explicitLabel = readObjectString(
    sourceRecord,
    'provider_label',
    'label',
    'provider_name',
    'providerName'
  )

  return {
    provider,
    label: explicitLabel || providerLabels.value[provider]
  }
}

const sourceHints = computed(() => {
  const currentUser = props.user
  if (!currentUser) {
    return []
  }

  const hints: Array<{ key: string; text: string }> = []
  const avatarSource = resolveThirdPartySource(
    currentUser.profile_sources?.avatar ?? currentUser.avatar_source
  )
  const usernameSource = resolveThirdPartySource(
    currentUser.profile_sources?.username ??
      currentUser.profile_sources?.display_name ??
      currentUser.profile_sources?.nickname ??
      currentUser.display_name_source ??
      currentUser.username_source ??
      currentUser.nickname_source
  )

  if (avatarSource) {
    hints.push({
      key: 'avatar',
      text: t('profile.authBindings.source.avatar', { providerName: avatarSource.label })
    })
  }

  if (usernameSource) {
    hints.push({
      key: 'username',
      text: t('profile.authBindings.source.username', { providerName: usernameSource.label })
    })
  }

  return hints
})
</script>
