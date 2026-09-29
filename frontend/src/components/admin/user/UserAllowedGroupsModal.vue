<template>
  <BaseDialog :show="show" :title="t('admin.users.groupConfig')" width="wide" @close="$emit('close')">
    <div v-if="user" class="space-y-6">
      <!-- 用户信息头部 -->
      <div class="border-b-2 border-accent pb-3">
        <div>
          <p class="text-h2 font-bold text-fg">{{ user.email }}</p>
          <p class="mt-1 text-sm text-fg-muted">{{ t('admin.users.groupConfigHint', { email: user.email }) }}</p>
        </div>
      </div>

      <!-- 加载状态 -->
      <div v-if="loading" class="flex justify-center py-12">
        <svg class="h-10 w-10 animate-spin text-accent" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>

      <div v-else class="space-y-6">
        <!-- 专属分组区域 -->
        <div v-if="exclusiveGroups.length > 0">
          <div class="mb-3 flex items-center gap-2">
            <h4 class="text-h3 font-bold text-accent-strong">{{ t('admin.users.exclusiveGroups') }}</h4>
            <span class="text-xs tabular-nums text-fg-subtle">({{ exclusiveGroupConfigs.filter(c => c.isSelected).length }}/{{ exclusiveGroupConfigs.length }})</span>
          </div>
          <div class="divide-y divide-border border-y border-border">
            <div
              v-for="config in exclusiveGroupConfigs"
              :key="config.groupId"
              class="px-2 py-3 transition-colors"
              :class="config.isSelected
                ? 'bg-accent-weak'
                : 'hover:bg-accent-weak/50'"
            >
              <div class="flex items-center gap-4">
                <!-- 复选框 -->
                <div class="flex-shrink-0">
                  <label class="relative flex h-6 w-6 cursor-pointer items-center justify-center">
                    <input
                      type="checkbox"
                      :checked="config.isSelected"
                      @change="toggleExclusiveGroup(config.groupId)"
                      class="peer sr-only"
                    />
                    <div class="h-5 w-5 rounded-sm border-2 border-border-strong transition-colors peer-checked:border-accent peer-checked:bg-accent">
                      <svg v-if="config.isSelected" class="h-full w-full text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                      </svg>
                    </div>
                  </label>
                </div>

                <!-- 分组信息 -->
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="text-body font-semibold text-fg">{{ config.groupName }}</span>
                    <span class="badge badge-primary">
                      {{ t('admin.groups.exclusive') }}
                    </span>
                  </div>
                  <div class="mt-1.5 flex items-center gap-3 text-sm">
                    <span class="inline-flex items-center gap-1 text-fg-muted">
                      <PlatformIcon :platform="config.platform" size="xs" />
                      <span>{{ config.platform }}</span>
                    </span>
                    <span class="text-fg-subtle">•</span>
                    <span class="text-fg-muted">
                      {{ t('admin.users.defaultRate') }}: <span class="font-semibold tabular-nums text-fg">×{{ config.defaultRate }}</span>
                    </span>
                  </div>
                </div>

                <!-- 专属倍率输入 -->
                <div class="flex flex-shrink-0 items-center gap-3">
                  <label class="text-sm font-medium text-fg-muted">{{ t('admin.users.customRate') }}</label>
                  <input
                    type="number"
                    step="0.001"
                    min="0.001"
                    :value="config.customRate ?? ''"
                    @input="updateCustomRate(config.groupId, ($event.target as HTMLInputElement).value)"
                    :placeholder="String(config.defaultRate)"
                    class="hide-spinner input w-24 text-right tabular-nums"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 公开分组区域 -->
        <div v-if="publicGroups.length > 0">
          <div class="mb-3 flex flex-wrap items-center gap-2">
            <h4 class="text-h3 font-bold text-accent-strong">
              {{ restrictPublicGroups ? t('admin.users.publicGroupsRestricted') : t('admin.users.publicGroups') }}
            </h4>
            <span class="text-xs tabular-nums text-fg-subtle">({{ publicGroupConfigs.length }})</span>
            <label class="ml-auto flex cursor-pointer items-center gap-2 text-sm text-fg-muted">
              <input
                type="checkbox"
                :checked="restrictPublicGroups"
                @change="toggleRestrictPublicGroups"
                class="h-4 w-4 cursor-pointer rounded-sm border-border-strong text-accent focus:ring-accent"
              />
              {{ t('admin.users.restrictPublicGroups') }}
            </label>
          </div>
          <p class="mb-3 text-xs text-fg-muted">{{ t('admin.users.restrictPublicGroupsHint') }}</p>
          <div class="divide-y divide-border border-y border-border">
            <div
              v-for="config in publicGroupConfigs"
              :key="config.groupId"
              class="px-2 py-3"
            >
              <div class="flex items-center gap-4">
                <!-- 未开启限制时公开分组恒可用，此处仅作展示；开启后才是真实开关 -->
                <div class="flex-shrink-0">
                  <input
                    v-if="restrictPublicGroups"
                    type="checkbox"
                    :checked="config.isSelected"
                    @change="togglePublicGroup(config.groupId)"
                    class="h-5 w-5 cursor-pointer rounded-sm border-2 border-border-strong text-accent focus:ring-accent"
                  />
                  <div
                    v-else
                    class="flex h-5 w-5 items-center justify-center rounded-sm border-2 border-accent bg-accent"
                  >
                    <svg class="h-full w-full text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  </div>
                </div>

                <!-- 分组信息 -->
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="text-body font-semibold text-fg">{{ config.groupName }}</span>
                  </div>
                  <div class="mt-1.5 flex items-center gap-3 text-sm">
                    <span class="inline-flex items-center gap-1 text-fg-muted">
                      <PlatformIcon :platform="config.platform" size="xs" />
                      <span>{{ config.platform }}</span>
                    </span>
                    <span class="text-fg-subtle">•</span>
                    <span class="text-fg-muted">
                      {{ t('admin.users.defaultRate') }}: <span class="font-semibold tabular-nums text-fg">×{{ config.defaultRate }}</span>
                    </span>
                  </div>
                </div>

                <!-- 专属倍率输入 -->
                <div class="flex flex-shrink-0 items-center gap-3">
                  <label class="text-sm font-medium text-fg-muted">{{ t('admin.users.customRate') }}</label>
                  <input
                    type="number"
                    step="0.001"
                    min="0.001"
                    :value="config.customRate ?? ''"
                    @input="updateCustomRate(config.groupId, ($event.target as HTMLInputElement).value)"
                    :placeholder="String(config.defaultRate)"
                    class="hide-spinner input w-24 text-right tabular-nums"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 无分组提示 -->
        <div v-if="groups.length === 0" class="flex flex-col items-center justify-center py-12 text-center">
          <svg class="mb-3 h-10 w-10 text-fg-subtle" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
            </svg>
          <p class="text-fg-muted">{{ t('common.noGroupsAvailable') }}</p>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" class="btn btn-secondary px-5">{{ t('common.cancel') }}</button>
        <button @click="handleSave" :disabled="submitting || !loaded" class="btn btn-primary px-6">
          <svg v-if="submitting" class="-ml-1 mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          {{ submitting ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { AdminUser, Group, GroupPlatform } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

