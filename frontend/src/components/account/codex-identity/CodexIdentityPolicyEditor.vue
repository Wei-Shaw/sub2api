<template>
  <section class="space-y-5" :aria-labelledby="`${resolvedIdPrefix}-title`">
    <div class="flex items-start justify-between gap-4">
      <div class="min-w-0">
        <h3 :id="`${resolvedIdPrefix}-title`" class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ title || copy('admin.accounts.codexIdentity.title', 'Codex OS profile device pool') }}
        </h3>
        <p :id="`${resolvedIdPrefix}-description`" class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
          {{ description || copy('admin.accounts.codexIdentity.description', 'Keep a bounded, stable set of device identities per OAuth account. Connection and WebSocket state remain isolated by API key.') }}
        </p>
      </div>
      <button
        v-if="showModeToggle"
        type="button"
        role="switch"
        :aria-checked="enabled"
        :aria-labelledby="`${resolvedIdPrefix}-title`"
        :aria-describedby="`${resolvedIdPrefix}-description`"
        :disabled="disabled"
        class="relative inline-flex h-6 w-11 shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
        :class="enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'"
        data-testid="codex-identity-policy-toggle"
        @click="toggleEnabled"
      >
        <span
          aria-hidden="true"
          class="pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transition-transform"
          :class="enabled ? 'translate-x-5' : 'translate-x-0'"
        />
      </button>
    </div>

    <div
      v-if="showModeToggle && !enabled"
      class="border-y border-gray-100 py-3 text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400"
      role="status"
      data-testid="codex-identity-policy-off"
    >
      {{ copy('admin.accounts.codexIdentity.offState', 'Disabled. Existing Codex identity behavior is unchanged.') }}
    </div>

    <template v-if="enabled">
      <fieldset :disabled="disabled" class="space-y-3">
        <div>
          <legend class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ copy('admin.accounts.codexIdentity.profiles', 'Operating-system profiles') }}
          </legend>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
            {{ copy('admin.accounts.codexIdentity.profilesDesc', 'Enable only the environments this account should accept. Desktop and CLI can converge within the same OS; cross-OS conversion is never allowed.') }}
          </p>
        </div>

        <ul class="divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-600" data-testid="codex-profile-list">
          <OSProfileRow
            v-for="profileID in CODEX_OS_PROFILE_IDS"
            :key="profileID"
            :profile-id="profileID"
            :model-value="profilesFor(profileID)"
            :proxies="proxies"
            :account-proxy-id="accountProxyId"
            :template-context="templateContext"
            :disabled="disabled"
            :issues-by-surface="issuesForOS(profileID)"
            :id-prefix="`${resolvedIdPrefix}-${profileID}`"
            @update:profile="setSurfaceProfile(profileID, $event.surface, $event.profile)"
          />
        </ul>
      </fieldset>

      <div class="border-t border-gray-200 pt-5 dark:border-dark-600">
        <SessionPolicyEditor
          :model-value="policy.session_policy"
          :affinity-ttl-seconds="policy.affinity_ttl_seconds"
          :disabled="disabled"
          :id-prefix="`${resolvedIdPrefix}-session`"
          @update:model-value="updateSessionPolicy"
          @update:affinity-ttl-seconds="updateAffinityTTL"
        />
      </div>

      <div
        v-if="validation.errors.length"
        :id="`${resolvedIdPrefix}-errors`"
        class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 dark:border-red-900 dark:bg-red-900/15"
        role="alert"
        aria-live="polite"
        data-testid="codex-identity-errors"
      >
        <p class="text-xs font-semibold text-red-700 dark:text-red-300">
          {{ copy('admin.accounts.codexIdentity.fixErrors', 'Complete the identity policy before saving.') }}
        </p>
        <ul class="mt-1 list-disc space-y-1 pl-4 text-xs text-red-600 dark:text-red-400">
          <li v-for="issue in validation.errors" :key="`${issue.code}:${issue.path}`">{{ validationMessage(issue) }}</li>
        </ul>
      </div>

      <div
        v-else
        class="flex items-center gap-2 text-xs text-emerald-700 dark:text-emerald-400"
        role="status"
        aria-live="polite"
        data-testid="codex-identity-valid"
      >
        <Icon name="checkCircle" size="sm" />
        <span>{{ copy('admin.accounts.codexIdentity.valid', 'Identity policy is ready to validate on the server.') }}</span>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useCodexIdentityPolicy } from '@/composables/useCodexIdentityPolicy'
