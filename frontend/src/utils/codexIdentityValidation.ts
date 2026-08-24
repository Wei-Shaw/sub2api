import {
  CODEX_AFFINITY_TTL_MAX_SECONDS,
  CODEX_AFFINITY_TTL_MIN_SECONDS,
  CODEX_DEVICE_SLOT_MAX,
  CODEX_DEVICE_SLOT_MIN,
  CODEX_SESSION_SLOT_MAX,
  CODEX_SESSION_SLOT_MIN,
  type CodexArchitecture,
  type CodexClientSurface,
  type CodexIdentityPolicy,
  type CodexIdentityProxyOption,
  type CodexIdentityValidationIssue,
  type CodexIdentityValidationResult,
  type CodexOSProfileID,
  type CodexOSProfilePolicy,
  type CodexSessionPolicy,
} from '@/types/codexIdentity'

const PROFILE_SURFACES: Record<CodexOSProfileID, readonly CodexClientSurface[]> = {
  windows: ['desktop', 'cli'],
  macos: ['desktop', 'cli'],
  linux: ['desktop', 'cli'],
  generic: ['sdk', 'third_party'],
}

const PROFILE_ARCHITECTURES: Record<CodexOSProfileID, readonly CodexArchitecture[]> = {
  windows: ['x86_64', 'arm64'],
  macos: ['arm64', 'x86_64'],
  linux: ['x86_64', 'arm64'],
  generic: [''],
}

const VALIDATION_MESSAGE_KEYS: Record<string, string> = {
  PROXY_INVALID: 'proxyInvalid',
  PROXY_NOT_FOUND: 'proxyNotFound',
  SURFACE_NOT_ALLOWED: 'surfaceNotAllowed',
  ARCHITECTURE_NOT_ALLOWED: 'architectureNotAllowed',
  SLOT_COUNT_OUT_OF_RANGE: 'slotCountOutOfRange',
  SLOT_OVERRIDE_OUT_OF_RANGE: 'slotOverrideOutOfRange',
  DUPLICATE_SLOT_OVERRIDE: 'duplicateSlotOverride',
  BINDING_SCOPE_INVALID: 'bindingScopeInvalid',
  UNSUPPORTED_POLICY_INVALID: 'unsupportedPolicyInvalid',
  OFF_MODE_HAS_PROFILES: 'offModeHasProfiles',
  PROFILE_REQUIRED: 'profileRequired',
  DUPLICATE_PROFILE: 'duplicateProfile',
  AFFINITY_TTL_OUT_OF_RANGE: 'affinityTtlOutOfRange',
  SESSION_SLOT_COUNT_OUT_OF_RANGE: 'sessionSlotCountOutOfRange',
  SESSION_SLOT_COUNT_NOT_APPLICABLE: 'sessionSlotCountNotApplicable',
  DEVICE_SHARED_RESTRICTIONS_INVALID: 'deviceSharedRestrictionsInvalid',
}

export const codexIdentityValidationMessageKey = (code: string): string | null => {
  const suffix = VALIDATION_MESSAGE_KEYS[code]
  return suffix ? `admin.accounts.codexIdentity.validation.${suffix}` : null
}

export const allowedCodexSurfaces = (profileID: CodexOSProfileID): readonly CodexClientSurface[] =>
  PROFILE_SURFACES[profileID]

export const allowedCodexArchitectures = (
  profileID: CodexOSProfileID,
): readonly CodexArchitecture[] => PROFILE_ARCHITECTURES[profileID]

export const cloneCodexIdentityPolicy = (policy: CodexIdentityPolicy): CodexIdentityPolicy =>
  JSON.parse(JSON.stringify(policy)) as CodexIdentityPolicy

export const cloneCodexOSProfile = (profile: CodexOSProfilePolicy): CodexOSProfilePolicy =>
  JSON.parse(JSON.stringify(profile)) as CodexOSProfilePolicy

export const availableCodexIdentityProxyIDs = (
  proxies: readonly CodexIdentityProxyOption[],
): ReadonlySet<number> => new Set(
  proxies
    .filter((proxy) => proxy.status === undefined || proxy.status === 'active')
    .map((proxy) => proxy.id),
)

export const createDefaultCodexOSProfile = (id: CodexOSProfileID): CodexOSProfilePolicy => {
  const defaults: Record<CodexOSProfileID, Pick<CodexOSProfilePolicy, 'canonical_surface' | 'architecture'>> = {
    windows: { canonical_surface: 'desktop', architecture: 'x86_64' },
    macos: { canonical_surface: 'desktop', architecture: 'arm64' },
    linux: { canonical_surface: 'cli', architecture: 'x86_64' },
    generic: { canonical_surface: 'sdk', architecture: '' },
  }
  return {
    os_class: id,
    ...defaults[id],
    slot_count: 1,
  }
}

