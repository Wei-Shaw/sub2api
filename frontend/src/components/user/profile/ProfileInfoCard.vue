<template>
  <div class="space-y-6">
    <!--
      Account overview. This was a gradient hero (primary → white → amber) with
      a 20px-radius avatar tile, pill "source" chips on a translucent ground and
      three rounded-2xl metric wells stacked on top of the gradient — five
      surface treatments for one block of facts. It is now one bordered block:
      identity on top, quantities in a hairline strip beneath, and the only
      round thing left is the avatar.
    -->
    <section
      data-testid="profile-overview-hero"
      class="rounded border border-line bg-surface"
    >
      <div class="flex flex-col gap-4 px-4 py-4 sm:flex-row sm:items-start sm:gap-5">
        <!-- Avatar: the second and last sanctioned `rounded-full` in the system. -->
        <div
          class="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-full border border-line bg-surface-sunken text-md font-semibold text-ink-secondary"
        >
          <img
            v-if="avatarUrl"
            :src="avatarUrl"
            :alt="displayName"
            class="h-full w-full object-cover"
          >
          <span v-else aria-hidden="true">{{ avatarInitial }}</span>
        </div>

        <div class="min-w-0 flex-1 space-y-2">
          <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
            <h2 class="min-w-0 truncate text-lg font-semibold text-ink">
              {{ displayName }}
            </h2>
            <!--
              Role is a CATEGORY, not a state, so it stays neutral — the accent
              only ever means interactive or selected. Account state is the one
              thing here that gets a tone, and it carries its own word.
            -->
            <Badge caps>
              {{ user?.role === 'admin' ? t('profile.administrator') : t('profile.user') }}
            </Badge>
            <StatusDot
              :tone="user?.status === 'active' ? 'success' : 'danger'"
              :label="user?.status === 'active' ? t('common.active') : t('common.disabled')"
            />
          </div>

          <!--
            The synced-from-provider hints used to be rendered twice: as pill
            chips here and again as a panel below. One screen, one place.
          -->
          <p v-if="primaryEmailDisplay" class="truncate text-sm text-ink-secondary">
            {{ primaryEmailDisplay }}
          </p>
        </div>
      </div>

      <!--
        Quantities. Mono tabular through `NumCell`, so balance and concurrency
        align on the decimal and a missing user reads as an en dash rather than
        as a confident zero.
      -->
      <dl class="grid grid-cols-1 gap-px border-t border-line bg-line-subtle sm:grid-cols-3">
        <div data-testid="profile-overview-metric-balance" class="bg-surface px-4 py-3">
          <dt class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('profile.accountBalance') }}
          </dt>
          <dd class="mt-1">
            <NumCell :value="balanceValue" :precision="2" unit="USD" />
          </dd>
        </div>
        <div data-testid="profile-overview-metric-concurrency" class="bg-surface px-4 py-3">
          <dt class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('profile.concurrencyLimit') }}
          </dt>
          <dd class="mt-1">
            <NumCell :value="concurrencyValue" />
          </dd>
        </div>
        <div data-testid="profile-overview-metric-member-since" class="bg-surface px-4 py-3">
          <dt class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('profile.memberSince') }}
          </dt>
          <dd class="mt-1 font-mono text-sm tabular-nums text-ink">
            {{ memberSinceLabel }}
          </dd>
        </div>
      </dl>
    </section>

    <div class="space-y-6">
      <div data-testid="profile-main-column" class="space-y-6">
        <Surface
          data-testid="profile-basics-panel"
          :title="t('profile.basicsTitle')"
          :description="t('profile.basicsDescription')"
          flush
        >
          <!--
            One hairline between the two halves instead of two nested rounded
            wells inside a third rounded panel. The gutter IS the separator.
          -->
          <div class="grid gap-px bg-line-subtle md:grid-cols-2">
            <div class="bg-surface p-4">
              <ProfileAvatarCard :user="user" embedded />
            </div>

            <div class="bg-surface p-4">
              <ProfileEditForm :initial-username="user?.username || ''" embedded />
            </div>
          </div>
        </Surface>

        <Surface data-testid="profile-auth-bindings-panel" flush>
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
        </Surface>
      </div>

      <div data-testid="profile-side-column" class="space-y-6">
        <Surface
          v-if="sourceHints.length"
          :title="t('profile.linkedProfileSources')"
          :description="t('profile.linkedProfileSourcesDescription')"
          flush
        >
          <ul class="divide-y divide-line-subtle">
            <li
              v-for="hint in sourceHints"
              :key="hint.key"
              class="flex items-start gap-2 px-4 py-2.5 text-sm text-ink-secondary"
            >
              <Icon name="link" size="xs" class="mt-1 shrink-0 text-ink-tertiary" />
              <span class="min-w-0">{{ hint.text }}</span>
            </li>
          </ul>
        </Surface>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Badge from '@/components/common/Badge.vue'
import NumCell from '@/components/common/NumCell.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import Surface from '@/components/common/Surface.vue'
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

/*
 * `null` when there is no user, NOT `0`. A missing account and an account with
 * an empty wallet are different facts; `NumCell` renders the former as an en
 * dash instead of asserting a balance nobody measured.
 */
const balanceValue = computed<number | null>(() =>
  props.user?.balance == null ? null : Number(props.user.balance)
)
const concurrencyValue = computed<number | null>(() =>
  props.user?.concurrency == null ? null : Number(props.user.concurrency)
)

const memberSinceLabel = computed(() => {
  const raw = props.user?.created_at?.trim()
  if (!raw) {
    return '–'
  }

  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return '–'
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
