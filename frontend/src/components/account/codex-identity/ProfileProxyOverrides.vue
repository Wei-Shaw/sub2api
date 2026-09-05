<template>
  <div class="space-y-3 border-t border-gray-100 pt-3 dark:border-dark-700">
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(12rem,18rem)] sm:items-center">
      <div class="min-w-0">
        <label :id="`${idPrefix}-profile-proxy-label`" class="input-label mb-0">
          {{ copy('admin.accounts.codexIdentity.profileProxy', 'Profile proxy') }}
        </label>
        <p :id="`${idPrefix}-profile-proxy-hint`" class="input-hint">
          {{ accountProxyHint }}
        </p>
      </div>
      <Select
        :id="`${idPrefix}-profile-proxy`"
        :model-value="profileProxySelection"
        :options="proxyOptions"
        :disabled="disabled"
        :aria-label="copy('admin.accounts.codexIdentity.profileProxy', 'Profile proxy')"
        :aria-describedby="`${idPrefix}-profile-proxy-hint`"
        searchable="auto"
        @update:model-value="updateProfileProxy"
      />
    </div>

    <details class="group" data-testid="slot-proxy-overrides">
      <summary
        class="flex cursor-pointer list-none items-center justify-between gap-3 rounded px-2 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-200 dark:hover:bg-dark-700"
      >
        <span>{{ copy('admin.accounts.codexIdentity.slotOverrides', 'Per-slot proxy overrides and versions') }}</span>
        <Icon name="chevronDown" size="sm" class="transition-transform group-open:rotate-180" />
      </summary>
      <div class="mt-2 divide-y divide-gray-100 border-y border-gray-100 dark:divide-dark-700 dark:border-dark-700">
        <div
          v-for="slotIndex in slotIndexes"
          :key="slotIndex"
          class="grid grid-cols-1 gap-3 py-3 sm:grid-cols-[minmax(0,1fr)_minmax(12rem,18rem)_minmax(15rem,20rem)] sm:items-start"
        >
          <div>
            <label :id="`${idPrefix}-slot-${slotIndex}-label`" class="text-sm font-medium text-gray-700 dark:text-dark-200">
              {{ copy('admin.accounts.codexIdentity.deviceSlot', 'Device slot') }} {{ slotIndex + 1 }}
            </label>
            <p class="text-xs text-gray-500 dark:text-dark-400">
              {{ copy('admin.accounts.codexIdentity.slotProxyHint', 'Inherits the profile proxy unless overridden.') }}
            </p>
          </div>
          <Select
            :id="`${idPrefix}-slot-${slotIndex}-proxy`"
            :model-value="slotProxySelection(slotIndex)"
            :options="slotProxyOptions"
            :disabled="disabled"
            :aria-label="`${copy('admin.accounts.codexIdentity.deviceSlot', 'Device slot')} ${slotIndex + 1}`"
            searchable="auto"
            @update:model-value="updateSlotProxy(slotIndex, $event)"
          />
          <div class="min-w-0 space-y-2">
            <label :id="`${idPrefix}-slot-${slotIndex}-client-version-label`" class="input-label mb-0">
              {{ copy('admin.accounts.codexIdentity.clientVersion', 'Codex client version') }}
            </label>
            <Select
              :id="`${idPrefix}-slot-${slotIndex}-client-version-mode`"
              :model-value="slotClientVersionMode(slotIndex)"
              :options="clientVersionModeOptions"
              :disabled="disabled"
              :aria-label="`${copy('admin.accounts.codexIdentity.clientVersion', 'Codex client version')} ${slotIndex + 1}`"
              :aria-describedby="`${idPrefix}-slot-${slotIndex}-client-version-hint`"
              :data-testid="`slot-${slotIndex}-client-version-mode`"
              @update:model-value="updateSlotClientVersionMode(slotIndex, $event)"
            />
            <input
              v-if="slotClientVersionMode(slotIndex) === 'pinned'"
              :id="`${idPrefix}-slot-${slotIndex}-client-version`"
              :value="slotClientVersion(slotIndex)"
              type="text"
              inputmode="decimal"
              maxlength="64"
              class="input font-mono text-sm"
              :placeholder="copy('admin.accounts.codexIdentity.clientVersionPlaceholder', 'e.g. 0.146.0')"
              :aria-label="`${copy('admin.accounts.codexIdentity.fixedClientVersion', 'Fixed Codex client version')} ${slotIndex + 1}`"
              :data-testid="`slot-${slotIndex}-client-version`"
              @input="updateSlotClientVersionValue(slotIndex, ($event.target as HTMLInputElement).value)"
            />
            <p :id="`${idPrefix}-slot-${slotIndex}-client-version-hint`" class="text-xs text-gray-500 dark:text-dark-400">
              {{ slotClientVersionMode(slotIndex) === 'pinned'
                ? copy('admin.accounts.codexIdentity.clientVersionPinnedHint', 'Sets the version declared upstream and updates the User-Agent version. It does not install Codex, update the Desktop app, change the model, or extract a new release fingerprint.')
                : copy('admin.accounts.codexIdentity.clientVersionInheritHint', 'Follow the deployment Codex client version automatically.') }}
            </p>
          </div>
        </div>
      </div>
    </details>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type {
  CodexIdentityProxyOption,
  CodexOSProfilePolicy,
  CodexClientVersionMode,
  CodexProxyMode,
} from '@/types/codexIdentity'
import { useCodexIdentityCopy } from './copy'
import { cloneCodexOSProfile } from '@/utils/codexIdentityValidation'

