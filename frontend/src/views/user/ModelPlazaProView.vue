<template>
  <AppLayout>
    <div class="space-y-2 pb-12">
      <!--
        工具栏：时间范围 / 平台 / 分组筛选 + 重置 + 可用率图例 + 刷新。

        ⚠️ 这里**刻意既不带标题、也不套卡片**：
        1. 页面级标题与描述由路由的 `titleKey` / `descriptionKey` 交给布局渲染（见 router/index.ts），
           所以内容区再写一遍标题就是重复的「模型广场 Pro」，已按主人要求删除。
        2. 筛选区做成一条裸排、与「账号管理」等页面的 `TablePageLayout #filters` 一致
           （那边该插槽也是不带 card 的裸 div，只有表格才套 .card），
           这样页面顶部高度压到最低，也不再有一层多余的边框与阴影。

        ⚠️ 这里是**刻意不做 `sticky`** 的：主人明确要求「鼠标往下滚动的时候，这块要跟着往上走，
        不要一直悬浮着挡着」。所以工具栏就是普通一行，随页面滚走。

        历史坑（若将来有人想改回悬浮，务必先读这段）：官方顶栏是 `AppHeader` 的
        `sticky top-0 z-30`，高度固定 `h-16`（64px）。本工具栏曾经写 `sticky top-0 z-20`，
        结果被 z-30 的官方顶栏压掉最上面 64px —— 标题行整块消失、只剩半透明残影，
        滚动后看起来就是「上面空了一大片」。当时靠 `sticky top-16` 让位修好；
        现在按主人要求直接取消悬浮，这个冲突源也就不存在了。
      -->
      <div
        class="monitor-toolbar flex flex-nowrap items-center gap-1.5 sm:gap-2"
        data-testid="pro-toolbar"
      >
        <!--
          可横向滚动的筛选区。
          ⚠️ `overflow-x-auto` 会把 `overflow-y` **连带强制成 auto**（CSS 规则：只要有一个轴不是
          visible，另一个轴的 visible 就会被算成 auto），于是任何绝对定位的浮层都会被这个容器
          的高度裁掉。图例与自动刷新的下拉菜单就踩过这个坑：下拉只剩第一行「启用自动刷新」，
          30/60/120 三档全被切没了。**所以它们必须放在这个 div 外面。**
        -->
        <div class="flex min-w-0 flex-1 flex-nowrap items-center gap-1.5 overflow-x-auto sm:gap-2">
        <div class="tabs inline-flex shrink-0" role="group" :aria-label="t('modelPlazaPro.timeRange')">
          <button
            v-for="option in rangeOptions"
            :key="option.value"
            type="button"
            class="tab !px-2 !py-1 text-xs sm:!px-2.5"
            :class="range === option.value ? 'tab-active' : ''"
            :data-range="option.value"
            @click="setRange(option.value)"
          >
            {{ option.label }}
          </button>
        </div>

        <span
          class="mx-0.5 hidden h-5 w-px shrink-0 bg-gray-200 dark:bg-dark-700 sm:block"
          aria-hidden="true"
        ></span>

        <FilterMultiSelect
          v-model="selectedPlatforms"
          compact
          :label="t('modelPlazaPro.filters.platform')"
          :all-label="t('modelPlazaPro.filters.allPlatforms')"
          :options="platformOptions"
        />
        <FilterMultiSelect
          v-model="selectedGroupIds"
          compact
          :label="t('modelPlazaPro.filters.group')"
          :all-label="t('modelPlazaPro.filters.allGroups')"
          :options="groupOptions"
        />
        <button
          type="button"
          class="btn btn-ghost btn-sm shrink-0 !px-2 !py-1 text-xs"
          :disabled="!filterActive"
          :class="!filterActive ? 'opacity-40' : ''"
          @click="clearFilters"
        >
          {{ t('modelPlazaPro.filters.clear') }}
        </button>

        </div>

        <!--
          右侧固定区：图例 + 自动刷新。
          刻意放在上面那个滚动容器**外面** —— 里面的 `overflow-x-auto` 会把下拉菜单裁掉。

          可用率图例：**只有三色**。
          第四态「样本不足」（灰）已随脉冲曲线改成「一次探测一个点」而取消 ——
          曲线上每个点都对应一次真实探测，渠道监控关掉的那段时间根本不产生点，
          所以再也不会出现灰色格子，图例也就不该再列出这一项。
        -->
        <div
          class="flex shrink-0 items-center gap-x-3 text-[11px] text-gray-500 dark:text-gray-400"
          :aria-label="t('modelPlazaPro.legend.aria')"
        >
          <span class="inline-flex items-center gap-1.5">
            <i class="legend-dot legend-good"></i>{{ t('modelPlazaPro.legend.healthy') }}
          </span>
          <span class="inline-flex items-center gap-1.5">
            <i class="legend-dot legend-warn"></i>{{ t('modelPlazaPro.legend.warning') }}
          </span>
          <span class="inline-flex items-center gap-1.5">
            <i class="legend-dot legend-bad"></i>{{ t('modelPlazaPro.legend.critical') }}
          </span>
        </div>

        <!--
          自动刷新开关：官方 `AutoRefreshButton` 原样复用（点开有 30 / 60 / 120 秒三档 +
          实时倒计时，开关与档位记在 localStorage 里，页面切到后台自动暂停）。

          手动刷新按钮已按主人要求删除：有了自动刷新它纯属重复，还占地方。
          需要立刻刷一次时，点这个控件切一下档位即可（切档会把倒计时归零重来）。
        -->
        <AutoRefreshButton
          :enabled="autoRefresh.enabled.value"
          :interval-seconds="autoRefresh.intervalSeconds.value"
          :countdown="autoRefresh.countdown.value"
          :intervals="autoRefresh.intervals"
          @update:enabled="autoRefresh.setEnabled"
          @update:interval="autoRefresh.setInterval"
        />
      </div>

      <!--
        降级提示：健康脉冲与定价各自独立，一侧不可用时只提示、不打断另一侧浏览。
        `notices` 在两侧都拿不到数据时为空，由下面的整页错误态接管。
      -->
      <div
        v-for="notice in notices"
        :key="notice.key"
        class="flex items-start gap-2 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-xs text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
        role="status"
        aria-live="polite"
      >
        <Icon name="infoCircle" size="xs" class="mt-0.5 h-3.5 w-3.5 shrink-0" />
        <span>
          <strong v-if="notice.title" class="font-semibold">{{ notice.title }}：</strong>{{ notice.text }}
        </span>
      </div>

      <!-- 首屏骨架 -->
      <div v-if="loading && !hasAnySource" class="space-y-4" aria-hidden="true">
        <div
          v-for="i in 3"
          :key="i"
          class="h-32 animate-pulse rounded-2xl bg-gray-50 dark:bg-dark-900/30"
        ></div>
      </div>

      <!-- 模型广场 Pro 自身未启用（直接访问 URL 时） -->
      <div v-else-if="!proEnabled" class="rounded-2xl border border-gray-100 bg-white py-8 dark:border-dark-700/50 dark:bg-dark-800/50">
        <EmptyState :title="t('modelPlazaPro.disabledTitle')" :description="t('modelPlazaPro.disabled')" />
      </div>

      <!--
        依赖的官方开关未开启、且健康数据也没拿到 → 整页只剩这一条线索。
        官方 /model-plaza 接口在 model_plaza_enabled=false 时返回 404。
        这里必须点名是「模型广场」开关，不能报成「Pro 未启用」——否则管理员会去开一个本来就开着的开关。
      -->
      <div
        v-else-if="!hasAnySource && plazaGateClosed"
        class="rounded-2xl border border-gray-100 bg-white py-8 dark:border-dark-700/50 dark:bg-dark-800/50"
      >
        <EmptyState
          :title="t('modelPlazaPro.plazaGateTitle')"
          :description="t('modelPlazaPro.plazaGate')"
        />
      </div>

      <!-- 两侧数据源都没拿到 → 这才是真正的整页错误态 -->
      <div
        v-else-if="!hasAnySource"
        class="rounded-2xl border border-red-200 bg-red-50 px-5 py-8 text-center text-sm text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
        role="alert"
      >
        <p>{{ loadError || t('modelPlazaPro.monitorLoadFailed') }}</p>
        <button type="button" class="btn btn-secondary btn-sm mt-3" @click="reload(false)">
          {{ t('modelPlazaPro.retry') }}
        </button>
      </div>

      <!-- 空态 -->
      <div
        v-else-if="!visibleCards.length"
        class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
      >
        {{ filterActive ? t('modelPlazaPro.emptyFiltered') : t('modelPlazaPro.empty') }}
      </div>

      <!--
        卡片单元 = 一个「渠道监控」= 一个 (platform, group)。
        刻意不按模型拆卡：分组有多少个模型只影响定价表行数，绝不裂成多张卡 ——
        否则「分组有 10 个模型、其中 7 个没有监控数据」就会造出 7 张空白卡。
      -->
      <!-- 一行一个：一个渠道监控 = 一整行的卡片，不做多列并排 -->
      <div class="grid grid-cols-1 gap-2">
        <article
          v-for="card in visibleCards"
          :key="card.key"
          class="flex flex-col overflow-hidden rounded-2xl border bg-white shadow-card dark:bg-dark-800/50"
          :class="[platformBorderStrongClass(card.platform)]"
          :data-group-id="card.groupId"
          :data-group-name="card.name"
        >
          <!--
            第一行（**全部挤在一行**，这是压缩的关键）：
              渠道名（放大加粗、最显眼） + 徽章区（平台/分组/模型数/独占/订阅） + 成功率（右侧、放大着色）

            压缩前这里是「徽章一行 + 渠道名一行 + 描述一行 + 成功率右侧」共 96px；
            改成一行后只剩 ~28px。原描述行（`card.group.description`，形如
            `gpt-6-luna|gpt-5.6-luna：官方1折`）移到渠道名的 title 悬浮提示里，不再占高度。
          -->
          <div class="flex items-center gap-2 px-3 pb-1 pt-2.5">
            <!-- 渠道名：全卡最大最粗的元素，一眼就能定位 -->
            <h2
              class="max-w-[15rem] shrink-0 truncate text-[15px] font-bold leading-6 text-gray-900 dark:text-white"
              :title="[card.name, card.group?.description].filter(Boolean).join(' · ')"
            >
              {{ card.name }}
            </h2>

            <!-- 徽章区固定一行高：徽章再多也不换行，否则会把脉冲条往下挤、一排卡片对不齐 -->
            <div class="flex h-5 min-w-0 flex-1 flex-nowrap items-center gap-1.5 overflow-hidden">
              <!-- 平台徽章 -->
              <span
                class="inline-flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium"
                :class="platformBadgeLightClass(card.platform)"
              >
                <PlatformIcon :platform="card.platform as GroupPlatform" size="xs" />
                {{ platformLabel(card.platform) }}
              </span>
              <!-- 分组 + 倍率徽章（复用官方 GroupBadge） -->
              <GroupBadge
                v-if="card.group"
                :name="card.group.name"
                :platform="card.group.platform as GroupPlatform"
                :subscription-type="(card.group.subscription_type || 'standard') as SubscriptionType"
                :rate-multiplier="card.group.rate_multiplier"
                :user-rate-multiplier="card.group.user_rate_multiplier ?? null"
                :peak-rate-enabled="card.group.peak_rate_enabled"
                :peak-start="card.group.peak_start"
                :peak-end="card.group.peak_end"
                :peak-rate-multiplier="card.group.peak_rate_multiplier"
                always-show-rate
              />
              <!-- 模型数：只决定定价表行数，不决定卡片数量 -->
              <span class="badge badge-gray shrink-0">
                {{ t('modelPlazaPro.group.models', { count: card.models.length }) }}
              </span>
              <span
                v-if="card.group?.is_exclusive"
                class="inline-flex shrink-0 items-center gap-1 rounded-md bg-purple-50 px-1.5 py-0.5 text-[10px] font-medium text-purple-600 dark:bg-purple-900/20 dark:text-purple-400"
              >
                <Icon name="shield" size="xs" class="h-3 w-3" />
                {{ t('modelPlazaPro.badge.exclusive') }}
              </span>
              <span
                v-if="card.group?.subscription_type === 'subscription'"
                class="inline-flex shrink-0 items-center rounded-md bg-violet-50 px-1.5 py-0.5 text-[10px] font-medium text-violet-600 dark:bg-violet-900/20 dark:text-violet-400"
              >
                {{ t('modelPlazaPro.badge.subscription') }}
              </span>
            </div>

            <!-- 成功率：右侧，数字放大加粗并按健康度着色（绿/黄/红），是全卡第二显眼的元素 -->
            <div class="flex shrink-0 items-baseline gap-1" :title="availabilityTitle">
              <span class="text-[10px] font-semibold uppercase tracking-wide text-gray-400 dark:text-dark-500">
                {{ availabilityLabel }}
              </span>
              <!--
                时间档位小灰字（**只在 V1 主动探测模式出现**）。

                为什么必须标出来：V1 的柱子和成功率是**两种时间口径** ——
                柱子恒为「最近 300 次探测」（与档位无关），成功率才是「所选窗口内的统计」。
                不标档位，用户会以为切到「近 1 小时」连柱子也只剩 1 小时的，看不懂为什么
                还有几百根柱子；标出来就能对上「这个百分比是按近 1 小时算的」。

                V2 被动聚合模式不显示：那边的 buckets 本来就是窗口内的桶，不存在两个口径。

                ⚠️ 必须是**行内极小灰字**：卡片是紧凑三行结构（渠道名行 / 柱子行 / 定价行），
                这个小灰字绝不能新增一行、不能撑高卡片。也**不能**加 `font-mono`
                —— 卡片右上角那个 `.font-mono` 是成功率的数字（测试与样式都按它定位）。
              -->
              <span
                v-if="effectiveMonitorSource === 'v1'"
                class="text-[9px] font-normal leading-none text-gray-400 dark:text-dark-500"
                :title="t('modelPlazaPro.availabilityProbeRangeHint')"
                :data-testid="`availability-range-${card.groupId}`"
                >{{ rangeLabel }}</span
              >
              <span class="font-mono text-base font-bold leading-6" :class="availabilityClass(card)">
                {{ availabilityText(card) }}
              </span>
            </div>
          </div>

          <!--
            第二行：**直接就是柱子**。

            ⚠️ 柱子 = **最近 300 次探测**（后端已按这个口径取数），与工具栏选的时间档位
            **完全无关**：切档位只重新计算成功率，不会让柱子变少或重新采样。
            档位对柱子的唯一影响是「数据库里还没攒够 300 条时，柱子就是现有的全部条数」。

            原先这里还有一行「近 300 次探测」的计数说明（照抄官方 `monitorCommon.history60pts`），
            按主人要求取消 —— 槽位数恒定 300 是既定事实，没必要每张卡都复述一遍，省下 ~20px。
          -->
          <div class="px-3 pb-1.5">
            <PlazaPulseTrack
              v-if="cardBuckets(card).length"
              :buckets="cardBuckets(card)"
              :loading="loading && !hasAnySource"
              :aria-label="t('modelPlazaPro.pulse.groupAria', { group: card.name })"
            />
            <p
              v-else
              class="flex h-4 items-center text-xs text-gray-400 dark:text-dark-500"
              :data-testid="`no-data-${card.groupId}`"
            >
              {{ card.degraded ? t('modelPlazaPro.noMonitor') : t('modelPlazaPro.noData') }}
            </p>
          </div>

          <!-- 折叠：该分组的全部模型定价（一个分组一份定价，与卡片一一对应） -->
          <div
            v-if="card.group"
            class="mt-auto border-t border-gray-100 px-3 dark:border-dark-700/60"
          >
            <button
              type="button"
              class="flex w-full items-center gap-1.5 py-1 text-xs font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
              :aria-expanded="expanded.has(card.key)"
              :aria-controls="panelId(card)"
              :data-testid="`toggle-pricing-${card.groupId}`"
              @click="togglePricing(card.key)"
            >
              <Icon
                name="chevronDown"
                size="xs"
                class="h-3.5 w-3.5 transition-transform"
                :class="expanded.has(card.key) ? 'rotate-180' : ''"
              />
              {{
                expanded.has(card.key)
                  ? t('modelPlazaPro.pricing.hide')
                  : t('modelPlazaPro.pricing.showAll', { count: card.models.length })
              }}
            </button>
          </div>

          <!-- 定价明细：复用官方 PlazaModelPricingTable（用法照搬 PlazaGroupSection） -->
          <div
            v-if="card.group && expanded.has(card.key)"
            :id="panelId(card)"
            class="border-t border-gray-100 dark:border-dark-700/60"
            :data-testid="`pricing-panel-${card.groupId}`"
          >
            <PlazaModelPricingTable
              :models="card.models"
              :platform="card.group.platform"
              :rate-multiplier="card.group.rate_multiplier"
              :user-rate-multiplier="card.group.user_rate_multiplier ?? null"
              :image-rate-independent="card.group.image_rate_independent"
              :image-rate-multiplier="card.group.image_rate_multiplier"
              :video-rate-independent="card.group.video_rate_independent"
              :video-rate-multiplier="card.group.video_rate_multiplier"
              :peak-window="peakWindows.get(card.groupId) ?? ''"
              :peak-rate-multiplier="card.group.peak_rate_multiplier"
            />
          </div>
        </article>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useAutoRefresh } from '@/composables/useAutoRefresh'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import PlazaModelPricingTable from '@/components/modelPlaza/PlazaModelPricingTable.vue'