interface GroupRateConfig {
  groupId: number
  groupName: string
  platform: GroupPlatform
  isExclusive: boolean
  defaultRate: number
  customRate: number | null
  isSelected: boolean
}

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits(['close', 'success'])
const { t } = useI18n()
const appStore = useAppStore()

const groups = ref<Group[]>([])
const groupConfigs = ref<GroupRateConfig[]>([])
const originalGroupRates = ref<Record<number, number>>({}) // 记录原始专属倍率，用于检测删除
const loading = ref(false)
const loaded = ref(false)
const submitting = ref(false)
const restrictPublicGroups = ref(false)

// 分离专属分组和公开分组
const exclusiveGroups = computed(() => groups.value.filter((g) => g.is_exclusive))
const publicGroups = computed(() => groups.value.filter((g) => !g.is_exclusive))

const exclusiveGroupConfigs = computed(() => groupConfigs.value.filter((c) => c.isExclusive))
const publicGroupConfigs = computed(() => groupConfigs.value.filter((c) => !c.isExclusive))

watch(
  () => props.show,
  (v) => {
    if (v && props.user) {
      load()
    }
  }
)

const load = async () => {
  loaded.value = false
  loading.value = true
  try {
    const res = await adminAPI.groups.list(1, 1000)
    // 只显示标准类型且活跃的分组
    groups.value = res.items.filter((g) => g.subscription_type === 'standard' && g.status === 'active')

    // 初始化配置
    const userAllowedGroups = props.user?.allowed_groups || []
    const userGroupRates = props.user?.group_rates || {}
    restrictPublicGroups.value = props.user?.restrict_public_groups ?? false

    // 保存原始专属倍率，用于检测删除操作
    originalGroupRates.value = { ...userGroupRates }

    groupConfigs.value = groups.value.map((g) => ({
      groupId: g.id,
      groupName: g.name,
      platform: g.platform,
      isExclusive: g.is_exclusive,
      defaultRate: g.rate_multiplier,
      customRate: userGroupRates[g.id] ?? null,
      // 专属分组：检查是否在 allowed_groups 中
      // 公开分组：未开启限制时恒可用；开启后同样以 allowed_groups 为准
      isSelected:
        g.is_exclusive || restrictPublicGroups.value ? userAllowedGroups.includes(g.id) : true,
    }))
    loaded.value = true
  } catch (error) {
    console.error('Failed to load groups:', error)
    appStore.showError(extractApiErrorMessage(error, t('admin.users.failedToLoadGroups')))
  } finally {
    loading.value = false
  }
}