const props = withDefaults(defineProps<{
  modelValue: CodexOSProfilePolicy
  proxies?: readonly CodexIdentityProxyOption[]
  accountProxyId?: number | null
  templateContext?: boolean
  disabled?: boolean
  idPrefix: string
}>(), {
  proxies: () => [],
  accountProxyId: null,
  templateContext: false,
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: CodexOSProfilePolicy]
}>()

const copy = useCodexIdentityCopy()
const activeProxies = computed(() => props.proxies.filter((proxy) => proxy.status === undefined || proxy.status === 'active'))
const proxyLabel = (proxy: CodexIdentityProxyOption) => {
  const endpoint = proxy.protocol && proxy.host && proxy.port
    ? ` (${proxy.protocol}://${proxy.host}:${proxy.port})`
    : ''
  return `${proxy.name}${endpoint}`
}

const accountProxyHint = computed(() => {
  if (props.templateContext) {
    return copy('admin.accounts.codexIdentity.templateProxyInherit', 'Inherits each assigned account connection at runtime.')
  }
  if (!props.accountProxyId) {
    return copy('admin.accounts.codexIdentity.accountProxyDirect', 'Inherit the account connection (currently direct).')
  }
  const proxy = props.proxies.find((item) => item.id === props.accountProxyId)
  const name = proxy?.name ?? `#${props.accountProxyId}`
  return `${copy('admin.accounts.codexIdentity.accountProxyInherit', 'Inherit account proxy')}: ${name}`
})

const normalizedProxyMode = (mode: CodexProxyMode | undefined, proxyID?: number): CodexProxyMode =>
  mode || (proxyID === undefined ? 'inherit' : 'proxy')

const profileProxySelection = computed<string | number>(() => {
  const mode = normalizedProxyMode(props.modelValue.proxy_mode, props.modelValue.proxy_id)
  return mode === 'proxy' && props.modelValue.proxy_id !== undefined
    ? props.modelValue.proxy_id
    : mode
})

const proxyOptions = computed(() => [
  { value: 'inherit', label: copy('admin.accounts.codexIdentity.inheritAccountProxy', 'Inherit account connection') },
  { value: 'direct', label: copy('admin.accounts.codexIdentity.directConnection', 'Direct connection') },
  ...props.proxies.map((proxy) => ({
    value: proxy.id,
    label: proxyLabel(proxy),
    disabled: proxy.status === 'inactive' || proxy.status === 'expired',
  })),
])

const slotProxyOptions = computed(() => [
  { value: 'inherit', label: copy('admin.accounts.codexIdentity.inheritProfileProxy', 'Inherit profile connection') },
  { value: 'direct', label: copy('admin.accounts.codexIdentity.directConnection', 'Direct connection') },
  ...props.proxies.map((proxy) => ({
    value: proxy.id,
    label: proxyLabel(proxy),
    disabled: proxy.status === 'inactive' || proxy.status === 'expired',
  })),
])

const clientVersionModeOptions = computed(() => [
  {
    value: 'inherit',
    label: copy('admin.accounts.codexIdentity.clientVersionInherit', 'Automatic (recommended)'),
  },
  {
    value: 'pinned',
    label: copy('admin.accounts.codexIdentity.clientVersionPinned', 'Fixed version'),
  },
])

