import { computed, type MaybeRefOrGetter, type WritableComputedRef, toValue } from 'vue'
import type {
  CodexClientSurface,
  CodexIdentityPolicy,
  CodexOSProfileID,
  CodexOSProfilePolicy,
} from '@/types/codexIdentity'
import {
  cloneCodexIdentityPolicy,
  cloneCodexOSProfile,
  normalizeCodexIdentityPolicy,
  validateCodexIdentityPolicy,
} from '@/utils/codexIdentityValidation'

export function useCodexIdentityPolicy(
  model: WritableComputedRef<CodexIdentityPolicy>,
  availableProxyIDs: MaybeRefOrGetter<readonly number[]> = [],
) {
  const validation = computed(() =>
    validateCodexIdentityPolicy(model.value, {
      availableProxyIDs: new Set(toValue(availableProxyIDs)),
    }),
  )

  const replace = (policy: CodexIdentityPolicy) => {
    model.value = normalizeCodexIdentityPolicy(policy)
  }

  const update = (mutator: (draft: CodexIdentityPolicy) => void) => {
    const draft = cloneCodexIdentityPolicy(model.value)
    mutator(draft)
    replace(draft)
  }

  const setProfile = (
    profileID: CodexOSProfileID,
    surface: CodexClientSurface,
    profile: CodexOSProfilePolicy | null,
  ) => {
    update((draft) => {
      const profiles = (draft.profiles ?? []).filter(
        (item) => item.os_class !== profileID || item.canonical_surface !== surface,
      )
      if (profile) profiles.push(cloneCodexOSProfile(profile))
      draft.profiles = profiles
    })
  }

  return {
    policy: model,
    validation,
    replace,
    update,
    setProfile,
  }
}
