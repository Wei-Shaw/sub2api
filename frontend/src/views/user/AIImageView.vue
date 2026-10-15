<template>
  <AppLayout>
    <!-- 铺满可用宽度：不设 max-w。
         这个页面是「左边看参数、中间看图」的重活页面，主图越大越好核对。
         所以除了左侧导航和顶部浏览器那两条，中间全部给它用。 -->
    <div class="flex w-full flex-col gap-4">
      <!-- 页头 -->
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('aiImage.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('aiImage.subtitle') }}</p>
        </div>
        <div class="flex items-center gap-2">
          <span
            v-if="keyState === 'valid'"
            class="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-3 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400"
          >
            <Icon name="checkCircle" size="sm" />
            {{ keyGroupName ? t('aiImage.key.valid', { group: keyGroupName }) : t('aiImage.key.validNoGroup') }}
          </span>
          <!-- 这个按钮只是展开/收起密钥面板，不负责保存。
               两点必须成立，否则用户会以为「点了没反应」：
               1. 文案不能和面板里的「保存密钥」重复；
               2. 密钥可用之后仍然要能打开面板，否则永远换不了密钥。
          -->
          <button type="button" class="btn btn-secondary btn-sm" @click="showKeyPanel = !showKeyPanel">
            <Icon name="key" size="sm" class="mr-1.5" />
            {{ keyState === 'valid' ? t('aiImage.key.change') : t('aiImage.key.configure') }}
          </button>
        </div>
      </div>

      <!-- 密钥面板 -->
      <div
        v-if="showKeyPanel || keyState !== 'valid'"
        class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="mb-2 flex items-center gap-2 text-sm font-medium text-gray-900 dark:text-white">
          <Icon name="key" size="sm" />
          {{ t('aiImage.key.sectionTitle') }}
        </div>
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
          <input
            v-model="keyInput"
            type="password"
            autocomplete="off"
            class="input flex-1 font-mono text-sm"
            :placeholder="t('aiImage.key.placeholder')"
            @keyup.enter="saveKey"
          />
          <div class="flex items-center gap-2">
            <button type="button" class="btn btn-primary" :disabled="keyChecking" @click="saveKey">
              <Icon :name="keyChecking ? 'refresh' : 'check'" size="sm" class="mr-1.5" :class="keyChecking ? 'animate-spin' : ''" />
              {{ keyChecking ? t('aiImage.key.checking') : t('aiImage.key.save') }}
            </button>
            <button v-if="keyState === 'valid'" type="button" class="btn btn-secondary" @click="clearKey">
              {{ t('aiImage.key.clear') }}
            </button>
          </div>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.key.hint') }}</p>

        <!-- 密钥校验结果 -->
        <div
          v-if="keyState === 'invalid' || keyState === 'not-image-model' || keyState === 'not-owned' || keyState === 'load-failed'"
          class="mt-3 rounded-lg border px-3 py-2 text-sm"
          :class="
            keyState === 'not-image-model' || keyState === 'invalid'
              ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-400'
              : 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-400'
          "
        >
          <div class="flex items-start gap-2">
            <Icon name="exclamationTriangle" size="sm" class="mt-0.5 flex-shrink-0" />
            <div class="min-w-0">
              <p class="font-medium">{{ keyErrorTitle }}</p>
              <p v-if="keyState === 'not-image-model'" class="mt-1 text-xs leading-relaxed">
                {{ t('aiImage.key.notImageModelHint', { group: keyGroupName || '-' }) }}
              </p>
              <p v-else-if="keyState === 'not-owned'" class="mt-1 text-xs leading-relaxed">
                {{ t('aiImage.key.notOwned') }}
              </p>
              <p v-else-if="keyState === 'load-failed'" class="mt-1 text-xs leading-relaxed">
                {{ t('aiImage.key.loadFailed') }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- ============ 功能模块 ============
           主人反馈：这些按钮原来埋在提示词卡片的下半部分，根本注意不到。
           所以统一提到页面最上面一排，点哪个就在下面展开哪个模块的内容，
           提示词输入框顺位向下排。

           分两档：主要功能是「单张图片放入去生图」和「爆款复刻」——
           这两个是日常真在用的；其余四个是偶尔才用到的辅助，归到次要里，
           免得一上来一堆按钮分不清主次。
      -->
      <div
        class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
        data-testid="module-bar"
      >
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('aiImage.modules.title') }}</h2>
          <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('aiImage.modules.hint') }}</span>
        </div>

        <!-- 主要功能 -->
        <div class="flex flex-wrap items-center gap-2" data-testid="primary-modules">
          <span class="text-xs font-semibold text-primary-600 dark:text-primary-400">
            {{ t('aiImage.modules.primary') }}
          </span>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="toggle-hot-panel"
            :class="showHotPanel ? 'ring-2 ring-primary-400' : ''"
            @click="showHotPanel = !showHotPanel"
          >
            <Icon name="sparkles" size="sm" class="mr-1.5" />
            {{ t('aiImage.hot.title') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="toggle-script-panel"
            :class="showScriptPanel ? 'ring-2 ring-primary-400' : ''"
            @click="showScriptPanel = !showScriptPanel"
          >
            <Icon name="document" size="sm" class="mr-1.5" />
            {{ t('aiImage.script.title') }}
          </button>
          <!-- 这句是「单张生图就写在下面」的指路话，不是哪个按钮的说明。
               用 ml-auto 推到行尾，免得紧跟按钮后面被当成「套图」的解释。 -->
          <span class="ml-auto text-xs text-gray-400 dark:text-gray-500">{{ t('aiImage.modules.primaryHint') }}</span>
        </div>

        <!-- 次要功能 -->
        <div
          class="mt-3 flex flex-wrap items-center gap-2 border-t border-gray-100 pt-3 dark:border-dark-700"
          data-testid="secondary-modules"
        >
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('aiImage.modules.secondary') }}
          </span>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="toggle-template-panel"
            :class="showTemplatePanel ? 'ring-2 ring-primary-400' : ''"
            @click="showTemplatePanel = !showTemplatePanel"
          >
            <Icon name="edit" size="sm" class="mr-1.5" />
            {{ t('aiImage.template.title') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="toggle-composite-panel"
            :class="showCompositePanel ? 'ring-2 ring-primary-400' : ''"
            @click="showCompositePanel = !showCompositePanel"
          >
            <Icon name="grid" size="sm" class="mr-1.5" />
            {{ t('aiImage.composite.title') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="toggle-enhance-panel"
            :class="showEnhancePanel ? 'ring-2 ring-primary-400' : ''"
            @click="showEnhancePanel = !showEnhancePanel"
          >
            <Icon name="shield" size="sm" class="mr-1.5" />
            {{ t('aiImage.watermark.title') }}
          </button>
        </div>
      </div>

      <!-- 爆款复刻：拆同行版式 → 套到自己的商品上。
           自带产品信息输入，不依赖「套图」面板：
           只要有版式（拆出来的，或套用的已存模板），填上产品名就能直接生成。
      -->
      <div
        v-if="showHotPanel"
        class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
        data-testid="hot-panel"
      >
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('aiImage.hot.title') }}</h2>
          <button type="button" class="btn btn-secondary btn-sm" @click="clearHotRef">
            {{ t('aiImage.hot.clear') }}
          </button>
        </div>

        <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('aiImage.hot.hint') }}</p>

        <!-- 第 1 步：传爆款图 → 拆版式 -->
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <input
            ref="hotRefInputRef"
            type="file"
            accept="image/*"
            multiple
            class="hidden"
            @change="onHotRefPicked"
          />
          <button type="button" class="btn btn-secondary btn-sm" data-testid="hot-pick" @click="hotRefInputRef?.click()">
            <Icon name="upload" size="sm" class="mr-1.5" />
            {{ t('aiImage.hot.pick') }}
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm"
            data-testid="hot-analyze"
            :disabled="hotAnalyzing || !hotRefFiles.length"
            @click="runHotAnalyze"
          >
            <Icon :name="hotAnalyzing ? 'refresh' : 'brain'" size="sm" class="mr-1.5" :class="hotAnalyzing ? 'animate-spin' : ''" />
            {{ hotAnalyzing ? t('aiImage.hot.analyzing') : t('aiImage.hot.analyze') }}
          </button>
          <span v-if="hotProgress.total" class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('aiImage.hot.progress', { done: hotProgress.done, total: hotProgress.total }) }}
          </span>
        </div>

        <!-- 拆解出来的版式描述 -->
        <div v-if="hotStyleDesc" class="mt-3 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-900">
          <p class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('aiImage.hot.styleTitle') }}</p>
          <p class="mt-1 break-words text-xs leading-relaxed text-gray-700 dark:text-gray-200" data-testid="hot-style-desc">
            {{ hotStyleDesc }}
          </p>
        </div>

        <!-- 当前用哪套「模板」。没有的话直接说清楚，免得点了生成才发现不对。 -->
        <p
          class="mt-3 text-xs leading-relaxed"
          :class="hotTemplate ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'"
          data-testid="hot-template-state"
        >
          {{ hotTemplate ? t('aiImage.hot.templateReady') : t('aiImage.hot.needTemplate') }}
        </p>

        <!-- 第 2 步：填自己的产品信息（这一步是独立的，不用先去把 Ozon 脚本那张表） -->
        <div class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-700">
          <p class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('aiImage.hot.step2') }}</p>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <div class="flex flex-col gap-1">
              <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.product') }}</label>
              <input
                v-model="hotForm.product"
                class="input text-sm"
                :placeholder="t('aiImage.script.productPlaceholder')"
                data-testid="hot-product"
              />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.sp1') }}</label>
              <input v-model="hotForm.sp1" class="input text-sm" data-testid="hot-sp1" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.sp2') }}</label>
              <input v-model="hotForm.sp2" class="input text-sm" data-testid="hot-sp2" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.sp3') }}</label>
              <input v-model="hotForm.sp3" class="input text-sm" data-testid="hot-sp3" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.pain') }}</label>
              <input v-model="hotForm.pain" class="input text-sm" data-testid="hot-pain" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.diff') }}</label>
              <input v-model="hotForm.diff" class="input text-sm" data-testid="hot-diff" />
            </div>
          </div>

          <div class="mt-3 flex flex-wrap items-center gap-2">
            <label class="inline-flex cursor-pointer items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
              <input v-model="hotIncludeExtras" type="checkbox" class="h-3.5 w-3.5" data-testid="hot-extras" />
              {{ t('aiImage.hot.extras') }}
            </label>
            <button
              type="button"
              class="btn btn-primary btn-sm"
              data-testid="hot-build-script"
              @click="buildHotScriptCards"
            >
              {{ t('aiImage.hot.buildScript') }}
            </button>
          </div>
        </div>

        <p v-if="hotNotice" class="mt-2 text-xs text-emerald-600 dark:text-emerald-400">{{ hotNotice }}</p>
        <p v-if="hotError" class="mt-2 text-xs text-red-600 dark:text-red-400">{{ hotError }}</p>
      </div>

      <!-- 提示词模板 -->
      <div
        v-if="showTemplatePanel"
        data-testid="template-panel"
        class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('aiImage.template.title') }}</h2>
        <div class="flex flex-wrap items-center gap-2">
          <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.template.label') }}</label>
          <select
            v-model="selectedTemplateId"
            class="input w-56 text-sm"
            data-testid="template-select"
            @change="applyTemplate"
          >
            <option value="">{{ t('aiImage.template.placeholder') }}</option>
            <option v-for="item in templates" :key="item.id" :value="item.id">{{ item.name }}</option>
          </select>
          <button type="button" class="btn btn-secondary btn-sm" data-testid="template-save" @click="openTemplateSave">
            {{ t('aiImage.template.save') }}
          </button>
          <button
            v-if="selectedTemplateId"
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="template-delete"
            @click="deleteSelectedTemplate"
          >
            {{ t('aiImage.template.delete') }}
          </button>
          <span v-if="templateNotice" class="text-xs text-gray-500 dark:text-gray-400">{{ templateNotice }}</span>
        </div>

        <!-- 存模板时填名字 -->
        <div v-if="showTemplateSave" class="mt-2 flex flex-wrap items-center gap-2">
          <input
            v-model="templateName"
            class="input w-56 text-sm"
            :maxlength="MAX_TEMPLATE_NAME"
            :placeholder="t('aiImage.template.namePlaceholder')"
            data-testid="template-name"
            @keyup.enter="confirmTemplateSave"
          />
          <button type="button" class="btn btn-primary btn-sm" data-testid="template-confirm" @click="confirmTemplateSave">
            {{ t('aiImage.template.confirm') }}
          </button>
          <button type="button" class="btn btn-secondary btn-sm" @click="cancelTemplateSave">
            {{ t('aiImage.template.cancel') }}
          </button>
        </div>
      </div>

      <!-- 一图多视：把多张商品图拼成一张参考图 -->
      <div
        v-if="showCompositePanel"
        data-testid="composite-panel"
        class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('aiImage.composite.title') }}</h2>
        <div class="flex flex-wrap items-center gap-2">
          <input
            ref="multiFileInputRef"
            type="file"
            accept="image/*"
            multiple
            class="hidden"
            @change="onMultiReferencePicked"
          />
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="composite-pick"
            :disabled="compositing"
            @click="multiFileInputRef?.click()"
          >
            <Icon :name="compositing ? 'refresh' : 'upload'" size="sm" class="mr-1.5" :class="compositing ? 'animate-spin' : ''" />
            {{ compositing ? t('aiImage.composite.working') : t('aiImage.composite.pick') }}
          </button>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.composite.hint') }}</span>
        </div>
        <p v-if="compositeNotice" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ compositeNotice }}</p>
      </div>

      <!-- 水印 -->
      <div
        v-if="showEnhancePanel"
        data-testid="enhance-panel"
        class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('aiImage.watermark.title') }}</h2>

        <label class="inline-flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input v-model="watermark.enabled" type="checkbox" class="h-3.5 w-3.5" data-testid="watermark-enabled" />
          {{ t('aiImage.watermark.enable') }}
        </label>

        <div class="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <div class="flex items-center gap-2">
            <label class="w-16 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.watermark.text') }}</label>
            <input v-model="watermark.text" class="input w-full text-sm" data-testid="watermark-text" />
          </div>
          <div class="flex items-center gap-2">
            <label class="w-16 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.watermark.alpha') }}</label>
            <input
              v-model.number="watermark.alpha"
              type="range"
              :min="WATERMARK_ALPHA_MIN"
              :max="WATERMARK_ALPHA_MAX"
              class="w-full"
              data-testid="watermark-alpha"
            />
            <span class="w-8 text-right text-xs text-gray-500 dark:text-gray-400">{{ watermark.alpha }}</span>
          </div>
          <div class="flex items-center gap-2">
            <label class="w-16 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.watermark.fontSize') }}</label>
            <input v-model.number="watermark.fontSize" type="number" min="0" max="200" class="input w-full text-sm" />
            <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('aiImage.watermark.auto') }}</span>
          </div>
        </div>

        <!-- 说清楚两个刻意的改动，免得以后有人「顺手改回桌面程序那样」 -->
        <p class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
          {{ t('aiImage.watermark.note') }}
        </p>
        <p v-if="watermarkNotice" class="mt-2 text-xs text-gray-600 dark:text-gray-300">{{ watermarkNotice }}</p>
      </div>

      <!-- 套图（原「Ozon 八张主图脚本」） -->
      <div
        v-if="showScriptPanel"
        data-testid="script-panel"
        class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('aiImage.script.title') }}</h2>
          <div class="flex items-center gap-2">
            <button type="button" class="btn btn-secondary btn-sm" @click="clearScript">
              {{ t('aiImage.script.clear') }}
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <div class="flex flex-col gap-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.product') }}</label>
            <input v-model="scriptForm.product" class="input text-sm" :placeholder="t('aiImage.script.productPlaceholder')" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.sp1') }}</label>
            <input v-model="scriptForm.sp1" class="input text-sm" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.sp2') }}</label>
            <input v-model="scriptForm.sp2" class="input text-sm" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.sp3') }}</label>
            <input v-model="scriptForm.sp3" class="input text-sm" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.pain') }}</label>
            <input v-model="scriptForm.pain" class="input text-sm" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.script.diff') }}</label>
            <input v-model="scriptForm.diff" class="input text-sm" />
          </div>
        </div>

        <!-- 卖点框里显示中文（看着顺），但图上印的是俄语。这里把「图上真正会出现的字」
             明明白白列出来，免得生成完才发现图上是中文。 -->
        <p class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-gray-400" data-testid="printed-ru-hint">
          <template v-if="hasPrintedRussian">
            {{ t('aiImage.script.printedRu') }}
            <span class="font-medium text-gray-700 dark:text-gray-200">{{ printedSellingPoints.filter(Boolean).join(' / ') }}</span>
          </template>
          <template v-else>{{ t('aiImage.script.printedRuNone') }}</template>
        </p>

        <!-- 自动匹配场景 -->
        <div class="mt-3 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
          <span>{{ t('aiImage.script.scene') }}：</span>
          <span class="font-medium text-gray-700 dark:text-gray-200">{{ scriptSceneText }}</span>
          <span class="text-gray-400 dark:text-gray-500">（{{ matchedScene.en }}）</span>
        </div>

        <!-- 视觉模型 + AI 识别 -->
        <div class="mt-3 flex flex-wrap items-center gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
          <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.options.model') }}</label>
          <select v-model="visionModel" class="input w-48 text-sm">
            <option v-for="item in VISION_MODEL_OPTIONS" :key="item" :value="item">{{ item }}</option>
          </select>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="aiExtracting" @click="aiExtract">
            <Icon :name="aiExtracting ? 'refresh' : 'brain'" size="sm" class="mr-1.5" :class="aiExtracting ? 'animate-spin' : ''" />
            {{ aiExtracting ? t('aiImage.script.aiExtracting') : t('aiImage.script.aiExtract') }}
          </button>
          <button type="button" class="btn btn-primary btn-sm" @click="buildScript">
            {{ t('aiImage.script.build') }}
          </button>
        </div>

        <p v-if="scriptNotice" class="mt-2 text-xs text-emerald-600 dark:text-emerald-400">{{ scriptNotice }}</p>
        <p v-if="scriptError" class="mt-2 text-xs text-red-600 dark:text-red-400">{{ scriptError }}</p>

        <!-- 脚本卡片不在这里显示。放在下面常驻的那一条里。
             爆款复刻也会产出卡片，卡片藏在这个折叠面板里的话，
             用爆款复刻的人就看不到自己刚生成的东西。 -->
      </div>

      <!-- 提示词：模块入口都提到上面去了，这里就是纯粹的「写提示词 + 生成」 -->
      <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 lg:flex-row lg:items-start">
          <!-- 输入框左侧的状态筛选 -->
          <div class="flex flex-shrink-0 flex-col gap-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.filter.label') }}</label>
            <select v-model="statusFilter" class="input w-full lg:w-32">
              <option value="all">{{ t('aiImage.filter.all') }}</option>
              <option value="completed">{{ t('aiImage.filter.completed') }}</option>
              <option value="generating">{{ t('aiImage.filter.generating') }}</option>
              <option value="failed">{{ t('aiImage.filter.failed') }}</option>
            </select>
          </div>

          <!-- 提示词输入框 -->
          <div class="flex min-w-0 flex-1 flex-col gap-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('aiImage.prompt.label') }}</label>
            <textarea
              v-model="prompt"
              rows="8"
              class="input w-full resize-y text-sm leading-relaxed"
              :placeholder="t('aiImage.prompt.placeholder')"
              data-testid="prompt-input"
              @keydown.ctrl.enter="startGenerate"
              @keydown.meta.enter="startGenerate"
            ></textarea>
          </div>

          <!-- 参数与生成按钮 -->
          <div class="flex flex-shrink-0 flex-col gap-2 lg:w-56">
            <div class="flex items-center gap-2">
              <label class="w-12 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.options.model') }}</label>
              <!-- 固定下拉：平台给生图分组配的就是这三个，手打容易打错，
                   而模型名写错时上游只回一句「模型不存在」，用户看不出是自己打错了。 -->
              <select v-model="model" class="input w-full text-sm" data-testid="model-select">
                <option v-for="name in AI_IMAGE_MODEL_OPTIONS" :key="name" :value="name">{{ name }}</option>
              </select>
            </div>
            <div class="flex items-center gap-2">
              <label class="w-12 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.options.size') }}</label>
              <select v-model="size" class="input w-full text-sm">
                <option v-for="option in SIZE_CHOICES" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div class="flex flex-col gap-0.5">
              <div class="flex items-center gap-2">
                <label class="w-12 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.options.count') }}</label>
                <input
                  v-model.number="count"
                  type="number"
                  min="1"
                  :max="maxGenerateCount"
                  class="input w-full text-sm"
                  data-testid="count-input"
                />
              </div>
              <!-- 小字提示上限：数字取自同一个常量，改上限时这里自动跟着变，不会对不上。 -->
              <p class="pl-14 text-xs text-gray-400 dark:text-gray-500" data-testid="count-hint">
                {{ t('aiImage.options.countHint', { max: maxGenerateCount }) }}
              </p>
            </div>
            <div class="flex items-center gap-2">
              <label class="w-12 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.product.label') }}</label>
              <input
                v-model="productId"
                class="input w-full text-sm"
                :placeholder="t('aiImage.product.placeholder')"
              />
            </div>
          </div>
        </div>

        <!-- 参考图 + 生成 -->
        <div class="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-3 dark:border-dark-700">
          <div class="flex flex-wrap items-center gap-2">
            <input ref="fileInputRef" type="file" accept="image/*" class="hidden" @change="onReferencePicked" />
            <button type="button" class="btn btn-secondary btn-sm" @click="fileInputRef?.click()">
              <Icon name="upload" size="sm" class="mr-1.5" />
              {{ referenceImage ? t('aiImage.reference.replace') : t('aiImage.reference.pick') }}
            </button>
            <button v-if="referenceImage" type="button" class="btn btn-secondary btn-sm" @click="clearReference">
              {{ t('aiImage.reference.clear') }}
            </button>
            <span class="max-w-[22rem] truncate text-xs text-gray-500 dark:text-gray-400">
              {{ referenceImage ? referenceImage.name : t('aiImage.reference.none') }}
            </span>
            <img
              v-if="referencePreviewUrl"
              :src="referencePreviewUrl"
              alt=""
              class="h-10 w-10 rounded border border-gray-200 object-cover dark:border-dark-600"
            />

            <!-- 商品一致性锁定：保证生成的图里还是同一件商品 -->
            <label
              class="inline-flex cursor-pointer items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium"
              :class="
                productLock
                  ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-400'
                  : 'border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-400'
              "
              :title="t('aiImage.lock.hint')"
            >
              <input v-model="productLock" type="checkbox" class="h-3.5 w-3.5" />
              {{ t('aiImage.lock.label') }} · {{ productLock ? t('aiImage.lock.on') : t('aiImage.lock.off') }}
            </label>
          </div>
          <div class="flex items-center gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!prompt" @click="prompt = ''">
              {{ t('aiImage.prompt.clear') }}
            </button>
            <button type="button" class="btn btn-primary" :disabled="generating" @click="startGenerate">
              <Icon :name="generating ? 'refresh' : 'sparkles'" size="sm" class="mr-1.5" :class="generating ? 'animate-spin' : ''" />
              {{ generating ? t('aiImage.prompt.generating') : t('aiImage.prompt.generate') }}
            </button>
          </div>
        </div>
        <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
          {{ t('aiImage.reference.hint') }}
          <span v-if="productId.trim()" class="text-gray-500 dark:text-gray-400">
            · {{ t('aiImage.product.hint') }}
          </span>
          <span v-if="productLock && !referenceImage" class="text-amber-600 dark:text-amber-400">
            {{ t('aiImage.lock.needsReference') }}
          </span>
        </p>
      </div>

      <!-- 批量进度 + 并发 + 一键生成全部 + 暂停 / 停止。
           主人两次说「找不到暂停/停止」，所以这一条常驻在提示词下面：
           既不折叠、也不藏在任何面板里，没在跑的时候置灰。 -->
      <div
        class="rounded-xl border border-gray-200 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800"
        data-testid="batch-bar"
      >
        <div class="flex flex-wrap items-center gap-2 text-xs">
          <template v-if="scriptCards.length">
            <span class="font-medium text-gray-700 dark:text-gray-200">
              {{ t('aiImage.batch.progress', { done: batchProgress.done, total: batchProgress.total || scriptCards.length }) }}
            </span>
            <span v-if="batchProgress.failed" class="text-red-600 dark:text-red-400">
              {{ t('aiImage.batch.failedCount', { count: batchProgress.failed }) }}
            </span>
            <span v-if="batchPaused" class="font-medium text-amber-600 dark:text-amber-400">
              {{ t('aiImage.batch.paused') }}
            </span>
          </template>
          <span v-else class="text-gray-500 dark:text-gray-400">{{ t('aiImage.batch.needScript') }}</span>

          <!-- 并发数：和桌面程序一样上限 3，避免被上游限流、也避免一次烧太多额度 -->
          <label v-if="scriptCards.length" class="inline-flex items-center gap-1 text-gray-500 dark:text-gray-400">
            {{ t('aiImage.batch.concurrency') }}
            <select v-model.number="batchConcurrency" class="input w-16 py-1 text-xs" :disabled="batchRunning">
              <option v-for="n in BATCH_CONCURRENCY_OPTIONS" :key="n" :value="n">{{ n }}</option>
            </select>
          </label>

          <button
            v-if="scriptCards.length"
            type="button"
            class="btn btn-primary btn-sm"
            data-testid="generate-all-script-cards"
            :disabled="batchRunning"
            @click="generateAllScriptCards"
          >
            <Icon
              :name="batchRunning ? 'refresh' : 'sparkles'"
              size="sm"
              class="mr-1"
              :class="batchRunning ? 'animate-spin' : ''"
            />
            {{ batchRunning ? t('aiImage.batch.running') : t('aiImage.batch.generateAll', { count: scriptCards.length }) }}
          </button>

          <div class="flex items-center gap-2">
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              data-testid="batch-pause"
              :disabled="!batchRunning"
              @click="toggleBatchPause"
            >
              {{ batchPaused ? t('aiImage.batch.resume') : t('aiImage.batch.pause') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              data-testid="batch-stop"
              :disabled="!batchRunning"
              @click="stopBatch"
            >
              {{ t('aiImage.batch.stop') }}
            </button>
          </div>
          <span class="text-gray-400 dark:text-gray-500">{{ t('aiImage.batch.stopHint') }}</span>
        </div>

        <!-- 这组卡片是谁生成的（爆款复刻 / Ozon 脚本），标一下免得两个入口混在一起分不清 -->
        <p v-if="scriptCards.length && scriptSource" class="mt-2 text-xs text-gray-400 dark:text-gray-500">
          {{
            scriptSource === 'ozon'
              ? t('aiImage.batch.fromOzon')
              : scriptSource === 'hot-saved'
                ? t('aiImage.batch.fromHotSaved')
                : t('aiImage.batch.fromHot')
          }}
        </p>

        <!-- 生成出来的脚本卡片。放在这条常驻的栏里，爆款复刻和 Ozon 脚本两个入口
             产出的卡片都在这儿，点一张就把它的提示词填进上面的输入框。 -->
        <div v-if="scriptCards.length" class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-700">
          <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.script.built', { count: scriptCards.length }) }}</span>
            <button type="button" class="btn btn-secondary btn-sm" @click="applyAllScriptCards">
              {{ t('aiImage.script.applyAll') }}
            </button>
          </div>

          <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <button
              v-for="card in scriptCards"
              :key="card.key"
              type="button"
              class="rounded-lg border border-gray-200 bg-gray-50 px-2 py-2 text-left text-xs hover:border-primary-400 hover:bg-primary-50 dark:border-dark-600 dark:bg-dark-900 dark:hover:border-primary-500"
              :title="card.prompt"
              @click="applyScriptCard(card)"
            >
              <span class="block font-medium text-gray-800 dark:text-gray-100">{{ card.title }}</span>
              <span class="mt-0.5 block text-gray-400 dark:text-gray-500">{{ t('aiImage.script.fillOne') }}</span>
            </button>
          </div>
        </div>
      </div>


      <!-- 中间：结果区 -->
      <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('aiImage.result.sectionTitle') }}</h2>
          <!--
            客户端是按「images/<商品编号>/<序号>.png」存图的，序号就是粘贴顺序。
            所以这里按生成先后（早→晚）导出地址，粘进去正好落在第 1、2、3 张的位置。
          -->
          <button
            v-if="copyableUrls.length"
            type="button"
            class="btn btn-secondary btn-sm"
            @click="copyAllImageUrls"
          >
            <Icon name="link" size="sm" class="mr-1" />
            {{ t('aiImage.product.copyAll', { count: copyableUrls.length }) }}
          </button>
        </div>
        <p v-if="copyNotice" class="mb-3 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400">
          {{ copyNotice }}
        </p>
        <div v-if="filteredRecords.length === 0" class="flex min-h-[22rem] flex-col items-center justify-center gap-2 text-center">
          <Icon name="sparkles" size="xl" class="text-gray-300 dark:text-dark-600" />
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('aiImage.result.empty') }}</p>
          <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('aiImage.result.emptyHint') }}</p>
        </div>

        <!-- 结果区：段（本轮 / 历史）→ 行（每一轮）→ 卡片。
             主人要求「图片横向排列、一次生成之后下一次单开一行」，
             所以不再用固定列数的网格，改成：每轮一个横向容器，
             卡片在里面横向流动、排满自动换行，轮与轮之间各自另起一行。
             卡片用 flex-basis + grow：一轮只有一张时它会撑宽一点，不会缩成细条。 -->
        <div v-else class="flex flex-col gap-5">
          <section v-for="section in recordSections" :key="section.key" class="flex flex-col gap-3">
            <h3
              v-if="section.title"
              class="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500"
              :data-testid="`result-group-${section.key}`"
            >
              {{ section.title }}
            </h3>
            <div
              v-for="row in section.rows"
              :key="`${section.key}-${row.key}`"
              class="flex flex-wrap items-stretch gap-4"
              :data-testid="`result-round-${section.key}-${row.key}`"
            >
              <div
                v-for="record in row.records"
                :key="record.id"
                class="flex max-w-[26rem] grow basis-[20rem] flex-col overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900"
              >
                <!-- 状态条 -->
                <div class="flex items-center justify-between gap-2 border-b border-gray-200 px-3 py-2 dark:border-dark-700">
                  <span class="inline-flex items-center gap-1.5 text-xs font-medium" :class="statusClass(record.status)">
                    <Icon :name="statusIcon(record.status)" size="sm" :class="record.status === 'generating' ? 'animate-spin' : ''" />
                    {{ statusLabel(record.status) }}
                  </span>
                  <span class="text-xs text-gray-400 dark:text-gray-500">{{ formatTime(record.createdAt) }}</span>
                </div>

                <!-- 图片 / 失败原因 -->
                <div class="flex min-h-[16rem] items-center justify-center p-2">
                  <!-- 一张卡片里有多张图时也横向排：主人明确要求不要竖着排。
                       外面这层是卡片，本来就窄，所以允许换行，避免挤成一条。 -->
                  <div v-if="record.images.length" class="flex w-full flex-wrap justify-center gap-2">
                    <img
                      v-for="(url, index) in record.images"
                      :key="index"
                      :src="url"
                      :alt="record.prompt"
                      class="max-h-[34rem] min-w-0 flex-1 rounded object-contain"
                      :data-testid="`result-image-${index}`"
                    />
                  </div>
                <div v-else-if="record.status === 'failed'" class="px-2 text-center">
                  <Icon name="xCircle" size="lg" class="mx-auto text-red-400" />
                  <p class="mt-2 text-xs font-medium text-red-600 dark:text-red-400">{{ t('aiImage.result.failedReason') }}</p>
                  <p class="mt-1 break-words text-xs text-gray-600 dark:text-gray-300">{{ record.error }}</p>
                </div>
                <div v-else-if="record.status === 'completed'" class="px-3 text-center">
                  <Icon name="clock" size="lg" class="mx-auto text-gray-300 dark:text-dark-600" />
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.result.imageNotRetained') }}</p>
                </div>
                <div v-else class="text-center">
                  <Icon name="refresh" size="lg" class="mx-auto animate-spin text-gray-300 dark:text-dark-600" />
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.filter.generating') }}</p>
                </div>
                </div>

                <!-- 提示词与操作 -->
                <div class="mt-auto border-t border-gray-200 px-3 py-2 dark:border-dark-700">
                  <p
                    v-if="record.productId"
                    class="mb-1 truncate text-xs font-medium text-primary-600 dark:text-primary-400"
                    :title="record.productId"
                  >
                    {{ t('aiImage.product.label') }}：{{ record.productId }}
                  </p>
                  <!-- 提示词完整显示（原来截成 2 行，看不全就得靠悬停提示）。 -->
                  <p
                    class="max-h-40 overflow-y-auto whitespace-pre-wrap break-words text-xs leading-relaxed text-gray-600 dark:text-gray-300"
                    data-testid="record-prompt"
                  >
                    {{ record.prompt }}
                  </p>
                  <div class="mt-2 flex flex-wrap items-center gap-2">
                    <button
                      v-if="record.status === 'failed'"
                      type="button"
                      class="btn btn-secondary btn-sm"
                      @click="retryRecord(record)"
                    >
                      <Icon name="refresh" size="sm" class="mr-1" />
                      {{ t('aiImage.result.retry') }}
                    </button>
                    <template v-if="record.images.length">
                      <!-- 图片地址是给客户端用的：客户端拿它下载到 images/<商品编号>/<序号>.jpg -->
                      <button
                        v-if="isPersistableImageUrl(record.images[0])"
                        type="button"
                        class="btn btn-primary btn-sm"
                        @click="copyImageUrl(record)"
                      >
                        <Icon name="link" size="sm" class="mr-1" />
                        {{ t('aiImage.product.copyUrl') }}
                      </button>
                      <button type="button" class="btn btn-secondary btn-sm" @click="downloadRecord(record)">
                        <Icon name="download" size="sm" class="mr-1" />
                        {{ t('aiImage.result.download') }}
                      </button>
                      <!-- 水印只在下载时叠加，成图上只有这一层水印。 -->
                      <button
                        v-if="watermark.enabled"
                        type="button"
                        class="btn btn-secondary btn-sm"
                        :disabled="watermarking"
                        @click="downloadWithWatermark(record)"
                      >
                        <Icon name="download" size="sm" class="mr-1" />
                        {{ t('aiImage.watermark.download') }}
                      </button>
                      <a
                        :href="record.images[0]"
                        target="_blank"
                        rel="noopener noreferrer"
                        class="btn btn-secondary btn-sm"
                      >
                        {{ t('aiImage.result.openInNewTab') }}
                      </a>
                    </template>
                    <button type="button" class="btn btn-secondary btn-sm" @click="removeRecord(record)">
                      <Icon name="trash" size="sm" class="mr-1" />
                      {{ t('aiImage.result.remove') }}
                    </button>
                  </div>
                  <p
                    v-if="record.images.length && !isPersistableImageUrl(record.images[0])"
                    class="mt-1 text-xs text-amber-600 dark:text-amber-400"
                  >
                    {{ t('aiImage.product.noUrl') }}
                  </p>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  AI_IMAGE_DEFAULT_MODEL,
  AI_IMAGE_DEFAULT_SIZE,
  AI_IMAGE_MODEL_OPTIONS,
  AIImageError,
  extractSellingPoints,
  analyzeHotStyle,
  generateAIImage,
  isImagePermissionError,
  isPersistableImageUrl,
  normalizeAIImageModel,
  toBlob,
  toDisplayUrl,
  validateAIImageKey,
} from '@/api/aiImage'
import {
  DEFAULT_SCENE,
  OZON_SIZE_OPTIONS,
  PRODUCT_LOCK,
  VISION_DEFAULT_MODEL,
  VISION_MODEL_OPTIONS,
  autoMatchScene,
  buildOzonScript,
  type OzonScriptCard,
} from '@/api/aiImagePrompts'
import {
  findTemplate,
  loadTemplates,
  persistTemplates,
  removeTemplate,
  upsertTemplate,
  MAX_TEMPLATE_NAME,
  type PromptTemplate,
} from '@/api/aiImageTemplates'
import {
  DEFAULT_WATERMARK,
  WATERMARK_ALPHA_MAX,
  WATERMARK_ALPHA_MIN,
  watermarkImageBlob,
  type WatermarkOptions,
} from '@/api/aiImageWatermark'
import {
  DEFAULT_COMPOSITE,
  MAX_COMPOSITE_IMAGES,
  compositeImageBlobs,
} from '@/api/aiImageComposite'
import {
  HOT_ANALYZE_PROMPT,
  HOT_ANALYZE_TEMPERATURE,
  buildHotExtraCards,
  HOT_VISION_TIMEOUT_MS,
  buildHotScript,
  extractJsonObject,
  mergeStyleSamples,
  normalizeStyleSample,
  type HotStyleDimensionKey,
} from '@/api/aiImageHot'