export const createDefaultCodexIdentityPolicy = (): CodexIdentityPolicy => ({
  mode: 'off',
  binding_scope: 'api_key_os',
  profiles: [],
  session_policy: { mode: 'conversation_isolated' },
  affinity_ttl_seconds: 3600,
  unsupported_policy: 'reject',
})

const isPositiveInteger = (value: number): boolean => Number.isInteger(value) && value > 0

const addIssue = (
  target: CodexIdentityValidationIssue[],
  code: string,
  path: string,
  message: string,
  severity: CodexIdentityValidationIssue['severity'] = 'error',
) => target.push({ code, path, message, severity })

const validateProxyID = (
  proxyID: number | undefined,
  path: string,
  errors: CodexIdentityValidationIssue[],
  availableProxyIDs?: ReadonlySet<number>,
) => {
  if (proxyID === undefined) return
  if (!isPositiveInteger(proxyID)) {
    addIssue(errors, 'PROXY_INVALID', path, 'The selected proxy identifier is invalid.')
  } else if (availableProxyIDs && !availableProxyIDs.has(proxyID)) {
    addIssue(errors, 'PROXY_NOT_FOUND', path, 'The selected proxy is unavailable.')
  }
}

const validateProfile = (
  profile: CodexOSProfilePolicy,
  index: number,
  errors: CodexIdentityValidationIssue[],
  availableProxyIDs?: ReadonlySet<number>,
) => {
  const path = `profiles.${index}`
  if (!PROFILE_SURFACES[profile.os_class]?.includes(profile.canonical_surface)) {
    addIssue(
      errors,
      'SURFACE_NOT_ALLOWED',
      `${path}.canonical_surface`,
      'This client surface is not supported by the selected operating system.',
    )
  }
  if (!PROFILE_ARCHITECTURES[profile.os_class]?.includes(profile.architecture)) {
    addIssue(
      errors,
      'ARCHITECTURE_NOT_ALLOWED',
      `${path}.architecture`,
      'This architecture is not supported by the selected profile.',
    )
  }
  if (
    !Number.isInteger(profile.slot_count) ||
    profile.slot_count < CODEX_DEVICE_SLOT_MIN ||
    profile.slot_count > CODEX_DEVICE_SLOT_MAX
  ) {
    addIssue(
      errors,
      'SLOT_COUNT_OUT_OF_RANGE',
      `${path}.slot_count`,
      `Device slots must be between ${CODEX_DEVICE_SLOT_MIN} and ${CODEX_DEVICE_SLOT_MAX}.`,
    )
  }
  validateProxyID(profile.proxy_id, `${path}.proxy_id`, errors, availableProxyIDs)

  const seenSlots = new Set<number>()
  for (const slot of profile.slots ?? []) {
    const slotPath = `${path}.slots.${slot.index}`
    if (!Number.isInteger(slot.index) || slot.index < 0 || slot.index >= profile.slot_count) {
      addIssue(errors, 'SLOT_OVERRIDE_OUT_OF_RANGE', slotPath, 'This slot override is out of range.')
      continue
    }
    if (seenSlots.has(slot.index)) {
      addIssue(errors, 'DUPLICATE_SLOT_OVERRIDE', slotPath, 'A slot can only have one proxy override.')
      continue
    }
    seenSlots.add(slot.index)
    validateProxyID(slot.proxy_id, `${slotPath}.proxy_id`, errors, availableProxyIDs)
  }
}

