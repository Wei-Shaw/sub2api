<template>
  <div :class="flat ? '' : 'card overflow-hidden'">
    <div
      v-if="showIpGeoToolbar"
      class="flex items-center justify-end gap-2 border-b border-gray-200 px-4 py-2 dark:border-dark-700"
    >
      <span v-if="pendingIpCount > 0" class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('usage.ipGeo.pending', { count: pendingIpCount }) }}
      </span>
      <button
        type="button"
        class="inline-flex items-center gap-1 rounded px-2 py-1 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-400 dark:hover:bg-primary-900/30"
        :disabled="ipGeoBatchLoading || pendingIpCount === 0"
        @click="handleBatchFetchIpGeo"
      >
        {{ ipGeoBatchLoading ? t('usage.ipGeo.batchFetching') : t('usage.ipGeo.batchFetch') }}
      </button>
    </div>
    <div class="overflow-auto">
      <DataTable
        :columns="columns"
        :data="data"
        :loading="loading"
        :server-side-sort="serverSideSort"
        :default-sort-key="defaultSortKey"
        :default-sort-order="defaultSortOrder"
        @sort="(key, order) => $emit('sort', key, order)"
      >
        <template #cell-user="{ row }">
          <div class="text-sm">
            <button
              v-if="row.user?.email"
              class="font-medium text-primary-600 underline decoration-dashed underline-offset-2 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
              @click="$emit('userClick', row.user_id, row.user?.email)"
              :title="t('admin.usage.clickToViewBalance')"
            >
              {{ row.user.email }}
            </button>
            <span v-else class="font-medium text-gray-900 dark:text-white">-</span>
            <span v-if="row.user?.deleted_at" class="ml-1 inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-100 text-rose-600 ring-1 ring-inset ring-rose-200 dark:bg-rose-500/20 dark:text-rose-400 dark:ring-rose-500/30">
              {{ t('admin.usage.userDeletedBadge') }}
            </span>
            <span class="ml-1 text-gray-500 dark:text-gray-400">#{{ row.user_id }}</span>
          </div>
        </template>

        <template #cell-api_key="{ row }">
          <span class="text-sm text-gray-900 dark:text-white">{{ row.api_key?.name || '-' }}</span>
        </template>

        <template #cell-account="{ row }">
          <span class="text-sm text-gray-900 dark:text-white">{{ row.account?.name || '-' }}</span>
        </template>

        <template #cell-model="{ row }">
          <div class="space-y-0.5 text-xs">
            <div v-if="row.model_mapping_chain && row.model_mapping_chain.includes('→')" class="space-y-0.5">
              <div v-for="(step, i) in row.model_mapping_chain.split('→')" :key="i"
                   class="break-all"
                   :class="i === 0 ? 'font-medium text-gray-900 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                   :style="i > 0 ? `padding-left: ${i * 0.75}rem` : ''">
                <span v-if="i > 0" class="mr-0.5">↳</span>{{ step }}
              </div>
            </div>
            <div v-else-if="row.upstream_model && row.upstream_model !== row.model" class="space-y-0.5">
              <div class="break-all font-medium text-gray-900 dark:text-white">
                {{ row.model }}
              </div>
              <div class="break-all text-gray-500 dark:text-gray-400">
                <span class="mr-0.5">↳</span>{{ row.upstream_model }}
              </div>
            </div>
            <span v-else class="font-medium text-gray-900 dark:text-white">{{ row.model }}</span>
            <div
              v-if="row.upstream_model_mismatch === true && row.upstream_response_model"
              class="break-all pl-3 text-[11px]"
              :class="isLikelyModelVariant(row) ? 'text-amber-600 dark:text-amber-400' : 'text-orange-600 dark:text-orange-400'"
              :title="modelAuditTitle(row)"
            >
              <span class="mr-1">↳ {{ t('usage.upstreamResponseModel') }}:</span>{{ row.upstream_response_model }}
              <span
                class="ml-1 inline-flex rounded px-1 py-px text-[10px] font-medium ring-1 ring-inset"
                :class="isLikelyModelVariant(row)
                  ? 'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-500/10 dark:text-amber-300 dark:ring-amber-500/30'
                  : 'bg-orange-50 text-orange-700 ring-orange-200 dark:bg-orange-500/10 dark:text-orange-300 dark:ring-orange-500/30'"
              >
                {{ isLikelyModelVariant(row) ? t('usage.modelVariant') : t('usage.modelMismatch') }}
              </span>
            </div>
          </div>
        </template>

        <template #cell-reasoning_effort="{ row }">
          <div v-if="hasReasoningEffortMapping(row)" data-testid="reasoning-effort-cell" class="space-y-0.5 text-xs">
            <div class="font-medium text-gray-900 dark:text-white">
              {{ formatReasoningEffort(row.reasoning_effort) }}
            </div>
            <div class="text-gray-500 dark:text-gray-400">
              <span class="mr-0.5">↳</span>{{ formatReasoningEffort(row.upstream_reasoning_effort) }}
            </div>
          </div>
          <span v-else data-testid="reasoning-effort-cell" class="text-sm text-gray-900 dark:text-white">
            {{ formatReasoningEffort(row.reasoning_effort) }}
          </span>
        </template>

        <template #cell-endpoint="{ row }">
          <div class="max-w-[320px] space-y-1 text-xs">
            <div class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('usage.inbound') }}:</span>
              <span class="ml-1">{{ inboundEndpointLabel(row.inbound_endpoint) }}</span>
            </div>
            <div v-if="showUpstreamEndpoint" class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('usage.upstream') }}:</span>
              <span class="ml-1">{{ row.upstream_endpoint?.trim() || '-' }}</span>
            </div>
          </div>
        </template>

        <template #cell-group="{ row }">
          <span v-if="row.group" class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200">
            {{ row.group.name }}
          </span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-stream="{ row }">
          <div class="flex flex-wrap items-center gap-1">
            <span data-testid="request-type-badge" class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="getRequestTypeBadgeClass(row)">
              {{ getRequestTypeLabel(row) }}
            </span>
            <span
              v-if="row.native_compaction_v2"
              data-testid="native-compaction-badge"
              class="inline-flex items-center rounded bg-teal-100 px-2 py-0.5 text-xs font-medium text-teal-800 dark:bg-teal-900 dark:text-teal-200"
            >
              {{ t('usage.nativeCompactionV2') }}
            </span>
          </div>
        </template>

        <template #cell-billing_mode="{ row }">
          <span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="getBillingModeBadgeClass(getDisplayBillingMode(row))">
            {{ getBillingModeLabel(getDisplayBillingMode(row), t) }}
          </span>
        </template>

        <template #cell-tokens="{ row }">
          <!-- 图片生成请求（仅按次计费时显示图片格式） -->
          <div v-if="isImageUsage(row)" class="flex items-center gap-1.5">
            <svg class="h-4 w-4 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
            <span class="font-medium text-gray-900 dark:text-white">{{ row.image_count }}{{ t('usage.imageUnit') }}</span>
            <span class="text-gray-400">({{ formatImageBillingSize(row, t) }})</span>
          </div>
          <!-- Token 请求 -->
          <div v-else class="flex items-center gap-1.5">
            <div class="space-y-1 text-sm">
              <div class="flex items-center gap-2">
                <div class="inline-flex items-center gap-1">
                  <Icon name="arrowDown" size="sm" class="h-3.5 w-3.5 text-emerald-500" />
                  <span class="font-medium text-gray-900 dark:text-white">{{ row.input_tokens?.toLocaleString() || 0 }}</span>
                </div>
                <div class="inline-flex items-center gap-1">
                  <Icon name="arrowUp" size="sm" class="h-3.5 w-3.5 text-violet-500" />
                  <span class="font-medium text-gray-900 dark:text-white">{{ row.output_tokens?.toLocaleString() || 0 }}</span>
                </div>
              </div>
              <div v-if="row.cache_read_tokens > 0 || row.cache_creation_tokens > 0" class="flex items-center gap-2">
                <div v-if="row.cache_read_tokens > 0" class="inline-flex items-center gap-1">
                  <svg class="h-3.5 w-3.5 text-sky-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" /></svg>
                  <span class="font-medium text-sky-600 dark:text-sky-400">{{ formatCacheTokens(row.cache_read_tokens) }}</span>
                </div>
                <div v-if="row.cache_creation_tokens > 0" class="inline-flex items-center gap-1">
                  <svg class="h-3.5 w-3.5 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
                  <span class="font-medium text-amber-600 dark:text-amber-400">{{ formatCacheTokens(row.cache_creation_tokens) }}</span>
                  <span v-if="row.cache_creation_1h_tokens > 0" class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-orange-100 text-orange-600 ring-1 ring-inset ring-orange-200 dark:bg-orange-500/20 dark:text-orange-400 dark:ring-orange-500/30">1h</span>
                  <span v-if="row.cache_ttl_overridden" :title="t('usage.cacheTtlOverriddenHint')" class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-100 text-rose-600 ring-1 ring-inset ring-rose-200 dark:bg-rose-500/20 dark:text-rose-400 dark:ring-rose-500/30 cursor-help">R</span>
                </div>
              </div>
              <div v-if="hasImageInputTokens(row)" class="flex items-center gap-2">
                <div class="inline-flex items-center gap-1">
                  <svg class="h-3.5 w-3.5 text-fuchsia-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" /></svg>
                  <span class="font-medium text-fuchsia-600 dark:text-fuchsia-400">{{ row.image_input_tokens.toLocaleString() }}</span>
                </div>
              </div>
              <div v-if="hasImageOutputTokens(row)" class="flex items-center gap-2">
                <div class="inline-flex items-center gap-1">
                  <svg class="h-3.5 w-3.5 text-pink-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" /></svg>
                  <span class="font-medium text-pink-600 dark:text-pink-400">{{ row.image_output_tokens.toLocaleString() }}</span>
                </div>
              </div>
            </div>
            <!-- Token Detail Tooltip -->
            <div
              class="group relative"
              @mouseenter="showTokenTooltip($event, row)"
              @mouseleave="hideTokenTooltip"
            >
              <div class="flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-100 transition-colors group-hover:bg-blue-100 dark:bg-gray-700 dark:group-hover:bg-blue-900/50">
                <Icon name="infoCircle" size="xs" class="text-gray-400 group-hover:text-blue-500 dark:text-gray-500 dark:group-hover:text-blue-400" />
              </div>
            </div>
          </div>
        </template>

        <template #cell-cost="{ row }">
          <div class="text-sm">
            <div class="flex items-center gap-1.5">
              <span class="font-medium text-green-600 dark:text-green-400">${{ row.actual_cost?.toFixed(6) || '0.000000' }}</span>
              <span
                v-if="row.long_context_billing_applied"
                data-testid="long-context-billing-marker"
                class="inline-flex items-center rounded px-1 py-px text-[10px] font-semibold leading-tight bg-amber-100 text-amber-700 ring-1 ring-inset ring-amber-200 dark:bg-amber-500/20 dark:text-amber-300 dark:ring-amber-500/30"
              >x2</span>
              <!-- Cost Detail Tooltip -->
              <div
                class="group relative"
                @mouseenter="showTooltip($event, row)"
                @mouseleave="hideTooltip"
              >
                <div class="flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-100 transition-colors group-hover:bg-blue-100 dark:bg-gray-700 dark:group-hover:bg-blue-900/50">
                  <Icon name="infoCircle" size="xs" class="text-gray-400 group-hover:text-blue-500 dark:text-gray-500 dark:group-hover:text-blue-400" />
                </div>
              </div>
            </div>
            <div v-if="showAccountBilling && row.account_rate_multiplier != null" class="mt-0.5 text-[11px] text-orange-500 dark:text-orange-400">
              A ${{ accountBilled(row).toFixed(6) }}
            </div>
          </div>
        </template>

        <!-- 上游真实成本：与左侧「费用」是两个口径——费用是本站向用户收的（收入），
             这里是上游向我们收的（成本）。两者同为美元原值，**不做任何汇率换算**，
             所以可以直接相减得毛利（见右侧「盈亏」列）。
             取不到时显示「—」而不是 0：「还没查到」和「上游真的没扣钱」必须区分开，
             否则会把未取数误读成零成本，进而把毛利算高。 -->
        <template #cell-upstream_cost="{ row }">
          <div v-if="row.upstream_cost != null" class="text-sm">
            <span class="font-medium text-blue-600 dark:text-blue-400">
              {{ currencySymbol(row.upstream_cost_currency) }}{{ row.upstream_cost.toFixed(6) }}
            </span>
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500" title="—">—</span>
        </template>

        <!-- 盈亏：收入 − 成本。正为盈利（绿 +），负为亏损（红 −）。
             成本未知时显示「—」而不是把成本当 0 算出一个虚高的盈利——
             这正是最容易误导经营判断的地方。 -->
        <template #cell-profit="{ row }">
          <div v-if="rowProfit(row) != null" class="text-sm font-medium tabular-nums" :class="profitClass(rowProfit(row)!)">
            {{ formatSignedUSD(rowProfit(row)!) }}
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500" title="—">—</span>
        </template>

        <!-- 利润率：盈亏 ÷ 收入。收入为 0 时无意义，同样显示「—」而不是 0% 或无穷。 -->
        <template #cell-profit_margin="{ row }">
          <div v-if="rowProfitMargin(row) != null" class="text-sm font-medium tabular-nums" :class="profitClass(rowProfit(row)!)">
            {{ formatSignedPercent(rowProfitMargin(row)!) }}
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500" title="—">—</span>
        </template>

        <!-- 合并首字/总耗时的健康度列：左侧色条上端随首字档、下端随总耗时档，中段(40%-60%)短渐变过渡，便于纵向扫视整体健康状况 -->
        <template #cell-latency="{ row }">
          <div class="flex items-stretch gap-2">
            <span
              class="w-1 shrink-0 rounded-full"
              :class="row.first_token_ms != null
                ? ['bg-gradient-to-b from-40% to-60%', LATENCY_BAR_FROM_CLASSES[firstTokenSeverity(row.first_token_ms)], LATENCY_BAR_TO_CLASSES[durationSeverity(row.duration_ms ?? 0)]]
                : LATENCY_BAR_CLASSES[durationSeverity(row.duration_ms ?? 0)]"
              aria-hidden="true"
            ></span>
            <div class="grid grid-cols-[max-content_max-content] items-baseline gap-x-2 gap-y-0.5 text-xs">
              <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyFirstToken') }}</span>
              <span v-if="row.first_token_ms != null" class="font-medium tabular-nums" :class="LATENCY_TEXT_CLASSES[firstTokenSeverity(row.first_token_ms)]">{{ formatDuration(row.first_token_ms) }}</span>
              <span v-else class="text-gray-400 dark:text-gray-500">-</span>
              <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyDuration') }}</span>
              <span class="font-medium tabular-nums" :class="LATENCY_TEXT_CLASSES[durationSeverity(row.duration_ms ?? 0)]">{{ formatDuration(row.duration_ms) }}</span>
            </div>
          </div>
        </template>

        <!-- 上游延迟：与左侧「延迟」并排对照。左列是本站观测（含中转开销 + 网络往返
             + 上游全部处理时间），右列是上游账单自报，只覆盖上游内部那一段；两者相减
             就是「中转开销」，这是判断「是不是我们的中转把请求拖慢了」的唯一依据。

             刻意不做健康度着色：右侧是上游口径，用左侧那套阈值上色会暗示两列可直接
             横向比大小，而它们的含义本就不同。

             空值一律显示「—」而不是 0：上游账单要等落库后才会被反查回来（实测
             P50≈13s、P99≈599s），刚发出的请求这列本来就是空的；若显示 0 会被读成
             「上游瞬间返回」，与「还不知道」的排障结论完全相反。 -->
        <template #cell-upstream_latency="{ row }">
          <div v-if="row.upstream_first_token_ms != null || row.upstream_duration_ms != null" class="flex items-stretch gap-2">
            <span
              class="w-1 shrink-0 rounded-full"
              :class="row.upstream_first_token_ms != null
                ? ['bg-gradient-to-b from-40% to-60%', LATENCY_BAR_FROM_CLASSES[firstTokenSeverity(row.upstream_first_token_ms)], LATENCY_BAR_TO_CLASSES[durationSeverity(row.upstream_duration_ms ?? 0)]]
                : LATENCY_BAR_CLASSES[durationSeverity(row.upstream_duration_ms ?? 0)]"
              aria-hidden="true"
            ></span>
            <div class="grid grid-cols-[max-content_max-content] items-baseline gap-x-2 gap-y-0.5 text-xs">
              <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyFirstToken') }}</span>
              <span v-if="row.upstream_first_token_ms != null" class="font-medium tabular-nums" :class="LATENCY_TEXT_CLASSES[firstTokenSeverity(row.upstream_first_token_ms)]">{{ formatDuration(row.upstream_first_token_ms) }}</span>
              <span v-else class="text-gray-400 dark:text-gray-500">-</span>
              <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyDuration') }}</span>
              <span v-if="row.upstream_duration_ms != null" class="font-medium tabular-nums" :class="LATENCY_TEXT_CLASSES[durationSeverity(row.upstream_duration_ms ?? 0)]">{{ formatDuration(row.upstream_duration_ms) }}</span>
              <span v-else class="text-gray-400 dark:text-gray-500">-</span>
            </div>
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500" title="—">—</span>
        </template>

        <!-- 中转开销：本站延迟 − 上游延迟，单位毫秒。
             上格是首字差值、下格是总耗时差值，与左侧两列的上下顺序一一对应，
             因此不再重复写「首字 / 总耗时」文字——那既占列宽又无信息量。

             用差值而不是百分比：上游值一旦很小，比值会爆炸成几百个百分点，
             反而看不出绝对量；排障要回答的是「中转吃掉了多少毫秒」。

             任一侧没取到就显示「—」：0 的含义是「完全没有中转开销」，
             与「还不知道」正好相反，不能互相顶替。 -->
        <template #cell-upstream_overhead="{ row }">
          <div v-if="overheadFirst(row) != null || overheadDuration(row) != null" class="grid justify-items-end gap-y-0.5 text-xs">
            <span
              v-if="overheadFirst(row) != null"
              class="font-medium tabular-nums"
              :class="LATENCY_TEXT_CLASSES[firstTokenOverheadSeverity(overheadFirst(row)!)]"
            >
              {{ formatOverheadMs(overheadFirst(row)!) }}
            </span>
            <span v-else class="text-gray-400 dark:text-gray-500">-</span>
            <span
              v-if="overheadDuration(row) != null"
              class="font-medium tabular-nums"
              :class="LATENCY_TEXT_CLASSES[durationOverheadSeverity(overheadDuration(row)!)]"
            >
              {{ formatOverheadMs(overheadDuration(row)!) }}
            </span>
            <span v-else class="text-gray-400 dark:text-gray-500">-</span>
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500" title="—">—</span>
        </template>

        <!-- 上游商家归属：这一笔最终由「哪家商户的哪个渠道」服务。
             四个值同属一件事，堆叠展示（与左侧「端点」列同形态）；拆成 4 列会把表格撑得过宽。

             空值一律「—」：这些值来自上游账单，是**延迟对账**回来的
             （实测 P50≈13s、P99≈599s），刚发出的请求本来就是空的。
             折成 0 会被读成「第 0 号商户」，与「还不知道是哪家」正好相反 ——
             而这一列的全部意义就是定位责任方，误导的代价比留空大得多。 -->
        <template #cell-upstream_supplier="{ row }">
          <div
            v-if="row.upstream_supplier_id != null || row.upstream_supplier_name || row.upstream_channel_id != null || row.upstream_target_channel_id != null"
            class="max-w-[220px] space-y-0.5 text-xs"
          >
            <div class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.supplierId') }}:</span>
              <span class="ml-1 tabular-nums">{{ row.upstream_supplier_id ?? '-' }}</span>
            </div>
            <!-- 商家名只留数字：上游回的是「Supplier ID 514」这种带前缀的名称，
                 主人 2026-11-06 要求只显示后面的编号，前面那串文字不要。 -->
            <div class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.supplierName') }}:</span>
              <span class="ml-1 tabular-nums">{{ supplierLabel(row.upstream_supplier_name, row.upstream_supplier_id) }}</span>
            </div>
            <div class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.upstreamChannel') }}:</span>
              <span class="ml-1 tabular-nums">{{ row.upstream_channel_id ?? '-' }}</span>
            </div>
            <div class="break-all text-gray-700 dark:text-gray-300">
              <span class="font-medium text-gray-500 dark:text-gray-400">{{ t('admin.usage.upstreamTargetChannel') }}:</span>
              <span class="ml-1 tabular-nums">{{ row.upstream_target_channel_id ?? '-' }}</span>
            </div>
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500" title="—">—</span>
        </template>

        <!-- 操作列：把这一行的上游商户/渠道直接在上游处置掉。
             两个动作的**作用域完全不同**（整商户 vs 单渠道），所以分成两个标签按钮、
             且确认框里把范围与影响面写清楚 —— 拉黑商户不可轻易撤销，
             而且该商户名下的固定绑定不会自动改绑。

             样式刻意复用左侧「分组」列的小标签（同 rounded/px-2/py-0.5/text-xs/font-medium），
             让整行在视觉上维持一致；只用颜色区分动作的破坏性：
             拉黑商户是红灯级动作（红），拉黑渠道范围小一档（琥珀）。

             没有上游商户信息（尚未对账到）时禁用：那种情况下这一行还不知道该拉黑谁。 -->
        <template #cell-upstream_action="{ row }">
          <div class="flex flex-col items-start gap-1">
            <button
              type="button"
              data-testid="block-supplier"
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40 bg-red-100 text-red-800 hover:bg-red-200 dark:bg-red-900 dark:text-red-200 dark:hover:bg-red-800"
              :disabled="row.upstream_supplier_id == null || blocking || isSupplierBlocked(row)"
              :title="row.upstream_supplier_id == null ? t('admin.usage.blockNoSupplier') : t('admin.usage.blockSupplierHint')"
              @click="blockRow(row, 'supplier')"
            >
              {{ isSupplierBlocked(row) ? t('admin.usage.alreadyBlocked') : t('admin.usage.blockSupplier') }}
            </button>
            <button
              type="button"
              data-testid="block-channel"
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40 bg-amber-100 text-amber-800 hover:bg-amber-200 dark:bg-amber-900 dark:text-amber-200 dark:hover:bg-amber-800"
              :disabled="row.upstream_channel_id == null || blocking || isChannelBlocked(row)"
              @click="blockRow(row, 'channel_model')"
            >
              {{ isChannelBlocked(row) ? t('admin.usage.alreadyBlocked') : t('admin.usage.blockChannel') }}
            </button>
          </div>
        </template>

        <template #cell-created_at="{ value }">
          <span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDateTime(value) }}</span>
        </template>

        <template #cell-request_id="{ row }">
          <div v-if="row.request_id" class="flex max-w-[160px] items-center gap-1.5">
            <span class="truncate font-mono text-xs text-gray-500 dark:text-gray-400" :title="row.request_id">
              {{ row.request_id }}
            </span>
            <button
              type="button"
              class="shrink-0 rounded p-0.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-300"
              :class="copiedRequestId === row.request_id ? 'text-green-500 hover:text-green-500' : ''"
              :title="copiedRequestId === row.request_id ? t('keys.copied') : t('keys.copyToClipboard')"
              @click="copyRequestId(row.request_id)"
            >
              <Icon :name="copiedRequestId === row.request_id ? 'check' : 'copy'" size="sm" class="h-3.5 w-3.5" />
            </button>
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-upstream_request_id="{ row }">
          <div v-if="row.upstream_request_id" class="flex max-w-[160px] items-center gap-1.5">
            <span class="truncate font-mono text-xs text-gray-500 dark:text-gray-400" :title="row.upstream_request_id">
              {{ row.upstream_request_id }}
            </span>
            <button
              type="button"
              class="shrink-0 rounded p-0.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-300"
              :class="copiedRequestId === row.upstream_request_id ? 'text-green-500 hover:text-green-500' : ''"
              :title="copiedRequestId === row.upstream_request_id ? t('keys.copied') : t('keys.copyToClipboard')"
              @click="copyUpstreamRequestId(row.upstream_request_id)"
            >
              <Icon :name="copiedRequestId === row.upstream_request_id ? 'check' : 'copy'" size="sm" class="h-3.5 w-3.5" />
            </button>
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-user_agent="{ row }">
          <span v-if="row.user_agent" class="text-sm text-gray-600 dark:text-gray-400 block max-w-[320px] truncate" :title="row.user_agent">{{ formatUserAgent(row.user_agent) }}</span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #cell-ip_address="{ row }">
          <div v-if="row.ip_address">
            <span class="text-sm font-mono text-gray-600 dark:text-gray-400">{{ row.ip_address }}</span>
            <IpGeoCell :ip="row.ip_address" />
          </div>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
        </template>

        <template #empty><EmptyState :message="t('usage.noRecords')" /></template>
      </DataTable>
    </div>
  </div>

  <!-- Token Tooltip Portal -->
  <Teleport to="body">
    <div
      v-if="tokenTooltipVisible"
      class="fixed z-[9999] pointer-events-none -translate-y-1/2"
      :style="{
        left: tokenTooltipPosition.x + 'px',
        top: tokenTooltipPosition.y + 'px'
      }"
    >
      <div class="whitespace-nowrap rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl dark:border-gray-600 dark:bg-gray-800">
        <div class="space-y-1.5">
          <div>
            <div class="text-xs font-semibold text-gray-300 mb-1">{{ t('usage.tokenDetails') }}</div>
            <div v-if="tokenTooltipData && tokenTooltipData.input_tokens > 0 && !hasImageInputTokens(tokenTooltipData)" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputTokens') }}</span>
              <span class="font-medium text-white">{{ tokenTooltipData.input_tokens.toLocaleString() }}</span>
            </div>
            <div v-if="tokenTooltipData && hasImageInputTokens(tokenTooltipData) && textInputTokens(tokenTooltipData) > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputTokens') }}</span>
              <span class="font-medium text-white">{{ textInputTokens(tokenTooltipData).toLocaleString() }}</span>
            </div>
            <div v-if="tokenTooltipData && hasImageInputTokens(tokenTooltipData)" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('usage.imageInputTokens') }}</span>
              <span class="font-medium text-fuchsia-300">{{ tokenTooltipData.image_input_tokens.toLocaleString() }}</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.output_tokens > 0 && !hasImageOutputTokens(tokenTooltipData)" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputTokens') }}</span>
              <span class="font-medium text-white">{{ tokenTooltipData.output_tokens.toLocaleString() }}</span>
            </div>
            <div v-if="tokenTooltipData && hasImageOutputTokens(tokenTooltipData) && textOutputTokens(tokenTooltipData) > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputTokens') }}</span>
              <span class="font-medium text-white">{{ textOutputTokens(tokenTooltipData).toLocaleString() }}</span>
            </div>
            <div v-if="tokenTooltipData && hasImageOutputTokens(tokenTooltipData)" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('usage.imageOutputTokens') }}</span>
              <span class="font-medium text-pink-300">{{ tokenTooltipData.image_output_tokens.toLocaleString() }}</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_creation_tokens > 0">
              <!-- 有 5m/1h 明细时，展开显示 -->
              <template v-if="tokenTooltipData.cache_creation_5m_tokens > 0 || tokenTooltipData.cache_creation_1h_tokens > 0">
                <div v-if="tokenTooltipData.cache_creation_5m_tokens > 0" class="flex items-center justify-between gap-4">
                  <span class="text-gray-400 flex items-center gap-1.5">
                    {{ t('admin.usage.cacheCreation5mTokens') }}
                    <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-amber-500/20 text-amber-400 ring-1 ring-inset ring-amber-500/30">5m</span>
                  </span>
                  <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_5m_tokens.toLocaleString() }}</span>
                </div>
                <div v-if="tokenTooltipData.cache_creation_1h_tokens > 0" class="flex items-center justify-between gap-4">
                  <span class="text-gray-400 flex items-center gap-1.5">
                    {{ t('admin.usage.cacheCreation1hTokens') }}
                    <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-orange-500/20 text-orange-400 ring-1 ring-inset ring-orange-500/30">1h</span>
                  </span>
                  <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_1h_tokens.toLocaleString() }}</span>
                </div>
              </template>
              <!-- 无明细时，只显示聚合值 -->
              <div v-else class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('admin.usage.cacheCreationTokens') }}</span>
                <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_tokens.toLocaleString() }}</span>
              </div>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_ttl_overridden" class="flex items-center justify-between gap-4">
              <span class="text-gray-400 flex items-center gap-1.5">
                {{ t('usage.cacheTtlOverriddenLabel') }}
                <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-500/20 text-rose-400 ring-1 ring-inset ring-rose-500/30">R-{{ tokenTooltipData.cache_creation_1h_tokens > 0 ? '5m' : '1H' }}</span>
              </span>
              <span class="font-medium text-rose-400">{{ tokenTooltipData.cache_creation_1h_tokens > 0 ? t('usage.cacheTtlOverridden1h') : t('usage.cacheTtlOverridden5m') }}</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_read_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheReadTokens') }}</span>
              <span class="font-medium text-white">{{ tokenTooltipData.cache_read_tokens.toLocaleString() }}</span>
            </div>
          </div>
          <div class="flex items-center justify-between gap-6 border-t border-gray-700 pt-1.5">
            <span class="text-gray-400">{{ t('usage.totalTokens') }}</span>
            <span class="font-semibold text-blue-400">{{ ((tokenTooltipData?.input_tokens || 0) + (tokenTooltipData?.output_tokens || 0) + (tokenTooltipData?.cache_creation_tokens || 0) + (tokenTooltipData?.cache_read_tokens || 0)).toLocaleString() }}</span>
          </div>
        </div>
        <div class="absolute right-full top-1/2 h-0 w-0 -translate-y-1/2 border-b-[6px] border-r-[6px] border-t-[6px] border-b-transparent border-r-gray-900 border-t-transparent dark:border-r-gray-800"></div>
      </div>
    </div>
  </Teleport>

  <!-- Cost Tooltip Portal -->
  <Teleport to="body">
    <div
      v-if="tooltipVisible"
      class="fixed z-[9999] pointer-events-none -translate-y-1/2"
      :style="{
        left: tooltipPosition.x + 'px',
        top: tooltipPosition.y + 'px'
      }"
    >
      <div class="whitespace-nowrap rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl dark:border-gray-600 dark:bg-gray-800">
        <div class="space-y-1.5">
          <!-- Cost Breakdown -->
          <div class="mb-2 border-b border-gray-700 pb-1.5">
            <div class="text-xs font-semibold text-gray-300 mb-1">{{ t('usage.costDetails') }}</div>
            <div v-if="tooltipData && tooltipData.input_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputCost') }}</span>
              <span class="font-medium text-white">${{ tooltipData.input_cost.toFixed(8) }}</span>
            </div>
            <div v-if="tooltipData && hasImageInputCost(tooltipData)" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('usage.imageInputCost') }}</span>
              <span class="font-medium text-fuchsia-300">${{ tooltipData.image_input_cost.toFixed(8) }}</span>
            </div>
            <div v-if="tooltipData && tooltipData.output_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputCost') }}</span>
              <span class="font-medium text-white">${{ tooltipData.output_cost.toFixed(8) }}</span>
            </div>
            <div v-if="tooltipData && hasImageOutputCost(tooltipData)" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('usage.imageOutputCost') }}</span>
              <span class="font-medium text-pink-300">${{ tooltipData.image_output_cost.toFixed(8) }}</span>
            </div>
            <!-- Token billing: show unit prices per 1M tokens -->
            <template v-if="tooltipData && !isImageUsage(tooltipData) && (!tooltipData.billing_mode || tooltipData.billing_mode === BILLING_MODE_TOKEN)">
              <div v-if="tooltipData && textInputTokens(tooltipData) > 0" class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.inputTokenPrice') }}</span>
                <span class="font-medium text-sky-300">{{ formatTokenPricePerMillion(tooltipData.input_cost, textInputTokens(tooltipData)) }} {{ t('usage.perMillionTokens') }}</span>
              </div>
              <div v-if="tooltipData && hasImageInputTokens(tooltipData)" class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageInputTokenPrice') }}</span>
                <span class="font-medium text-fuchsia-300">{{ formatTokenPricePerMillion(tooltipData.image_input_cost ?? 0, tooltipData.image_input_tokens) }} {{ t('usage.perMillionTokens') }}</span>
              </div>
              <div v-if="tooltipData && tooltipData.output_cost > 0 && textOutputTokens(tooltipData) > 0" class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.outputTokenPrice') }}</span>
                <span class="font-medium text-violet-300">{{ formatTokenPricePerMillion(tooltipData.output_cost, textOutputTokens(tooltipData)) }} {{ t('usage.perMillionTokens') }}</span>
              </div>
              <div v-if="tooltipData && hasImageOutputTokens(tooltipData)" class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageOutputTokenPrice') }}</span>
                <span class="font-medium text-pink-300">{{ formatTokenPricePerMillion(tooltipData.image_output_cost ?? 0, tooltipData.image_output_tokens) }} {{ t('usage.perMillionTokens') }}</span>
              </div>
            </template>
            <template v-else-if="tooltipData && isImageUsage(tooltipData)">
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageCount') }}</span>
                <span class="font-medium text-white">{{ tooltipData.image_count }}{{ t('usage.imageUnit') }}</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageBillingSize') }}</span>
                <span class="font-medium text-white">{{ formatImageBillingSize(tooltipData, t) }}</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageSizeSource') }}</span>
                <span class="font-medium text-white">{{ formatImageSizeSource(tooltipData, t) }}</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageInputSize') }}</span>
                <span class="font-medium text-white">{{ formatImageInputSize(tooltipData, t) }}</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageOutputSize') }}</span>
                <span class="font-medium text-white">{{ formatImageOutputSize(tooltipData, t) }}</span>
              </div>
              <div v-if="formatImageSizeBreakdown(tooltipData)" class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageSizeBreakdown') }}</span>
                <span class="font-medium text-white">{{ formatImageSizeBreakdown(tooltipData) }}</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageUnitPrice') }}</span>
                <span class="font-medium text-sky-300">${{ imageUnitPrice(tooltipData).toFixed(8) }}</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('usage.imageTotalPrice') }}</span>
                <span class="font-medium text-white">${{ tooltipData.total_cost?.toFixed(8) || '0.00000000' }}</span>
              </div>
            </template>
            <div v-else class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('usage.unitPrice') }}</span>
              <span class="font-medium text-sky-300">${{ tooltipData?.total_cost?.toFixed(8) || '0.00000000' }}</span>
            </div>
            <div v-if="tooltipData && tooltipData.cache_creation_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheCreationCost') }}</span>
              <span class="font-medium text-white">${{ tooltipData.cache_creation_cost.toFixed(8) }}</span>
            </div>
            <div v-if="tooltipData && tooltipData.cache_read_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheReadCost') }}</span>
              <span class="font-medium text-white">${{ tooltipData.cache_read_cost.toFixed(8) }}</span>
            </div>
          </div>
          <!-- Rate and Summary -->
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.serviceTier') }}</span>
            <span class="font-semibold text-cyan-300">{{ getUsageServiceTierLabel(tooltipData?.service_tier, t) }}</span>
          </div>
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.rate') }}</span>
            <span class="font-semibold text-blue-400">{{ formatMultiplier(tooltipData?.rate_multiplier || 1) }}x</span>
          </div>
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.original') }}</span>
            <span class="font-medium text-white">${{ tooltipData?.total_cost?.toFixed(8) || '0.00000000' }}</span>
          </div>
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.userBilled') }}</span>
            <span class="font-semibold text-green-400">${{ tooltipData?.actual_cost?.toFixed(8) || '0.00000000' }}</span>
          </div>
          <!-- Account billing (separated from user billing) -->
          <template v-if="showAccountBilling">
            <div class="flex items-center justify-between gap-6 border-t border-gray-700 pt-1.5">
              <span class="text-gray-400">{{ t('usage.accountMultiplier') }}</span>
              <span class="font-semibold text-blue-400">{{ formatMultiplier(tooltipData?.account_rate_multiplier ?? 1) }}x</span>
            </div>
            <div class="flex items-center justify-between gap-6">
              <span class="text-gray-400">{{ t('usage.accountBilled') }}</span>
              <span class="font-semibold text-green-400">
                ${{ accountBilled({
                  total_cost: tooltipData?.total_cost,
                  account_stats_cost: tooltipData?.account_stats_cost,
                  account_rate_multiplier: tooltipData?.account_rate_multiplier,
                }).toFixed(8) }}
              </span>
            </div>
          </template>
        </div>
        <div class="absolute right-full top-1/2 h-0 w-0 -translate-y-1/2 border-b-[6px] border-r-[6px] border-t-[6px] border-b-transparent border-r-gray-900 border-t-transparent dark:border-r-gray-800"></div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { formatDateTime, formatReasoningEffort, reasoningEffortValuesEqual } from '@/utils/format'