/**
 * 尺寸下拉：前三个是桌面程序 Ozon 主图页的三个 3:4 竖版尺寸，
 * 后面三个是「单图自由生成」页的尺寸，一起放方便按需选。
 */
const SIZE_CHOICES = [
  ...OZON_SIZE_OPTIONS,
  { label: '1024 × 1536（2:3 竖版）', value: '1024x1536' },
  { label: '1024 × 1024（1:1 方图）', value: '1024x1024' },
  { label: '1536 × 1024（3:2 横版）', value: '1536x1024' },
]

type RecordStatus = 'generating' | 'completed' | 'failed'
type KeyState = 'empty' | 'valid' | 'invalid' | 'not-image-model' | 'not-owned' | 'load-failed'

interface AIImageRecord {
  id: string
  prompt: string
  model: string
  size: string
  status: RecordStatus
  images: string[]
  error: string
  createdAt: number
  /** 生成时是否开启了商品一致性锁定（重试时要还原成同样的输入）。 */
  locked?: boolean
  /** 对应的商品编号；客户端按 images/<product_id>/<index>.jpg 认图 */
  productId?: string
  /**
   * 提交顺序，单调递增。
   *
   * 为什么不能只用 createdAt：批量生成时几个请求是并发发出去的，
   * createdAt 可能撞在同一毫秒，排序就不稳定了。
   * 而客户端是按地址的先后顺序落成第 1、2、3 张的。
   * 顺序一乱，上架时主图顺序就错了。所以用这个字段兜住顺序。
   */
  seq?: number
  /**
   * 第几轮生成。
   *
   * 主人要求「进行下一个图片处理的时候，不要显示之前进行的图片」，
   * 定的做法是：**旧图保留，但当前这轮排在最前面**。
   * 一次批量（或一次单张生成）= 一轮，同一轮的记录共享同一个 roundId。
   * 结果区据此分成「本轮生成」和「历史记录」两段。
   */
  roundId?: number
}

