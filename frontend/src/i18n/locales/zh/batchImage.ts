/**
 * 批量生图使用说明里提供给 Codex 的 Skill 文档。
 *
 * 这是一个刻意放在下方消息树之外的具名导出，英文孪生版本位于
 * `locales/en/batchImage.ts` 的同一位置。原因是 vue-i18n 会在渲染时编译消息树里
 * 的每一条文案，而这段内容不是界面文案 —— 它是用户直接复制进自己 Agent 的约 65 行
 * 文档，里面包含一个 JSON 请求体和六条带字面量 `{id}` 段的 URL 模板。写进消息树的
 * 话，每一个花括号都要写成 `{'{'}` / `{'}'}`，请求体示例会变成不可读的噪音，而且
 * "某个花括号被悄悄吃掉" 会成为这个文件最可能出错的方式。保持普通模板字符串，
 * 意味着这里写的就是用户复制到的字节。
 *
 * 只有说明性的行文会翻译。字段名、参数名、URL、JSON 结构、字面量取值
 * （`"1K"`、`"image/png"`、`"img_001"`）、数字以及换行与缩进结构在两种语言里完全
 * 一致 —— 这些地方出错会直接破坏读者的工作流，而不只是读起来别扭。
 *
 * @param endpoint API 端点，已去掉末尾斜杠。
 */