import PlazaPulseTrack from '@/components/modelPlaza/PlazaPulseTrack.vue'
import FilterMultiSelect from '@/features/channel-monitor-v2/FilterMultiSelect.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import { formatMonitorPercent } from '@/features/channel-monitor-v2/monitorFormat'
import { platformBadgeLightClass, platformBorderStrongClass, platformLabel } from '@/utils/platformColors'
import { formatPeakRateWindow, hasPeakRate, serverTimezoneLabel } from '@/utils/peak-rate'
import type { GroupPlatform, SubscriptionType } from '@/types'
import {
  PLAZA_PRO_RANGE_SPECS,
  buildPlazaProCards,
  buildPulseBuckets,
  getModelPlaza,
  getModelPlazaProMonitorMatrix,
  plazaProMonitorSource,
  type ModelPlazaGroup,
  type ModelPlazaResponse,
  type PlazaProCard,
  type PlazaProPulseBucket,
  type PlazaProRange,
} from '@/api/modelPlazaPro'
import type { MonitorMatrixResponse } from '@/api/channelMonitorV2'
import type { PlazaProMonitorSource } from '@/api/modelPlazaPro'

/**
 * 默认窗口：近 1 小时 / 1 分钟一桶（60 根柱条）。
 *
 * ⚠️ 这个默认值**每次进页面都必须生效**：主人要求「用户切到其他页面再切回来，
 * 不管他之前选的是哪一个时间，都默认显示近一小时」。
 *
 * 当前机制天然满足 —— 全项目没有任何 `<keep-alive>`（已全仓 grep 确认），
 * 路由切换会销毁本组件，再进来时 `range` 重新从这个常量初始化。
 *
 * ⚠️ 因此**禁止**做下面任何一件事，否则这条需求立刻失效：
 *   · 给本视图或 `<router-view>` 加 `<keep-alive>`
 *   · 把 `range` 持久化到 localStorage / sessionStorage / pinia / URL query
 * 如果将来确实需要缓存页面（性能原因加了 keep-alive），必须同时补 `onActivated`
 * 把 `range` 重置回 `DEFAULT_RANGE`。回归测试见
 * `views/user/__tests__/ModelPlazaProView.spec.ts` 的「重新进入页面回到近 1 小时」。
 */
