<template>
  <div v-show="activeTab === 'general'" class="space-y-6">
    <!-- Site Settings -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.site.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.site.description") }}
        </p>
      </div>
      <div class="space-y-6 p-6">
        <!-- Backend Mode -->
        <div
          class="flex items-center justify-between rounded-sm border border border-warn/40 bg-warn-tint p-4"
        >
          <div>
            <h3 class="text-sm font-medium text-ink">
              {{ t("admin.settings.site.backendMode") }}
            </h3>
            <p class="mt-1 text-xs text-ink-secondary">
              {{ t("admin.settings.site.backendModeDescription") }}
            </p>
          </div>
          <Toggle v-model="form.backend_mode_enabled" />
        </div>

        <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
          <div>
            <label
              class="mb-2 block text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.site.siteName") }}
            </label>
            <input
              v-model="form.site_name"
              type="text"
              class="input"
              :placeholder="t('admin.settings.site.siteNamePlaceholder')"
            />
            <p class="mt-1.5 text-xs text-ink-secondary">
              {{ t("admin.settings.site.siteNameHint") }}
            </p>
          </div>
          <div>
            <label
              class="mb-2 block text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.site.siteSubtitle") }}
            </label>
            <input
              v-model="form.site_subtitle"
              type="text"
              class="input"
              :placeholder="
                t('admin.settings.site.siteSubtitlePlaceholder')
              "
            />
            <p class="mt-1.5 text-xs text-ink-secondary">
              {{ t("admin.settings.site.siteSubtitleHint") }}
            </p>
          </div>
        </div>

        <!-- API Base URL -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{ t("admin.settings.site.apiBaseUrl") }}
          </label>
          <input
            v-model="form.api_base_url"
            type="text"
            class="input font-mono text-sm"
            :placeholder="t('admin.settings.site.apiBaseUrlPlaceholder')"
          />
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{ t("admin.settings.site.apiBaseUrlHint") }}
          </p>
        </div>

        <!-- Global Table Preferences -->
        <div class="border-t border-line-subtle pt-4">
          <h3 class="text-sm font-medium text-ink">
            {{ t("admin.settings.site.tablePreferencesTitle") }}
          </h3>
          <p class="mt-1 text-xs text-ink-secondary">
            {{ t("admin.settings.site.tablePreferencesDescription") }}
          </p>
          <div class="mt-4 grid grid-cols-1 gap-6 md:grid-cols-2">
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.site.tableDefaultPageSize") }}
              </label>
              <input
                v-model.number="form.table_default_page_size"
                type="number"
                min="5"
                max="1000"
                step="1"
                class="input w-40"
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.site.tableDefaultPageSizeHint") }}
              </p>
            </div>
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.site.tablePageSizeOptions") }}
              </label>
              <input
                v-model="tablePageSizeOptionsInput"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.site.tablePageSizeOptionsPlaceholder')
                "
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.site.tablePageSizeOptionsHint") }}
              </p>
            </div>
          </div>
        </div>

        <!-- Custom Endpoints -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{ t("admin.settings.site.customEndpoints.title") }}
          </label>
          <p class="mb-3 text-xs text-ink-secondary">
            {{ t("admin.settings.site.customEndpoints.description") }}
          </p>

          <div class="space-y-3">
            <div
              v-for="(ep, index) in form.custom_endpoints"
              :key="index"
              class="rounded-sm border border-line p-4"
            >
              <div class="mb-3 flex items-center justify-between">
                <span
                  class="text-sm font-medium text-ink-secondary"
                >
                  {{
                    t("admin.settings.site.customEndpoints.itemLabel", {
                      n: index + 1,
                    })
                  }}
                </span>
                <button
                  type="button"
                  class="rounded p-1 text-danger hover:bg-danger-tint hover:text-danger"
                  @click="removeEndpoint(index)"
                >
                  <svg
                    class="h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    />
                  </svg>
                </button>
              </div>
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label
                    class="mb-1 block text-xs font-medium text-ink-secondary"
                  >
                    {{ t("admin.settings.site.customEndpoints.name") }}
                  </label>
                  <input
                    v-model="ep.name"
                    type="text"
                    class="input text-sm"
                    :placeholder="
                      t(
                        'admin.settings.site.customEndpoints.namePlaceholder',
                      )
                    "
                  />
                </div>
                <div>
                  <label
                    class="mb-1 block text-xs font-medium text-ink-secondary"
                  >
                    {{
                      t("admin.settings.site.customEndpoints.endpointUrl")
                    }}
                  </label>
                  <input
                    v-model="ep.endpoint"
                    type="url"
                    class="input font-mono text-sm"
                    :placeholder="
                      t(
                        'admin.settings.site.customEndpoints.endpointUrlPlaceholder',
                      )
                    "
                  />
                </div>
                <div class="sm:col-span-2">
                  <label
                    class="mb-1 block text-xs font-medium text-ink-secondary"
                  >
                    {{
                      t(
                        "admin.settings.site.customEndpoints.descriptionLabel",
                      )
                    }}
                  </label>
                  <input
                    v-model="ep.description"
                    type="text"
                    class="input text-sm"
                    :placeholder="
                      t(
                        'admin.settings.site.customEndpoints.descriptionPlaceholder',
                      )
                    "
                  />
                </div>
              </div>
            </div>
          </div>

          <button
            type="button"
            class="mt-3 flex w-full items-center justify-center gap-2 rounded-sm border-2 border-dashed border-line px-4 py-2.5 text-sm text-ink-secondary transition-colors hover:border-accent-line hover:text-accent-hover dark:hover:border-accent-line"
            @click="addEndpoint"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M12 4v16m8-8H4"
              />
            </svg>
            {{ t("admin.settings.site.customEndpoints.add") }}
          </button>
        </div>

        <!-- Contact Info -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{ t("admin.settings.site.contactInfo") }}
          </label>
          <input
            v-model="form.contact_info"
            type="text"
            class="input"
            :placeholder="t('admin.settings.site.contactInfoPlaceholder')"
          />
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{ t("admin.settings.site.contactInfoHint") }}
          </p>
        </div>

        <!-- Doc URL -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{ t("admin.settings.site.docUrl") }}
          </label>
          <input
            v-model="form.doc_url"
            type="url"
            class="input font-mono text-sm"
            :placeholder="t('admin.settings.site.docUrlPlaceholder')"
          />
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{ t("admin.settings.site.docUrlHint") }}
          </p>
        </div>

        <!-- Site Logo Upload -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{ t("admin.settings.site.siteLogo") }}
          </label>
          <ImageUpload
            v-model="form.site_logo"
            mode="image"
            :upload-label="t('admin.settings.site.uploadImage')"
            :remove-label="t('admin.settings.site.remove')"
            :hint="t('admin.settings.site.logoHint')"
            :max-size="300 * 1024"
          />
        </div>

        <!-- Home Content -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
          >
            {{ t("admin.settings.site.homeContent") }}
          </label>
          <textarea
            v-model="form.home_content"
            rows="6"
            class="input font-mono text-sm"
            :placeholder="t('admin.settings.site.homeContentPlaceholder')"
          ></textarea>
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{ t("admin.settings.site.homeContentHint") }}
          </p>
          <!-- iframe CSP Warning -->
          <p class="mt-2 text-xs text-warn">
            {{ t("admin.settings.site.homeContentIframeWarning") }}
          </p>
        </div>

        <!-- Compact Home Page -->
        <div class="flex items-center justify-between border-t border-line-subtle pt-4">
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.site.compactHome")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.site.compactHomeHint") }}
            </p>
          </div>
          <Toggle v-model="form.compact_home_enabled" data-testid="compact-home-toggle" />
        </div>

        <!-- Hide CCS Import Button -->
        <div
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.site.hideCcsImportButton")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.site.hideCcsImportButtonHint") }}
            </p>
          </div>
          <Toggle v-model="form.hide_ccs_import_button" />
        </div>
      </div>
    </div>

    <!-- Custom Menu Items -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.customMenu.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.customMenu.description") }}
        </p>
      </div>
      <div class="space-y-4 p-6">
        <!-- Existing menu items -->
        <div
          v-for="(item, index) in form.custom_menu_items"
          :key="item.id || index"
          class="rounded-sm border border-line p-4"
        >
          <div class="mb-3 flex items-center justify-between">
            <span
              class="text-sm font-medium text-ink-secondary"
            >
              {{
                t("admin.settings.customMenu.itemLabel", { n: index + 1 })
              }}
            </span>
            <div class="flex items-center gap-2">
              <!-- Move up -->
              <button
                v-if="index > 0"
                type="button"
                class="rounded p-1 text-ink-tertiary hover:bg-surface-hover hover:text-ink"
                :title="t('admin.settings.customMenu.moveUp')"
                @click="moveMenuItem(index, -1)"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M5 15l7-7 7 7"
                  />
                </svg>
              </button>
              <!-- Move down -->
              <button
                v-if="index < form.custom_menu_items.length - 1"
                type="button"
                class="rounded p-1 text-ink-tertiary hover:bg-surface-hover hover:text-ink"
                :title="t('admin.settings.customMenu.moveDown')"
                @click="moveMenuItem(index, 1)"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M19 9l-7 7-7-7"
                  />
                </svg>
              </button>
              <!-- Delete -->
              <button
                type="button"
                class="rounded p-1 text-danger hover:bg-danger-tint hover:text-danger"
                :title="t('admin.settings.customMenu.remove')"
                @click="removeMenuItem(index)"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                  />
                </svg>
              </button>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <!-- Label -->
            <div>
              <label
                class="mb-1 block text-xs font-medium text-ink-secondary"
              >
                {{ t("admin.settings.customMenu.name") }}
              </label>
              <input
                v-model="item.label"
                type="text"
                class="input text-sm"
                :placeholder="
                  t('admin.settings.customMenu.namePlaceholder')
                "
              />
            </div>

            <!-- Visibility -->
            <div>
              <label
                class="mb-1 block text-xs font-medium text-ink-secondary"
              >
                {{ t("admin.settings.customMenu.visibility") }}
              </label>
              <select v-model="item.visibility" class="input text-sm">
                <option value="user">
                  {{ t("admin.settings.customMenu.visibilityUser") }}
                </option>
                <option value="admin">
                  {{ t("admin.settings.customMenu.visibilityAdmin") }}
                </option>
              </select>
            </div>

            <!-- URL (full width) -->
            <div class="sm:col-span-2">
              <label
                class="mb-1 block text-xs font-medium text-ink-secondary"
              >
                {{ t("admin.settings.customMenu.url") }}
              </label>
              <input
                v-model="item.url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.customMenu.urlPlaceholder')
                "
              />
            </div>

            <!-- SVG Icon (full width) -->
            <div class="sm:col-span-2">
              <label
                class="mb-1 block text-xs font-medium text-ink-secondary"
              >
                {{ t("admin.settings.customMenu.iconSvg") }}
              </label>
              <ImageUpload
                :model-value="item.icon_svg"
                mode="svg"
                size="sm"
                :upload-label="t('admin.settings.customMenu.uploadSvg')"
                :remove-label="t('admin.settings.customMenu.removeSvg')"
                @update:model-value="(v: string) => (item.icon_svg = v)"
              />
            </div>
          </div>
        </div>

        <!-- Add button -->
        <button
          type="button"
          class="flex w-full items-center justify-center gap-2 rounded-sm border-2 border-dashed border-line py-3 text-sm text-ink-secondary transition-colors hover:border-accent-line hover:text-accent-hover dark:hover:border-accent-line"
          @click="addMenuItem"
        >
          <svg
            class="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M12 4v16m8-8H4"
            />
          </svg>
          {{ t("admin.settings.customMenu.add") }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import ImageUpload from "@/components/common/ImageUpload.vue";
import Toggle from "@/components/common/Toggle.vue";
import { useSettingsFormContext } from "../context";

const {
  activeTab,
  addEndpoint,
  addMenuItem,
  form,
  moveMenuItem,
  removeEndpoint,
  removeMenuItem,
  t,
  tablePageSizeOptionsInput,
} = useSettingsFormContext();
</script>
