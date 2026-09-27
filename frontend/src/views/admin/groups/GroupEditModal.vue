<template>
  <!-- Edit Group Modal -->
  <BaseDialog
    :show="showEditModal"
    :title="t('admin.groups.editGroup')"
    width="wide"
    confirm-discard
    @close="closeEditModal"
  >
    <form
      v-if="editingGroup"
      id="edit-group-form"
      @submit.prevent="handleUpdateGroup"
      class="space-y-5"
    >
      <div>
        <label class="input-label">{{ t("admin.groups.form.name") }}</label>
        <input
          v-model="editForm.name"
          type="text"
          required
          class="input"
          data-tour="edit-group-form-name"
        />
      </div>
      <div>
        <label class="input-label">{{
          t("admin.groups.form.description")
        }}</label>
        <textarea
          v-model="editForm.description"
          rows="3"
          class="input"
        ></textarea>
      </div>
      <div>
        <label class="input-label">{{
          t("admin.groups.form.platform")
        }}</label>
        <Select
          v-model="editForm.platform"
          :options="platformOptions"
          :disabled="true"
          data-tour="group-form-platform"
        />
        <p class="input-hint">{{ t("admin.groups.platformNotEditable") }}</p>
      </div>
      <template v-if="!authStore.isSimpleMode">
      <!-- 从分组复制账号（编辑时） -->
      <div v-if="copyAccountsGroupOptionsForEdit.length > 0">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-sm font-medium text-fg">
            {{ t("admin.groups.copyAccounts.title") }}
          </label>
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-fg-subtle transition-colors hover:text-accent-strong"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-sm border border-border-strong bg-surface-raised p-3 text-fg shadow-overlay"
              >
                <p class="text-xs leading-relaxed text-fg-subtle">
                  {{ t("admin.groups.copyAccounts.tooltipEdit") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 border-b border-r border-border-strong bg-surface-raised"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <!-- 已选分组标签 -->
        <div
          v-if="editForm.copy_accounts_from_group_ids.length > 0"
          class="flex flex-wrap gap-1.5 mb-2"
        >
          <span
            v-for="groupId in editForm.copy_accounts_from_group_ids"
            :key="groupId"
            class="badge badge-primary gap-1"
          >
            {{
              copyAccountsGroupOptionsForEdit.find((o) => o.value === groupId)
                ?.label || `#${groupId}`
            }}
            <button
              type="button"
              @click="
                editForm.copy_accounts_from_group_ids =
                  editForm.copy_accounts_from_group_ids.filter(
                    (id) => id !== groupId,
                  )
              "
              class="ml-0.5 text-accent hover:text-accent-strong"
            >
              <Icon name="x" size="xs" />
            </button>
          </span>
        </div>
        <!-- 分组选择下拉 -->
        <select
          class="input"
          @change="
            (e) => {
              const val = Number((e.target as HTMLSelectElement).value);
              if (
                val &&
                !editForm.copy_accounts_from_group_ids.includes(val)
              ) {
                editForm.copy_accounts_from_group_ids.push(val);
              }
              (e.target as HTMLSelectElement).value = '';
            }
          "
        >
          <option value="">
            {{ t("admin.groups.copyAccounts.selectPlaceholder") }}
          </option>
          <option
            v-for="opt in copyAccountsGroupOptionsForEdit"
            :key="opt.value"
            :value="opt.value"
            :disabled="
              editForm.copy_accounts_from_group_ids.includes(opt.value)
            "
          >
            {{ opt.label }}
          </option>
        </select>
        <p class="input-hint">
          {{ t("admin.groups.copyAccounts.hintEdit") }}
        </p>
      </div>
      <div>
        <label class="input-label">{{
          t("admin.groups.form.rateMultiplier")
        }}</label>
        <input
          v-model.number="editForm.rate_multiplier"
          type="number"
          step="0.001"
          min="0.001"
          required
          class="input"
          data-tour="group-form-multiplier"
        />
      </div>
      <div>
        <label class="input-label">{{ t("admin.groups.form.rpmLimit") }}</label>
        <input
          v-model.number="editForm.rpm_limit"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.groups.form.rpmLimitPlaceholder')"
        />
        <p class="input-hint">{{ t("admin.groups.form.rpmLimitHint") }}</p>
      </div>
      <div>
        <label class="input-label">{{ t("admin.groups.form.concurrency") }}</label>
        <input
          v-model.number="editForm.concurrency"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.groups.form.concurrencyPlaceholder')"
        />
        <p class="input-hint">{{ t("admin.groups.form.concurrencyHint") }}</p>
      </div>
      <ReasoningEffortPolicyFields
        v-if="supportsReasoningEffortPolicyPlatform(editForm.platform)"
        ref="editReasoningEffortPolicyRef"
        id-prefix="edit-group-reasoning"
        :platform="editForm.platform"
        v-model:max-effort="editForm.max_reasoning_effort"
        v-model:over-limit="editForm.max_reasoning_effort_over_limit"
        v-model:mappings="editForm.reasoning_effort_mappings"
      />
      <div v-if="editForm.subscription_type !== 'subscription'">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-sm font-medium text-fg">
            {{ t("admin.groups.form.exclusive") }}
          </label>
          <!-- Help Tooltip -->
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-fg-subtle transition-colors hover:text-accent-strong"
            />
            <!-- Tooltip Popover -->
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-sm border border-border-strong bg-surface-raised p-3 text-fg shadow-overlay"
              >
                <p class="mb-2 text-xs font-medium">
                  {{ t("admin.groups.exclusiveTooltip.title") }}
                </p>
                <p class="mb-2 text-xs leading-relaxed text-fg-subtle">
                  {{ t("admin.groups.exclusiveTooltip.description") }}
                </p>
                <div class="border-t border-border pt-2">
                  <p class="text-xs leading-relaxed text-fg-subtle">
                    <span
                      class="inline-flex items-center gap-1 text-accent"
                      ><Icon name="lightbulb" size="xs" />
                      {{ t("admin.groups.exclusiveTooltip.example") }}</span
                    >
                    {{ t("admin.groups.exclusiveTooltip.exampleContent") }}
                  </p>
                </div>
                <!-- Arrow -->
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 border-b border-r border-border-strong bg-surface-raised"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <Toggle v-model="editForm.is_exclusive" />
          <span class="text-sm text-fg-muted">
            {{
              editForm.is_exclusive
                ? t("admin.groups.exclusive")
                : t("admin.groups.public")
            }}
          </span>
        </div>
      </div>
      <div>
        <label class="input-label">{{ t("admin.groups.form.status") }}</label>
        <Select v-model="editForm.status" :options="editStatusOptions" />
      </div>

      <!-- Subscription Configuration -->
      <div class="mt-4 border-t pt-4">
        <div>
          <label class="input-label">{{
            t("admin.groups.subscription.type")
          }}</label>
          <Select
            v-model="editForm.subscription_type"
            :options="subscriptionTypeOptions"
            :disabled="true"
          />
          <p class="input-hint">
            {{ t("admin.groups.subscription.typeNotEditable") }}
          </p>
        </div>

        <!-- Subscription limits (only show when subscription type is selected) -->
        <div
          v-if="editForm.subscription_type === 'subscription'"
          class="space-y-4 border border-accent/40 pl-4"
        >
          <div>
            <label class="input-label">{{
              t("admin.groups.subscription.dailyLimit")
            }}</label>
            <input
              v-model.number="editForm.daily_limit_usd"
              type="number"
              step="0.01"
              min="0"
              class="input"
              :placeholder="t('admin.groups.subscription.noLimit')"
            />
          </div>
          <div>
            <label class="input-label">{{
              t("admin.groups.subscription.weeklyLimit")
            }}</label>
            <input
              v-model.number="editForm.weekly_limit_usd"
              type="number"
              step="0.01"
              min="0"
              class="input"
              :placeholder="t('admin.groups.subscription.noLimit')"
            />
          </div>
          <div>
            <label class="input-label">{{
              t("admin.groups.subscription.monthlyLimit")
            }}</label>
            <input
              v-model.number="editForm.monthly_limit_usd"
              type="number"
              step="0.01"
              min="0"
              class="input"
              :placeholder="t('admin.groups.subscription.noLimit')"
            />
          </div>
        </div>
      </div>

      <div class="border-t pt-4">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <label class="text-h3 font-bold text-accent-strong">
              {{ t("admin.groups.modelAllowlist.title") }}
            </label>
            <p class="mt-1 text-xs text-fg-muted">
              {{ t("admin.groups.modelAllowlist.hint") }}
            </p>
          </div>
          <Toggle v-model="editModelAllowlistState.enabled" />
        </div>
        <div
          v-if="editModelAllowlistState.enabled"
          class="overflow-hidden rounded-sm border border-border bg-surface"
        >
          <div
            v-if="!editModelAllowlistLoading && editModelAllowlistState.items.length > 0"
            class="flex items-center justify-between gap-2 border-b-2 border-accent bg-accent-weak px-3 py-2 text-xs"
          >
            <span class="text-fg-muted">
              {{
                t("admin.groups.modelAllowlist.selectedSummary", {
                  selected: editModelAllowlistSelectedCount,
                  total: editModelAllowlistState.items.length,
                })
              }}
            </span>
            <div class="flex items-center gap-1.5">
              <button
                type="button"
                class="rounded-sm px-2 py-1 font-semibold text-accent-strong transition-colors hover:bg-surface"
                @click="selectAllModelAllowlistItems(editModelAllowlistState)"
              >
                {{ t("admin.groups.modelAllowlist.selectAll") }}
              </button>
              <button
                type="button"
                class="rounded-sm px-2 py-1 font-medium text-fg-muted transition-colors hover:bg-surface hover:text-accent-strong"
                @click="invertModelAllowlistSelection(editModelAllowlistState)"
              >
                {{ t("admin.groups.modelAllowlist.invertSelection") }}
              </button>
            </div>
          </div>
          <div
            class="max-h-64 divide-y divide-border overflow-y-auto"
          >
            <p v-if="editModelAllowlistLoading" class="px-3 py-2 text-xs text-fg-muted">
              {{ t("admin.groups.modelAllowlist.loading") }}
            </p>
            <p
              v-else-if="editModelAllowlistState.items.length === 0"
              class="px-3 py-2 text-xs text-fg-muted"
            >
              {{ t("admin.groups.modelAllowlist.empty") }}
            </p>
            <div
              v-for="(item, index) in editModelAllowlistState.items"
              :key="item.id"
              class="flex items-center gap-2 px-3 py-1.5 hover:bg-accent-weak/50"
            >
              <input
                v-model="item.selected"
                type="checkbox"
                class="h-4 w-4 rounded border-border-strong text-accent focus:ring-accent"
              />
              <span class="min-w-0 flex-1 break-all font-mono text-label text-fg">
                {{ item.id }}
                <span
                  v-if="item.id.endsWith('*')"
                  class="badge badge-primary ml-1 font-sans"
                >
                  {{ t("admin.groups.modelAllowlist.wildcardTag") }}
                </span>
              </span>
              <button
                type="button"
                :disabled="index === 0"
                class="rounded-sm p-1 text-fg-subtle hover:bg-accent-weak hover:text-accent-strong disabled:opacity-40"
                @click="moveEditModelAllowlistItem(index, index - 1)"
              >
                <Icon name="arrowUp" size="sm" />
              </button>
              <button
                type="button"
                :disabled="index === editModelAllowlistState.items.length - 1"
                class="rounded-sm p-1 text-fg-subtle hover:bg-accent-weak hover:text-accent-strong disabled:opacity-40"
                @click="moveEditModelAllowlistItem(index, index + 1)"
              >
                <Icon name="arrowDown" size="sm" />
              </button>
            </div>
          </div>
          <div class="border-t border-border px-3 py-2">
            <div class="flex items-center gap-2">
              <input
                v-model="editAllowlistCustomEntry"
                type="text"
                :placeholder="t('admin.groups.modelAllowlist.customPlaceholder')"
                class="input min-w-0 flex-1 py-1.5 font-mono"
                @keydown.enter.prevent="submitEditAllowlistCustomEntry"
              />
              <button
                type="button"
                class="btn btn-primary btn-sm"
                @click="submitEditAllowlistCustomEntry"
              >
                {{ t("admin.groups.modelAllowlist.addCustom") }}
              </button>
            </div>
            <p
              v-if="editAllowlistCustomErrorKey"
              class="mt-1 text-xs text-danger"
            >
              {{ t(editAllowlistCustomErrorKey) }}
            </p>
          </div>
        </div>
      </div>

      <!-- 图片生成计费配置 -->
      <div
        v-if="supportsImagePricingPlatform(editForm.platform)"
        class="border-t pt-4"
      >
        <label
          class="mb-2 block text-h3 font-bold text-accent-strong"
        >
          {{ t(imagePricingI18nKey(editForm.platform, "title")) }}
        </label>
        <p class="text-xs text-fg-muted mb-3">
          {{ t(imagePricingI18nKey(editForm.platform, "description")) }}
        </p>
        <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-2">
          <label class="flex items-center gap-2 text-sm text-fg">
            <input
              v-model="editForm.allow_image_generation"
              type="checkbox"
              class="rounded border-border-strong text-accent focus:ring-accent"
            />
            {{ t(imagePricingI18nKey(editForm.platform, "allowImageGeneration")) }}
          </label>
          <label class="flex items-center gap-2 text-sm text-fg">
            <input
              v-model="editForm.image_rate_independent"
              type="checkbox"
              class="rounded border-border-strong text-accent focus:ring-accent"
            />
            {{ t(imagePricingI18nKey(editForm.platform, "independentMultiplier")) }}
          </label>
        </div>
        <div
          v-if="editForm.image_rate_independent"
          class="mb-4"
        >
          <label class="input-label">{{
            t(imagePricingI18nKey(editForm.platform, "imageMultiplier"))
          }}</label>
          <input
            v-model.number="editForm.image_rate_multiplier"
            type="number"
            step="0.0001"
            min="0"
            class="input"
            placeholder="1"
          />
        </div>
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label class="input-label">1K ($)</label>
            <input
              v-model.number="editForm.image_price_1k"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getImagePricePlaceholder(editForm.platform, 'image_price_1k')"
            />
          </div>
          <div>
            <label class="input-label">2K ($)</label>
            <input
              v-model.number="editForm.image_price_2k"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getImagePricePlaceholder(editForm.platform, 'image_price_2k')"
            />
          </div>
          <div>
            <label class="input-label">4K ($)</label>
            <input
              v-model.number="editForm.image_price_4k"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getImagePricePlaceholder(editForm.platform, 'image_price_4k')"
            />
          </div>
        </div>
        <p class="mt-3 text-xs text-fg-muted">
          {{ t(imagePricingI18nKey(editForm.platform, "modeHint")) }}
        </p>
        <div class="mt-2">
          <p class="mb-1 text-meta font-medium text-fg-muted">
            {{ t(imagePricingI18nKey(editForm.platform, "finalPricePreview")) }}
          </p>
          <div class="table-container">
            <table class="table text-label">
              <thead>
                <tr>
                  <th v-for="item in editImageFinalPricePreview" :key="item.label" class="text-right">{{ item.label }}</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td v-for="item in editImageFinalPricePreview" :key="item.label" class="text-right tabular-nums">{{ item.value }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div v-if="editForm.platform === 'gemini' && editForm.allow_image_generation" class="mt-4 border-t border-border pt-4">
          <label
            class="flex items-center gap-2 text-sm font-medium text-fg"
          >
            <input
              v-model="editForm.allow_batch_image_generation"
              type="checkbox"
              class="rounded border-border-strong text-accent focus:ring-accent"
            />
            {{ t("admin.groups.imagePricing.allowBatchImageGeneration") }}
          </label>
          <p class="mt-2 text-xs text-fg-muted">
            {{ t("admin.groups.imagePricing.batchSectionHint") }}
          </p>
          <div
            v-if="editForm.allow_batch_image_generation"
            class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2"
          >
            <div>
              <label class="input-label">{{
                t("admin.groups.imagePricing.batchDiscountMultiplier")
              }}</label>
              <input
                v-model.number="editForm.batch_image_discount_multiplier"
                type="number"
                step="0.0001"
                min="0"
                class="input"
                placeholder="0.5"
              />
            </div>
            <div>
              <label class="input-label">{{
                t("admin.groups.imagePricing.batchHoldMultiplier")
              }}</label>
              <input
                v-model.number="editForm.batch_image_hold_multiplier"
                type="number"
                step="0.0001"
                min="0"
                class="input"
                placeholder="0.6"
              />
            </div>
          </div>
        </div>
        <p
          v-else-if="editForm.platform !== 'gemini'"
          class="mt-4 border-t border-border pt-4 text-xs text-fg-muted"
        >
          {{ t("admin.groups.imagePricing.batchGeminiOnlyHint") }}
        </p>
      </div>

      <!-- 视频生成计费配置（仅 Grok 平台） -->
      <div
        v-if="supportsVideoPricingPlatform(editForm.platform)"
        class="border-t pt-4"
      >
        <label
          class="mb-2 block text-h3 font-bold text-accent-strong"
        >
          {{ t(videoPricingI18nKey("title")) }}
        </label>
        <p class="text-xs text-fg-muted mb-3">
          {{ t(videoPricingI18nKey("description")) }}
        </p>
        <div class="mb-4">
          <label class="flex items-center gap-2 text-sm text-fg">
            <input
              v-model="editForm.video_rate_independent"
              type="checkbox"
              class="rounded border-border-strong text-accent focus:ring-accent"
            />
            {{ t(videoPricingI18nKey("independentMultiplier")) }}
          </label>
        </div>
        <div
          v-if="editForm.video_rate_independent"
          class="mb-4"
        >
          <label class="input-label">{{
            t(videoPricingI18nKey("videoMultiplier"))
          }}</label>
          <input
            v-model.number="editForm.video_rate_multiplier"
            type="number"
            step="0.0001"
            min="0"
            class="input"
            placeholder="1"
          />
        </div>
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label class="input-label">480p ($/s)</label>
            <input
              v-model.number="editForm.video_price_480p"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getVideoPricePlaceholder(editForm.platform, 'video_price_480p')"
            />
          </div>
          <div>
            <label class="input-label">720p ($/s)</label>
            <input
              v-model.number="editForm.video_price_720p"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getVideoPricePlaceholder(editForm.platform, 'video_price_720p')"
            />
          </div>
          <div>
            <label class="input-label">1080p ($/s)</label>
            <input
              v-model.number="editForm.video_price_1080p"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getVideoPricePlaceholder(editForm.platform, 'video_price_1080p')"
            />
          </div>
        </div>
        <div
          class="mt-4 border-t border-border pt-4"
          data-testid="edit-grok-video-model-prices"
        >
          <p class="text-label font-semibold text-fg">
            {{ t("admin.groups.videoPricing.modelOverridesTitle") }}
          </p>
          <p class="mt-1 text-xs text-fg-muted">
            {{ t("admin.groups.videoPricing.modelOverridesDescription") }}
          </p>
          <div class="mt-3 space-y-3">
            <div
              v-for="family in videoModelPriceFamilyRows(editForm.video_model_prices)"
              :key="family.key"
              class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_repeat(3,minmax(0,7rem))] sm:items-end"
            >
              <div class="min-w-0 pb-1 font-mono text-label text-fg">
                {{ family.label }}
              </div>
              <label
                v-for="resolution in grokVideoPriceResolutions"
                :key="resolution.key"
                class="block"
              >
                <span class="mb-1 block text-xs text-fg-muted">
                  {{ resolution.label }} ($/s)
                </span>
                <input
                  v-model.number="editForm.video_model_prices[family.key][resolution.key]"
                  type="number"
                  step="0.001"
                  min="0"
                  class="input"
                  :data-testid="`edit-grok-video-price-${family.key}-${resolution.key}`"
                />
              </label>
            </div>
          </div>
        </div>
        <p class="mt-3 text-xs text-fg-muted">
          {{ t(videoPricingI18nKey("modeHint")) }}
        </p>
        <div class="mt-2">
          <p class="mb-1 text-meta font-medium text-fg-muted">
            {{ t(videoPricingI18nKey("finalPricePreview")) }}
          </p>
          <div class="table-container">
            <table class="table text-label">
              <thead>
                <tr>
                  <th v-for="item in editVideoFinalPricePreview" :key="item.label" class="text-right">{{ item.label }}</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td v-for="item in editVideoFinalPricePreview" :key="item.label" class="text-right tabular-nums">{{ item.value }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- 高峰时段倍率配置（仅订阅类型分组） -->
      <div v-if="editForm.subscription_type === 'subscription'" class="border-t pt-4">
        <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-2">
          <label class="flex items-center gap-2 text-sm text-fg">
            <input
              v-model="editForm.peak_rate_enabled"
              type="checkbox"
              class="rounded border-border-strong text-accent focus:ring-accent"
            />
            <span>{{ t("admin.groups.peakRate.enable") }}</span>
          </label>
        </div>
        <div
          v-if="editForm.peak_rate_enabled"
          class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-3"
        >
          <div>
            <label class="input-label">{{ t("admin.groups.peakRate.peakStart") }}</label>
            <input
              v-model="editForm.peak_start"
              type="time"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.peakRate.peakEnd") }}</label>
            <input
              v-model="editForm.peak_end"
              type="time"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.peakRate.peakMultiplier") }}</label>
            <input
              v-model.number="editForm.peak_rate_multiplier"
              type="number"
              step="0.001"
              min="0"
              class="input"
              placeholder="1"
              :title="t('admin.groups.peakRate.multiplierHint')"
            />
          </div>
        </div>
      </div>

      <!-- 分组利润控制（五个平台 token 请求） -->
      <div v-if="isProfitControlPlatform(editForm.platform)" class="border-t pt-4">
        <label class="flex items-center gap-2 text-sm text-fg">
          <input
            v-model="editForm.profit_control_enabled"
            type="checkbox"
            class="rounded border-border-strong text-accent focus:ring-accent"
          />
          <span>{{ t("admin.groups.profitControl.enable") }}</span>
        </label>
        <p class="mb-3 mt-1.5 text-xs text-fg-muted">
          {{
            editForm.profit_control_enabled
              ? t("admin.groups.profitControl.enabledHint")
              : t("admin.groups.profitControl.disabledHint")
          }}
        </p>
        <div
          v-if="editForm.profit_control_enabled"
          class="mb-3 grid grid-cols-1 gap-3 sm:grid-cols-2"
        >
          <div>
            <label class="input-label">{{ t("admin.groups.profitControl.minMargin") }}</label>
            <input
              v-model.number="editForm.profit_min_margin_percent"
              type="number"
              step="0.1"
              min="0"
              max="99.99"
              class="input"
              placeholder="0"
              :title="t('admin.groups.profitControl.minMarginHint')"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.profitControl.safetyBuffer") }}</label>
            <input
              v-model.number="editForm.profit_safety_buffer_percent"
              type="number"
              step="0.1"
              min="0"
              max="99.99"
              class="input"
              placeholder="0"
              :title="t('admin.groups.profitControl.safetyBufferHint')"
            />
          </div>
        </div>
      </div>

      <!-- 支持的模型系列（仅 antigravity 平台） -->
      <div v-if="editForm.platform === 'antigravity'" class="border-t pt-4">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-h3 font-bold text-accent-strong">
            {{ t("admin.groups.supportedScopes.title") }}
          </label>
          <!-- Help Tooltip -->
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-fg-subtle transition-colors hover:text-accent-strong"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-sm border border-border-strong bg-surface-raised p-3 text-fg shadow-overlay"
              >
                <p class="text-xs leading-relaxed text-fg-subtle">
                  {{ t("admin.groups.supportedScopes.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 border-b border-r border-border-strong bg-surface-raised"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <div class="space-y-2">
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              :checked="editForm.supported_model_scopes.includes('claude')"
              @change="toggleEditScope('claude')"
              class="h-4 w-4 rounded-sm border-border-strong text-accent focus:ring-accent"
            />
            <span class="text-sm text-fg">{{
              t("admin.groups.supportedScopes.claude")
            }}</span>
          </label>
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              :checked="
                editForm.supported_model_scopes.includes('gemini_text')
              "
              @change="toggleEditScope('gemini_text')"
              class="h-4 w-4 rounded-sm border-border-strong text-accent focus:ring-accent"
            />
            <span class="text-sm text-fg">{{
              t("admin.groups.supportedScopes.geminiText")
            }}</span>
          </label>
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              :checked="
                editForm.supported_model_scopes.includes('gemini_image')
              "
              @change="toggleEditScope('gemini_image')"
              class="h-4 w-4 rounded-sm border-border-strong text-accent focus:ring-accent"
            />
            <span class="text-sm text-fg">{{
              t("admin.groups.supportedScopes.geminiImage")
            }}</span>
          </label>
        </div>
        <p class="mt-2 text-xs text-fg-muted">
          {{ t("admin.groups.supportedScopes.hint") }}
        </p>
      </div>

      <!-- MCP XML 协议注入（仅 antigravity 平台） -->
      <div v-if="editForm.platform === 'antigravity'" class="border-t pt-4">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-h3 font-bold text-accent-strong">
            {{ t("admin.groups.mcpXml.title") }}
          </label>
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-fg-subtle transition-colors hover:text-accent-strong"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-sm border border-border-strong bg-surface-raised p-3 text-fg shadow-overlay"
              >
                <p class="text-xs leading-relaxed text-fg-subtle">
                  {{ t("admin.groups.mcpXml.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 border-b border-r border-border-strong bg-surface-raised"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <Toggle v-model="editForm.mcp_xml_inject" />
          <span class="text-sm text-fg-muted">
            {{
              editForm.mcp_xml_inject
                ? t("admin.groups.mcpXml.enabled")
                : t("admin.groups.mcpXml.disabled")
            }}
          </span>
        </div>
      </div>

      <!-- Claude Code 客户端限制（仅 anthropic 平台） -->
      <div v-if="editForm.platform === 'anthropic'" class="border-t pt-4">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-h3 font-bold text-accent-strong">
            {{ t("admin.groups.claudeCode.title") }}
          </label>
          <!-- Help Tooltip -->
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-fg-subtle transition-colors hover:text-accent-strong"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-sm border border-border-strong bg-surface-raised p-3 text-fg shadow-overlay"
              >
                <p class="text-xs leading-relaxed text-fg-subtle">
                  {{ t("admin.groups.claudeCode.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 border-b border-r border-border-strong bg-surface-raised"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <Toggle v-model="editForm.claude_code_only" />
          <span class="text-sm text-fg-muted">
            {{
              editForm.claude_code_only
                ? t("admin.groups.claudeCode.enabled")
                : t("admin.groups.claudeCode.disabled")
            }}
          </span>
        </div>
        <!-- 降级分组选择（仅当启用 claude_code_only 时显示） -->
        <div v-if="editForm.claude_code_only" class="mt-3">
          <label class="input-label">{{
            t("admin.groups.claudeCode.fallbackGroup")
          }}</label>
          <Select
            v-model="editForm.fallback_group_id"
            :options="fallbackGroupOptionsForEdit"
            :placeholder="t('admin.groups.claudeCode.noFallback')"
          />
          <p class="input-hint">
            {{ t("admin.groups.claudeCode.fallbackHint") }}
          </p>
        </div>
      </div>

      <!-- Codex 网页搜索按次计费（仅 openai 平台） -->
      <div
        v-if="editForm.platform === 'openai'"
        class="border-t border-border pt-4 mt-4"
      >
        <h4 class="mb-3 text-h3 font-bold text-accent-strong">
          {{ t("admin.groups.webSearchPricing.title") }}
        </h4>
        <div>
          <label class="input-label">{{
            t("admin.groups.webSearchPricing.pricePerCall")
          }}</label>
          <input
            v-model.number="editForm.web_search_price_per_call"
            type="number"
            step="0.001"
            min="0"
            placeholder="0.01"
            class="input"
          />
          <p class="input-hint">
            {{ t("admin.groups.webSearchPricing.pricePerCallHint") }}
          </p>
          <div
            class="mt-2 border border-accent/40 pl-3 text-xs tabular-nums text-fg"
          >
            {{
              t("admin.groups.webSearchPricing.finalPricePreview", {
                price: editWebSearchFinalPricePreview,
              })
            }}
          </div>
        </div>
      </div>

      <!-- 固定账号获取 Codex Model Manifest（仅 openai 平台，仅编辑对话框） -->
      <CodexManifestAccountsField
        v-if="editForm.platform === 'openai' && editingGroup"
        ref="editCodexManifestRef"
        :group-id="editingGroup.id"
        :model-value="editCodexManifestConfig"
        @update:model-value="Object.assign(editCodexManifestConfig, $event)"
        :account-names="editCodexManifestAccountNames"
      />


      <div class="border-t border-border pt-4 mt-4">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <h4 class="text-h3 font-bold text-accent-strong">{{ t("admin.groups.modelPricing.title") }}</h4>
            <p class="mt-1 text-xs text-fg-muted">{{ t("admin.groups.modelPricing.description") }}</p>
          </div>
          <button type="button" class="btn btn-secondary shrink-0 whitespace-nowrap" @click="addGroupPricing(editForm.model_pricing)">
            <Icon name="plus" size="sm" class="mr-1" />{{ t("admin.groups.modelPricing.add") }}
          </button>
        </div>
        <label class="mt-3 flex items-start gap-2">
          <input v-model="editForm.long_context_pricing_enabled" type="checkbox" class="mt-0.5" />
          <span><span class="block text-sm text-fg">{{ t("admin.groups.modelPricing.longContext") }}</span><span class="block text-xs text-fg-muted">{{ t("admin.groups.modelPricing.longContextHint") }}</span></span>
        </label>
        <div class="mt-3 space-y-2">
          <PricingEntryCard v-for="(entry, index) in editForm.model_pricing" :key="index" :entry="entry" :platform="editForm.platform" hide-token-intervals @update="editForm.model_pricing[index] = $event" @remove="editForm.model_pricing.splice(index, 1)" />
        </div>
      </div>

      <!-- Grok Voice 显式定价（仅 grok 平台） -->
      <div
        v-if="editForm.platform === 'grok'"
        class="border-t border-border pt-4 mt-4"
      >
        <h4 class="mb-1 text-h3 font-bold text-accent-strong">
          {{ t("admin.groups.explicitPricing.title") }}
        </h4>
        <p class="text-xs text-fg-muted mb-3">
          {{ t("admin.groups.explicitPricing.description") }}
        </p>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
          <div>
            <label class="input-label">{{ t("admin.groups.explicitPricing.searchPricePer1k") }}</label>
            <input
              v-model.number="editForm.search_price_per_1k"
              type="number"
              step="0.000001"
              min="0"
              class="input"
              :placeholder="t('admin.groups.explicitPricing.pricePlaceholder')"
              data-testid="edit-search-price"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.voicePricing.audioRealtimePerMin") }}</label>
            <input
              v-model.number="editForm.audio_realtime_price_per_min"
              type="number"
              step="0.000001"
              min="0"
              class="input"
              :placeholder="t('admin.groups.voicePricing.pricePlaceholder')"
              data-testid="edit-audio-realtime-price"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.voicePricing.audioTtsPerMillionChars") }}</label>
            <input
              v-model.number="editForm.audio_tts_price_per_million_chars"
              type="number"
              step="0.000001"
              min="0"
              class="input"
              :placeholder="t('admin.groups.voicePricing.pricePlaceholder')"
              data-testid="edit-audio-tts-price"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.voicePricing.audioSttPerHour") }}</label>
            <input
              v-model.number="editForm.audio_stt_price_per_hour"
              type="number"
              step="0.000001"
              min="0"
              class="input"
              :placeholder="t('admin.groups.voicePricing.pricePlaceholder')"
              data-testid="edit-audio-stt-price"
            />
          </div>
        </div>
      </div>
      <!-- OpenAI Fast 开关（OpenAI 与 Composite 平台） -->
      <div
        v-if="supportsGroupOpenAIFast(editForm.platform)"
        class="border-t border-border pt-4 mt-4"
      >
        <h4 class="mb-3 text-h3 font-bold text-accent-strong">
          {{ t("admin.groups.openaiFast.title") }}
        </h4>
        <div class="flex items-center justify-between gap-4">
          <label class="text-sm text-fg-muted">
            {{ t("admin.groups.openaiFast.force") }}
          </label>
          <Toggle
            data-testid="edit-force-openai-fast"
            :aria-label="t('admin.groups.openaiFast.force')"
            :model-value="editForm.force_openai_fast"
            @update:modelValue="
              editForm.force_openai_fast = $event;
              if ($event) {
                editForm.disable_openai_fast = false;
                editForm.force_openai_ultrafast = false;
              }
            "
          />
        </div>
        <p class="text-xs text-fg-muted mt-1">
          {{ t("admin.groups.openaiFast.hint") }}
        </p>
        <div class="flex items-center justify-between gap-4 mt-4">
          <label class="text-sm text-fg-muted">
            {{ t("admin.groups.openaiFast.free") }}
          </label>
          <Toggle
            data-testid="edit-free-openai-fast"
            :aria-label="t('admin.groups.openaiFast.free')"
            v-model="editForm.free_openai_fast"
          />
        </div>
        <p class="text-xs text-fg-muted mt-1">
          {{ t("admin.groups.openaiFast.freeHint") }}
        </p>
        <div class="flex items-center justify-between gap-4 mt-4">
          <label class="text-sm text-fg-muted">
            {{ t("admin.groups.openaiFast.disable") }}
          </label>
          <button
            type="button"
            role="switch"
            :aria-checked="editForm.disable_openai_fast"
            :aria-label="t('admin.groups.openaiFast.disable')"
            data-testid="edit-disable-openai-fast"
            @click="
              editForm.disable_openai_fast = !editForm.disable_openai_fast;
              if (editForm.disable_openai_fast) {
                editForm.force_openai_fast = false;
                editForm.force_openai_ultrafast = false;
              }
            "
            class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="
              editForm.disable_openai_fast
                ? 'bg-danger'
                : 'bg-border-strong'
            "
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white ring-0 transition duration-200 ease-in-out"
              :class="
                editForm.disable_openai_fast ? 'translate-x-6' : 'translate-x-1'
              "
            />
          </button>
        </div>
        <p class="text-xs text-fg-muted mt-1">
          {{ t("admin.groups.openaiFast.disableHint") }}
        </p>
        <div class="flex items-center justify-between gap-4 mt-4">
          <label class="text-sm text-fg-muted">
            {{ t("admin.groups.openaiFast.ultrafast") }}
          </label>
          <button
            type="button"
            role="switch"
            :aria-checked="editForm.force_openai_ultrafast"
            :aria-label="t('admin.groups.openaiFast.ultrafast')"
            data-testid="edit-force-openai-ultrafast"
            @click="
              editForm.force_openai_ultrafast = !editForm.force_openai_ultrafast;
              if (editForm.force_openai_ultrafast) {
                editForm.force_openai_fast = false;
                editForm.disable_openai_fast = false;
              }
            "
            class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="
              editForm.force_openai_ultrafast
                ? 'bg-accent'
                : 'bg-border-strong'
            "
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white ring-0 transition duration-200 ease-in-out"
              :class="
                editForm.force_openai_ultrafast ? 'translate-x-6' : 'translate-x-1'
              "
            />
          </button>
        </div>
        <p class="text-xs text-fg-muted mt-1">
          {{ t("admin.groups.openaiFast.ultrafastHint") }}
        </p>
      </div>

      <!-- Codex Live 开关（OpenAI 与 Composite 平台） -->
      <div
        v-if="supportsLivePlatform(editForm.platform)"
        class="border-t border-border pt-4 mt-4"
      >
        <h4 class="mb-3 text-h3 font-bold text-accent-strong">
          {{ t("admin.groups.openaiLive.title") }}
        </h4>
        <div class="flex items-center justify-between">
          <label class="text-sm text-fg-muted">{{
            t("admin.groups.openaiLive.allow")
          }}</label>
          <Toggle
            :model-value="editForm.allow_live"
            @update:model-value="toggleLive('edit')"
          />
        </div>
        <p class="text-xs text-fg-muted mt-1">
          {{ t("admin.groups.openaiLive.hint") }}
        </p>
      </div>

      <!-- OpenAI Messages 调度配置（OpenAI 与 Composite 平台） -->
      <div
        v-if="supportsMessagesDispatchPlatform(editForm.platform)"
        class="border-t border-border pt-4 mt-4"
      >
        <h4 class="mb-3 text-h3 font-bold text-accent-strong">
          {{ t("admin.groups.openaiMessages.title") }}
        </h4>

        <!-- 允许 Messages 调度开关 -->
        <div class="flex items-center justify-between">
          <label class="text-sm text-fg-muted">{{
            t("admin.groups.openaiMessages.allowDispatch")
          }}</label>
          <Toggle v-model="editForm.allow_messages_dispatch" />
        </div>
        <p class="text-xs text-fg-muted mt-1">
          {{ t("admin.groups.openaiMessages.allowDispatchHint") }}
        </p>

        <div
          v-if="
            editForm.platform === 'openai' && editForm.allow_messages_dispatch
          "
          class="mt-3"
        >
          <div
            class="border-t border-border"
          >
            <div
              class="pt-3"
            >
              <div class="flex items-center gap-2">
                <label
                  class="text-sm font-medium text-fg"
                  >{{
                    t("admin.groups.openaiMessages.familyMappingTitle")
                  }}</label
                >
              </div>
              <p class="mt-1 text-xs text-fg-muted">
                {{ t("admin.groups.openaiMessages.familyMappingHint") }}
              </p>
            </div>
            <div class="pt-3">
              <div class="grid gap-4 md:grid-cols-3">
                <div>
                  <label class="input-label">{{
                    t("admin.groups.openaiMessages.opusModel")
                  }}</label>
                  <input
                    v-model="editForm.opus_mapped_model"
                    type="text"
                    :placeholder="
                      t('admin.groups.openaiMessages.opusModelPlaceholder')
                    "
                    class="input"
                  />
                </div>
                <div>
                  <label class="input-label">{{
                    t("admin.groups.openaiMessages.sonnetModel")
                  }}</label>
                  <input
                    v-model="editForm.sonnet_mapped_model"
                    type="text"
                    :placeholder="
                      t('admin.groups.openaiMessages.sonnetModelPlaceholder')
                    "
                    class="input"
                  />
                </div>
                <div>
                  <label class="input-label">{{
                    t("admin.groups.openaiMessages.haikuModel")
                  }}</label>
                  <input
                    v-model="editForm.haiku_mapped_model"
                    type="text"
                    :placeholder="
                      t('admin.groups.openaiMessages.haikuModelPlaceholder')
                    "
                    class="input"
                  />
                </div>
              </div>
            </div>
          </div>

          <div
            class="mt-5 border-t border-border"
          >
            <div
              class="pt-3"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <div class="flex items-center gap-2">
                    <label
                      class="text-label font-semibold text-fg"
                      >{{
                        t("admin.groups.openaiMessages.exactMappingTitle")
                      }}</label
                    >
                  </div>
                  <p
                    class="mt-1 text-xs text-fg-muted"
                  >
                    {{ t("admin.groups.openaiMessages.exactMappingHint") }}
                  </p>
                </div>
              </div>
            </div>

            <div class="pt-3">
              <div
                v-if="editForm.exact_model_mappings.length === 0"
                class="flex items-center justify-between gap-3 border border-dashed border-border-strong px-4 py-3 text-sm text-fg-muted"
              >
                <span>{{
                  t("admin.groups.openaiMessages.noExactMappings")
                }}</span>
                <button
                  type="button"
                  @click="addEditMessagesDispatchMapping"
                  class="flex items-center gap-1.5 text-sm font-medium text-accent transition-colors hover:text-accent-strong"
                >
                  <Icon name="plus" size="sm" />
                  {{ t("admin.groups.openaiMessages.addExactMapping") }}
                </button>
              </div>

              <div v-else class="space-y-3">
                <div
                  v-for="row in editForm.exact_model_mappings"
                  :key="getEditMessagesDispatchRowKey(row)"
                  class="border-b border-border pb-3"
                >
                  <div class="flex items-center gap-4">
                    <div
                      class="grid flex-1 gap-4 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] md:items-start"
                    >
                      <div>
                        <label class="input-label">{{
                          t("admin.groups.openaiMessages.claudeModel")
                        }}</label>
                        <input
                          v-model="row.claude_model"
                          type="text"
                          :placeholder="
                            t(
                              'admin.groups.openaiMessages.claudeModelPlaceholder',
                            )
                          "
                          class="input font-mono"
                        />
                      </div>
                      <div
                        class="hidden text-fg-subtle md:flex md:justify-center md:pt-7"
                      >
                        <Icon
                          name="arrowRight"
                          size="sm"
                          
                        />
                      </div>
                      <div>
                        <label class="input-label">{{
                          t("admin.groups.openaiMessages.targetModel")
                        }}</label>
                        <input
                          v-model="row.target_model"
                          type="text"
                          :placeholder="
                            t(
                              'admin.groups.openaiMessages.targetModelPlaceholder',
                            )
                          "
                          class="input font-mono"
                        />
                      </div>
                    </div>
                    <button
                      type="button"
                      @click="removeEditMessagesDispatchMapping(row)"
                      class="mt-6 flex h-9 w-9 items-center justify-center rounded-sm text-fg-subtle transition-colors hover:bg-danger-weak hover:text-danger"
                      :title="
                        t('admin.groups.openaiMessages.removeExactMapping')
                      " :aria-label="
                        t('admin.groups.openaiMessages.removeExactMapping')
                      "
                    >
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </div>

                <button
                  type="button"
                  @click="addEditMessagesDispatchMapping"
                  class="btn btn-secondary w-full border-dashed"
                >
                  <Icon name="plus" size="sm" />
                  {{ t("admin.groups.openaiMessages.addExactMapping") }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 账号过滤控制 (OpenAI/Antigravity/Anthropic/Gemini) -->
      <div
        v-if="
          ['openai', 'antigravity', 'anthropic', 'gemini'].includes(
            editForm.platform,
          )
        "
        class="border-t border-border pt-4 mt-4 space-y-4"
      >
        <h4 class="mb-3 text-h3 font-bold text-accent-strong">
          {{ t("admin.groups.accountFilters.title") }}
        </h4>

        <!-- require_oauth_only toggle -->
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm text-fg-muted"
              >{{ t("admin.groups.accountFilters.oauthOnly") }}</label
            >
            <p class="text-xs text-fg-muted mt-0.5">
              {{
                editForm.require_oauth_only
                  ? t("admin.groups.accountFilters.oauthOnlyEnabled")
                  : t("admin.groups.accountFilters.disabled")
              }}
            </p>
          </div>
          <Toggle v-model="editForm.require_oauth_only" />
        </div>

        <!-- require_privacy_set toggle -->
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm text-fg-muted"
              >{{ t("admin.groups.accountFilters.privacySetOnly") }}</label
            >
            <p class="text-xs text-fg-muted mt-0.5">
              {{
                editForm.require_privacy_set
                  ? t("admin.groups.accountFilters.privacySetOnlyEnabled")
                  : t("admin.groups.accountFilters.disabled")
              }}
            </p>
          </div>
          <Toggle v-model="editForm.require_privacy_set" />
        </div>
      </div>

      <!-- 无效请求兜底（仅 anthropic/antigravity 平台，且非订阅分组） -->
      <div
        v-if="
          ['anthropic', 'antigravity'].includes(editForm.platform) &&
          editForm.subscription_type !== 'subscription'
        "
        class="border-t pt-4"
      >
        <label class="input-label">{{
          t("admin.groups.invalidRequestFallback.title")
        }}</label>
        <Select
          v-model="editForm.fallback_group_id_on_invalid_request"
          :options="invalidRequestFallbackOptionsForEdit"
          :placeholder="t('admin.groups.invalidRequestFallback.noFallback')"
        />
        <p class="input-hint">
          {{ t("admin.groups.invalidRequestFallback.hint") }}
        </p>
      </div>

      <!-- 模型路由配置（仅 anthropic 平台） -->
      <div v-if="editForm.platform === 'anthropic'" class="border-t pt-4">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-h3 font-bold text-accent-strong">
            {{ t("admin.groups.modelRouting.title") }}
          </label>
          <!-- Help Tooltip -->
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-fg-subtle transition-colors hover:text-accent-strong"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-80 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-sm border border-border-strong bg-surface-raised p-3 text-fg shadow-overlay"
              >
                <p class="text-xs leading-relaxed text-fg-subtle">
                  {{ t("admin.groups.modelRouting.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 border-b border-r border-border-strong bg-surface-raised"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <!-- 启用开关 -->
        <div class="flex items-center gap-3 mb-3">
          <Toggle v-model="editForm.model_routing_enabled" />
          <span class="text-sm text-fg-muted">
            {{
              editForm.model_routing_enabled
                ? t("admin.groups.modelRouting.enabled")
                : t("admin.groups.modelRouting.disabled")
            }}
          </span>
        </div>
        <p
          v-if="!editForm.model_routing_enabled"
          class="text-xs text-fg-muted mb-3"
        >
          {{ t("admin.groups.modelRouting.disabledHint") }}
        </p>
        <p v-else class="text-xs text-fg-muted mb-3">
          {{ t("admin.groups.modelRouting.noRulesHint") }}
        </p>
        <!-- 路由规则列表（仅在启用时显示） -->
        <div v-if="editForm.model_routing_enabled" class="space-y-3">
          <div
            v-for="rule in editModelRoutingRules"
            :key="getEditRuleRenderKey(rule)"
            class="border-t border-border pt-3"
          >
            <div class="flex items-start gap-3">
              <div class="flex-1 space-y-2">
                <div>
                  <label class="input-label text-xs">{{
                    t("admin.groups.modelRouting.modelPattern")
                  }}</label>
                  <input
                    v-model="rule.pattern"
                    type="text"
                    class="input text-sm"
                    :placeholder="
                      t('admin.groups.modelRouting.modelPatternPlaceholder')
                    "
                  />
                </div>
                <div>
                  <label class="input-label text-xs">{{
                    t("admin.groups.modelRouting.accounts")
                  }}</label>
                  <!-- 已选账号标签 -->
                  <div
                    v-if="rule.accounts.length > 0"
                    class="flex flex-wrap gap-1.5 mb-2"
                  >
                    <span
                      v-for="account in rule.accounts"
                      :key="account.id"
                      class="badge badge-primary gap-1"
                    >
                      {{ account.name }}
                      <button
                        type="button"
                        @click="removeSelectedAccount(rule, account.id, true)"
                        class="ml-0.5 text-accent hover:text-accent-strong"
                      >
                        <Icon name="x" size="xs" />
                      </button>
                    </span>
                  </div>
                  <!-- 账号搜索输入框 -->
                  <div class="relative account-search-container">
                    <input
                      v-model="
                        accountSearchKeyword[getEditRuleSearchKey(rule)]
                      "
                      type="text"
                      class="input text-sm"
                      :placeholder="
                        t(
                          'admin.groups.modelRouting.searchAccountPlaceholder',
                        )
                      "
                      @input="searchAccountsByRule(rule, true)"
                      @focus="onAccountSearchFocus(rule, true)"
                    />
                    <!-- 搜索结果下拉框 -->
                    <div
                      v-if="
                        showAccountDropdown[getEditRuleSearchKey(rule)] &&
                        accountSearchResults[getEditRuleSearchKey(rule)]
                          ?.length > 0
                      "
                      class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-sm border border-border-strong bg-surface-raised shadow-overlay"
                    >
                      <button
                        v-for="account in accountSearchResults[
                          getEditRuleSearchKey(rule)
                        ]"
                        :key="account.id"
                        type="button"
                        @click="selectAccount(rule, account, true)"
                        class="w-full px-3 py-2 text-left text-sm hover:bg-accent-weak"
                        :class="{
                          'opacity-50': rule.accounts.some(
                            (a) => a.id === account.id,
                          ),
                        }"
                        :disabled="
                          rule.accounts.some((a) => a.id === account.id)
                        "
                      >
                        <span>{{ account.name }}</span>
                        <span class="ml-2 font-mono text-xs text-fg-subtle"
                          >#{{ account.id }}</span
                        >
                      </button>
                    </div>
                  </div>
                  <p class="text-xs text-fg-subtle mt-1">
                    {{ t("admin.groups.modelRouting.accountsHint") }}
                  </p>
                </div>
              </div>
              <button
                type="button"
                @click="removeEditRoutingRule(rule)"
                class="mt-5 p-1.5 text-fg-subtle hover:text-danger transition-colors"
                :title="t('admin.groups.modelRouting.removeRule')" :aria-label="t('admin.groups.modelRouting.removeRule')"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>
        <!-- 添加规则按钮（仅在启用时显示） -->
        <button
          v-if="editForm.model_routing_enabled"
          type="button"
          @click="addEditRoutingRule"
          class="mt-3 flex items-center gap-1.5 text-sm text-accent hover:text-accent-strong"
        >
          <Icon name="plus" size="sm" />
          {{ t("admin.groups.modelRouting.addRule") }}
        </button>
      </div>
      </template>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3 pt-4">
        <button
          @click="closeEditModal"
          type="button"
          class="btn btn-secondary"
        >
          {{ t("common.cancel") }}
        </button>
        <button
          type="submit"
          form="edit-group-form"
          :disabled="submitting"
          class="btn btn-primary"
          data-tour="group-form-submit"
        >
          <svg
            v-if="submitting"
            class="-ml-1 mr-2 h-4 w-4 animate-spin"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          {{ submitting ? t("admin.groups.updating") : t("common.update") }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useGroupsViewContext } from "./context";
import BaseDialog from "@/components/common/BaseDialog.vue";
import CodexManifestAccountsField from "@/components/admin/group/CodexManifestAccountsField.vue";
import Icon from "@/components/icons/Icon.vue";
import PricingEntryCard from "@/components/admin/channel/PricingEntryCard.vue";
import ReasoningEffortPolicyFields from "@/components/admin/group/ReasoningEffortPolicyFields.vue";
import Select from "@/components/common/Select.vue";
import Toggle from "@/components/common/Toggle.vue";

// 纯移动拆分：所有状态与方法来自 GroupsView 提供的上下文（openspec: rebuild-frontend-design-system Phase 3）
const ctx = useGroupsViewContext();
const {
  accountSearchKeyword,
  accountSearchResults,
  addEditMessagesDispatchMapping,
  addEditRoutingRule,
  addGroupPricing,
  authStore,
  closeEditModal,
  copyAccountsGroupOptionsForEdit,
  editAllowlistCustomEntry,
  editAllowlistCustomErrorKey,
  editCodexManifestAccountNames,
  editCodexManifestConfig,
  editForm,
  editImageFinalPricePreview,
  editModelAllowlistLoading,
  editModelAllowlistSelectedCount,
  editModelAllowlistState,
  editModelRoutingRules,
  editStatusOptions,
  editVideoFinalPricePreview,
  editWebSearchFinalPricePreview,
  editingGroup,
  fallbackGroupOptionsForEdit,
  getEditMessagesDispatchRowKey,
  getEditRuleRenderKey,
  getEditRuleSearchKey,
  getImagePricePlaceholder,
  getVideoPricePlaceholder,
  grokVideoPriceResolutions,
  handleUpdateGroup,
  imagePricingI18nKey,
  invalidRequestFallbackOptionsForEdit,
  invertModelAllowlistSelection,
  isProfitControlPlatform,
  moveEditModelAllowlistItem,
  onAccountSearchFocus,
  platformOptions,
  removeEditMessagesDispatchMapping,
  removeEditRoutingRule,
  removeSelectedAccount,
  searchAccountsByRule,
  selectAccount,
  selectAllModelAllowlistItems,
  showAccountDropdown,
  showEditModal,
  submitEditAllowlistCustomEntry,
  submitting,
  subscriptionTypeOptions,
  supportsGroupOpenAIFast,
  supportsImagePricingPlatform,
  supportsLivePlatform,
  supportsMessagesDispatchPlatform,
  supportsReasoningEffortPolicyPlatform,
  supportsVideoPricingPlatform,
  t,
  toggleEditScope,
  toggleLive,
  videoModelPriceFamilyRows,
  videoPricingI18nKey,
  editCodexManifestRef,
  editReasoningEffortPolicyRef,
} = ctx;
</script>