const DEFAULT_RANGE: PlazaProRange = '1h-1m'

const { t } = useI18n()
const appStore = useAppStore()

const range = ref<PlazaProRange>(DEFAULT_RANGE)
const selectedPlatforms = ref<string[]>([])
const selectedGroupIds = ref<string[]>([])

const data = ref<ModelPlazaResponse | null>(null)
const matrix = ref<MonitorMatrixResponse | null>(null)
const loading = ref(true)
const refreshing = ref(false)

/**
 * 自动刷新：**整机制复用官方 V1 渠道状态页那套**（`useAutoRefresh` + `AutoRefreshButton`），
 * 档位、倒计时、localStorage 记忆、「页面切到后台就暂停」的行为与官方逐字一致，
 * 不新造第二套刷新逻辑，也不改官方任何一个文件。
 *
 * 官方对照（`views/user/ChannelStatusV1View.vue`）：
 *   intervals: [30, 60, 120] / defaultEnabled: true / shouldPause: () => document.hidden || loading
 * 这里只多一个 `refreshing` 条件：手动刷新还在飞的时候不要叠一次自动刷新。
 */
const autoRefresh = useAutoRefresh({
  storageKey: 'model-plaza-pro-auto-refresh',
  intervals: [30, 60, 120] as const,
  defaultInterval: 60,
  defaultEnabled: true,
  // 静默刷新：不闪骨架屏、不打断正在看的定价展开，柱子与成功率的数字原地更新。
  onRefresh: () => reload(false),
  shouldPause: () => document.hidden || loading.value || refreshing.value,
})

