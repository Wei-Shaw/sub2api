<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-b-2 border-accent"
        ></div>
      </div>

      <!-- Settings Form -->
      <form v-else @submit.prevent="saveSettings" class="space-y-6" novalidate>
        <!-- Tab Navigation -->
        <div class="sticky top-16 z-20 -mx-1 bg-surface-sunken px-1 py-1">
          <nav
            class="scrollbar-hide overflow-x-auto"
            role="tablist"
            :aria-label="t('admin.settings.title')"
          >
            <div class="tabs min-w-max">
              <button
                v-for="tab in settingsTabs"
                :key="tab.key"
                :id="`settings-tab-${tab.key}`"
                type="button"
                role="tab"
                :aria-selected="activeTab === tab.key"
                :tabindex="activeTab === tab.key ? 0 : -1"
                :class="[
                  'tab inline-flex min-h-[40px] shrink-0 items-center gap-1.5 whitespace-nowrap focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent',
                  activeTab === tab.key && 'tab-active',
                ]"
                @click="selectSettingsTab(tab.key)"
                @keydown="handleSettingsTabKeydown($event, tab.key)"
              >
                <Icon :name="tab.icon" size="sm" class="shrink-0" />
                <span>{{ t(`admin.settings.tabs.${tab.key}`) }}</span>
              </button>
            </div>
          </nav>
        </div>

        <SettingsSecurityTab />

        <SettingsGatewayTab />

        <SettingsUsersTab />

        <SettingsGeneralTab />

        <SettingsAgreementTab />

        <SettingsFeaturesTab />

        <SettingsPaymentTab />

        <SettingsEmailTab />

        <SettingsBackupTab />

        <!-- Save Button: sticks to the viewport bottom while there are unsaved changes -->
        <div
          v-show="activeTab !== 'backup'"
          :class="[
            'flex items-center justify-end gap-3',
            isDirty &&
              'sticky bottom-0 z-20 -mx-1 border-t border-border bg-surface-sunken px-1 py-3',
          ]"
        >
          <span
            v-if="isDirty"
            class="text-sm text-warning"
            role="status"
            data-testid="settings-unsaved-indicator"
          >
            {{ t("admin.settings.unsavedChanges") }}
          </span>
          <button
            type="submit"
            :disabled="saving || loadFailed"
            class="btn btn-primary"
          >
            <svg
              v-if="saving"
              class="h-4 w-4 animate-spin"
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
            {{
              saving
                ? t("admin.settings.saving")
                : t("admin.settings.saveSettings")
            }}
          </button>
        </div>
      </form>

      <!-- Provider dialogs placed outside the settings form to prevent form submission bubbling -->
      <PaymentProviderDialog
        ref="providerDialogRef"
        :show="showProviderDialog"
        :saving="providerSaving"
        :editing="editingProvider"
        :all-key-options="providerKeyOptions"
        :enabled-key-options="enabledProviderKeyOptions"
        :all-payment-types="allPaymentTypes"
        @close="showProviderDialog = false"
        @save="handleSaveProvider"
      />
      <ConfirmDialog
        :show="showDeleteProviderDialog"
        :title="t('admin.settings.payment.deleteProvider')"
        :message="t('admin.settings.payment.deleteProviderConfirm')"
        :confirm-text="t('common.delete')"
        danger
        @confirm="handleDeleteProvider"
        @cancel="showDeleteProviderDialog = false"
      />
      <ConfirmDialog
        :show="affiliateConfirmDialog.show"
        :title="affiliateConfirmDialog.title"
        :message="affiliateConfirmDialog.message"
        :confirm-text="affiliateConfirmDialog.confirmText"
        danger
        @confirm="handleAffiliateConfirm"
        @cancel="cancelAffiliateConfirm"
      />
      <!-- 关闭 step-up 开关等敏感保存操作触发的 TOTP 二次验证 -->
      <TotpStepUpDialog :controller="settingsStepUp" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useSettingsView } from "./settings/useSettingsView";
import { provideSettingsViewContext } from "./settings/context";
import AppLayout from "@/components/layout/AppLayout.vue";
import ConfirmDialog from "@/components/common/ConfirmDialog.vue";
import Icon from "@/components/icons/Icon.vue";
import PaymentProviderDialog from "@/components/payment/PaymentProviderDialog.vue";
import SettingsAgreementTab from "./settings/SettingsAgreementTab.vue";
import SettingsBackupTab from "./settings/SettingsBackupTab.vue";
import SettingsEmailTab from "./settings/SettingsEmailTab.vue";
import SettingsFeaturesTab from "./settings/SettingsFeaturesTab.vue";
import SettingsGatewayTab from "./settings/SettingsGatewayTab.vue";
import SettingsGeneralTab from "./settings/SettingsGeneralTab.vue";
import SettingsPaymentTab from "./settings/SettingsPaymentTab.vue";
import SettingsSecurityTab from "./settings/SettingsSecurityTab.vue";
import SettingsUsersTab from "./settings/SettingsUsersTab.vue";
import TotpStepUpDialog from "@/components/auth/TotpStepUpDialog.vue";

// 状态与逻辑已纯移动到 settings/useSettingsView.ts；本文件只保留 Tab 导航与对话框（openspec Phase 3）
const ctx = useSettingsView();
provideSettingsViewContext(ctx);
const {
  activeTab,
  affiliateConfirmDialog,
  allPaymentTypes,
  cancelAffiliateConfirm,
  editingProvider,
  enabledProviderKeyOptions,
  handleAffiliateConfirm,
  handleDeleteProvider,
  handleSaveProvider,
  handleSettingsTabKeydown,
  isDirty,
  loadFailed,
  loading,
  providerKeyOptions,
  providerSaving,
  saveSettings,
  saving,
  selectSettingsTab,
  settingsStepUp,
  settingsTabs,
  showDeleteProviderDialog,
  showProviderDialog,
  t,
  providerDialogRef,
} = ctx;
</script>

