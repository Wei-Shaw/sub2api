<!--
  VideoPlaygroundView：视频演练台独立页面（取代之前的弹窗形态）。

  路由 :  /video-models/:slug(.*)+/playground
    slug 使用 catchAll 语法（.*），因为 model slug 可能含 "/"，如
      "bytedance/seedance-2.5/text-to-video"。

  页面布局：
    ┌─────────────────────────────────────────────────────┐
    │  ← 返回视频模型     视频演练台 · <slug>              │
    ├───────────────────────────┬─────────────────────────┤
    │  参数区                   │  结果区                  │
    │   - API Key (Select)      │   - 运行状态             │
    │   - 表单 / JSON           │   - resolvedOutputs      │
    │   - curl 预览             │   - 原始 payload         │
    │   - 提交按钮              │                          │
    └───────────────────────────┴─────────────────────────┘

  与旧 VideoPlaygroundDialog 的差异：
    1. 不再是遮罩弹窗；作为 AppLayout 下的常规页面挂载。
    2. API Key / mode 切换 tab 里的原生 <select> / <button> 换成
       通用 Select 组件，与项目其它下拉视觉一致。
    3. 结果放到右侧列；提交/取消/重置按钮固定在参数栏底部。
    4. 顶部提供"返回视频模型"按钮，走 router.push('/video-models')。
