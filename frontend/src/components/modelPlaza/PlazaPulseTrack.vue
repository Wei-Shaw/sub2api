<template>
  <!--
    分组健康脉冲：一个渠道监控一条，颜色与顶栏图例**严格一一对应**（绿 / 黄 / 红，只有三种）。

    ⚠️ 每根柱子 = **一次真实探测**，不是一段时间桶。
    后端只返回**最近 300 次探测**（与页面顶部选的时间档位无关，最多 300 条、按时间升序），因此：
      · 探测间隔 300 秒的渠道在「近 30 分钟」里就只有 5~6 根柱子，而不是 30 格里 25 格灰；
      · 渠道监控被关掉的那段时间没有任何探测记录 → 不产生柱子，相邻两次探测直接相连；
      · 渠道开着却挂掉时探测照常发生、状态是 failed/error → 画成红色（红色只代表「这次探测失败」）；
      · 因此**不存在**灰色「样本不足」柱，第四态已从本页语义中取消。

    ⚠️⚠️ 几何是「**固定槽位 + 柱子等宽 + 右侧对齐**」，照抄官方原版「近 60 次」：
      整行恒按上限 300 个槽位均分（`PLAZA_PRO_PULSE_SLOTS`），柱子宽度**与这张卡实际有几条数据无关**。
      数据不满 300 条时，柱数变少、每根宽度不变，**左侧空着**（官方是补灰色占位柱，本页留白）。
      这样「近 1 小时只有 4 次探测」的渠道就是右侧 4 根细柱 + 左侧一大片空白，
      既能一眼看出「采到的样本少」，又能让不同渠道的柱宽、时间刻度保持一致、可以横向比较。
      绝不按实际条数等分铺满 —— 那会让样本少的渠道被拉成几根巨宽柱子，看着像「很正常」。
  -->
  <div
    class="pulse-track"
    :class="{ 'pulse-track--loading': loading }"
    :style="{ '--pulse-slots': PLAZA_PRO_PULSE_SLOTS }"
    role="img"
    :aria-label="ariaLabel"
    data-testid="pulse-track"
  >
    <span v-if="loading" class="pulse-skeleton" aria-hidden="true"></span>
    <template v-else>
      <span
        v-for="(bar, index) in bars"
        :key="bar.key"
        class="pulse-bar"
        :class="`bar-${bar.state}`"
        :title="bar.title"
        :data-band="bar.state"
        :data-index="index"
      ></span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { PLAZA_PRO_PULSE_SLOTS, type PlazaProPulseBucket } from '@/api/modelPlazaPro'

/**
 * 槽位状态 → 图例三色。
 *
 * ⚠️ 这里**只认粗粒度状态**，不再按 `health.score` 走官方 11 档渐变：
 * 官方那套是「4 个图例点 + 11 档色值」，图例与实际颜色对不上（绿色就有 4 档），
 * 看图的人只会困惑「到底几种颜色」。本页统一成图例声明的 3 种，图例即真相。
 *
 * ⚠️ 判定「有没有样本」**只看后端 health，绝不能看计数**：用户端
 * `/channel-monitor-v2/matrix` 会把绝对计数（request_count / rpm / tpm）统一脱敏成 0
 * （handler 把 admin 参数硬编码为 false），而 `health.score` / `health.overall` /
 * `success_rate` 在两个端点上都是真实值。若用计数判空，本页每一根柱子都会变成灰色。
 */
const KNOWN_STATES = new Set(['healthy', 'warning', 'critical'])

function normalizedState(bucket: PlazaProPulseBucket): string {
  return KNOWN_STATES.has(bucket.state) ? bucket.state : 'unknown'
}

const props = withDefaults(
  defineProps<{
    /**
     * 按时间升序排列的**探测点**数组。
     *
     * ⚠️ 每个元素对应**一次真实探测**，不是一段时间桶：后端只返回窗口内实际发生过的探测
     * （最多 300 条），没有探测的时间段根本不产生元素。因此这里「有多少个元素就画多少根柱子」，
     * 也**不存在**灰色「样本不足」柱 —— 渠道监控被关掉的那段时间自然没有柱子，相邻两次探测直接相连。
     */
    buckets: PlazaProPulseBucket[]
    /** 加载中：渲染骨架条而不是空轨。 */
    loading?: boolean
    /** 整条轨道的无障碍描述。 */
    ariaLabel?: string
  }>(),
  { loading: false, ariaLabel: '' },
)

const { t, locale } = useI18n()

interface PulseBar {
  key: string
  state: string
  title: string
}