import { formatCacheTokens, formatMultiplier } from '@/utils/formatters'
import { formatTokenPricePerMillion } from '@/utils/usagePricing'
import { getUsageServiceTierLabel } from '@/utils/usageServiceTier'
import { resolveUsageRequestType } from '@/utils/usageRequestType'
import {
  LATENCY_BAR_CLASSES,
  LATENCY_BAR_FROM_CLASSES,
  LATENCY_BAR_TO_CLASSES,
  LATENCY_TEXT_CLASSES,
  durationOverheadSeverity,
  durationSeverity,
  firstTokenOverheadSeverity,
  firstTokenSeverity,
  upstreamOverheadMs,
} from '@/utils/latencyHealth'
import {
  BILLING_MODE_TOKEN,
  getBillingModeLabel,
  getBillingModeBadgeClass,
  isImageUsage,
  getDisplayBillingMode,
  imageUnitPrice,
} from '@/utils/billingMode'
import {
  formatImageBillingSize,
  formatImageInputSize,
  formatImageOutputSize,
  formatImageSizeBreakdown,
  formatImageSizeSource,
  hasImageOutputTokens,
  textOutputTokens,
  hasImageOutputCost,
  hasImageInputTokens,
  textInputTokens,
  hasImageInputCost,
} from '@/utils/imageUsage'