const KEY_STORAGE = 'sub2api_ai_image_key'
const MODEL_STORAGE = 'sub2api_ai_image_model'
const SIZE_STORAGE = 'sub2api_ai_image_size'
const RECORDS_STORAGE = 'sub2api_ai_image_records'
const PRODUCT_ID_STORAGE = 'sub2api_ai_image_product_id'

const { t } = useI18n()

const keyInput = ref('')
const apiKey = ref('')
const keyState = ref<KeyState>('empty')
const keyGroupName = ref('')
const keyChecking = ref(false)
const showKeyPanel = ref(false)

const prompt = ref('')
const model = ref(AI_IMAGE_DEFAULT_MODEL)
const size = ref(AI_IMAGE_DEFAULT_SIZE)
const count = ref(1)
const generating = ref(false)
const statusFilter = ref<'all' | RecordStatus>('all')

/**
 * 提示词模板（桌面程序的「提示词模板」功能）。
 * 只存本机浏览器，方便把常用的提示词反复套用，不用每次重打。
 */
const templates = ref<PromptTemplate[]>([])
const selectedTemplateId = ref('')
const templateName = ref('')
const showTemplateSave = ref(false)
const templateNotice = ref('')

const referenceImage = ref<File | null>(null)
const referencePreviewUrl = ref('')
const fileInputRef = ref<HTMLInputElement | null>(null)

