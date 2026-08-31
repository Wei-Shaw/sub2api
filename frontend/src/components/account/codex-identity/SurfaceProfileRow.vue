<template>
  <section class="py-3" :data-testid="`codex-profile-${profileId}-${surface}`">
    <div class="flex items-center justify-between gap-4">
      <div class="min-w-0">
        <label :for="`${idPrefix}-enabled`" class="text-sm font-medium text-gray-800 dark:text-dark-100">
          {{ surfaceLabel }}
        </label>
        <p :id="`${idPrefix}-description`" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
          {{ surfaceDescription }}
        </p>
      </div>
      <button
        :id="`${idPrefix}-enabled`"
        type="button"
        role="switch"
        class="relative inline-flex h-6 w-11 shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
        :class="enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'"
        :aria-checked="enabled"
        :disabled="disabled"
        :aria-describedby="`${idPrefix}-description`"
        @click="emit('update:enabled', !enabled)"
      >
        <span
          aria-hidden="true"
          class="pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transition-transform"
          :class="enabled ? 'translate-x-5' : 'translate-x-0'"
        />
      </button>
    </div>

    <div v-if="enabled" class="mt-3 space-y-4 pl-0 sm:pl-3">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div class="min-w-0">
          <label :id="`${idPrefix}-architecture-label`" class="input-label">
            {{ copy('admin.accounts.codexIdentity.architecture', 'Architecture') }}
          </label>
          <Select
            v-if="profileId !== 'generic'"
            :id="`${idPrefix}-architecture`"
            :model-value="modelValue.architecture"
            :options="architectureOptions"
            :disabled="disabled"
            :aria-label="copy('admin.accounts.codexIdentity.architecture', 'Architecture')"
            @update:model-value="updateArchitecture"
          />
          <div
            v-else
            class="flex h-10 items-center rounded-lg border border-gray-200 bg-gray-50 px-3 text-sm text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-400"
          >
            {{ copy('admin.accounts.codexIdentity.notApplicable', 'Not applicable') }}
          </div>
        </div>

        <div class="min-w-0">
          <label :id="`${idPrefix}-slots-label`" class="input-label">
            {{ copy('admin.accounts.codexIdentity.deviceSlots', 'Device slots') }}
          </label>
          <div class="grid h-10 grid-cols-[2.5rem_minmax(2.5rem,1fr)_2.5rem] overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
            <button
              type="button"
              class="flex items-center justify-center border-r border-gray-200 text-gray-600 hover:bg-gray-50 focus:z-10 focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:text-dark-300 dark:hover:bg-dark-700"
              :disabled="disabled || modelValue.slot_count <= CODEX_DEVICE_SLOT_MIN"
              :aria-label="`${copy('admin.accounts.codexIdentity.decreaseSlots', 'Decrease device slots')} - ${surfaceLabel}`"
              @click="updateSlotCount(modelValue.slot_count - 1)"
            >
              <Icon name="minus" size="sm" />
            </button>
            <output
              :id="`${idPrefix}-slots`"
              class="flex items-center justify-center bg-white text-sm font-semibold tabular-nums text-gray-900 dark:bg-dark-800 dark:text-white"
              :aria-labelledby="`${idPrefix}-slots-label`"
            >
              {{ modelValue.slot_count }}
            </output>
            <button
              type="button"
              class="flex items-center justify-center border-l border-gray-200 text-gray-600 hover:bg-gray-50 focus:z-10 focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:text-dark-300 dark:hover:bg-dark-700"
              :disabled="disabled || modelValue.slot_count >= CODEX_DEVICE_SLOT_MAX"
              :aria-label="`${copy('admin.accounts.codexIdentity.increaseSlots', 'Increase device slots')} - ${surfaceLabel}`"
              @click="updateSlotCount(modelValue.slot_count + 1)"
            >
              <Icon name="plus" size="sm" />
            </button>
          </div>
        </div>
      </div>

      <ProfileProxyOverrides
        :model-value="modelValue"
        :proxies="proxies"
        :account-proxy-id="accountProxyId"
        :template-context="templateContext"
        :disabled="disabled"
        :id-prefix="`${idPrefix}-proxy`"
        @update:model-value="emit('update:modelValue', $event)"
      />

      <ul v-if="issues.length" :id="`${idPrefix}-errors`" class="space-y-1" role="alert">
        <li v-for="issue in issues" :key="`${issue.code}:${issue.path}`" class="text-xs text-red-600 dark:text-red-400">
          {{ validationMessage(issue) }}
        </li>
      </ul>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  CODEX_DEVICE_SLOT_MAX,
  CODEX_DEVICE_SLOT_MIN,
  type CodexArchitecture,
  type CodexClientSurface,
  type CodexIdentityProxyOption,
  type CodexIdentityValidationIssue,
  type CodexOSProfileID,
  type CodexOSProfilePolicy,
} from '@/types/codexIdentity'
import {
  allowedCodexArchitectures,
  cloneCodexOSProfile,
  codexIdentityValidationMessageKey,
} from '@/utils/codexIdentityValidation'
import ProfileProxyOverrides from './ProfileProxyOverrides.vue'
import { useCodexIdentityCopy } from './copy'