/** 手动刷新入口只留给「整页错误态」里的重试按钮（工具栏那个独立刷新按钮已删除）。 */
/** 广场侧非 404 的失败（500 / 网络）：健康卡照常渲染，只是没有模型名与定价。 */
const plazaFailed = ref(false)
const loadError = ref('')
/**
 * 官方 `/model-plaza` 接口返回 404 = 依赖的「模型广场」开关未开启。
 *
 * 注意：这里**不能**叫 `disabled`、更不能报成「模型广场 Pro 未启用」——本页自身的门禁由
 * `proEnabled` 判断。曾出现「Pro 开关明明开着、页面却说没启用」的误报，根因就是把官方
 * 门禁的 404 当成了本功能的门禁，把管理员指向了一个本来就开着的开关。
 */
const plazaGateClosed = ref(false)
/** 本页自身的功能开关（public settings 的 `model_plaza_pro_enabled`）。 */
const proEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlazaPro))
const monitorFailed = ref(false)
const expanded = ref<Set<string>>(new Set())

let controller: AbortController | null = null
let sequence = 0

const rangeOptions = computed(() =>
  PLAZA_PRO_RANGE_SPECS.map((spec) => ({ value: spec.token, label: t(spec.labelKey) })),
)

/** 广场分组（旁路数据：只提供分组名 / 模型清单 / 定价，**不决定卡片数量**）。 */
const plazaGroups = computed<ModelPlazaGroup[]>(() => data.value?.groups ?? [])
/** 两侧数据源只要有一侧可用，页面就不该整页报错。 */
const hasAnySource = computed(() => Boolean(data.value) || Boolean(matrix.value))
const filterActive = computed(
  () => selectedPlatforms.value.length > 0 || selectedGroupIds.value.length > 0,
)

