<template>
  <!-- title/description 已上移到 channel-management manifest descriptions,
       host AppHeader 唯一渲染标题区. PluginPageLayout 继续作为 layout 容器使用,
       FilterBar/PageActions 仍由 view 提供。 -->
  <PluginPageLayout>
    <template #actions>
      <PageActions>
        <button
          @click="loadChannels"
          :disabled="loading"
          class="btn btn-secondary"
          :title="t('common.refresh', 'Refresh')"
        >
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
        </button>
      </PageActions>
    </template>

    <div class="layout-section-fixed">
      <FilterBar>
        <template #left>
          <div class="w-full sm:w-80">
            <SearchInput
              v-model="searchQuery"
              :placeholder="t('availableChannels.searchPlaceholder')"
            />
          </div>
        </template>
      </FilterBar>
    </div>

    <div class="layout-section-scrollable">
      <AvailableChannelsTable
        :columns="columnLabels"
        :rows="filteredChannels"
        :loading="loading"
        :user-group-rates="userGroupRates"
        pricing-key-prefix="availableChannels.pricing"
        :no-pricing-label="t('availableChannels.noPricing')"
        :no-models-label="t('availableChannels.noModels')"
        :empty-label="t('availableChannels.empty')"
      />
    </div>
  </PluginPageLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { FilterBar, Icon, PageActions, PluginPageLayout, SearchInput } from '@sub2api/plugin-sdk'
import AvailableChannelsTable from '../../components/channels/AvailableChannelsTable.vue'
import userChannelsAPI, { type UserAvailableChannel } from '../../api/user/availableChannels'
import { getSdk } from '../../api/sdk'

const { t } = useI18n()
const sdk = getSdk()

const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const searchQuery = ref('')

const columnLabels = computed(() => ({
  name: t('availableChannels.columns.name'),
  description: t('availableChannels.columns.description'),
  platform: t('availableChannels.columns.platform'),
  groups: t('availableChannels.columns.groups'),
  supportedModels: t('availableChannels.columns.supportedModels'),
}))

const filteredChannels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return channels.value
  return channels.value
    .map((ch) => {
      const nameHit = ch.name.toLowerCase().includes(q)
      const descHit = (ch.description || '').toLowerCase().includes(q)
      if (nameHit || descHit) return ch
      const matchingSections = ch.platforms.filter(
        (p) =>
          p.platform.toLowerCase().includes(q) ||
          p.groups.some((g) => g.name.toLowerCase().includes(q)) ||
          p.supported_models.some((m) => m.name.toLowerCase().includes(q)),
      )
      if (matchingSections.length === 0) return null
      return { ...ch, platforms: matchingSections }
    })
    .filter((ch): ch is UserAvailableChannel => ch !== null)
})

function extractMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string' && m) return m
  }
  return fallback
}

async function loadChannels() {
  loading.value = true
  try {
    const [list, rates] = await Promise.all([
      userChannelsAPI.getAvailable(),
      userChannelsAPI.getUserGroupRates().catch((err: unknown) => {
        // 专属倍率失败不阻塞渠道展示——降级为仅显示默认倍率.
        console.error('[plugin-channel-management] failed to load user group rates:', err)
        return {} as Record<number, number>
      }),
    ])
    channels.value = list
    userGroupRates.value = rates
  } catch (err: unknown) {
    sdk.notify.error(extractMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadChannels)
</script>