/**
 * 一图多视（桌面程序的「拼接」）。
 * 选了多张时先横向拼成一张长图，再作为第 1 张图的参考图送进去，
 * 让模型一次看全商品的正面/侧面/背面。
 */
const referenceFiles = ref<File[]>([])
const multiFileInputRef = ref<HTMLInputElement | null>(null)
const compositing = ref(false)
const compositeNotice = ref('')

/**
 * 水印（桌面程序的「水印」）。
 *
 * 一个重要的取舍：桌面程序第 2 张起拿「第 1 张的成品」当参考图，
 * 而那张**已经打过水印**，水印会被 AI 看到、可能被复刻进后续图，
 * 最终图上就会叠两层水印。
 * 这里改成**参考图一律用没打水印的干净图**，
 * 水印只在最后产物上打一次 —— 这样成图上只有一层水印。
 */
const watermark = ref<WatermarkOptions>({ ...DEFAULT_WATERMARK })
const showEnhancePanel = ref(false)
const watermarking = ref(false)
const watermarkNotice = ref('')

/**
 * 功能模块面板的开关。这些都放在页面最上面一排，点一下就展开对应面板。
 * 放在顶部是因为它们是「先决定怎么干」的入口，比提示词输入框更该先看到。
 */
const showTemplatePanel = ref(false)
const showCompositePanel = ref(false)

