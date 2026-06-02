(function () {
  window.DOCS_I18N = {
    zh: {
      lang: "zh-CN",
      localeLabel: "语言",
      viewDocs: "查看文档",
      goLogin: "登录",
      dashboard: "控制台",
      themeDark: "切换到深色模式",
      themeLight: "切换到浅色模式",
      titleSuffix: "大模型 API 接入文档",
      messages: {
        copy: "复制",
        copied: "已复制",
        copyFailed: "复制失败，请手动复制。",
        pasteApiKeyFirst: "请先粘贴 API Key。",
        tryingOpen: "正在尝试拉起 CC-Switch...",
        clientNotOpened:
          "没有拉起客户端。请先安装 CC-Switch，或检查协议处理程序是否已注册。",
        deepLinkCopied: "CC-Switch 深链已复制。",
        importFailed: "导入失败。",
        copyRootHost: "复制根 Host",
        copyV1: "复制 /v1",
      },
      hero: {
        priority: "CC-Switch 优先",
        defaultHostLabel: "默认 Host：",
        docPathLabel: "文档路径：",
        title: "大模型 API 接入文档",
        copyHtml:
          '推荐流程：先进入 <code>/keys</code> 创建或复制 API Key，再优先点击 <code>导入到 CCS</code> 一键导入到 CC-Switch。其余客户端按对应协议接入：OpenAI SDK / HTTP 使用 <code>/v1</code>，Claude Code 使用根 Host，Gemini / OpenClaw / Hermes 按各自配置格式接入。',
        actions: ["进入 API 密钥", "下载 CC-Switch", "查看 Host 列表"],
        statLabels: ["默认主 Host", "站内取 Key 路径", "文档放置方式", "兼容协议"],
        keyPathHtml:
          "<code>/keys</code> -> 创建密钥 / 使用密钥 / 导入到 CCS",
        placement:
          "静态页，适合挂到文档链接或自定义菜单",
        protocols:
          "OpenAI / Claude / Gemini / Responses / OpenClaw / Hermes",
      },
      sidebar: {
        title: "目录",
        links: [
          "快速开始",
          "CC-Switch",
          "支持平台概览",
          "Host 列表",
          "HTTP 示例",
          "JavaScript SDK",
          "Python SDK",
          "Claude Code",
          "Gemini CLI",
          "Codex CLI",
          "OpenCode",
          "OpenClaw",
          "Hermes",
          "如何开启 Codex 插件",
          "常见问题",
        ],
      },
      quickstart: {
        title: "快速开始",
        descHtml:
          '默认推荐走 CC-Switch。它和站内 <code>API 密钥</code> 页面已经对齐，拿到 Key 后可以直接一键导入。',
        cards: [
          {
            title: "获取 API Key",
            bodyHtml:
              '登录站点后进入 <code>/keys</code>。创建密钥后，列表行内可以直接看到 <code>使用密钥</code> 和 <code>导入到 CCS</code> 两个动作。',
          },
          {
            title: "优先一键导入",
            bodyHtml:
              '点击 <code>导入到 CCS</code>。OpenAI 分组会导入为 Codex，Anthropic 分组导入为 Claude Code，Gemini 分组导入为 Gemini CLI，Antigravity 会先让你选 Claude 或 Gemini。',
          },
          {
            title: "按客户端使用",
            bodyHtml:
              "如果不使用 CC-Switch，按下方目录选择对应格式：HTTP、OpenAI SDK、Claude Code、Gemini CLI、Codex CLI、OpenClaw、Hermes。",
          },
        ],
        noteHtml:
          '后台管理员如果隐藏了 <code>导入到 CCS</code> 按钮，用户仍然可以在本页使用“手动导入 CC-Switch”工具，或按下方 CLI / SDK 示例手动配置。',
      },
      ccswitch: {
        title: "CC-Switch",
        descHtml:
          '站内默认推荐使用 CC-Switch。先在站内 <code>/keys</code> 获取 API Key，再导入到对应客户端配置。',
        cards: {
          download: {
            title: "下载安装索引",
            items: [
              '官方 Releases：<a href="https://github.com/farion1231/cc-switch/releases" target="_blank" rel="noopener noreferrer">GitHub Releases</a>',
              'Windows：推荐 <code>CC-Switch-*-Windows.msi</code>；便携版可用 <code>Windows-Portable.zip</code>',
              'macOS：推荐 <code>macOS.zip</code>；Homebrew 可用 <code>brew tap farion1231/ccswitch</code> + <code>brew install --cask cc-switch</code>',
              'Linux：Debian/Ubuntu 用 <code>.deb</code>，Fedora/RHEL 用 <code>.rpm</code>，通用可用 <code>.AppImage</code>',
            ],
            actions: ["打开下载页", "去 API 密钥"],
          },
          oneClick: {
            title: "站内一键导入位置",
            items: [
              "路径：<code>/keys</code>",
              '拿到 Key 后，点击同一行的 <code>导入到 CCS</code>',
              'Antigravity 分组会先选择导入到 <code>Claude Code</code> 或 <code>Gemini CLI</code>',
              "导入完成后，在 CC-Switch 对应应用的 Provider 列表里查看",
            ],
            noteHtml:
              '如果点击导入后没有拉起客户端，通常是 CC-Switch 未安装，或系统还没有注册 <code>ccswitch://</code> 协议。',
          },
          importMap: {
            title: "导入映射",
            headers: ["站内分组", "导入应用", "CC-Switch Endpoint"],
            rows: [
              ["OpenAI", "Codex", "<code>HOST</code>"],
              ["Anthropic / Claude", "Claude Code", "<code>HOST</code>"],
              ["Gemini", "Gemini CLI", "<code>HOST</code>"],
              [
                "Antigravity",
                "Claude Code / Gemini CLI",
                "<code>HOST/antigravity</code>",
              ],
            ],
          },
          manual: {
            title: "手动导入 CC-Switch",
            labels: ["Host", "导入类型", "Provider 名称", "API Key"],
            hostHelpHtml:
              '默认优先使用 <span data-default-host-text>https://re.94xy.cn</span>。',
            apiKeyPlaceholder: "粘贴从 /keys 复制出来的 API Key",
            buttons: ["导入到 CC-Switch", "复制深链"],
            options: [
              "OpenAI / Codex",
              "Claude Code",
              "Gemini CLI",
              "Antigravity -> Claude",
              "Antigravity -> Gemini",
            ],
          },
        },
      },
      overview: {
        title: "支持平台概览",
        descHtml:
          '不同客户端读取 Host 的方式不一样。HTTP / OpenAI SDK 通常需要显式写 <code>/v1</code>；站内 CLI 配置模板大多使用根 Host。',
        headers: ["平台 / 客户端", "协议", "推荐写法", "说明"],
        rows: [
          [
            "HTTP / OpenAI SDK",
            "OpenAI Chat / Responses",
            '<code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>',
            "标准服务端与脚本最稳妥",
          ],
          [
            "Claude Code",
            "Anthropic 兼容",
            '<code><span data-default-host-text>https://re.94xy.cn</span></code>',
            "使用 <code>ANTHROPIC_BASE_URL</code>",
          ],
          [
            "Gemini CLI",
            "Gemini CLI 兼容",
            '<code><span data-default-host-text>https://re.94xy.cn</span></code>',
            "使用 <code>GOOGLE_GEMINI_BASE_URL</code>",
          ],
          [
            "Codex CLI",
            "OpenAI Responses",
            '<code><span data-default-host-text>https://re.94xy.cn</span></code>',
            "按站内生成模板写入 <code>~/.codex</code>",
          ],
          [
            "OpenCode",
            "OpenAI / Anthropic / Google Provider",
            '<code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>',
            "OpenAI 分组按 <code>baseURL</code> 配置",
          ],
          [
            "OpenClaw",
            "Custom Provider",
            '<code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>',
            '按 <code>~/.openclaw/openclaw.json</code> 写 <code>models.providers</code>',
          ],
          [
            "Hermes",
            "Custom Endpoint",
            '<code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>',
            "当前版本使用 <code>~/.hermes/config.yaml</code>",
          ],
        ],
      },
      hosts: {
        title: "API 接口地址 Host 列表",
        descHtml:
          '默认主 Host 为 <code>re.94xy.cn</code>。如果后台配置了公开的 <code>API Base URL</code> 或自定义端点，下方会自动带出来。',
        noteHtml:
          'HTTP / OpenAI SDK 通常写 <code>HOST/v1</code>；Gemini / OpenClaw / Hermes 的详细格式看各自章节。站内 <code>API 密钥</code> 页面也会展示管理员配置的可用 Host。',
      },
      http: {
        title: "HTTP 示例",
        descHtml:
          '直接脚本、后端服务、第三方网关优先用 OpenAI 兼容格式。下方示例默认使用 <code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>。',
      },
      sdkJs: {
        title: "JavaScript SDK",
        descHtml:
          "只要 SDK 支持自定义 <code>baseURL</code>，大多数都能直接接入。",
      },
      sdkPython: {
        title: "Python SDK",
        descHtml:
          'Python 接入方式和 JavaScript 一样，关键是把 <code>base_url</code> 指向网关的 <code>/v1</code>。',
      },
      claude: {
        title: "Claude Code",
        descHtml:
          'Claude Code 按站内现有模板使用根 Host，不额外拼 <code>/v1</code>。',
      },
      gemini: {
        title: "Gemini CLI",
        descHtml:
          "Gemini CLI 也使用根 Host。模型名按你当前可用权限调整。",
      },
      codex: {
        title: "Codex CLI",
        descHtml:
          '站内 <code>使用密钥</code> 页面给 Codex CLI 生成的是 <code>~/.codex/config.toml</code> 与 <code>auth.json</code> 两个文件。以下格式与站内模板一致。',
      },
      codexplusplus: {
        title: "如何开启 Codex 插件（Codex++）",
        descHtml:
          "如果你使用第三方 API 或本站中转，不想只依赖官方 ChatGPT 登录态，可以用 Codex++ 给 Codex App 开启增强插件和中转 Provider。Codex++ 是外部启动器，不改 Codex App 安装文件，而是通过 Chromium DevTools Protocol 注入增强脚本。",
        features: {
          title: "它能做什么",
          items: [
            "开启 Codex App 的插件入口、特殊插件安装、会话删除、Markdown 导出、项目移动、Timeline 等增强能力。",
            "通过“中转注入”写入 <code>CodexPlusPlus</code> Provider，让 Codex App 使用第三方 API Base URL 与 API Key。",
            "支持多个中转配置，并可在增强 Provider 与官方 ChatGPT 登录态之间切换。",
            "独立管理用户脚本，启动 Codex 时自动注入自定义脚本。",
          ],
          actions: ["打开 Codex++ 仓库", "下载 Releases"],
        },
        setup: {
          title: "第三方 API 开启步骤",
          items: [
            "先安装官方 Codex App，再从 Codex++ Releases 下载并安装对应系统版本。",
            "进入本站 <code>/keys</code> 创建或复制 API Key。",
            "打开 <code>Codex++ 管理工具</code>，进入中转注入 / Relay 配置。",
            "新增一个配置：Base URL 填 <code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>，API Key 填本站 Key，模型按可用权限填写，例如 <code>gpt-5.4</code>。",
            "保存后用 <code>Codex++</code> 或管理工具启动 Codex App，确认当前 Provider 为 <code>CodexPlusPlus</code>。",
          ],
          warningHtml:
            "Codex++ README 的中转注入示例使用 OpenAI Responses 兼容地址，通常应填写 <code>HOST/v1</code>。如果你的 Codex++ 版本界面明确提示只填根地址，再按界面提示填写 <code>HOST</code>。",
        },
        noteHtml:
          "如果启动后没有看到插件或 Provider，先在 Codex++ 管理工具里运行检查 / 修复，再确认 Codex App 版本更新后是否需要等待 Codex++ 适配。",
      },
      opencode: {
        title: "OpenCode",
        descHtml:
          'OpenCode 建议写入 <code>opencode.json</code>。OpenAI 分组的核心是 <code>provider.openai.options.baseURL</code> 与 <code>apiKey</code>。',
      },
      openclaw: {
        title: "OpenClaw",
        descHtml:
          'OpenClaw 当前推荐按 <code>~/.openclaw/openclaw.json</code> 的 <code>models.providers</code> 写自定义 Provider。这里采用站内实际生成格式：OpenAI 分组使用 <code>api: "openai-responses"</code>。',
        noteHtml:
          '参考 OpenClaw 官方模型提供者文档：自定义 Provider 建议放在 <code>models.providers</code> 下，并按实际后端选择 <code>openai-responses</code> 或其他 API 类型。',
      },
      hermes: {
        title: "Hermes",
        descHtml:
          'Hermes 当前版本以 <code>~/.hermes/config.yaml</code> 为准。官方文档已经明确：旧的 <code>OPENAI_BASE_URL</code> / <code>LLM_MODEL</code> 环境变量不再作为主配置来源。',
        interactiveTitle: "交互式配置",
        warningHtml:
          '当前 Hermes 的自定义端点以 <code>config.yaml</code> 为单一事实来源。不要再依赖旧版 <code>.env</code> 里的 <code>OPENAI_BASE_URL</code>。',
      },
      faq: {
        title: "常见问题",
        headers: ["问题", "排查方向"],
        rows: [
          [
            "CC-Switch 一键导入没有拉起",
            '确认已安装 CC-Switch，且系统已注册 <code>ccswitch://</code> 协议；否则先走安装。',
          ],
          [
            "<code>401</code> / <code>invalid_api_key</code>",
            '检查 Key 是否从 <code>/keys</code> 复制完整，确认没有多空格、换行或旧 Key。',
          ],
          [
            "<code>403</code>",
            "密钥可能被禁用、触发 IP 限制，或当前分组不可用。",
          ],
          [
            "<code>404</code>",
            '大多数是 Host 或路径写错。HTTP / SDK 常用 <code>/v1</code>；Claude / Gemini / Codex CLI 多数写根 Host。',
          ],
          [
            "Hermes / OpenClaw 不工作",
            '先确认配置文件路径正确，再确认 Host 是否按当前客户端格式填写。OpenClaw 看 <code>openclaw.json</code>，Hermes 看 <code>~/.hermes/config.yaml</code>。',
          ],
        ],
      },
      footerHtml: '静态文档地址：<code>/docs/api.html</code>',
      codeTitles: {
        httpChat: "cURL /v1/chat/completions",
        httpModels: "cURL /v1/models",
        sdkJs: "openai npm",
        sdkPython: "openai python",
        claude: "macOS / Linux",
        gemini: "macOS / Linux",
        codexConfig: "~/.codex/config.toml",
        codexAuth: "~/.codex/auth.json",
        opencode: "opencode.json",
        openclaw: "~/.openclaw/openclaw.json",
        hermes: "~/.hermes/config.yaml",
        hermesInteractive: "交互式配置",
      },
      codeSamples: {
        httpChat: `curl https://re.94xy.cn/v1/chat/completions \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-5.4",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "请给我一份三点接入建议"}
    ]
  }'`,
        hermesInteractive: `hermes model
# 选择 "Custom endpoint (self-hosted / VLLM / etc.)"
# API base URL: https://re.94xy.cn/v1
# API key: YOUR_API_KEY
# Model name: gpt-5.4`,
      },
      hostCard: {
        fallbackDesc: "公开可访问的网关入口",
        labels: {
          root: "根 Host",
          openai: "OpenAI SDK / HTTP",
          gemini: "Gemini / OpenClaw / Hermes",
          antigravity: "Antigravity",
        },
        builtin: {
          default: {
            name: "默认主 Host",
            description: "文档默认推荐值",
          },
          current: {
            name: "当前访问地址",
            description: "当前浏览器访问的站点地址",
          },
          apiBase: {
            name: "后台 API Base URL",
            description: "系统设置 -> 通用设置 中公开的 API Base URL",
          },
          customPrefix: "自定义端点",
          customDescription: "后台公开的自定义 Host",
        },
      },
    },
    en: {
      lang: "en",
      localeLabel: "Language",
      viewDocs: "Docs",
      goLogin: "Login",
      dashboard: "Dashboard",
      themeDark: "Switch to dark mode",
      themeLight: "Switch to light mode",
      titleSuffix: "API Integration Guide",
      messages: {
        copy: "Copy",
        copied: "Copied",
        copyFailed: "Copy failed. Please copy it manually.",
        pasteApiKeyFirst: "Paste an API key first.",
        tryingOpen: "Trying to open CC-Switch...",
        clientNotOpened:
          "The client was not opened. Install CC-Switch or check whether the protocol handler is registered.",
        deepLinkCopied: "CC-Switch deep link copied.",
        importFailed: "Import failed.",
        copyRootHost: "Copy root host",
        copyV1: "Copy /v1",
      },
      hero: {
        priority: "CC-Switch First",
        defaultHostLabel: "Default Host:",
        docPathLabel: "Docs path:",
        title: "API Integration Guide",
        copyHtml:
          'Recommended flow: go to <code>/keys</code> to create or copy an API key, then click <code>Import to CCS</code> to import it into CC-Switch in one step. For other clients, use the matching protocol: OpenAI SDK / HTTP uses <code>/v1</code>; Claude Code uses the root host; Gemini / OpenClaw / Hermes use their own config formats.',
        actions: ["Open API Keys", "Download CC-Switch", "View Host List"],
        statLabels: [
          "Default host",
          "Where to get keys",
          "Recommended placement",
          "Supported protocols",
        ],
        keyPathHtml:
          "<code>/keys</code> -> create key / use key / import to CCS",
        placement:
          "Static page, suitable for the Docs link or a custom menu entry",
        protocols:
          "OpenAI / Claude / Gemini / Responses / OpenClaw / Hermes",
      },
      sidebar: {
        title: "Contents",
        links: [
          "Quick Start",
          "CC-Switch",
          "Platform Overview",
          "Host List",
          "HTTP Examples",
          "JavaScript SDK",
          "Python SDK",
          "Claude Code",
          "Gemini CLI",
          "Codex CLI",
          "OpenCode",
          "OpenClaw",
          "Hermes",
          "Enable Codex Plugins",
          "FAQ",
        ],
      },
      quickstart: {
        title: "Quick Start",
        descHtml:
          'CC-Switch is the default recommendation. It already matches the <code>API Keys</code> page on this site, so once you have a key you can import it directly.',
        cards: [
          {
            title: "Get an API key",
            bodyHtml:
              'After logging in, open <code>/keys</code>. Once a key is created, each row shows both <code>Use Key</code> and <code>Import to CCS</code>.',
          },
          {
            title: "Prefer one-click import",
            bodyHtml:
              'Click <code>Import to CCS</code>. OpenAI groups are imported as Codex, Anthropic groups as Claude Code, Gemini groups as Gemini CLI, and Antigravity lets you choose Claude or Gemini first.',
          },
          {
            title: "Choose by client",
            bodyHtml:
              "If you do not use CC-Switch, pick the matching format below: HTTP, OpenAI SDK, Claude Code, Gemini CLI, Codex CLI, OpenClaw, or Hermes.",
          },
        ],
        noteHtml:
          'If the admin hides the <code>Import to CCS</code> button, users can still use the manual CC-Switch import tool on this page or configure the client manually with the CLI / SDK examples below.',
      },
      ccswitch: {
        title: "CC-Switch",
        descHtml:
          'CC-Switch is the default recommendation on this site. First get an API key from <code>/keys</code>, then import it into the target client.',
        cards: {
          download: {
            title: "Download Index",
            items: [
              'Official releases: <a href="https://github.com/farion1231/cc-switch/releases" target="_blank" rel="noopener noreferrer">GitHub Releases</a>',
              'Windows: use <code>CC-Switch-*-Windows.msi</code>; a portable build is available as <code>Windows-Portable.zip</code>',
              'macOS: use <code>macOS.zip</code>; Homebrew is also available with <code>brew tap farion1231/ccswitch</code> + <code>brew install --cask cc-switch</code>',
              'Linux: use <code>.deb</code> for Debian/Ubuntu, <code>.rpm</code> for Fedora/RHEL, or <code>.AppImage</code> for a generic build',
            ],
            actions: ["Open download page", "Open API Keys"],
          },
          oneClick: {
            title: "Where one-click import lives",
            items: [
              "Path: <code>/keys</code>",
              'After you get the key, click <code>Import to CCS</code> on the same row',
              'For Antigravity groups, choose either <code>Claude Code</code> or <code>Gemini CLI</code> first',
              "After import, check the provider list for that app inside CC-Switch",
            ],
            noteHtml:
              'If clicking import does not open the client, CC-Switch is usually not installed yet, or the <code>ccswitch://</code> protocol has not been registered on the system.',
          },
          importMap: {
            title: "Import Mapping",
            headers: ["Site group", "Imported app", "CC-Switch endpoint"],
            rows: [
              ["OpenAI", "Codex", "<code>HOST</code>"],
              ["Anthropic / Claude", "Claude Code", "<code>HOST</code>"],
              ["Gemini", "Gemini CLI", "<code>HOST</code>"],
              [
                "Antigravity",
                "Claude Code / Gemini CLI",
                "<code>HOST/antigravity</code>",
              ],
            ],
          },
          manual: {
            title: "Manual CC-Switch Import",
            labels: ["Host", "Import type", "Provider name", "API key"],
            hostHelpHtml:
              'Default recommendation: <span data-default-host-text>https://re.94xy.cn</span>.',
            apiKeyPlaceholder: "Paste the API key copied from /keys",
            buttons: ["Import to CC-Switch", "Copy deep link"],
            options: [
              "OpenAI / Codex",
              "Claude Code",
              "Gemini CLI",
              "Antigravity -> Claude",
              "Antigravity -> Gemini",
            ],
          },
        },
      },
      overview: {
        title: "Platform Overview",
        descHtml:
          'Different clients read the host in different ways. HTTP / OpenAI SDK usually needs an explicit <code>/v1</code>; most CLI templates on this site use the root host.',
        headers: ["Platform / Client", "Protocol", "Recommended format", "Notes"],
        rows: [
          [
            "HTTP / OpenAI SDK",
            "OpenAI Chat / Responses",
            '<code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>',
            "Best default for servers and scripts",
          ],
          [
            "Claude Code",
            "Anthropic compatible",
            '<code><span data-default-host-text>https://re.94xy.cn</span></code>',
            "Use <code>ANTHROPIC_BASE_URL</code>",
          ],
          [
            "Gemini CLI",
            "Gemini CLI compatible",
            '<code><span data-default-host-text>https://re.94xy.cn</span></code>',
            "Use <code>GOOGLE_GEMINI_BASE_URL</code>",
          ],
          [
            "Codex CLI",
            "OpenAI Responses",
            '<code><span data-default-host-text>https://re.94xy.cn</span></code>',
            "Write the template generated by this site into <code>~/.codex</code>",
          ],
          [
            "OpenCode",
            "OpenAI / Anthropic / Google Provider",
            '<code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>',
            "OpenAI groups use <code>baseURL</code>",
          ],
          [
            "OpenClaw",
            "Custom Provider",
            '<code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>',
            'Write <code>models.providers</code> in <code>~/.openclaw/openclaw.json</code>',
          ],
          [
            "Hermes",
            "Custom Endpoint",
            '<code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>',
            "Current releases use <code>~/.hermes/config.yaml</code>",
          ],
        ],
      },
      hosts: {
        title: "API Host List",
        descHtml:
          'The default primary host is <code>re.94xy.cn</code>. If the admin configured a public <code>API Base URL</code> or custom endpoints, they will appear below automatically.',
        noteHtml:
          'HTTP / OpenAI SDK usually uses <code>HOST/v1</code>; for Gemini / OpenClaw / Hermes, see each section for the exact format. The <code>API Keys</code> page on this site also shows the available hosts configured by the admin.',
      },
      http: {
        title: "HTTP Examples",
        descHtml:
          'For direct scripts, backend services, or third-party gateways, prefer the OpenAI-compatible format. The examples below use <code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>.',
      },
      sdkJs: {
        title: "JavaScript SDK",
        descHtml:
          "As long as the SDK supports a custom <code>baseURL</code>, most clients work directly.",
      },
      sdkPython: {
        title: "Python SDK",
        descHtml:
          'Python works the same way as JavaScript: point <code>base_url</code> at the gateway <code>/v1</code>.',
      },
      claude: {
        title: "Claude Code",
        descHtml:
          'Claude Code uses the root host from the current site templates and does not need an extra <code>/v1</code>.',
      },
      gemini: {
        title: "Gemini CLI",
        descHtml:
          "Gemini CLI also uses the root host. Adjust the model name based on the access you currently have.",
      },
      codex: {
        title: "Codex CLI",
        descHtml:
          'The <code>Use Key</code> page on this site generates two files for Codex CLI: <code>~/.codex/config.toml</code> and <code>auth.json</code>. The format below matches that template.',
      },
      codexplusplus: {
        title: "Enable Codex Plugins (Codex++)",
        descHtml:
          "If you use a third-party API or this site's relay and do not want to rely only on the official ChatGPT login state, Codex++ can enable enhanced plugins and a relay provider for Codex App. Codex++ is an external launcher: it does not modify the Codex App installation, and injects enhancement scripts through Chromium DevTools Protocol.",
        features: {
          title: "What it does",
          items: [
            "Enables the Codex App plugin entry, special plugin installation, session deletion, Markdown export, project moving, Timeline, and other enhancements.",
            "Writes a <code>CodexPlusPlus</code> provider through relay injection, so Codex App can use a third-party API Base URL and API key.",
            "Supports multiple relay configs and switching between the enhanced provider and the official ChatGPT login mode.",
            "Manages user scripts independently and injects custom scripts when Codex starts.",
          ],
          actions: ["Open Codex++ repo", "Download Releases"],
        },
        setup: {
          title: "Third-party API setup",
          items: [
            "Install the official Codex App first, then download and install the matching Codex++ build from Releases.",
            "Open <code>/keys</code> on this site and create or copy an API key.",
            "Open <code>Codex++ Manager</code>, then go to relay injection / Relay settings.",
            "Add a config: set Base URL to <code><span data-default-host-text>https://re.94xy.cn</span>/v1</code>, paste this site's API key, and choose a model you can access, such as <code>gpt-5.4</code>.",
            "Save it, launch Codex App through <code>Codex++</code> or the manager, and confirm the current provider is <code>CodexPlusPlus</code>.",
          ],
          warningHtml:
            "The Codex++ README relay injection example uses an OpenAI Responses-compatible endpoint, so <code>HOST/v1</code> is usually the right value. If your Codex++ version explicitly asks for only the root host, follow its UI and use <code>HOST</code>.",
        },
        noteHtml:
          "If plugins or the provider do not appear after launch, run check / repair in Codex++ Manager first, then verify whether a Codex App update needs a matching Codex++ update.",
      },
      opencode: {
        title: "OpenCode",
        descHtml:
          'OpenCode is best configured through <code>opencode.json</code>. For OpenAI groups, the key fields are <code>provider.openai.options.baseURL</code> and <code>apiKey</code>.',
      },
      openclaw: {
        title: "OpenClaw",
        descHtml:
          'For OpenClaw, the current recommendation is to define a custom provider in <code>models.providers</code> inside <code>~/.openclaw/openclaw.json</code>. This page follows the format generated by the site: OpenAI groups use <code>api: "openai-responses"</code>.',
        noteHtml:
          'See the official OpenClaw provider docs as well: custom providers should live under <code>models.providers</code>, and the API type should match the backend, such as <code>openai-responses</code>.',
      },
      hermes: {
        title: "Hermes",
        descHtml:
          'Current Hermes releases use <code>~/.hermes/config.yaml</code>. The official docs already make it clear that the older <code>OPENAI_BASE_URL</code> / <code>LLM_MODEL</code> environment variables are no longer the primary source of truth.',
        interactiveTitle: "Interactive setup",
        warningHtml:
          'For Hermes, the custom endpoint should now be configured from <code>config.yaml</code>. Do not rely on the legacy <code>OPENAI_BASE_URL</code> value from older <code>.env</code> setups anymore.',
      },
      faq: {
        title: "FAQ",
        headers: ["Issue", "What to check"],
        rows: [
          [
            "CC-Switch one-click import does not open",
            'Confirm that CC-Switch is installed and the <code>ccswitch://</code> protocol is registered on the system first.',
          ],
          [
            "<code>401</code> / <code>invalid_api_key</code>",
            'Check whether the key was copied completely from <code>/keys</code>, without extra spaces, line breaks, or an older key value.',
          ],
          [
            "<code>403</code>",
            "The key may have been disabled, failed an IP restriction, or the current group may not be available.",
          ],
          [
            "<code>404</code>",
            'Most of the time the host or path is wrong. HTTP / SDK usually uses <code>/v1</code>; Claude / Gemini / Codex CLI usually use the root host.',
          ],
          [
            "Hermes / OpenClaw does not work",
            'Check the config file path first, then verify the host format for the current client. OpenClaw uses <code>openclaw.json</code>; Hermes uses <code>~/.hermes/config.yaml</code>.',
          ],
        ],
      },
      footerHtml: 'Static docs path: <code>/docs/api.html</code>',
      codeTitles: {
        httpChat: "cURL /v1/chat/completions",
        httpModels: "cURL /v1/models",
        sdkJs: "openai npm",
        sdkPython: "openai python",
        claude: "macOS / Linux",
        gemini: "macOS / Linux",
        codexConfig: "~/.codex/config.toml",
        codexAuth: "~/.codex/auth.json",
        opencode: "opencode.json",
        openclaw: "~/.openclaw/openclaw.json",
        hermes: "~/.hermes/config.yaml",
        hermesInteractive: "Interactive setup",
      },
      codeSamples: {
        httpChat: `curl https://re.94xy.cn/v1/chat/completions \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-5.4",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Please give me three integration tips"}
    ]
  }'`,
        hermesInteractive: `hermes model
# Choose "Custom endpoint (self-hosted / VLLM / etc.)"
# API base URL: https://re.94xy.cn/v1
# API key: YOUR_API_KEY
# Model name: gpt-5.4`,
      },
      hostCard: {
        fallbackDesc: "Publicly accessible gateway entry",
        labels: {
          root: "Root host",
          openai: "OpenAI SDK / HTTP",
          gemini: "Gemini / OpenClaw / Hermes",
          antigravity: "Antigravity",
        },
        builtin: {
          default: {
            name: "Default primary host",
            description: "Recommended default in this document",
          },
          current: {
            name: "Current site origin",
            description: "The address currently opened in the browser",
          },
          apiBase: {
            name: "Public API Base URL",
            description: "The public API Base URL exposed in Settings -> General",
          },
          customPrefix: "Custom endpoint",
          customDescription: "Custom host exposed by the admin",
        },
      },
    },
  };
})();
