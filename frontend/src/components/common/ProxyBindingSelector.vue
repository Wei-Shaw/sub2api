<template>
  <div class="space-y-3">
    <select v-model="selectedBindingKey" class="input" :disabled="disabled">
      <option value="none">{{ t('admin.proxyGroups.binding.none') }}</option>

      <optgroup v-if="proxyGroups.length" :label="t('admin.proxyGroups.binding.group')">
        <option v-for="group in proxyGroups" :key="`group-${group.id}`" :value="`group:${group.id}`">
          {{ group.name }} · {{ group.available_member_count }}/{{ group.member_count }}
        </option>
      </optgroup>

      <optgroup v-if="proxies.length" :label="t('admin.proxyGroups.binding.proxy')">
        <option v-for="proxy in proxies" :key="`proxy-${proxy.id}`" :value="`proxy:${proxy.id}`">
          {{ proxy.name }} ({{ proxy.protocol }}://{{ proxy.host }}:{{ proxy.port }})
        </option>
      </optgroup>
    </select>

    <p v-if="selectedGroup" class="input-hint">
      {{ t('admin.proxyGroups.binding.groupHint', {
        available: selectedGroup.available_member_count,
        total: selectedGroup.member_count
      }) }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Proxy, ProxyGroup } from '@/types'

const { t } = useI18n()

interface Props {
  proxyId: number | null
  proxyGroupId: number | null
  proxies: Proxy[]
  proxyGroups?: ProxyGroup[]
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  proxyGroups: () => [],
  disabled: false
})

const emit = defineEmits<{
  'update:proxyId': [value: number | null]
  'update:proxyGroupId': [value: number | null]
}>()

type BindingKey = 'none' | `proxy:${number}` | `group:${number}`

const selectedBindingKey = computed<BindingKey>({
  get: () => {
    if (props.proxyGroupId !== null) return `group:${props.proxyGroupId}` as BindingKey
    if (props.proxyId !== null) return `proxy:${props.proxyId}` as BindingKey
    return 'none'
  },
  set: (value: BindingKey) => {
    if (value === 'none') {
      emit('update:proxyId', null)
      emit('update:proxyGroupId', null)
      return
    }

    const separatorIndex = value.indexOf(':')
    const type = value.slice(0, separatorIndex)
    const id = Number(value.slice(separatorIndex + 1))
    if (!['group', 'proxy'].includes(type) || !Number.isInteger(id)) return

    if (type === 'group') {
      emit('update:proxyId', null)
      emit('update:proxyGroupId', id)
      return
    }

    emit('update:proxyGroupId', null)
    emit('update:proxyId', id)
  }
})

const selectedGroup = computed(() => props.proxyGroups.find((group) => group.id === props.proxyGroupId))
</script>