/**
 * 上游商家名只取编号显示。
 *
 * 上游（A6）回的商家名形如「Supplier ID 514」「Current Supplier ID 514」，
 * 前面的文字对我们没有信息量，主人 2026-11-06 要求只显示后面的数字。
 * 名称里确实没有数字时退回原样（总比显示「-」丢掉信息强），全空才显示「-」。
 */
function supplierLabel(name?: string | null, supplierId?: number | null): string {
  const raw = (name ?? '').trim()
  if (raw) {
    const trailing = raw.match(/(\d+)\s*$/)
    if (trailing) return trailing[1]
    const anywhere = raw.match(/(\d+)/)
    if (anywhere) return anywhere[1]
    return raw
  }
  return supplierId != null ? String(supplierId) : '-'
}

/** Compute the account-billed cost for display: (account_stats_cost ?? total_cost) * rate_multiplier */
function accountBilled(row: { total_cost?: number | null; account_stats_cost?: number | null; account_rate_multiplier?: number | null }): number {
  const base = row.account_stats_cost != null ? row.account_stats_cost : (row.total_cost ?? 0)
  const result = base * (row.account_rate_multiplier ?? 1)
  return Number.isNaN(result) ? 0 : result
}

/** 成本币种 → 货币符号。**未知币种直接显示代码**，不要猜成 $ —— 猜错等于报了个假数字。 */
const CURRENCY_SYMBOLS: Record<string, string> = { USD: '$', CNY: '¥', EUR: '€' }

