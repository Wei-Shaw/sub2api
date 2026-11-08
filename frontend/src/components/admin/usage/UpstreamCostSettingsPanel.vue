<template>
  <!--
    上游 A6 账单配置面板。

    为什么是内嵌面板而不是弹窗：它现在挂在「系统设置」里，进来的人就是来配置的，
    弹窗只会多一层点击。原先放在「使用记录」页时用弹窗是因为那一页的主语是数据表，
    配置是例外动作。
  -->
  <div class="card p-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.upstreamCostSettings.title') }}
        </h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.usage.upstreamCostSettings.intro') }}
        </p>
      </div>
      <button
        type="button"
        class="btn btn-ghost btn-sm shrink-0"
        :disabled="loading"
        data-testid="upstream-cost-settings-reload"
        @click="load"
      >
        {{ t('common.refresh') }}
      </button>
    </div>

    <!-- 生效状态：先告诉人「现在到底有没有配好」，再让他填。
         不然进来看到三个空输入框，根本不知道是真没配还是只是没回显。 -->
    <div
      class="mt-4 rounded-xl border px-4 py-3 text-sm"
      :class="settings?.a6_token_configured
        ? 'border-green-200 bg-green-50 text-green-700 dark:border-green-500/30 dark:bg-green-500/10 dark:text-green-200'
        : 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200'"
      data-testid="upstream-cost-settings-status"
    >
      <template v-if="loading">{{ t('common.loading') }}</template>
      <template v-else-if="settings?.a6_token_configured">
        {{ t('admin.usage.upstreamCostSettings.configured', { mask: settings?.a6_token_mask || '***' }) }}
      </template>
      <template v-else>
        {{ t('admin.usage.upstreamCostSettings.notConfigured') }}
      </template>
    </div>

    <div v-if="loadError" class="mt-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
      {{ loadError }}
    </div>

    <div class="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2">
      <div>
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.usage.upstreamCostSettings.baseUrl') }}
          <span v-if="isOverridden('a6_base_url')" class="ml-2 rounded bg-blue-100 px-1.5 py-0.5 text-[11px] font-normal text-blue-700 dark:bg-blue-500/20 dark:text-blue-200">
            {{ t('admin.usage.upstreamCostSettings.overridden') }}
          </span>
        </label>
        <input
          v-model="form.a6_base_url"
          type="text"
          class="input w-full"
          placeholder="https://a6.example.com"
          data-testid="upstream-cost-settings-base-url"
        />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.usage.upstreamCostSettings.baseUrlHint') }}</p>
      </div>

      <div>
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.usage.upstreamCostSettings.userId') }}
          <span v-if="isOverridden('a6_user_id')" class="ml-2 rounded bg-blue-100 px-1.5 py-0.5 text-[11px] font-normal text-blue-700 dark:bg-blue-500/20 dark:text-blue-200">
            {{ t('admin.usage.upstreamCostSettings.overridden') }}
          </span>
        </label>
        <input
          v-model="form.a6_user_id"
          type="text"
          class="input w-full"
          data-testid="upstream-cost-settings-user-id"
        />
      </div>

      <div class="lg:col-span-2">
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.usage.upstreamCostSettings.accessToken') }}
          <span v-if="isOverridden('a6_access_token')" class="ml-2 rounded bg-blue-100 px-1.5 py-0.5 text-[11px] font-normal text-blue-700 dark:bg-blue-500/20 dark:text-blue-200">
            {{ t('admin.usage.upstreamCostSettings.overridden') }}
          </span>
        </label>
        <input
          v-model="form.a6_access_token"
          type="password"
          autocomplete="new-password"
          class="input w-full"
          :placeholder="t('admin.usage.upstreamCostSettings.accessTokenPlaceholder')"
          data-testid="upstream-cost-settings-token"
        />
        <!-- 关键语义：留空 = 不改，而不是「清空」。不说清楚的话，人每次来
             改个地址就会顺手把令牌清掉，然后成本整整一天取不到。 -->
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.usage.upstreamCostSettings.accessTokenHint') }}</p>
      </div>

      <label class="flex items-center gap-2 text-sm text-gray-600 lg:col-span-2 dark:text-gray-300">
        <input v-model="clearToken" type="checkbox" class="rounded border-gray-300" data-testid="upstream-cost-settings-clear-token" />
        {{ t('admin.usage.upstreamCostSettings.clearToken') }}
      </label>
    </div>

    <div v-if="saveError" class="mt-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
      {{ saveError }}
    </div>
    <div v-if="saved" class="mt-3 rounded-xl border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700 dark:border-green-500/30 dark:bg-green-500/10 dark:text-green-200">
      {{ t('admin.usage.upstreamCostSettings.saved') }}
    </div>

    <div class="mt-5 flex justify-end gap-2">
      <button type="button" class="btn btn-ghost" :disabled="saving" @click="load">
        {{ t('common.reset') }}
      </button>
      <button type="button" class="btn btn-primary" :disabled="saving || loading" data-testid="upstream-cost-settings-save" @click="save">
        {{ saving ? t('common.saving') : t('common.save') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminUsageAPI, type UpstreamCostSettings } from '@/api/admin/usage'

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const saved = ref(false)
const loadError = ref('')
const saveError = ref('')
const clearToken = ref(false)
const settings = ref<UpstreamCostSettings | null>(null)

const form = ref({ a6_base_url: '', a6_user_id: '', a6_access_token: '' })

const overriddenKeys = computed(() => settings.value?.override_keys ?? [])
const isOverridden = (key: string) => overriddenKeys.value.includes(key)

async function load() {
  loading.value = true
  loadError.value = ''
  saveError.value = ''
  saved.value = false
  clearToken.value = false
  try {
    const data = await adminUsageAPI.getUpstreamCostSettings()
    settings.value = data
    // 令牌**不回显**（接口本来就不返回明文），所以这里只填地址与用户标识。
    // 想清空令牌得显式勾选，避免「保存一次就把令牌抹了」这种事故。
    form.value = {
      a6_base_url: data.a6_base_url || '',
      a6_user_id: data.a6_user_id || '',
      a6_access_token: '',
    }
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
    const payload: Record<string, unknown> = {
      a6_base_url: form.value.a6_base_url,
      a6_user_id: form.value.a6_user_id,
    }
    // 只在真的填了令牌时才把字段发上去：传空串会被后端当成「清除覆盖」，
    // 而用户的意图往往只是「我只改地址」。
    if (form.value.a6_access_token) {
      payload.a6_access_token = form.value.a6_access_token
    }
    if (clearToken.value) {
      payload.clear_a6_access_token = true
    }
    const data = await adminUsageAPI.updateUpstreamCostSettings(payload)
    settings.value = data
    form.value.a6_access_token = ''
    clearToken.value = false
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
