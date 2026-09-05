import {
  CODEX_AFFINITY_TTL_MAX_SECONDS,
  CODEX_AFFINITY_TTL_MIN_SECONDS,
  CODEX_CLIENT_VERSION_MAX_LENGTH,
  CODEX_CLIENT_VERSION_MIN,
  CODEX_DEVICE_SLOT_MAX,
  CODEX_DEVICE_SLOT_MIN,
  CODEX_OS_PROFILE_IDS,
  CODEX_SESSION_SLOT_MAX,
  CODEX_SESSION_SLOT_MIN,
  type CodexArchitecture,
  type CodexClientSurface,
  type CodexClientVersionMode,
  type CodexIdentityPolicy,
  type CodexIdentityProxyOption,
  type CodexIdentityValidationIssue,
  type CodexIdentityValidationResult,
  type CodexOSProfileID,
  type CodexOSProfilePolicy,
  type CodexProxyMode,
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
  PROXY_MODE_INVALID: 'proxyModeInvalid',
  PROXY_REQUIRED: 'proxyRequired',
  PROXY_NOT_ALLOWED: 'proxyNotAllowed',
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
  CLIENT_VERSION_MODE_INVALID: 'clientVersionModeInvalid',
  CLIENT_VERSION_REQUIRED: 'clientVersionRequired',
  CLIENT_VERSION_INVALID: 'clientVersionInvalid',
  CLIENT_VERSION_TOO_OLD: 'clientVersionTooOld',
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

export const createDefaultCodexOSProfile = (
  id: CodexOSProfileID,
  surface?: CodexClientSurface,
): CodexOSProfilePolicy => {
  const defaults: Record<CodexOSProfileID, Pick<CodexOSProfilePolicy, 'canonical_surface' | 'architecture'>> = {
    windows: { canonical_surface: 'desktop', architecture: 'x86_64' },
    macos: { canonical_surface: 'desktop', architecture: 'arm64' },
    linux: { canonical_surface: 'cli', architecture: 'x86_64' },
    generic: { canonical_surface: 'sdk', architecture: '' },
  }
  const canonicalSurface = surface && PROFILE_SURFACES[id].includes(surface)
    ? surface
    : defaults[id].canonical_surface
  return {
    os_class: id,
    ...defaults[id],
    canonical_surface: canonicalSurface,
    slot_count: 1,
    proxy_mode: 'inherit',
    slots: [{ index: 0, proxy_mode: 'inherit', client_version_mode: 'inherit' }],
  }
}

export const createDefaultCodexIdentityPolicy = (): CodexIdentityPolicy => ({
  mode: 'off',
  binding_scope: 'api_key_os_surface',
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

const CODEX_PROXY_MODES: readonly CodexProxyMode[] = ['inherit', 'proxy', 'direct']
const CODEX_CLIENT_VERSION_MODES: readonly CodexClientVersionMode[] = ['inherit', 'pinned']
const CODEX_CLIENT_VERSION_PATTERN = /^[0-9]+(\.[0-9]+){1,3}(-[0-9A-Za-z.]+)?$/

const compareCodexVersions = (left: string, right: string): number => {
  const parse = (value: string) => value.split('-')[0].split('.').map((part) => Number(part) || 0)
  const leftParts = parse(left)
  const rightParts = parse(right)
  for (let index = 0; index < Math.max(leftParts.length, rightParts.length); index += 1) {
    const difference = (leftParts[index] ?? 0) - (rightParts[index] ?? 0)
    if (difference !== 0) return difference
  }
  return 0
}

export const normalizeCodexClientVersion = (value: string | undefined): string => {
  const trimmed = value?.trim() ?? ''
  if (!trimmed || trimmed.length > CODEX_CLIENT_VERSION_MAX_LENGTH || !CODEX_CLIENT_VERSION_PATTERN.test(trimmed)) {
    return ''
  }
  return trimmed
}

const inferredCodexProxyMode = (
  mode: CodexProxyMode | '' | undefined,
  proxyID: number | undefined,
): CodexProxyMode => mode || (proxyID === undefined ? 'inherit' : 'proxy')

const validateProxyRoute = (
  modeInput: CodexProxyMode | '' | undefined,
  proxyID: number | undefined,
  path: string,
  errors: CodexIdentityValidationIssue[],
  availableProxyIDs?: ReadonlySet<number>,
) => {
  const mode = inferredCodexProxyMode(modeInput, proxyID)
  if (!CODEX_PROXY_MODES.includes(mode)) {
    addIssue(errors, 'PROXY_MODE_INVALID', `${path}.proxy_mode`, 'The selected proxy mode is invalid.')
    return
  }
  if (mode === 'proxy') {
    if (proxyID === undefined) {
      addIssue(errors, 'PROXY_REQUIRED', `${path}.proxy_id`, 'Select a proxy for explicit proxy mode.')
      return
    }
    validateProxyID(proxyID, `${path}.proxy_id`, errors, availableProxyIDs)
    return
  }
  if (proxyID !== undefined) {
    addIssue(errors, 'PROXY_NOT_ALLOWED', `${path}.proxy_id`, 'Proxy ID is only valid in explicit proxy mode.')
  }
}

const validateClientVersion = (
  modeInput: CodexClientVersionMode | '' | undefined,
  versionInput: string | undefined,
  path: string,
  errors: CodexIdentityValidationIssue[],
) => {
  const mode = modeInput || 'inherit'
  const version = versionInput?.trim() ?? ''
  if (!CODEX_CLIENT_VERSION_MODES.includes(mode)) {
    addIssue(errors, 'CLIENT_VERSION_MODE_INVALID', `${path}.client_version_mode`, 'The selected Codex client version mode is invalid.')
    return
  }
  if (mode === 'inherit') {
    if (version !== '') {
      addIssue(errors, 'CLIENT_VERSION_INVALID', `${path}.client_version`, 'Automatic Codex client version mode cannot include a fixed version.')
    }
    return
  }
  if (!version) {
    addIssue(errors, 'CLIENT_VERSION_REQUIRED', `${path}.client_version`, 'Enter a Codex client version when fixed version mode is selected.')
    return
  }
  const normalized = normalizeCodexClientVersion(version)
  if (!normalized) {
    addIssue(errors, 'CLIENT_VERSION_INVALID', `${path}.client_version`, 'Enter a valid Codex client version such as 0.146.0.')
    return
  }
  if (compareCodexVersions(normalized, CODEX_CLIENT_VERSION_MIN) < 0) {
    addIssue(errors, 'CLIENT_VERSION_TOO_OLD', `${path}.client_version`, `Codex client version must be at least ${CODEX_CLIENT_VERSION_MIN}.`)
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
  validateProxyRoute(profile.proxy_mode, profile.proxy_id, path, errors, availableProxyIDs)

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
    validateProxyRoute(slot.proxy_mode, slot.proxy_id, slotPath, errors, availableProxyIDs)
    validateClientVersion(slot.client_version_mode, slot.client_version, slotPath, errors)
  }
}

export const validateCodexIdentityPolicy = (
  policy: CodexIdentityPolicy,
  options: { availableProxyIDs?: ReadonlySet<number> } = {},
): CodexIdentityValidationResult => {
  const errors: CodexIdentityValidationIssue[] = []
  const warnings: CodexIdentityValidationIssue[] = []
  const profiles = policy.profiles ?? []

  if (policy.binding_scope !== 'api_key_os_surface' && policy.binding_scope !== 'api_key_os') {
    addIssue(errors, 'BINDING_SCOPE_INVALID', 'binding_scope', 'Binding scope must be API key plus OS and client surface.')
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

  const seenProfiles = new Set<string>()
  profiles.forEach((profile, index) => {
    const profileKey = `${profile.os_class}:${profile.canonical_surface}`
    if (seenProfiles.has(profileKey)) {
      addIssue(
        errors,
        'DUPLICATE_PROFILE',
        `profiles.${index}.canonical_surface`,
        'Each operating-system and client-surface combination can appear only once.',
      )
      return
    }
    seenProfiles.add(profileKey)
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
      !Number.isInteger(sessionPolicy.max_active_conversations_per_slot ?? 0) ||
      (sessionPolicy.max_active_conversations_per_slot ?? 0) < 0 ||
      (sessionPolicy.max_active_conversations_per_slot ?? 0) > 1000 ||
      sessionPolicy.disable_cross_key_continuation !== true
    ) {
      addIssue(
        errors,
        'DEVICE_SHARED_RESTRICTIONS_INVALID',
        'session_policy',
        'Device-shared mode requires a slot concurrency limit between 0 and 1000 and disables cross-key continuation.',
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
    binding_scope: 'api_key_os_surface',
    session_policy: policy.session_policy.mode === 'session_pool'
      ? {
          mode: 'session_pool',
          sessions_per_device: policy.session_policy.sessions_per_device,
        }
      : policy.session_policy.mode === 'device_shared'
        ? {
            mode: 'device_shared',
            max_active_conversations_per_slot: policy.session_policy.max_active_conversations_per_slot ?? 0,
            disable_cross_key_continuation: true,
          }
        : { mode: policy.session_policy.mode },
    affinity_ttl_seconds: policy.affinity_ttl_seconds,
    unsupported_policy: 'reject',
    profiles: policy.mode === 'off'
      ? []
      : (policy.profiles ?? []).map((profile) => {
          const proxyMode = inferredCodexProxyMode(profile.proxy_mode, profile.proxy_id)
          return {
            os_class: profile.os_class,
            canonical_surface: profile.canonical_surface,
            architecture: profile.os_class === 'generic' ? '' : profile.architecture,
            slot_count: profile.slot_count,
            proxy_mode: proxyMode,
            ...(proxyMode === 'proxy' && profile.proxy_id !== undefined
              ? { proxy_id: profile.proxy_id }
              : {}),
            slots: (profile.slots ?? [])
              .filter((slot) => slot.index >= 0 && slot.index < profile.slot_count)
              .sort((left, right) => left.index - right.index)
              .map((slot) => {
                const slotProxyMode = inferredCodexProxyMode(slot.proxy_mode, slot.proxy_id)
                const clientVersionMode = slot.client_version_mode || 'inherit'
                const clientVersion = clientVersionMode === 'pinned'
                  ? normalizeCodexClientVersion(slot.client_version)
                  : ''
                return {
                  index: slot.index,
                  proxy_mode: slotProxyMode,
                  ...(slotProxyMode === 'proxy' && slot.proxy_id !== undefined
                    ? { proxy_id: slot.proxy_id }
                    : {}),
                  client_version_mode: clientVersionMode,
                  ...(clientVersionMode === 'pinned' && clientVersion ? { client_version: clientVersion } : {}),
                }
              }),
          }
        }),
  }
  normalized.profiles?.sort((left, right) => {
    const osOrder = CODEX_OS_PROFILE_IDS.indexOf(left.os_class)
      - CODEX_OS_PROFILE_IDS.indexOf(right.os_class)
    if (osOrder !== 0) return osOrder
    return PROFILE_SURFACES[left.os_class].indexOf(left.canonical_surface)
      - PROFILE_SURFACES[right.os_class].indexOf(right.canonical_surface)
  })
  return normalized
}

export const serializeCodexIdentityPolicy = normalizeCodexIdentityPolicy