const toggleExclusiveGroup = (groupId: number) => {
  const config = groupConfigs.value.find((c) => c.groupId === groupId)
  if (config && config.isExclusive) {
    config.isSelected = !config.isSelected
  }
}

const togglePublicGroup = (groupId: number) => {
  const config = groupConfigs.value.find((c) => c.groupId === groupId)
  if (config && !config.isExclusive) {
    config.isSelected = !config.isSelected
  }
}

// 关闭限制时把公开分组全部勾回，避免保存出一份"限制已关但只勾了两个"的误导状态。
const toggleRestrictPublicGroups = () => {
  restrictPublicGroups.value = !restrictPublicGroups.value
  if (!restrictPublicGroups.value) {
    for (const config of groupConfigs.value) {
      if (!config.isExclusive) config.isSelected = true
    }
  }
}

const updateCustomRate = (groupId: number, value: string) => {
  const config = groupConfigs.value.find((c) => c.groupId === groupId)
  if (config) {
    if (value === '' || value === null || value === undefined) {
      config.customRate = null
    } else {
      const numValue = parseFloat(value)
      config.customRate = isNaN(numValue) ? null : numValue
    }
  }
}

const handleSave = async () => {
  if (!props.user || !loaded.value) return
  submitting.value = true

  try {
    // 构建 allowed_groups：专属分组中被勾选的，以及开启限制后被勾选的公开分组。
    // 未开启限制时不写入公开分组，保持该表"额外授予"的原有语义。
    // 弹窗未展示的分组（停用、非标准类型等）无法在此编辑，原样保留其授权，避免被整体覆盖时丢失。
    const editableIds = new Set(groupConfigs.value.map((c) => c.groupId))
    const allowedGroups = [
      ...(props.user.allowed_groups || []).filter((id) => !editableIds.has(id)),
      ...groupConfigs.value
        .filter((c) => c.isSelected && (c.isExclusive || restrictPublicGroups.value))
        .map((c) => c.groupId),
    ]

    // 构建 group_rates
    // - 有新专属倍率: 设置为该值
    // - 原本有专属倍率但现在被清空: 设置为 null（表示删除）
    const groupRates: Record<number, number | null> = {}
    for (const c of groupConfigs.value) {
      const hadOriginalRate = originalGroupRates.value[c.groupId] !== undefined

      if (c.customRate !== null) {
        // 有专属倍率
        groupRates[c.groupId] = c.customRate
      } else if (hadOriginalRate) {
        // 原本有专属倍率，现在被清空，需要显式删除
        groupRates[c.groupId] = null
      }
    }

    await adminAPI.users.update(props.user.id, {
      allowed_groups: allowedGroups,
      restrict_public_groups: restrictPublicGroups.value,
      group_rates: Object.keys(groupRates).length > 0 ? groupRates : undefined,
    })

    appStore.showSuccess(t('admin.users.groupConfigUpdated'))
    emit('success')
    emit('close')
  } catch (error) {
    console.error('Failed to update user group config:', error)
    appStore.showError(extractApiErrorMessage(error, t('admin.users.failedToUpdateAllowedGroups')))
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
/* 隐藏数字输入框的箭头按钮 */
.hide-spinner::-webkit-outer-spin-button,
.hide-spinner::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.hide-spinner {
  -moz-appearance: textfield;
}
</style>
