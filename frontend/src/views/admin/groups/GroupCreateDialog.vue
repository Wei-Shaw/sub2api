<template>
  <BaseDialog
    :show="showCreateModal"
    :title="t('admin.groups.createGroup')"
    width="normal"
    @close="closeCreateModal"
  >
    <form
      id="create-group-form"
      @submit.prevent="handleCreateGroup"
      class="space-y-5"
    >
      <div>
        <label class="input-label">{{ t("admin.groups.form.name") }}</label>
        <input
          v-model="createForm.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.groups.enterGroupName')"
          data-tour="group-form-name"
        />
      </div>
      <div>
        <label class="input-label">{{
          t("admin.groups.form.description")
        }}</label>
        <textarea
          v-model="createForm.description"
          rows="3"
          class="input"
          :placeholder="t('admin.groups.optionalDescription')"
        ></textarea>
      </div>
      <div>
        <label class="input-label">{{
          t("admin.groups.form.platform")
        }}</label>
        <Select
          v-model="createForm.platform"
          :options="platformOptions"
          data-tour="group-form-platform"
          @change="createForm.copy_accounts_from_group_ids = []"
        />
        <p class="input-hint">{{ t("admin.groups.platformHint") }}</p>
      </div>
      <!-- 从分组复制账号 -->
      <div v-if="copyAccountsGroupOptions.length > 0">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-sm font-medium text-ink-secondary">
            {{ t("admin.groups.copyAccounts.title") }}
          </label>
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-ink-tertiary transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
              >
                <p class="text-xs leading-relaxed text-gray-300">
                  {{ t("admin.groups.copyAccounts.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <!-- 已选分组标签 -->
        <div
          v-if="createForm.copy_accounts_from_group_ids.length > 0"
          class="flex flex-wrap gap-1.5 mb-2"
        >
          <span
            v-for="groupId in createForm.copy_accounts_from_group_ids"
            :key="groupId"
            class="inline-flex items-center gap-1 rounded-sm bg-primary-100 px-2.5 py-1 text-xs font-medium text-accent dark:bg-primary-900/30"
          >
            {{
              copyAccountsGroupOptions.find((o) => o.value === groupId)
                ?.label || `#${groupId}`
            }}
            <button
              type="button"
              @click="
                createForm.copy_accounts_from_group_ids =
                  createForm.copy_accounts_from_group_ids.filter(
                    (id) => id !== groupId,
                  )
              "
              class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
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
                !createForm.copy_accounts_from_group_ids.includes(val)
              ) {
                createForm.copy_accounts_from_group_ids.push(val);
              }
              (e.target as HTMLSelectElement).value = '';
            }
          "
        >
          <option value="">
            {{ t("admin.groups.copyAccounts.selectPlaceholder") }}
          </option>
          <option
            v-for="opt in copyAccountsGroupOptions"
            :key="opt.value"
            :value="opt.value"
            :disabled="
              createForm.copy_accounts_from_group_ids.includes(opt.value)
            "
          >
            {{ opt.label }}
          </option>
        </select>
        <p class="input-hint">{{ t("admin.groups.copyAccounts.hint") }}</p>
      </div>
      <div>
        <label class="input-label">{{
          t("admin.groups.form.rateMultiplier")
        }}</label>
        <input
          v-model.number="createForm.rate_multiplier"
          type="number"
          step="0.001"
          min="0.001"
          required
          class="input"
          data-tour="group-form-multiplier"
        />
        <p class="input-hint">{{ t("admin.groups.rateMultiplierHint") }}</p>
      </div>
      <div>
        <label class="input-label">{{ t("admin.groups.form.rpmLimit") }}</label>
        <input
          v-model.number="createForm.rpm_limit"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.groups.form.rpmLimitPlaceholder')"
        />
        <p class="input-hint">{{ t("admin.groups.form.rpmLimitHint") }}</p>
      </div>
      <ReasoningEffortPolicyFields
        v-if="supportsReasoningEffortPolicyPlatform(createForm.platform)"
        ref="createReasoningEffortPolicyRef"
        id-prefix="create-group-reasoning"
        :platform="createForm.platform"
        v-model:max-effort="createForm.max_reasoning_effort"
        v-model:mappings="createForm.reasoning_effort_mappings"
      />
      <div
        v-if="createForm.subscription_type !== 'subscription'"
        data-tour="group-form-exclusive"
      >
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-sm font-medium text-ink-secondary">
            {{ t("admin.groups.form.exclusive") }}
          </label>
          <!-- Help Tooltip -->
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-ink-tertiary transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
            />
            <!-- Tooltip Popover -->
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
              >
                <p class="mb-2 text-xs font-medium">
                  {{ t("admin.groups.exclusiveTooltip.title") }}
                </p>
                <p class="mb-2 text-xs leading-relaxed text-gray-300">
                  {{ t("admin.groups.exclusiveTooltip.description") }}
                </p>
                <div class="rounded bg-gray-800 p-2 dark:bg-gray-700">
                  <p class="text-xs leading-relaxed text-gray-300">
                    <span
                      class="inline-flex items-center gap-1 text-primary-400"
                      ><Icon name="lightbulb" size="xs" />
                      {{ t("admin.groups.exclusiveTooltip.example") }}</span
                    >
                    {{ t("admin.groups.exclusiveTooltip.exampleContent") }}
                  </p>
                </div>
                <!-- Arrow -->
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <button
            type="button"
            @click="createForm.is_exclusive = !createForm.is_exclusive"
            :class="[
              'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
              createForm.is_exclusive
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600',
            ]"
          >
            <span
              :class="[
                'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                createForm.is_exclusive ? 'translate-x-6' : 'translate-x-1',
              ]"
            />
          </button>
          <span class="text-sm text-ink-secondary">
            {{
              createForm.is_exclusive
                ? t("admin.groups.exclusive")
                : t("admin.groups.public")
            }}
          </span>
        </div>
      </div>

      <!-- Subscription Configuration -->
      <div class="mt-4 border-t pt-4">
        <div>
          <label class="input-label">{{
            t("admin.groups.subscription.type")
          }}</label>
          <Select
            v-model="createForm.subscription_type"
            :options="subscriptionTypeOptions"
          />
          <p class="input-hint">
            {{ t("admin.groups.subscription.typeHint") }}
          </p>
        </div>

        <!-- Subscription limits (only show when subscription type is selected) -->
        <div
          v-if="createForm.subscription_type === 'subscription'"
          class="space-y-4 border-l-2 border-primary-200 pl-4 dark:border-primary-800"
        >
          <div>
            <label class="input-label">{{
              t("admin.groups.subscription.dailyLimit")
            }}</label>
            <input
              v-model.number="createForm.daily_limit_usd"
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
              v-model.number="createForm.weekly_limit_usd"
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
              v-model.number="createForm.monthly_limit_usd"
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
            <label class="text-sm font-medium text-ink-secondary">
              {{ t("admin.groups.modelsList.title") }}
            </label>
            <p class="mt-1 text-xs text-ink-secondary">
              {{ t("admin.groups.modelsList.hint") }}
            </p>
          </div>
          <button
            type="button"
            @click="createModelsListState.enabled = !createModelsListState.enabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors',
              createModelsListState.enabled
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600',
            ]"
          >
            <span
              :class="[
                'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                createModelsListState.enabled ? 'translate-x-6' : 'translate-x-1',
              ]"
            />
          </button>
        </div>
        <div
          v-if="createModelsListState.enabled"
          class="overflow-hidden rounded-lg border border-line bg-gray-50/50 dark:bg-dark-800/40"
        >
          <div
            v-if="!createModelsListLoading && createModelsListState.items.length > 0"
            class="flex items-center justify-between gap-2 border-b border-line bg-surface-sunken px-3 py-2 text-xs"
          >
            <span class="text-ink-secondary">
              {{
                t("admin.groups.modelsList.selectedSummary", {
                  selected: createModelsListSelectedCount,
                  total: createModelsListState.items.length,
                })
              }}
            </span>
            <div class="flex items-center gap-1.5">
              <button
                type="button"
                class="rounded px-2 py-1 font-medium text-primary-600 transition-colors hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-900/20"
                @click="selectAllModelsListItems(createModelsListState)"
              >
                {{ t("admin.groups.modelsList.selectAll") }}
              </button>
              <button
                type="button"
                class="rounded px-2 py-1 font-medium text-ink-secondary transition-colors hover:bg-surface-hover"
                @click="invertModelsListSelection(createModelsListState)"
              >
                {{ t("admin.groups.modelsList.invertSelection") }}
              </button>
            </div>
          </div>
          <div
            class="max-h-64 space-y-2 overflow-y-auto p-2"
          >
            <p v-if="createModelsListLoading" class="text-xs text-ink-secondary">
              {{ t("admin.groups.modelsList.loading") }}
            </p>
            <p
              v-else-if="createModelsListState.items.length === 0"
              class="text-xs text-ink-secondary"
            >
              {{ t("admin.groups.modelsList.empty") }}
            </p>
            <div
              v-for="(item, index) in createModelsListState.items"
              :key="item.id"
              class="flex items-center gap-2 rounded border border-line bg-surface px-3 py-2"
            >
              <input
                v-model="item.selected"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="min-w-0 flex-1 break-all text-sm text-ink-secondary">
                {{ item.id }}
              </span>
              <button
                type="button"
                :disabled="index === 0"
                class="rounded p-1 text-ink-tertiary hover:bg-gray-100 hover:text-gray-700 disabled:opacity-40 dark:hover:bg-dark-600 dark:hover:text-gray-200"
                @click="moveCreateModelsListItem(index, index - 1)"
              >
                <Icon name="arrowUp" size="sm" />
              </button>
              <button
                type="button"
                :disabled="index === createModelsListState.items.length - 1"
                class="rounded p-1 text-ink-tertiary hover:bg-gray-100 hover:text-gray-700 disabled:opacity-40 dark:hover:bg-dark-600 dark:hover:text-gray-200"
                @click="moveCreateModelsListItem(index, index + 1)"
              >
                <Icon name="arrowDown" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 图片生成计费配置 -->
      <div
        v-if="supportsImagePricingPlatform(createForm.platform)"
        class="border-t pt-4"
      >
        <label
          class="block mb-2 font-medium text-ink-secondary"
        >
          {{ t(imagePricingI18nKey(createForm.platform, "title")) }}
        </label>
        <p class="text-xs text-ink-secondary mb-3">
          {{ t(imagePricingI18nKey(createForm.platform, "description")) }}
        </p>
        <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-2">
          <label class="flex items-center gap-2 text-sm text-ink-secondary">
            <input
              v-model="createForm.allow_image_generation"
              type="checkbox"
              class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            {{ t(imagePricingI18nKey(createForm.platform, "allowImageGeneration")) }}
          </label>
          <label class="flex items-center gap-2 text-sm text-ink-secondary">
            <input
              v-model="createForm.image_rate_independent"
              type="checkbox"
              class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            {{ t(imagePricingI18nKey(createForm.platform, "independentMultiplier")) }}
          </label>
        </div>
        <div
          v-if="createForm.image_rate_independent"
          class="mb-4"
        >
          <label class="input-label">{{
            t(imagePricingI18nKey(createForm.platform, "imageMultiplier"))
          }}</label>
          <input
            v-model.number="createForm.image_rate_multiplier"
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
              v-model.number="createForm.image_price_1k"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getImagePricePlaceholder(createForm.platform, 'image_price_1k')"
            />
          </div>
          <div>
            <label class="input-label">2K ($)</label>
            <input
              v-model.number="createForm.image_price_2k"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getImagePricePlaceholder(createForm.platform, 'image_price_2k')"
            />
          </div>
          <div>
            <label class="input-label">4K ($)</label>
            <input
              v-model.number="createForm.image_price_4k"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getImagePricePlaceholder(createForm.platform, 'image_price_4k')"
            />
          </div>
        </div>
        <p class="mt-3 text-xs text-ink-secondary">
          {{ t(imagePricingI18nKey(createForm.platform, "modeHint")) }}
        </p>
        <div class="mt-2 rounded-lg bg-gray-50 p-3 text-xs text-ink-secondary dark:bg-gray-800">
          <div class="mb-1 font-medium">
            {{ t(imagePricingI18nKey(createForm.platform, "finalPricePreview")) }}
          </div>
          <div class="grid grid-cols-3 gap-2">
            <div
              v-for="item in createImageFinalPricePreview"
              :key="item.label"
            >
              {{ item.label }}: {{ item.value }}
            </div>
          </div>
        </div>
        <div v-if="createForm.platform === 'gemini' && createForm.allow_image_generation" class="mt-4 border-t border-dashed border-line pt-4">
          <label
            class="flex items-center gap-2 text-sm font-medium text-ink-secondary"
          >
            <input
              v-model="createForm.allow_batch_image_generation"
              type="checkbox"
              class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            {{ t("admin.groups.imagePricing.allowBatchImageGeneration") }}
          </label>
          <p class="mt-2 text-xs text-ink-secondary">
            {{ t("admin.groups.imagePricing.batchSectionHint") }}
          </p>
          <div
            v-if="createForm.allow_batch_image_generation"
            class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2"
          >
            <div>
              <label class="input-label">{{
                t("admin.groups.imagePricing.batchDiscountMultiplier")
              }}</label>
              <input
                v-model.number="createForm.batch_image_discount_multiplier"
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
                v-model.number="createForm.batch_image_hold_multiplier"
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
          v-else-if="createForm.platform !== 'gemini'"
          class="mt-4 border-t border-dashed border-line pt-4 text-xs text-ink-secondary dark:text-gray-400"
        >
          {{ t("admin.groups.imagePricing.batchGeminiOnlyHint") }}
        </p>
      </div>

      <!-- 视频生成计费配置（仅 Grok 平台） -->
      <div
        v-if="supportsVideoPricingPlatform(createForm.platform)"
        class="border-t pt-4"
      >
        <label
          class="block mb-2 font-medium text-ink-secondary"
        >
          {{ t(videoPricingI18nKey("title")) }}
        </label>
        <p class="text-xs text-ink-secondary mb-3">
          {{ t(videoPricingI18nKey("description")) }}
        </p>
        <div class="mb-4">
          <label class="flex items-center gap-2 text-sm text-ink-secondary">
            <input
              v-model="createForm.video_rate_independent"
              type="checkbox"
              class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            {{ t(videoPricingI18nKey("independentMultiplier")) }}
          </label>
        </div>
        <div
          v-if="createForm.video_rate_independent"
          class="mb-4"
        >
          <label class="input-label">{{
            t(videoPricingI18nKey("videoMultiplier"))
          }}</label>
          <input
            v-model.number="createForm.video_rate_multiplier"
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
              v-model.number="createForm.video_price_480p"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getVideoPricePlaceholder(createForm.platform, 'video_price_480p')"
            />
          </div>
          <div>
            <label class="input-label">720p ($/s)</label>
            <input
              v-model.number="createForm.video_price_720p"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getVideoPricePlaceholder(createForm.platform, 'video_price_720p')"
            />
          </div>
          <div>
            <label class="input-label">1080p ($/s)</label>
            <input
              v-model.number="createForm.video_price_1080p"
              type="number"
              step="0.001"
              min="0"
              class="input"
              :placeholder="getVideoPricePlaceholder(createForm.platform, 'video_price_1080p')"
            />
          </div>
        </div>
        <div
          class="mt-4 border-t border-dashed border-line pt-4"
          data-testid="create-grok-video-model-prices"
        >
          <p class="text-sm font-medium text-ink-secondary">
            {{ t("admin.groups.videoPricing.modelOverridesTitle") }}
          </p>
          <p class="mt-1 text-xs text-ink-secondary">
            {{ t("admin.groups.videoPricing.modelOverridesDescription") }}
          </p>
          <div class="mt-3 space-y-3">
            <div
              v-for="family in videoModelPriceFamilyRows(createForm.video_model_prices)"
              :key="family.key"
              class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_repeat(3,minmax(0,7rem))] sm:items-end"
            >
              <div class="min-w-0 pb-1 font-mono text-xs text-ink-secondary">
                {{ family.label }}
              </div>
              <label
                v-for="resolution in grokVideoPriceResolutions"
                :key="resolution.key"
                class="block"
              >
                <span class="mb-1 block text-xs text-ink-secondary">
                  {{ resolution.label }} ($/s)
                </span>
                <input
                  v-model.number="createForm.video_model_prices[family.key][resolution.key]"
                  type="number"
                  step="0.001"
                  min="0"
                  class="input"
                  :data-testid="`create-grok-video-price-${family.key}-${resolution.key}`"
                />
              </label>
            </div>
          </div>
        </div>
        <p class="mt-3 text-xs text-ink-secondary">
          {{ t(videoPricingI18nKey("modeHint")) }}
        </p>
        <div class="mt-2 rounded-lg bg-gray-50 p-3 text-xs text-ink-secondary dark:bg-gray-800">
          <div class="mb-1 font-medium">
            {{ t(videoPricingI18nKey("finalPricePreview")) }}
          </div>
          <div class="grid grid-cols-3 gap-2">
            <div
              v-for="item in createVideoFinalPricePreview"
              :key="item.label"
            >
              {{ item.label }}: {{ item.value }}
            </div>
          </div>
        </div>
      </div>

      <!-- 高峰时段倍率配置（仅订阅类型分组） -->
      <div v-if="createForm.subscription_type === 'subscription'" class="border-t pt-4">
        <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-2">
          <label class="flex items-center gap-2 text-sm text-ink-secondary">
            <input
              v-model="createForm.peak_rate_enabled"
              type="checkbox"
              class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            <span>{{ t("admin.groups.peakRate.enable") }}</span>
          </label>
        </div>
        <div
          v-if="createForm.peak_rate_enabled"
          class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-3"
        >
          <div>
            <label class="input-label">{{ t("admin.groups.peakRate.peakStart") }}</label>
            <input
              v-model="createForm.peak_start"
              type="time"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.peakRate.peakEnd") }}</label>
            <input
              v-model="createForm.peak_end"
              type="time"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.peakRate.peakMultiplier") }}</label>
            <input
              v-model.number="createForm.peak_rate_multiplier"
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
      <div v-if="isProfitControlPlatform(createForm.platform)" class="border-t pt-4">
        <label class="flex items-center gap-2 text-sm text-ink-secondary">
          <input
            v-model="createForm.profit_control_enabled"
            type="checkbox"
            class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
          />
          <span>{{ t("admin.groups.profitControl.enable") }}</span>
        </label>
        <p class="mb-3 mt-1.5 text-xs text-ink-secondary">
          {{
            createForm.profit_control_enabled
              ? t("admin.groups.profitControl.enabledHint")
              : t("admin.groups.profitControl.disabledHint")
          }}
        </p>
        <div
          v-if="createForm.profit_control_enabled"
          class="mb-3 grid grid-cols-1 gap-3 sm:grid-cols-2"
        >
          <div>
            <label class="input-label">{{ t("admin.groups.profitControl.minMargin") }}</label>
            <input
              v-model.number="createForm.profit_min_margin_percent"
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
              v-model.number="createForm.profit_safety_buffer_percent"
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
      <div v-if="createForm.platform === 'antigravity'" class="border-t pt-4">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-sm font-medium text-ink-secondary">
            {{ t("admin.groups.supportedScopes.title") }}
          </label>
          <!-- Help Tooltip -->
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-ink-tertiary transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
              >
                <p class="text-xs leading-relaxed text-gray-300">
                  {{ t("admin.groups.supportedScopes.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <div class="space-y-2">
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              :checked="createForm.supported_model_scopes.includes('claude')"
              @change="toggleCreateScope('claude')"
              class="h-4 w-4 rounded border-line text-primary-600 focus:ring-primary-500 dark:bg-dark-700"
            />
            <span class="text-sm text-ink-secondary">{{
              t("admin.groups.supportedScopes.claude")
            }}</span>
          </label>
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              :checked="
                createForm.supported_model_scopes.includes('gemini_text')
              "
              @change="toggleCreateScope('gemini_text')"
              class="h-4 w-4 rounded border-line text-primary-600 focus:ring-primary-500 dark:bg-dark-700"
            />
            <span class="text-sm text-ink-secondary">{{
              t("admin.groups.supportedScopes.geminiText")
            }}</span>
          </label>
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              :checked="
                createForm.supported_model_scopes.includes('gemini_image')
              "
              @change="toggleCreateScope('gemini_image')"
              class="h-4 w-4 rounded border-line text-primary-600 focus:ring-primary-500 dark:bg-dark-700"
            />
            <span class="text-sm text-ink-secondary">{{
              t("admin.groups.supportedScopes.geminiImage")
            }}</span>
          </label>
        </div>
        <p class="mt-2 text-xs text-ink-secondary">
          {{ t("admin.groups.supportedScopes.hint") }}
        </p>
      </div>

      <!-- MCP XML 协议注入（仅 antigravity 平台） -->
      <div v-if="createForm.platform === 'antigravity'" class="border-t pt-4">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-sm font-medium text-ink-secondary">
            {{ t("admin.groups.mcpXml.title") }}
          </label>
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-ink-tertiary transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
              >
                <p class="text-xs leading-relaxed text-gray-300">
                  {{ t("admin.groups.mcpXml.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <button
            type="button"
            @click="createForm.mcp_xml_inject = !createForm.mcp_xml_inject"
            :class="[
              'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
              createForm.mcp_xml_inject
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600',
            ]"
          >
            <span
              :class="[
                'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                createForm.mcp_xml_inject ? 'translate-x-6' : 'translate-x-1',
              ]"
            />
          </button>
          <span class="text-sm text-ink-secondary">
            {{
              createForm.mcp_xml_inject
                ? t("admin.groups.mcpXml.enabled")
                : t("admin.groups.mcpXml.disabled")
            }}
          </span>
        </div>
      </div>

      <!-- Claude Code 客户端限制（仅 anthropic 平台） -->
      <div v-if="createForm.platform === 'anthropic'" class="border-t pt-4">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-sm font-medium text-ink-secondary">
            {{ t("admin.groups.claudeCode.title") }}
          </label>
          <!-- Help Tooltip -->
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-ink-tertiary transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
              >
                <p class="text-xs leading-relaxed text-gray-300">
                  {{ t("admin.groups.claudeCode.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <button
            type="button"
            @click="
              createForm.claude_code_only = !createForm.claude_code_only
            "
            :class="[
              'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
              createForm.claude_code_only
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600',
            ]"
          >
            <span
              :class="[
                'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                createForm.claude_code_only
                  ? 'translate-x-6'
                  : 'translate-x-1',
              ]"
            />
          </button>
          <span class="text-sm text-ink-secondary">
            {{
              createForm.claude_code_only
                ? t("admin.groups.claudeCode.enabled")
                : t("admin.groups.claudeCode.disabled")
            }}
          </span>
        </div>
        <!-- 降级分组选择（仅当启用 claude_code_only 时显示） -->
        <div v-if="createForm.claude_code_only" class="mt-3">
          <label class="input-label">{{
            t("admin.groups.claudeCode.fallbackGroup")
          }}</label>
          <Select
            v-model="createForm.fallback_group_id"
            :options="fallbackGroupOptions"
            :placeholder="t('admin.groups.claudeCode.noFallback')"
          />
          <p class="input-hint">
            {{ t("admin.groups.claudeCode.fallbackHint") }}
          </p>
        </div>
      </div>

      <!-- Codex 网页搜索按次计费（仅 openai 平台） -->
      <div
        v-if="createForm.platform === 'openai'"
        class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
      >
        <h4 class="text-sm font-medium text-ink-secondary mb-3">
          {{ t("admin.groups.webSearchPricing.title") }}
        </h4>
        <div>
          <label class="input-label">{{
            t("admin.groups.webSearchPricing.pricePerCall")
          }}</label>
          <input
            v-model.number="createForm.web_search_price_per_call"
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
            class="mt-2 rounded-lg bg-surface-sunken p-3 text-xs text-ink-secondary"
          >
            {{
              t("admin.groups.webSearchPricing.finalPricePreview", {
                price: createWebSearchFinalPricePreview,
              })
            }}
          </div>
        </div>
      </div>


      <div class="border-t border-gray-200 pt-4 mt-4 dark:border-dark-400">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h4 class="text-sm font-medium text-ink-secondary">{{ t("admin.groups.modelPricing.title") }}</h4>
            <p class="mt-1 text-xs text-ink-secondary">{{ t("admin.groups.modelPricing.description") }}</p>
          </div>
          <button type="button" class="btn btn-secondary" @click="addGroupPricing(createForm.model_pricing)">
            <Icon name="plus" size="sm" class="mr-1" />{{ t("admin.groups.modelPricing.add") }}
          </button>
        </div>
        <label class="mt-3 flex items-start gap-2">
          <input v-model="createForm.long_context_pricing_enabled" type="checkbox" class="mt-0.5" />
          <span><span class="block text-sm text-ink-secondary">{{ t("admin.groups.modelPricing.longContext") }}</span><span class="block text-xs text-ink-secondary">{{ t("admin.groups.modelPricing.longContextHint") }}</span></span>
        </label>
        <div class="mt-3 space-y-2">
          <PricingEntryCard v-for="(entry, index) in createForm.model_pricing" :key="index" :entry="entry" :platform="createForm.platform" hide-token-intervals @update="createForm.model_pricing[index] = $event" @remove="createForm.model_pricing.splice(index, 1)" />
        </div>
      </div>

      <!-- Grok Voice 显式定价（仅 grok 平台） -->
      <div
        v-if="createForm.platform === 'grok'"
        class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
      >
        <h4 class="text-sm font-medium text-ink-secondary mb-1">
          {{ t("admin.groups.explicitPricing.title") }}
        </h4>
        <p class="text-xs text-ink-secondary mb-3">
          {{ t("admin.groups.explicitPricing.description") }}
        </p>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
          <div>
            <label class="input-label">{{ t("admin.groups.explicitPricing.searchPricePer1k") }}</label>
            <input
              v-model.number="createForm.search_price_per_1k"
              type="number"
              step="0.000001"
              min="0"
              class="input"
              :placeholder="t('admin.groups.explicitPricing.pricePlaceholder')"
              data-testid="create-search-price"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.voicePricing.audioRealtimePerMin") }}</label>
            <input
              v-model.number="createForm.audio_realtime_price_per_min"
              type="number"
              step="0.000001"
              min="0"
              class="input"
              :placeholder="t('admin.groups.voicePricing.pricePlaceholder')"
              data-testid="create-audio-realtime-price"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.voicePricing.audioTtsPerMillionChars") }}</label>
            <input
              v-model.number="createForm.audio_tts_price_per_million_chars"
              type="number"
              step="0.000001"
              min="0"
              class="input"
              :placeholder="t('admin.groups.voicePricing.pricePlaceholder')"
              data-testid="create-audio-tts-price"
            />
          </div>
          <div>
            <label class="input-label">{{ t("admin.groups.voicePricing.audioSttPerHour") }}</label>
            <input
              v-model.number="createForm.audio_stt_price_per_hour"
              type="number"
              step="0.000001"
              min="0"
              class="input"
              :placeholder="t('admin.groups.voicePricing.pricePlaceholder')"
              data-testid="create-audio-stt-price"
            />
          </div>
        </div>
      </div>
      <!-- OpenAI Live 开关（仅 openai 平台） -->
      <div
        v-if="createForm.platform === 'openai'"
        class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
      >
        <h4 class="text-sm font-medium text-ink-secondary mb-3">
          {{ t("admin.groups.openaiLive.title") }}
        </h4>
        <div class="flex items-center justify-between">
          <label class="text-sm text-ink-secondary">{{
            t("admin.groups.openaiLive.allow")
          }}</label>
          <button
            type="button"
            @click="toggleLive('create')"
            class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="
              createForm.allow_live
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600'
            "
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="createForm.allow_live ? 'translate-x-6' : 'translate-x-1'"
            />
          </button>
        </div>
        <p class="text-xs text-ink-secondary mt-1">
          {{ t("admin.groups.openaiLive.hint") }}
        </p>
      </div>

      <!-- OpenAI Messages 调度配置（仅 openai 平台） -->
      <div
        v-if="createForm.platform === 'openai'"
        class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
      >
        <h4 class="text-sm font-medium text-ink-secondary mb-3">
          {{ t("admin.groups.openaiMessages.title") }}
        </h4>

        <!-- 允许 Messages 调度开关 -->
        <div class="flex items-center justify-between">
          <label class="text-sm text-ink-secondary">{{
            t("admin.groups.openaiMessages.allowDispatch")
          }}</label>
          <button
            type="button"
            @click="
              createForm.allow_messages_dispatch =
                !createForm.allow_messages_dispatch
            "
            class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="
              createForm.allow_messages_dispatch
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600'
            "
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="
                createForm.allow_messages_dispatch
                  ? 'translate-x-6'
                  : 'translate-x-1'
              "
            />
          </button>
        </div>
        <p class="text-xs text-ink-secondary mt-1">
          {{ t("admin.groups.openaiMessages.allowDispatchHint") }}
        </p>

        <div v-if="createForm.allow_messages_dispatch" class="mt-3">
          <div
            class="relative overflow-hidden rounded-xl border border-line bg-surface shadow-sm"
          >
            <div
              class="border-b border-line-subtle bg-gray-50/80 px-4 py-3 dark:bg-dark-700/50"
            >
              <div class="flex items-center gap-2">
                <div class="h-2 w-2 rounded-full bg-blue-500"></div>
                <label
                  class="text-sm font-medium text-ink"
                  >{{
                    t("admin.groups.openaiMessages.familyMappingTitle")
                  }}</label
                >
              </div>
              <p class="mt-1 text-xs text-ink-secondary">
                {{ t("admin.groups.openaiMessages.familyMappingHint") }}
              </p>
            </div>
            <div class="p-4">
              <div class="grid gap-4 md:grid-cols-3">
                <div>
                  <label class="input-label">{{
                    t("admin.groups.openaiMessages.opusModel")
                  }}</label>
                  <input
                    v-model="createForm.opus_mapped_model"
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
                    v-model="createForm.sonnet_mapped_model"
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
                    v-model="createForm.haiku_mapped_model"
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
            class="mt-5 relative overflow-hidden rounded-xl border border-primary-200 bg-surface shadow-sm dark:border-primary-900/50"
          >
            <div
              class="border-b border-primary-100 border border-accent/40 bg-accent-tint/80 px-4 py-3 dark:border-primary-900/40"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <div class="flex items-center gap-2">
                    <div class="h-2 w-2 rounded-full bg-primary-500"></div>
                    <label
                      class="text-sm font-medium text-primary-900 dark:text-primary-100"
                      >{{
                        t("admin.groups.openaiMessages.exactMappingTitle")
                      }}</label
                    >
                  </div>
                  <p
                    class="mt-1 text-xs text-primary-600/90 dark:text-primary-400/90"
                  >
                    {{ t("admin.groups.openaiMessages.exactMappingHint") }}
                  </p>
                </div>
              </div>
            </div>

            <div class="p-4 bg-gray-50/30 dark:bg-dark-800/30">
              <div
                v-if="createForm.exact_model_mappings.length === 0"
                class="flex items-center justify-between gap-3 rounded-xl border-2 border-dashed border-primary-200 bg-surface px-5 py-4 text-sm text-accent transition-colors hover:border-primary-300 dark:border-primary-900/40 dark:hover:border-primary-800"
              >
                <span>{{
                  t("admin.groups.openaiMessages.noExactMappings")
                }}</span>
                <button
                  type="button"
                  @click="addCreateMessagesDispatchMapping"
                  class="flex items-center gap-1.5 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  <Icon name="plus" size="sm" />
                  {{ t("admin.groups.openaiMessages.addExactMapping") }}
                </button>
              </div>

              <div v-else class="space-y-3">
                <div
                  v-for="row in createForm.exact_model_mappings"
                  :key="getCreateMessagesDispatchRowKey(row)"
                  class="group relative rounded-xl border border-line bg-surface p-4 shadow-sm transition-all hover:border-primary-300 hover:shadow-md dark:hover:border-primary-700"
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
                          class="input bg-gray-50 focus:bg-surface dark:focus:bg-dark-900"
                        />
                      </div>
                      <div
                        class="hidden md:flex md:justify-center md:pt-7 text-primary-300 dark:text-primary-700"
                      >
                        <Icon
                          name="arrowRight"
                          size="sm"
                          class="transition-transform group-hover:translate-x-1"
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
                          class="input bg-gray-50 focus:bg-surface dark:focus:bg-dark-900"
                        />
                      </div>
                    </div>
                    <button
                      type="button"
                      @click="removeCreateMessagesDispatchMapping(row)"
                      class="mt-6 flex h-9 w-9 items-center justify-center rounded-lg text-ink-tertiary transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                      :title="
                        t('admin.groups.openaiMessages.removeExactMapping')
                      "
                    >
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </div>

                <button
                  type="button"
                  @click="addCreateMessagesDispatchMapping"
                  class="flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-line bg-surface py-3 text-sm font-medium text-ink-secondary transition-all hover:border-primary-300 hover:bg-primary-50/50 hover:text-primary-600 dark:text-gray-400 dark:hover:border-primary-800 dark:hover:bg-primary-900/20 dark:hover:text-primary-400"
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
            createForm.platform,
          )
        "
        class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4 space-y-4"
      >
        <h4 class="text-sm font-medium text-ink-secondary mb-3">
          {{ t("admin.groups.accountFilters.title") }}
        </h4>

        <!-- require_oauth_only toggle -->
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm text-ink-secondary"
              >{{ t("admin.groups.accountFilters.oauthOnly") }}</label
            >
            <p class="text-xs text-ink-secondary mt-0.5">
              {{
                createForm.require_oauth_only
                  ? t("admin.groups.accountFilters.oauthOnlyEnabled")
                  : t("admin.groups.accountFilters.disabled")
              }}
            </p>
          </div>
          <button
            type="button"
            @click="
              createForm.require_oauth_only = !createForm.require_oauth_only
            "
            class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="
              createForm.require_oauth_only
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600'
            "
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="
                createForm.require_oauth_only
                  ? 'translate-x-6'
                  : 'translate-x-1'
              "
            />
          </button>
        </div>

        <!-- require_privacy_set toggle -->
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm text-ink-secondary"
              >{{ t("admin.groups.accountFilters.privacySetOnly") }}</label
            >
            <p class="text-xs text-ink-secondary mt-0.5">
              {{
                createForm.require_privacy_set
                  ? t("admin.groups.accountFilters.privacySetOnlyEnabled")
                  : t("admin.groups.accountFilters.disabled")
              }}
            </p>
          </div>
          <button
            type="button"
            @click="
              createForm.require_privacy_set = !createForm.require_privacy_set
            "
            class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="
              createForm.require_privacy_set
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600'
            "
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="
                createForm.require_privacy_set
                  ? 'translate-x-6'
                  : 'translate-x-1'
              "
            />
          </button>
        </div>
      </div>

      <!-- 无效请求兜底（仅 anthropic/antigravity 平台，且非订阅分组） -->
      <div
        v-if="
          ['anthropic', 'antigravity'].includes(createForm.platform) &&
          createForm.subscription_type !== 'subscription'
        "
        class="border-t pt-4"
      >
        <label class="input-label">{{
          t("admin.groups.invalidRequestFallback.title")
        }}</label>
        <Select
          v-model="createForm.fallback_group_id_on_invalid_request"
          :options="invalidRequestFallbackOptions"
          :placeholder="t('admin.groups.invalidRequestFallback.noFallback')"
        />
        <p class="input-hint">
          {{ t("admin.groups.invalidRequestFallback.hint") }}
        </p>
      </div>

      <!-- 模型路由配置（仅 anthropic 平台） -->
      <div v-if="createForm.platform === 'anthropic'" class="border-t pt-4">
        <div class="mb-1.5 flex items-center gap-1">
          <label class="text-sm font-medium text-ink-secondary">
            {{ t("admin.groups.modelRouting.title") }}
          </label>
          <!-- Help Tooltip -->
          <div class="group relative inline-flex">
            <Icon
              name="questionCircle"
              size="sm"
              :stroke-width="2"
              class="cursor-help text-ink-tertiary transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
            />
            <div
              class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-80 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
            >
              <div
                class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
              >
                <p class="text-xs leading-relaxed text-gray-300">
                  {{ t("admin.groups.modelRouting.tooltip") }}
                </p>
                <div
                  class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                ></div>
              </div>
            </div>
          </div>
        </div>
        <!-- 启用开关 -->
        <div class="flex items-center gap-3 mb-3">
          <button
            type="button"
            @click="
              createForm.model_routing_enabled =
                !createForm.model_routing_enabled
            "
            :class="[
              'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
              createForm.model_routing_enabled
                ? 'bg-primary-500'
                : 'bg-gray-300 dark:bg-dark-600',
            ]"
          >
            <span
              :class="[
                'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                createForm.model_routing_enabled
                  ? 'translate-x-6'
                  : 'translate-x-1',
              ]"
            />
          </button>
          <span class="text-sm text-ink-secondary">
            {{
              createForm.model_routing_enabled
                ? t("admin.groups.modelRouting.enabled")
                : t("admin.groups.modelRouting.disabled")
            }}
          </span>
        </div>
        <p
          v-if="!createForm.model_routing_enabled"
          class="text-xs text-ink-secondary mb-3"
        >
          {{ t("admin.groups.modelRouting.disabledHint") }}
        </p>
        <p v-else class="text-xs text-ink-secondary mb-3">
          {{ t("admin.groups.modelRouting.noRulesHint") }}
        </p>
        <!-- 路由规则列表（仅在启用时显示） -->
        <div v-if="createForm.model_routing_enabled" class="space-y-3">
          <div
            v-for="rule in createModelRoutingRules"
            :key="getCreateRuleRenderKey(rule)"
            class="rounded-lg border border-line p-3"
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
                      class="inline-flex items-center gap-1 rounded-sm bg-primary-100 px-2.5 py-1 text-xs font-medium text-accent dark:bg-primary-900/30"
                    >
                      {{ account.name }}
                      <button
                        type="button"
                        @click="removeSelectedAccount(rule, account.id)"
                        class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
                      >
                        <Icon name="x" size="xs" />
                      </button>
                    </span>
                  </div>
                  <!-- 账号搜索输入框 -->
                  <div class="relative account-search-container">
                    <input
                      v-model="
                        accountSearchKeyword[getCreateRuleSearchKey(rule)]
                      "
                      type="text"
                      class="input text-sm"
                      :placeholder="
                        t(
                          'admin.groups.modelRouting.searchAccountPlaceholder',
                        )
                      "
                      @input="searchAccountsByRule(rule)"
                      @focus="onAccountSearchFocus(rule)"
                    />
                    <!-- 搜索结果下拉框 -->
                    <div
                      v-if="
                        showAccountDropdown[getCreateRuleSearchKey(rule)] &&
                        accountSearchResults[getCreateRuleSearchKey(rule)]
                          ?.length > 0
                      "
                      class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-surface shadow-lg dark:border-dark-600"
                    >
                      <button
                        v-for="account in accountSearchResults[
                          getCreateRuleSearchKey(rule)
                        ]"
                        :key="account.id"
                        type="button"
                        @click="selectAccount(rule, account)"
                        class="w-full px-3 py-2 text-left text-sm hover:bg-surface-hover"
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
                        <span class="ml-2 text-xs text-ink-tertiary"
                          >#{{ account.id }}</span
                        >
                      </button>
                    </div>
                  </div>
                  <p class="text-xs text-ink-tertiary mt-1">
                    {{ t("admin.groups.modelRouting.accountsHint") }}
                  </p>
                </div>
              </div>
              <button
                type="button"
                @click="removeCreateRoutingRule(rule)"
                class="mt-5 p-1.5 text-ink-tertiary hover:text-red-500 transition-colors"
                :title="t('admin.groups.modelRouting.removeRule')"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>
        <!-- 添加规则按钮（仅在启用时显示） -->
        <button
          v-if="createForm.model_routing_enabled"
          type="button"
          @click="addCreateRoutingRule"
          class="mt-3 flex items-center gap-1.5 text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
        >
          <Icon name="plus" size="sm" />
          {{ t("admin.groups.modelRouting.addRule") }}
        </button>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3 pt-4">
        <button
          @click="closeCreateModal"
          type="button"
          class="btn btn-secondary"
        >
          {{ t("common.cancel") }}
        </button>
        <button
          type="submit"
          form="create-group-form"
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
          {{ submitting ? t("admin.groups.creating") : t("common.create") }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import BaseDialog from "@/components/common/BaseDialog.vue";
import Select from "@/components/common/Select.vue";
import Icon from "@/components/icons/Icon.vue";
import ReasoningEffortPolicyFields from "@/components/admin/group/ReasoningEffortPolicyFields.vue";
import PricingEntryCard from "@/components/admin/channel/PricingEntryCard.vue";
import { useGroupsViewContext } from "./context";
import {
  getImagePricePlaceholder,
  getVideoPricePlaceholder,
  imagePricingI18nKey,
  supportsImagePricingPlatform,
  supportsVideoPricingPlatform,
  videoPricingI18nKey,
} from "../groupsImagePricing";
import {
  grokVideoPriceResolutions,
  videoModelPriceFamilyRows,
} from "../groupsVideoModelPricing";
import {
  invertModelsListSelection,
  selectAllModelsListItems,
} from "../groupsModelsList";
import { isProfitControlPlatform } from "../groupsProfitControl";
import { supportsReasoningEffortPolicyPlatform } from "../groupsReasoningEffort";
import { addGroupPricing } from "./useGroupsView";

const ctx = useGroupsViewContext();

const {
  t,
  platformOptions,
  subscriptionTypeOptions,
  fallbackGroupOptions,
  invalidRequestFallbackOptions,
  copyAccountsGroupOptions,
  showCreateModal,
  submitting,
  createModelsListState,
  createModelsListLoading,
  createReasoningEffortPolicyRef,
  createModelsListSelectedCount,
  createForm,
  createModelRoutingRules,
  getCreateRuleRenderKey,
  getCreateMessagesDispatchRowKey,
  getCreateRuleSearchKey,
  accountSearchKeyword,
  accountSearchResults,
  showAccountDropdown,
  searchAccountsByRule,
  selectAccount,
  removeSelectedAccount,
  toggleCreateScope,
  onAccountSearchFocus,
  addCreateRoutingRule,
  removeCreateRoutingRule,
  moveCreateModelsListItem,
  createImageFinalPricePreview,
  createVideoFinalPricePreview,
  createWebSearchFinalPricePreview,
  toggleLive,
  closeCreateModal,
  handleCreateGroup,
  addCreateMessagesDispatchMapping,
  removeCreateMessagesDispatchMapping,
} = ctx;
</script>