export function batchImageAgentInstruction(endpoint: string): string {
  return `---
name: sub2api-batch-image
description: 当用户希望用 Gemini/Vertex 批量生成图片、批量跑提示词、下载批量生图结果、重试失败图片时使用。
---

你是 Codex 中的批量生图执行 Agent。用户不需要手动填写页面表单；你应从当前聊天、用户给的文件、目录或上下文中整理任务名称、prompt 列表和输出目录，只有缺少关键决策时才向用户提问。

默认端点：
${endpoint}

你需要自己完成：
1. 从用户聊天或附件中提取 prompt。每条 prompt 保留完整文本，按顺序生成稳定 custom_id，例如 img_001、img_002。
2. 从用户要求或上下文推断任务名称；没有明确名称时用当前时间生成任务名。
3. 从用户要求或上下文推断输出目录；如果用户没有说保存到哪里，才询问用户。
4. 提交前必须先计算 expected_output_count = 所有 item 的 output_count 之和。单个批量任务硬性最多 200 张输出图；超过 200 张必须拆成多组任务，不能提交一个超大任务，也不能把参考图附件上限当成生成张数上限。
5. 如果用户提供参考图，把参考图按用途绑定到具体 item。参考图只是输入附件，不是输出图数量。模型单条限制必须按模型执行：Gemini 2.5 Flash Image 每条最多 3 张参考图；Gemini 3 Pro Image 每条最多 14 张参考图。不要把后端附件风控理解成 Pro 单条能力：按 output_count 展开后，所有 item 的参考图附件总数还有内部保护阈值 1000 个，inline base64 参考图解码后总量最多 128MB。这个 1000 只是服务器拒绝异常请求的保护阈值，不是推荐规模；参考图很多或总请求体较大时应主动拆分任务。
6. 参考图会按 output_count 重复消耗输入 token；大量任务、重复复用同一张参考图或参考图总体积较大时，优先使用 gs:// file_uri 或拆分成多组任务。
7. 选择 API Key 和模型：先获取当前可用的批量生图 Key/模型；如果用户指定模型且该 Key 支持，则使用用户指定模型；否则使用该 Key 可用模型中的默认/第一个。不要展示或询问内部 provider 名称。
8. 调用批量生图 API 提交、轮询、下载，不要求用户去页面里手填。

API 调用规范：
- 模型：GET ${endpoint}/v1/images/batches/models
- 提交：POST ${endpoint}/v1/images/batches
- 查询：GET ${endpoint}/v1/images/batches/{id}
- 明细：GET ${endpoint}/v1/images/batches/{id}/items
- 下载：GET ${endpoint}/v1/images/batches/{id}/download
- 取消：POST ${endpoint}/v1/images/batches/{id}/cancel

提交请求体：
{
  "model": "<按所选 Key 可用模型填写>",
  "task_name": "<从聊天推断；为空则用当前时间>",
  "image_size": "1K",
  "response_mime_type": "image/png",
  "items": [
    {
      "custom_id": "img_001",
      "prompt": "<第一条完整 prompt>",
      "output_count": 1,
      "reference_images": [
        {
          "id": "face",
          "type": "subject",
          "mime_type": "image/png",
          "data": "<base64，不含 data:image/png;base64, 前缀>"
        }
      ]
    }
  ]
}

必须遵守：
- 不要把 API Key 写入仓库、日志、提交记录或最终回复。
- 不要把参考图 base64 写入最终回复、日志或公开文件。恢复记录中只保存参考图文件名、用途、数量和请求 JSON 文件路径；若请求 JSON 文件包含 base64，应保存在用户指定输出目录且不要提交到仓库。
- output_count 表示同一 prompt 和参考图重复生成几张，默认 1，每条最多 4；这不是依赖 Gemini 单次请求返回多图，而是系统展开成多个真实任务项。提交前必须确认预计输出图总数不超过 200，超过就拆分成多组任务。绝不能因为参考图附件有更高的内部保护阈值，就提交会生成超过 200 张图的任务。
- 当前对用户的批量生图计费仍按成功输出图片数量结算，不单独对参考图加价。可以向用户说明：参考图会产生少量上游输入 token 和临时存储成本，且会随 output_count 重复计算；页面显示的冻结/结算金额按输出图片数量计算。
- 提交成功后，必须立刻在输出目录写入本地恢复记录，例如 batch-image-resume.json。不要在恢复记录里保存 API Key。
- 恢复记录至少包含：endpoint、task_name、batch_id、model、output_dir、request_file、submitted_at、last_status、status_url、items_url、download_url、prompt_count、expected_output_count，以及可用于失败重试的 custom_id 到 prompt 映射或请求 JSON 文件路径。
- 每次查询状态后更新恢复记录，写入 last_checked_at、last_status、成功数、失败数、实际扣费和失败摘要。会话中断或暂停后，下次必须能凭该文件继续查询、下载或重试。
- 不要高频轮询。首次查询等待约 20 到 30 秒；queued 状态每 60 到 120 秒查询一次；如果连续 3 次仍是 queued，就先停止主动查询，告诉用户任务仍在排队，并保留恢复记录，之后可继续其他任务或等待用户稍后让你恢复。
- running 状态每约 60 秒查询一次，服务器压力大或大批量任务时可以更久；processing_results 等接近完成的状态可每 20 到 45 秒查询一次。
- 任务完成后报告任务名、任务 id、成功数、失败数、实际扣费和保存路径。
- 只下载成功图片。部分失败时，先展示失败 custom_id、错误码、错误来源和简要原因。
- 重试只能重试失败项，不能重复提交已成功项。若历史任务没有保存失败项 prompt，必须告诉用户无法自动重试，并询问用户是否提供原 prompt。
- 取消任务前必须提醒：已被系统索引为成功的图片仍会按成功项结算扣费，其余冻结金额会释放。
- 图片预览按需加载；不要为了查看列表自动批量加载图片内容。`
}