/**
 * 爆款复刻（桌面程序的「爆款复刻模式」）。
 * 拆同行的爆款版式 → 套到自己的商品上。
 *
 * 注意：这一套**自带产品信息输入**，不依赖「套图」面板。
 * 只要拆出了版式（或者套了一个已存的模板），填上产品名就能直接生成，
 * 不需要先去把 Ozon 脚本那一步做一遍。
 */
const showHotPanel = ref(false)
const hotRefFiles = ref<File[]>([])
const hotRefInputRef = ref<HTMLInputElement | null>(null)
const hotAnalyzing = ref(false)
const hotProgress = ref({ done: 0, total: 0 })
const hotStyleDesc = ref('')
const hotNotice = ref('')
const hotError = ref('')
const hotIncludeExtras = ref(false)
const hotForm = ref({
  product: '',
  sp1: '',
  sp2: '',
  sp3: '',
  sp1Ru: '',
  sp2Ru: '',
  sp3Ru: '',
  pain: '',
  diff: '',
})

/** 当前这组脚本卡片是谁生成的（界面上标一下，免得两个入口的卡片混在一起分不清）。 */
const scriptSource = ref<'ozon' | 'hot-extracted' | 'hot-saved' | ''>('')

/**
 * 商品编号。客户端按 images/<商品编号>/<序号>.jpg 存放和识别图片，
 * 所以生成时带上它，图才能和客户端显示的商品对上号。
 */
const productId = ref('')
/** 复制图片地址后的提示 */
const copyNotice = ref('')

/**
 * 商品一致性锁定：默认开启。
 * 开启且传了参考图时，会在提示词最前面拼上 PRODUCT_LOCK，
 * 让模型只改背景/光线/排版/文字，商品本身保持不变。
 */
const productLock = ref(true)

/** 套图面板的开关。界面叫「套图」，产出的是 Ozon 八张主图脚本。 */
const showScriptPanel = ref(false)
/**
 * 产品信息表单。
 *
 * `sp1/sp2/sp3` 是**中文**，给界面看（主人要求识别结果显示中文）。
 * `sp1Ru/sp2Ru/sp3Ru` 是**俄语**，才是真正印在图上那段文案 —— Ozon/WB 是俄语市场。
 * 手填中文时没有俄语版，就退回用中文（用户自己知道图上会是中文）。
 */
const scriptForm = ref({
  product: '',
  sp1: '',
  sp2: '',
  sp3: '',
  sp1Ru: '',
  sp2Ru: '',
  sp3Ru: '',
  pain: '',
  diff: '',
})
const scriptCards = ref<OzonScriptCard[]>([])
const aiExtracting = ref(false)
const scriptNotice = ref('')
const scriptError = ref('')

const visionModel = ref(VISION_DEFAULT_MODEL)

/**
 * 批量生成（桌面程序的「批量任务(并发)」）。
 *
 * 为什么需要它：八张主图脚本一次要出八张图，一张一张点太慢。
 * 桌面程序允许 1/2/3 个并发同时跑，还能暂停和停止。
 * 这里的并发数刻意**上限 3**——和桌面程序一致：
 * 生图接口又慢又贵，开太多并发容易被上游限流，也容易一次烧掉太多额度。
 */
const BATCH_CONCURRENCY_OPTIONS = [1, 2, 3] as const
const batchConcurrency = ref<number>(2)
const batchRunning = ref(false)
const batchPaused = ref(false)
/** 暂停时用：等待被唤醒。 */
let batchResume: (() => void) | null = null
/** 停止标记：worker 每轮检查一次，见到就退出。 */
let batchAborted = false
const batchProgress = ref({ done: 0, total: 0, failed: 0 })

const records = ref<AIImageRecord[]>([])
// 记录 id → 该次生成用的参考图。只存在内存里（File 无法序列化）。
// 用来保证「重试」用的是同一组输入，而不是当前碰巧选中的参考图。
const referenceByRecord = new Map<string, File>()

/**
 * 提交顺序发号器。批量生成时并发提交，createdAt 可能撞在同一毫秒。
 * 所以用这个单调递增的号来保证「谁先提交」是确定的。
 * 从已有记录的最大值之后接着发，刷新后也不会和旧记录串号。
 */
let seqCounter = 0
function nextSeq(): number {
  seqCounter += 1
  return seqCounter
}

/**
 * 轮次发号器。一次批量 / 一次单张生成 = 一轮。
 * 当前这轮的记录会排在结果区最前面，旧的沉到「历史记录」里。
 */
let roundCounter = 0
const currentRoundId = ref(0)
function nextRound(): number {
  roundCounter += 1
  currentRoundId.value = roundCounter
  return roundCounter
}

/** 按提交顺序（早→晚）排。没有 seq 的旧记录回退用 createdAt。 */
function bySubmitOrder(a: AIImageRecord, b: AIImageRecord): number {
  const left = a.seq ?? a.createdAt
  const right = b.seq ?? b.createdAt
  if (left !== right) return left - right
  return a.createdAt - b.createdAt
}

/** 产品名变化时自动配场景，显示给用户看。 */
const matchedScene = computed(() => autoMatchScene(scriptForm.value.product))

/**
 * 界面上显示中文，图上印俄语。
 * 有俄语版就用俄语，没有（用户手填的中文）就退回中文 —— 用户自己知道图上会是中文。
 */
function printedText(chinese: string, russian: string): string {
  return (russian || '').trim() || (chinese || '').trim()
}

/** 这一组「图上真正会出现的俄语文案」，显示出来让用户核对。 */
const printedSellingPoints = computed(() =>
  [scriptForm.value.sp1Ru, scriptForm.value.sp2Ru, scriptForm.value.sp3Ru].map((item) => (item || '').trim()),
)

const hasPrintedRussian = computed(() => printedSellingPoints.value.some(Boolean))

const scriptSceneText = computed(() => matchedScene.value.zh || DEFAULT_SCENE[1])

const keyErrorTitle = computed(() => {
  switch (keyState.value) {
    case 'not-image-model':
      return t('aiImage.key.notImageModel')
    case 'not-owned':
      return t('aiImage.key.invalid')
    case 'load-failed':
      return t('aiImage.key.loadFailed')
    default:
      return t('aiImage.key.invalid')
  }
})

const filteredRecords = computed(() => {
  if (statusFilter.value === 'all') return records.value
  return records.value.filter((record) => record.status === statusFilter.value)
})

/**
 * 结果区按「本轮 / 历史」分两段。
 *
 * 主人要求：进行下一个图片处理时不要把之前那批图混在一起看。
 * 做法是旧图**保留**（不删数据），但当前这轮单独拎出来放最前面。
 * 一眼就能分清哪几张是刚生成的。
 */
const currentRoundRecords = computed(() =>
  filteredRecords.value.filter((record) => record.roundId !== undefined && record.roundId === currentRoundId.value),
)

const historyRecords = computed(() => {
  const current = currentRoundRecords.value
  if (!current.length) return filteredRecords.value
  const currentIds = new Set(current.map((record) => record.id))
  return filteredRecords.value.filter((record) => !currentIds.has(record.id))
})

/** 有没有「本轮」和「历史」两段同时存在（只有一段时不用显示分组标题）。 */
const hasBothRounds = computed(() => currentRoundRecords.value.length > 0 && historyRecords.value.length > 0)

/** 结果区分组：两段都在时分「本轮生成 / 历史记录」，否则就一段不分组。 */
const recordGroups = computed(() => {
  if (!hasBothRounds.value) {
    return [{ key: 'all', title: '', records: filteredRecords.value }]
  }
  return [
    { key: 'current', title: t('aiImage.result.currentRound'), records: currentRoundRecords.value },
    { key: 'history', title: t('aiImage.result.history'), records: historyRecords.value },
  ]
})

/**
 * 把一组记录按「轮次」切成若干行。
 *
 * 主人要求：图片横向排列，一次生成之后下一次单开一行。
 * 所以**一轮 = 一行**：一轮的卡片排在同一个横向容器里，排满自动换行。
 *
 * 记录本来就是按轮次相邻排列的（新的在前），所以只要顺序扫一遍、
 * 遇到轮次变了就换行即可，不需要排序。没有 roundId 的旧记录归到同一行——
 * 它们本来就没有轮次的概念，硬拆反而会拆出一堆孤零零的行。
 */