-->
<template>
  <AppLayout>
    <div class="space-y-4">
      <!--
        页头：标题在左，“返回视频模型”按钮靠到最右。
        标题行本身加上白底（card 卡片风格），与下方结果卡片一致，提高频道感。
      -->
      <!--
        页头：
          - 左侧标题：模型 display_name（更贴合"是哪个模型"的语义）
            未加载出来时回退到 slug；下方仍展示灰色 slug 作为唯一标识。
          - 右侧：价格表 + 返回按钮；点价格表弹出 pricing 表格。
        标题行本身加上白底（card 卡片风格），与下方结果卡片一致，提高频道感。
      -->
      <div class="card flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="min-w-0">
          <h1 class="truncate text-2xl font-semibold text-gray-900 dark:text-gray-100">
            {{ headerTitle }}
          </h1>
          <!--
            标签行：展示当前模型 slug 的两级分类（vendor / family）。
            举例：slug = "bytedance/seedance-2.5/text-to-video" 时，
              vendor = "bytedance"，family = "seedance-2.5"，此处渲染两枚 chip。
            点击 chip：跳转到视频模型列表页 VideoModels，通过 query 传入
            vendor / family，目标页会把过滤器同步到该值上（类似"搜索该 tag"）。
            仅当对应值非空时才渲染，避免"只有 vendor 没有 family"的模型出现空 chip。
          -->
          <div v-if="vendorTag || familyTag" class="mt-1.5 flex flex-wrap items-center gap-1.5">
            <button
              v-if="vendorTag"
              type="button"
              class="inline-flex items-center rounded-full bg-blue-50 px-2 py-0.5 text-[11px] font-medium text-blue-700 transition hover:bg-blue-100 dark:bg-blue-950 dark:text-blue-300 dark:hover:bg-blue-900"
              :title="t('videoModels.playground.tagJumpHint', { tag: vendorTag })"
              @click="jumpToVideoModelsByTag('vendor', vendorTag)"
            >
              {{ vendorTag }}
            </button>
            <button
              v-if="familyTag"
              type="button"
              class="inline-flex items-center rounded-full bg-indigo-50 px-2 py-0.5 text-[11px] font-medium text-indigo-700 transition hover:bg-indigo-100 dark:bg-indigo-950 dark:text-indigo-300 dark:hover:bg-indigo-900"
              :title="t('videoModels.playground.tagJumpHint', { tag: familyTag })"
              @click="jumpToVideoModelsByTag('family', familyTag)"
            >
              {{ familyTag }}
            </button>
          </div>
          <!--
            备注：过去这里还展示了一行小灰字 slug（displaySlug）作为唯一标识。
            用户反馈"重复且冗余"，故移除；vendor/family 已经能让用户认知模型出处，
            完整 slug 仍可在页面其它入口（价格表 tooltip、请求 URL 等）看到。
          -->
        </div>
        <div class="flex items-center gap-2 self-start sm:self-auto">
          <button
            v-if="model && model.pricing && model.pricing.length > 0"
            type="button"
            class="btn btn-secondary"
            @click="showPricingDialog = true"
            :title="t('videoModels.playground.pricingBtn')"
          >
            <Icon name="dollar" size="md" />
            <span class="ml-1">{{ t('videoModels.playground.pricingBtn') }}</span>
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            @click="goBack"
            :title="t('videoModels.playground.backToList')"
          >
            <Icon name="chevronLeft" size="md" />
            <span class="ml-1">{{ t('videoModels.playground.backToList') }}</span>
          </button>
        </div>
      </div>

      <!-- 价格表弹窗：只读展示当前模型的 pricing[*]（resolution × price_per_second × enabled）
           不改动配置，仅供用户对照参考。 -->
      <BaseDialog
        :show="showPricingDialog"
        :title="t('videoModels.playground.pricingDialogTitle', { name: headerTitle })"
        width="normal"
        @close="showPricingDialog = false"
      >
        <div v-if="model && model.pricing && model.pricing.length > 0" class="space-y-2">
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('videoModels.playground.pricingDialogHint') }}
          </p>
          <div class="overflow-hidden rounded border border-gray-200 dark:border-gray-800">
            <table class="w-full text-sm">
              <thead class="bg-gray-50 dark:bg-gray-900">
                <tr>
                  <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                    {{ t('videoModels.playground.pricingColResolution') }}
                  </th>
                  <th class="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-300">
                    {{ t('videoModels.playground.pricingColUnit') }}
                  </th>
                  <th class="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-300">
                    {{ t('videoModels.playground.pricingColStatus') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-200 dark:divide-gray-800">
                <tr v-for="p in model.pricing" :key="p.resolution">
                  <td class="px-3 py-2 font-mono text-gray-800 dark:text-gray-200">{{ p.resolution }}</td>
                  <td class="px-3 py-2 text-right font-mono text-blue-600 dark:text-blue-400">
                    ${{ p.price_per_second.toFixed(4) }} / {{ t('videoModels.playground.pricingUnitSecond') }}
                  </td>
                  <td class="px-3 py-2 text-right">
                    <span
                      v-if="p.enabled"
                      class="rounded bg-green-100 px-1.5 py-0.5 text-[10px] font-medium text-green-700 dark:bg-green-950 dark:text-green-300"
                    >
                      {{ t('videoModels.playground.pricingEnabled') }}
                    </span>
                    <span v-else class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                      {{ t('videoModels.playground.pricingDisabled') }}
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </BaseDialog>

      <!-- 模型未找到的兜底提示 -->
      <div
        v-if="!modelLoaded && !modelLoading"
        class="card p-10 text-center text-sm text-gray-500"
      >
        {{ t('videoModels.playground.modelNotFound') }}
      </div>

      <div
        v-else-if="modelLoading"
        class="card p-10 text-center text-sm text-gray-500"
      >
        {{ t('common.loading') }}
      </div>

      <!-- 顶部 Tabs：Playground / 历史记录（白底卡片，与其余控件同一视觉 tier） -->
      <div v-if="modelLoaded" class="card flex items-center gap-2 border-b border-gray-200 p-2 dark:border-gray-800">
        <button
          type="button"
          @click="topTab = 'playground'"
          :class="topTabClass(topTab === 'playground')"
        >
          {{ t('videoModels.playground.tabPlayground') }}
        </button>
        <button
          type="button"
          @click="onSwitchToHistory"
          :class="topTabClass(topTab === 'history')"
        >
          {{ t('videoModels.playground.tabHistory') }}
          <span v-if="historyTotal > 0" class="ml-1 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-gray-800 dark:text-gray-400">
            {{ historyTotal }}
          </span>
        </button>
      </div>

      <!-- 历史记录 Tab 内容 -->
      <div v-if="modelLoaded && topTab === 'history'" class="card p-5">
        <VideoPlaygroundHistory
          :slug="slug"
          @replay="onReplayTask"
          @loaded="(total: number) => historyTotal = total"
        />
      </div>

      <!-- 主体：左右两栏（仅 Playground Tab 下渲染）
           栅格比例：左（参数）7 / 右（结果）5，右栏留白窄一点，视觉更紧凑。 -->
      <div v-else-if="modelLoaded" class="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <!-- ================== 左栏：参数区 ================== -->
        <section class="card space-y-4 p-5 lg:col-span-7">
          <!-- API Key 选择：用通用 Select -->
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('videoModels.playground.apiKeyLabel') }}
              <span class="text-red-500">*</span>
            </label>
            <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.apiKeyHelper') }}
            </p>
            <div v-if="keysLoading" class="text-sm text-gray-500">
              {{ t('common.loading') }}
            </div>
            <!-- 没有可用密钥：直接在这里给出创建入口。
                 让用户为了建一把 key 跳去"API 密钥"页，回来后演练台已填的参数
                 全丢了，体验很差；所以这里内联一个快速创建弹窗。 -->
            <div
              v-else-if="!compositeKeys.length"
              class="rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-800 dark:bg-yellow-950/40 dark:text-yellow-200"
            >
              <p>{{ t('videoModels.playground.noCompositeKey') }}</p>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                <button type="button" class="btn btn-primary btn-xs" @click="openCreateKey">
                  {{ t('videoModels.playground.createKeyNow') }}
                </button>
                <button type="button" class="btn btn-ghost btn-xs" @click="goKeysPage">
                  {{ t('videoModels.playground.createKeyAdvanced') }}
                </button>
              </div>
            </div>
            <Select
              v-else
              :model-value="selectedKeyId"
              :options="keyOptions"
              :disabled="playground.isBusy.value"
              :placeholder="t('videoModels.playground.selectKey')"
              :searchable="compositeKeys.length > 5"
              @update:model-value="(v: string | number | boolean | null) => selectedKeyId = (v == null ? '' : Number(v))"
            />
          </div>

          <!-- 快速创建 API 密钥：只暴露"名称 + 分组"两个必要字段。
               配额 / IP 名单 / 有效期等高级项交给"API 密钥"页，这里保持最短路径。 -->
          <BaseDialog
            :show="showCreateKey"
            :title="t('videoModels.playground.createKeyTitle')"
            @close="showCreateKey = false"
          >
            <div class="space-y-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('videoModels.playground.createKeyHint') }}
              </p>

              <div v-if="createKeyGroupsLoading" class="text-sm text-gray-500">
                {{ t('common.loading') }}
              </div>

              <!-- 一个可用的 composite 分组都没有：建了 key 也调不通视频模型，
                   因此不给创建入口，直接引导去分组广场订阅。 -->
              <div
                v-else-if="!compositeGroupOptions.length"
                class="rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-800 dark:bg-yellow-950/40 dark:text-yellow-200"
              >
                {{ t('videoModels.playground.createKeyNoGroup') }}
              </div>

              <template v-else>
                <div>
                  <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('videoModels.playground.createKeyName') }}
                    <span class="text-red-500">*</span>
                  </label>
                  <input
                    v-model="createKeyName"
                    type="text"
                    class="input"
                    maxlength="100"
                    :disabled="createKeySubmitting"
                    :placeholder="t('videoModels.playground.createKeyNamePlaceholder')"
                    @keyup.enter="submitCreateKey"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('videoModels.playground.createKeyGroup') }}
                    <span class="text-red-500">*</span>
                  </label>
                  <Select
                    :model-value="createKeyGroupId"
                    :options="compositeGroupOptions"
                    :disabled="createKeySubmitting"
                    :searchable="compositeGroupOptions.length > 5"
                    :placeholder="t('videoModels.playground.createKeyGroupPlaceholder')"
                    @update:model-value="(v: string | number | boolean | null) => createKeyGroupId = (v == null ? '' : Number(v))"
                  />
                </div>
              </template>
            </div>

            <template #footer>
              <button type="button" class="btn btn-secondary" @click="showCreateKey = false">
                {{ t('common.cancel') }}
              </button>
              <button
                v-if="compositeGroupOptions.length"
                type="button"
                class="btn btn-primary"
                :disabled="createKeySubmitting || !createKeyName.trim() || createKeyGroupId === ''"
                @click="submitCreateKey"
              >
                {{ createKeySubmitting ? t('common.submitting') : t('videoModels.playground.createKeySubmit') }}
              </button>
              <button v-else type="button" class="btn btn-primary" @click="goKeysPage">
                {{ t('videoModels.playground.createKeyAdvanced') }}
              </button>
            </template>
          </BaseDialog>

          <!-- 参数模式切换：表单 / JSON -->
          <div class="flex items-center gap-2 border-b border-gray-200 pb-2 dark:border-gray-800">
            <button
              v-if="fieldSpecs.length > 0"
              @click="mode = 'form'"
              :class="tabClass(mode === 'form')"
            >
              {{ t('videoModels.playground.tabForm') }}
            </button>
            <button
              @click="switchToJsonMode"
              :class="tabClass(mode === 'json')"
            >
              {{ t('videoModels.playground.tabJson') }}
            </button>
          </div>

          <!-- 表单模式 -->
          <div v-if="mode === 'form' && fieldSpecs.length > 0" class="space-y-4">
            <!-- 普通字段：一进入演练台就展开，主流程一步到位。 -->
            <VideoPlaygroundSchemaField
              v-for="f in primaryFieldSpecs"
              :key="f.key"
              :spec="f"
              :model-value="formData[f.key]"
              :disabled="playground.isBusy.value"
              :media-references="promptMediaReferences"
              @update:model-value="onFieldValueChange(f.key, $event)"
            />

            <!--
              高级参数：默认折叠，避免 seed / camera_control 等偏专业字段
              铺满左栏。使用原生 <details> 保持“组件轻量 + 键盘可及”，
              summary 上加个 chevron 提示可展开。仅当管理员声明了至少一个
              高级字段时才渲染整个折叠区，避免出现空的“高级参数（0）”标题。
            -->
            <details
              v-if="advancedFieldSpecs.length > 0"
              class="rounded-lg border border-dashed border-gray-300 bg-gray-50 dark:border-gray-700 dark:bg-gray-950/40"
            >
              <summary
                class="flex cursor-pointer select-none items-center justify-between gap-2 rounded-lg px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-900"
              >
                <span class="flex items-center gap-2">
                  <svg class="h-3.5 w-3.5 transition-transform" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                    <path fill-rule="evenodd" d="M6 6l8 4-8 4V6z" clip-rule="evenodd" />
                  </svg>
                  {{ t('videoModels.playground.advancedFields') }}
                </span>
                <span class="rounded-full bg-gray-200 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-gray-800 dark:text-gray-400">
                  {{ advancedFieldSpecs.length }}
                </span>
              </summary>
              <div class="space-y-4 border-t border-dashed border-gray-300 px-3 py-3 dark:border-gray-700">
                <VideoPlaygroundSchemaField
                  v-for="f in advancedFieldSpecs"
                  :key="f.key"
                  :spec="f"
                  :model-value="formData[f.key]"
                  :disabled="playground.isBusy.value"
                  :media-references="promptMediaReferences"
                  @update:model-value="onFieldValueChange(f.key, $event)"
                />
              </div>
            </details>
          </div>

          <!-- JSON 模式 -->
          <div v-else>
            <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('videoModels.playground.jsonLabel') }}
            </label>
            <p
              v-if="fieldSpecs.length === 0"
              class="mb-1 text-xs text-gray-500 dark:text-gray-400"
            >
              {{ t('videoModels.playground.jsonOnlyHint') }}
            </p>
            <textarea
              v-model="jsonInput"
              :disabled="playground.isBusy.value"
              rows="12"
              spellcheck="false"
              class="w-full rounded border border-gray-300 bg-gray-50 px-3 py-2 font-mono text-xs focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-100"
            />
            <p v-if="jsonError" class="mt-1 text-xs text-red-600 dark:text-red-400">
              {{ jsonError }}
            </p>
          </div>

          <!-- curl 示例（可折叠） -->
          <details class="rounded-lg border border-gray-200 bg-gray-50 dark:border-gray-800 dark:bg-gray-950/50">
            <summary
              class="flex cursor-pointer items-center justify-between gap-2 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-900"
            >
              <span>{{ t('videoModels.playground.curlLabel') }}</span>
              <!-- 与标题同一行：是否在示例里展示真实 API Key。
                   使用 <span> 包裹（而非 <label>）以避免嵌套 label；显式 stop
                   点击/键盘事件，防止操作 toggle 时误触发 <details> 的展开/收起。 -->
              <span
                class="flex cursor-pointer items-center gap-2 text-xs font-normal text-gray-600 dark:text-gray-300"
                @click.stop
                @keydown.stop
              >
                <span>{{ t('videoModels.playground.curlShowApiKey') }}</span>
                <Toggle v-model="showApiKeyInCurl" />
              </span>
            </summary>
            <div class="border-t border-gray-200 p-3 dark:border-gray-800">
              <!-- 复制按钮放在代码块右上角，缩小尺寸；使用绝对定位覆盖在 <pre> 之上，
                   不占用垂直空间，视觉更清爽。按钮半透明背景避免遮挡代码。 -->
              <div class="relative">
                <button
                  type="button"
                  class="absolute right-1.5 top-1.5 z-10 rounded border border-white/10 bg-white/10 px-1.5 py-0.5 text-[10px] font-medium text-gray-100 backdrop-blur-sm transition hover:bg-white/20"
                  @click="copyCurl"
                  :title="t('videoModels.playground.curlCopy')"
                >
                  {{ t('videoModels.playground.curlCopy') }}
                </button>
                <pre
                  class="max-h-72 overflow-auto rounded-md bg-gray-900 p-3 pr-16 text-xs leading-relaxed text-gray-100 dark:bg-black"
                ><code>{{ curlSnippet }}</code></pre>
              </div>
            </div>
          </details>

          <!-- "如何引入你的程序里"指引（可折叠）
               目的：给不熟悉异步 submit + poll 模型的用户一个"一键喂给 AI 助手"的落地路径。
               交互：选/填目标语言 → 下方 prompt 自动替换 <你的语言> → 复制发给 AI 助手。
               prompt 里会自动嵌入当前 curl 示例，保证 AI 看到与网关一致的真实请求形态。 -->
          <details class="rounded-lg border border-gray-200 bg-gray-50 dark:border-gray-800 dark:bg-gray-950/50">
            <summary
              class="flex cursor-pointer items-center justify-between gap-2 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-900"
            >
              <span>{{ t('videoModels.playground.integrateLabel') }}</span>
              <span class="text-xs font-normal text-gray-500 dark:text-gray-400">
                {{ t('videoModels.playground.integrateHint') }}
              </span>
            </summary>
            <div class="border-t border-gray-200 p-3 dark:border-gray-800 space-y-2">
              <!-- 目标语言：使用项目通用 Select 组件（creatable=支持自由输入，
                   searchable=支持搜索），与其它下拉视觉一致。 -->
              <div class="flex flex-wrap items-center gap-2">
                <label
                  for="integrate-language-input"
                  class="text-xs font-medium text-gray-600 dark:text-gray-300"
                >
                  {{ t('videoModels.playground.integrateLanguageLabel') }}
                </label>
                <div class="min-w-[180px]">
                  <Select
                    id="integrate-language-input"
                    v-model="integrateLanguage"
                    :options="integrateLanguageOptions"
                    size="sm"
                    creatable
                    searchable
                    :placeholder="t('videoModels.playground.integrateLanguagePlaceholder')"
                  />
                </div>
              </div>
              <!-- prompt 展示 + 复制。与 curl 保持同款：右上角悬浮复制按钮。 -->
              <div class="relative">
                <button
                  type="button"
                  class="absolute right-1.5 top-1.5 z-10 rounded border border-white/10 bg-white/10 px-1.5 py-0.5 text-[10px] font-medium text-gray-100 backdrop-blur-sm transition hover:bg-white/20"
                  @click="copyIntegratePrompt"
                  :title="t('videoModels.playground.integrateCopy')"
                >
                  {{ t('videoModels.playground.integrateCopy') }}
                </button>
                <pre
                  class="max-h-96 overflow-auto rounded-md bg-gray-900 p-3 pr-20 text-xs leading-relaxed text-gray-100 dark:bg-black whitespace-pre-wrap"
                ><code>{{ integratePrompt }}</code></pre>
              </div>
            </div>
          </details>

          <!-- 操作按钮栏
               左侧：费用信息（提交前=预估，任务完成后=实扣）
               右侧：cancel / reset / submit 按钮
          -->
          <div class="flex items-center justify-between gap-3 border-t border-gray-200 pt-3 dark:border-gray-800">
            <div class="min-w-0 space-y-0.5">
              <!-- 已完成任务且拿到 final_cost：展示"实扣" -->
              <template v-if="playground.phase.value === 'completed' && actualCost !== null">
                <div class="flex items-center gap-1.5 text-sm">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('videoModels.playground.actualCost') }}</span>
                  <span class="font-mono font-semibold text-emerald-600 dark:text-emerald-400">
                    ${{ actualCost.toFixed(4) }}
                  </span>
                </div>
                <div class="text-[10px] text-gray-400 dark:text-gray-500">
                  {{ t('videoModels.playground.actualCostHint') }}
                </div>
              </template>
              <!-- 其他情况：展示"预估"（含公式；无法算时提示"无法预估"） -->
              <template v-else>
                <div class="flex flex-wrap items-center gap-1.5 text-sm">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('videoModels.playground.estimatedCost') }}</span>
                  <template v-if="estimateBreakdown">
                    <span class="font-mono font-semibold text-blue-600 dark:text-blue-400">
                      ~${{ estimateBreakdown.total.toFixed(4) }}
                    </span>
                    <!-- 计算公式：resolution 单价 × 时长 = 预估
                         使用小灰字，展示到 4 位小数保持视觉一致。 -->
                    <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400">
                      = ${{ estimateBreakdown.unitPricePerSecond.toFixed(4) }}/s
                      ({{ estimateBreakdown.resolution }})
                      × {{ estimateBreakdown.durationSeconds }}s
                      <span v-if="estimateBreakdown.isAutoDuration" class="ml-1 rounded bg-amber-50 px-1 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-950 dark:text-amber-300">
                        {{ t('videoModels.playground.estimatedAutoBadge', { n: AUTO_DURATION_FALLBACK_SECONDS }) }}
                      </span>
                    </span>
                  </template>
                  <span v-else class="text-xs text-gray-400 dark:text-gray-500">
                    {{ t('videoModels.playground.estimatedCostUnknown') }}
                  </span>
                </div>
                <div class="text-[10px] text-gray-400 dark:text-gray-500">
                  {{ t('videoModels.playground.estimatedCostHint') }}
                </div>
                <!-- 冻结/防攻击说明：让用户理解为什么要"先冻结再执行" -->
                <div class="text-[10px] text-gray-400 dark:text-gray-500">
                  {{ t('videoModels.playground.estimatedHoldHint') }}
                </div>
              </template>
            </div>
            <div class="flex items-center gap-2">
              <button
                v-if="playground.isBusy.value"
                @click="onCancel"
                class="btn btn-secondary"
              >
                {{ t('videoModels.playground.btnCancel') }}
              </button>
              <button
                v-if="playground.isTerminal.value"
                @click="playground.reset"
                class="btn btn-secondary"
              >
                {{ t('videoModels.playground.btnReset') }}
              </button>
              <button
                @click="onSubmit"
                :disabled="!canSubmit"
                class="btn btn-primary"
              >
                {{ t('videoModels.playground.btnSubmit') }}
              </button>
            </div>
          </div>
        </section>

        <!-- ================== 右栏：结果区 ================== -->
        <section class="card p-5 space-y-3 lg:col-span-5">
          <h2 class="text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('videoModels.playground.resultTitle') }}
          </h2>

          <!-- 进行中动画：submitting / queued / running 阶段展示，替代默认预览，
               避免用户看到 default_params 的示例视频误以为任务已完成。
               排版逻辑：把 loading 卡片放在 primaryPreview 之前，并在 primaryPreview 上
               追加 `!playground.isBusy.value` 条件保证互斥；已完成后自然显示真实结果。 -->
          <div
            v-if="playground.isBusy.value"
            class="flex flex-col items-center justify-center gap-3 rounded border border-dashed border-blue-300 bg-blue-50/50 py-16 dark:border-blue-800 dark:bg-blue-950/20"
          >
            <!-- Tailwind 内置的旋转 spinner；double-border 圆环 + transparent top 制造转动效果 -->
            <svg
              class="h-10 w-10 animate-spin text-blue-500 dark:text-blue-400"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              aria-hidden="true"
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
                d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"
              ></path>
            </svg>
            <div class="text-sm font-medium text-blue-700 dark:text-blue-300">
              {{ inflightPhaseLabel }}
              <span class="ml-1 font-mono text-xs text-blue-500 dark:text-blue-400">
                {{ playground.displayElapsed.value }}s
              </span>
            </div>
            <div
              v-if="inflightQueuePosition !== null"
              class="text-xs text-blue-600/80 dark:text-blue-300/70"
            >
              {{ t('videoModels.playground.queuePosition', { pos: inflightQueuePosition }) }}
            </div>
            <div class="text-[11px] text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.pollingHint') }}
            </div>
            <!-- 历史记录提示：让用户知道即使切走、刷新或稍后回访也不会丢失当前请求；
                 复用页面顶部已存在的 "历史记录" Tab 语义，无需额外跳转按钮。 -->
            <div class="max-w-[80%] text-center text-[11px] text-gray-400 dark:text-gray-500">
              {{ t('videoModels.playground.inflightHistoryHint') }}
            </div>
          </div>

          <!-- 主结果预览：始终展示在右上角
               取值优先级：payload > default_params。
               resultType 决定用 <video> 还是 <img>；无 URL 时不渲染，避免空占位。
               需求变更：去掉左上角文案标题；只保留右上角来源角标：
                 - 任务已完成（payload 已到）→ "请求结果"
                 - 尚未提交/进行中，仅用默认参数占位 → "示例（来自默认参数）"
               追加：进行中阶段（isBusy=true）时不渲染，让位给上方的 loading 卡片。
          -->
          <div v-if="primaryPreview && !playground.isBusy.value" class="space-y-2">
            <div class="flex items-center justify-end gap-2">
              <span
                v-if="primaryPreview.source === 'payload'"
                class="rounded bg-green-100 px-1.5 py-0.5 text-[10px] font-medium text-green-700 dark:bg-green-950 dark:text-green-300"
              >
                {{ t('videoModels.playground.previewFromResult') }}
              </span>
              <span
                v-else
                class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-gray-800 dark:text-gray-400"
              >
                {{ t('videoModels.playground.previewFromDefault') }}
              </span>
            </div>
            <div v-if="resultType === 'video'" class="mx-auto w-fit max-w-full">
              <video
                :src="primaryPreview.url"
                controls
                preload="metadata"
                class="block h-auto max-h-[520px] max-w-full rounded border border-gray-200 bg-black dark:border-gray-700"
              />
            </div>
            <div
              v-if="resultType === 'video' && primaryPreview.source === 'payload' && playground.phase.value === 'completed'"
              class="flex flex-wrap justify-start gap-2"
            >
              <button
                type="button"
                class="inline-flex items-center gap-1.5 rounded border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200 dark:hover:bg-gray-800"
                :disabled="downloadingVideoURLs.has(primaryPreview.url)"
                @click="downloadVideo(primaryPreview.url)"
              >
                <Icon name="download" size="xs" />
                {{ downloadingVideoURLs.has(primaryPreview.url) ? t('videoModels.playground.downloadingVideo') : t('videoModels.playground.downloadVideo') }}
              </button>
              <button
                type="button"
                class="rounded bg-blue-600 px-3 py-1.5 text-xs font-medium text-white shadow-sm hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-blue-500 dark:hover:bg-blue-600"
                :disabled="savingMaterialURLs.has(primaryPreview.url) || savedMaterialURLs.has(primaryPreview.url)"
                @click="saveVideoToMaterials(primaryPreview.url)"
              >
                {{ savedMaterialURLs.has(primaryPreview.url) ? t('videoModels.playground.savedToMaterials') : savingMaterialURLs.has(primaryPreview.url) ? t('videoModels.playground.savingToMaterials') : t('videoModels.playground.saveToMaterials') }}
              </button>
            </div>
            <img
              v-else
              :src="primaryPreview.url"
              :alt="primaryPreview.source === 'payload' ? t('videoModels.playground.previewFromResult') : t('videoModels.playground.previewFromDefault')"
              loading="lazy"
              class="w-full max-h-[520px] rounded border border-gray-200 object-contain dark:border-gray-700"
            />
            <a
              :href="primaryPreview.url"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-block break-all text-xs text-blue-600 hover:underline dark:text-blue-400"
            >
              {{ primaryPreview.url }}
            </a>
          </div>

          <!-- idle：无任何任务 -->
          <div
            v-if="playground.phase.value === 'idle'"
            class="rounded border border-dashed border-gray-300 bg-gray-50 p-8 text-center text-sm text-gray-500 dark:border-gray-700 dark:bg-gray-950/50"
          >
            {{ t('videoModels.playground.idleHint') }}
          </div>

          <template v-else>
            <!-- 只保留 request_id + 错误原因；状态徽章/耗时/队列位置/非 primary 输出（含 seed 等）全部下线，
                 用户如需查看细节请切到"输出结构 → 原始 payload"或历史记录详情。 -->
            <div
              v-if="playground.requestId.value"
              class="truncate font-mono text-xs text-gray-500 dark:text-gray-400"
            >
              request_id: {{ playground.requestId.value }}
            </div>

            <div
              v-if="playground.errorMessage.value"
              class="rounded bg-red-50 p-2 text-xs text-red-700 dark:bg-red-950/40 dark:text-red-300"
            >
              {{ playground.errorMessage.value }}
            </div>

            <!-- resolvedOutputs——仅在"没有 primary 预览 且 有非 primary 输出"时兜底展示，
                 避免视频类主输出的 seed 等冗余字段占据主视图。 -->
            <div
              v-if="playground.phase.value === 'completed' && !primaryPreview && nonPrimaryOutputs.length"
              class="space-y-4"
            >
              <div
                v-for="(item, idx) in nonPrimaryOutputs"
                :key="idx"
                class="space-y-1"
              >
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-xs font-medium text-gray-600 dark:text-gray-400">
                    {{ item.spec.label || item.spec.key }}
                  </span>
                  <span class="font-mono text-[10px] text-gray-400">
                    {{ item.effectiveType }}<span v-if="item.isPrimary"> · primary</span>
                  </span>
                </div>

                <div class="space-y-2">
                  <template v-for="(leaf, j) in item.values" :key="j">
                    <!-- video -->
                    <div v-if="item.effectiveType === 'video' && leafToUrl(leaf)" class="space-y-1">
                      <video
                        :src="leafToUrl(leaf)"
                        controls
                        preload="metadata"
                        :class="[
                          'mx-auto block h-auto max-w-full rounded border border-gray-200 bg-black dark:border-gray-700',
                          item.isPrimary ? 'max-h-[520px]' : 'max-h-[240px]'
                        ]"
                      />
                      <a
                        :href="leafToUrl(leaf)"
                        target="_blank"
                        rel="noopener noreferrer"
                        class="inline-block break-all text-xs text-blue-600 hover:underline dark:text-blue-400"
                      >
                        {{ leafToUrl(leaf) }}
                      </a>
                    </div>

                    <!-- image -->
                    <div v-else-if="item.effectiveType === 'image' && leafToUrl(leaf)" class="space-y-1">
                      <img
                        :src="leafToUrl(leaf)"
                        :alt="item.spec.label || item.spec.key"
                        loading="lazy"
                        :class="[
                          'w-full rounded border border-gray-200 object-contain dark:border-gray-700',
                          item.isPrimary ? 'max-h-[520px]' : 'max-h-[240px]'
                        ]"
                      />
                      <a
                        :href="leafToUrl(leaf)"
                        target="_blank"
                        rel="noopener noreferrer"
                        class="inline-block break-all text-xs text-blue-600 hover:underline dark:text-blue-400"
                      >
                        {{ leafToUrl(leaf) }}
                      </a>
                    </div>

                    <!-- url -->
                    <a
                      v-else-if="item.effectiveType === 'url' && leafToUrl(leaf)"
                      :href="leafToUrl(leaf)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="block break-all rounded bg-gray-50 px-2 py-1 text-xs text-blue-600 hover:underline dark:bg-gray-900 dark:text-blue-400"
                    >
                      {{ leafToUrl(leaf) }}
                    </a>

                    <!-- object / array / 遗留 json：预格式化 -->
                    <pre
                      v-else-if="item.effectiveType === 'object' || item.effectiveType === 'array' || item.effectiveType === 'json'"
                      class="max-h-64 overflow-auto rounded bg-gray-900 p-2 text-xs text-gray-100"
                    >{{ leafToPrettyJson(leaf) }}</pre>

                    <!-- text / number -->
                    <div
                      v-else
                      class="break-all rounded bg-gray-50 px-2 py-1 text-xs text-gray-800 dark:bg-gray-900 dark:text-gray-200"
                    >
                      {{ leafToText(leaf) }}
                    </div>
                  </template>

                  <p
                    v-if="item.spec.description"
                    class="text-[11px] text-gray-500 dark:text-gray-400"
                  >
                    {{ item.spec.description }}
                  </p>
                </div>
              </div>
            </div>

            <!-- 已完成但没有任何可展示内容时的兜底提示（primary 与非 primary 都为空） -->
            <div
              v-else-if="playground.phase.value === 'completed' && !primaryPreview"
              class="text-xs text-gray-600 dark:text-gray-400"
            >
              {{ outputFields.length
                ? t('videoModels.playground.noOutputMatched')
                : t('videoModels.playground.noOutputConfigured') }}
            </div>
          </template>

          <!-- ================== 输出区（内嵌）==================
               依然在右栏结果卡片内部，与左栏输入参数对齐。
               四栏内部右上采用 Tab 切换：
                 - schema Tab：递归展示 output_fields 声明的结构 + 当前值
                 - payload Tab：展示 fal 上游原始 result payload JSON（代替之前任务结果内的 <details> 折叠块）
               payload Tab 在无请求结果时展示 output_fields 中配置的示例默认值。
          -->
          <div
            v-if="outputSchemaNodes.length > 0 || playground.resultPayload.value"
            class="mt-4 space-y-3 border-t border-dashed border-gray-200 pt-3 dark:border-gray-800"
          >
            <div class="flex items-center justify-between gap-2">
              <h3 class="text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ t('videoModels.playground.outputSchemaTitle') }}
              </h3>
              <div class="flex items-center gap-1">
                <button
                  v-if="outputSchemaNodes.length > 0"
                  type="button"
                  @click="outputTab = 'schema'"
                  :class="miniTabClass(outputTab === 'schema')"
                >
                  {{ t('videoModels.playground.outputTabSchema') }}
                </button>
                <button
                  type="button"
                  @click="outputTab = 'payload'"
                  :class="miniTabClass(outputTab === 'payload')"
                >
                  {{ t('videoModels.playground.outputTabPayload') }}
                </button>
              </div>
            </div>
            <div v-if="outputTab === 'schema' && outputSchemaNodes.length > 0" class="space-y-3">
              <div
                v-for="spec in outputFields"
                :key="spec.key"
                class="space-y-1.5"
              >
                <div class="flex items-center gap-2">
                  <span
                    class="rounded px-1.5 py-0.5 text-[10px] font-medium"
                    :class="{
                      'bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300':
                        outputValueSourceFor(spec) === 'payload',
                      'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400':
                        outputValueSourceFor(spec) !== 'payload',
                    }"
                  >
                    {{
                      outputValueSourceFor(spec) === 'payload'
                        ? t('videoModels.playground.outputValueFromPayload')
                        : outputValueSourceFor(spec) === 'example'
                        ? t('videoModels.playground.outputValueFromExample')
                        : t('videoModels.playground.outputValueNone')
                    }}
                  </span>
                </div>
                <OutputSchemaValueTree
                  :node="outputSchemaNodes[outputFields.indexOf(spec)]"
                  :value="outputValueFor(spec)"
                />
              </div>
            </div>
            <div v-else-if="outputTab === 'payload'">
              <div class="mb-2 flex justify-end">
                <span
                  class="rounded px-1.5 py-0.5 text-[10px] font-medium"
                  :class="playground.resultPayload.value
                    ? 'bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300'
                    : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'"
                >
                  {{ playground.resultPayload.value
                    ? t('videoModels.playground.outputValueFromPayload')
                    : t('videoModels.playground.outputValueFromExample') }}
                </span>
              </div>
              <pre class="max-h-96 overflow-auto rounded bg-gray-900 p-2 text-xs text-gray-100">{{ payloadForOutputTab }}</pre>
            </div>
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import VideoPlaygroundSchemaField from '@/components/video/VideoPlaygroundSchemaField.vue'
import VideoPlaygroundHistory from '@/components/video/VideoPlaygroundHistory.vue'
import OutputSchemaValueTree, {
  type SchemaNode,
} from '@/components/video/OutputSchemaValueTree.vue'
import videoModelsAPI, {
  type OutputFieldSpec,
  type VideoModelItem,
} from '@/api/videoModels'
import keysAPI from '@/api/keys'
import userGroupsAPI from '@/api/groups'
import userMaterialsAPI from '@/api/userMaterials'
import type { ApiKey, Group } from '@/types'
import { buildGatewayUrl } from '@/api/url'
import { useAppStore } from '@/stores/app'
import { useVideoPlayground } from '@/composables/useVideoPlayground'
import {
  buildDefaultBody,
  extractFieldSpecs,
  fieldSpecToDefaultValue,
  pickByPath,
  type FieldSpec,
} from '@/components/video/paramSpec'
import { normalizeMediaUrlWidget } from '@/utils/mediaUrlWidget'
import { collectPromptMediaReferences } from '@/components/video/promptMediaReferences'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const playground = useVideoPlayground()
const downloadingVideoURLs = reactive(new Set<string>())
const savingMaterialURLs = reactive(new Set<string>())
const savedMaterialURLs = reactive(new Set<string>())