function currencySymbol(currency?: string | null): string {
  if (!currency) return '$'
  return CURRENCY_SYMBOLS[currency.toUpperCase()] ?? `${currency.toUpperCase()} `
}

/**
 * 单行毛利 = 收入 − 成本。成本未知（还没反查到）时返回 null。
 *
 * 关键：**绝不能把未知成本当 0**。那会算出一个虚高的盈利，比不显示更危险——
 * 页面上一列绿字「+$0.0250」看不出它是真赚了还是只是还没查到成本。
 */
function rowProfit(row: Pick<AdminUsageLog, 'actual_cost' | 'upstream_cost'>): number | null {
  if (row.upstream_cost == null) return null
  return (row.actual_cost ?? 0) - row.upstream_cost
}

/** 利润率 = 毛利 ÷ 收入 × 100。收入为 0 时无意义，返回 null 而不是 0% 或 ±∞。 */
function rowProfitMargin(row: Pick<AdminUsageLog, 'actual_cost' | 'upstream_cost'>): number | null {
  const profit = rowProfit(row)
  if (profit == null) return null
  const revenue = row.actual_cost ?? 0
  if (revenue === 0) return null
  return (profit / revenue) * 100
}

/** 盈利绿、亏损红、持平中性；未知成本走不到这里（上游已判 null）。 */
function profitClass(profit: number): string {
  if (profit > 0) return 'text-green-600 dark:text-green-400'
  if (profit < 0) return 'text-red-600 dark:text-red-400'
  return 'text-gray-600 dark:text-gray-400'
}

