<template>
  <details class="rounded-lg border border-gray-200 bg-gray-50/50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
    <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('admin.channelMonitor.advanced.section') }}
    </summary>
    <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.advanced.sectionHint') }}</p>

    <div class="mt-4 space-y-4">
      <div>
        <label class="input-label">{{ t('admin.channelMonitor.templateField.label') }}</label>
        <Select
          v-model="templateSelectValue"
          :options="templateOptions"
          :placeholder="t('admin.channelMonitor.templateField.placeholder')"
        />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.templateField.applyHint') }}</p>
      </div>

      <MonitorAdvancedRequestConfig
        :extra-headers="extraHeaders"
        :body-override-mode="bodyOverrideMode"
        :body-override="bodyOverride"
        @update:extra-headers="extraHeaders = $event"
        @update:body-override-mode="bodyOverrideMode = $event"
        @update:body-override="bodyOverride = $event"
      />
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { BodyOverrideMode, Provider } from '@/api/admin/channelMonitor'
import type { ChannelMonitorTemplate } from '@/api/admin/channelMonitorTemplate'
import Select from '@/components/common/Select.vue'
import MonitorAdvancedRequestConfig from '@/components/admin/monitor/MonitorAdvancedRequestConfig.vue'

const props = defineProps<{
  // 当 dialog 打开时父组件设为 true，触发模板列表懒加载
  active: boolean
  provider: Provider
}>()

const templateId = defineModel<number | null>('templateId', { required: true })
const extraHeaders = defineModel<Record<string, string>>('extraHeaders', { required: true })
const bodyOverrideMode = defineModel<BodyOverrideMode>('bodyOverrideMode', { required: true })
const bodyOverride = defineModel<Record<string, unknown> | null>('bodyOverride', { required: true })

const { t } = useI18n()

// 可用模板列表（dialog 打开时一次性拉取 cache；按 provider 过滤）。
const templatesCache = ref<ChannelMonitorTemplate[]>([])
const templatesLoading = ref(false)

const templateOptions = computed(() => {
  const items = templatesCache.value.filter((tpl) => tpl.provider === props.provider)
  return [
    { value: '', label: t('admin.channelMonitor.templateField.none') },
    ...items.map((tpl) => ({ value: String(tpl.id), label: tpl.name })),
  ]
})

async function loadTemplates() {
  if (templatesCache.value.length > 0) return
  templatesLoading.value = true
  try {
    const { items } = await adminAPI.channelMonitorTemplate.list()
    templatesCache.value = items
  } catch (err: unknown) {
    // 模板拉取失败不阻塞监控表单，用户可以不选模板
    console.warn('load monitor templates failed', err)
  } finally {
    templatesLoading.value = false
  }
}

watch(
  () => props.active,
  (active) => {
    if (active) void loadTemplates()
  },
  { immediate: true },
)

// 模板下拉绑定：value 是 string（Select 组件约束），需要与 number | null 互转。
// set 时若选中合法模板，复制其快照到当前 form（extra_headers / body_override_mode /
// body_override），与原主文件行为完全一致。
const templateSelectValue = computed<string>({
  get: () => (templateId.value == null ? '' : String(templateId.value)),
  set: (raw: string) => {
    if (raw === '') {
      templateId.value = null
      return
    }
    const id = Number(raw)
    if (!Number.isFinite(id)) return
    templateId.value = id
    const tpl = templatesCache.value.find((item) => item.id === id)
    if (tpl) {
      extraHeaders.value = { ...(tpl.extra_headers || {}) }
      bodyOverrideMode.value = tpl.body_override_mode
      bodyOverride.value = tpl.body_override ? { ...tpl.body_override } : null
    }
  },
})
</script>