function videoDownloadFileName(url: string): string {
  try {
    const parsed = new URL(url, window.location.href)
    const segment = parsed.pathname.split('/').filter(Boolean).pop()
    if (segment) return decodeURIComponent(segment)
  } catch {
    // Use the stable fallback below for malformed or non-standard URLs.
  }
  return `video-${Date.now()}.mp4`
}

async function downloadVideo(url: string) {
  const normalized = url.trim()
  if (!normalized || downloadingVideoURLs.has(normalized)) return
  downloadingVideoURLs.add(normalized)
  try {
    const response = await fetch(normalized)
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const blobURL = URL.createObjectURL(await response.blob())
    const link = document.createElement('a')
    link.href = blobURL
    link.download = videoDownloadFileName(normalized)
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(blobURL)
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error)
    appStore.showError(t('videoModels.playground.downloadVideoFailed', { msg: message }))
  } finally {
    downloadingVideoURLs.delete(normalized)
  }
}

async function saveVideoToMaterials(url: string) {
  const normalized = url.trim()
  if (!normalized || savingMaterialURLs.has(normalized) || savedMaterialURLs.has(normalized)) return
  savingMaterialURLs.add(normalized)
  try {
    await userMaterialsAPI.importFromUrl(normalized)
    savedMaterialURLs.add(normalized)
    appStore.showSuccess(t('videoModels.playground.saveToMaterialsSuccess'))
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error)
    appStore.showError(t('videoModels.playground.saveToMaterialsFailed', { msg: message }))
  } finally {
    savingMaterialURLs.delete(normalized)
  }
}

