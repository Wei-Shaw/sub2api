<template>
  <fieldset class="space-y-4" :disabled="disabled">
    <div>
      <legend class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ copy('admin.accounts.codexIdentity.sessionPolicy', 'Session policy') }}
      </legend>
      <p :id="`${idPrefix}-description`" class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
        {{ copy('admin.accounts.codexIdentity.sessionPolicyDesc', 'Controls upstream application identity only. HTTP and WebSocket state remain isolated by API key.') }}
      </p>
    </div>

    <div class="grid grid-cols-1 gap-2 lg:grid-cols-2" :aria-describedby="`${idPrefix}-description`">
      <label
        v-for="option in modeOptions"
        :key="option.value"
        class="flex min-w-0 cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors focus-within:ring-2 focus-within:ring-primary-500"
        :class="modelValue.mode === option.value
          ? 'border-primary-500 bg-primary-50/60 dark:border-primary-500 dark:bg-primary-900/15'
          : 'border-gray-200 hover:border-gray-300 dark:border-dark-600 dark:hover:border-dark-500'"
      >
        <input
          type="radio"
          :name="`${idPrefix}-mode`"
          :value="option.value"
          :checked="modelValue.mode === option.value"
          class="mt-0.5 h-4 w-4 shrink-0 border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700"
          @change="updateMode(option.value)"
        />
        <span class="min-w-0">
          <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ option.label }}</span>
          <span class="mt-0.5 block text-xs leading-5 text-gray-500 dark:text-dark-400">{{ option.description }}</span>
        </span>
      </label>
    </div>

    <div
      v-if="modelValue.mode === 'session_pool'"
      class="grid grid-cols-1 gap-3 border-t border-gray-100 pt-4 sm:grid-cols-[minmax(0,1fr)_10rem] sm:items-center dark:border-dark-700"
    >
      <div>
        <label :id="`${idPrefix}-session-slots-label`" class="input-label mb-0">
          {{ copy('admin.accounts.codexIdentity.sessionsPerDevice', 'Sessions per device') }}
        </label>
        <p class="input-hint">
          {{ copy('admin.accounts.codexIdentity.sessionsPerDeviceDesc', 'Each API key and conversation is assigned to one stable slot in the shared pool.') }}
        </p>
      </div>
      <div class="grid h-10 grid-cols-[2.5rem_minmax(2.5rem,1fr)_2.5rem] overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
        <button
          type="button"
          class="flex items-center justify-center border-r border-gray-200 text-gray-600 hover:bg-gray-50 focus:z-10 focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:opacity-40 dark:border-dark-600 dark:text-dark-300 dark:hover:bg-dark-700"
          :disabled="sessionCount <= CODEX_SESSION_SLOT_MIN"
          :aria-label="copy('admin.accounts.codexIdentity.decreaseSessions', 'Decrease session slots')"
          @click="updateSessionCount(sessionCount - 1)"
        >
          <Icon name="minus" size="sm" />
        </button>
        <output
          class="flex items-center justify-center bg-white text-sm font-semibold tabular-nums text-gray-900 dark:bg-dark-800 dark:text-white"
          :aria-labelledby="`${idPrefix}-session-slots-label`"
        >
          {{ sessionCount }}
        </output>
        <button
          type="button"
          class="flex items-center justify-center border-l border-gray-200 text-gray-600 hover:bg-gray-50 focus:z-10 focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:opacity-40 dark:border-dark-600 dark:text-dark-300 dark:hover:bg-dark-700"
          :disabled="sessionCount >= CODEX_SESSION_SLOT_MAX"
          :aria-label="copy('admin.accounts.codexIdentity.increaseSessions', 'Increase session slots')"
          @click="updateSessionCount(sessionCount + 1)"
        >
          <Icon name="plus" size="sm" />
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-3 border-t border-gray-100 pt-4 sm:grid-cols-[minmax(0,1fr)_10rem] sm:items-center dark:border-dark-700">
      <div>
        <label :for="`${idPrefix}-ttl`" class="input-label mb-0">
          {{ copy('admin.accounts.codexIdentity.affinityTtl', 'Conversation affinity') }}
        </label>
        <p :id="`${idPrefix}-ttl-hint`" class="input-hint">
          {{ copy('admin.accounts.codexIdentity.affinityTtlDesc', 'Keep a conversation on the same OAuth account and device slot.') }}
        </p>
      </div>
      <div class="relative">
        <input
          :id="`${idPrefix}-ttl`"
          :value="affinityMinutes"
          type="number"
          min="1"
          max="1440"
          step="1"
          class="input pr-14"
          :aria-describedby="`${idPrefix}-ttl-hint`"
          @change="updateAffinity"
        />
        <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-gray-500 dark:text-dark-400">
          {{ copy('admin.accounts.codexIdentity.minutes', 'min') }}
        </span>
      </div>
    </div>

    <div
      v-if="highRiskMessage"
      class="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
      role="status"
    >
      <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" />
      <span>{{ highRiskMessage }}</span>
    </div>
    <p
      v-if="modelValue.mode === 'device_shared'"
      class="text-xs leading-5 text-gray-500 dark:text-dark-400"
      data-testid="device-shared-restrictions"
    >
      {{ copy('admin.accounts.codexIdentity.deviceSharedRestriction', 'One active conversation per slot; cross-key continuation is disabled.') }}
    </p>
  </fieldset>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import {
  CODEX_SESSION_SLOT_MAX,
  CODEX_SESSION_SLOT_MIN,
  type CodexSessionPolicy,
  type CodexSessionPolicyMode,
} from '@/types/codexIdentity'
import { useCodexIdentityCopy } from './copy'

