<template>
  <!--
    A price list. The only job of this surface is that a column of numbers
    reads as a column: mono tabular figures, right aligned, one hairline under
    every row, one heavy rule under the header, and no zebra.

    What used to be here: a platform-hued wash behind the "your price" zone
    (`color-mix` off the platform accent), a second hue on hover, colored
    pills for the tier chips, and eight `dark:` pairs per row. Tinting a third
    of a pricing table is how a reader stops seeing the numbers.
  -->
  <div class="overflow-x-auto">
    <table class="table min-w-[54rem] table-fixed" data-testid="plaza-pricing-table">
      <colgroup>
        <col class="w-[22%]" />
        <col class="w-[11%]" />
        <col class="w-[11%]" />
        <col class="w-[13%]" />
        <col class="w-[11%]" />
        <col class="w-[11%]" />
        <col class="w-[13%]" />
        <col class="w-[8%]" />
      </colgroup>
      <thead>
        <!--
          Two-level header. Only the SECOND row carries the strong rule, so the
          page keeps exactly one heavy line; the zone labels above it are
          separated by a hairline. The two `rowspan` cells keep the strong
          border because their bottom edge lands on that same line.
        -->
        <tr>
          <th rowspan="2" scope="col" class="align-bottom">
            {{ t('modelPlaza.table.model') }}
          </th>
          <th
            colspan="3"
            scope="colgroup"
            class="border-b border-b-line-subtle border-l border-l-line text-center"
          >
            {{ t('modelPlaza.table.paidPrice') }}
            <span class="ml-1 font-normal normal-case tracking-normal">
              {{ t('modelPlaza.table.unitPerMillion') }}
            </span>
          </th>
          <th
            colspan="3"
            scope="colgroup"
            class="border-b border-b-line-subtle border-l border-l-line text-center"
          >
            {{ t('modelPlaza.table.officialPrice') }}
            <span class="ml-1 font-normal normal-case tracking-normal">
              {{ t('modelPlaza.table.unitPerMillion') }}
            </span>
          </th>
          <th rowspan="2" scope="col" class="is-numeric border-l border-l-line align-bottom">
            {{ t('modelPlaza.table.rate') }}
          </th>
        </tr>
        <tr>
          <th scope="col" class="is-numeric border-l border-l-line">
            {{ t('modelPlaza.table.input') }}
          </th>
          <th scope="col" class="is-numeric">{{ t('modelPlaza.table.output') }}</th>
          <th scope="col" class="is-numeric">{{ t('modelPlaza.table.cache') }}</th>
          <th scope="col" class="is-numeric border-l border-l-line">
            {{ t('modelPlaza.table.input') }}
          </th>
          <th scope="col" class="is-numeric">{{ t('modelPlaza.table.output') }}</th>
          <th scope="col" class="is-numeric">{{ t('modelPlaza.table.cache') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="m in sortedModels" :key="`${m.platform}:${m.name}`">
          <!-- Model name is an identifier, so it is mono like every other -->
          <!-- fixed-width value on the page. Badges are neutral: platform is a -->
          <!-- category, and a category is not a status. -->
          <td>
            <div class="flex flex-wrap items-center gap-1.5 py-1">
              <span class="font-mono text-xs text-ink">{{ m.name }}</span>
              <Badge v-if="platform && m.platform !== platform" caps>
                {{ platformLabel(m.platform) }}
              </Badge>
              <Badge v-if="billingMode(m) !== BILLING_MODE_TOKEN" caps>
                {{ billingModeLabel(m) }}
              </Badge>
            </div>
          </td>

          <!-- token billing: input / output (tiers inline) / cache (write, read) -->
          <template v-if="billingMode(m) === BILLING_MODE_TOKEN">
            <td class="is-numeric border-l border-l-line font-medium">
              <div v-if="tokenIntervals(m).length" class="space-y-0.5 py-1.5">
                <div v-for="(iv, idx) in tokenIntervals(m)" :key="idx" :class="TIER_ROW">
                  <span :class="TIER_LABEL">{{ tierLabel(iv) }}</span>
                  <NumCell v-bind="paidPerMillion(iv.input_price)" />
                </div>
              </div>
              <NumCell v-else v-bind="paidPerMillion(m.pricing?.input_price)" />
            </td>
            <td class="is-numeric font-medium">
              <div v-if="tokenIntervals(m).length" class="space-y-0.5 py-1.5">
                <div v-for="(iv, idx) in tokenIntervals(m)" :key="idx" :class="TIER_ROW">
                  <span :class="TIER_LABEL">{{ tierLabel(iv) }}</span>
                  <NumCell v-bind="paidPerMillion(iv.output_price)" />
                </div>
              </div>
              <NumCell v-else v-bind="paidPerMillion(m.pricing?.output_price)" />
            </td>
            <td class="is-numeric font-medium">
              <div v-if="hasCachePricing(m)" class="space-y-0.5 py-1.5">
                <div :class="TIER_ROW">
                  <span :class="TIER_LABEL">{{ t('modelPlaza.table.cacheWrite') }}</span>
                  <NumCell v-bind="paidPerMillion(m.pricing?.cache_write_price)" />
                </div>
                <div :class="TIER_ROW">
                  <span :class="TIER_LABEL">{{ t('modelPlaza.table.cacheRead') }}</span>
                  <NumCell v-bind="paidPerMillion(m.pricing?.cache_read_price)" />
                </div>
              </div>
              <NumCell v-else :value="null" />
            </td>
          </template>

          <!-- per-request / per-image billing: the paid zone merges, because a -->
          <!-- price per image has no input/output/cache decomposition. -->
          <template v-else>
            <td colspan="3" class="is-numeric border-l border-l-line font-medium">
              <div v-if="requestIntervals(m).length" class="space-y-0.5 py-1.5">
                <div v-for="(iv, idx) in requestIntervals(m)" :key="idx" :class="TIER_ROW">
                  <span :class="TIER_LABEL">{{ tierLabel(iv) }}</span>
                  <NumCell
                    v-bind="paidRequestPrice(m, iv.per_request_price)"
                    :unit="perUnitSuffix(m)"
                  />
                </div>
              </div>
              <NumCell
                v-else-if="m.pricing?.per_request_price != null"
                v-bind="paidRequestPrice(m, m.pricing.per_request_price)"
                :unit="perUnitSuffix(m)"
              />
              <NumCell v-else :value="null" />
            </td>
          </template>

          <!-- Official reference price (LiteLLM), never multiplied by the rate. -->
          <!-- One step down in size: the number the user pays leads. -->
          <td class="is-numeric border-l border-l-line text-xs">
            <NumCell v-bind="official(m.official_pricing?.input_price)" />
          </td>
          <td class="is-numeric text-xs">
            <NumCell v-bind="official(m.official_pricing?.output_price)" />
          </td>
          <td class="is-numeric text-xs">
            <div
              v-if="m.official_pricing && hasOfficialCache(m.official_pricing)"
              class="space-y-0.5 py-1.5"
            >
              <div :class="TIER_ROW">
                <span :class="TIER_LABEL">{{ t('modelPlaza.table.cacheWrite') }}</span>
                <NumCell v-bind="official(m.official_pricing.cache_write_price)" />
              </div>
              <div v-if="m.official_pricing.cache_write_1h_price != null" :class="TIER_ROW">
                <span :class="TIER_LABEL">1h</span>
                <NumCell v-bind="official(m.official_pricing.cache_write_1h_price)" />
              </div>
              <div :class="TIER_ROW">
                <span :class="TIER_LABEL">{{ t('modelPlaza.table.cacheRead') }}</span>
                <NumCell v-bind="official(m.official_pricing.cache_read_price)" />
              </div>
            </div>
            <NumCell v-else :value="null" />
          </td>

          <!-- Effective rate. A personal rate strikes the group default rather -->
          <!-- than recoloring it: the accent means interaction, never status. -->
          <td class="is-numeric border-l border-l-line text-xs">
            <NumCell
              v-if="usesIndependentImageRate(m)"
              :value="requestRate(m)"
              unit="x"
              data-testid="plaza-rate-effective"
            />
            <template v-else-if="hasCustomRate">
              <span
                class="mr-1.5 font-mono text-2xs text-ink-tertiary line-through"
                data-testid="plaza-rate-original"
                >{{ rateMultiplier }}x</span
              >
              <NumCell :value="effectiveRate" unit="x" data-testid="plaza-rate-effective" />
            </template>
            <NumCell v-else :value="effectiveRate" unit="x" data-testid="plaza-rate-effective" />
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Badge from '@/components/common/Badge.vue'
import NumCell from '@/components/common/NumCell.vue'
import { platformLabel } from '@/utils/platformColors'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_IMAGE,
  type BillingMode
} from '@/constants/channel'
import type { PlazaModel } from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'

const props = defineProps<{
  models: PlazaModel[]
  /** 分组平台;用于判断复合分组中哪些行需要标注具体平台。 */
  platform?: string
  /** 分组默认倍率。 */
  rateMultiplier: number
  /** 用户专属倍率;与默认不同,实付价按此计算并划线展示原倍率。 */
  userRateMultiplier?: number | null
  /** 生图独立倍率:true 时图片计费模型的实付倍率取 imageRateMultiplier,不取分组/专属倍率。 */
  imageRateIndependent?: boolean
  imageRateMultiplier?: number | null
}>()

const { t } = useI18n()

/** A tier/qualifier label sits left, its number right, on one baseline. */
const TIER_ROW = 'flex items-baseline justify-end gap-2 whitespace-nowrap'
const TIER_LABEL = 'font-sans text-2xs font-normal text-ink-tertiary'

const PER_MILLION = 1_000_000

/**
 * 展示顺序:
 * 1. token 计费的排在前,按图/按次计费的沉到末尾——它们的官方 token 价与实付的按张/按次价不同量纲,混排无意义;
 * 2. 组内按官方输出价从高到低,无官方价的排最后;
 * 3. 同价按名称降序(新版本号在前,如 gpt-5.6 先于 gpt-5.5)。
 */
const sortedModels = computed(() => {
  return [...props.models].sort((a, b) => {
    const ta = billingMode(a) === BILLING_MODE_TOKEN
    const tb = billingMode(b) === BILLING_MODE_TOKEN
    if (ta !== tb) return ta ? -1 : 1
    const pa = a.official_pricing?.output_price ?? null
    const pb = b.official_pricing?.output_price ?? null
    if (pa != null && pb != null && pa !== pb) return pb - pa
    if (pa != null && pb == null) return -1
    if (pa == null && pb != null) return 1
    return b.name.localeCompare(a.name)
  })
})

const effectiveRate = computed(() => props.userRateMultiplier ?? props.rateMultiplier)
const hasCustomRate = computed(
  () => props.userRateMultiplier != null && props.userRateMultiplier !== props.rateMultiplier
)

function billingMode(m: PlazaModel): BillingMode {
  return (m.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

function billingModeLabel(m: PlazaModel): string {
  return billingMode(m) === BILLING_MODE_IMAGE
    ? t('modelPlaza.table.perImage')
    : t('modelPlaza.table.perRequest')
}

/** 价格统一保底 2 位小数,更长的有效小数原样保留。 */
const MIN_DECIMALS = 2
/** 上限只是防御 IEEE 754 的指数尾巴,正常价格永远够不到。 */
const MAX_DECIMALS = 10

/**
 * What `NumCell` needs to render one price.
 *
 * `precision` is per value rather than a global 2, because this table spans
 * four orders of magnitude: $15.00 per 1M output tokens and $0.001 per image
 * are both prices here, and rounding the second to two places prints "$0.00".
 */
interface PriceCell {
  value: number | null
  precision: number
}

/**
 * Decimals actually carried by the value, floored at 2.
 *
 * `toPrecision(10)` first, for the same reason `formatScaled` does it: the
 * float behind "$3.00" is 3.0000000000000004, and its raw decimal expansion is
 * display noise, not data.
 */
function significantDecimals(n: number): number {
  const s = Math.abs(n).toPrecision(10)
  if (s.includes('e')) return MAX_DECIMALS
  const trimmed = s.replace(/\.?0+$/, '')
  const dot = trimmed.indexOf('.')
  const digits = dot === -1 ? 0 : trimmed.length - dot - 1
  return Math.min(MAX_DECIMALS, Math.max(MIN_DECIMALS, digits))
}

function priceCell(scaled: number | null): PriceCell {
  if (scaled == null || !Number.isFinite(scaled)) return { value: null, precision: MIN_DECIMALS }
  return { value: scaled, precision: significantDecimals(scaled) }
}

/** 实付价 = 渠道单价 × 生效倍率,按 $/1M token 展示。 */
function paidPerMillion(value: number | null | undefined): PriceCell {
  if (value == null) return priceCell(null)
  return priceCell(value * effectiveRate.value * PER_MILLION)
}

/** 图片计费模型且分组开启生图独立倍率:实付倍率取独立倍率,与计费口径一致。 */
function usesIndependentImageRate(m: PlazaModel): boolean {
  return billingMode(m) === BILLING_MODE_IMAGE && props.imageRateIndependent === true
}

/** 按次/按图片行的生效倍率。 */
function requestRate(m: PlazaModel): number {
  return usesIndependentImageRate(m) ? (props.imageRateMultiplier ?? 1) : effectiveRate.value
}

/** 按次 / 按图片单价(乘该行生效倍率,不换算 1M)。 */
function paidRequestPrice(m: PlazaModel, value: number | null | undefined): PriceCell {
  if (value == null) return priceCell(null)
  return priceCell(value * requestRate(m))
}

/** 官方参考价不乘倍率。 */
function official(value: number | null | undefined): PriceCell {
  if (value == null) return priceCell(null)
  return priceCell(value * PER_MILLION)
}

/** 非 token 计费的单位后缀:按图片 → “/ 张”,按次 → “/ 次”。 */
function perUnitSuffix(m: PlazaModel): string {
  return billingMode(m) === BILLING_MODE_IMAGE
    ? t('modelPlaza.table.perUnitImage')
    : t('modelPlaza.table.perUnitRequest')
}

function hasCachePricing(m: PlazaModel): boolean {
  return m.pricing?.cache_write_price != null || m.pricing?.cache_read_price != null
}

function hasOfficialCache(o: NonNullable<PlazaModel['official_pricing']>): boolean {
  return o.cache_write_price != null || o.cache_read_price != null || o.cache_write_1h_price != null
}

/** token 模式的阶梯定价(内联进输入/输出列)。 */
function tokenIntervals(m: PlazaModel): UserPricingInterval[] {
  return m.pricing?.intervals ?? []
}

/** 按次/按图模式的阶梯定价(仅保留配了按次价的档位)。 */
function requestIntervals(m: PlazaModel): UserPricingInterval[] {
  return (m.pricing?.intervals ?? []).filter((iv) => iv.per_request_price != null)
}

/** 档位标签:优先管理员配置的 tier_label,否则按 token 区间生成(≤200K / >200K / 200K–1M)。 */
function tierLabel(iv: UserPricingInterval): string {
  if (iv.tier_label) return iv.tier_label
  const { min_tokens: min, max_tokens: max } = iv
  if (max == null) return `>${formatTokenCount(min)}`
  if (min === 0) return `≤${formatTokenCount(max)}`
  return `${formatTokenCount(min)}–${formatTokenCount(max)}`
}

function formatTokenCount(n: number): string {
  if (n >= 1_000_000) return `${trimZero(n / 1_000_000)}M`
  if (n >= 1_000) return `${trimZero(n / 1_000)}K`
  return String(n)
}

function trimZero(n: number): string {
  return String(Math.round(n * 100) / 100)
}
</script>

<!--
  No `<style scoped>`.
  What used to be here: `--pz-title` / `--pz-bg` / `--pz-bg-hover` derived from
  the platform accent via `color-mix`, a `.dark` duplicate of all three, and a
  hover rule that changed the background of half the row to a second hue. Row
  hover is now a single neutral ground, from `.table` in style.css.
-->