const props = withDefaults(defineProps<{
  profileId: CodexOSProfileID
  surface: CodexClientSurface
  modelValue: CodexOSProfilePolicy
  enabled: boolean
  proxies?: readonly CodexIdentityProxyOption[]
  accountProxyId?: number | null
  templateContext?: boolean
  disabled?: boolean
  issues?: readonly CodexIdentityValidationIssue[]
  idPrefix: string
}>(), {
  proxies: () => [],
  accountProxyId: null,
  templateContext: false,
  disabled: false,
  issues: () => [],
})

const emit = defineEmits<{
  'update:modelValue': [value: CodexOSProfilePolicy]
  'update:enabled': [value: boolean]
}>()

const copy = useCodexIdentityCopy()
const surfaceLabel = computed(() => ({
  desktop: copy('admin.accounts.codexIdentity.desktop', 'Desktop'),
  cli: copy('admin.accounts.codexIdentity.cli', 'CLI'),
  sdk: copy('admin.accounts.codexIdentity.sdk', 'SDK'),
  third_party: copy('admin.accounts.codexIdentity.thirdParty', 'Third-party'),
})[props.surface])
const surfaceDescription = computed(() => ({
  desktop: copy('admin.accounts.codexIdentity.desktopDesc', 'Use a dedicated Desktop identity and device pool.'),
  cli: copy('admin.accounts.codexIdentity.cliDesc', 'Use a dedicated CLI identity and device pool.'),
  sdk: copy('admin.accounts.codexIdentity.sdkDesc', 'Use a dedicated SDK identity and device pool.'),
  third_party: copy('admin.accounts.codexIdentity.thirdPartyDesc', 'Use a dedicated third-party identity and device pool.'),
})[props.surface])
const architectureOptions = computed(() =>
  allowedCodexArchitectures(props.profileId).map((value) => ({ value, label: value })),
)

const updateArchitecture = (value: string | number | boolean | null) => {
  if (value !== 'x86_64' && value !== 'arm64') return
  emit('update:modelValue', {
    ...cloneCodexOSProfile(props.modelValue),
    architecture: value as CodexArchitecture,
  })
}
const updateSlotCount = (count: number) => {
  const slotCount = Math.max(CODEX_DEVICE_SLOT_MIN, Math.min(CODEX_DEVICE_SLOT_MAX, count))
  const next = cloneCodexOSProfile(props.modelValue)
  next.slot_count = slotCount
  next.slots = (next.slots ?? []).filter((slot) => slot.index < slotCount)
  emit('update:modelValue', next)
}
const validationMessage = (issue: CodexIdentityValidationIssue): string => {
  const key = codexIdentityValidationMessageKey(issue.code)
  return key ? copy(key, issue.message) : issue.message
}
</script>
