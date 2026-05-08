<template>
  <div class="space-y-6">
    <section
      data-testid="profile-overview-hero"
      class="card overflow-hidden border border-primary-100/80 bg-gradient-to-br from-primary-50 via-white to-amber-50/70 dark:border-primary-900/40 dark:from-primary-950/40 dark:via-dark-900 dark:to-dark-950"
    >
      <div class="px-6 py-6 md:px-8">
        <div class="flex flex-col gap-6 lg:flex-row lg:items-start">
          <div
            class="flex h-20 w-20 shrink-0 items-center justify-center overflow-hidden rounded-[1.75rem] bg-gradient-to-br from-primary-500 to-primary-600 text-2xl font-bold text-white shadow-lg shadow-primary-500/20"
          >
            <img
              v-if="avatarUrl"
              :src="avatarUrl"
              :alt="displayName"
              class="h-full w-full object-cover"
            >
            <span v-else>{{ avatarInitial }}</span>
          </div>

          <div class="min-w-0 flex-1 space-y-5">
            <div class="space-y-3">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="truncate text-2xl font-semibold text-gray-900 dark:text-white">
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

              <div class="space-y-1">
                <p class="truncate text-sm text-gray-600 dark:text-gray-300">
                  {{ primaryEmailDisplay }}
                </p>
                <div
                  v-if="sourceHints.length"
                  class="flex flex-wrap gap-2 text-xs text-gray-500 dark:text-gray-400"
                >
                  <span
                    v-for="hint in sourceHints"
                    :key="hint.key"
                    class="inline-flex items-center gap-1 rounded-full bg-white/80 px-3 py-1 ring-1 ring-primary-100 dark:bg-dark-900/70 dark:ring-primary-900/40"
                  >
                    <Icon name="link" size="sm" />
                    {{ hint.text }}
                  </span>
                </div>
              </div>
            </div>

            <div class="grid gap-3 sm:grid-cols-3">
              <div
                data-testid="profile-overview-metric-balance"
                class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700"
              >
                <p class="text-xs font-medium uppercase tracking-[0.16em] text-gray-400 dark:text-gray-500">
                  {{ t('profile.accountBalance') }}
                </p>
                <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">
                  {{ formatCurrency(user?.balance || 0) }}
                </p>
              </div>
              <div
                data-testid="profile-overview-metric-concurrency"
                class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700"
              >
                <p class="text-xs font-medium uppercase tracking-[0.16em] text-gray-400 dark:text-gray-500">
                  {{ t('profile.concurrencyLimit') }}
                </p>
                <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">
                  {{ user?.concurrency || 0 }}
                </p>
              </div>
              <div
                data-testid="profile-overview-metric-member-since"
                class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700"
              >
                <p class="text-xs font-medium uppercase tracking-[0.16em] text-gray-400 dark:text-gray-500">
                  {{ t('profile.memberSince') }}
                </p>
                <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">
                  {{ memberSinceLabel }}
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <div class="space-y-6">
      <div data-testid="profile-main-column" class="space-y-6">
        <section
          data-testid="profile-basics-panel"
          class="card border border-gray-100 bg-white/90 p-6 dark:border-dark-700 dark:bg-dark-900/50"
        >
          <div class="mb-5 flex items-start justify-between gap-4">
            <div>
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('profile.basicsTitle') }}
              </h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('profile.basicsDescription') }}
              </p>
            </div>
          </div>

          <div class="grid gap-6 sm:grid-cols-1 md:grid-cols-2">
            <div class="rounded-3xl border border-gray-100 bg-gray-50/80 p-5 dark:border-dark-700 dark:bg-dark-900/30">
              <ProfileAvatarCard
                :user="user"
                embedded
              />
            </div>

            <div class="rounded-3xl border border-gray-100 bg-gray-50/80 p-5 dark:border-dark-700 dark:bg-dark-900/30">
              <ProfileEditForm
                :initial-username="user?.username || ''"
                embedded
              />
            </div>
          </div>
        </section>

        <section
          data-testid="profile-auth-bindings-panel"
          class="card border border-gray-100 bg-white/90 p-6 dark:border-dark-700 dark:bg-dark-900/50"
        >
          <ProfileIdentityBindingsSection
            :user="user"
            :linuxdo-enabled="linuxdoEnabled"
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
          class="card border border-gray-100 bg-white/90 p-6 dark:border-dark-700 dark:bg-dark-900/50"
        >
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('profile.linkedProfileSources') }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('profile.linkedProfileSourcesDescription') }}
          </p>

          <div class="mt-5 grid gap-3">
            <div
              v-for="hint in sourceHints"
              :key="hint.key"
              class="flex items-start gap-3 rounded-2xl border border-gray-100 bg-gray-50/80 px-4 py-3 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-900/30 dark:text-gray-300"
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
  oidcEnabled?: boolean
  oidcProviderName?: string
  wechatEnabled?: boolean
  wechatOpenEnabled?: boolean
  wechatMpEnabled?: boolean
}>(), {
  linuxdoEnabled: false,
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
