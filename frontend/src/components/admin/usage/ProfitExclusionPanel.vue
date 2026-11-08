<template>
  <!--
    盈亏排除名单面板。

    业务背景：内部人员也在用本站，但他们的余额是管理员手工调整的、并没有真实付款，
    所以他们的「收入」是假的；可他们消耗掉的上游额度是真花钱，成本必须照实计入。
    这份名单就是给收入侧用的——名单里的人，收入不计入盈亏，成本照算。

    为什么与「上游账单配置」分成两张卡：两者的失败模式完全不同。凭据保存失败要
    重填令牌，名单保存失败只影响统计口径。合成一张卡之后，一次令牌校验没过就会
    让名单也跟着不生效，反之亦然。
  -->
  <div class="card p-6" data-testid="profit-exclusion-panel">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.profitExclusion.title') }}
        </h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.usage.profitExclusion.intro') }}
        </p>
      </div>
      <button
        type="button"
        class="btn btn-ghost btn-sm shrink-0"
        :disabled="loading"
        data-testid="profit-exclusion-reload"
        @click="load"
      >
        {{ t('common.refresh') }}
      </button>
    </div>

    <!-- 生效状态：先把「现在到底排除了几个人」说清楚，再让人改。
         不写清楚的话，进来看到一个空选择框分不清是「没配」还是「加载失败」。 -->
    <div
      class="mt-4 rounded-xl border px-4 py-3 text-sm"
      :class="selectedUserIds.length > 0
        ? 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200'
        : 'border-gray-200 bg-gray-50 text-gray-600 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-300'"
      data-testid="profit-exclusion-status"
    >
      <template v-if="loading">{{ t('common.loading') }}</template>
      <template v-else-if="selectedUserIds.length > 0">
        {{ t('admin.usage.profitExclusion.active', { count: selectedUserIds.length }) }}
      </template>
      <template v-else>
        {{ t('admin.usage.profitExclusion.inactive') }}
      </template>
    </div>

    <div v-if="loadError" class="mt-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
      {{ loadError }}
    </div>
    <div v-if="saveError" class="mt-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
      {{ saveError }}
    </div>
    <div v-if="saved" class="mt-3 rounded-xl border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700 dark:border-green-500/30 dark:bg-green-500/10 dark:text-green-200" data-testid="profit-exclusion-saved">
      {{ t('admin.usage.profitExclusion.saved') }}
    </div>

    <div class="mt-4">
      <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
        {{ t('admin.usage.profitExclusion.membersLabel') }}
      </label>
      <!--
        复用既有的通用用户选择器（防抖搜索 + 已选列表 + 名字自动补全）。
        它目前住在「OpenAI 快速策略」目录下，但内部文案与逻辑都是通用的；
        搬去公共目录要连带改它自己的测试与 SettingsView 的引用，属于另一次
        独立的小重构，不在这条改动里顺手做。
      -->
      <OpenAIFastPolicyUserSelector v-model="selectedUserIds" />
      <p class="mt-2 text-xs text-gray-400">
        {{ t('admin.usage.profitExclusion.membersHint') }}
      </p>
      <p class="mt-1 text-xs text-gray-400">
        {{ t('admin.usage.profitExclusion.costStillCounted') }}
      </p>
    </div>

    <div class="mt-4 flex items-center gap-3">
      <button
        type="button"
        class="btn btn-primary btn-sm"
        :disabled="saving || loading || !dirty"
        data-testid="profit-exclusion-save"
        @click="save"
      >
        {{ saving ? t('common.saving') : t('common.save') }}
      </button>
      <span v-if="dirty" class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.usage.profitExclusion.unsaved') }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminUsageAPI } from '@/api/admin/usage'
import OpenAIFastPolicyUserSelector from '@/views/admin/settings/OpenAIFastPolicyUserSelector.vue'

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const saved = ref(false)
const loadError = ref('')
const saveError = ref('')
const selectedUserIds = ref<number[]>([])
// loadedUserIds 是「服务端当前生效的名单」，用来判断有没有未保存的改动。
// 不能拿 selectedUserIds 与它自身比，必须在加载时留一份快照。
const loadedUserIds = ref<number[]>([])

const normalize = (ids: number[]) =>
  Array.from(new Set(ids.filter((id) => Number.isInteger(id) && id > 0))).sort((a, b) => a - b)

const dirty = computed(() => {
  const a = normalize(selectedUserIds.value)
  const b = normalize(loadedUserIds.value)
  return a.length !== b.length || a.some((id, i) => id !== b[i])
})

async function load() {
  loading.value = true
  loadError.value = ''
  saveError.value = ''
  saved.value = false
  try {
    const data = await adminUsageAPI.getUsageProfitExclusion()
    const ids = normalize(data.user_ids ?? [])
    selectedUserIds.value = ids
    loadedUserIds.value = [...ids]
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  saveError.value = ''
  saved.value = false
  try {
    const data = await adminUsageAPI.updateUsageProfitExclusion({
      user_ids: normalize(selectedUserIds.value)
    })
    const ids = normalize(data.user_ids ?? [])
    selectedUserIds.value = ids
    loadedUserIds.value = [...ids]
    saved.value = true
  } catch (err) {
    saveError.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void load()
})
</script>