// ============ slug 与模型加载 ============
// 路由用 pathMatch (:slug(.*)+)。vue-router 会把它作为 route.params.slug（string 或 string[]）。
// slug 可能含 "/"，比如 "bytedance/seedance-2.5/text-to-video"。
const slug = computed<string>(() => {
  const raw = route.params.slug
  if (Array.isArray(raw)) return raw.join('/')
  return String(raw || '')
})

const modelLoading = ref(false)
const modelLoaded = ref(false)
const model = ref<VideoModelItem | null>(null)

const displaySlug = computed(() =>
  slug.value.startsWith('fal-ai/') ? slug.value.slice('fal-ai/'.length) : slug.value
)

// headerTitle：顶部大标题优先显示 display_name（"模型是什么"更直观），
// 未加载/未命中时回退到 displaySlug；两者都为空时兜底为国际化的"演练台"。
const headerTitle = computed(() => {
  const name = model.value?.display_name?.trim()
  if (name) return name
  if (displaySlug.value) return displaySlug.value
  return t('videoModels.playground.title')
})

// showPricingDialog：控制"价格表"弹窗开关，纯 UI 状态。
const showPricingDialog = ref(false)

/**
 * vendorTag / familyTag：从 displaySlug 拆出的两级分类标签。
 *   例："bytedance/seedance-2.5/text-to-video"
 *        → vendor = "bytedance"，family = "seedance-2.5"
 * 与 VideoModelsView 的 getVendor/getFamily 保持完全一致的切分口径，
 * 保证点击 chip 后目标页能匹配到相同的过滤值。
 * 说明：displaySlug 已经把可能的 "fal-ai/" 前缀剥掉，所以第一段就是 vendor。
 */
const vendorTag = computed<string>(() => {
  const parts = displaySlug.value.split('/').filter(Boolean)
  return parts[0] || ''
})
const familyTag = computed<string>(() => {
  const parts = displaySlug.value.split('/').filter(Boolean)
  return parts[1] || ''
})

/**
 * jumpToVideoModelsByTag：跳到视频模型列表页，并把某一级 tag 作为初始过滤值。
 *   - kind='vendor' → 只带 vendor query，family 由用户在目标页再选
 *   - kind='family' → 同时带 vendor+family（因为 family 通常隶属某个 vendor，
 *     没有 vendor 上下文时目标页的 familyOptions 是从当前 vendor 计算出来的，
 *     所以必须一起带过去，否则 family 选中态会因为 options 未包含而被回落成 'all'）
 * 目标页 VideoModelsView 在 onMounted / items 加载后会读取 query 并同步过滤器。
 */
function jumpToVideoModelsByTag(kind: 'vendor' | 'family', tag: string) {
  if (!tag) return
  const query: Record<string, string> = { vendor: vendorTag.value || tag }
  if (kind === 'family') {
    query.family = tag
  }
  router.push({ name: 'VideoModels', query })
}

async function loadModel() {
  if (!slug.value) {
    modelLoaded.value = false
    return
  }
  modelLoading.value = true
  try {
    const { data } = await videoModelsAPI.list()
    const found = (data.items || []).find((m) => m.slug === slug.value) || null
    model.value = found
    modelLoaded.value = !!found
  } catch {
    modelLoaded.value = false
    model.value = null
  } finally {
    modelLoading.value = false
  }
}

// ============ 派生：字段声明 / 输出字段声明 ============
const fieldSpecs = computed<FieldSpec[]>(() =>
  extractFieldSpecs(
    model.value?.intro?.default_params &&
      Object.keys(model.value.intro.default_params).length > 0
      ? model.value.intro.default_params
      : null
  )
)

// primaryFieldSpecs / advancedFieldSpecs：按管理员声明的 spec.advanced
// 把顶层字段拆成两组渲染。默认展开 primary 组；advanced 组塞进右下的
// <details> 折叠区，避免大量“高阶参数”铺满整个左栏干扰新手。
// 注：拆分只在顶层生效——object.children 内部子字段的 advanced 语义交给
// 演练台内部渲染器判断即可，这里不递归展开。
const primaryFieldSpecs = computed<FieldSpec[]>(() =>
  fieldSpecs.value.filter((f) => !f.advanced)
)
const advancedFieldSpecs = computed<FieldSpec[]>(() =>
  fieldSpecs.value.filter((f) => f.advanced)
)

const outputFields = computed<OutputFieldSpec[]>(() =>
  Array.isArray(model.value?.intro?.output_fields)
    ? (model.value!.intro!.output_fields as OutputFieldSpec[])
    : []
)

const resultField = computed<string>(() => model.value?.intro?.result_field || '')
const resultType = computed<'video' | 'image'>(() =>
  model.value?.intro?.result_type === 'image' ? 'image' : 'video'
)

const defaultParamsForCurl = computed<Record<string, unknown> | null>(() =>
  model.value?.intro?.default_params &&
    Object.keys(model.value.intro.default_params).length > 0
    ? model.value.intro.default_params
    : null
)

// ============ API Keys 加载 ============
const keysLoading = ref(false)
const allKeys = ref<ApiKey[]>([])
const selectedKeyId = ref<number | ''>('')