export default {
  batchImage: {
    columns: {
      taskName: '任务名称',
      model: '模型',
      apiKey: '提交密钥',
      result: '结果',
      cost: '费用',
      downloadStatus: '下载状态',
    },
    status: {
      queued: '排队中',
      running: '生成中',
      processingResults: '整理结果',
      settling: '结算中',
      completed: '已完成',
      failed: '失败',
      cancelled: '已取消',
      outputDeleted: '结果已删除',
      partialSuccess: '部分成功',
      allFailed: '全部失败',
    },
    itemStatus: {
      pending: '排队中',
      succeeded: '成功',
      failed: '失败',
      cancelled: '已取消',
      recovered: '已补成功',
    },
    filters: {
      searchTaskName: '搜索任务名称',
      allApiKeys: '全部 API Key',
      allStatuses: '全部状态',
      allDownloadStates: '全部下载状态',
      downloaded: '已下载',
      notDownloaded: '未下载',
    },
    actions: {
      usageGuide: '使用说明',
      createJob: '创建批量任务',
      downloadSelected: '下载选中',
      deleteRecords: '删除记录',
      retryFailedItems: '重试失败项',
      cancelJob: '取消任务',
      downloadZip: '下载 ZIP',
      viewDetail: '查看详情',
      download: '下载',
      moreActions: '更多操作',
      copyInstruction: '复制说明',
      submitJob: '提交任务',
    },
    list: {
      selectedJobs: '已选择 {count} 个任务',
      expandChildren: '展开 {n} 个子任务',
      collapseChildren: '收起子任务',
      childCount: '{n} 子任务',
      childBadge: '子任务',
      keyNotRecorded: '未记录',
      totalCount: '共 {n}',
      notDownloaded: '未下载',
      empty: '暂无批量任务',
      emptyHint: '点击右上角创建批量任务。',
    },
    pagination: {
      pageNumber: '第 {page} 页',
      pageItems: '本页 {count} 条',
    },
    promptPopover: {
      title: '完整 Prompt',
      copied: 'Prompt 已复制',
    },
    detail: {
      title: '任务详情',
      aggregatedResult: '汇总结果',
      result: '结果',
      cost: '费用',
      downloadStatus: '下载状态',
      items: '明细',
      customId: 'Custom ID',
      prompt: 'Prompt',
      preview: '预览',
      previewZoom: '放大压缩预览 {id}',
      previewReload: '重新加载压缩预览',
      previewLoad: '加载压缩预览',
      previewUnavailable: '不可预览',
      noImage: '无图片',
      loadingItems: '正在加载明细...',
      noItems: '暂无明细',
      noItemsHint: '排队或生成中的任务会先显示已提交的 prompt，结果整理完成后会更新图片状态。',
      mainTask: '主任务：{name}',
      childTask: '子任务：{name}',
      holdCost: '冻结 {amount}',
    },
    itemResult: {
      recoveredByRetry: '旧失败已由重试子任务补成功',
      readyPreview: '图片已生成，可点击预览',
      readyDownload: '图片已生成，可下载',
      noUsableImage: '未生成可用图片',
      cancelled: '任务已取消',
      waiting: '等待生成结果',
      emptyImageOutput: '上游返回了结果，但这条没有图片内容。通常是 Gemini/Vertex 单条生成失败或被安全策略拦截。',
      providerItemFailed: '上游返回的这条结果没有可用图片。',
    },
    imagePreview: {
      title: '图片预览',
      notice: '当前显示的是浏览器本地缓存的压缩缩略图，清晰度会有影响；需要查看原图请下载 ZIP。',
    },
    create: {
      title: '创建批量任务',
      taskName: '任务名称',
      taskNamePlaceholder: '不填写则默认使用当前时间',
      loadingKeys: '加载 API Key 中...',
      selectKeyPlaceholder: '请选择 Gemini API Key',
      noKeysHint: '当前没有可用于批量生图的 Gemini API Key。请先创建并绑定已开启批量生图的 Gemini 分组。',
      model: '模型',
      imageSize: '图片尺寸',
      imageSizeHint: '当前批量任务固定按 1K 图片提交。',
      outputFormat: '输出格式',
      estimatedOutput: '预计生成',
      estimatedOutputValue: '{images} 张 / {prompts} 条',
      promptSection: 'Prompt',
      promptAdded: '已添加 {count} 条',
      promptPlaceholder: '粘贴 prompt，添加后进入下方列表',
      customIdPlaceholder: 'Custom ID 可选',
      outputCountPerPrompt: '每条生成张数',
      outputCountOption: '{n} 张',
      referenceImage: '参考图',
      removeReferenceImage: '移除参考图',
      limitsHint: '每条最多 {maxPerItem} 张，整组最多 {maxPerJob} 张；当前模型每条最多 {refLimit} 张参考图，参考图按生成张数重复消耗输入 token。',
      referenceCount: '{n} 参考图',
      noPrompts: '还没有添加 prompt。',
      cancelNotice: '取消任务会请求上游取消；已被系统索引为成功的图片仍会按成功项结算扣费，其余冻结金额会释放。',
      submittingNotice: '正在创建上游批量任务，通常需要几秒，请不要重复提交。',
      modelNoReferenceImages: '当前模型不支持参考图。',
      refLimitReached: '当前模型每条最多 {limit} 张参考图。',
      refLimitExceededIgnored: '当前模型每条最多 {limit} 张参考图，已忽略超出的文件。',
      refFormatUnsupported: '参考图仅支持 PNG、JPEG 或 WebP。',
      refFileTooLarge: '{name} 超过 10MB，已忽略。',
    },
    guide: {
      title: '批量生图使用说明',
      uiTitle: '当前界面如何使用',
      step1: '1. 选择已开启批量生图的 Gemini API Key，模型列表会按该 Key 所属分组可用模型展示。',
      step2: '2. 任务名称可以留空，提交时会自动使用当前时间；Prompt 需要一条条添加到列表里，每条 Prompt 可附参考图，也可以设置重复生成张数。',
      step3: '3. 提交后任务会先排队，明细会展示已提交的 Prompt；图片预览默认不加载，点击明细里的预览按钮才会加载单张图。',
      step4: '4. 完成后可以下载 ZIP；部分失败时，更多菜单里可以只重试失败项。当前结算仍按成功输出图张数计算，不单独对参考图加价。',
      skillTitle: '给 Codex 的 Skill 说明',
      skillDesc: '用于告诉 Codex 如何代替用户整理 prompt、提交任务和下载结果。',
      /*
       * 既没有配置 API 基址、也拿不到 `window.location` 时用来占位。它会被打印进上方
       * 的 Skill 文档，所以保持和该文档里其他占位符一样的尖括号 "请自行填写" 形态。
       */
      endpointFallback: '<你的 Sub2API API 端点>',
    },
    messages: {
      loadKeysFailed: '加载 API Key 失败',
      loadModelsFailed: '加载可用模型失败',
      loadJobsFailed: '加载批量任务失败',
      selectApiKey: '请选择可用的 Gemini API Key',
      noModelsForKey: '当前密钥没有可用的批量生图模型',
      selectModel: '请选择模型',
      promptRequired: '请至少填写一条 prompt',
      submitted: '批量任务已提交',
      submitFailed: '提交失败',
      refreshFailed: '刷新失败',
      cancelConfirm: '取消会请求上游取消；已被系统索引为成功的图片仍会按成功项结算扣费，其余冻结金额会释放。确定取消吗？',
      cancelled: '已请求取消任务',
      cancelFailed: '取消失败',
      batchDownloadStarted: '已开始下载选中的任务',
      downloadFailed: '下载失败',
      retrySubmitted: '已提交失败项重试任务',
      retryFailed: '重试失败项失败',
      retryMissingPrompts: '这个任务没有保存失败项 prompt，无法自动重试。请复制原 prompt 后重新创建任务。',
      retryTaskNameSuffix: '重试失败项',
      deleteConfirm: '删除后这个任务会从你的列表隐藏，但账务记录仍会保留。确定删除吗？',
      deleteSelectedConfirm: '删除后选中的任务会从你的列表隐藏，但账务记录仍会保留。确定删除吗？',
      deleted: '任务记录已删除',
      deleteFailed: '删除任务记录失败',
      loadItemsFailed: '加载明细失败',
      loadPreviewFailed: '加载图片预览失败',
      copiedInstruction: '已复制批量生图说明',
      loadingModels: '加载可用模型中...',
      noModels: '无可用模型',
      noModelsHint: '当前密钥所属分组没有配置可用于批量生图的模型。',
      noCompatibleAccount: '当前密钥所属分组没有可用的批量生图上游账号。请联系管理员检查：该分组是否绑定了可调度的 Gemini API Key 或 Vertex 服务账号，以及账号是否支持所选模型。',
      unsupportedProvider: '这个任务使用的批量生图通道当前不可用。请联系管理员检查批量生图通道配置。',
      providerSubmitFailed: '上游批量生图任务提交失败。请联系管理员检查上游账号状态、模型权限或服务状态。',
      vertexGcsBucketMissing: 'Vertex 批量生图缺少托管 GCS 存储桶配置。请联系管理员配置 BATCH_IMAGE_VERTEX_MANAGED_GCS_BUCKET 后再提交。',
      queueFailed: '任务队列暂时不可用，批量任务没有成功入队。请联系管理员检查队列服务。',
      billingHoldFailed: '费用冻结失败，批量任务没有成功提交。请联系管理员检查余额冻结或计费服务。',
      groupDisabled: '当前密钥所属分组没有开启批量生图。你可以换一个已开启批量生图的密钥，或联系管理员开启。',
      pricingMissing: '所选模型还没有配置批量生图价格。请联系管理员补充价格配置。',
      insufficientBalance: '余额不足，无法冻结本次批量生图费用。',
      invalidModel: '请选择一个可用于当前密钥的批量生图模型。',
      invalidItems: 'Prompt 列表格式不正确，请检查是否为空、是否超过数量限制，或图片尺寸是否仍为 1K。',
      duplicateCustomId: 'Prompt 列表里的 custom_id 不能重复。',
      promptTooLong: '单条 prompt 过长，请缩短后重试。',
      invalidReferenceImage: '参考图格式不正确，请使用 10MB 以内的 PNG、JPEG 或 WebP。',
      tooManyReferenceImages: '参考图数量超过限制：Flash Image 每条最多 3 张，Pro Image 每条最多 14 张，整组最多 1000 张。',
      referenceImagesTooLarge: '参考图总量过大。inline 参考图整组最多 128MB；大量参考图请改用 gs:// file_uri 或拆分任务。',
      tooManyOutputImages: '预计生成张数超过限制：每条最多 4 张，整组最多 200 张。',
      idempotencyConflict: '这次提交和之前的请求标识冲突，请刷新页面后重新提交。',
      notReady: '任务还没有完成，完成后才能下载。',
      outputDeleted: '这个任务的结果文件已经被清理，无法下载。',
      resultMissing: '结果文件不可用，可能是上游结果文件已清理、存储权限异常，或管理员迁移过存储配置。请联系管理员检查结果文件。',
      itemFailed: '这条明细没有成功图片，无法预览。',
      itemImageIndexOutOfRange: '这条明细没有可预览的图片。',
      downloadLimited: '当前下载请求太多，请稍后再试。',
      downloadTooLarge: '这个 ZIP 太大，已超过单次下载限制。请减少单次下载数量，或联系管理员调整批量下载上限。',
      deleteNotReady: '任务结束后才能删除记录。正在生成或结算中的任务请先等待完成。',
      disabled: '批量生图功能当前未开启。',
      authRequired: '当前 API Key 不可用或已失效，请重新选择密钥。',
      adminReference: '请把错误码和请求 ID 发给管理员排查。',
      errorReference: '错误信息',
      errorCodeRef: '错误码：{code}',
      requestIdRef: '请求 ID：{id}',
      httpStatusRef: 'HTTP 状态：{status}',
    },
  },
}