function groupIntoRoundRows(records: AIImageRecord[]): Array<{ key: string; records: AIImageRecord[] }> {
  const rows: Array<{ key: string; records: AIImageRecord[] }> = []
  for (const record of records) {
    const key = record.roundId === undefined ? 'legacy' : `round-${record.roundId}`
    const last = rows[rows.length - 1]
    if (last && last.key === key) last.records.push(record)
    else rows.push({ key, records: [record] })
  }
  return rows
}

/** 结果区最终结构：段（本轮/历史）→ 行（每一轮）→ 卡片。 */
const recordSections = computed(() =>
  recordGroups.value.map((group) => ({
    key: group.key,
    title: group.title,
    rows: groupIntoRoundRows(group.records),
  })),
)

function statusLabel(status: RecordStatus): string {
  if (status === 'completed') return t('aiImage.filter.completed')
  if (status === 'failed') return t('aiImage.filter.failed')
  return t('aiImage.filter.generating')
}

function statusIcon(status: RecordStatus): 'checkCircle' | 'xCircle' | 'refresh' {
  if (status === 'completed') return 'checkCircle'
  if (status === 'failed') return 'xCircle'
  return 'refresh'
}

function statusClass(status: RecordStatus): string {
  if (status === 'completed') return 'text-emerald-600 dark:text-emerald-400'
  if (status === 'failed') return 'text-red-600 dark:text-red-400'
  return 'text-primary-600 dark:text-primary-400'
}

function formatTime(timestamp: number): string {
  try {
    return new Date(timestamp).toLocaleString()
  } catch {
    return ''
  }
}

/**
 * 单次生成的张数上限。
 * 主人要求「可以自由的选择图片数量」，所以从 4 放宽到 10 ——
 * 但仍留一道闸门：再往上就是成倍烧额度、也容易被上游限流。
 */
const MAX_GENERATE_COUNT = 10

/**
 * 张数输入框旁边那句「最多可生成 N 张」里的 N。
 *
 * 做成 computed 而不是把 10 写死在模板里：上限将来要按「放进去几张图」分档时，
 * 只改这一处，输入框的 max、夹取逻辑、提示文案三处会一起跟着变，不会对不上。
 *
 * 目前是**单张和多张同一个上限**——查过网关和上游的能力判定，
 * n 的限制只跟「账号是否具备 images-native 能力」有关，跟传了几张参考图无关。
 * 所以现在只有这一档。（要按参考图张数分档的话，得先确认真实模型那边的口径。）
 */
const maxGenerateCount = computed(() => MAX_GENERATE_COUNT)

/** 张数输入框可能被清空成 NaN，这里夹在 1～10 之间再发请求。 */
function normalizedCount(): number {
  const value = Math.floor(Number(count.value))
  if (!Number.isFinite(value)) return 1
  return Math.min(MAX_GENERATE_COUNT, Math.max(1, value))
}

function persistRecords() {
  try {
    // 只保留最近 60 条；并且把 base64 图片剔除后再落盘——data URL 动辄几 MB，
    // 写进 localStorage 会立刻撑爆配额。URL 形式的图片才持久化。
    const trimmed = records.value.slice(0, 60)
    records.value = trimmed
    const serializable = trimmed.map((record) => ({
      ...record,
      images: record.images.filter(isPersistableImageUrl),
    }))
    window.localStorage.setItem(RECORDS_STORAGE, JSON.stringify(serializable))
  } catch {
    // 存储不可用时忽略，页面仍可正常使用。
  }
}

function loadRecords() {
  try {
    const raw = window.localStorage.getItem(RECORDS_STORAGE)
    if (!raw) return
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed)) records.value = parsed as AIImageRecord[]
  } catch {
    records.value = []
  }
  // 发号器接着已有记录的最大值往后发，刷新后不会和旧记录串号。
  const maxSeq = records.value.reduce((max, record) => Math.max(max, record.seq ?? 0), 0)
  if (maxSeq > seqCounter) seqCounter = maxSeq
  const maxRound = records.value.reduce((max, record) => Math.max(max, record.roundId ?? 0), 0)
  if (maxRound > roundCounter) roundCounter = maxRound
}

function clearReference() {
  referenceImage.value = null
  if (referencePreviewUrl.value) {
    try {
      URL.revokeObjectURL(referencePreviewUrl.value)
    } catch {
      // 预览地址释放失败不影响使用。
    }
  }
  referencePreviewUrl.value = ''
  if (fileInputRef.value) fileInputRef.value.value = ''
}

function onReferencePicked(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0] || null
  clearReference()
  if (!file) return
  referenceImage.value = file
  // 缩略图只是给人看的：生成缩略图失败不能连带把「选好的图」也丢掉。
  try {
    referencePreviewUrl.value = URL.createObjectURL(file)
  } catch {
    referencePreviewUrl.value = ''
  }
}

/** 选多张图 → 横向拼成一张，作为参考图。 */
async function onMultiReferencePicked(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  if (!files.length) return

  if (files.length > MAX_COMPOSITE_IMAGES) {
    compositeNotice.value = t('aiImage.composite.tooMany', { max: MAX_COMPOSITE_IMAGES })
  } else {
    compositeNotice.value = ''
  }

  referenceFiles.value = files.slice(0, MAX_COMPOSITE_IMAGES)
  compositing.value = true
  try {
    const merged = await compositeImageBlobs(referenceFiles.value, DEFAULT_COMPOSITE)
    // 拼接结果当成「参考图」走原有流程，后面的生成逻辑不用改。
    const file = new File([merged], 'composite-reference.png', { type: 'image/png' })
    clearReference()
    referenceImage.value = file
    try {
      referencePreviewUrl.value = URL.createObjectURL(file)
    } catch {
      referencePreviewUrl.value = ''
    }
    compositeNotice.value = t('aiImage.composite.done', { count: referenceFiles.value.length })
  } catch (error) {
    compositeNotice.value = t('aiImage.composite.failed', {
      reason: error instanceof Error ? error.message : String(error),
    })
  } finally {
    compositing.value = false
    if (multiFileInputRef.value) multiFileInputRef.value.value = ''
  }
}

/**
 * 给一张图片地址加水印并下载。
 *
 * 注意：生成的图在上游的 CDN 上，浏览器直接 fetch 可能被跨域策略挡住。
 * 挡不住就直接下载原图并明确告诉用户，而不是静默失败。
 */
async function downloadWithWatermark(record: AIImageRecord) {
  const url = record.images[0]
  if (!url) return
  watermarkNotice.value = ''
  watermarking.value = true
  try {
    const response = await fetch(url)
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const source = await response.blob()
    const output = await watermarkImageBlob(source, watermark.value)
    const objectUrl = URL.createObjectURL(output)
    const link = document.createElement('a')
    link.href = objectUrl
    link.download = `${record.productId || 'ai-image'}-${record.id}.jpg`
    link.click()
    // 下载是异步的，稍后再释放地址，否则可能下到空文件。
    setTimeout(() => URL.revokeObjectURL(objectUrl), 10000)
    watermarkNotice.value = t('aiImage.watermark.done')
  } catch {
    // 跨域被挡时退化成直接下载原图，并说清楚原因。
    const link = document.createElement('a')
    link.href = url
    link.download = ''
    link.target = '_blank'
    link.rel = 'noopener noreferrer'
    link.click()
    watermarkNotice.value = t('aiImage.watermark.corsBlocked')
  } finally {
    watermarking.value = false
  }
}

async function saveKey() {
  const value = keyInput.value.trim()
  if (!value) {
    keyState.value = 'invalid'
    return
  }
  keyChecking.value = true
  try {
    const result = await validateAIImageKey(value)
    if (result.ok) {
      apiKey.value = value
      keyGroupName.value = result.groupName || ''
      keyState.value = 'valid'
      showKeyPanel.value = false
      try {
        window.localStorage.setItem(KEY_STORAGE, value)
      } catch {
        // 忽略存储失败
      }
      return
    }
    apiKey.value = ''
    keyGroupName.value = result.groupName || ''
    keyState.value = result.reason === 'not-image-model' ? 'not-image-model' : (result.reason as KeyState)
  } finally {
    keyChecking.value = false
  }
}

function clearKey() {
  apiKey.value = ''
  keyInput.value = ''
  keyGroupName.value = ''
  keyState.value = 'empty'
  showKeyPanel.value = true
  try {
    window.localStorage.removeItem(KEY_STORAGE)
  } catch {
    // 忽略
  }
}

function errorMessage(error: unknown): string {
  if (error instanceof AIImageError) {
    if (isImagePermissionError(error)) return t('aiImage.errors.noImagePermission')
    if (error.status === 401) return t('aiImage.errors.badKey')
    if (error.status === 429) return t('aiImage.errors.rateLimited')
    if (error.status === 402 || error.status === 403) return t('aiImage.errors.insufficient')
    return error.message || t('aiImage.errors.unknown')
  }
  if (error instanceof TypeError) return t('aiImage.errors.network')
  if (error instanceof Error && error.message) return error.message
  return t('aiImage.errors.unknown')
}

/**
 * 拼出发给模型的最终提示词。
 * 有参考图且开了锁定时，在最前面加上商品一致性锁定——
 * 这是「生成的图和客户端显示的商品是同一件」的保障。
 */
function buildEffectivePrompt(record: AIImageRecord, reference: File | null): string {
  if (record.locked && reference) return `${PRODUCT_LOCK}${record.prompt}`
  return record.prompt
}

