<template>
  <BaseDialog
    :show="show"
    :title="dialogTitle"
    width="extra-wide"
    @close="onCancel"
  >
    <div class="channel-dialog-body">
      <!-- Tab Bar -->
      <div class="flex items-center border-b border-gray-200 dark:border-dark-700 flex-shrink-0 -mx-4 sm:-mx-6 px-4 sm:px-6 -mt-3 sm:-mt-4">
        <button
          type="button"
          @click="activeTab = 'basic'"
          class="channel-tab"
          :class="activeTab === 'basic' ? 'channel-tab-active' : 'channel-tab-inactive'"
        >
          {{ t('admin.channels.form.basicSettings', 'Basic Settings') }}
        </button>
        <button
          v-for="section in enabledPlatforms"
          :key="section.platform"
          type="button"
          @click="activeTab = section.platform"
          class="channel-tab group"
          :class="activeTab === section.platform ? 'channel-tab-active' : 'channel-tab-inactive'"
        >
          <PlatformIcon :platform="section.platform" size="xs" :class="platformTextClass(section.platform)" />
          <span :class="platformTextClass(section.platform)">{{ t('admin.groups.platforms.' + section.platform, section.platform) }}</span>
        </button>
      </div>

      <!-- Tab Content -->
      <form id="channel-form" @submit.prevent="onSubmit" class="flex-1 overflow-y-auto pt-4">
        <ChannelBasicSettings
          v-show="activeTab === 'basic'"
          :form="form"
          :is-edit="isEdit"
          @update-field="onFieldUpdate"
          @toggle-platform="togglePlatform"
        />

        <template v-for="(section, sIdx) in form.platforms" :key="'tab-' + section.platform">
          <ChannelPlatformTab
            v-show="section.enabled && activeTab === section.platform"
            :section="section"
            :all-groups="allGroups"
            :groups-loading="groupsLoading"
            :group-channel-map="groupChannelMap"
            @update:section="updateSection(sIdx, $event)"
          />
        </template>
      </form>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="onCancel" type="button" class="btn btn-secondary">
          {{ t('common.cancel', 'Cancel') }}
        </button>
        <button
          type="submit"
          form="channel-form"
          :disabled="submitting"
          class="btn btn-primary"
        >
          {{ submitting
            ? t('common.submitting', 'Submitting...')
            : isEdit
              ? t('common.update', 'Update')
              : t('common.create', 'Create')
          }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
/**
 * ChannelDialog — 创建/编辑渠道对话框 (T9 拆出).
 *
 * Props: show / channel (null = 创建) / allGroups / groupsLoading /
 *        allChannelsForConflict
 * Emits: update:show, saved
 *
 * 内部职责: form state 维护 + tab 切换 + 校验 + 提交. form 通过 buildEditForm
 * / createEmptyForm 初始化, 不暴露给父 (封装).
 */
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog, PlatformIcon } from '@sub2api/plugin-sdk'
import ChannelBasicSettings from './ChannelBasicSettings.vue'
import ChannelPlatformTab from './ChannelPlatformTab.vue'
import {
  type ChannelFormState,
  type GroupPlatform,
  type PlatformSection,
  type AdminGroup,
} from './types'
import type { Channel } from '../../api/channels'
import {
  buildCreateRequest,
  buildEditForm,
  buildUpdateRequest,
  createEmptyForm,
} from '../../composables/useChannelFormSerializer'
import { useChannelCrud } from '../../composables/useChannelCrud'
import {
  validateChannelForm,
  type ValidationError,
} from '../../composables/useChannelValidation'
import { platformTextClass } from '@sub2api/plugin-sdk'
import { getSdk } from '../../api/sdk'

const props = defineProps<{
  show: boolean
  channel: Channel | null
  allGroups: AdminGroup[]
  groupsLoading: boolean
  allChannelsForConflict: Channel[]
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const sdk = getSdk()
const { createChannel, updateChannel } = useChannelCrud()

const form = reactive<ChannelFormState>(createEmptyForm())
const activeTab = ref<string>('basic')
const submitting = ref(false)

const isEdit = computed(() => props.channel !== null)
const enabledPlatforms = computed(() => form.platforms.filter((s) => s.enabled))
const dialogTitle = computed(() =>
  isEdit.value
    ? t('admin.channels.editChannel', 'Edit Channel')
    : t('admin.channels.createChannel', 'Create Channel'),
)

/** group → 占用此 group 的 channel name (排除当前编辑的渠道). */
const groupChannelMap = computed<Map<number, string>>(() => {
  const map = new Map<number, string>()
  for (const ch of props.allChannelsForConflict) {
    if (props.channel && ch.id === props.channel.id) continue
    for (const gid of ch.group_ids || []) map.set(gid, ch.name)
  }
  return map
})

watch(
  () => [props.show, props.channel] as const,
  ([show, channel]) => {
    if (!show) return
    Object.assign(
      form,
      channel ? buildEditForm(channel, props.allGroups) : createEmptyForm(),
    )
    activeTab.value = 'basic'
  },
  { immediate: true },
)

function onFieldUpdate(key: keyof ChannelFormState, value: unknown): void {
  // form 是 reactive, 直接赋值. value 类型已在 ChannelBasicSettings 内 narrow.
  ;(form as Record<string, unknown>)[key as string] = value
}

function togglePlatform(platform: GroupPlatform): void {
  const section = form.platforms.find((s) => s.platform === platform)
  if (section) {
    section.enabled = !section.enabled
    if (!section.enabled && activeTab.value === platform) activeTab.value = 'basic'
    return
  }
  form.platforms.push({
    platform,
    enabled: true,
    collapsed: false,
    group_ids: [],
    model_mapping: {},
    model_pricing: [],
    account_stats_pricing_rules: [],
  })
}

function updateSection(idx: number, next: PlatformSection): void {
  form.platforms.splice(idx, 1, next)
}

function onCancel(): void {
  emit('update:show', false)
}

async function onSubmit(): Promise<void> {
  if (submitting.value) return
  const result = validateChannelForm(form)
  if (!result.ok) {
    showValidationError(result.error)
    return
  }
  submitting.value = true
  try {
    const ok = isEdit.value && props.channel
      ? await updateChannel(props.channel.id, buildUpdateRequest(form))
      : await createChannel(buildCreateRequest(form))
    if (ok) {
      emit('saved')
      emit('update:show', false)
    }
  } finally {
    submitting.value = false
  }
}

function showValidationError(err: ValidationError): void {
  // params.platform 是 platform id, 渲染前翻译为 admin.groups.platforms.<id>.
  const params = err.params ? { ...err.params } : undefined
  if (params && typeof params.platform === 'string') {
    const id = params.platform
    params.platform = t('admin.groups.platforms.' + id, id)
  }
  sdk.notify.error(t(err.key, params || {}, err.key))
  if (err.platform) activeTab.value = err.platform
}
</script>

<style scoped>
.channel-dialog-body {
  display: flex;
  flex-direction: column;
  height: 70vh;
  min-height: 400px;
}

.channel-tab {
  @apply flex items-center gap-1.5 px-3 py-2.5 text-sm font-medium border-b-2 transition-colors whitespace-nowrap;
}

.channel-tab-active {
  @apply border-primary-600 text-primary-600 dark:border-primary-400 dark:text-primary-400;
}

.channel-tab-inactive {
  @apply border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300;
}
</style>