/**
 * 卡片列表：**由监控矩阵驱动数量**，广场只作旁路 join。
 *
 * 传入 `!monitorFailed` 作为 `monitorAvailable`：只有监控整体不可用时，才退回
 * 「一个广场分组一张卡」的降级形态（卡片带 `degraded` 标记）。
 */
const cards = computed<PlazaProCard[]>(() =>
  buildPlazaProCards(matrix.value?.items, plazaGroups.value, !monitorFailed.value),
)

const platformOptions = computed(() =>
  [...new Set(cards.value.map((card) => card.platform).filter(Boolean))]
    .sort()
    .map((platform) => ({ value: platform, label: platformLabel(platform) })),
)

const selectedPlatformSet = computed(() => new Set(selectedPlatforms.value))
const groupOptions = computed(() =>
  cards.value
    .filter(
      (card) => selectedPlatformSet.value.size === 0 || selectedPlatformSet.value.has(card.platform),
    )
    .map((card) => ({
      value: String(card.groupId),
      label: `${platformLabel(card.platform)} / ${card.name}`,
    })),
)

const selectedGroupIdNumbers = computed(() =>
  selectedGroupIds.value.map(Number).filter((id) => Number.isInteger(id) && id > 0),
)

/** 展示用卡片（纯前端过滤，与矩阵请求口径保持一致）。 */
const visibleCards = computed(() => {
  let list = cards.value
  if (selectedPlatformSet.value.size > 0) {
    list = list.filter((card) => selectedPlatformSet.value.has(card.platform))
  }
  if (selectedGroupIdNumbers.value.length > 0) {
    const allowed = new Set(selectedGroupIdNumbers.value)
    list = list.filter((card) => allowed.has(card.groupId))
  }
  return list
})