/** 显式带符号，避免「一片绿字分不清正负」：+$0.000015 / -$0.000001。 */
function formatSignedUSD(value: number): string {
  return `${value < 0 ? '-' : '+'}$${Math.abs(value).toFixed(6)}`
}

function formatSignedPercent(value: number): string {
  return `${value < 0 ? '-' : '+'}${Math.abs(value).toFixed(2)}%`
}


import DataTable from '@/components/common/DataTable.vue'
import { blockUpstream, fetchUpstreamBlocks } from '@/api/admin/usage'
import EmptyState from '@/components/common/EmptyState.vue'
import IpGeoCell from '@/components/common/IpGeoCell.vue'
import Icon from '@/components/icons/Icon.vue'
import { fetchBatch, getEntry } from '@/utils/ipGeoLookup'
import type { AdminUsageLog } from '@/types'
import type { Column } from '@/components/common/types'

interface Props {
  data: AdminUsageLog[]
  loading?: boolean
  columns: Column[]
  serverSideSort?: boolean
  defaultSortKey?: string
  defaultSortOrder?: 'asc' | 'desc'
  showAccountBilling?: boolean
  showUpstreamEndpoint?: boolean
  /** 嵌入统一卡片内使用：去掉自身卡片外观 */
  flat?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  serverSideSort: false,
  defaultSortKey: '',
  defaultSortOrder: 'asc',
  showAccountBilling: true,
  showUpstreamEndpoint: true,
  flat: false
})
const emit = defineEmits<{
  userClick: [userID: number, email?: string]
  sort: [key: string, order: 'asc' | 'desc']
  ipGeoBatchFailed: []
}>()
const { t } = useI18n()
const appStore = useAppStore()
const copiedRequestId = ref<string | null>(null)
const showAccountBilling = props.showAccountBilling
const showUpstreamEndpoint = props.showUpstreamEndpoint
const ipGeoBatchLoading = ref(false)