const compositeKeys = computed(() =>
  allKeys.value.filter((k) => k.group?.platform === 'composite' && k.status === 'active')
)

const selectedKey = computed<ApiKey | null>(() => {
  if (selectedKeyId.value === '') return null
  return compositeKeys.value.find((k) => k.id === selectedKeyId.value) ?? null
})

// keyOptions：给通用 Select 用。value 为 key.id（number）；label 展示"名称 · 分组 · 掩码 key"。
const keyOptions = computed<SelectOption[]>(() =>
  compositeKeys.value.map((k) => ({
    value: k.id,
    label: `${k.name} · ${k.group?.name || '-'} · ${maskKey(k.key)}`,
  }))
)

async function loadKeys() {
  keysLoading.value = true
  try {
    const resp = await keysAPI.list(1, 100)
    allKeys.value = resp.items ?? []
    if (!selectedKeyId.value && compositeKeys.value.length) {
      selectedKeyId.value = compositeKeys.value[0].id
    }
  } catch {
    allKeys.value = []
  } finally {
    keysLoading.value = false
  }
}

function maskKey(key: string): string {
  if (!key) return '-'
  if (key.length <= 10) return key
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}

// ============ 快速创建 API 密钥 ============
// 没有可用密钥时，演练台整个提交按钮都是禁用的，等于死路。让用户跳去"API 密钥"
// 页创建再回来，已填的参数（prompt、上传好的图片组…）就全丢了。所以这里内联一个
// 只含"名称 + 分组"的最短创建路径，建完自动选中并留在当前页继续跑。
const showCreateKey = ref(false)
const createKeyName = ref('')
const createKeyGroupId = ref<number | ''>('')
const createKeySubmitting = ref(false)
const createKeyGroupsLoading = ref(false)
const availableGroups = ref<Group[]>([])

/**
 * compositeGroupOptions：可绑定的 composite 分组。
 * 与 compositeKeys 的口径保持一致 —— 视频门面只接受 composite 分组下的密钥，
 * 因此这里也只列 composite 且 active 的分组，避免用户建出一把用不了的 key。
 */
const compositeGroupOptions = computed<SelectOption[]>(() =>
  availableGroups.value
    .filter((g) => g.platform === 'composite' && g.status === 'active')
    .map((g) => ({ value: g.id, label: g.name }))
)

/** openCreateKey：打开弹窗并按需拉取一次可用分组（懒加载，正常有 key 的用户不会付这次请求）。 */
async function openCreateKey() {
  showCreateKey.value = true
  createKeyName.value = t('videoModels.playground.createKeyDefaultName')
  if (availableGroups.value.length === 0) {
    createKeyGroupsLoading.value = true
    try {
      availableGroups.value = await userGroupsAPI.getAvailable()
    } catch {
      availableGroups.value = []
    } finally {
      createKeyGroupsLoading.value = false
    }
  }
  // 默认选中第一个可用分组：多数用户只有一个，省一步点击。
  if (createKeyGroupId.value === '' && compositeGroupOptions.value.length) {
    createKeyGroupId.value = Number(compositeGroupOptions.value[0].value)
  }
}

/** goKeysPage：跳到"API 密钥"页并自动打开创建弹窗（复用 KeysView 的 openCreate query）。 */
function goKeysPage() {
  const query: Record<string, string> = { openCreate: '1' }
  if (createKeyGroupId.value !== '') query.group_id = String(createKeyGroupId.value)
  router.push({ path: '/keys', query })
}

/**
 * submitCreateKey：创建后刷新列表并选中新 key。
 *
 * 这里显式把 selectedKeyId 指向新 key，而不是依赖 loadKeys 里的
 * "没选中就选第一个"兜底 —— 那个兜底取的是 compositeKeys[0]，跟创建顺序无关，
 * 用户刚建的 key 未必排在首位。
 */
async function submitCreateKey() {
  const name = createKeyName.value.trim()
  if (!name || createKeyGroupId.value === '' || createKeySubmitting.value) return
  const groupId = Number(createKeyGroupId.value)
  createKeySubmitting.value = true
  try {
    const created = await keysAPI.create(name, groupId)
    showCreateKey.value = false
    await loadKeys()
    if (created?.id) {
      // 创建接口返回的 DTO 只有 group_id，没有嵌套的 group 对象，而 compositeKeys
      // 的过滤依赖 k.group?.platform。正常情况上面 loadKeys 拉回的列表里就带了；
      // 但 list 只取前 100 条，key 很多的用户可能捞不到刚建的这把。这种情况用本地
      // 已知的分组信息补一条，避免"创建成功却选不上"。
      if (!allKeys.value.some((k) => k.id === created.id)) {
        const g = availableGroups.value.find((x) => x.id === groupId)
        allKeys.value = [{ ...created, group: g } as ApiKey, ...allKeys.value]
      }
      selectedKeyId.value = created.id
    }
    appStore.showSuccess(t('videoModels.playground.createKeySuccess'))
  } catch (e: unknown) {
    const msg =
      (e as { response?: { data?: { message?: string } } })?.response?.data?.message ||
      (e as { message?: string })?.message ||
      'unknown error'
    appStore.showError(t('videoModels.playground.createKeyFailed', { msg }))
  } finally {
    createKeySubmitting.value = false
  }
}

// ============ 表单值 ============
const formData = reactive<Record<string, unknown>>({})
const promptMediaReferences = computed(() =>
  collectPromptMediaReferences(fieldSpecs.value, formData)
)

function initFormDefaults() {
  for (const k of Object.keys(formData)) delete formData[k]
  for (const f of fieldSpecs.value) {
    formData[f.key] = fieldSpecToDefaultValue(f)
  }
}

function onFieldValueChange(key: string, v: unknown) {
  if (v === undefined) {
    delete formData[key]
  } else {
    formData[key] = v
  }
}

// 模式：form / json
const mode = ref<'form' | 'json'>('form')
const jsonInput = ref<string>('')
const jsonError = ref<string>('')

// ============ 顶部 Tab（Playground / 历史记录） ============
// 独立于 mode（表单/JSON）之上的一层导航；从"演练台"切到"历史记录"时不销毁
// playground 状态（仅切换视图），这样用户回到 playground 页面可以继续查看当前
// 任务进度。历史 tab 首次进入时才发起 API 请求（懒加载）。
type TopTab = 'playground' | 'history'
const topTab = ref<TopTab>('playground')
const historyTotal = ref(0)

// 输出结构区域内的小 Tab（schema / payload）：
//   - schema：递归展开 output_fields 的结构 + 当前值
//   - payload：fal 上游 result_payload 原文（未完成任务展示为空提示）
type OutputTab = 'schema' | 'payload'
const outputTab = ref<OutputTab>('schema')

function topTabClass(active: boolean): string {
  return [
    '-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors',
    active
      ? 'border-blue-600 text-blue-700 dark:border-blue-400 dark:text-blue-300'
      : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200',
  ].join(' ')
}

function miniTabClass(active: boolean): string {
  return [
    'rounded px-2 py-0.5 text-[11px] font-medium transition-colors',
    active
      ? 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
      : 'text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800',
    // disabled 只对 payload 生效
    'disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent',
  ].join(' ')
}

function onSwitchToHistory() {
  topTab.value = 'history'
}

// onReplayTask：从历史卡片点"重放"时，将该历史任务的 request_payload 塞回演练台
// 并自动切回 Playground tab。空 payload 忽略。
//
// 交互目标：重放优先落在「表单」tab（用户所见即所改），因此这里按 fieldSpecs
// 把 payload 展开到 formData：
//   - payload 里存在的 key → 覆盖 formData[key]
//   - fieldSpecs 里存在但 payload 未提供的 key → 回落到该字段的默认值，避免残留上次运行的脏值
//   - payload 里存在但 fieldSpecs 不认识的 key → 忽略（表单没有位置承载它，需要用 JSON 模式）
// 同步把 jsonInput 也刷新一份，用户切到 JSON tab 也能看到完整原始 payload。
function onReplayTask(payload: Record<string, unknown> | null) {
  if (!payload || typeof payload !== 'object') return
  // 1) 按 fieldSpecs 覆盖/回填 formData
  for (const f of fieldSpecs.value) {
    if (Object.prototype.hasOwnProperty.call(payload, f.key)) {
      formData[f.key] = payload[f.key]
    } else {
      formData[f.key] = fieldSpecToDefaultValue(f)
    }
  }
  // 2) 同步 JSON 输入，保留完整原始 payload（含 fieldSpecs 之外的自定义字段）
  jsonInput.value = JSON.stringify(payload, null, 2)
  jsonError.value = ''
  // 3) 切回「表单」tab + Playground 顶层 tab
  mode.value = 'form'
  topTab.value = 'playground'
}

function switchToJsonMode() {
  if (mode.value === 'json') return
  if (fieldSpecs.value.length > 0) {
    jsonInput.value = JSON.stringify(currentFormBody(), null, 2)
  } else if (!jsonInput.value) {
    const fallback = buildDefaultBody(defaultParamsForCurl.value)
    jsonInput.value = JSON.stringify(
      Object.keys(fallback).length > 0 ? fallback : { prompt: 'A cinematic shot ...' },
      null,
      2
    )
  }
  jsonError.value = ''
  mode.value = 'json'
}

function tabClass(active: boolean): string {
  return [
    'rounded px-3 py-1.5 text-sm',
    active
      ? 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
      : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800',
  ].join(' ')
}

function currentFormBody(): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  for (const f of fieldSpecs.value) {
    const v = formData[f.key]
    if (v === undefined) continue
    if (typeof v === 'string' && v.trim() === '') continue
    // 空数组视为"未填"：媒体 URL 组初始化就是 []，
    // 原样提交会让上游收到一个无意义的空 images 参数，部分平台会直接报错。
    if (Array.isArray(v) && v.length === 0) continue
    body[f.key] = v
  }
  return body
}

// ============ 提交 / 取消 ============
const canSubmit = computed(() => {
  if (playground.isBusy.value) return false
  if (!selectedKey.value) return false
  return true
})

function validateFormRequired(): string | null {
  for (const f of fieldSpecs.value) {
    // array：先做与类型无关的元素个数上限校验（maxItems 由管理员在编辑页声明）。
    // 控件层已经禁用了"添加"，这里再兜一次，防止用户走 JSON 模式改完再切回表单。
    if (f.rawType === 'array') {
      const arr = Array.isArray(formData[f.key]) ? (formData[f.key] as unknown[]) : []
      if (f.maxItems > 0 && arr.length > f.maxItems) {
        return t('videoModels.playground.maxItemsExceeded', {
          field: f.key,
          max: f.maxItems,
          n: arr.length,
        })
      }
      // 必填的媒体组要求至少一个 URL。仅限媒体 URL 组：通用 array 的 required 历史上
      // 只作展示徽章用，这里不改变其行为，避免影响既有模型配置。
      if (f.required && normalizeMediaUrlWidget(f.widget) && arr.length === 0) {
        return t('videoModels.playground.requiredMissing', { field: f.key })
      }
      continue
    }
    if (!f.required) continue
    if (f.rawType === 'boolean') continue
    if (f.rawType === 'object') continue
    const raw = formData[f.key]
    if (raw === undefined || String(raw).trim() === '') {
      return t('videoModels.playground.requiredMissing', { field: f.key })
    }
  }
  return null
}

