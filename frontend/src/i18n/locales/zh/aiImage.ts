export default {
  aiImage: {
    title: 'AI生图',
    subtitle: '填入平台创建的密钥，输入提示词即可生成或改图。',

    modules: {
      title: '功能模块',
      hint: '点一下展开，再点一下收起。一次只开一个。',
      primary: '主要功能',
      primaryHint: '单张图片放入去生图，直接在下面写提示词就行。',
      secondary: '辅助功能',
    },

    /**
     * 单张生图。
     * 主人要求它也是一个「点按钮才展开的页面」，不再是常驻输入框 ——
     * 因为「不可以两个及以上的功能页面同时出现，单张图片生成也不行」。
     */
    single: {
      title: '单张生图',
      hint: '写提示词，放不放参考图都行。',
    },

    key: {
      sectionTitle: '生图密钥',
      label: '平台密钥',
      placeholder: '粘贴平台创建的密钥（sk- / cns- 开头）',
      hint: '密钥来自本平台「API 密钥」页面，仅保存在你自己的浏览器里。',
      save: '保存密钥',
      configure: '填入密钥',
      change: '更换密钥',
      saved: '已保存',
      clear: '清除密钥',
      checking: '正在校验密钥…',
      valid: '密钥可用：{group}',
      validNoGroup: '密钥可用',
      invalid: '密钥无效或已停用，请检查后重试。',
      notImageModel: '这个密钥不是生图模型',
      notImageModelHint:
        '该密钥所属分组（{group}）没有开启生图能力，无法用于 AI 生图。请到「API 密钥」换一个属于生图分组的密钥，或联系管理员为分组开启生图。',
      notOwned:
        '这个密钥不属于当前账号，无法在页面内校验。请使用当前账号在本平台创建的密钥。',
      empty: '请先填入密钥。',
      loadFailed: '读取密钥列表失败，请稍后重试。',
    },

    /**
     * 视觉模型密钥。
     *
     * 平台按分组隔离模型：生图分组通常只绑了生图渠道，分组里没有视觉模型。
     * 拿生图密钥去调视觉模型，网关在路由阶段就返回 503，请求根本到不了上游。
     * 所以「AI 识别卖点」「拆解爆款版式」要单独一把能调视觉模型的密钥。
     */
    visionKey: {
      sectionTitle: '视觉模型密钥',
      // 文案必须和生图密钥的「保存密钥」区分开：页面上同时存在两个保存按钮时，
      // 两个都叫「保存密钥」会让用户不知道自己点的是哪一个。
      save: '保存视觉密钥',
      placeholder: '粘贴能调用视觉模型（Gemini 等）的密钥',
      // 主人要求：这里只留这一句，其它解释都删掉。
      hint: '要一把所属分组含 Gemini 等视觉模型渠道的密钥。',
      valid: '视觉密钥可用：{group}',
      validNoGroup: '视觉密钥可用',
      usingImageKey: '未填，暂用生图密钥',
      missing: '未填',
      invalidTitle: '视觉密钥无效',
      invalid: '这把密钥无效或已停用，请检查后重试。',
      modelLabel: '视觉模型',
      // 主人点名要的提示：这个框里的模型请选 Gemini 渠道能提供的。
      modelHint:
        '这个模型请选 Gemini 渠道（gemini-3.5-flash / gemini-3.8-flash / gemini-3.1-pro-preview 等）。视觉模型和生图模型不在同一个分组，模型选对了、密钥分组不对照样调不通。',
      needed: '请先填入「视觉模型密钥」——「AI 识别卖点」和「拆解爆款版式」需要它。',
      failedRouting:
        '视觉模型调用失败：这把密钥所在的分组不能调用视觉模型（请求在平台路由阶段就被拒了，没有到达上游）。请换一把所属分组含 Gemini 等视觉模型的密钥——换模型或稍后重试都不会有用。',
    },

    prompt: {
      label: '提示词',
      placeholder: '描述你要生成或修改的内容，例如：把这张产品图换成白色背景、3:4 竖版、加俄语卖点文案',
      generate: '开始生成',
      generating: '生成中…',
      clear: '清空',
    },

    template: {
      title: '提示词模板',
      label: '提示词模板',
      placeholder: '选择已保存的模板…',
      promptLabel: '提示词（存为模板／套用模板都用这里的内容）',
      generateHint: '想生成的话，去「单张生图」里点开始生成，提示词已经带过去了。',
      save: '存为模板',
      delete: '删除模板',
      namePlaceholder: '给这个模板起个名字',
      confirm: '保存',
      cancel: '取消',
      applied: '已套用模板「{name}」',
      saved: '模板「{name}」已保存',
      overwritten: '模板「{name}」已更新',
      deleted: '模板「{name}」已删除',
      saveFailed: '模板没能存下来：浏览器存储不可用或已满。',
      needPrompt: '请先写好提示词，再存为模板。',
      needName: '请给模板起个名字。',
    },

    hot: {
      title: '爆款复刻',
      hint:
        '传几张同行的爆款主图，让 AI 拆出版式（布局/配色/文案位置等 9 项），再把这份版式套到你自己的商品上生成整套主图。',
      pick: '添加爆款参考图',
      clear: '清空',
      picked: '已添加 {count} 张爆款参考图。',
      analyzing: '正在拆解…',
      analyze: '拆解爆款版式',
      progress: '已拆 {done}/{total} 张',
      styleTitle: '拆解出的版式（多张取共同特征）',
      analyzed: '已拆解 {count} 张爆款，共同特征已合并。',
      extras: '额外再生成 4 张（侧面多角度 / 细节特写 / 使用场景 / FAQ 对比）',
      buildScript: '按爆款版式生成脚本',
      scriptBuilt: '已按爆款版式生成 {count} 张脚本，可在下面逐张生成或一键生成全部。',
      needRef: '请先添加至少一张爆款参考图。',
      needProduct: '请先填上你的产品名称（就在这一步下面的「产品名称」里）。',
      needTemplate:
        '还没有可用的版式模板 —— 要么在上面传爆款图点「拆解爆款版式」，要么在「提示词模板」里选一个已存的模板。',
      templateReady: '已就绪：本次复刻用的模板已确定，填好产品名就能生成。',
      noResult: '这几张图都没能拆解出结果，换几张试试，或确认视觉模型可用。',
      oneFailed: '有一张拆解失败（已跳过，其余继续）：{reason}',
      step2: '第 2 步：放你自己商品的图，并填商品信息（这一步是独立的，不用先去填「套图」）',
      // 我自己的商品图：爆款复刻是「用我的商品主体套同行的版式」，必须有这个入口。
      pickProductImage: '选择我的商品图',
      replaceProductImage: '更换我的商品图',
      noProductImage: '还没选商品图（生成时会用它当商品主体）',
      // 卖点/痛点/差异化交给视觉模型；痛点和差异化是「跟这张商品图对比」得出的。
      extract: 'AI 识别卖点 / 痛点 / 差异化',
      extracting: '正在识别…',
      extractHint:
        '放上商品图后点识别：卖点由视觉模型生成，痛点和差异化是拿这件商品跟同类普通款对比得出的。识别完仍可手动修改。',
      extracted: '已识别并填入卖点、痛点、差异化（可直接修改）。',
      extractFailed: '识别失败',
      needProductImage: '请先选一张你自己的商品图，识别要用它。',
    },

    composite: {
      title: '一图多视',
      pick: '选择多张商品图',
      working: '正在拼接…',
      hint: '选 2 张以上，横排拼成一张长图当参考图（让 AI 一次看全商品各个角度）。',
      done: '已把 {count} 张拼成一张参考图。',
      tooMany: '最多拼 {max} 张，超出的已忽略（拼太长会被生图接口拒绝）。',
      failed: '拼接失败：{reason}',
      // 拼出来的图是给「单张生图」当参考图用的，这里说清楚下一步去哪。
      nextStep: '拼好后去「单张生图」里，它已经在参考图那一栏了，点开始生成就行。',
    },

    watermark: {
      title: '水印',
      enable: '下载时加水印',
      text: '水印文字',
      alpha: '浓度',
      fontSize: '字号',
      auto: '（0=自动）',
      download: '加水印下载',
      done: '已加水印并开始下载。',
      corsBlocked: '上游图片不允许跨域读取，没法在浏览器里加水印，已改为直接打开原图。',
      // 水印只在「下载」时叠加，本身不生成图，说清楚去哪下载。
      hint: '水印只在下载时叠上去，不改变已生成的图。在图下面的「加水印下载」按钮上点一下就会带上。',
      note:
        '两个和桌面程序不一样的地方（都是刻意改的）：一是水印是正的隔行交错网格，桌面程序的设置窗虽然写着「斜向满屏」，但它的代码里其实没有旋转；二是参考图一律用没打水印的干净图，这样成图上只会有一层水印，不会叠两层。',
    },

    filter: {
      label: '状态',
      all: '全部',
      completed: '已完成',
      generating: '生成中',
      failed: '失败',
      empty: '当前筛选下没有记录',
    },

    reference: {
      label: '参考图',
      pick: '选择参考图',
      replace: '更换参考图',
      clear: '移除参考图',
      hint: '上传参考图走图生图（改图）；不上传则按提示词纯文生图。',
      none: '未选择（纯文生图）',
    },

    product: {
      label: '商品编号',
      placeholder: '你自己用来区分的标记，例如 Ozon 货号',
      hint: '只是给你自己看的标记，方便回头区分哪个商品的图；客户端存图用的是它内部自己的商品 ID，不需要你填。真正要交给客户端的是下面的图片地址。',
      copied: '图片地址已复制，粘到客户端即可上架。',
      copyFailed: '复制失败，请手动选中复制。',
      copyUrl: '复制图片地址',
      copyAll: '复制全部地址（{count} 张）',
      copiedAll: '已按顺序复制 {count} 个图片地址，粘到客户端就会按顺序存成第 1、2、3… 张。',
      urlLabel: '图片地址',
      noUrl: '这次返回的是 base64，没有可直接给客户端用的地址。',
    },

    lock: {
      label: '锁定商品',
      hint: '开启后，生成的图只换背景/光线/排版/文字，商品本身保持不变——保证图里的商品和客户端显示的是同一件。',
      on: '已锁定',
      off: '未锁定',
      needsReference: '上传参考图后，锁定商品才会生效。',
    },

    script: {
      title: '套图',
      toggle: '展开 / 收起',
      product: '产品名称',
      productPlaceholder: '例：316 不锈钢保温杯',
      sp1: '卖点 1',
      sp2: '卖点 2',
      sp3: '卖点 3',
      pain: '用户痛点',
      diff: '差异化',
      scene: '自动匹配场景',
      scenePlaceholder: '填产品名称后自动匹配',
      build: '生成 9 张脚本',
      built: '已生成 {count} 张，点下面任意一张填入提示词。',
      applyAll: '全部填入（换行分隔）',
      fillOne: '填入提示词',
      aiExtract: 'AI 识别卖点',
      aiExtracting: '识别中…',
      aiExtracted: 'AI 已识别并自动填入，请检查后微调。',
      printedRu: '图上会印的俄语文案：',
      printedRuNone: '还没有俄语版卖点：AI 识别会自动生成；手填中文的话，图上就会是中文。',
      aiNeedsImage: '请先上传产品图，再让 AI 识别卖点。',
      aiFailed: 'AI 识别失败',
      clear: '清空',
    },

    batch: {
      concurrency: '并发数',
      generateAll: '一键生成全部（{count} 张）',
      running: '正在批量生成…',
      progress: '已完成 {done} / {total}',
      failedCount: '其中失败 {count} 张',
      paused: '已暂停',
      pause: '暂停',
      resume: '继续',
      stop: '停止',
      stopHint: '停止后不再提交新任务，正在生成的那张会跑完。',
      needScript: '还没有脚本 —— 先在「爆款复刻」或「套图」里生成脚本，才能一键生成全部并暂停/停止。',
      fromHot: '这组脚本来自「爆款复刻」（已套用拆解出的爆款版式）。',
      fromHotSaved: '这组脚本来自「爆款复刻」（已套用「提示词模板」里选中的模板）。',
      fromOzon: '这组脚本来自「套图」。',
    },

    options: {
      model: '模型',
      size: '尺寸',
      count: '张数',
      countHint: '最多可生成 {max} 张图片',
    },

    result: {
      sectionTitle: '生成结果',
      currentRound: '本轮生成',
      history: '历史记录',
      empty: '生成的图片会显示在这里',
      emptyHint: '左侧选择状态可筛选历史记录。',
      // 各功能页里那个结果区的标题：只显示本功能做出来的图。
      thisPanel: '本功能的结果',
      panelEmpty: '这个功能还没有生成过图片',
      failedReason: '失败原因',
      imageNotRetained: '这张图只保留在当前会话里（接口返回的是 base64），刷新后需要重新生成。',
      retry: '重试',
      download: '下载',
      remove: '删除',
      copyPrompt: '复制提示词',
      copied: '已复制',
      promptLabel: '提示词',
      modelLabel: '模型',
      sizeLabel: '尺寸',
      timeLabel: '时间',
      openInNewTab: '新窗口打开',
      downloadAll: '下载全部',
    },

    errors: {
      missingKey: '请先填入并保存生图密钥。',
      missingPrompt: '请输入提示词。',
      badKey: '密钥无效或已停用。',
      noImagePermission: '这个密钥不是生图模型：所属分组未开启生图能力。',
      rateLimited: '请求过于频繁，请稍后再试。',
      insufficient: '余额或额度不足，请先充值。',
      timeout: '生成超时，请稍后重试。',
      network: '网络异常，请检查网络后重试。',
      unknown: '生成失败，请稍后重试。',
    },
  },
}