async function runGeneration(record: AIImageRecord, reference: File | null, imageCount?: number) {
  record.status = 'generating'
  record.error = ''
  record.images = []
  persistRecords()

  try {
    const response = await generateAIImage({
      apiKey: apiKey.value,
      prompt: buildEffectivePrompt(record, reference),
      model: record.model,
      size: record.size,
      n: imageCount ?? normalizedCount(),
      referenceImage: reference,
    })
    const images = (response.data || []).map(toDisplayUrl).filter(Boolean)
    if (images.length === 0) {
      record.status = 'failed'
      record.error = t('aiImage.errors.unknown')
    } else {
      record.images = images
      record.status = 'completed'
    }
  } catch (error) {
    record.status = 'failed'
    record.error = errorMessage(error)
    if (error instanceof AIImageError && error.status === 401) {
      keyState.value = 'invalid'
    } else if (isImagePermissionError(error)) {
      keyState.value = 'not-image-model'
    }
  } finally {
    persistRecords()
  }
}

/** 建一条待生成的记录。批量与单张共用，保证字段完全一致。 */
function newRecord(text: string, roundId?: number): AIImageRecord {
  return {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    prompt: text,
    model: model.value || AI_IMAGE_DEFAULT_MODEL,
    size: size.value || AI_IMAGE_DEFAULT_SIZE,
    status: 'generating',
    images: [],
    error: '',
    createdAt: Date.now(),
    locked: productLock.value,
    productId: productId.value.trim(),
    seq: nextSeq(),
    roundId: roundId ?? currentRoundId.value,
  }
}

// ==================== 提示词模板 ====================

/** 把选中的模板套用进提示词输入框。 */
function applyTemplate() {
  const found = findTemplate(templates.value, selectedTemplateId.value)
  if (!found) return
  prompt.value = found.prompt
  templateNotice.value = t('aiImage.template.applied', { name: found.name })
}

function openTemplateSave() {
  if (!prompt.value.trim()) {
    templateNotice.value = t('aiImage.template.needPrompt')
    return
  }
  // 默认拿提示词开头当名字，用户不用自己想名字。
  templateName.value = prompt.value.trim().slice(0, MAX_TEMPLATE_NAME)
  showTemplateSave.value = true
  templateNotice.value = ''
}

function confirmTemplateSave() {
  const name = templateName.value.trim()
  if (!name) {
    templateNotice.value = t('aiImage.template.needName')
    return
  }
  const before = templates.value.length
  templates.value = upsertTemplate(templates.value, name, prompt.value)
  const stored = persistTemplates(templates.value)
  // 同名是覆盖：条数不变，但同样算成功。
  const overwritten = templates.value.length === before
  templateNotice.value = stored
    ? t(overwritten ? 'aiImage.template.overwritten' : 'aiImage.template.saved', { name })
    : t('aiImage.template.saveFailed')
  // 存完就选中它：不然刚存下的模板没被选上，爆款复刻那边会以为「没有模板」。
  const saved = templates.value.find((item) => item.name === name)
  if (saved) selectedTemplateId.value = saved.id
  showTemplateSave.value = false
  templateName.value = ''
}

function cancelTemplateSave() {
  showTemplateSave.value = false
  templateName.value = ''
}

function deleteSelectedTemplate() {
  const found = findTemplate(templates.value, selectedTemplateId.value)
  if (!found) return
  templates.value = removeTemplate(templates.value, found.id)
  persistTemplates(templates.value)
  selectedTemplateId.value = ''
  templateNotice.value = t('aiImage.template.deleted', { name: found.name })
}

// ==================== 爆款复刻 ====================

/** 选爆款参考图（可以多选，追加式）。 */
function onHotRefPicked(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  if (files.length) {
    hotRefFiles.value = [...hotRefFiles.value, ...files]
    hotNotice.value = t('aiImage.hot.picked', { count: hotRefFiles.value.length })
  }
  if (hotRefInputRef.value) hotRefInputRef.value.value = ''
}

function clearHotRef() {
  hotRefFiles.value = []
  hotStyleDesc.value = ''
  hotNotice.value = ''
  hotError.value = ''
  hotProgress.value = { done: 0, total: 0 }
}

/**
 * 拆解爆款版式：逐张送视觉模型，再把多张的结果按维度投票合并。
 *
 * 逐张串行是有意的：桌面程序也是串行的（每张之间还停 0.3 秒）。
 * 并发打视觉模型容易被限流，而且这里本来就慢，串行更好排查是哪张出的问题。
 */
async function runHotAnalyze() {
  if (!apiKey.value || keyState.value !== 'valid') {
    showKeyPanel.value = true
    return
  }
  if (!hotRefFiles.value.length) {
    hotError.value = t('aiImage.hot.needRef')
    return
  }

  hotAnalyzing.value = true
  hotError.value = ''
  hotNotice.value = ''
  hotStyleDesc.value = ''
  hotProgress.value = { done: 0, total: hotRefFiles.value.length }

  const samples: Array<Partial<Record<HotStyleDimensionKey, string>>> = []
  try {
    for (const file of hotRefFiles.value) {
      try {
        const raw = await analyzeHotStyleRequest(file)
        const sample = normalizeStyleSample(extractJsonObject(raw))
        if (Object.keys(sample).length) samples.push(sample)
      } catch (error) {
        // 单张失败不中断整批：其余几张照样能投票出结果。
        hotError.value = t('aiImage.hot.oneFailed', {
          reason: error instanceof Error ? error.message : String(error),
        })
      }
      hotProgress.value = { ...hotProgress.value, done: hotProgress.value.done + 1 }
      // 每张之间歇一下，别把视觉接口打急。
      await new Promise((resolve) => setTimeout(resolve, 300))
    }

    const merged = mergeStyleSamples(samples)
    if (!merged.description) {
      hotError.value = t('aiImage.hot.noResult')
      return
    }
    hotStyleDesc.value = merged.description
    hotNotice.value = t('aiImage.hot.analyzed', { count: merged.used })
  } finally {
    hotAnalyzing.value = false
  }
}

/** 抽出来单独一层，方便测试替换。 */
async function analyzeHotStyleRequest(file: File): Promise<string> {
  return analyzeHotStyle({
    apiKey: apiKey.value,
    image: file,
    model: visionModel.value,
    prompt: HOT_ANALYZE_PROMPT,
    temperature: HOT_ANALYZE_TEMPERATURE,
    timeoutMs: HOT_VISION_TIMEOUT_MS,
  })
}

/**
 * 爆款复刻用的「模板」——也就是套到商品上的那套版式/写法。
 * 两个来源，有哪个用哪个：
 *   1. 拆解爆款图得到的版式（hotStyleDesc）。
 *   2. 「提示词模板」里选中的那个模板的提示词。
 * 都没有就没法复刻（会明确提示），而不是悄悄生成一套没版式的脚本。
 */
const hotTemplate = computed(() => {
  if (hotStyleDesc.value.trim()) {
    return { text: hotStyleDesc.value.trim(), from: 'extracted' as const }
  }
  const saved = findTemplate(templates.value, selectedTemplateId.value)
  if (saved && saved.prompt.trim()) {
    return { text: saved.prompt.trim(), from: 'saved' as const }
  }
  return null
})

/**
 * 用爆款版式生成脚本卡片。
 *
 * 只用**爆款复刻面板自己那套**产品信息，不去读「套图」那份表单——
 * 否则用户必须先打开、填完那个面板才能用爆款复刻，这就是主人反馈的逻辑问题。
 * 产品名在爆款面板里没填时，才退回去用 Ozon 面板已填的值（省得重复输入）。
 */
function buildHotScriptCards() {
  const style = hotTemplate.value
  if (!style) {
    hotError.value = t('aiImage.hot.needTemplate')
    return
  }
  const product = (hotForm.value.product || scriptForm.value.product).trim()
  if (!product) {
    hotError.value = t('aiImage.hot.needProduct')
    return
  }
  hotError.value = ''
  const pick = (hotValue: string, scriptValue: string) => (hotValue || '').trim() || scriptValue
  // 图上印俄语：爆款面板填了就用它的，没填就用 Ozon 面板的；都没有俄语版才退回中文。
  const printed = (zhHot: string, ruHot: string, zhScript: string, ruScript: string) =>
    printedText(pick(zhHot, zhScript), pick(ruHot, ruScript))
  const cards = buildHotScript({
    product,
    sp1: printed(hotForm.value.sp1, hotForm.value.sp1Ru, scriptForm.value.sp1, scriptForm.value.sp1Ru),
    sp2: printed(hotForm.value.sp2, hotForm.value.sp2Ru, scriptForm.value.sp2, scriptForm.value.sp2Ru),
    sp3: printed(hotForm.value.sp3, hotForm.value.sp3Ru, scriptForm.value.sp3, scriptForm.value.sp3Ru),
    pain: pick(hotForm.value.pain, scriptForm.value.pain),
    diff: pick(hotForm.value.diff, scriptForm.value.diff),
    styleDesc: style.text,
    sceneZh: matchedScene.value.zh,
  })
  scriptCards.value = hotIncludeExtras.value ? [...cards, ...buildHotExtraCards(product)] : cards
  scriptSource.value = style.from === 'saved' ? 'hot-saved' : 'hot-extracted'
  hotNotice.value = t('aiImage.hot.scriptBuilt', { count: scriptCards.value.length })
}

async function startGenerate() {
  if (!apiKey.value || keyState.value !== 'valid') {
    // 没有可用密钥时把密钥面板打开，让用户先补密钥。
    showKeyPanel.value = true
    return
  }
  const text = prompt.value.trim()
  if (!text) return

  const record = newRecord(text, nextRound())
  records.value = [record, ...records.value]
  persistRecords()

  if (referenceImage.value) referenceByRecord.set(record.id, referenceImage.value)

  generating.value = true
  try {
    await runGeneration(record, referenceImage.value)
  } finally {
    generating.value = false
  }
}

async function retryRecord(record: AIImageRecord) {
  if (generating.value || !apiKey.value) return
  generating.value = true
  try {
    // 用这张记录原本的参考图重试，保证输入一致。
    await runGeneration(record, referenceByRecord.get(record.id) ?? null)
  } finally {
    generating.value = false
  }
}

/** 生成一套 Ozon 八张主图脚本，每张一条完整提示词（界面入口叫「套图」）。 */
/* ------------------------------------------------------------------ *
 * 批量生成（桌面程序的「批量任务(并发)」）
 * ------------------------------------------------------------------ */

