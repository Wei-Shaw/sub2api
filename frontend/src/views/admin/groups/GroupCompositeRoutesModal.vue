<template>
  <!-- Composite Routes Modal -->
  <BaseDialog
    :show="showCompositeRoutesModal"
    :title="
      compositeRoutesGroup
        ? t('admin.groups.compositeRoutes.titleWithGroup', {
            name: compositeRoutesGroup.name,
          })
        : t('admin.groups.compositeRoutes.title')
    "
    width="wide"
    @close="closeCompositeRoutesModal"
  >
    <div class="grid gap-5 lg:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
      <section class="min-w-0">
        <div class="mb-3 flex items-center justify-between gap-3">
          <h3 class="text-h3 font-bold text-accent-strong">
            {{ t("admin.groups.compositeRoutes.routes") }}
          </h3>
          <button
            :aria-label="t('common.refresh')"
            :title="t('common.refresh')"
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="compositeRoutesLoading"
            @click="loadCompositeRoutes"
          >
            <Icon
              name="refresh"
              size="sm"
              :class="compositeRoutesLoading ? 'animate-spin' : ''"
            />
          </button>
        </div>

        <div class="table-container">
          <div
            v-if="compositeRoutesLoading"
            class="flex h-36 items-center justify-center text-sm text-fg-muted"
          >
            {{ t("common.loading") }}
          </div>
          <div
            v-else-if="compositeRoutes.length === 0"
            class="flex h-36 items-center justify-center text-sm text-fg-muted"
          >
            {{ t("admin.groups.compositeRoutes.empty") }}
          </div>
          <div v-else>
            <table class="table">
              <thead>
                <tr>
                  <th>
                    {{ t("admin.groups.compositeRoutes.publicModel") }}
                  </th>
                  <th>
                    {{ t("admin.groups.compositeRoutes.target") }}
                  </th>
                  <th>
                    {{ t("admin.groups.compositeRoutes.scope") }}
                  </th>
                  <th class="text-right">
                    {{ t("admin.groups.columns.actions") }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="route in compositeRoutes"
                  :key="route.id"
                  :class="!route.enabled && 'opacity-60'"
                >
                  <td class="max-w-[15rem]">
                    <div class="break-all font-mono font-medium text-fg">
                      {{ route.public_model }}
                    </div>
                    <div class="mt-1 flex flex-wrap items-center gap-1.5">
                      <span class="badge badge-gray">{{
                        compositeRouteMatchLabel(route.match_type)
                      }}</span>
                      <span
                        v-if="!route.enabled"
                        class="badge badge-danger"
                      >
                        {{ t("admin.accounts.status.inactive") }}
                      </span>
                    </div>
                  </td>
                  <td>
                    <div class="flex items-center gap-1.5 text-fg">
                      <PlatformIcon :platform="route.target_platform" size="xs" />
                      <span>{{ formatCompositePlatform(route.target_platform) }}</span>
                    </div>
                    <div class="mt-1 break-all font-mono text-xs text-fg-muted">
                      {{ route.upstream_model || route.public_model }}
                    </div>
                  </td>
                  <td>
                    <div class="text-fg">
                      {{ formatCompositeEndpoint(route.endpoint) }}
                    </div>
                    <div class="text-xs tabular-nums text-fg-muted">
                      {{ t("admin.groups.compositeRoutes.priority") }}:
                      {{ route.priority }}
                    </div>
                  </td>
                  <td>
                    <div class="flex justify-end gap-1">
                      <button
                        type="button"
                        class="rounded-sm p-1.5 text-fg-muted hover:bg-accent-weak hover:text-accent-strong"
                        :title="t('common.edit')" :aria-label="t('common.edit')"
                        @click="editCompositeRoute(route)"
                      >
                        <Icon name="edit" size="sm" />
                      </button>
                      <button
                        type="button"
                        class="rounded-sm p-1.5 text-fg-muted hover:bg-danger-weak hover:text-danger"
                        :title="t('common.delete')" :aria-label="t('common.delete')"
                        @click="deleteCompositeRoute(route)"
                      >
                        <Icon name="trash" size="sm" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section class="space-y-5">
        <form class="space-y-3" @submit.prevent="saveCompositeRoute">
          <div class="flex items-center justify-between gap-3">
            <h3 class="text-h3 font-bold text-accent-strong">
              {{
                compositeRouteEditingId
                  ? t("admin.groups.compositeRoutes.editRoute")
                  : t("admin.groups.compositeRoutes.addRoute")
              }}
            </h3>
            <button
              v-if="compositeRouteEditingId"
              type="button"
              class="text-xs font-medium text-fg-muted hover:text-fg"
              @click="resetCompositeRouteForm"
            >
              {{ t("common.cancel") }}
            </button>
          </div>

          <div>
            <label class="input-label">{{
              t("admin.groups.compositeRoutes.publicModel")
            }}</label>
            <input
              v-model.trim="compositeRouteForm.public_model"
              type="text"
              class="input font-mono"
              required
              placeholder="openrouter/gpt-5"
            />
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label class="input-label">{{
                t("admin.groups.compositeRoutes.matchType")
              }}</label>
              <Select
                v-model="compositeRouteForm.match_type"
                :options="compositeRouteMatchOptions"
              />
            </div>
            <div>
              <label class="input-label">{{
                t("admin.groups.compositeRoutes.endpoint")
              }}</label>
              <Select
                v-model="compositeRouteForm.endpoint"
                :options="compositeRouteEndpointOptions"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label class="input-label">{{
                t("admin.groups.compositeRoutes.targetPlatform")
              }}</label>
              <Select
                v-model="compositeRouteForm.target_platform"
                :options="compositeRoutePlatformOptions"
              />
            </div>
            <div>
              <label class="input-label">{{
                t("admin.groups.compositeRoutes.priority")
              }}</label>
              <input
                v-model.number="compositeRouteForm.priority"
                type="number"
                min="1"
                step="1"
                class="input"
              />
            </div>
          </div>

          <div>
            <label class="input-label">{{
              t("admin.groups.compositeRoutes.upstreamModel")
            }}</label>
            <input
              v-model.trim="compositeRouteForm.upstream_model"
              type="text"
              class="input font-mono"
              placeholder="gpt-5"
            />
            <p class="mt-1 text-xs text-fg-muted">
              {{ t("admin.groups.compositeRoutes.upstreamModelHint") }}
            </p>
          </div>

          <div>
            <label class="input-label">{{
              t("admin.groups.compositeRoutes.notes")
            }}</label>
            <textarea
              v-model.trim="compositeRouteForm.notes"
              rows="2"
              class="input"
            ></textarea>
          </div>

          <div class="flex items-center justify-between gap-3">
            <label class="flex items-center gap-2 text-sm text-fg">
              <input
                v-model="compositeRouteForm.enabled"
                type="checkbox"
                class="h-4 w-4 rounded-sm border-border-strong text-accent focus:ring-accent"
              />
              {{ t("admin.groups.compositeRoutes.enabled") }}
            </label>
            <button
              type="submit"
              class="btn btn-primary"
              :disabled="compositeRouteSaving"
            >
              <Icon
                v-if="!compositeRouteSaving"
                name="check"
                size="sm"
                class="mr-2"
              />
              {{ compositeRouteEditingId ? t("common.update") : t("common.create") }}
            </button>
          </div>
        </form>

        <div class="border-t border-border pt-4">
          <h3 class="mb-3 text-h3 font-bold text-accent-strong">
            {{ t("admin.groups.compositeRoutes.preview") }}
          </h3>
          <div class="space-y-3">
            <input
              v-model.trim="compositePreviewModel"
              type="text"
              class="input font-mono"
              placeholder="openrouter/gpt-5"
              @keyup.enter="previewCompositeRoute"
            />
            <div class="flex gap-2">
              <Select
                v-model="compositePreviewEndpoint"
                :options="compositeRouteEndpointOptions"
                class="min-w-0 flex-1"
              />
              <button
                :aria-label="t('admin.groups.compositeRoutes.preview')"
                :title="t('admin.groups.compositeRoutes.preview')"
                type="button"
                class="btn btn-secondary"
                :disabled="compositePreviewLoading || !compositePreviewModel"
                @click="previewCompositeRoute"
              >
                <Icon name="play" size="sm" />
              </button>
            </div>

            <div
              v-if="compositePreviewDecision"
              class="border border-accent/40 pl-3 text-sm"
            >
              <div class="mb-2 flex items-center gap-2">
                <span
                  :class="[
                    'badge',
                    compositePreviewDecision.matched
                      ? 'badge-success'
                      : 'badge-danger',
                  ]"
                >
                  {{
                    compositePreviewDecision.matched
                      ? t("admin.groups.compositeRoutes.matched")
                      : t("admin.groups.compositeRoutes.notMatched")
                  }}
                </span>
                <span class="badge badge-gray">
                  {{
                    compositeRouteSourceLabel(
                      compositePreviewDecision.source,
                    )
                  }}
                </span>
              </div>
              <div
                v-if="compositePreviewDecision.matched"
                class="space-y-1 text-fg"
              >
                <div>
                  {{ t("admin.groups.compositeRoutes.targetPlatform") }}:
                  {{
                    formatCompositePlatform(
                      compositePreviewDecision.target_platform,
                    )
                  }}
                </div>
                <div class="break-all">
                  {{ t("admin.groups.compositeRoutes.upstreamModel") }}:
                  {{ compositePreviewDecision.upstream_model }}
                </div>
                <div
                  v-if="compositePreviewDecision.available_accounts === 0"
                  class="font-medium text-danger"
                >
                  {{ t("admin.groups.compositeRoutes.noAvailableAccounts") }}
                </div>
                <div v-else-if="compositePreviewDecision.available_accounts !== undefined">
                  {{ t("admin.groups.compositeRoutes.availableAccounts") }}:
                  {{ compositePreviewDecision.available_accounts }}
                </div>
              </div>
              <div
                v-else
                class="text-fg-muted"
              >
                {{ compositePreviewDecision.reason }}
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <template #footer>
      <div class="flex justify-end pt-4">
        <button
          type="button"
          class="btn btn-secondary"
          @click="closeCompositeRoutesModal"
        >
          {{ t("common.close") }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useGroupsViewContext } from "./context";
import BaseDialog from "@/components/common/BaseDialog.vue";
import Icon from "@/components/icons/Icon.vue";
import PlatformIcon from "@/components/common/PlatformIcon.vue";
import Select from "@/components/common/Select.vue";

// 纯移动拆分：所有状态与方法来自 GroupsView 提供的上下文（openspec: rebuild-frontend-design-system Phase 3）
const ctx = useGroupsViewContext();
const {
  closeCompositeRoutesModal,
  compositePreviewDecision,
  compositePreviewEndpoint,
  compositePreviewLoading,
  compositePreviewModel,
  compositeRouteEditingId,
  compositeRouteEndpointOptions,
  compositeRouteForm,
  compositeRouteMatchLabel,
  compositeRouteMatchOptions,
  compositeRoutePlatformOptions,
  compositeRouteSaving,
  compositeRouteSourceLabel,
  compositeRoutes,
  compositeRoutesGroup,
  compositeRoutesLoading,
  deleteCompositeRoute,
  editCompositeRoute,
  formatCompositeEndpoint,
  formatCompositePlatform,
  loadCompositeRoutes,
  previewCompositeRoute,
  resetCompositeRouteForm,
  saveCompositeRoute,
  showCompositeRoutesModal,
  t,
} = ctx;
</script>