import {
  CODEX_OS_PROFILE_IDS,
  type CodexIdentityPolicy,
  type CodexIdentityProxyOption,
  type CodexIdentityValidationIssue,
  type CodexIdentityValidationResult,
  type CodexOSProfileID,
  type CodexOSProfilePolicy,
  type CodexSessionPolicy,
} from '@/types/codexIdentity'
import {
  allowedCodexSurfaces,
  codexIdentityValidationMessageKey,
  availableCodexIdentityProxyIDs,
} from '@/utils/codexIdentityValidation'
import OSProfileRow from './OSProfileRow.vue'
import SessionPolicyEditor from './SessionPolicyEditor.vue'
import { useCodexIdentityCopy } from './copy'

const props = withDefaults(defineProps<{
  modelValue: CodexIdentityPolicy
  proxies?: readonly CodexIdentityProxyOption[]
  accountProxyId?: number | null
  templateContext?: boolean
  disabled?: boolean
  idPrefix?: string
  showModeToggle?: boolean
  title?: string
  description?: string
}>(), {
  proxies: () => [],
  accountProxyId: null,
  templateContext: false,
  disabled: false,
  idPrefix: '',
  showModeToggle: true,
  title: '',
  description: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: CodexIdentityPolicy]
  'validation-change': [value: CodexIdentityValidationResult]
}>()

const copy = useCodexIdentityCopy()
const instanceID = getCurrentInstance()?.uid ?? 0
const resolvedIdPrefix = computed(() => props.idPrefix || `codex-identity-policy-${instanceID}`)
const policy = computed<CodexIdentityPolicy>({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})
const availableProxyIDs = computed(() => [...availableCodexIdentityProxyIDs(props.proxies)])
const { validation, update, setProfile } = useCodexIdentityPolicy(policy, availableProxyIDs)
const enabled = computed(() => policy.value.mode === 'os_profile_device_pool')

watch(validation, (value) => emit('validation-change', value), { immediate: true, deep: true })

const toggleEnabled = () => {
  update((draft) => {
    draft.mode = enabled.value ? 'off' : 'os_profile_device_pool'
    if (draft.mode === 'off') draft.profiles = []
  })
}

const profilesFor = (profileID: CodexOSProfileID): CodexOSProfilePolicy[] =>
  (policy.value.profiles ?? []).filter((profile) => profile.os_class === profileID)

const issuesForProfile = (
  profileID: CodexOSProfileID,
  surface: CodexOSProfilePolicy['canonical_surface'],
): CodexIdentityValidationIssue[] => {
  const index = (policy.value.profiles ?? []).findIndex(
    (profile) => profile.os_class === profileID && profile.canonical_surface === surface,
  )
  if (index < 0) return []
  return validation.value.errors.filter((issue) => issue.path.startsWith(`profiles.${index}.`))
}

const issuesForOS = (profileID: CodexOSProfileID) => Object.fromEntries(
  allowedCodexSurfaces(profileID).map((surface) => [
    surface,
    issuesForProfile(profileID, surface),
  ]),
)

const setSurfaceProfile = (
  profileID: CodexOSProfileID,
  surface: CodexOSProfilePolicy['canonical_surface'],
  profile: CodexOSProfilePolicy | null,
) => setProfile(profileID, surface, profile)

const validationMessage = (issue: CodexIdentityValidationIssue): string => {
  const key = codexIdentityValidationMessageKey(issue.code)
  return key ? copy(key, issue.message) : issue.message
}

const updateSessionPolicy = (sessionPolicy: CodexSessionPolicy) => {
  update((draft) => {
    draft.session_policy = sessionPolicy
  })
}

const updateAffinityTTL = (seconds: number) => {
  update((draft) => {
    draft.affinity_ttl_seconds = seconds
  })
}

defineExpose({
  validation,
  validate: (): CodexIdentityValidationResult => validation.value,
})
</script>
