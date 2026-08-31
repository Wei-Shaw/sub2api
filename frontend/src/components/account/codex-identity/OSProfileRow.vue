<template>
  <li class="px-3 py-4 sm:px-4" :data-testid="`codex-profile-${profileId}`">
    <div class="min-w-0">
      <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ profileName }}</h4>
      <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ profileDescription }}</p>
    </div>

    <div class="mt-3 divide-y divide-gray-100 border-y border-gray-100 dark:divide-dark-700 dark:border-dark-700">
      <SurfaceProfileRow
        v-for="surface in surfaces"
        :key="surface"
        :profile-id="profileId"
        :surface="surface"
        :model-value="profileFor(surface)"
        :enabled="profileEnabled(surface)"
        :proxies="proxies"
        :account-proxy-id="accountProxyId"
        :template-context="templateContext"
        :disabled="disabled"
        :issues="issuesBySurface[surface] ?? []"
        :id-prefix="`${idPrefix}-${surface}`"
        @update:model-value="updateProfile(surface, $event)"
        @update:enabled="toggleProfile(surface, $event)"
      />
    </div>
  </li>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type {
  CodexClientSurface,
  CodexIdentityProxyOption,
  CodexIdentityValidationIssue,
  CodexOSProfileID,
  CodexOSProfilePolicy,
} from '@/types/codexIdentity'
import {
  allowedCodexSurfaces,
  cloneCodexOSProfile,
  createDefaultCodexOSProfile,
} from '@/utils/codexIdentityValidation'
import SurfaceProfileRow from './SurfaceProfileRow.vue'
import { useCodexIdentityCopy } from './copy'

const props = withDefaults(defineProps<{
  profileId: CodexOSProfileID
  modelValue: readonly CodexOSProfilePolicy[]
  proxies?: readonly CodexIdentityProxyOption[]
  accountProxyId?: number | null
  templateContext?: boolean
  disabled?: boolean
  issuesBySurface?: Partial<Record<CodexClientSurface, readonly CodexIdentityValidationIssue[]>>
  idPrefix: string
}>(), {
  proxies: () => [],
  accountProxyId: null,
  templateContext: false,
  disabled: false,
  issuesBySurface: () => ({}),
})

const emit = defineEmits<{
  'update:profile': [value: {
    surface: CodexClientSurface
    profile: CodexOSProfilePolicy | null
  }]
}>()

const copy = useCodexIdentityCopy()
const profileName = computed(() => ({
  windows: copy('admin.accounts.codexIdentity.windows', 'Windows'),
  macos: copy('admin.accounts.codexIdentity.macos', 'macOS'),
  linux: copy('admin.accounts.codexIdentity.linux', 'Linux'),
  generic: copy('admin.accounts.codexIdentity.generic', 'Generic SDK / third-party'),
})[props.profileId])
const profileDescription = computed(() => ({
  windows: copy('admin.accounts.codexIdentity.windowsDesc', 'Configure Windows Desktop and CLI independently.'),
  macos: copy('admin.accounts.codexIdentity.macosDesc', 'Configure macOS Desktop and CLI independently.'),
  linux: copy('admin.accounts.codexIdentity.linuxDesc', 'Configure Linux Desktop and CLI independently.'),
  generic: copy('admin.accounts.codexIdentity.genericDesc', 'Configure SDK and third-party clients independently.'),
})[props.profileId])

const surfaces = computed(() => allowedCodexSurfaces(props.profileId))
const profileEnabled = (surface: CodexClientSurface): boolean =>
  props.modelValue.some((profile) => profile.canonical_surface === surface)
const profileFor = (surface: CodexClientSurface): CodexOSProfilePolicy =>
  cloneCodexOSProfile(
    props.modelValue.find((profile) => profile.canonical_surface === surface)
      ?? createDefaultCodexOSProfile(props.profileId, surface),
  )
const updateProfile = (surface: CodexClientSurface, profile: CodexOSProfilePolicy) => {
  emit('update:profile', { surface, profile })
}
const toggleProfile = (surface: CodexClientSurface, enabled: boolean) => {
  emit('update:profile', { surface, profile: enabled ? profileFor(surface) : null })
}
</script>
