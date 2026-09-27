<template>
  <div v-show="activeTab === 'agreement'" class="space-y-6">
    <div class="card">
      <div class="card-header">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h2 class="card-title">
              {{ t("admin.settings.agreement.title") }}
            </h2>
            <p class="mt-1 text-sm text-fg-muted">
              {{
                t("admin.settings.agreement.description")
              }}
            </p>
          </div>
          <div class="flex items-center gap-3">
            <span class="text-sm text-fg-muted">
              {{ form.login_agreement_enabled ? t("admin.settings.agreement.enabled") : t("admin.settings.agreement.disabled") }}
            </span>
            <Toggle v-model="form.login_agreement_enabled" />
          </div>
        </div>
      </div>

      <div class="card-body space-y-6">
        <div class="grid grid-cols-1 gap-5 lg:grid-cols-[minmax(0,1fr)_220px]">
          <div>
            <label class="input-label">
              {{ t("admin.settings.agreement.displayMode") }}
            </label>
            <div class="grid grid-cols-2 divide-x divide-border-strong border border-border-strong">
              <button
                type="button"
                class="inline-flex min-h-[40px] items-center justify-center gap-2 px-3 py-2 text-sm font-semibold transition-colors"
                :class="
                  form.login_agreement_mode === 'modal'
                    ? 'bg-accent text-white dark:text-surface-sunken'
                    : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                "
                @click="form.login_agreement_mode = 'modal'"
              >
                <Icon name="shield" size="sm" />
                {{ t("admin.settings.agreement.modeModal") }}
              </button>
              <button
                type="button"
                class="inline-flex min-h-[40px] items-center justify-center gap-2 px-3 py-2 text-sm font-semibold transition-colors"
                :class="
                  form.login_agreement_mode === 'checkbox'
                    ? 'bg-accent text-white dark:text-surface-sunken'
                    : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                "
                @click="form.login_agreement_mode = 'checkbox'"
              >
                <Icon name="checkCircle" size="sm" />
                {{ t("admin.settings.agreement.modeCheckbox") }}
              </button>
            </div>
            <p class="input-hint">
              {{
                form.login_agreement_mode === "checkbox"
                  ? t("admin.settings.agreement.modeCheckboxHint")
                  : t("admin.settings.agreement.modeModalHint")
              }}
            </p>
          </div>

          <div>
            <label class="input-label">
              {{ t("admin.settings.agreement.updatedAt") }}
            </label>
            <input
              v-model="form.login_agreement_updated_at"
              type="date"
              class="input"
            />
            <p class="input-hint">
              {{ t("admin.settings.agreement.updatedAtHint") }}
            </p>
          </div>
        </div>

        <div>
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 class="text-sm font-medium text-fg">
                {{ t("admin.settings.agreement.documents") }}
              </h3>
              <p class="mt-1 text-xs text-fg-muted">
                {{
                  t("admin.settings.agreement.documentsHint")
                }}
              </p>
            </div>
            <button
              type="button"
              class="btn btn-primary btn-sm inline-flex items-center gap-1.5"
              @click="addLoginAgreementDocument"
            >
              <Icon name="plus" size="sm" />
              {{ t("admin.settings.agreement.addDocument") }}
            </button>
          </div>

          <div class="mt-4 space-y-3">
            <div
              v-for="(doc, index) in form.login_agreement_documents"
              :key="doc.id || index"
              class="border-t border-border pt-4"
            >
              <div class="mb-3 flex items-center justify-between gap-3">
                <div class="flex min-w-0 items-center gap-3">
                  <span class="flex h-5 w-5 flex-shrink-0 items-center justify-center text-accent">
                    <Icon
                      :name="
                        index === 1
                          ? 'shield'
                          : index === 2
                            ? 'globe'
                            : index === 3
                              ? 'cog'
                              : 'document'
                      "
                      size="sm"
                    />
                  </span>
                  <div class="min-w-0">
                    <p class="truncate text-sm font-semibold text-fg">
                      {{ doc.title || t("admin.settings.agreement.untitledDocument") }}
                    </p>
                    <p class="truncate text-xs text-fg-muted">
                      {{ loginAgreementRoutePath(doc, index) }}
                    </p>
                  </div>
                </div>
                <button
                  :aria-label="t('common.delete')"
                  :title="t('common.delete')"
                  type="button"
                  class="rounded-sm p-2 text-danger transition hover:bg-danger-weak hover:text-danger-strong disabled:cursor-not-allowed disabled:opacity-40"
                  :disabled="
                    form.login_agreement_enabled &&
                    form.login_agreement_documents.length <= 1
                  "
                  @click="removeLoginAgreementDocument(index)"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </div>

              <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
                <div>
                  <label class="mb-1 block text-xs font-medium text-fg-muted">
                    {{ t("admin.settings.agreement.documentTitle") }}
                  </label>
                  <input
                    v-model="doc.title"
                    type="text"
                    class="input text-sm"
                    :placeholder="t('admin.settings.agreement.documentTitlePlaceholder')"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-fg-muted">
                    {{ t("admin.settings.agreement.routeSlug") }}
                  </label>
                  <div class="flex overflow-hidden rounded-sm border border-border-strong bg-surface focus-within:border-accent focus-within:ring-1 focus-within:ring-accent">
                    <span class="inline-flex flex-shrink-0 items-center border-r border-border bg-surface-sunken px-3 text-sm text-fg-muted">
                      /legal/
                    </span>
                    <input
                      v-model="doc.id"
                      type="text"
                      class="min-w-0 flex-1 border-0 bg-transparent px-3 py-2 text-sm text-fg outline-none placeholder:text-fg-subtle focus:ring-0"
                      placeholder="usage-policy"
                    />
                  </div>
                </div>
              </div>
              <div class="mt-3">
                <label class="mb-1 block text-xs font-medium text-fg-muted">
                  {{ t("admin.settings.agreement.markdownContent") }}
                </label>
                  <textarea
                    v-model="doc.content_md"
                    rows="8"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.settings.agreement.markdownContentPlaceholder')"
                  ></textarea>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useSettingsViewContext } from "./context";
import Icon from "@/components/icons/Icon.vue";
import Toggle from "@/components/common/Toggle.vue";

// 纯移动拆分：所有状态与方法来自 SettingsView 提供的上下文（openspec: rebuild-frontend-design-system Phase 3）
const ctx = useSettingsViewContext();
const {
  activeTab,
  addLoginAgreementDocument,
  form,
  loginAgreementRoutePath,
  removeLoginAgreementDocument,
  t,
} = ctx;
</script>