export const validateCodexIdentityPolicy = (
  policy: CodexIdentityPolicy,
  options: { availableProxyIDs?: ReadonlySet<number> } = {},
): CodexIdentityValidationResult => {
  const errors: CodexIdentityValidationIssue[] = []
  const warnings: CodexIdentityValidationIssue[] = []
  const profiles = policy.profiles ?? []

  if (policy.binding_scope !== 'api_key_os') {
    addIssue(errors, 'BINDING_SCOPE_INVALID', 'binding_scope', 'Binding scope must be API key plus OS.')
  }
  if (policy.unsupported_policy !== 'reject') {
    addIssue(errors, 'UNSUPPORTED_POLICY_INVALID', 'unsupported_policy', 'Unsupported client profiles must be rejected.')
  }

  if (policy.mode === 'off' && profiles.length > 0) {
    addIssue(errors, 'OFF_MODE_HAS_PROFILES', 'profiles', 'Disabled identity policy cannot define OS profiles.')
  }
  if (policy.mode === 'os_profile_device_pool' && profiles.length === 0) {
    addIssue(errors, 'PROFILE_REQUIRED', 'profiles', 'Enable at least one operating-system profile.')
  }

  const seenProfiles = new Set<CodexOSProfileID>()
  profiles.forEach((profile, index) => {
    if (seenProfiles.has(profile.os_class)) {
      addIssue(errors, 'DUPLICATE_PROFILE', `profiles.${index}.os_class`, 'Each OS profile can appear only once.')
      return
    }
    seenProfiles.add(profile.os_class)
    validateProfile(profile, index, errors, options.availableProxyIDs)
  })

  if (
    !Number.isInteger(policy.affinity_ttl_seconds) ||
    policy.affinity_ttl_seconds < CODEX_AFFINITY_TTL_MIN_SECONDS ||
    policy.affinity_ttl_seconds > CODEX_AFFINITY_TTL_MAX_SECONDS
  ) {
    addIssue(
      errors,
      'AFFINITY_TTL_OUT_OF_RANGE',
      'affinity_ttl_seconds',
      `Affinity must be between ${CODEX_AFFINITY_TTL_MIN_SECONDS / 60} minute and ${CODEX_AFFINITY_TTL_MAX_SECONDS / 3600} hours.`,
    )
  }

  const sessionPolicy = policy.session_policy as CodexSessionPolicy & {
    sessions_per_device?: number
    max_active_conversations_per_slot?: number
    disable_cross_key_continuation?: boolean
  }
  const sessions = sessionPolicy.sessions_per_device
  if (policy.session_policy.mode === 'session_pool') {
    if (
      sessions === undefined ||
      !Number.isInteger(sessions) ||
      sessions < CODEX_SESSION_SLOT_MIN ||
      sessions > CODEX_SESSION_SLOT_MAX
    ) {
      addIssue(
        errors,
        'SESSION_SLOT_COUNT_OUT_OF_RANGE',
        'session_policy.sessions_per_device',
        `Session slots must be between ${CODEX_SESSION_SLOT_MIN} and ${CODEX_SESSION_SLOT_MAX}.`,
      )
    }
  } else if (sessions !== undefined) {
    addIssue(
      errors,
      'SESSION_SLOT_COUNT_NOT_APPLICABLE',
      'session_policy.sessions_per_device',
      'Session slots are only valid for session-pool mode.',
    )
  }

  if (sessionPolicy.mode === 'device_shared') {
    if (
      sessionPolicy.max_active_conversations_per_slot !== 1 ||
      sessionPolicy.disable_cross_key_continuation !== true
    ) {
      addIssue(
        errors,
        'DEVICE_SHARED_RESTRICTIONS_INVALID',
        'session_policy',
        'Device-shared mode requires one active conversation per slot and disables cross-key continuation.',
      )
    }
  } else if (
    sessionPolicy.max_active_conversations_per_slot !== undefined ||
    sessionPolicy.disable_cross_key_continuation !== undefined
  ) {
    addIssue(
      errors,
      'DEVICE_SHARED_RESTRICTIONS_INVALID',
      'session_policy',
      'Device-shared restrictions are only valid in device-shared mode.',
    )
  }

  if (policy.session_policy.mode === 'device_shared') {
    addIssue(
      warnings,
      'DEVICE_SHARED_SESSION_RISK',
      'session_policy.mode',
      'All API keys assigned to a device slot will share one upstream session.',
      'warning',
    )
  } else if (policy.session_policy.mode === 'api_key_shared') {
    addIssue(
      warnings,
      'API_KEY_SHARED_SESSION_RISK',
      'session_policy.mode',
      'All conversations using the same downstream API key will share one upstream session.',
      'warning',
    )
  } else if (policy.session_policy.mode === 'session_pool') {
    addIssue(
      warnings,
      'SESSION_POOL_SHARED_SESSION_RISK',
      'session_policy.mode',
      'Different API keys and conversations can share one upstream session slot.',
      'warning',
    )
  }

  return { valid: errors.length === 0, errors, warnings }
}

export const normalizeCodexIdentityPolicy = (policy: CodexIdentityPolicy): CodexIdentityPolicy => {
  const normalized: CodexIdentityPolicy = {
    mode: policy.mode,
    binding_scope: 'api_key_os',
    session_policy: policy.session_policy.mode === 'session_pool'
      ? {
          mode: 'session_pool',
          sessions_per_device: policy.session_policy.sessions_per_device,
        }
      : policy.session_policy.mode === 'device_shared'
        ? {
            mode: 'device_shared',
            max_active_conversations_per_slot: 1,
            disable_cross_key_continuation: true,
          }
        : { mode: policy.session_policy.mode },
    affinity_ttl_seconds: policy.affinity_ttl_seconds,
    unsupported_policy: 'reject',
    profiles: policy.mode === 'off'
      ? []
      : (policy.profiles ?? []).map((profile) => ({
          os_class: profile.os_class,
          canonical_surface: profile.canonical_surface,
          architecture: profile.os_class === 'generic' ? '' : profile.architecture,
          slot_count: profile.slot_count,
          ...(profile.proxy_id === undefined ? {} : { proxy_id: profile.proxy_id }),
          slots: (profile.slots ?? [])
      .filter((slot) => slot.index >= 0 && slot.index < profile.slot_count)
            .sort((left, right) => left.index - right.index)
            .map((slot) => ({
              index: slot.index,
              ...(slot.proxy_id === undefined ? {} : { proxy_id: slot.proxy_id }),
            })),
        })),
  }
  return normalized
}

export const serializeCodexIdentityPolicy = normalizeCodexIdentityPolicy