const slotIndexes = computed(() => Array.from({ length: props.modelValue.slot_count }, (_, index) => index))

const updateProfileProxy = (value: string | number | boolean | null) => {
  const next = cloneCodexOSProfile(props.modelValue)
  if (typeof value === 'number' && activeProxies.value.some((proxy) => proxy.id === value)) {
    next.proxy_mode = 'proxy'
    next.proxy_id = value
  } else if (value === 'direct') {
    next.proxy_mode = 'direct'
    delete next.proxy_id
  } else {
    next.proxy_mode = 'inherit'
    delete next.proxy_id
  }
  emit('update:modelValue', next)
}

const slotProxySelection = (slotIndex: number): string | number => {
  const slot = props.modelValue.slots?.find((item) => item.index === slotIndex)
  const mode = normalizedProxyMode(slot?.proxy_mode, slot?.proxy_id)
  return mode === 'proxy' && slot?.proxy_id !== undefined ? slot.proxy_id : mode
}

const slotClientVersionMode = (slotIndex: number): CodexClientVersionMode =>
  props.modelValue.slots?.find((item) => item.index === slotIndex)?.client_version_mode ?? 'inherit'

const slotClientVersion = (slotIndex: number): string =>
  props.modelValue.slots?.find((item) => item.index === slotIndex)?.client_version ?? ''

const updateSlotProxy = (slotIndex: number, value: string | number | boolean | null) => {
  const next = cloneCodexOSProfile(props.modelValue)
  const existing = next.slots?.find((slot) => slot.index === slotIndex)
  const slots = (next.slots ?? []).filter((slot) => slot.index !== slotIndex)
  const clientVersionMode = existing?.client_version_mode ?? 'inherit'
  if (typeof value === 'number' && activeProxies.value.some((proxy) => proxy.id === value)) {
    slots.push({
      index: slotIndex,
      proxy_mode: 'proxy',
      proxy_id: value,
      client_version_mode: clientVersionMode,
      ...(clientVersionMode === 'pinned' && existing?.client_version ? { client_version: existing.client_version } : {}),
    })
  } else if (value === 'direct') {
    slots.push({
      index: slotIndex,
      proxy_mode: 'direct',
      client_version_mode: clientVersionMode,
      ...(clientVersionMode === 'pinned' && existing?.client_version ? { client_version: existing.client_version } : {}),
    })
  } else {
    slots.push({
      index: slotIndex,
      proxy_mode: 'inherit',
      client_version_mode: clientVersionMode,
      ...(clientVersionMode === 'pinned' && existing?.client_version ? { client_version: existing.client_version } : {}),
    })
  }
  next.slots = slots.sort((left, right) => left.index - right.index)
  emit('update:modelValue', next)
}

const updateSlotClientVersionMode = (slotIndex: number, value: string | number | boolean | null) => {
  const next = cloneCodexOSProfile(props.modelValue)
  const existing = next.slots?.find((slot) => slot.index === slotIndex)
  const slots = (next.slots ?? []).filter((slot) => slot.index !== slotIndex)
  const mode: CodexClientVersionMode = value === 'pinned' ? 'pinned' : 'inherit'
  slots.push({
    index: slotIndex,
    proxy_mode: existing?.proxy_mode ?? 'inherit',
    ...(existing?.proxy_id !== undefined ? { proxy_id: existing.proxy_id } : {}),
    client_version_mode: mode,
    ...(mode === 'pinned' && existing?.client_version ? { client_version: existing.client_version } : {}),
  })
  next.slots = slots.sort((left, right) => left.index - right.index)
  emit('update:modelValue', next)
}

const updateSlotClientVersionValue = (slotIndex: number, value: string) => {
  const next = cloneCodexOSProfile(props.modelValue)
  const existing = next.slots?.find((slot) => slot.index === slotIndex)
  const slots = (next.slots ?? []).filter((slot) => slot.index !== slotIndex)
  slots.push({
    index: slotIndex,
    proxy_mode: existing?.proxy_mode ?? 'inherit',
    ...(existing?.proxy_id !== undefined ? { proxy_id: existing.proxy_id } : {}),
    client_version_mode: 'pinned',
    client_version: value,
  })
  next.slots = slots.sort((left, right) => left.index - right.index)
  emit('update:modelValue', next)
}
</script>