/** 暂停时挂起；恢复或停止时唤醒。 */
function waitWhilePaused(): Promise<void> {
  if (!batchPaused.value || batchAborted) return Promise.resolve()
  return new Promise((resolve) => {
    batchResume = resolve
  })
}

function wakeBatch() {
  const resume = batchResume
  batchResume = null
  if (resume) resume()
}

function toggleBatchPause() {
  if (!batchRunning.value) return
  batchPaused.value = !batchPaused.value
  if (!batchPaused.value) wakeBatch()
}

/** 停止：不再领新任务，已经发出去的那个请求让它自己跑完（取消会浪费已花的钱）。 */
function stopBatch() {
  if (!batchRunning.value) return
  batchAborted = true
  batchPaused.value = false
  wakeBatch()
}

/** 把一批提示词并发跑出来，并发数上限 3。 */
async function runBatch(prompts: string[]) {
  const texts = prompts.map((item) => item.trim()).filter(Boolean)
  if (!texts.length) return
  if (!apiKey.value || keyState.value !== 'valid') {
    showKeyPanel.value = true
    return
  }
  if (batchRunning.value) return

  batchRunning.value = true
  batchPaused.value = false
  batchAborted = false
  const reference = referenceImage.value
  const workers = Math.min(batchConcurrency.value, texts.length)
  batchProgress.value = { done: 0, total: texts.length, failed: 0 }

  // 先把所有记录按顺序建好，这样列表里的顺序和提示词顺序一致，
  // 复制地址时的顺序也就和「第 1、2、3 张」对得上。
  // 整批共用同一个 roundId：这一批就是「本轮」，一起排在结果区最前面。
  const roundId = nextRound()
  const queue = texts.map((text) => {
    const record = newRecord(text, roundId)
    if (reference) referenceByRecord.set(record.id, reference)
    return record
  })
  records.value = [...queue.slice().reverse(), ...records.value]
  persistRecords()

  let cursor = 0
  const takeNext = (): AIImageRecord | null => {
    if (batchAborted) return null
    if (cursor >= queue.length) return null
    const item = queue[cursor]
    cursor += 1
    return item
  }

  const worker = async () => {
    for (;;) {
      await waitWhilePaused()
      const record = takeNext()
      if (!record) return
      // 批量里每张固定出 1 张图：脚本本身就是 8 个不同画面，
      // 每张再要 4 张只会成倍花钱，不是这里的意图。
      await runGeneration(record, reference, 1)
      batchProgress.value = {
        ...batchProgress.value,
        done: batchProgress.value.done + 1,
        failed: batchProgress.value.failed + (record.status === 'failed' ? 1 : 0),
      }
    }
  }

  try {
    await Promise.all(Array.from({ length: workers }, () => worker()))
  } finally {
    batchRunning.value = false
    batchPaused.value = false
    batchAborted = false
    batchResume = null
  }
}

/** 一键把八张脚本全部生成出来。 */
async function generateAllScriptCards() {
  if (!scriptCards.value.length) return
  await runBatch(scriptCards.value.map((card) => card.prompt))
}

function buildScript() {
  scriptError.value = ''
  scriptNotice.value = ''
  const product = scriptForm.value.product.trim()
  if (!product) {
    scriptError.value = t('aiImage.script.product')
    return
  }
  scriptCards.value = buildOzonScript(
    {
      product,
      // 图上印俄语；没有俄语版（手填中文）才退回中文。
      sp1: printedText(scriptForm.value.sp1, scriptForm.value.sp1Ru),
      sp2: printedText(scriptForm.value.sp2, scriptForm.value.sp2Ru),
      sp3: printedText(scriptForm.value.sp3, scriptForm.value.sp3Ru),
      pain: scriptForm.value.pain,
      diff: scriptForm.value.diff,
      sceneEn: matchedScene.value.en,
      sceneZh: matchedScene.value.zh,
    },
    // 没有参考图就锁不住商品，不带这段前缀。
    { lockProduct: productLock.value && Boolean(referenceImage.value) },
  )
  scriptSource.value = 'ozon'
  scriptNotice.value = t('aiImage.script.built', { count: scriptCards.value.length })
}

/** 把某一张脚本填进提示词框。 */
function applyScriptCard(card: OzonScriptCard) {
  prompt.value = card.prompt
}

/** 把八张脚本全部填进提示词框（换行分隔，可配合批量生成）。 */
function applyAllScriptCards() {
  prompt.value = scriptCards.value.map((card) => card.prompt).join('\n')
}

/** 让视觉模型看产品图，自动填产品名/卖点/痛点/差异化。 */
async function aiExtract() {
  if (!apiKey.value || keyState.value !== 'valid') {
    showKeyPanel.value = true
    return
  }
  const source = referenceImage.value
  if (!source) {
    scriptError.value = t('aiImage.script.aiNeedsImage')
    return
  }
  aiExtracting.value = true
  scriptError.value = ''
  scriptNotice.value = ''
  try {
    const result = await extractSellingPoints({
      apiKey: apiKey.value,
      image: source,
      model: visionModel.value,
      hints: { 产品名称: scriptForm.value.product },
    })
    if (result.product) scriptForm.value.product = result.product
    // 中文进界面，俄语留给图用。
    if (result.sp1) scriptForm.value.sp1 = result.sp1
    if (result.sp2) scriptForm.value.sp2 = result.sp2
    if (result.sp3) scriptForm.value.sp3 = result.sp3
    if (result.sp1_ru) scriptForm.value.sp1Ru = result.sp1_ru
    if (result.sp2_ru) scriptForm.value.sp2Ru = result.sp2_ru
    if (result.sp3_ru) scriptForm.value.sp3Ru = result.sp3_ru
    if (result.pain) scriptForm.value.pain = result.pain
    if (result.diff) scriptForm.value.diff = result.diff
    scriptNotice.value = t('aiImage.script.aiExtracted')
  } catch (error) {
    scriptError.value = `${t('aiImage.script.aiFailed')}：${errorMessage(error)}`
  } finally {
    aiExtracting.value = false
  }
}

function clearScript() {
  scriptForm.value = { product: '', sp1: '', sp2: '', sp3: '', sp1Ru: '', sp2Ru: '', sp3Ru: '', pain: '', diff: '' }
  scriptCards.value = []
  scriptNotice.value = ''
  scriptError.value = ''
}

async function downloadRecord(record: AIImageRecord) {
  const source = record.images[0]
  if (!source) return
  try {
    const blob = source.startsWith('data:')
      ? await toBlob({ b64_json: source.split(',')[1] })
      : await toBlob({ url: source })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `ai-image-${record.id}.png`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  } catch {
    window.open(source, '_blank', 'noopener,noreferrer')
  }
}

function removeRecord(record: AIImageRecord) {
  records.value = records.value.filter((item) => item.id !== record.id)
  referenceByRecord.delete(record.id)
  persistRecords()
}

/**
 * 复制图片地址，方便直接粘给客户端。
 * 客户端就是拿这个地址下载到 images/<商品编号>/<序号>.jpg 的。
 */
async function copyImageUrl(record: AIImageRecord) {
  const source = record.images[0]
  if (!source) return
  copyNotice.value = ''
  try {
    await navigator.clipboard.writeText(source)
    copyNotice.value = t('aiImage.product.copied')
  } catch {
    copyNotice.value = t('aiImage.product.copyFailed')
  }
}

/**
 * 可交给客户端的图片地址，按生成先后（早→晚）排。
 *
 * 排序很关键：客户端把地址按粘贴顺序落成 images/<商品编号>/0.png、1.png…
 * 所以「第 1 张脚本生成的图」必须排在最前面，才能对上商品主图的第 1 个位置。
 * 这里刻意不跟随列表的倒序展示，只跟随当前状态下拉筛选。
 */
const copyableUrls = computed(() =>
  filteredRecords.value
    .slice()
    .sort(bySubmitOrder)
    .flatMap((record) => record.images.filter((url) => isPersistableImageUrl(url))),
)

/** 一次性把当前筛选下所有可用地址按顺序复制出来，粘一次就能上满一组主图。 */
async function copyAllImageUrls() {
  const urls = copyableUrls.value
  if (!urls.length) return
  copyNotice.value = ''
  try {
    await navigator.clipboard.writeText(urls.join('\n'))
    copyNotice.value = t('aiImage.product.copiedAll', { count: urls.length })
  } catch {
    copyNotice.value = t('aiImage.product.copyFailed')
  }
}

watch(model, (value) => {
  try {
    window.localStorage.setItem(MODEL_STORAGE, value)
  } catch {
    // 忽略
  }
})

watch(size, (value) => {
  try {
    window.localStorage.setItem(SIZE_STORAGE, value)
  } catch {
    // 忽略
  }
})

watch(productId, (value) => {
  try {
    window.localStorage.setItem(PRODUCT_ID_STORAGE, value)
  } catch {
    // 忽略
  }
})

onMounted(() => {
  loadRecords()
  // loadTemplates 内部已经兜住坏数据，读失败会返回空数组，不会影响其它初始化。
  templates.value = loadTemplates()
  try {
    const storedModel = window.localStorage.getItem(MODEL_STORAGE)
    // 存过的模型名要过一遍白名单：模型改成固定下拉之后，
    // 旧值（手打过的别的名字）会让下拉框显示空白，看着像坏了。
    if (storedModel) model.value = normalizeAIImageModel(storedModel)
    const storedSize = window.localStorage.getItem(SIZE_STORAGE)
    if (storedSize) size.value = storedSize
    const storedProductId = window.localStorage.getItem(PRODUCT_ID_STORAGE)
    if (storedProductId) productId.value = storedProductId
    const storedKey = window.localStorage.getItem(KEY_STORAGE)
    if (storedKey) {
      keyInput.value = storedKey
      void saveKey()
    }
  } catch {
    // 忽略存储读取失败
  }
})

onBeforeUnmount(() => {
  if (!referencePreviewUrl.value) return
  try {
    URL.revokeObjectURL(referencePreviewUrl.value)
  } catch {
    // 页面卸载时释放失败无所谓。
  }
})
</script>