function currentBody(): Record<string, unknown> | null {
  if (mode.value === 'json') {
    const raw = jsonInput.value.trim()
    if (!raw) {
      jsonError.value = t('videoModels.playground.jsonEmpty')
      return null
    }
    try {
      const parsed = JSON.parse(raw)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        jsonError.value = t('videoModels.playground.jsonMustBeObject')
        return null
      }
      jsonError.value = ''
      return parsed as Record<string, unknown>
    } catch (e) {
      jsonError.value = (e as Error).message
      return null
    }
  }
  const missing = validateFormRequired()
  if (missing) {
    appStore.showError(missing)
    return null
  }
  return currentFormBody()
}

async function onSubmit() {
  const body = currentBody()
  if (!body) return
  if (!selectedKey.value) return
  await playground.start(slug.value, body, selectedKey.value.key)
}

async function onCancel() {
  if (!selectedKey.value) {
    playground.reset()
    return
  }
  await playground.cancel(slug.value, selectedKey.value.key)
}

// ============ 主结果预览（右上角） ============
// 需求：右栏顶部要固定一块"主结果预览"，用管理端配置的 result_field / result_type
// 来定位并渲染。取值优先级：
//   1) 一旦任务返回 payload，就从 payload 里按 result_field 路径 pickByPath 取值；
//   2) 否则回退到 model.intro.default_params 同一路径的值——这样一进入演练台
//      就能看到"示例视频/图片"，用户能一眼看懂这个模型主结果长什么样。
// 之所以把 URL 提取和渲染做在这个组件里而不是复用下方 resolvedOutputs 循环，是
// 因为主预览需要在 idle 阶段（还没跑任务时）也展示，与 outputs 列表的展示生命周期
// 不同；且展示体积更大、样式独立。
const primaryPreview = computed<{ url: string; source: 'payload' | 'default' } | null>(() => {
  const rf = (resultField.value || '').trim()
  if (!rf) return null

  // 通用：从一份数据里按路径取第一个可播放的 URL。
  const tryPick = (src: unknown): string => {
    if (!src) return ''
    const leaves = pickByPath(src, rf)
    for (const leaf of leaves) {
      const u = leafToUrl(leaf)
      if (u) return u
    }
    return ''
  }

  // 1) 有 payload 时优先用 payload。
  const payload = playground.resultPayload.value
  if (payload) {
    const u = tryPick(payload)
    if (u) return { url: u, source: 'payload' }
  }

  // 2) 回退：使用示例值。分两个来源：
  //
  //   a) default_params（输入参数）：
  //      形如 { video: { properties: { url: { value: '...' } } } }
  //      buildDefaultBody 已经把 schema 展平为 { video: { url: '...' } }，
  //      然后按 result_field 路径 pickByPath 就能取到 URL。
  //
  //   b) output_fields（输出参数）：
  //      **关键点**：result_field 的候选路径其实来自 output_fields，用户配置的
  //      "video.url" 通常是输出参数里的路径。而输出参数的默认值/示例值同样
  //      是以 { properties: { url: { value: '...' } } } 形态挂在 output_fields 上的。
  //      如果只查 default_params，用户会遇到"给 output_fields 的 video.url 填了
  //      默认值，但演练台右上没视频"的问题——因为路径根本不指向输入参数。
  //      这里补一份 fallback：把 output_fields 递归展平成"扁平示例对象"，再
  //      pickByPath；两份 fallback 都试，谁先命中用谁。
  const dp = model.value?.intro?.default_params
  if (dp && Object.keys(dp).length > 0) {
    const flattened = buildDefaultBody(dp)
    const u = tryPick(flattened)
    if (u) return { url: u, source: 'default' }
  }
  const ofExample = buildOutputExamplePayload(outputFields.value)
  if (ofExample && Object.keys(ofExample).length > 0) {
    const u = tryPick(ofExample)
    if (u) return { url: u, source: 'default' }
  }
  return null
})

/**
 * buildOutputExamplePayload：把 output_fields[] 递归展平为"示例 payload 对象"，
 * 供未提交任务时的兜底展示使用。
 *   - 顶层：{ [spec.key]: schemaExampleValue(spec), ... }
 *   - object：{ [child.key]: value, ... }（递归 properties）
 *   - array：[items_example]（items 示例一个元素）
 * schemaExampleValue 已经处理了这三种情况的递归；这里只做"顶层组装"。
 */
function buildOutputExamplePayload(
  specs: OutputFieldSpec[]
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const s of specs) {
    if (!s.key) continue
    const v = schemaExampleValue(s)
    if (v !== undefined) out[s.key] = v
  }
  return out
}

// ============ 结果解析（沿用弹窗版逻辑） ============
interface ResolvedOutput {
  spec: OutputFieldSpec
  values: unknown[]
  isPrimary: boolean
  effectiveType: string
}
const resolvedOutputs = computed<ResolvedOutput[]>(() => {
  const payload = playground.resultPayload.value
  if (!payload) return []
  const specs = outputFields.value
  const rf = (resultField.value || '').trim()
  const rt: 'video' | 'image' = resultType.value
  const matchedByResultField = rf.length > 0 && specs.some((s) => s.key === rf)

  let fallbackPrimaryKey = ''
  if (!matchedByResultField) {
    for (const s of specs) {
      const st = s.type as string
      if (st === 'object' || st === 'array' || st === 'video' || st === 'image') {
        fallbackPrimaryKey = s.key
        break
      }
    }
  }

  const out: ResolvedOutput[] = []
  for (const spec of specs) {
    const values = pickByPath(payload, spec.key)
    if (values.length === 0) continue
    let isPrimary = false
    let effectiveType: string = spec.type
    if (matchedByResultField && spec.key === rf) {
      isPrimary = true
      effectiveType = rt
    } else if (!matchedByResultField && spec.key === fallbackPrimaryKey) {
      isPrimary = true
    }
    out.push({ spec, values, isPrimary, effectiveType })
  }
  return out
})

// nonPrimaryOutputs：resolvedOutputs 中剔除 primary 项。
// 右上"主结果预览"已经渲染 primary 的大图/视频，下方 outputs 循环再渲染同一份
// primary 会造成重复；因此这里过滤掉。
const nonPrimaryOutputs = computed<ResolvedOutput[]>(() =>
  resolvedOutputs.value.filter((item) => !item.isPrimary)
)

function leafToText(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}
function leafToUrl(v: unknown): string {
  if (typeof v === 'string' && v) return v
  if (v && typeof v === 'object') {
    const u = (v as Record<string, unknown>).url
    if (typeof u === 'string' && u) return u
  }
  return ''
}
function leafToPrettyJson(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

// ============ curl 预览 ============
// showApiKeyInCurl：控制 curl 示例里是否把选中的 API Key 以明文形式展示。
// 默认 false —— 避免用户在没意识的情况下把包含明文密钥的截图/代码贴到公开渠道；
// 打开后即时把 {api-key} 占位符替换为真实值，方便用户直接复制到本地终端执行。
const showApiKeyInCurl = ref(false)

function curlBody(): Record<string, unknown> {
  const cur = tryPeekBody()
  if (cur && Object.keys(cur).length > 0) return cur
  const fallback = buildDefaultBody(defaultParamsForCurl.value)
  if (Object.keys(fallback).length > 0) return fallback
  return { prompt: 'A cinematic shot of a corgi surfing on a rainbow.' }
}
function tryPeekBody(): Record<string, unknown> | null {
  if (mode.value === 'json') {
    const raw = jsonInput.value.trim()
    if (!raw) return null
    try {
      const parsed = JSON.parse(raw)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>
      }
      return null
    } catch {
      return null
    }
  }
  if (fieldSpecs.value.length === 0) return null
  return currentFormBody()
}
const curlSnippet = computed(() => {
  // 优先使用系统设置中配置的 API 端点（appStore.apiBaseUrl），
  // 保证示例与用户实际对外暴露的网关地址一致；未配置时回落到当前页面 origin。
  const url = `${resolvedApiBase.value}/api/v1/model/${slug.value}`
  // 默认不泄露 API Key。仅当用户主动打开旁边的 "在示例中显示 API Key"
  // toggle，且已选定 key 时，才拼接真实密钥。其他情况一律用 {api-key} 占位符，
  // 避免用户直接复制后致密钥意外泄露（截图、共享代码等场景）。
  const keyPart =
    showApiKeyInCurl.value && selectedKey.value ? selectedKey.value.key : '{api-key}'
  const body = JSON.stringify(curlBody(), null, 2)
  const indented = body
    .split('\n')
    .map((line, i) => (i === 0 ? line : '  ' + line))
    .join('\n')
  return [
    `curl -X POST '${url}' \\`,
    `  -H 'Authorization: Bearer ${keyPart}' \\`,
    `  -H 'Content-Type: application/json' \\`,
    `  -d '${indented}'`,
  ].join('\n')
})

async function copyCurl() {
  try {
    await navigator.clipboard.writeText(curlSnippet.value)
    appStore.showSuccess(t('videoModels.copied'))
  } catch {
    appStore.showError(t('videoModels.copyFailed'))
  }
}

// ============ 接入到程序中的 prompt ============
// 交互目的：让用户把 prompt 一键喂给 AI 助手（Cursor / Copilot / Claude / Codex 等），
// 得到一份可运行的接入代码。为此 prompt 需要包含：
//   1) 目标语言（下拉+自由输入，locale 决定文案语种）；
//   2) 实时 curl 快照（curlSnippet），让 AI 直接看到 URL / header / body 全貌；
//   3) 网关 base URL（从系统设置读取）与 slug（用于说明轮询/查询接口）；
//   4) 输出字段 schema（模型声明的 output_fields），帮 AI 知道如何从 payload 里取值。
// 不暴露上游厂商信息（不提 fal）。
// 语言默认给个通用值，避免第一次进来 prompt 里出现空 <你的语言> 占位。
const integrateLanguage = ref<string | number | boolean | null>('Python')

// 下拉选项：与项目其他 Select 一致的 SelectOption 结构；creatable 允许用户自己键入。
const integrateLanguageOptions = computed<SelectOption[]>(() => [
  { value: 'Python', label: 'Python' },
  { value: 'Node.js', label: 'Node.js' },
  { value: 'TypeScript', label: 'TypeScript' },
  { value: 'Go', label: 'Go' },
  { value: 'Java', label: 'Java' },
  { value: 'Rust', label: 'Rust' },
  { value: 'C#', label: 'C#' },
  { value: 'PHP', label: 'PHP' },
  { value: 'Ruby', label: 'Ruby' },
  { value: 'cURL', label: 'cURL' },
])

// resolvedApiBase：优先使用系统设置里配置的 API 端点（appStore.apiBaseUrl）；
// 未配置时回落到当前页面 origin，保证 curl 示例、接入 prompt 里的地址与
// “提交端点”展示位都一致，避免用户拿到的 URL 与实际可用地址不一致。
const resolvedApiBase = computed(() => {
  const configured = appStore.apiBaseUrl?.trim()
  if (configured) return configured.replace(/\/+$/, '')
  if (typeof window !== 'undefined') return window.location.origin.replace(/\/+$/, '')
  return buildGatewayUrl('').replace(/\/+$/, '')
})

