<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-6 pb-8">
      <header class="rounded-3xl bg-white p-5 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:p-6">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 class="flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
              <span class="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-orange-50 text-orange-600 dark:bg-orange-500/10 dark:text-orange-300">
                <Icon name="chart" size="sm" />
              </span>
              {{ t('admin.displayPricing.title') }}
            </h1>
            <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.displayPricing.description') }}
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            <RouterLink to="/model-prices" class="btn btn-secondary">
              <Icon name="eye" size="sm" />
              {{ t('admin.displayPricing.preview') }}
            </RouterLink>
          </div>
        </div>
        <div class="mt-4 flex items-start gap-2 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-200">
          <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0" />
          <strong>{{ t('admin.displayPricing.isolationNotice') }}</strong>
        </div>
        <div class="mt-4 inline-flex rounded-xl bg-gray-100 p-1 dark:bg-dark-900" role="tablist" :aria-label="t('admin.displayPricing.tabs.label')">
          <button
            type="button"
            role="tab"
            class="rounded-lg px-4 py-2 text-sm font-semibold transition"
            :class="activePanel === 'configuration'
              ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
              : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200'"
            :aria-selected="activePanel === 'configuration'"
            data-testid="display-pricing-tab"
            @click="activePanel = 'configuration'"
          >
            {{ t('admin.displayPricing.tabs.configuration') }}
          </button>
          <button
            type="button"
            role="tab"
            class="rounded-lg px-4 py-2 text-sm font-semibold transition"
            :class="activePanel === 'official'
              ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
              : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200'"
            :aria-selected="activePanel === 'official'"
            data-testid="official-pricing-tab"
            @click="activePanel = 'official'"
          >
            {{ t('admin.displayPricing.tabs.official') }}
          </button>
        </div>
      </header>

      <OfficialPricingView v-if="activePanel === 'official'" />

      <div v-else-if="loading" class="flex min-h-[320px] items-center justify-center">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600"></div>
      </div>

      <template v-else>
        <section class="grid gap-5 xl:grid-cols-[360px_minmax(0,1fr)]">
          <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex items-center gap-3">
              <span class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300">
                <Icon name="cog" size="sm" />
              </span>
              <div>
                <h2 class="font-bold text-gray-900 dark:text-white">{{ t('admin.displayPricing.global.title') }}</h2>
                <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.displayPricing.global.hint') }}</p>
              </div>
            </div>
            <label class="mt-5 block text-xs font-semibold text-gray-600 dark:text-dark-300">
              {{ t('admin.displayPricing.global.multiplier') }}
            </label>
            <div class="mt-2 flex gap-2">
              <div class="relative flex-1">
                <input v-model.number="globalMultiplier" type="number" min="0.01" step="0.01" class="input pr-9 font-mono" />
                <span class="absolute right-3 top-1/2 -translate-y-1/2 text-sm font-bold text-gray-400">×</span>
              </div>
              <button class="btn btn-primary" :disabled="savingGlobal" @click="saveGlobal">
                {{ savingGlobal ? t('common.saving') : t('common.save') }}
              </button>
            </div>
            <p class="mt-3 text-xs leading-5 text-gray-400 dark:text-dark-500">
              {{ t('admin.displayPricing.global.priority') }}
            </p>
          </div>

          <div class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex items-start justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700/70">
              <div>
                <h2 class="font-bold text-gray-900 dark:text-white">{{ t('admin.displayPricing.providers.title') }}</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.displayPricing.providers.hint') }}</p>
              </div>
              <button class="btn btn-primary shrink-0" @click="openProviderCreate">
                <Icon name="plus" size="sm" />
                {{ t('admin.displayPricing.providers.add') }}
              </button>
            </div>
            <div v-if="providerDrafts.length" class="divide-y divide-gray-100 dark:divide-dark-700/70">
              <div v-for="provider in providerDrafts" :key="provider.provider" class="flex flex-wrap items-center gap-3 px-5 py-4">
                <span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300">
                  <ProviderLogo
                    :provider="provider.provider"
                    :logo-key="provider.logo_key"
                    :logo-url="provider.logo_url"
                    :alt="provider.display_name"
                    size="lg"
                  />
                </span>
                <div class="min-w-[140px] flex-1">
                  <p class="font-semibold text-gray-900 dark:text-white">{{ provider.display_name }}</p>
                  <p class="mt-0.5 font-mono text-xs text-gray-400">{{ provider.provider }}</p>
                  <div v-if="provider.provider_note || provider.per_request_note || provider.image_note" class="mt-1 space-y-0.5 text-xs leading-5 text-gray-500 dark:text-dark-400">
                    <p v-if="provider.provider_note" class="line-clamp-1">
                      <span class="font-semibold">{{ t('modelPlaza.filters.token') }}：</span>{{ provider.provider_note }}
                    </p>
                    <p v-if="provider.per_request_note" class="line-clamp-1">
                      <span class="font-semibold">{{ t('modelPlaza.filters.perRequest') }}：</span>{{ provider.per_request_note }}
                    </p>
                    <p v-if="provider.image_note" class="line-clamp-1">
                      <span class="font-semibold">{{ t('modelPlaza.filters.image') }}：</span>{{ provider.image_note }}
                    </p>
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2 text-xs">
                  <span class="provider-chip">{{ provider.currency }}</span>
                  <span class="provider-chip">
                    {{ t('admin.displayPricing.providers.multiplierValue', { value: provider.multiplier ?? 1 }) }}
                  </span>
                  <span class="provider-chip">#{{ provider.sort_order }}</span>
                </div>
                <div class="ml-auto flex items-center gap-2">
                  <button class="btn btn-secondary" @click="openProviderEdit(provider)">{{ t('common.edit') }}</button>
                  <button class="btn btn-danger" @click="requestProviderDelete(provider)">{{ t('common.delete') }}</button>
                </div>
              </div>
            </div>
            <p v-else class="px-5 py-10 text-center text-sm text-gray-400">{{ t('admin.displayPricing.providers.empty') }}</p>
          </div>
        </section>

        <section class="scroll-mt-6 overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700/70 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="font-bold text-gray-900 dark:text-white">{{ t('admin.displayPricing.models.title') }}</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.displayPricing.models.hint') }}</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <button class="btn btn-secondary" @click="showDiscovered = true">
                <Icon name="search" size="sm" />
                {{ t('admin.displayPricing.models.discover') }}
              </button>
              <button class="btn btn-primary" @click="openCreate()">
                <Icon name="plus" size="sm" />
                {{ t('admin.displayPricing.models.add') }}
              </button>
            </div>
          </div>

          <div class="grid gap-3 border-b border-gray-100 bg-gray-50/60 px-5 py-3 dark:border-dark-700/70 dark:bg-dark-900/30 sm:grid-cols-[minmax(220px,1fr)_180px_160px]">
            <input v-model="search" :placeholder="t('admin.displayPricing.models.search')" class="input" />
            <select v-model="providerFilter" class="input">
              <option value="">{{ t('admin.displayPricing.models.allProviders') }}</option>
              <option v-for="provider in providerOptions" :key="provider" :value="provider">{{ providerLabel(provider) }}</option>
            </select>
            <select v-model="billingFilter" class="input">
              <option value="">{{ t('admin.displayPricing.models.allModes') }}</option>
              <option value="token">{{ t('modelPlaza.filters.token') }}</option>
              <option value="per_request">{{ t('modelPlaza.filters.perRequest') }}</option>
              <option value="image">{{ t('modelPlaza.filters.image') }}</option>
            </select>
          </div>

          <div class="overflow-x-auto">
            <table class="w-full min-w-[920px] text-left">
              <thead class="border-b border-gray-100 bg-gray-50/70 text-xs font-semibold text-gray-500 dark:border-dark-700 dark:bg-dark-900/40 dark:text-dark-400">
                <tr>
                  <th class="px-5 py-3">{{ t('admin.displayPricing.models.model') }}</th>
                  <th class="px-4 py-3">{{ t('admin.displayPricing.models.provider') }}</th>
                  <th class="px-4 py-3">{{ t('admin.displayPricing.models.mode') }}</th>
                  <th class="px-4 py-3">{{ t('admin.displayPricing.currency') }}</th>
                  <th class="px-4 py-3">{{ t('admin.displayPricing.models.rule') }}</th>
                  <th class="px-4 py-3">{{ t('common.status') }}</th>
                  <th class="px-5 py-3 text-right">{{ t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700/70">
                <tr v-for="model in filteredModels" :key="model.id" class="hover:bg-gray-50/70 dark:hover:bg-dark-700/30">
                  <td class="px-5 py-3.5">
                    <p class="font-mono text-sm font-semibold text-gray-800 dark:text-gray-100">{{ model.model_name }}</p>
                    <p v-if="model.model_note" class="mt-1 line-clamp-2 text-xs font-medium leading-4 text-orange-600 dark:text-orange-300">
                      {{ model.model_note }}
                    </p>
                  </td>
                  <td class="px-4 py-3.5">
                    <span class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                      <PlatformIcon :platform="model.provider" size="sm" />
                      {{ providerLabel(model.provider) }}
                    </span>
                  </td>
                  <td class="px-4 py-3.5"><span class="mode-badge">{{ modeLabel(model.billing_mode) }}</span></td>
                  <td class="px-4 py-3.5 font-mono text-sm">{{ model.currency }}</td>
                  <td class="max-w-[300px] px-4 py-3.5 text-xs text-gray-500 dark:text-dark-400">{{ pricingSummary(model) }}</td>
                  <td class="px-4 py-3.5">
                    <span :class="model.enabled ? 'status-enabled' : 'status-disabled'" class="rounded-full px-2.5 py-1 text-xs font-semibold">
                      {{ model.enabled ? t('common.enabled') : t('common.disabled') }}
                    </span>
                  </td>
                  <td class="px-5 py-3.5 text-right">
                    <button class="mr-3 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-300" @click="openEdit(model)">{{ t('common.edit') }}</button>
                    <button class="text-sm font-medium text-red-500 hover:text-red-600" @click="requestDelete(model)">{{ t('common.delete') }}</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-if="filteredModels.length === 0" class="px-5 py-12 text-center text-sm text-gray-400">{{ t('admin.displayPricing.models.empty') }}</p>
        </section>
      </template>
    </div>

    <BaseDialog
      :show="showProviderEditor"
      :title="editingProviderKey ? t('admin.displayPricing.providers.editTitle') : t('admin.displayPricing.providers.createTitle')"
      @close="closeProviderEditor"
    >
      <form class="space-y-5" @submit.prevent="saveProvider">
        <div class="flex items-center gap-4 rounded-2xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-900/30">
          <span class="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-white text-gray-700 shadow-sm dark:bg-dark-800 dark:text-dark-200">
            <ProviderLogo
              :provider="providerForm.provider || 'composite'"
              :logo-key="providerForm.logo_key"
              :logo-url="providerForm.logo_url"
              :alt="providerForm.display_name"
              size="xl"
            />
          </span>
          <div>
            <p class="font-semibold text-gray-900 dark:text-white">{{ t('admin.displayPricing.providers.logoPreview') }}</p>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('admin.displayPricing.providers.logoHint') }}</p>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.providers.key') }}</span>
            <input
              v-model.trim="providerForm.provider"
              required
              :disabled="Boolean(editingProviderKey)"
              class="input mt-1.5 font-mono disabled:cursor-not-allowed disabled:bg-gray-100 dark:disabled:bg-dark-700"
              placeholder="openai"
            />
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.providers.name') }}</span>
            <input v-model.trim="providerForm.display_name" required class="input mt-1.5" placeholder="OpenAI" />
          </label>
          <label class="form-field sm:col-span-2">
            <span class="field-label">{{ t('admin.displayPricing.providers.tokenNote') }}</span>
            <textarea
              v-model="providerForm.provider_note"
              :maxlength="4000"
              rows="3"
              class="input mt-1.5 min-h-[88px] resize-y"
              :placeholder="t('admin.displayPricing.providers.tokenNotePlaceholder')"
            ></textarea>
            <span class="mt-1 block text-right text-[11px] text-gray-400">{{ providerForm.provider_note.length }}/4000</span>
          </label>
          <label class="form-field sm:col-span-2">
            <span class="field-label">{{ t('admin.displayPricing.providers.perRequestNote') }}</span>
            <textarea
              v-model="providerForm.per_request_note"
              :maxlength="4000"
              rows="3"
              class="input mt-1.5 min-h-[88px] resize-y"
              :placeholder="t('admin.displayPricing.providers.perRequestNotePlaceholder')"
            ></textarea>
            <span class="mt-1 block text-right text-[11px] text-gray-400">{{ providerForm.per_request_note.length }}/4000</span>
          </label>
          <label class="form-field sm:col-span-2">
            <span class="field-label">{{ t('admin.displayPricing.providers.imageNote') }}</span>
            <textarea
              v-model="providerForm.image_note"
              :maxlength="4000"
              rows="3"
              class="input mt-1.5 min-h-[88px] resize-y"
              :placeholder="t('admin.displayPricing.providers.imageNotePlaceholder')"
            ></textarea>
            <span class="mt-1 block text-right text-[11px] text-gray-400">{{ providerForm.image_note.length }}/4000</span>
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.providers.logoKey') }}</span>
            <select v-model="providerForm.logo_key" class="input mt-1.5">
              <option value="">{{ t('admin.displayPricing.providers.logoAuto') }}</option>
              <option v-for="option in builtInLogoOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.providers.logoUrl') }}</span>
            <input v-model.trim="providerForm.logo_url" class="input mt-1.5" placeholder="https://cdn.example.com/logo.svg" />
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.currency') }}</span>
            <select v-model="providerForm.currency" class="input mt-1.5">
              <option value="CNY">CNY · ¥</option>
              <option value="USD">USD · $</option>
            </select>
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.providers.multiplier') }}</span>
            <input v-model.number="providerForm.multiplier" type="number" min="0.01" step="0.01" class="input mt-1.5 font-mono" placeholder="1" />
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.sortOrder') }}</span>
            <input v-model.number="providerForm.sort_order" type="number" step="1" class="input mt-1.5 font-mono" />
          </label>
        </div>

        <div class="flex justify-end gap-2 border-t border-gray-100 pt-4 dark:border-dark-700">
          <button type="button" class="btn btn-secondary" @click="closeProviderEditor">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn btn-primary" :disabled="savingProvider">
            {{ savingProvider ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </form>
    </BaseDialog>

    <BaseDialog :show="showEditor" :title="editingId ? t('admin.displayPricing.editor.editTitle') : t('admin.displayPricing.editor.createTitle')" width="wide" @close="closeEditor">
      <form class="space-y-5" @submit.prevent="saveModel">
        <div class="rounded-xl border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-xs font-semibold text-amber-800 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-200">
          {{ t('admin.displayPricing.isolationNotice') }}
        </div>

        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <label class="form-field sm:col-span-2">
            <span class="field-label">{{ t('admin.displayPricing.models.model') }}</span>
            <input v-model="form.model_name" required class="input mt-1.5 font-mono" />
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.models.provider') }}</span>
            <input v-model="form.provider" required class="input mt-1.5" />
          </label>
          <label class="form-field">
            <span class="field-label">Platform</span>
            <input v-model="form.platform" required class="input mt-1.5" />
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.models.mode') }}</span>
            <select v-model="form.billing_mode" class="input mt-1.5">
              <option value="token">{{ t('modelPlaza.filters.token') }}</option>
              <option value="per_request">{{ t('modelPlaza.filters.perRequest') }}</option>
              <option value="image">{{ t('modelPlaza.filters.image') }}</option>
            </select>
          </label>
          <label class="form-field">
            <span class="field-label">{{ t('admin.displayPricing.currency') }}</span>
            <select v-model="form.currency" class="input mt-1.5">
              <option value="CNY">CNY · ¥</option>
              <option value="USD">USD · $</option>
            </select>
          </label>
          <label class="form-field sm:col-span-2 lg:col-span-3">
            <span class="field-label">{{ t('admin.displayPricing.models.note') }}</span>
            <textarea
              v-model="form.model_note"
              :maxlength="1000"
              rows="2"
              class="input mt-1.5 min-h-[72px] resize-y"
              :placeholder="t('admin.displayPricing.models.notePlaceholder')"
            ></textarea>
            <span class="mt-1 block text-right text-[11px] text-gray-400">{{ form.model_note.length }}/1000</span>
          </label>
        </div>

        <section v-if="form.billing_mode === 'token'" class="form-section">
          <div>
            <h3 class="form-section-title">{{ t('admin.displayPricing.editor.officialPrices') }}</h3>
            <p class="form-section-hint">{{ t('admin.displayPricing.editor.tokenFormula') }}</p>
          </div>
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <PriceInput v-model="form.official_input_per_million" :label="t('modelPlaza.table.input')" :currency="form.currency" />
            <PriceInput v-model="form.official_output_per_million" :label="t('modelPlaza.table.output')" :currency="form.currency" />
            <PriceInput v-model="form.official_cache_write_per_million" :label="t('modelPlaza.table.cacheWrite')" :currency="form.currency" />
            <PriceInput v-model="form.official_cache_read_per_million" :label="t('modelPlaza.table.cacheRead')" :currency="form.currency" />
          </div>
          <label class="block max-w-xs">
            <span class="field-label">{{ t('admin.displayPricing.editor.modelMultiplier') }}</span>
            <input v-model.number="form.model_multiplier" type="number" min="0.01" step="0.01" :placeholder="t('admin.displayPricing.editor.inheritMultiplier')" class="input mt-1.5 font-mono" />
          </label>
        </section>

        <section v-else-if="form.billing_mode === 'per_request'" class="form-section">
          <div>
            <h3 class="form-section-title">{{ t('admin.displayPricing.editor.perRequestPrices') }}</h3>
            <p class="form-section-hint">{{ t('admin.displayPricing.editor.perRequestFormula') }}</p>
          </div>
          <div class="grid gap-3 sm:grid-cols-3">
            <PriceInput v-model="form.per_request_lte_256k" label="≤ 256K" :currency="form.currency" />
            <PriceInput v-model="form.per_request_256k_512k_override" :label="t('admin.displayPricing.editor.tier2Override')" :currency="form.currency" :placeholder="derivedTier(1.5)" />
            <PriceInput v-model="form.per_request_gt_512k_override" :label="t('admin.displayPricing.editor.tier3Override')" :currency="form.currency" :placeholder="derivedTier(2)" />
          </div>
        </section>

        <section v-else class="form-section">
          <div class="flex items-start justify-between gap-3">
            <div>
              <h3 class="form-section-title">{{ t('admin.displayPricing.editor.imagePrices') }}</h3>
              <p class="form-section-hint">{{ t('admin.displayPricing.editor.imageHint') }}</p>
            </div>
            <button type="button" class="btn btn-secondary" @click="addImageTier">{{ t('admin.displayPricing.editor.addTier') }}</button>
          </div>
          <div v-if="form.image_prices.length" class="space-y-2">
            <div v-for="(tier, index) in form.image_prices" :key="index" class="grid items-end gap-2 sm:grid-cols-[1fr_180px_auto]">
              <label>
                <span class="field-label">{{ t('admin.displayPricing.editor.specLabel') }}</span>
                <input v-model="tier.label" required class="input mt-1.5" placeholder="1024×1024" />
              </label>
              <PriceInput v-model="tier.price" :label="t('admin.displayPricing.editor.pricePerImage')" :currency="form.currency" />
              <button type="button" class="btn btn-danger" @click="form.image_prices.splice(index, 1)">{{ t('common.remove') }}</button>
            </div>
          </div>
        </section>

        <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
          <div class="flex items-center gap-4">
            <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
              <Toggle v-model="form.enabled" />
              {{ t('common.enabled') }}
            </label>
            <label class="inline-flex items-center gap-2 text-sm">
              <span class="field-label">{{ t('admin.displayPricing.sortOrder') }}</span>
              <input v-model.number="form.sort_order" type="number" class="input w-24 font-mono" />
            </label>
          </div>
          <div class="flex gap-2">
            <button type="button" class="btn btn-secondary" @click="closeEditor">{{ t('common.cancel') }}</button>
            <button type="submit" class="btn btn-primary" :disabled="savingModel">{{ savingModel ? t('common.saving') : t('common.save') }}</button>
          </div>
        </div>
      </form>
    </BaseDialog>

    <BaseDialog :show="showDiscovered" :title="t('admin.displayPricing.discovered.title')" width="wide" @close="showDiscovered = false">
      <div class="mb-4">
        <input v-model="discoveredSearch" :placeholder="t('admin.displayPricing.discovered.search')" class="input" />
      </div>
      <div class="max-h-[55vh] divide-y divide-gray-100 overflow-y-auto rounded-xl border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
        <div v-for="model in filteredDiscovered" :key="`${model.platform}:${model.model_name}:${model.billing_mode}`" class="flex items-center justify-between gap-3 px-4 py-3">
          <div class="min-w-0">
            <p class="truncate font-mono text-sm font-semibold text-gray-800 dark:text-gray-100">{{ model.model_name }}</p>
            <p class="mt-0.5 text-xs text-gray-400">{{ providerLabel(model.provider) }} · {{ modeLabel(model.billing_mode) }}</p>
          </div>
          <span v-if="model.configured" class="text-xs font-semibold text-emerald-600">{{ t('admin.displayPricing.discovered.configured') }}</span>
          <button v-else class="btn btn-secondary" @click="configureDiscovered(model)">{{ t('admin.displayPricing.discovered.configure') }}</button>
        </div>
      </div>
    </BaseDialog>

    <ConfirmDialog
      :show="showProviderDeleteConfirm"
      :title="t('admin.displayPricing.providers.deleteTitle')"
      :message="t('admin.displayPricing.providers.deleteMessage', { provider: deletingProvider?.display_name ?? '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmProviderDelete"
      @cancel="cancelProviderDelete"
    />

    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.displayPricing.delete.title')"
      :message="t('admin.displayPricing.delete.message', { model: deletingModel?.model_name ?? '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteConfirm = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  DiscoveredDisplayModel,
  DisplayPricingModel,
  DisplayPricingModelInput,
  DisplayPricingProvider,
  DisplayPricingProviderCreateInput
} from '@/api/admin/displayPricing'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import ProviderLogo from '@/components/modelPlaza/ProviderLogo.vue'
import PriceInput from '@/components/admin/displayPricing/PriceInput.vue'
import OfficialPricingView from '@/views/admin/OfficialPricingView.vue'
import { platformLabel } from '@/utils/platformColors'
import { notifyDisplayPricingUpdated } from '@/utils/displayPricingSync'

const { t } = useI18n()
const appStore = useAppStore()
const activePanel = ref<'configuration' | 'official'>('configuration')
const loading = ref(true)
const globalMultiplier = ref(1)
const savingGlobal = ref(false)
const savingProvider = ref(false)
const savingModel = ref(false)
const providerDrafts = ref<DisplayPricingProvider[]>([])
const models = ref<DisplayPricingModel[]>([])
const discoveredModels = ref<DiscoveredDisplayModel[]>([])
const search = ref('')
const providerFilter = ref('')
const billingFilter = ref('')
const discoveredSearch = ref('')
const showEditor = ref(false)
const showDiscovered = ref(false)
const showDeleteConfirm = ref(false)
const editingId = ref<number | null>(null)
const deletingModel = ref<DisplayPricingModel | null>(null)
const showProviderEditor = ref(false)
const showProviderDeleteConfirm = ref(false)
const editingProviderKey = ref('')
const deletingProvider = ref<DisplayPricingProvider | null>(null)
function emptyProviderForm(): DisplayPricingProviderCreateInput {
  return {
    provider: '',
    display_name: '',
    provider_note: '',
    per_request_note: '',
    image_note: '',
    currency: 'USD',
    multiplier: null,
    sort_order: 0,
    logo_key: '',
    logo_url: ''
  }
}

const providerForm = reactive<DisplayPricingProviderCreateInput>(emptyProviderForm())
const builtInLogoOptions = [
  'openai',
  'anthropic',
  'gemini',
  'grok',
  'deepseek',
  'kimi',
  'qwen',
  'minimax',
  'zhipu',
  'mimo',
  'hunyuan',
  'auto',
  'composite'
].map((value) => ({ value, label: platformLabel(value) }))

function emptyForm(): DisplayPricingModelInput {
  return {
    platform: 'openai',
    model_name: '',
    model_note: '',
    provider: 'openai',
    billing_mode: 'token',
    currency: 'USD',
    enabled: true,
    sort_order: 0,
    official_input_per_million: null,
    official_output_per_million: null,
    official_cache_write_per_million: null,
    official_cache_read_per_million: null,
    model_multiplier: null,
    per_request_lte_256k: null,
    per_request_256k_512k_override: null,
    per_request_gt_512k_override: null,
    image_prices: []
  }
}

const form = reactive<DisplayPricingModelInput>(emptyForm())
const providerOptions = computed(() => [...new Set([...providerDrafts.value.map((item) => item.provider), ...models.value.map((item) => item.provider)])].sort())
const filteredModels = computed(() => {
  const query = search.value.trim().toLowerCase()
  return models.value.filter((model) => {
    if (providerFilter.value && model.provider !== providerFilter.value) return false
    if (billingFilter.value && model.billing_mode !== billingFilter.value) return false
    return !query || model.model_name.toLowerCase().includes(query) || model.provider.toLowerCase().includes(query)
  })
})
const filteredDiscovered = computed(() => {
  const query = discoveredSearch.value.trim().toLowerCase()
  return discoveredModels.value.filter((model) => !query || model.model_name.toLowerCase().includes(query) || model.provider.toLowerCase().includes(query))
})

async function loadData(): Promise<void> {
  loading.value = true
  try {
    const [settings, providers, configuredModels, discovered] = await Promise.all([
      adminAPI.displayPricing.getSettings(),
      adminAPI.displayPricing.listProviders(),
      adminAPI.displayPricing.listModels(),
      adminAPI.displayPricing.listDiscoveredModels()
    ])
    globalMultiplier.value = settings.global_multiplier
    providerDrafts.value = sortProviders(providers.map((provider) => ({
      ...provider,
      logo_key: provider.logo_key || provider.provider,
      logo_url: provider.logo_url || '',
      provider_note: provider.provider_note || '',
      per_request_note: provider.per_request_note || '',
      image_note: provider.image_note || ''
    })))
    models.value = configuredModels
    discoveredModels.value = discovered
  } catch (error) {
    appStore.showError(t('admin.displayPricing.loadFailed'))
    console.error(error)
  } finally {
    loading.value = false
  }
}

async function saveGlobal(): Promise<void> {
  savingGlobal.value = true
  try {
    const result = await adminAPI.displayPricing.updateSettings(Number(globalMultiplier.value))
    globalMultiplier.value = result.global_multiplier
    notifyDisplayPricingUpdated()
    appStore.showSuccess(t('admin.displayPricing.saved'))
  } catch {
    appStore.showError(t('admin.displayPricing.saveFailed'))
  } finally {
    savingGlobal.value = false
  }
}

function openProviderCreate(): void {
  editingProviderKey.value = ''
  Object.assign(providerForm, emptyProviderForm())
  showProviderEditor.value = true
}

function openProviderEdit(provider: DisplayPricingProvider): void {
  editingProviderKey.value = provider.provider
  Object.assign(providerForm, {
    provider: provider.provider,
    display_name: provider.display_name,
    provider_note: provider.provider_note || '',
    per_request_note: provider.per_request_note || '',
    image_note: provider.image_note || '',
    currency: provider.currency,
    multiplier: provider.multiplier,
    sort_order: provider.sort_order,
    logo_key: provider.logo_key || provider.provider,
    logo_url: provider.logo_url || ''
  })
  showProviderEditor.value = true
}

function closeProviderEditor(): void {
  showProviderEditor.value = false
  editingProviderKey.value = ''
}

async function saveProvider(): Promise<void> {
  savingProvider.value = true
  try {
    const payload = {
      display_name: providerForm.display_name.trim(),
      provider_note: providerForm.provider_note.trim(),
      per_request_note: providerForm.per_request_note.trim(),
      image_note: providerForm.image_note.trim(),
      currency: providerForm.currency,
      multiplier: nullableNumber(providerForm.multiplier),
      sort_order: Number(providerForm.sort_order) || 0,
      logo_key: providerForm.logo_key.trim(),
      logo_url: providerForm.logo_url.trim()
    }
    const saved = editingProviderKey.value
      ? await adminAPI.displayPricing.updateProvider(editingProviderKey.value, payload)
      : await adminAPI.displayPricing.createProvider({
          provider: providerForm.provider.trim(),
          ...payload
        })
    const index = providerDrafts.value.findIndex((item) => item.provider === saved.provider)
    if (index >= 0) providerDrafts.value.splice(index, 1, saved)
    else providerDrafts.value.push(saved)
    providerDrafts.value = sortProviders(providerDrafts.value)
    closeProviderEditor()
    notifyDisplayPricingUpdated()
    appStore.showSuccess(t('admin.displayPricing.saved'))
  } catch {
    appStore.showError(t('admin.displayPricing.saveFailed'))
  } finally {
    savingProvider.value = false
  }
}

function requestProviderDelete(provider: DisplayPricingProvider): void {
  deletingProvider.value = provider
  showProviderDeleteConfirm.value = true
}

function cancelProviderDelete(): void {
  deletingProvider.value = null
  showProviderDeleteConfirm.value = false
}

async function confirmProviderDelete(): Promise<void> {
  const provider = deletingProvider.value
  if (!provider) return
  try {
    await adminAPI.displayPricing.deleteProvider(provider.provider)
    providerDrafts.value = providerDrafts.value.filter((item) => item.provider !== provider.provider)
    models.value = models.value.filter((item) => item.provider !== provider.provider)
    for (const discovered of discoveredModels.value) {
      if (discovered.provider === provider.provider) discovered.configured = false
    }
    notifyDisplayPricingUpdated()
    appStore.showSuccess(t('admin.displayPricing.providers.deleted'))
  } catch {
    appStore.showError(t('admin.displayPricing.providers.deleteFailed'))
  } finally {
    cancelProviderDelete()
  }
}

function openCreate(seed?: Partial<DisplayPricingModelInput>): void {
  editingId.value = null
  Object.assign(form, emptyForm(), seed)
  form.image_prices = (seed?.image_prices ?? []).map((tier) => ({ ...tier }))
  showEditor.value = true
}

function openEdit(model: DisplayPricingModel): void {
  editingId.value = model.id
  Object.assign(form, model)
  form.image_prices = (model.image_prices ?? []).map((tier) => ({ ...tier }))
  showEditor.value = true
}

function closeEditor(): void {
  showEditor.value = false
  editingId.value = null
}

function normalizedPayload(): DisplayPricingModelInput {
  return {
    ...form,
    model_note: form.model_note.trim(),
    sort_order: Number(form.sort_order) || 0,
    official_input_per_million: nullableNumber(form.official_input_per_million),
    official_output_per_million: nullableNumber(form.official_output_per_million),
    official_cache_write_per_million: nullableNumber(form.official_cache_write_per_million),
    official_cache_read_per_million: nullableNumber(form.official_cache_read_per_million),
    model_multiplier: nullableNumber(form.model_multiplier),
    per_request_lte_256k: nullableNumber(form.per_request_lte_256k),
    per_request_256k_512k_override: nullableNumber(form.per_request_256k_512k_override),
    per_request_gt_512k_override: nullableNumber(form.per_request_gt_512k_override),
    image_prices: form.image_prices.map((tier) => ({ label: tier.label.trim(), price: Number(tier.price) }))
  }
}

async function saveModel(): Promise<void> {
  savingModel.value = true
  try {
    const payload = normalizedPayload()
    const saved = editingId.value
      ? await adminAPI.displayPricing.updateModel(editingId.value, payload)
      : await adminAPI.displayPricing.createModel(payload)
    const index = models.value.findIndex((item) => item.id === saved.id)
    if (index >= 0) models.value.splice(index, 1, saved)
    else models.value.push(saved)
    const discovered = discoveredModels.value.find((item) => item.platform === saved.platform && item.model_name === saved.model_name && item.billing_mode === saved.billing_mode)
    if (discovered) discovered.configured = true
    closeEditor()
    notifyDisplayPricingUpdated()
    appStore.showSuccess(t('admin.displayPricing.saved'))
  } catch {
    appStore.showError(t('admin.displayPricing.saveFailed'))
  } finally {
    savingModel.value = false
  }
}

function requestDelete(model: DisplayPricingModel): void {
  deletingModel.value = model
  showDeleteConfirm.value = true
}

async function confirmDelete(): Promise<void> {
  if (!deletingModel.value) return
  try {
    await adminAPI.displayPricing.deleteModel(deletingModel.value.id)
    models.value = models.value.filter((item) => item.id !== deletingModel.value?.id)
    notifyDisplayPricingUpdated()
    appStore.showSuccess(t('common.deleted'))
  } catch {
    appStore.showError(t('admin.displayPricing.delete.failed'))
  } finally {
    deletingModel.value = null
    showDeleteConfirm.value = false
  }
}

function configureDiscovered(model: DiscoveredDisplayModel): void {
  showDiscovered.value = false
  openCreate({
    platform: model.platform,
    model_name: model.model_name,
    provider: model.provider,
    billing_mode: model.billing_mode,
    currency: providerDrafts.value.find((item) => item.provider === model.provider)?.currency ?? 'USD'
  })
}

function addImageTier(): void {
  form.image_prices.push({ label: '', price: 0 })
}

function nullableNumber(value: unknown): number | null {
  if (value === '' || value == null) return null
  const number = Number(value)
  return Number.isFinite(number) ? number : null
}

function sortProviders(providers: DisplayPricingProvider[]): DisplayPricingProvider[] {
  return [...providers].sort(
    (left, right) => left.sort_order - right.sort_order || left.display_name.localeCompare(right.display_name)
  )
}

function derivedTier(multiplier: number): string {
  const base = nullableNumber(form.per_request_lte_256k)
  return base == null ? t('admin.displayPricing.editor.autoDerived') : String(Math.round(base * multiplier * 1e8) / 1e8)
}

function providerLabel(provider: string): string {
  return providerDrafts.value.find((item) => item.provider === provider)?.display_name || platformLabel(provider)
}

function modeLabel(mode: string): string {
  if (mode === 'token') return t('modelPlaza.filters.token')
  if (mode === 'per_request') return t('modelPlaza.filters.perRequest')
  if (mode === 'image') return t('modelPlaza.filters.image')
  return mode
}

function pricingSummary(model: DisplayPricingModel): string {
  if (model.billing_mode === 'token') {
    return model.model_multiplier == null
      ? t('admin.displayPricing.models.tokenInheritedSummary')
      : t('admin.displayPricing.models.tokenFixedSummary', { multiplier: model.model_multiplier })
  }
  if (model.billing_mode === 'per_request') {
    return t('admin.displayPricing.models.perRequestSummary', { price: model.per_request_lte_256k ?? '-' })
  }
  return t('admin.displayPricing.models.imageSummary', { count: model.image_prices?.length ?? 0 })
}

onMounted(() => void loadData())
</script>

<style scoped>
.field-label {
  @apply text-xs font-semibold text-gray-600 dark:text-dark-300;
}
.form-section {
  @apply space-y-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-4 dark:border-dark-700 dark:bg-dark-900/30;
}
.form-section-title {
  @apply font-bold text-gray-900 dark:text-white;
}
.form-section-hint {
  @apply mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400;
}
.mode-badge {
  @apply inline-flex rounded-lg bg-blue-50 px-2.5 py-1 text-xs font-semibold text-blue-700 dark:bg-blue-500/10 dark:text-blue-300;
}
.status-enabled {
  @apply bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300;
}
.status-disabled {
  @apply bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-400;
}
.provider-chip {
  @apply rounded-lg bg-gray-100 px-2.5 py-1 font-mono font-semibold text-gray-600 dark:bg-dark-700 dark:text-dark-300;
}
</style>