const showIpGeoToolbar = computed(() => props.columns.some((col) => col.key === 'ip_address'))

const hasReasoningEffortMapping = (row: AdminUsageLog): boolean => {
  const requested = row.reasoning_effort?.trim() || ''
  const forwarded = row.upstream_reasoning_effort?.trim() || ''
  return requested !== '' && forwarded !== '' && !reasoningEffortValuesEqual(requested, forwarded)
}

const sentUpstreamModel = (row: AdminUsageLog): string => row.upstream_model?.trim() || row.model?.trim() || ''

const normalizeModelVariant = (model: string): string => model
  .trim()
  .toLowerCase()
  .replace(/-latest$/, '')
  .replace(/-\d{4}-\d{2}-\d{2}$/, '')
  .replace(/-\d{8}$/, '')

const isLikelyModelVariant = (row: AdminUsageLog): boolean => {
  const sent = sentUpstreamModel(row)
  const response = row.upstream_response_model?.trim() || ''
  return sent !== '' && response !== '' && normalizeModelVariant(sent) === normalizeModelVariant(response)
}

const modelAuditTitle = (row: AdminUsageLog): string => [
  `${t('usage.requestedModel')}: ${row.model || '-'}`,
  `${t('usage.sentUpstreamModel')}: ${sentUpstreamModel(row) || '-'}`,
  `${t('usage.upstreamResponseModel')}: ${row.upstream_response_model || '-'}`,
].join('\n')