// outputSchemaJson：把当前模型声明的 output_fields 序列化为 JSON，供 prompt 内嵌。
// 未声明时给一个提示字串，让 AI 知道可以从 payload 里自行探测。
const outputSchemaJson = computed(() => {
  const fields = outputFields.value
  if (!fields || fields.length === 0) return ''
  try {
    return JSON.stringify(fields, null, 2)
  } catch {
    return ''
  }
})

// inputSchemaJson：把管理员在“编辑模型介绍”里配置的 default_params 原始 schema
// 序列化为 JSON，作为 prompt 的“输入字段 schema”嵌入。
//
// 之所以选用原始 default_params 而不是前端派生的 fieldSpecs：
//   - default_params 就是后端下发的真身，字段名 / required / description /
//     enum / value（默认值）语义完整，AI 直接就能读懂；
//   - fieldSpecs 是为表单渲染量身派生的富对象（rawType / widget / textareaRows
//     等前端专用属性），塞进 prompt 反而噪声大。
//
// 剥离约定：
//   - 递归删除每一层的 `extra` 键（承载 x-order / advanced 等仅前端编辑/渲染用
//     的元数据，AI 生成客户端代码时用不到，反而会误以为是接口字段）；
//   - 同时兼容删除旧数据里散在顶层的 `x-order`（新写入已迁到 extra，但历史
//     数据在下次保存前仍带顶层 x-order，需要一并剥离，保持 prompt 稳定）。
function stripSchemaExtras(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(stripSchemaExtras)
  if (v && typeof v === 'object') {
    const out: Record<string, unknown> = {}
    for (const [k, val] of Object.entries(v as Record<string, unknown>)) {
      if (k === 'extra' || k === 'x-order') continue
      out[k] = stripSchemaExtras(val)
    }
    return out
  }
  return v
}

const inputSchemaJson = computed(() => {
  const params = model.value?.intro?.default_params
  if (!params || typeof params !== 'object' || Object.keys(params as object).length === 0) return ''
  try {
    return JSON.stringify(stripSchemaExtras(params), null, 2)
  } catch {
    return ''
  }
})

const integratePrompt = computed(() => {
  const langRaw = typeof integrateLanguage.value === 'string' ? integrateLanguage.value.trim() : ''
  const lang = langRaw || 'Python'
  const base = resolvedApiBase.value
  const s = slug.value
  const curl = curlSnippet.value
  const schemaJson = outputSchemaJson.value
  const inputJson = inputSchemaJson.value
  if (locale.value.startsWith('zh')) {
    const lines = [
      `我需要把下面这个“视频生成”接口接入到我的 ${lang} 项目里，请帮我写一份可以直接运行的最小示例，并按下面要求补齐生产使用需要的细节。`,
      '',
      '【接口说明】',
      '这是一个视频生成网关，采用“提交任务 + 轮询状态”的异步模式：',
      '1) POST 提交任务，返回 request_id',
      '2) GET 查询状态，直到 status = COMPLETED / FAILED / CANCELED',
      '3) COMPLETED 时从响应体里取产物 URL',
    ]
    if (inputJson) {
      lines.push('')
      lines.push('【输入字段 schema】')
      lines.push('以下是提交任务时请求体（application/json）中可用的输入字段声明（每个字段可能带有 value 默认值 / required / description / enum 等约束），请据此构造请求体并暴露为可配置参数：')
      lines.push('```json')
      lines.push(inputJson)
      lines.push('```')
    } else {
      lines.push('')
      lines.push('【输入字段 schema】')
      lines.push('本模型未声明固定的输入字段 schema，请以下方【调用示例】中的请求体为准构造入参。')
    }
    if (schemaJson) {
      lines.push('')
      lines.push('【输出字段 schema】')
      lines.push('以下是本模型 result payload 中可用的字段声明（key 支持属性链 / images[0].url / images[*] 等形式），请按其中的 key 从 result payload 中提取目标字段：')
      lines.push('```json')
      lines.push(schemaJson)
      lines.push('```')
    } else {
      lines.push('')
      lines.push('【输出字段 schema】')
      lines.push('本模型未声明固定的输出字段 schema，请从实际 result payload 中自行探测字段（常见如 video.url / images[].url）。')
    }
    lines.push('')
    lines.push('【调用示例（请以此为准）】')
    lines.push(curl)
    lines.push('')
    lines.push('【要求】')
    lines.push('1. 认证：Authorization: Bearer <API_KEY>，请把 API Key 作为函数入参 / CLI 参数 / 配置项传入，不要硬编码到源码里。')
    lines.push('2. 提交后拿到 request_id，用 GET 轮询：')
    lines.push(`   - 轮询地址：${base}/api/v1/model/${s}/requests/{request_id}/status`)
    lines.push('   - 建议初始 2s，指数退避到 15s，总超时 10 分钟。')
    lines.push('3. 状态字段可能是 IN_QUEUE / IN_PROGRESS / COMPLETED / FAILED / CANCELED；只有 COMPLETED 才去读产物。')
    lines.push('4. 错误处理：网络错误重试 3 次；HTTP 4xx 直接抛错并把响应体打出来；HTTP 5xx 走退避重试。')
    lines.push(`5. 结果获取：COMPLETED 后再调一次 GET ${base}/api/v1/model/${s}/requests/{request_id} 拿最终 payload，返回给调用方。`)
    lines.push('6. 请把 timeout、baseUrl、apiKey、slug、请求体 都做成函数参数或配置项，不要写死。')
    lines.push('7. 产物必须包含：完整源码 + 依赖安装命令 + 运行方式 + 一个示例 main 函数演示端到端流程。')
    lines.push(`8. 代码风格要符合 ${lang} 的社区最佳实践，包含必要的类型标注/错误分支/日志。`)
    lines.push('')
    lines.push(`请直接给出完整代码，不要省略。如果开发语言是 cURL，只给 shell 脚本即可。`)
    return lines.join('\n')
  }
  const lines = [
    `I need to integrate the following "video generation" API into my ${lang} project. Please write a runnable minimal example and cover all production concerns below.`,
    '',
    '[API model]',
    'A video generation gateway using async submit + poll:',
    '1) POST to submit → returns request_id',
    '2) GET status until status = COMPLETED / FAILED / CANCELED',
    '3) On COMPLETED, read the output URL from the response body',
  ]
  if (inputJson) {
    lines.push('')
    lines.push('[Input field schema]')
    lines.push('The following declares the input fields accepted by the submit request body (application/json). Each field may carry a `value` default, `required`, `description`, `enum`, etc. Build the request body from this schema and expose the fields as configurable parameters:')
    lines.push('```json')
    lines.push(inputJson)
    lines.push('```')
  } else {
    lines.push('')
    lines.push('[Input field schema]')
    lines.push('No fixed input schema is declared. Please use the request body from the [Reference call] below as the source of truth.')
  }
  if (schemaJson) {
    lines.push('')
    lines.push('[Output field schema]')
    lines.push('The following declares the fields available on the result payload (keys support property chains like `images[0].url` or wildcards like `images[*]`). Use these keys to extract fields from the result payload:')
    lines.push('```json')
    lines.push(schemaJson)
    lines.push('```')
  } else {
    lines.push('')
    lines.push('[Output field schema]')
    lines.push('No fixed output schema is declared for this model. Please probe fields from the actual result payload (commonly `video.url` or `images[].url`).')
  }
  lines.push('')
  lines.push('[Reference call (source of truth)]')
  lines.push(curl)
  lines.push('')
  lines.push('[Requirements]')
  lines.push('1. Auth: `Authorization: Bearer <API_KEY>`. Pass the API key in as a function argument / CLI flag / config value — do not hardcode it into the source.')
  lines.push('2. After submit, poll GET:')
  lines.push(`   - ${base}/api/v1/model/${s}/requests/{request_id}/status`)
  lines.push('   - Start at 2s, exponential backoff up to 15s, hard timeout 10 min.')
  lines.push('3. Possible status values: IN_QUEUE / IN_PROGRESS / COMPLETED / FAILED / CANCELED. Only fetch output on COMPLETED.')
  lines.push('4. Errors: retry network errors 3x; on 4xx surface the response body and stop; on 5xx back-off retry.')
  lines.push(`5. On COMPLETED, GET ${base}/api/v1/model/${s}/requests/{request_id} to obtain the final payload and return it to the caller.`)
  lines.push('6. baseUrl / apiKey / slug / timeout / request body must be configurable, not hardcoded.')
  lines.push('7. Deliver: full source + install command + how to run + a `main` demo that runs end-to-end.')
  lines.push(`8. Follow the ${lang} community best practice — types, error branches, logging.`)
  lines.push('')
  lines.push(`Return the complete code; do not omit anything. If the target language is cURL, a shell script is fine.`)
  return lines.join('\n')
})

async function copyIntegratePrompt() {
  try {
    await navigator.clipboard.writeText(integratePrompt.value)
    appStore.showSuccess(t('videoModels.copied'))
  } catch {
    appStore.showError(t('videoModels.copyFailed'))
  }
}

// ============ 状态与队列展示 ============
// 说明：之前用来渲染右上角状态徽章 / 队列位置的 phaseIndicator / queuePosition
// 已随右栏结果区精简一并下线；如需回归可从 git 历史找回。

// inflightPhaseLabel：进行中卡片里的阶段文案。isBusy 只有 submitting / queued / running
// 三种阶段（idle/completed/failed/canceled 不属于 busy），这里按当前 phase 映射到对应
// i18n key；未知 phase 兜底走 phaseRunning 保持有内容。
const inflightPhaseLabel = computed(() => {
  switch (playground.phase.value) {
    case 'submitting':
      return t('videoModels.playground.phaseSubmitting')
    case 'queued':
      return t('videoModels.playground.phaseQueued')
    case 'running':
      return t('videoModels.playground.phaseRunning')
    default:
      return t('videoModels.playground.phaseRunning')
  }
})

// inflightQueuePosition：上游 status payload 里可能带 queue_position（fal 排队场景）。
// 有则展示，让用户知道自己在队列的哪个位置；否则返回 null 不渲染这一行。
const inflightQueuePosition = computed<number | null>(() => {
  const s = playground.statusPayload.value
  if (!s || typeof s !== 'object') return null
  const q = (s as Record<string, unknown>).queue_position
  return typeof q === 'number' ? q : null
})

const prettyResult = computed(() => {
  try {
    return JSON.stringify(playground.resultPayload.value, null, 2)
  } catch {
    return String(playground.resultPayload.value)
  }
})

// ============ 输出区 payload Tab 的显示内容 ============
// 有请求结果时展示真实 result_payload；尚无结果时展示 output_fields 中配置的
// 示例默认值。这里不使用 curlBody，避免把输入参数误标成输出 payload。
const payloadForOutputTab = computed(() => {
  if (playground.resultPayload.value) return prettyResult.value
  try {
    return JSON.stringify(buildOutputExamplePayload(outputFields.value), null, 2)
  } catch {
    return '{}'
  }
})

// ============ 费用估算 & 实扣 ============
// 预估费用（详细版）：命中 pricing 档位后返回结构化拆分，UI 里展示公式
//   单价 $X/s × 时长 Ns = 预估 $Y
// 说明：
//   - 这里是"上游 fal 计费"级的估算，不含用户所在渠道的 rate_multiplier；
//     所以文案上会明确标注"以实际扣费为主"。
//   - duration 为 "auto" / 缺失 → 使用 AUTO_DURATION_FALLBACK_SECONDS=10 兜底，
//     与后端 defaultAutoDurationSeconds 常量保持一致（后端预扣按这个值冻结）。
const AUTO_DURATION_FALLBACK_SECONDS = 10