const props = withDefaults(defineProps<{
  modelValue: CodexSessionPolicy
  affinityTtlSeconds: number
  disabled?: boolean
  idPrefix: string
}>(), {
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: CodexSessionPolicy]
  'update:affinityTtlSeconds': [value: number]
}>()

const copy = useCodexIdentityCopy()
const affinityMinutes = computed(() => Math.max(1, Math.round(props.affinityTtlSeconds / 60)))
const sessionCount = computed(() =>
  props.modelValue.mode === 'session_pool' ? props.modelValue.sessions_per_device : 1,
)
const modeOptions = computed<Array<{
  value: CodexSessionPolicyMode
  label: string
  description: string
}>>(() => [
  {
    value: 'conversation_isolated',
    label: copy('admin.accounts.codexIdentity.conversationIsolated', 'Isolate every conversation'),
    description: copy('admin.accounts.codexIdentity.conversationIsolatedDesc', 'Recommended. Every API key and conversation keeps its own session identity.'),
  },
  {
    value: 'api_key_shared',
    label: copy('admin.accounts.codexIdentity.apiKeyShared', 'Share within an API key'),
    description: copy('admin.accounts.codexIdentity.apiKeySharedDesc', 'Conversations using the same downstream API key share one session.'),
  },
  {
    value: 'session_pool',
    label: copy('admin.accounts.codexIdentity.sessionPool', 'Fixed session pool'),
    description: copy('admin.accounts.codexIdentity.sessionPoolDesc', 'Map API key and conversation pairs into a bounded shared session pool per device.'),
  },
  {
    value: 'device_shared',
    label: copy('admin.accounts.codexIdentity.deviceShared', 'Share by device'),
    description: copy('admin.accounts.codexIdentity.deviceSharedDesc', 'All API keys on a device slot use one upstream session.'),
  },
])

const highRiskMessage = computed(() => {
  if (props.modelValue.mode === 'device_shared') {
    return copy('admin.accounts.codexIdentity.deviceSharedRisk', 'High risk: unrelated API keys will present the same upstream session identity.')
  }
  if (props.modelValue.mode === 'api_key_shared') {
    return copy('admin.accounts.codexIdentity.apiKeySharedRisk', 'Conversations within one API key can affect the same upstream session identity.')
  }
  if (props.modelValue.mode === 'session_pool') {
    return copy('admin.accounts.codexIdentity.sessionPoolRisk', 'Different API keys and conversations can share an upstream session slot. HTTP and WebSocket runtime state remains isolated.')
  }
  return ''
})

const updateMode = (mode: CodexSessionPolicyMode) => {
  if (mode === 'session_pool') {
    emit('update:modelValue', {
      mode,
      sessions_per_device: props.modelValue.mode === 'session_pool'
        ? props.modelValue.sessions_per_device
        : 2,
    })
    return
  }
  if (mode === 'device_shared') {
    emit('update:modelValue', {
      mode,
      max_active_conversations_per_slot: 1,
      disable_cross_key_continuation: true,
    })
    return
  }
  emit('update:modelValue', { mode })
}

const updateSessionCount = (count: number) => {
  emit('update:modelValue', {
    mode: 'session_pool',
    sessions_per_device: Math.max(CODEX_SESSION_SLOT_MIN, Math.min(CODEX_SESSION_SLOT_MAX, count)),
  })
}

const updateAffinity = (event: Event) => {
  const value = Number((event.target as HTMLInputElement).value)
  if (!Number.isFinite(value)) return
  emit('update:affinityTtlSeconds', Math.round(value * 60))
}
</script>