const currentPageIps = computed(() =>
  Array.from(new Set(props.data.map((row) => row.ip_address).filter((ip): ip is string => Boolean(ip))))
)

const pendingIpCount = computed(() => {
  if (!showIpGeoToolbar.value) return 0
  return currentPageIps.value.filter((ip) => {
    const status = getEntry(ip).status
    return status === 'idle' || status === 'error'
  }).length
})

const handleBatchFetchIpGeo = async () => {
  ipGeoBatchLoading.value = true
  try {
    const ok = await fetchBatch(currentPageIps.value)
    if (!ok) emit('ipGeoBatchFailed')
  } finally {
    ipGeoBatchLoading.value = false
  }
}

const copyIdentifier = async (value: string, copiedMessage: string) => {
  try {
    await navigator.clipboard.writeText(value)
    copiedRequestId.value = value
    appStore.showSuccess(copiedMessage)
    window.setTimeout(() => {
      if (copiedRequestId.value === value) copiedRequestId.value = null
    }, 2000)
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}

const copyRequestId = (requestId: string) => copyIdentifier(requestId, t('admin.usage.requestIdCopied'))
const copyUpstreamRequestId = (upstreamRequestId: string) =>
  copyIdentifier(upstreamRequestId, t('admin.usage.upstreamRequestIdCopied'))

// Tooltip state - cost
const tooltipVisible = ref(false)
const tooltipPosition = ref({ x: 0, y: 0 })
const tooltipData = ref<AdminUsageLog | null>(null)

// Tooltip state - token
const tokenTooltipVisible = ref(false)
const tokenTooltipPosition = ref({ x: 0, y: 0 })
const tokenTooltipData = ref<AdminUsageLog | null>(null)

const getRequestTypeLabel = (row: AdminUsageLog): string => {
  const requestType = resolveUsageRequestType(row)
  if (requestType === 'cyber') return t('usage.cyber')
  if (requestType === 'live') return t('usage.live')
  if (requestType === 'ws_v2') return t('usage.ws')
  if (requestType === 'stream') return t('usage.stream')
  if (requestType === 'sync') return t('usage.sync')
  return t('usage.unknown')
}

const getRequestTypeBadgeClass = (row: AdminUsageLog): string => {
  const requestType = resolveUsageRequestType(row)
  if (requestType === 'cyber') return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
  if (requestType === 'live') return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200'
  if (requestType === 'ws_v2') return 'bg-violet-100 text-violet-800 dark:bg-violet-900 dark:text-violet-200'
  if (requestType === 'stream') return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
  if (requestType === 'sync') return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
  return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'
}



const formatUserAgent = (ua: string): string => {
  return ua
}

// 超过 1 分钟简化为 "Xm Ys"，免去人工换算（超过 1 小时再进位为 "Xh Ym"）
const formatDuration = (ms: number | null | undefined): string => {
  if (ms == null) return '-'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`
  const totalSec = Math.round(ms / 1000)
  if (totalSec < 3600) return `${Math.floor(totalSec / 60)}m ${totalSec % 60}s`
  return `${Math.floor(totalSec / 3600)}h ${Math.floor((totalSec % 3600) / 60)}m`
}

// 中转开销：本站延迟减去上游延迟（毫秒）。
//
// 拆成两个具名函数而不是在模板里现算，是为了让 v-if 与取值用同一个入口，
// 避免两处判断条件写歪（例如 v-if 用首字、显示却用总耗时）。
const overheadFirst = (
  row: Pick<AdminUsageLog, 'first_token_ms' | 'upstream_first_token_ms'>,
): number | null => upstreamOverheadMs(row.first_token_ms, row.upstream_first_token_ms)

const overheadDuration = (
  row: Pick<AdminUsageLog, 'duration_ms' | 'upstream_duration_ms'>,
): number | null => upstreamOverheadMs(row.duration_ms, row.upstream_duration_ms)

// 毫秒差值：正数显式带「+」，单位 `ms`（小写）跟在每个数值后面，如 `+724ms`。
// 单位不放表头：上下两格位数常不同，单位跟着数字走才不会"离得太远"。
const formatOverheadMs = (ms: number): string => `${ms > 0 ? '+' : ''}${ms}ms`

// 上游拉黑：把这一行对应的「整个商户」或「该渠道该模型」在上游 A6 侧处置掉。
//
// 用 window.confirm 而不是自建弹窗：这是低频、高危、需要"看清范围再点"的动作，
// 原生确认框足以表达，也省掉一个只为它存在的弹窗组件。
//
// 只用一个 blocking 标记而不是按行记：同一时刻只允许一个处置在飞，
// 避免管理员连点几下把同一商户重复拉黑（上游接口不做去重）。
const blocking = ref(false)

// 上游当前黑名单快照，用来把已拉黑的按钮置灰。
//
// 每次挂载现问上游、不在本地存：黑名单的权威在上游，本地存一份会在管理员
// 直接去上游后台拉黑/恢复时与上游漂移——而"以为已经拉黑了"比"显示未拉黑"更危险。
// 读取失败不报错（后端已回空集合）：置灰只是辅助，不该影响列表可用性。
const blockedSupplierIDs = ref<Set<number>>(new Set())
const blockedChannelModelKeys = ref<Set<string>>(new Set())

const loadUpstreamBlocks = async () => {
  try {
    const res = await fetchUpstreamBlocks()
    blockedSupplierIDs.value = new Set(res.supplier_ids || [])
    blockedChannelModelKeys.value = new Set(res.channel_model_keys || [])
  } catch {
    // 拉不到就都不置灰：按钮仍可点击，真拉黑了上游会拒绝并回显原因。
  }
}

onMounted(loadUpstreamBlocks)

// 该行的商户是否已被拉黑。
const isSupplierBlocked = (row: AdminUsageLog): boolean =>
  row.upstream_supplier_id != null && blockedSupplierIDs.value.has(row.upstream_supplier_id)

// 该行的「渠道+模型」是否已被拉黑。
//
// 商户被拉黑时其名下所有渠道也等同被拉黑，所以先看商户级——
// 否则会出现"商户已拉黑、但渠道按钮还亮着"的错觉。
const isChannelBlocked = (row: AdminUsageLog): boolean => {
  if (isSupplierBlocked(row)) return true
  if (row.upstream_channel_id == null) return false
  return blockedChannelModelKeys.value.has(`${row.upstream_channel_id}|${(row.model || '').trim()}`)
}

const blockRow = async (row: AdminUsageLog, scope: 'supplier' | 'channel_model') => {
  if (blocking.value) return

  let payload: Parameters<typeof blockUpstream>[0]
  if (scope === 'supplier') {
    const id = row.upstream_supplier_id
    // 尚未对账到的行没有商户 ID，不知道该拉黑谁 —— 按钮虽已禁用，这里再挡一次。
    if (id == null) {
      appStore.showError(t('admin.usage.blockNoSupplier'))
      return
    }
    const name = (row.upstream_supplier_name || '').trim() || `#${id}`
    if (!window.confirm(t('admin.usage.blockConfirmSupplier', { name, id }))) return
    payload = { scope: 'supplier', supplier_id: id }
  } else {
    const channelID = row.upstream_channel_id
    if (channelID == null) return
    const model = (row.model || '').trim()
    if (!model) return
    if (!window.confirm(t('admin.usage.blockConfirmChannelModel', { channel: channelID, model }))) return
    payload = { scope: 'channel_model', channel_id: channelID, model }
  }

  blocking.value = true
  try {
    await blockUpstream(payload)
    appStore.showSuccess(t('admin.usage.blockOk'))
    // 拉黑成功后立刻回读上游黑名单：同一页里其它行可能属于同一个商户/渠道，
    // 它们也该马上变成「已拉黑」，否则管理员会以为只影响了一行。
    await loadUpstreamBlocks()
  } catch (err: any) {
    // 把后端带回的上游原话原样透出：令牌失效/无权限的原因只在那里，
    // 换成笼统的「操作失败」会让管理员只能靠猜。
    appStore.showError(err?.message || String(err))
  } finally {
    blocking.value = false
  }
}

