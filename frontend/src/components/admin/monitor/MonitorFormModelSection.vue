<template>
  <div class="space-y-5">
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.form.primaryModel') }} <span class="text-red-500">*</span></label>
      <input
        v-model="primaryModel"
        type="text"
        required
        class="input font-medium"
        :class="getPlatformTextClass(provider)"
        :placeholder="t('admin.channelMonitor.form.primaryModelPlaceholder')"
      />
    </div>

    <div>
      <label class="input-label">{{ t('admin.channelMonitor.form.extraModels') }}</label>
      <ModelTagInput
        :models="extraModels"
        :platform="provider"
        :placeholder="t('admin.channelMonitor.form.extraModelsPlaceholder')"
        @update:models="extraModels = $event"
      />
    </div>

    <div>
      <label class="input-label">{{ t('admin.channelMonitor.form.groupName') }}</label>
      <input v-model="groupName" type="text" class="input" :placeholder="t('admin.channelMonitor.form.groupNamePlaceholder')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { Provider } from '@/api/admin/channelMonitor'
import ModelTagInput from '@/components/admin/channel/ModelTagInput.vue'
import { getPlatformTextClass } from '@/components/admin/channel/types'

defineProps<{
  provider: Provider
}>()

const primaryModel = defineModel<string>('primaryModel', { required: true })
const extraModels = defineModel<string[]>('extraModels', { required: true })
const groupName = defineModel<string>('groupName', { required: true })

const { t } = useI18n()
</script>