function formatDayClock(value: Date): string {
  return new Intl.DateTimeFormat(locale.value || undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(value)
}

/**
 * 探测点 tooltip：**一个点 = 一次真实探测**，所以只报「这次探测发生在什么时候 + 结果是什么」，
 * 不再报「某个时间区间的可用率」—— 数据语义变了，tooltip 也必须跟着变。
 *
 * 状态文案直接复用顶栏图例的三个 key，保证「图例说三种颜色，tooltip 就只可能出现三种结果」。
 */
const STATE_LABEL_KEYS: Record<string, string> = {
  healthy: 'modelPlazaPro.legend.healthy',
  warning: 'modelPlazaPro.legend.warning',
  critical: 'modelPlazaPro.legend.critical',
}

function bucketTitle(bucket: PlazaProPulseBucket): string {
  const at = new Date(bucket.start)
  const clock = Number.isNaN(at.getTime()) ? bucket.start : formatDayClock(at)
  const state = normalizedState(bucket)
  const labelKey = STATE_LABEL_KEYS[state]
  const label = labelKey ? t(labelKey) : state
  return `${t('modelPlazaPro.pulse.checkedAt', { time: clock })} · ${label}`
}

const bars = computed<PulseBar[]>(() =>
  (props.buckets ?? []).map((bucket, index) => ({
    key: bucket.start || `bucket-${index}`,
    state: normalizedState(bucket),
    title: bucketTitle(bucket),
  })),
)
</script>

<style scoped>
.pulse-track {
  /*
    槽位数量由 `PLAZA_PRO_PULSE_SLOTS` 通过行内样式注入（**一份真相**，见 modelPlazaPro.ts），
    它等于后端单条曲线的点数上限。柱子宽度只由这个常量决定，**与实际探测条数无关** ——
    这正是「官方原版近 60 次」的关键：官方也是固定 60 个槽位、柱子等宽，数据不满就用
    灰色占位柱补齐。本页与官方的差别只有一处：**不补灰**，没数据的槽位直接留白。

    ⚠️ 历史教训：曾经用 `repeat(N, minmax(0, 1fr))` 让柱子「等分铺满整行」，
    结果「近 1 小时只有 4 次探测」的渠道被拉成 4 根巨宽的柱子 —— 宽度随流量变化，
    既看不出数据量差异，也没法在不同渠道之间横向比较。固定槽位才是对的。
  */
  --pulse-gap: 1px;
  display: flex;
  /* 右对齐：最旧在左、最新在右（与官方横轴一致），未填满的槽位留在左侧空着。 */
  justify-content: flex-end;
  align-items: stretch;
  gap: var(--pulse-gap);
  width: 100%;
  height: 16px;
  min-width: 0;
  /* 槽位挤满整行时最后一根可能压到边缘，裁掉而不是把卡片撑破 */
  overflow: hidden;
}

.pulse-track--loading {
  display: block;
}

.pulse-skeleton {
  display: block;
  width: 100%;
  height: 100%;
  border-radius: 2px;
  background: rgb(229 231 235);
  animation: pulse-track-shimmer 1.4s ease-in-out infinite;
}

.dark .pulse-skeleton {
  background: rgb(55 65 81);
}

@keyframes pulse-track-shimmer {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}

/*
  **固定宽度**：把整行按上限 300 格均分（扣掉 299 条柱缝）。

  ⚠️ 绝不能用 `flex: 1` / `1fr` 让柱子去「铺满」剩余空间 —— 那样数据越少柱子越宽，
  不同渠道之间就没法横向比较了。数据不足 300 条时：柱数变少、**每根宽度不变**、左侧留白。
  圆角 2px 对齐官方 `rounded-sm`。
*/
.pulse-bar {
  display: block;
  flex: 0 0 auto;
  width: calc((100% - (var(--pulse-slots) - 1) * var(--pulse-gap)) / var(--pulse-slots));
  min-width: 0;
  height: 100%;
  border-radius: 2px;
}

/*
  三色与顶栏图例（`.legend-good/warn/bad`）**逐值相同** —— 图例说三种，柱子就只有三种。
  调色（主人 2026-09-30 明确要求）：
    · 红 = **纯红** `#ef4444`（red-500）。原先用的 `#fb7185` 是 rose-400，偏粉，不够醒目；
    · 黄 = **更明亮** `#fde047`（yellow-300），比原先的 `#facc15`（yellow-400）更亮；
    · 绿 = **更明亮** `#22c55e`（green-500），比原先的 `#16a34a`（green-600）更亮。
  ⚠️ 改这里必须同步改 `ModelPlazaProView.vue` 里的 `.legend-*`，否则图例与柱子会对不上色。
*/
.bar-healthy {
  background: #22c55e;
}
.bar-warning {
  background: #fde047;
}
.bar-critical {
  background: #ef4444;
}
/* 无样本：中性灰，绝不画成红 */
.bar-unknown {
  background: #d1d5db;
}

.dark .bar-unknown {
  background: #4b5563;
}
</style>