/**
 * 降级横幅：两侧数据源各自独立，一侧不可用时只提示、不打断另一侧浏览。
 * 两侧都没拿到数据时返回空数组 —— 交给下面的整页错误态，避免横幅和错误态同时出现。
 */
interface ProNotice {
  key: string
  /** 可选粗体引题；「点名是哪个开关」的信息不能只放在正文里。 */
  title?: string
  text: string
}

const notices = computed<ProNotice[]>(() => {
  if (!hasAnySource.value) return []
  const list: ProNotice[] = []
  if (plazaGateClosed.value) {
    list.push({
      key: 'plaza-gate',
      title: t('modelPlazaPro.plazaGateTitle'),
      text: t('modelPlazaPro.plazaGate'),
    })
  } else if (plazaFailed.value) {
    list.push({ key: 'plaza-failed', text: t('modelPlazaPro.plazaLoadFailed') })
  }
  if (monitorFailed.value) {
    list.push({ key: 'monitor-failed', text: t('modelPlazaPro.monitorLoadFailed') })
  }
  return list
})

/** 高峰窗口描述（复用官方 peak-rate 文案与时区标注）。 */
const peakWindows = computed(() => {
  const map = new Map<number, string>()
  const tzLabel = serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  for (const group of plazaGroups.value) {
    if (!hasPeakRate(group)) continue
    map.set(group.id, formatPeakRateWindow(group, tzLabel))
  }
  return map
})

/** 分组汇总脉冲桶：一个分组一条走势，不再按模型拆成多条。 */
function cardBuckets(card: PlazaProCard): PlazaProPulseBucket[] {
  return buildPulseBuckets(card.row)
}

/**
 * 卡片右上角的可用率文案。
 *
 * ⚠️ 窗口内一条探测记录都没有时（`successRate == null`）**不要再造一个「样本不足」专用文案**：
 * 脉冲曲线已经取消第四态，卡片这里统一落到 `noData`（与卡片下方那句占位文案同一个 key），
 * 否则会出现「曲线上一个灰格都没有、卡片却写着样本不足」的自相矛盾。
 */
function availabilityText(card: PlazaProCard): string {
  if (card.successRate != null) return formatMonitorPercent(card.successRate)
  return card.degraded ? t('modelPlazaPro.noMonitor') : t('modelPlazaPro.noData')
}