// EstimateBreakdown：预估费用的结构化描述。
// isAutoDuration：body 里 duration 是 "auto" / 缺失时置 true，UI 会在时长旁标出
//   "auto 按 10s 预估"以避免用户误以为是精确值。
interface EstimateBreakdown {
  resolution: string
  unitPricePerSecond: number
  durationSeconds: number
  isAutoDuration: boolean
  total: number
}

const estimateBreakdown = computed<EstimateBreakdown | null>(() => {
  const m = model.value
  if (!m || !m.pricing || m.pricing.length === 0) return null
  const body = tryPeekBody()
  if (!body) return null
  // resolution：字符串归一化后与 pricing[*].resolution 精确匹配
  const rawRes = body.resolution
  const resolution = typeof rawRes === 'string' ? rawRes.trim() : ''
  if (!resolution) return null
  const tier = m.pricing.find(
    (p) => p.enabled && p.resolution.toLowerCase() === resolution.toLowerCase()
  )
  if (!tier || tier.price_per_second <= 0) return null
  // duration：优先取 body.duration；数字/纯数字字符串使用之，其他（如 "auto"）走兜底
  let duration = 0
  let isAuto = false
  const rawDur = body.duration
  if (typeof rawDur === 'number' && rawDur > 0) {
    duration = rawDur
  } else if (typeof rawDur === 'string') {
    const trimmed = rawDur.trim()
    if (/^\d+(?:\.\d+)?$/.test(trimmed)) {
      duration = parseFloat(trimmed)
    } else {
      duration = AUTO_DURATION_FALLBACK_SECONDS
      isAuto = true
    }
  } else {
    duration = AUTO_DURATION_FALLBACK_SECONDS
    isAuto = true
  }
  if (duration <= 0) return null
  return {
    resolution: tier.resolution,
    unitPricePerSecond: tier.price_per_second,
    durationSeconds: duration,
    isAutoDuration: isAuto,
    total: tier.price_per_second * duration,
  }
})

// actualCost：任务终态后通过 GET /user/video-models/tasks/by-request/:rid 拉取。
// null → 未拉到（仍在加载或未终态）。
const actualCost = ref<number | null>(null)

// 每次任务终态时拉一次实扣；phase 从终态回退（reset）时清空。
watch(
  () => playground.phase.value,
  async (p) => {
    if (p === 'completed') {
      const rid = playground.internalRequestId.value
      if (!rid) return
      try {
        const resp = await videoModelsAPI.getTaskByRequestId(rid)
        const t = resp.data
        // 优先 final_cost；未终结前 finalCost=0，held_cost 作为兜底展示。
        if (t.final_cost > 0) {
          actualCost.value = t.final_cost
        } else if (t.held_cost > 0) {
          actualCost.value = t.held_cost
        } else {
          actualCost.value = null
        }
      } catch {
        // 静默失败，仅不展示实扣；不打扰用户主流程。
        actualCost.value = null
      }
    } else if (p === 'idle') {
      actualCost.value = null
    }
  }
)

// ============ 输出结构 + 值（右下卡片） ============
// 需求：右栏底部要固定展示"管理端声明的输出参数"的结构树，并在每个字段旁给出
// 当前值。取值优先级：
//   1) payload：任务完成后从 result payload 里按 key 精确取值；
//   2) schemaExample：payload 缺席时，从 output_fields 声明本身里抽出的"示例对象"，
//      递归收集 leaf 的 value（含 properties/items）；这样一进入演练台就能
//      看到"字段大概长什么样"。
//
// 与 resolvedOutputs（上方按 pickByPath 抽路径的列表）的区别：
//   - resolvedOutputs 只处理"有值 + 有 spec"的情况，缺失则跳过；且遵循 result_field
//     决定 primary 大图；
//   - 本区块永远按 schema 全量展开，"没有值"的字段也保留占位 —— 便于用户阅读
//     全量输出契约。

/**
 * adaptOutputFieldToNode：把一个 OutputFieldSpec / 内层 schema 递归适配为
 * OutputSchemaValueTree 需要的 SchemaNode。
 *
 *   - object：{ properties: {...} } → children
 *   - array ：{ items: {...} }      → items（单一子节点）
 *   - 叶子（type ∈ string/number/boolean，或"无法识别"）：children=[], items=null
 *
 * 参数：
 *   - key    : 当前节点在父层的字段名；数组元素传空字符串，组件展示时会用 `[i]`
 *   - raw    : 原始 spec / 内嵌 schema（可能是 OutputFieldSpec 也可能是 map）
 */
function adaptOutputFieldToNode(key: string, raw: unknown): SchemaNode {
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    const obj = raw as Record<string, unknown>
    const rawType = normalizeNodeType(obj.type)
    const required = obj.required === true
    const description = typeof obj.description === 'string' ? obj.description : ''
    // description_en：中英双文字段说明的英文版；渲染层按 locale 选。
    const descriptionEn = typeof obj.description_en === 'string' ? obj.description_en : ''
    // object
    if (rawType === 'object' || obj.properties) {
      const children: SchemaNode[] = []
      const props = obj.properties as Record<string, unknown> | undefined
      if (props && typeof props === 'object' && !Array.isArray(props)) {
        for (const ck of Object.keys(props)) {
          children.push(adaptOutputFieldToNode(ck, props[ck]))
        }
      }
      return {
        key,
        rawType: 'object',
        required,
        description,
        descriptionEn,
        children,
        items: null,
      }
    }
    // array
    if (rawType === 'array' || 'items' in obj) {
      const items = adaptOutputFieldToNode('', obj.items)
      return {
        key,
        rawType: 'array',
        required,
        description,
        descriptionEn,
        children: [],
        items,
      }
    }
    // leaf
    return {
      key,
      rawType: rawType === 'number' || rawType === 'boolean' ? rawType : 'string',
      required,
      description,
      descriptionEn,
      children: [],
      items: null,
    }
  }
  // 非对象裸值：直接当 string 叶子
  return {
    key,
    rawType: 'string',
    required: false,
    description: '',
    descriptionEn: '',
    children: [],
    items: null,
  }
}

/**
 * normalizeNodeType：把 spec.type 字段归一化。对当前的 OutputFieldType
 * ('string'|'number'|'boolean'|'object'|'array') 直接透传；对旧数据（比如
 * 'video'/'image'/'json'/'text'）做兼容映射，避免 rawType 落到未知值。
 */
function normalizeNodeType(v: unknown): SchemaNode['rawType'] {
  const s = typeof v === 'string' ? v : ''
  switch (s) {
    case 'number':
      return 'number'
    case 'boolean':
      return 'boolean'
    case 'object':
    case 'json': // 旧数据：json → object（当作对象处理）
      return 'object'
    case 'array':
      return 'array'
    default:
      // string / text / url / video / image / '' / 未知 → 一律按字符串叶子
      return 'string'
  }
}

/**
 * schemaExampleValue：从一个"输出参数节点"里递归抽取"示例值"，供 payload 缺席时
 * 展示在树上。规则：
 *   - 叶子（含 value）：取 value；否则 undefined
 *   - object：{ childKey: schemaExampleValue(child), ... }（跳过 undefined 子值）
 *   - array：[schemaExampleValue(items)]（若 items 有示例值则展示一个元素，否则空数组）
 */
function schemaExampleValue(raw: unknown): unknown {
  if (raw === null || raw === undefined) return undefined
  if (typeof raw !== 'object' || Array.isArray(raw)) return undefined
  const obj = raw as Record<string, unknown>
  const t = normalizeNodeType(obj.type)
  // object：递归 properties
  if (t === 'object' || obj.properties) {
    const props = obj.properties as Record<string, unknown> | undefined
    if (!props || typeof props !== 'object') return {}
    const out: Record<string, unknown> = {}
    for (const k of Object.keys(props)) {
      const v = schemaExampleValue(props[k])
      if (v !== undefined) out[k] = v
    }
    return Object.keys(out).length > 0 ? out : undefined
  }
  // array：一个元素
  if (t === 'array' || 'items' in obj) {
    const it = schemaExampleValue(obj.items)
    return it === undefined ? [] : [it]
  }
  // 叶子：取 value（若显式有值就用；否则返回 undefined 让上层跳过）
  if ('value' in obj) return obj.value
  // 兼容：管理员在旧数据里把示例塞在 default 字段中
  if ('default' in obj) return obj.default
  return undefined
}

/**
 * outputSchemaNodes：把顶层 output_fields 适配为 SchemaNode[]，供模板 v-for。
 * 与 outputFields 一一对应；顺序稳定。
 */
const outputSchemaNodes = computed<SchemaNode[]>(() =>
  outputFields.value.map((f) => adaptOutputFieldToNode(f.key, f))
)

/**
 * outputValueFor：为顶层 output_fields[i] 计算展示值。
 *   - payload 存在且 payload[key] 有值 → 用 payload[key]
 *   - 否则 → 用 schemaExampleValue(spec) 从 spec 自身抽取示例值
 *   - 都没有 → undefined
 */
function outputValueFor(spec: OutputFieldSpec): unknown {
  const payload = playground.resultPayload.value
  if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
    const v = (payload as Record<string, unknown>)[spec.key]
    if (v !== undefined) return v
  }
  return schemaExampleValue(spec)
}

/**
 * outputValueSourceFor：判断某个字段当前展示的值来自 payload 还是示例。
 * 用于在字段左上角标记来源，方便用户区分。
 */
function outputValueSourceFor(spec: OutputFieldSpec): 'payload' | 'example' | 'none' {
  const payload = playground.resultPayload.value
  if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
    const v = (payload as Record<string, unknown>)[spec.key]
    if (v !== undefined) return 'payload'
  }
  const ex = schemaExampleValue(spec)
  return ex === undefined ? 'none' : 'example'
}

// ============ 生命周期 ============
function goBack() {
  // 若任务还在运行，提示用户确认；确认后取消任务再返回。
  if (playground.isBusy.value) {
    const ok = window.confirm(t('videoModels.playground.confirmClose'))
    if (!ok) return
    if (selectedKey.value) {
      void playground.cancel(slug.value, selectedKey.value.key)
    } else {
      playground.reset()
    }
  }
  router.push({ name: 'VideoModels' })
}

// 路由离开前拦截：与 goBack 语义一致，防止用户直接按浏览器返回导致轮询残留。
onBeforeRouteLeave((_to, _from, next) => {
  if (playground.isBusy.value) {
    const ok = window.confirm(t('videoModels.playground.confirmClose'))
    if (!ok) {
      next(false)
      return
    }
    if (selectedKey.value) {
      void playground.cancel(slug.value, selectedKey.value.key)
    } else {
      playground.reset()
    }
  }
  next()
})

onMounted(async () => {
  await loadModel()
  initFormDefaults()
  mode.value = fieldSpecs.value.length > 0 ? 'form' : 'json'
  playground.reset()
  await loadKeys()
})

onBeforeUnmount(() => {
  playground.reset()
})

// slug 切换（比如浏览器地址栏直接改）时重新加载
watch(slug, async () => {
  await loadModel()
  initFormDefaults()
  mode.value = fieldSpecs.value.length > 0 ? 'form' : 'json'
  playground.reset()
})
</script>
