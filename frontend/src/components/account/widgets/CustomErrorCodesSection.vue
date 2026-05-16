<!--
  Host wrapper for the SDK CustomErrorCodesSection widget.
  Bridges host-specific dependencies (stores, composables) to SDK props.
  Canonical widget: plugin-sdk/src/components/account/CustomErrorCodesSection.vue
-->
<template>
  <SdkCustomErrorCodesSection
    v-bind="$attrs"
    :enabled="enabled"
    :codes="codes"
    :commonErrorCodes="hostCommonErrorCodes"
    :onNotifyError="showError"
    :onNotifyInfo="showInfo"
    @update:enabled="emit('update:enabled', $event)"
    @update:codes="emit('update:codes', $event)"
  />
</template>

<script setup lang="ts">
import SdkCustomErrorCodesSection from '@sub2api/plugin-sdk/src/components/account/CustomErrorCodesSection.vue'
import { commonErrorCodes } from '@/composables/useModelWhitelist'
import { useAppStore } from '@/stores/app'

defineProps<{
  enabled: boolean
  codes: number[]
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:codes': [value: number[]]
}>()

const appStore = useAppStore()
const hostCommonErrorCodes = commonErrorCodes
const showError = (msg: string) => appStore.showError(msg)
const showInfo = (msg: string) => appStore.showInfo(msg)
</script>