// 内部探针的入站标记是「伪路径」，是给后端过滤用的（usage_logs.inbound_endpoint），
// 直接摊在表里没人看得懂。这里只做**显示层**翻译，不改数据本身 ——
// 原始值仍可通过导出的 Excel 看到，也仍是筛选条件。
const INBOUND_ENDPOINT_LABELS: Record<string, string> = {
  'internal://channel-monitor': '渠道监控探针',
  'internal://intelligence-check': '智力检测探针',
}

const inboundEndpointLabel = (raw: string | null | undefined): string => {
  const value = (raw ?? '').trim()
  if (!value) return '-'
  return INBOUND_ENDPOINT_LABELS[value] ?? value
}

// Cost tooltip functions
const showTooltip = (event: MouseEvent, row: AdminUsageLog) => {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  tooltipData.value = row
  tooltipPosition.value.x = rect.right + 8
  tooltipPosition.value.y = rect.top + rect.height / 2
  tooltipVisible.value = true
}

const hideTooltip = () => {
  tooltipVisible.value = false
  tooltipData.value = null
}

// Token tooltip functions
const showTokenTooltip = (event: MouseEvent, row: AdminUsageLog) => {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  tokenTooltipData.value = row
  tokenTooltipPosition.value.x = rect.right + 8
  tokenTooltipPosition.value.y = rect.top + rect.height / 2
  tokenTooltipVisible.value = true
}

const hideTokenTooltip = () => {
  tokenTooltipVisible.value = false
  tokenTooltipData.value = null
}
</script>
