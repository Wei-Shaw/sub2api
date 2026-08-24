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
        :model-value="modelValue.proxy_id ?? null"
        :options="proxyOptions"
        :disabled="disabled"
        :aria-label="copy('admin.accounts.codexIdentity.profileProxy', 'Profile proxy')"
        :aria-describedby="`${idPrefix}-profile-proxy-hint`"
        searchable="auto"
        @update:model-value="updateProfileProxy"
      />
    </div>

    <details v-if="modelValue.slot_count > 1" class="group" data-testid="slot-proxy-overrides">
      <summary
        class="flex cursor-pointer list-none items-center justify-between gap-3 rounded px-2 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-200 dark:hover:bg-dark-700"
      >
        <span>{{ copy('admin.accounts.codexIdentity.slotOverrides', 'Per-slot proxy overrides') }}</span>
        <Icon name="chevronDown" size="sm" class="transition-transform group-open:rotate-180" />
      </summary>
      <div class="mt-2 divide-y divide-gray-100 border-y border-gray-100 dark:divide-dark-700 dark:border-dark-700">
        <div
          v-for="slotIndex in slotIndexes"
          :key="slotIndex"
          class="grid grid-cols-1 gap-2 py-3 sm:grid-cols-[minmax(0,1fr)_minmax(12rem,18rem)] sm:items-center"
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
            :model-value="slotProxyID(slotIndex)"
            :options="slotProxyOptions"
            :disabled="disabled"
            :aria-label="`${copy('admin.accounts.codexIdentity.deviceSlot', 'Device slot')} ${slotIndex + 1}`"
            searchable="auto"
            @update:model-value="updateSlotProxy(slotIndex, $event)"
          />
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
} from '@/types/codexIdentity'
import { useCodexIdentityCopy } from './copy'
import { cloneCodexOSProfile } from '@/utils/codexIdentityValidation'

const props = withDefaults(defineProps<{
  modelValue: CodexOSProfilePolicy
  proxies?: readonly CodexIdentityProxyOption[]
  accountProxyId?: number | null
  disabled?: boolean
  idPrefix: string
}>(), {
  proxies: () => [],
  accountProxyId: null,
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
  if (!props.accountProxyId) {
    return copy('admin.accounts.codexIdentity.accountProxyDirect', 'Inherit the account connection (currently direct).')
  }
  const proxy = props.proxies.find((item) => item.id === props.accountProxyId)
  const name = proxy?.name ?? `#${props.accountProxyId}`
  return `${copy('admin.accounts.codexIdentity.accountProxyInherit', 'Inherit account proxy')}: ${name}`
})

const proxyOptions = computed(() => [
  { value: null, label: copy('admin.accounts.codexIdentity.inheritAccountProxy', 'Inherit account proxy') },
  ...props.proxies.map((proxy) => ({
    value: proxy.id,
    label: proxyLabel(proxy),
    disabled: proxy.status === 'inactive' || proxy.status === 'expired',
  })),
])

const slotProxyOptions = computed(() => [
  { value: null, label: copy('admin.accounts.codexIdentity.inheritProfileProxy', 'Inherit profile proxy') },
  ...props.proxies.map((proxy) => ({
    value: proxy.id,
    label: proxyLabel(proxy),
    disabled: proxy.status === 'inactive' || proxy.status === 'expired',
  })),
])

const slotIndexes = computed(() => Array.from({ length: props.modelValue.slot_count }, (_, index) => index))

const updateProfileProxy = (value: string | number | boolean | null) => {
  const next = cloneCodexOSProfile(props.modelValue)
  if (typeof value === 'number' && activeProxies.value.some((proxy) => proxy.id === value)) {
    next.proxy_id = value
  } else {
    delete next.proxy_id
  }
  emit('update:modelValue', next)
}

const slotProxyID = (slotIndex: number): number | null =>
  props.modelValue.slots?.find((slot) => slot.index === slotIndex)?.proxy_id ?? null

const updateSlotProxy = (slotIndex: number, value: string | number | boolean | null) => {
  const next = cloneCodexOSProfile(props.modelValue)
  const slots = (next.slots ?? []).filter((slot) => slot.index !== slotIndex)
  if (typeof value === 'number' && activeProxies.value.some((proxy) => proxy.id === value)) {
    slots.push({ index: slotIndex, proxy_id: value })
  }
  next.slots = slots.sort((left, right) => left.index - right.index)
  emit('update:modelValue', next)
}
</script>