function availabilityClass(card: PlazaProCard): string {
  if (card.successRate == null) return 'text-gray-400 dark:text-dark-500'
  if (card.successRate >= 0.99) return 'text-emerald-600 dark:text-emerald-400'
  if (card.successRate >= 0.95) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

/**
 * 可用率表头文案：**V1 与 V2 的分母根本不同，标签必须跟着换**。
 *
 *   - V2（被动）：真实用户请求的成功率 —— 分母是「用户请求数」
 *   - V1（主动）：探针合成请求的成功率 —— 分母是「探测次数」
 *
 * 两个数字不可横向比较，用同一个「可用率」标签会把探测成功率误读成用户体验。
 */
const availabilityLabel = computed(() =>
  effectiveMonitorSource.value === 'v1'
    ? t('modelPlazaPro.availabilityProbe')
    : t('modelPlazaPro.availability'),
)

const availabilityTitle = computed(() =>
  effectiveMonitorSource.value === 'v1'
    ? t('modelPlazaPro.availabilityProbeHint')
    : t('modelPlazaPro.availabilityHint'),
)

/**
 * 当前时间档位的展示标签（如「近 1 小时」），显示在「探测成功率」旁边。
 *
 * ⚠️ 它标注的是**成功率的统计窗口**，不是柱子的取数范围：V1 的脉冲色块恒为
 * 「最近 300 次探测」，与所选档位无关（后端 `ChannelMonitorV1MatrixPointLimit`）。
 * 两句话都写在小灰字的悬浮提示（`availabilityProbeRangeHint`）里，避免误读。
 */
const rangeLabel = computed(() => {
  const spec = PLAZA_PRO_RANGE_SPECS.find((option) => option.token === range.value)
  return spec ? t(spec.labelKey) : ''
})

/** 稳定的定价面板 id（以分组为单元，与卡片一一对应）。 */
function panelId(card: PlazaProCard): string {
  return `plaza-pro-pricing-${card.platform}-${card.groupId}`
}

function togglePricing(key: string) {
  const next = new Set(expanded.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expanded.value = next
}

function setRange(value: PlazaProRange) {
  range.value = value
}

function clearFilters() {
  selectedPlatforms.value = []
  selectedGroupIds.value = []
}

function isCanceled(error: unknown): boolean {
  const e = error as { name?: string; code?: string } | null
  return e?.name === 'CanceledError' || e?.name === 'AbortError' || e?.code === 'ERR_CANCELED'
}

/**
 * 当前渠道监控数据源，取自公开设置 `channel_monitor_mode`。
 *
 * 后端把 V1/V2 做成**互斥开关**（`ActiveProbesAllowed()` 要 `mode==v1`、
 * `PassiveAggregationAllowed()` 要 `mode==v2`），所以同一时刻只会存在一种数据：
 *
 *   - `v2` 被动聚合 → `/channel-monitor-v2/matrix`（真实用户调用产生数据，
 *     **冷门分组没有柱子**）
 *   - `v1` 主动探测 → Pro 页新增的 `/channel-monitors/matrix`（探针主动打上游，
 *     **与用户流量无关**；明细保留 30 天，同样支持 6 档窗口）
 *
 * 两个端点的响应 JSON 完全同形，所以下方所有渲染逻辑都不需要分支。
 */
const monitorSource = computed(() =>
  plazaProMonitorSource(appStore.cachedPublicSettings?.channel_monitor_mode),
)

/**
 * **实际生效**的数据源，由取数层回报。
 *
 * 为什么不直接用 `monitorSource`：设置值经服务端注入的 `__APP_CONFIG__` 到达前端，
 * 而它随 HTML 一起被缓存，所以可能落后于后台的真实模式。取数层会在「403 或 0 行」时
 * 自动换另一个源重试，并把真正取到数据的那个源回报上来。
 *
 * 文案必须跟这个值走：可用率（用户请求成功率）与探测成功率是两个不同的口径，
 * 用错标签会把探测成功率误读成用户体验。
 */
const resolvedSource = ref<PlazaProMonitorSource | null>(null)

/** 文案用的数据源：已取到数据就用实际生效的，否则退回设置推测值。 */
const effectiveMonitorSource = computed(() => resolvedSource.value ?? monitorSource.value)

// 设置值一变就作废旧的回报值，避免换源后的第一句话术还挂在旧口径上。
watch(monitorSource, () => {
  resolvedSource.value = null
})

/**
 * 取监控汇总矩阵：`platform_group` 维度，一行 = 一个渠道监控 = 一张卡。
 * 按 `channel_monitor_mode` 自动在 V1 / V2 两个同形端点之间切换，
 * 并在设置过期选错端点时自愈换源（详见 `api/modelPlazaPro.ts`）。
 */
async function fetchMatrix(signal: AbortSignal): Promise<MonitorMatrixResponse> {
  const { response, source } = await getModelPlazaProMonitorMatrix(
    {
      range: range.value,
      platforms: selectedPlatforms.value,
      groupIds: selectedGroupIdNumbers.value,
    },
    monitorSource.value,
    signal,
  )
  resolvedSource.value = source
  return response
}

/**
 * 全量加载：广场（模型 / 定价）与监控（健康汇总）**并行，且各自独立降级**。
 *
 * 只有**两侧都拿不到数据**时才整页错误；任一侧成功都能渲染出卡片 ——
 * 监控成功 → 有健康脉冲的分组卡；广场成功 → 补上分组名 / 模型清单 / 定价。
 */
async function reload(showSpinner = true) {
  controller?.abort()
  const request = new AbortController()
  controller = request
  const id = ++sequence
  if (showSpinner) loading.value = true
  refreshing.value = true
  loadError.value = ''
  plazaGateClosed.value = false
  plazaFailed.value = false

  const [plaza, monitor] = await Promise.allSettled([
    getModelPlaza({ signal: request.signal }),
    fetchMatrix(request.signal),
  ])
  if (id !== sequence) {
    refreshing.value = false
    return
  }

  if (plaza.status === 'fulfilled') {
    data.value = plaza.value
  } else if (!isCanceled(plaza.reason)) {
    data.value = null
    // 404 = 官方「模型广场」门禁关闭：健康卡照常渲染，只是缺分组名与定价。
    plazaGateClosed.value = (plaza.reason as { status?: number } | null)?.status === 404
    plazaFailed.value = true
    if (!plazaGateClosed.value) {
      loadError.value = extractApiErrorMessage(plaza.reason, t('modelPlazaPro.loadFailed'))
      appStore.showError(loadError.value)
    }
  }

  if (monitor.status === 'fulfilled') {
    matrix.value = monitor.value
    monitorFailed.value = false
  } else if (!isCanceled(monitor.reason)) {
    matrix.value = null
    monitorFailed.value = true
    const text = extractApiErrorMessage(monitor.reason, t('modelPlazaPro.monitorLoadFailed'))
    if (!data.value) {
      // 两侧都挂了：只留一条错误文案交给整页错误态，不重复弹 toast。
      loadError.value = loadError.value || text
    } else {
      appStore.showError(text)
    }
  }

  if (id === sequence) {
    loading.value = false
    refreshing.value = false
  }
}

/** 只刷新监控：时间范围 / 筛选变化不影响广场的分组与定价。 */
async function reloadMatrix() {
  controller?.abort()
  const request = new AbortController()
  controller = request
  const id = ++sequence
  refreshing.value = true
  try {
    const next = await fetchMatrix(request.signal)
    if (id !== sequence) return
    matrix.value = next
    monitorFailed.value = false
  } catch (error) {
    if (id !== sequence || isCanceled(error)) return
    matrix.value = null
    monitorFailed.value = true
    appStore.showError(extractApiErrorMessage(error, t('modelPlazaPro.monitorLoadFailed')))
  } finally {
    if (id === sequence) refreshing.value = false
  }
}

watch(range, () => {
  if (hasAnySource.value) void reloadMatrix()
})

watch([selectedPlatforms, selectedGroupIds], () => {
  if (hasAnySource.value) void reloadMatrix()
})

onMounted(() => {
  void reload(true)
  // 官方同款：把「从 localStorage 读回来的开关状态」重新套用一次 —— 顺带启动定时器，
  // 这样上次开过自动刷新的用户下次进来就自动接着刷，不必再点一次。
  autoRefresh.setEnabled(autoRefresh.enabled.value)
})
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped>
.legend-dot {
  display: inline-block;
  height: 6px;
  width: 6px;
  flex: none;
  border-radius: 9999px;
}

/* 图例三色 **必须与 `PlazaPulseTrack.vue` 的 `.bar-*` 逐值相同**，否则图例与柱子对不上色。
   红用纯红（原 rose-400 #fb7185 偏粉）、黄与绿都调亮一档。 */
.legend-good {
  background: #22c55e;
}

.legend-warn {
  background: #fde047;
}

.legend-bad {
  background: #ef4444;
}

.legend-unknown {
  background: #d1d5db;
}

.dark .legend-unknown {
  background: #4b5563;
}
</style>
