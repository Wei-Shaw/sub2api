<template>
  <AppLayout>
    <div class="flex h-[calc(100vh-8.375rem)] min-h-0 w-full flex-col overflow-hidden bg-gray-50 dark:bg-dark-900">
      <div class="shrink-0 border-b border-gray-200 bg-white px-4 py-2 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-2 xl:flex-row xl:items-center xl:justify-between">
          <div class="min-w-0">
            <div class="flex items-center gap-3">
              <p class="text-xs font-medium uppercase tracking-wide text-primary-600 dark:text-primary-400">Playground</p>
              <div class="inline-flex rounded-xl bg-gray-100 p-1 dark:bg-dark-700">
                <RouterLink to="/playground/chat" class="rounded-lg px-3 py-1.5 text-sm font-medium transition" :class="activeMode === 'chat' ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-900 dark:text-primary-300' : 'text-gray-600 hover:text-gray-900 dark:text-gray-300'">聊天</RouterLink>
                <RouterLink to="/playground/image" class="rounded-lg px-3 py-1.5 text-sm font-medium transition" :class="activeMode === 'image' ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-900 dark:text-primary-300' : 'text-gray-600 hover:text-gray-900 dark:text-gray-300'">生图</RouterLink>
              </div>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前站点直连中转接口；选择 API Key 后即可测试。</p>
          </div>
          <label class="block w-full xl:w-[520px]">
            <span class="text-xs font-medium text-gray-600 dark:text-gray-300">API Key</span>
            <div class="mt-1 flex gap-2">
              <select v-model="selectedKeyId" class="input h-10 w-full text-sm" :disabled="keysLoading">
                <option value="">{{ keysLoading ? '加载 Key 中...' : '请选择 API Key' }}</option>
                <option v-for="key in availableKeys" :key="key.id" :value="String(key.id)">{{ key.name }} · {{ maskKey(key.key) }}</option>
              </select>
              <button class="btn btn-secondary h-9 shrink-0 px-3" type="button" :disabled="keysLoading" @click="loadKeys">刷新</button>
            </div>
            <p v-if="keysError" class="mt-1 text-xs text-red-600 dark:text-red-300">{{ keysError }}</p>
          </label>
        </div>
      </div>

      <main class="min-h-0 flex-1 overflow-hidden p-3">
        <section v-if="activeMode === 'chat'" class="grid h-full min-h-0 gap-3" :class="chatSessionsCollapsed ? 'xl:grid-cols-[60px_minmax(0,1fr)]' : 'xl:grid-cols-[260px_minmax(0,1fr)]'">
          <aside class="flex min-h-0 flex-col rounded-2xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-800" :class="chatSessionsCollapsed ? 'items-center' : ''">
            <div class="flex w-full items-center justify-between gap-2" :class="chatSessionsCollapsed ? 'flex-col' : ''">
              <h2 v-if="!chatSessionsCollapsed" class="text-sm font-semibold text-gray-900 dark:text-white">会话</h2>
              <div class="flex items-center gap-1" :class="chatSessionsCollapsed ? 'flex-col' : ''">
                <button class="btn btn-primary px-2.5 py-1.5 text-xs" type="button" title="新建会话" @click="newChatSession">{{ chatSessionsCollapsed ? '+' : '新建' }}</button>
                <button class="rounded-lg border border-gray-200 px-2.5 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-200 dark:hover:bg-dark-700" type="button" title="聊天设置" @click="showChatSettings = true">⚙</button>
                <button class="rounded-lg border border-gray-200 px-2.5 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-200 dark:hover:bg-dark-700" type="button" :title="chatSessionsCollapsed ? '展开会话' : '折叠会话'" @click="chatSessionsCollapsed = !chatSessionsCollapsed">{{ chatSessionsCollapsed ? '›' : '‹' }}</button>
              </div>
            </div>
            <div v-if="!chatSessionsCollapsed" class="mt-3 min-h-0 flex-1 space-y-2 overflow-y-auto">
              <button v-for="session in chatSessions" :key="session.id" class="w-full rounded-xl border px-3 py-2 text-left text-sm transition" :class="session.id === activeSessionId ? 'border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-800 dark:bg-primary-950/30 dark:text-primary-200' : 'border-gray-200 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700'" type="button" @click="activeSessionId = session.id">
                <div class="truncate font-medium">{{ session.title }}</div>
                <div class="mt-1 truncate text-xs text-gray-400">{{ formatHistoryTime(session.updatedAt) }}</div>
              </button>
            </div>
            <button v-if="!chatSessionsCollapsed" class="btn btn-secondary mt-3 w-full" type="button" @click="showChatHistory = true">聊天历史</button>
          </aside>

          <div class="flex min-h-0 flex-col rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex shrink-0 items-center justify-between border-b border-gray-200 px-4 py-2 dark:border-dark-700">
              <div class="min-w-0"><h2 class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ activeSession?.title || '聊天' }}</h2><p class="text-xs text-gray-400">流式响应 · {{ useContext ? '携带上下文' : '单轮请求' }}</p></div>
              <button v-if="chatLoading" class="btn btn-secondary px-3 py-1.5 text-xs" type="button" @click="abortChat">停止</button>
            </div>
            <div ref="chatMessagesRef" class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4" @scroll="handleChatScroll">
              <div v-if="currentChatMessages.length === 0" class="flex h-full items-center justify-center text-center text-gray-400">输入消息后发送，回复会以流式方式显示。</div>
              <div v-for="(message, index) in currentChatMessages" :key="index" class="flex" :class="message.role === 'user' ? 'justify-end' : 'justify-start'">
                <div class="max-w-[88%] rounded-2xl px-4 py-3 text-sm leading-6" :class="message.role === 'user' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-gray-100'">
                  <div v-if="message.role === 'assistant' && chatLoading && !message.content" class="flex items-center gap-2 text-gray-500 dark:text-gray-300">
                    <span class="inline-flex h-6 items-center gap-1 rounded-full bg-white/70 px-2.5 dark:bg-dark-800/70">
                      <span class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary-500 [animation-delay:-0.2s]"></span>
                      <span class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary-500 [animation-delay:-0.1s]"></span>
                      <span class="h-1.5 w-1.5 animate-bounce rounded-full bg-primary-500"></span>
                    </span>
                    <span class="text-xs">正在组织回复...</span>
                  </div>
                  <div v-else>
                    <div v-if="message.images?.length" class="mb-2 flex flex-wrap gap-2">
                      <button v-for="image in message.images" :key="image.dataUrl" type="button" class="overflow-hidden rounded-xl border border-white/20 bg-black/10" @click="previewImage = image.dataUrl">
                        <img :src="image.dataUrl" :alt="image.name" class="h-24 w-24 object-cover" />
                      </button>
                    </div>
                    <div class="whitespace-pre-wrap">{{ message.content }}</div>
                  </div>
                </div>
              </div>
              <div v-if="chatError" class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-200">{{ chatError }}</div>
            </div>
            <div class="shrink-0 border-t border-gray-200 p-3 dark:border-dark-700">
              <div v-if="chatImages.length" class="mb-2 flex flex-wrap gap-2">
                <div v-for="(image, index) in chatImages" :key="image.dataUrl" class="group relative h-16 w-16 overflow-hidden rounded-xl border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-900">
                  <img :src="image.dataUrl" :alt="image.name" class="h-full w-full object-cover" />
                  <button type="button" class="absolute right-1 top-1 hidden rounded-full bg-black/60 px-1.5 text-xs text-white group-hover:block" @click="removeChatImage(index)">×</button>
                </div>
              </div>
              <textarea v-model="chatInput" class="input min-h-[64px] w-full resize-y" placeholder="输入你想测试的问题，也可以添加图片后发送..." @keydown="handleChatInputKeydown" />
              <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">Enter 发送，Alt + Enter 换行</span>
                <div class="flex items-center gap-2">
                  <input id="chat-image-upload" class="hidden" type="file" accept="image/*" multiple @change="handleChatImageUpload" />
                  <label for="chat-image-upload" class="btn btn-secondary cursor-pointer">添加图片</label>
                  <button class="btn btn-primary" type="button" :disabled="chatLoading || !canSendChat" @click="sendChat">{{ chatLoading ? '回复中...' : '发送' }}</button>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section v-else class="grid h-full min-h-0 gap-3 xl:grid-cols-[400px_minmax(0,1fr)]">
          <aside class="flex min-h-0 flex-col rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="shrink-0 border-b border-gray-200 px-3 py-2 dark:border-dark-700">
              <div class="flex items-center justify-between gap-2">
                <div>
                  <h2 class="text-sm font-semibold text-gray-900 dark:text-white">生图任务</h2>
                  <p class="text-[11px] text-gray-500 dark:text-gray-400">提交后可继续创建下一张图。</p>
                </div>
                <span class="rounded-full bg-primary-50 px-2 py-1 text-xs text-primary-700 dark:bg-primary-950/40 dark:text-primary-200">{{ runningImageTasks.length }} 运行中</span>
              </div>
            </div>

            <div class="min-h-0 flex-1 space-y-2 overflow-y-auto p-2.5">
              <section class="rounded-xl bg-gray-50 p-2.5 dark:bg-dark-900/60">
                <button type="button" class="flex w-full items-center justify-between gap-2 text-left" @click="imageSettingsOpen = !imageSettingsOpen">
                  <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">基础设置</span>
                  <span class="text-xs text-gray-400">{{ imageSettingsOpen ? '收起' : '展开' }}</span>
                </button>
                <div v-if="imageSettingsOpen" class="mt-2">
                  <label class="block"><span class="sr-only">模型</span><select v-model="imageModel" class="input h-10 w-full text-sm"><option v-for="model in imageModelOptions" :key="model" :value="model">{{ model }}</option></select></label>
                  <div class="mt-2 space-y-1.5">
                    <div class="grid grid-cols-[48px_minmax(0,1fr)] items-center gap-2"><span class="text-xs font-medium text-gray-600 dark:text-gray-300">清晰度</span><div class="grid grid-cols-3 gap-1.5"><button v-for="quality in ['1k', '2k', '4k']" :key="quality" type="button" class="rounded-lg border px-2 py-1.5 text-xs font-medium transition" :class="imageQuality === quality ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/40 dark:text-primary-200' : 'border-gray-200 hover:bg-white dark:border-dark-700 dark:hover:bg-dark-800'" @click="imageQuality = quality">{{ quality.toUpperCase() }}</button></div></div>
                    <div class="grid grid-cols-[48px_minmax(0,1fr)] items-center gap-2"><span class="text-xs font-medium text-gray-600 dark:text-gray-300">比例</span><div class="grid grid-cols-3 gap-1.5"><button v-for="option in imageSizeOptions" :key="option.value" type="button" class="rounded-lg border px-2 py-1.5 text-xs font-medium transition" :class="imageSize === option.value ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/40 dark:text-primary-200' : 'border-gray-200 hover:bg-white dark:border-dark-700 dark:hover:bg-dark-800'" @click="imageSize = option.value">{{ option.label }}</button></div></div>
                    <div class="grid grid-cols-[48px_minmax(0,1fr)] items-center gap-2"><span class="text-xs font-medium text-gray-600 dark:text-gray-300">数量</span><div class="grid grid-cols-4 gap-1.5"><button v-for="count in [1, 2, 3, 4]" :key="count" type="button" class="rounded-lg border px-2 py-1.5 text-xs font-medium transition" :class="imageCount === count ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/40 dark:text-primary-200' : 'border-gray-200 hover:bg-white dark:border-dark-700 dark:hover:bg-dark-800'" @click="imageCount = count">{{ count }}</button></div></div>
                  </div>
                </div>
              </section>

              <section class="rounded-xl bg-gray-50 p-2.5 dark:bg-dark-900/60">
                <button type="button" class="flex w-full items-center justify-between gap-2 text-left" @click="imageReferenceOpen = !imageReferenceOpen">
                  <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">参考图<span v-if="imageReference" class="ml-1 text-primary-600">已添加</span></span>
                  <span class="text-xs text-gray-400">{{ imageReferenceOpen ? '收起' : '展开' }}</span>
                </button>
                <div v-if="imageReferenceOpen" class="mt-2">
                  <div class="mb-2 flex justify-end"><button v-if="imageReference" class="text-xs text-red-500 hover:text-red-600" type="button" @click="imageReference = null">移除</button></div>
                  <div v-if="imageReference" class="flex items-center gap-2 rounded-xl border border-gray-200 bg-white p-2 dark:border-dark-700 dark:bg-dark-800"><button class="h-14 w-14 overflow-hidden rounded-lg bg-gray-100" type="button" @click="previewImage = imageReference?.dataUrl || ''"><img :src="imageReference.dataUrl" :alt="imageReference.name" class="h-full w-full object-cover" /></button><div class="min-w-0"><p class="truncate text-xs font-medium text-gray-700 dark:text-gray-200">{{ imageReference.name }}</p><p class="text-[11px] text-gray-400">将优先走图片编辑接口</p></div></div>
                  <label v-else for="image-reference-upload" class="flex cursor-pointer items-center justify-center rounded-xl border border-dashed border-gray-300 bg-white px-3 py-3 text-xs text-gray-500 hover:border-primary-300 hover:text-primary-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">+ 添加图片作为参考</label>
                  <input id="image-reference-upload" class="hidden" type="file" accept="image/*" @change="handleImageReferenceUpload" />
                </div>
              </section>

              <section class="min-h-0 flex-1 rounded-xl bg-gray-50 p-2.5 dark:bg-dark-900/60">
                <div class="mb-1.5 flex items-center justify-between gap-2"><span class="text-xs font-semibold text-gray-700 dark:text-gray-200">Prompt</span><button class="text-xs text-primary-600 hover:text-primary-700 dark:text-primary-300" type="button" @click="imagePrompt = ''">清空</button></div>
                <div class="mb-2 flex gap-1 overflow-x-auto pb-1"><button v-for="template in imagePromptTemplates" :key="template.name" type="button" class="shrink-0 rounded-full border border-gray-200 px-2 py-0.5 text-[11px] text-gray-600 hover:border-primary-300 hover:text-primary-600 dark:border-dark-700 dark:text-gray-300" @click="applyImageTemplate(template.prompt)">{{ template.name }}</button></div>
                <textarea v-model="imagePrompt" class="input min-h-[260px] w-full resize-y text-sm" placeholder="描述你想生成的图片。提交后会创建任务，可以继续写下一个 prompt。" />
              </section>
            </div>

            <div class="shrink-0 border-t border-gray-200 p-2.5 dark:border-dark-700">
              <button class="btn btn-primary w-full" type="button" :disabled="!canGenerateImage" @click="generateImage">创建生图任务</button>
            </div>
          </aside>

          <div class="grid min-h-0 gap-3 xl:grid-cols-[minmax(0,1fr)_300px]">
            <div class="flex min-h-0 flex-col rounded-2xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="flex shrink-0 items-center justify-between gap-3"><div><h2 class="text-sm font-semibold text-gray-900 dark:text-white">结果画布</h2><p class="text-xs text-gray-400">任务完成后自动显示最新结果，并写入历史记录。</p></div><button class="btn btn-secondary px-3 py-1.5 text-xs" type="button" @click="showHistory = true">历史记录</button></div>
              <div class="mt-2 min-h-0 flex-1 overflow-hidden">
                <div v-if="currentImageTask && currentImageTask.status === 'running'" class="flex h-full min-h-0 flex-col items-center justify-center rounded-2xl border border-dashed border-primary-200 bg-primary-50/60 text-center text-primary-700 dark:border-primary-900/60 dark:bg-primary-950/20 dark:text-primary-200"><div class="h-10 w-10 animate-spin rounded-full border-4 border-primary-200 border-t-primary-600 dark:border-primary-900 dark:border-t-primary-300"></div><p class="mt-4 text-sm font-medium">任务生成中...</p><p class="mt-1 max-w-md truncate text-xs opacity-80">{{ currentImageTask.prompt }}</p><p class="mt-2 text-xs opacity-70">最长等待 10 分钟，完成后会自动进入历史记录。</p></div>
                <div v-else-if="currentImageTask && currentImageTask.status === 'error'" class="flex h-full min-h-0 flex-col items-center justify-center rounded-2xl border border-dashed border-red-200 bg-red-50/70 p-6 text-center text-red-700 dark:border-red-900/60 dark:bg-red-950/20 dark:text-red-200"><div class="text-4xl">⚠️</div><p class="mt-3 text-sm font-semibold">当前任务生成失败</p><p class="mt-2 max-w-xl whitespace-pre-wrap text-xs leading-5">{{ currentImageTask.error }}</p><p class="mt-3 text-xs opacity-75">你可以点右侧其他任务查看结果，或调整参数后重新创建任务。</p></div>
                <div v-else-if="!currentImageTask || currentImageTask.images.length === 0" class="flex h-full min-h-0 flex-col items-center justify-center rounded-2xl border border-dashed border-gray-200 text-center text-gray-400 dark:border-dark-700"><div class="text-4xl">🎨</div><p class="mt-3 text-sm">创建任务后，最新结果会显示在这里。</p><p class="mt-1 text-xs">你可以连续提交多个任务，然后在右侧任务列表或历史记录查看。</p></div>
                <div v-else class="grid h-full min-h-0 gap-3" :class="currentImageTask.images.length === 1 ? 'grid-cols-1' : 'grid-cols-2'"><div v-for="(image, index) in currentImageTask.images" :key="index" class="group relative flex min-h-0 flex-col overflow-hidden rounded-2xl bg-gray-100 dark:bg-dark-950"><button class="min-h-0 flex-1 cursor-zoom-in" type="button" @click="previewImage = image"><img :src="image" :alt="`Generated image ${index + 1}`" class="h-full w-full object-contain" /></button><div class="absolute bottom-2 right-2 hidden gap-2 group-hover:flex"><button class="rounded-lg bg-black/60 px-3 py-1.5 text-xs text-white" type="button" @click="previewImage = image">预览</button><a class="rounded-lg bg-black/60 px-3 py-1.5 text-xs text-white" :href="image" target="_blank" rel="noopener noreferrer" download>下载</a></div></div></div>
              </div>
            </div>

            <aside class="flex min-h-0 flex-col rounded-2xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="flex shrink-0 items-center justify-between"><h2 class="text-sm font-semibold text-gray-900 dark:text-white">任务列表</h2><button v-if="imageTasks.length" class="text-xs text-gray-500 hover:text-red-600" type="button" @click="clearCompletedImageTasks">清理完成</button></div>
              <div class="mt-3 min-h-0 flex-1 space-y-2 overflow-y-auto">
                <div v-if="imageTasks.length === 0" class="flex h-full items-center justify-center rounded-xl border border-dashed border-gray-200 text-center text-xs text-gray-400 dark:border-dark-700">暂无任务</div>
                <button v-for="task in imageTasks" :key="task.id" type="button" class="w-full rounded-xl border p-3 text-left transition" :class="task.id === activeImageTaskId ? 'border-primary-300 bg-primary-50 dark:border-primary-800 dark:bg-primary-950/30' : 'border-gray-200 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700'" @click="activeImageTaskId = task.id">
                  <div class="flex items-center justify-between gap-2"><span class="rounded-full px-2 py-0.5 text-xs" :class="taskStatusClass(task.status)">{{ taskStatusText(task.status) }}</span><span class="text-xs text-gray-400">{{ formatHistoryTime(task.createdAt) }}</span></div>
                  <p class="mt-2 line-clamp-2 text-sm text-gray-700 dark:text-gray-200">{{ task.prompt }}</p>
                  <p class="mt-1 text-xs text-gray-400">{{ task.model }} · {{ task.quality.toUpperCase() }} · {{ task.size }} · {{ task.count }}张</p>
                  <p v-if="task.error" class="mt-2 line-clamp-2 text-xs text-red-600 dark:text-red-300">{{ task.error }}</p>
                </button>
              </div>
            </aside>
          </div>
        </section>
      </main>
    </div>

    <div v-if="previewImage" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4" @click.self="previewImage = ''"><div class="relative max-h-full max-w-6xl"><button class="absolute right-3 top-3 rounded-full bg-black/60 px-3 py-1 text-sm text-white hover:bg-black/80" type="button" @click="previewImage = ''">关闭</button><img :src="previewImage" alt="Preview" class="max-h-[90vh] max-w-full rounded-2xl object-contain shadow-2xl" /></div></div>

    <div v-if="showHistory" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="showHistory = false"><div class="flex max-h-[86vh] w-full max-w-5xl flex-col rounded-2xl bg-white shadow-2xl dark:bg-dark-800"><div class="flex items-center justify-between border-b border-gray-200 p-4 dark:border-dark-700"><div><h3 class="text-lg font-semibold text-gray-900 dark:text-white">生图历史记录</h3><p class="text-sm text-gray-500 dark:text-gray-400">仅缓存在当前浏览器本地，最多保留 {{ maxHistoryItems }} 条。</p></div><div class="flex items-center gap-2"><button v-if="imageHistory.length" class="btn btn-secondary" type="button" @click="clearHistory">清空历史</button><button class="btn btn-secondary" type="button" @click="showHistory = false">关闭</button></div></div><div class="min-h-0 flex-1 overflow-y-auto p-4"><div v-if="imageHistory.length === 0" class="flex min-h-[180px] items-center justify-center rounded-2xl border border-dashed border-gray-200 text-gray-400 dark:border-dark-700">暂无历史记录。</div><div v-else class="space-y-3"><div v-for="entry in imageHistory" :key="entry.id" class="rounded-2xl border border-gray-200 p-3 dark:border-dark-700"><div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between"><div class="min-w-0"><div class="text-sm font-medium text-gray-900 dark:text-white">{{ entry.model }} · {{ entry.quality.toUpperCase() }} · {{ entry.size }}</div><div class="mt-1 line-clamp-2 text-sm text-gray-500 dark:text-gray-400">{{ entry.prompt }}</div><div class="mt-1 text-xs text-gray-400">{{ formatHistoryTime(entry.createdAt) }}</div></div><button class="btn btn-primary shrink-0" type="button" @click="restoreHistory(entry)">载入</button></div><div class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4"><button v-for="(image, index) in entry.images" :key="index" class="aspect-square overflow-hidden rounded-xl bg-gray-100 dark:bg-dark-900" type="button" @click="previewImage = image"><img :src="image" :alt="`History image ${index + 1}`" class="h-full w-full object-cover" /></button></div></div></div></div></div></div>

    <div v-if="showChatHistory" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="showChatHistory = false"><div class="flex max-h-[86vh] w-full max-w-4xl flex-col rounded-2xl bg-white shadow-2xl dark:bg-dark-800"><div class="flex items-center justify-between border-b border-gray-200 p-4 dark:border-dark-700"><div><h3 class="text-lg font-semibold text-gray-900 dark:text-white">聊天历史</h3><p class="text-sm text-gray-500 dark:text-gray-400">本地缓存的会话记录，最多保留 {{ maxChatSessions }} 个会话。</p></div><div class="flex items-center gap-2"><button v-if="chatSessions.length" class="btn btn-secondary" type="button" @click="clearAllChatSessions">清空历史</button><button class="btn btn-secondary" type="button" @click="showChatHistory = false">关闭</button></div></div><div class="min-h-0 flex-1 overflow-y-auto p-4"><div v-if="chatSessions.length === 0" class="flex min-h-[180px] items-center justify-center rounded-2xl border border-dashed border-gray-200 text-gray-400 dark:border-dark-700">暂无聊天历史。</div><div v-else class="space-y-3"><div v-for="session in chatSessions" :key="session.id" class="rounded-2xl border border-gray-200 p-3 dark:border-dark-700"><div class="flex items-start justify-between gap-3"><div class="min-w-0"><div class="font-medium text-gray-900 dark:text-white">{{ session.title }}</div><div class="mt-1 text-xs text-gray-400">{{ formatHistoryTime(session.updatedAt) }} · {{ session.messages.length }} 条消息</div><div class="mt-2 line-clamp-2 text-sm text-gray-500 dark:text-gray-400">{{ session.messages[0]?.content || '空会话' }}</div></div><button class="btn btn-primary shrink-0" type="button" @click="restoreChatSession(session.id)">打开</button></div></div></div></div></div></div>

    <div v-if="showChatSettings" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="showChatSettings = false">
      <div class="flex max-h-[86vh] w-full max-w-xl flex-col rounded-2xl bg-white shadow-2xl dark:bg-dark-800">
        <div class="flex items-center justify-between border-b border-gray-200 p-4 dark:border-dark-700">
          <div><h3 class="text-lg font-semibold text-gray-900 dark:text-white">聊天设置</h3><p class="text-sm text-gray-500 dark:text-gray-400">默认提示词为空；设置保存后用于后续请求。</p></div>
          <button class="btn btn-secondary" type="button" @click="showChatSettings = false">关闭</button>
        </div>
        <div class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
          <label class="block"><span class="text-xs font-medium text-gray-700 dark:text-gray-200">模型</span><select v-model="chatModel" class="input mt-1 h-10 w-full text-sm"><option v-for="model in chatModelOptions" :key="model" :value="model">{{ model }}</option></select></label>
          <label class="block"><span class="text-xs font-medium text-gray-700 dark:text-gray-200">提示词模板</span><select v-model="selectedPromptId" class="input mt-1 h-10 w-full text-sm" @change="applyPromptPreset"><option value="">不使用模板</option><option v-for="preset in promptPresets" :key="preset.id" :value="preset.id">{{ preset.category }} · {{ preset.name }}</option></select></label>
          <label class="block"><span class="text-xs font-medium text-gray-700 dark:text-gray-200">System Prompt</span><textarea v-model="systemPrompt" class="input mt-1 min-h-[180px] w-full resize-y text-sm" placeholder="默认留空。需要指定角色、语气或输出格式时再填写。" /></label>
          <div class="grid grid-cols-2 gap-2">
            <label class="block"><span class="text-xs font-medium text-gray-700 dark:text-gray-200">Temperature</span><input v-model.number="temperature" type="number" min="0" max="2" step="0.1" class="input mt-1 h-10 w-full text-sm" /></label>
            <label class="block"><span class="text-xs font-medium text-gray-700 dark:text-gray-200">Max Tokens</span><input v-model.number="maxTokens" type="number" min="1" step="1" class="input mt-1 h-10 w-full text-sm" /></label>
          </div>
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200"><input v-model="useContext" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />携带当前会话上下文</label>
          <button class="btn btn-secondary w-full" type="button" :disabled="chatLoading" @click="clearCurrentChat">清空当前会话</button>
        </div>
        <div class="flex justify-end gap-2 border-t border-gray-200 p-4 dark:border-dark-700">
          <button class="btn btn-secondary" type="button" @click="showChatSettings = false">取消</button>
          <button class="btn btn-primary" type="button" @click="saveChatSettings">保存使用</button>
        </div>
      </div>
    </div>

  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { keysAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import type { ApiKey } from '@/types'

type PlaygroundMode = 'chat' | 'image'
type ChatRole = 'user' | 'assistant'
interface UploadedImage { name: string; type: string; size: number; dataUrl: string; file?: File }
interface ChatMessage { role: ChatRole; content: string; images?: UploadedImage[] }
interface ChatSession { id: string; title: string; createdAt: number; updatedAt: number; messages: ChatMessage[] }
interface PromptPreset { id: string; category: string; name: string; prompt: string }
interface ImageHistoryEntry { id: string; createdAt: number; prompt: string; model: string; quality: string; size: string; images: string[] }
type ImageTaskStatus = 'running' | 'success' | 'error'
interface ImageTask { id: string; createdAt: number; completedAt?: number; prompt: string; model: string; quality: string; size: string; count: number; status: ImageTaskStatus; images: string[]; error?: string; referenceImage?: UploadedImage | null }

const route = useRoute()
const chatModelOptions = ['gpt-5.5', 'gpt-5.3-codex', 'gpt5.4']
const promptPresets: PromptPreset[] = [
  { id: 'general', category: '通用', name: '通用助手', prompt: '你是一个严谨、直接、实用的 AI 助手。回答要结构清晰，必要时给出步骤和示例。' },
  { id: 'code', category: '技术', name: '资深工程师', prompt: '你是一名资深全栈工程师。优先给出可执行方案，指出风险、边界条件和测试方法。代码要简洁、可维护。' },
  { id: 'product', category: '产品', name: '产品经理', prompt: '你是一名经验丰富的产品经理。请从用户价值、使用场景、交互细节、优先级和验收标准角度分析问题。' },
  { id: 'copy', category: '营销', name: '文案专家', prompt: '你是一名中文营销文案专家。输出要有吸引力、清楚、少空话，并给出多个可选版本。' },
  { id: 'legal', category: '合规', name: '合规顾问', prompt: '你是一名合规顾问。请识别潜在风险，给出审慎建议。不要替代专业律师意见。' },
  { id: 'finance', category: '商业', name: '商业分析师', prompt: '你是一名商业分析师。请用数据和假设拆解问题，关注成本、收益、风险和关键指标。' },
  { id: 'design', category: '设计', name: 'UX 设计师', prompt: '你是一名 UX/UI 设计师。请关注信息架构、视觉层级、用户路径、可访问性和可用性。' },
  { id: 'translate', category: '语言', name: '翻译润色', prompt: '你是一名专业翻译和中文编辑。请保持原意，提升表达的自然度、准确性和质感。' },
]
const availableKeys = ref<ApiKey[]>([])
const selectedKeyId = ref(localStorage.getItem('playground_selected_key_id') || '')
const keysLoading = ref(false)
const keysError = ref('')
const selectedPromptId = ref(localStorage.getItem('playground_prompt_preset') || '')
const chatModel = ref(localStorage.getItem('playground_chat_model') || chatModelOptions[0])
const systemPrompt = ref(localStorage.getItem('playground_system_prompt') || '')
const temperature = ref(0.7)
const maxTokens = ref(1024)
const chatInput = ref('')
const chatImages = ref<UploadedImage[]>([])
const chatLoading = ref(false)
const chatError = ref('')
const useContext = ref(localStorage.getItem('playground_use_context') !== 'false')
const chatSessions = ref<ChatSession[]>([])
const activeSessionId = ref(localStorage.getItem('playground_active_chat_session') || '')
const showChatHistory = ref(false)
const showChatSettings = ref(false)
const chatSessionsCollapsed = ref(localStorage.getItem('playground_chat_sessions_collapsed') === 'true')
const maxChatSessions = 30
const chatSessionsStorageKey = 'playground_chat_sessions'
const chatMessagesRef = ref<HTMLElement | null>(null)
const chatAutoScroll = ref(true)
let chatAbortController: AbortController | null = null

const imageModelOptions = ref<string[]>(['gpt-image-1', 'gpt-image-2'])
const imageModelsLoading = ref(false)
const imageModelsError = ref('')
const imageModel = ref(localStorage.getItem('playground_image_model') || 'gpt-image-1')
const imageSize = ref(localStorage.getItem('playground_image_size') || '1024x1024')
const imageQuality = ref(localStorage.getItem('playground_image_quality') || '1k')
const imageCount = ref(1)
const imagePrompt = ref('')
const imageSettingsOpen = ref(false)
const imageReference = ref<UploadedImage | null>(null)
const imageReferenceOpen = ref(false)
const imageError = ref('')
const imageRequestTimeoutMs = 10 * 60 * 1000
const imageTasks = ref<ImageTask[]>([])
const activeImageTaskId = ref('')
const maxImageTasks = 20
const previewImage = ref('')
const showHistory = ref(false)
const imageHistory = ref<ImageHistoryEntry[]>([])
const maxHistoryItems = 20
const historyStorageKey = 'playground_image_history'

const activeMode = computed<PlaygroundMode>(() => route.path.includes('/image') ? 'image' : 'chat')
const selectedKey = computed(() => availableKeys.value.find(key => String(key.id) === selectedKeyId.value) || null)
const apiKey = computed(() => selectedKey.value?.key || '')
const activeSession = computed(() => chatSessions.value.find(s => s.id === activeSessionId.value) || null)
const currentChatMessages = computed(() => activeSession.value?.messages || [])
const canSendChat = computed(() => Boolean(apiKey.value && chatModel.value.trim() && (chatInput.value.trim() || chatImages.value.length) && activeSession.value))
const canGenerateImage = computed(() => Boolean(apiKey.value && imageModel.value.trim() && imagePrompt.value.trim()))
const runningImageTasks = computed(() => imageTasks.value.filter(task => task.status === 'running'))
const currentImageTask = computed(() => imageTasks.value.find(task => task.id === activeImageTaskId.value) || imageTasks.value[0] || null)
const imageSizeOptions = [
  { label: '1:1', value: '1024x1024' },
  { label: '2:3', value: '1024x1536' },
  { label: '3:2', value: '1536x1024' },
]
const imagePromptTemplates = [
  { name: '写实人像', prompt: '写实人像摄影，电影感光影，浅景深，高级质感，细节丰富' },
  { name: '产品摄影', prompt: '高端产品摄影，干净背景，柔和棚拍灯光，商业广告质感' },
  { name: '电商海报', prompt: '电商宣传海报，突出主体，强视觉冲击，清晰排版空间' },
  { name: 'Logo图标', prompt: '极简 logo 图标，矢量风格，干净线条，现代品牌感' },
  { name: '插画', prompt: '精致插画风格，丰富细节，柔和配色，画面完整' },
  { name: '赛博朋克', prompt: '赛博朋克风格，霓虹灯，未来城市，高对比光影，电影构图' },
]

watch(selectedKeyId, value => localStorage.setItem('playground_selected_key_id', value))
watch(selectedPromptId, value => localStorage.setItem('playground_prompt_preset', value))
watch(chatModel, value => localStorage.setItem('playground_chat_model', value))
watch(systemPrompt, value => localStorage.setItem('playground_system_prompt', value))
watch(chatSessionsCollapsed, value => localStorage.setItem('playground_chat_sessions_collapsed', String(value)))
watch(useContext, value => localStorage.setItem('playground_use_context', String(value)))
watch(activeSessionId, value => localStorage.setItem('playground_active_chat_session', value))
watch(imageModel, value => localStorage.setItem('playground_image_model', value))
watch(imageSize, value => localStorage.setItem('playground_image_size', value))
watch(imageQuality, value => localStorage.setItem('playground_image_quality', value))
watch(activeSessionId, () => { chatAutoScroll.value = true; void scrollChatToBottom(true) })

onMounted(async () => { loadHistory(); loadChatSessions(); await Promise.all([loadKeys(), loadImageModels()]) })
function maskKey(key: string): string { return key.length <= 12 ? key : `${key.slice(0, 6)}...${key.slice(-4)}` }
function formatHistoryTime(timestamp: number): string { try { return new Date(timestamp).toLocaleString() } catch { return '' } }
function extractErrorMessage(error: unknown, fallback = '请求失败，请检查 API Key、模型名或服务端日志。'): string { if (error instanceof Error) return error.message; if (typeof error === 'string') return error; if (typeof error === 'object' && error !== null && 'message' in error) return String((error as { message: unknown }).message); return fallback }
function buildUrl(path: string): string { return path }
async function loadKeys() { keysLoading.value = true; keysError.value = ''; try { const response = await keysAPI.list(1, 100, { status: 'active' }); availableKeys.value = response.items || []; if (selectedKeyId.value && !availableKeys.value.some(key => String(key.id) === selectedKeyId.value)) selectedKeyId.value = ''; if (!selectedKeyId.value && availableKeys.value.length > 0) selectedKeyId.value = String(availableKeys.value[0].id) } catch (error) { keysError.value = extractErrorMessage(error, '加载 API Key 失败，请稍后重试。') } finally { keysLoading.value = false } }
async function loadImageModels() {
  imageModelsLoading.value = false
  imageModelsError.value = ''
  imageModelOptions.value = ['gpt-image-1', 'gpt-image-2']
  if (!imageModelOptions.value.includes(imageModel.value)) imageModel.value = 'gpt-image-1'
}
function applyPromptPreset() { if (!selectedPromptId.value) return; const preset = promptPresets.find(p => p.id === selectedPromptId.value); if (preset) systemPrompt.value = preset.prompt }
function saveChatSettings() { localStorage.setItem('playground_chat_model', chatModel.value); localStorage.setItem('playground_prompt_preset', selectedPromptId.value); localStorage.setItem('playground_system_prompt', systemPrompt.value); localStorage.setItem('playground_use_context', String(useContext.value)); showChatSettings.value = false }
function isChatSession(value: unknown): value is ChatSession { const item = value as Partial<ChatSession>; return Boolean(item && item.id && item.title && item.createdAt && item.updatedAt && Array.isArray(item.messages)) }
function loadChatSessions() { try { const raw = localStorage.getItem(chatSessionsStorageKey); const parsed = raw ? JSON.parse(raw) : []; chatSessions.value = Array.isArray(parsed) ? parsed.filter(isChatSession).slice(0, maxChatSessions) : [] } catch { chatSessions.value = [] } if (!chatSessions.value.length) newChatSession(); else if (!activeSessionId.value || !chatSessions.value.some(s => s.id === activeSessionId.value)) activeSessionId.value = chatSessions.value[0].id }
function saveChatSessions() { localStorage.setItem(chatSessionsStorageKey, JSON.stringify(chatSessions.value.slice(0, maxChatSessions))) }
function newChatSession() { const now = Date.now(); const session: ChatSession = { id: `${now}-${Math.random().toString(36).slice(2, 8)}`, title: '新会话', createdAt: now, updatedAt: now, messages: [] }; chatSessions.value = [session, ...chatSessions.value].slice(0, maxChatSessions); activeSessionId.value = session.id; saveChatSessions() }
function updateActiveSession(mutator: (session: ChatSession) => void) { const session = activeSession.value; if (!session) return; mutator(session); session.updatedAt = Date.now(); if (session.messages[0]?.content) session.title = session.messages[0].content.slice(0, 24); chatSessions.value = [session, ...chatSessions.value.filter(s => s.id !== session.id)].slice(0, maxChatSessions); saveChatSessions() }
function clearCurrentChat() { updateActiveSession(session => { session.messages = []; session.title = '新会话' }); chatError.value = '' }
function clearAllChatSessions() { chatSessions.value = []; localStorage.removeItem(chatSessionsStorageKey); newChatSession(); showChatHistory.value = false }
function restoreChatSession(id: string) { activeSessionId.value = id; showChatHistory.value = false; chatAutoScroll.value = true; void scrollChatToBottom(true) }
function abortChat() { chatAbortController?.abort() }

function isChatNearBottom(element: HTMLElement): boolean { return element.scrollHeight - element.scrollTop - element.clientHeight < 80 }
function handleChatScroll() { const element = chatMessagesRef.value; if (!element) return; chatAutoScroll.value = isChatNearBottom(element) }
async function scrollChatToBottom(force = false) {
  if (!force && !chatAutoScroll.value) return
  await nextTick()
  const element = chatMessagesRef.value
  if (!element) return
  element.scrollTop = element.scrollHeight
}

function readUploadedImage(file: File): Promise<UploadedImage> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve({ name: file.name, type: file.type || 'image/png', size: file.size, dataUrl: String(reader.result || ''), file })
    reader.onerror = () => reject(new Error('读取图片失败'))
    reader.readAsDataURL(file)
  })
}
async function handleChatImageUpload(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || []).filter(file => file.type.startsWith('image/')).slice(0, 4 - chatImages.value.length)
  if (files.length) chatImages.value = [...chatImages.value, ...(await Promise.all(files.map(readUploadedImage)))].slice(0, 4)
  input.value = ''
}
function removeChatImage(index: number) { chatImages.value.splice(index, 1) }
async function handleImageReferenceUpload(event: Event) {
  const input = event.target as HTMLInputElement
  const file = Array.from(input.files || []).find(item => item.type.startsWith('image/'))
  if (file) { imageReference.value = await readUploadedImage(file); imageReferenceOpen.value = false }
  input.value = ''
}
function messageToApiMessage(message: ChatMessage): Record<string, unknown> {
  if (message.role !== 'user' || !message.images?.length) return { role: message.role, content: message.content }
  return { role: message.role, content: [
    { type: 'text', text: message.content || '请分析这张图片。' },
    ...message.images.map(image => ({ type: 'image_url', image_url: { url: image.dataUrl } })),
  ] }
}
function handleChatInputKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter') return
  if (event.altKey) return
  event.preventDefault()
  void sendChat()
}
async function sendChat() { if (!canSendChat.value || chatLoading.value) return; chatError.value = ''; chatLoading.value = true; chatAutoScroll.value = true; const userMessage = chatInput.value.trim(); const userImages = chatImages.value.map(image => ({ name: image.name, type: image.type, size: image.size, dataUrl: image.dataUrl })); chatInput.value = ''; chatImages.value = []; updateActiveSession(s => s.messages.push({ role: 'user', content: userMessage || (userImages.length ? '图片消息' : ''), images: userImages }, { role: 'assistant', content: '' })); await scrollChatToBottom(true); const session = activeSession.value; if (!session) return; const assistantIndex = session.messages.length - 1; chatAbortController = new AbortController(); try { const contextMessages = useContext.value ? session.messages.slice(0, assistantIndex) : [session.messages[assistantIndex - 1]]; const messages = [...(systemPrompt.value.trim() ? [{ role: 'system', content: systemPrompt.value.trim() }] : []), ...contextMessages.filter(Boolean).map(messageToApiMessage)]; await requestChatStream({ model: chatModel.value.trim(), messages, temperature: temperature.value, max_tokens: maxTokens.value || undefined, stream: true }, chunk => { updateActiveSession(s => { s.messages[assistantIndex].content += chunk }); void scrollChatToBottom() }) } catch (error) { if (error instanceof DOMException && error.name === 'AbortError') chatError.value = '已停止生成。'; else chatError.value = extractErrorMessage(error) } finally { chatLoading.value = false; chatAbortController = null; saveChatSessions(); void scrollChatToBottom() } }
async function requestChatStream(body: Record<string, unknown>, onChunk: (chunk: string) => void) { const response = await fetch(buildUrl('/v1/chat/completions'), { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${apiKey.value}` }, body: JSON.stringify(body), signal: chatAbortController?.signal }); if (!response.ok) throw new Error(await response.text() || `HTTP ${response.status}`); const contentType = response.headers.get('content-type') || ''; if (!response.body || !contentType.includes('text/event-stream')) { const data = await response.json(); const content = data?.choices?.[0]?.message?.content || data?.choices?.[0]?.text || JSON.stringify(data, null, 2); onChunk(content); return } const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ''; while (true) { const { done, value } = await reader.read(); if (done) break; buffer += decoder.decode(value, { stream: true }); const lines = buffer.split('\n'); buffer = lines.pop() || ''; for (const rawLine of lines) { const line = rawLine.trim(); if (!line.startsWith('data:')) continue; const payload = line.slice(5).trim(); if (!payload || payload === '[DONE]') continue; try { const data = JSON.parse(payload); const delta = data?.choices?.[0]?.delta?.content ?? data?.choices?.[0]?.text ?? ''; if (delta) onChunk(delta) } catch { /* ignore malformed SSE line */ } } } }
async function requestJson(url: string, body: Record<string, unknown>, timeoutMs?: number): Promise<unknown> { const controller = timeoutMs ? new AbortController() : null; const timeout = controller ? window.setTimeout(() => controller.abort(), timeoutMs) : null; try { const response = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${apiKey.value}` }, body: JSON.stringify(body), signal: controller?.signal }); const text = await response.text(); let data: unknown = null; if (text) { try { data = JSON.parse(text) } catch { data = text } } if (!response.ok) { const message = typeof data === 'object' && data !== null && 'error' in data ? JSON.stringify((data as { error: unknown }).error) : typeof data === 'object' && data !== null && 'message' in data ? String((data as { message: unknown }).message) : text || `HTTP ${response.status}`; throw new Error(message) } return data } catch (error) { if (error instanceof DOMException && error.name === 'AbortError') throw new Error('生成超过 10 分钟仍未返回，已自动结束。建议换用 gpt-image-1、降低清晰度/数量，或稍后重试。'); throw error } finally { if (timeout) window.clearTimeout(timeout) } }
async function requestImageEdit(task: ImageTask): Promise<unknown> { const form = new FormData(); form.append('model', task.model); form.append('prompt', task.prompt); form.append('quality', task.quality); form.append('size', task.size); form.append('n', String(task.count)); if (task.referenceImage?.file) form.append('image', task.referenceImage.file, task.referenceImage.name); else if (task.referenceImage?.dataUrl) form.append('image', await (await fetch(task.referenceImage.dataUrl)).blob(), task.referenceImage.name); const controller = new AbortController(); const timeout = window.setTimeout(() => controller.abort(), imageRequestTimeoutMs); try { const response = await fetch(buildUrl('/v1/images/edits'), { method: 'POST', headers: { Authorization: `Bearer ${apiKey.value}` }, body: form, signal: controller.signal }); const text = await response.text(); let data: unknown = text; if (text) { try { data = JSON.parse(text) } catch {} } if (!response.ok) throw new Error(typeof data === 'string' ? data : JSON.stringify(data)); return data } catch (error) { if (error instanceof DOMException && error.name === 'AbortError') throw new Error('参考图生成超过 10 分钟仍未返回，已自动结束。'); throw error } finally { window.clearTimeout(timeout) } }
function loadHistory() { try { const raw = localStorage.getItem(historyStorageKey); const parsed = raw ? JSON.parse(raw) : []; imageHistory.value = Array.isArray(parsed) ? parsed.filter(isHistoryEntry).slice(0, maxHistoryItems) : [] } catch { imageHistory.value = [] } }
function isHistoryEntry(value: unknown): value is ImageHistoryEntry { const item = value as Partial<ImageHistoryEntry>; return Boolean(item && item.id && item.createdAt && item.prompt && item.model && item.quality && item.size && Array.isArray(item.images)) }
function saveHistory() { try { localStorage.setItem(historyStorageKey, JSON.stringify(imageHistory.value.slice(0, maxHistoryItems))) } catch { while (imageHistory.value.length > 1) { imageHistory.value.pop(); try { localStorage.setItem(historyStorageKey, JSON.stringify(imageHistory.value.slice(0, maxHistoryItems))); return } catch {} } } }
function restoreHistory(entry: ImageHistoryEntry) { imagePrompt.value = entry.prompt; imageModel.value = entry.model; imageQuality.value = entry.quality; imageSize.value = entry.size; const task: ImageTask = { id: entry.id, createdAt: entry.createdAt, completedAt: entry.createdAt, prompt: entry.prompt, model: entry.model, quality: entry.quality, size: entry.size, count: entry.images.length, status: 'success', images: [...entry.images] }; imageTasks.value = [task, ...imageTasks.value.filter(item => item.id !== task.id)].slice(0, maxImageTasks); activeImageTaskId.value = task.id; showHistory.value = false }
function clearHistory() { imageHistory.value = []; localStorage.removeItem(historyStorageKey) }
function applyImageTemplate(prompt: string) { imagePrompt.value = imagePrompt.value ? `${imagePrompt.value}，${prompt}` : prompt }
function taskStatusText(status: ImageTaskStatus): string { return status === 'running' ? '生成中' : status === 'success' ? '已完成' : '失败' }
function taskStatusClass(status: ImageTaskStatus): string { return status === 'running' ? 'bg-blue-50 text-blue-700 dark:bg-blue-950/40 dark:text-blue-200' : status === 'success' ? 'bg-green-50 text-green-700 dark:bg-green-950/40 dark:text-green-200' : 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-200' }
function updateImageTask(id: string, patch: Partial<ImageTask>) { imageTasks.value = imageTasks.value.map(task => task.id === id ? { ...task, ...patch } : task) }
function clearCompletedImageTasks() { imageTasks.value = imageTasks.value.filter(task => task.status === 'running'); if (!imageTasks.value.some(task => task.id === activeImageTaskId.value)) activeImageTaskId.value = imageTasks.value[0]?.id || '' }
async function generateImage() {
  if (!canGenerateImage.value) return
  imageError.value = ''
  const prompt = imagePrompt.value.trim()
  const task: ImageTask = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    createdAt: Date.now(),
    prompt,
    model: imageModel.value.trim(),
    quality: imageQuality.value,
    size: imageSize.value,
    count: Math.max(1, Math.min(4, Number(imageCount.value) || 1)),
    status: 'running',
    images: [],
    referenceImage: imageReference.value ? { ...imageReference.value } : null,
  }
  imageTasks.value = [task, ...imageTasks.value].slice(0, maxImageTasks)
  activeImageTaskId.value = task.id
  imagePrompt.value = ''
  imageReference.value = null
  runImageTask(task)
}
async function runImageTask(task: ImageTask) {
  try {
    const data = task.referenceImage ? await requestImageEdit(task) : await requestJson(buildUrl('/v1/images/generations'), { model: task.model, prompt: task.prompt, quality: task.quality, size: task.size, n: task.count }, imageRequestTimeoutMs)
    const items = typeof data === 'object' && data !== null && 'data' in data ? (data as { data?: Array<{ url?: string, b64_json?: string }> }).data || [] : []
    const images = items.map(item => item.url || (item.b64_json ? `data:image/png;base64,${item.b64_json}` : '')).filter(Boolean)
    if (images.length === 0) throw new Error('接口已返回，但没有找到图片 URL 或 b64_json。')
    const completedAt = Date.now()
    updateImageTask(task.id, { status: 'success', images, completedAt })
    activeImageTaskId.value = task.id
    const entry: ImageHistoryEntry = { id: task.id, createdAt: completedAt, prompt: task.prompt, model: task.model, quality: task.quality, size: task.size, images }
    imageHistory.value = [entry, ...imageHistory.value].slice(0, maxHistoryItems)
    saveHistory()
  } catch (error) {
    updateImageTask(task.id, { status: 'error', error: extractErrorMessage(error, '生成失败：请检查 Key、模型、额度或稍后重试。') })
    activeImageTaskId.value = task.id
  }
}

</script>
