(function () {
  const DEFAULT_HOST = "https://re.94xy.cn";
  const DEFAULT_SITE_NAME = "Sub2API";
  const DEFAULT_SITE_SUBTITLE = "AI API Gateway Platform";
  const THEME_KEY = "theme";
  const LOCALE_KEY = "sub2api_locale";
  const AUTH_TOKEN_KEY = "auth_token";
  const AUTH_USER_KEY = "auth_user";
  const DOCS_I18N = window.DOCS_I18N || {};

  const defaultHostEls = document.querySelectorAll("[data-default-host-text]");
  const hostListEl = document.getElementById("host-list");
  const ccsHostSelect = document.getElementById("ccs-host");
  const providerNameInput = document.getElementById("ccs-provider-name");
  const apiKeyInput = document.getElementById("ccs-api-key");
  const platformSelect = document.getElementById("ccs-platform");
  const importBtn = document.getElementById("ccs-import-btn");
  const copyLinkBtn = document.getElementById("ccs-copy-btn");
  const resultEl = document.getElementById("ccs-result");
  const siteLogoEl = document.getElementById("site-logo");
  const siteNameEl = document.getElementById("site-name");
  const siteSubtitleEl = document.getElementById("site-subtitle");
  const siteHeaderNavEl = document.getElementById("site-header-nav");
  const headerLoginLinkEl = document.getElementById("header-login-link");
  const localeSwitcherEl = document.getElementById("locale-switcher");
  const themeToggleEl = document.getElementById("theme-toggle");

  let currentLocale = resolveLocale();
  let currentTheme = resolveTheme();
  let publicSettings = null;

  applyTheme(currentTheme);
  document.documentElement.setAttribute("lang", getText().lang || currentLocale);
  if (localeSwitcherEl) {
    localeSwitcherEl.value = currentLocale;
  }

  function getText() {
    return DOCS_I18N[currentLocale] || DOCS_I18N.zh || {};
  }

  function getSearchParams() {
    return new URLSearchParams(window.location.search);
  }

  function normalizeLocale(value) {
    if (typeof value !== "string") return "";
    const normalized = value.trim().toLowerCase();
    if (!normalized) return "";
    if (normalized === "zh" || normalized.startsWith("zh-")) return "zh";
    if (normalized === "en" || normalized.startsWith("en-")) return "en";
    return "";
  }

  function normalizeTheme(value) {
    return value === "dark" || value === "light" ? value : "";
  }

  function resolveLocale() {
    const queryLocale = normalizeLocale(
      getSearchParams().get("lang") || getSearchParams().get("locale"),
    );
    if (queryLocale) {
      return queryLocale;
    }

    const saved = normalizeLocale(localStorage.getItem(LOCALE_KEY));
    if (saved) {
      return saved;
    }

    const documentLang = normalizeLocale(
      document.documentElement.getAttribute("lang"),
    );
    if (documentLang) {
      return documentLang;
    }

    return navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en";
  }

  function resolveTheme() {
    const queryTheme = normalizeTheme(getSearchParams().get("theme"));
    if (queryTheme) {
      return queryTheme;
    }

    const saved = normalizeTheme(localStorage.getItem(THEME_KEY));
    if (saved) {
      return saved;
    }

    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }

  function applyTheme(theme) {
    currentTheme = theme === "dark" ? "dark" : "light";
    document.documentElement.classList.toggle("dark", currentTheme === "dark");
    localStorage.setItem(THEME_KEY, currentTheme);
    const text = getText();
    if (themeToggleEl) {
      themeToggleEl.textContent = currentTheme === "dark" ? "☀" : "☾";
      themeToggleEl.title =
        currentTheme === "dark" ? text.themeLight : text.themeDark;
      themeToggleEl.setAttribute("aria-label", themeToggleEl.title || "Theme");
    }
  }

  function readAuthUser() {
    const rawUser = localStorage.getItem(AUTH_USER_KEY);
    const token = localStorage.getItem(AUTH_TOKEN_KEY);
    if (!rawUser || !token) {
      return null;
    }
    try {
      const user = JSON.parse(rawUser);
      return {
        token,
        email: typeof user?.email === "string" ? user.email : "",
        role: typeof user?.role === "string" ? user.role : "",
      };
    } catch (error) {
      return null;
    }
  }

  function uniqueBy(items, keyFn) {
    const seen = new Set();
    return items.filter((item) => {
      const key = keyFn(item);
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }

  function stripSuffix(value) {
    return value
      .replace(/\/antigravity\/v1beta\/?$/i, "")
      .replace(/\/antigravity\/v1\/?$/i, "")
      .replace(/\/antigravity\/?$/i, "")
      .replace(/\/v1beta\/?$/i, "")
      .replace(/\/v1\/?$/i, "")
      .replace(/\/+$/g, "");
  }

  function resolveUrl(value) {
    if (!value) return "";
    const trimmed = String(value).trim();
    if (!trimmed) return "";
    try {
      if (/^https?:\/\//i.test(trimmed)) {
        return stripSuffix(new URL(trimmed).toString());
      }
      if (trimmed.startsWith("/")) {
        return stripSuffix(new URL(trimmed, window.location.origin).toString());
      }
      return stripSuffix(new URL(`https://${trimmed}`).toString());
    } catch (error) {
      return "";
    }
  }

  function joinUrl(root, suffix) {
    return `${root.replace(/\/+$/g, "")}${suffix}`;
  }

  function buildUsageScript() {
    return `({
  request: {
    url: "{{baseUrl}}/v1/usage",
    method: "GET",
    headers: { "Authorization": "Bearer {{apiKey}}" }
  },
  extractor: function(response) {
    const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
    const unit = response?.unit ?? response?.quota?.unit ?? "USD";
    return {
      isValid: response?.is_active ?? response?.isValid ?? true,
      remaining,
      unit
    };
  }
})`;
  }

  function resolveCcSwitchConfig(platform, baseUrl) {
    const root = baseUrl.replace(/\/+$/g, "");
    switch (platform) {
      case "openai":
        return { app: "codex", endpoint: root, model: "gpt-5.4" };
      case "gemini":
        return { app: "gemini", endpoint: root };
      case "antigravity-claude":
        return { app: "claude", endpoint: `${root}/antigravity` };
      case "antigravity-gemini":
        return { app: "gemini", endpoint: `${root}/antigravity` };
      default:
        return { app: "claude", endpoint: root };
    }
  }

  function buildCcSwitchLink() {
    const text = getText();
    const baseUrl = ccsHostSelect.value || DEFAULT_HOST;
    const apiKey = apiKeyInput.value.trim();
    const providerName = providerNameInput.value.trim() || DEFAULT_SITE_NAME;
    const platform = platformSelect.value;

    if (!apiKey) {
      throw new Error(text.messages?.pasteApiKeyFirst || "Paste API key first.");
    }

    const config = resolveCcSwitchConfig(platform, baseUrl);
    const params = new URLSearchParams();
    params.set("resource", "provider");
    params.set("app", config.app);
    if (config.model) {
      params.set("model", config.model);
    }
    params.set("name", providerName);
    params.set("homepage", baseUrl);
    params.set("endpoint", config.endpoint);
    params.set("apiKey", apiKey);
    params.set("configFormat", "json");
    params.set("usageEnabled", "true");
    params.set("usageScript", btoa(buildUsageScript()));
    params.set("usageAutoInterval", "30");
    return `ccswitch://v1/import?${params.toString()}`;
  }

  function normalizePlacement(value) {
    return value === "home_header" || value === "both" ? value : "sidebar";
  }

  function normalizeLinkOpenMode(value) {
    return value === "external" ? "external" : "internal";
  }

  function isCustomPageMenu(item) {
    return Boolean(item?.page_slug) || String(item?.url || "").trim().startsWith("md:");
  }

  function escapeAttr(value) {
    return String(value || "")
      .replace(/&/g, "&amp;")
      .replace(/"/g, "&quot;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }

  function escapeHtml(value) {
    return String(value || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }

  function buildCustomMenuHref(item) {
    if (!isCustomPageMenu(item)) {
      const rawUrl = String(item?.url || "").trim();
      if (rawUrl) {
        return rawUrl;
      }
    }
    return `/custom/${encodeURIComponent(item.id)}`;
  }

  function buildCustomMenuTargetAttrs(item) {
    if (normalizeLinkOpenMode(item?.link_open_mode) === "external") {
      return ' target="_blank" rel="noopener noreferrer"';
    }
    return "";
  }

  function hostCardTemplate(item) {
    const text = getText();
    const labels = text.hostCard?.labels || {};
    const root = item.root;
    const openaiBase = joinUrl(root, "/v1");
    const geminiBase = joinUrl(root, "/v1beta");
    const antigravityBase = joinUrl(root, "/antigravity");
    const antigravityGeminiBase = joinUrl(root, "/antigravity/v1beta");

    return `
      <article class="host-card">
        <h3>${item.name}</h3>
        <p>${item.description || text.hostCard?.fallbackDesc || ""}</p>
        <div class="host-grid">
          <div class="host-item">
            <strong>${labels.root || "Root host"}</strong>
            <code>${root}</code>
          </div>
          <div class="host-item">
            <strong>${labels.openai || "OpenAI SDK / HTTP"}</strong>
            <code>${openaiBase}</code>
          </div>
          <div class="host-item">
            <strong>${labels.gemini || "Gemini / OpenClaw / Hermes"}</strong>
            <code>${geminiBase}</code>
          </div>
          <div class="host-item">
            <strong>${labels.antigravity || "Antigravity"}</strong>
            <code>${antigravityBase}</code><br />
            <code>${antigravityGeminiBase}</code>
          </div>
        </div>
        <div class="host-actions">
          <button class="tiny-btn" type="button" data-copy-text="${root}">${text.messages?.copyRootHost || "Copy root host"}</button>
          <button class="tiny-btn" type="button" data-copy-text="${openaiBase}">${text.messages?.copyV1 || "Copy /v1"}</button>
        </div>
      </article>
    `;
  }

  function renderHosts(items) {
    hostListEl.innerHTML = items.map(hostCardTemplate).join("");
    ccsHostSelect.innerHTML = items
      .map((item) => `<option value="${item.root}">${item.name} - ${item.root}</option>`)
      .join("");
  }

  async function copyText(text, trigger) {
    const i18n = getText();
    try {
      await navigator.clipboard.writeText(text);
      if (trigger) {
        const prev = trigger.textContent;
        trigger.textContent = i18n.messages?.copied || "Copied";
        setTimeout(() => {
          trigger.textContent = prev;
        }, 1200);
      }
    } catch (error) {
      if (resultEl) {
        resultEl.textContent =
          i18n.messages?.copyFailed || "Copy failed. Please copy it manually.";
      }
    }
  }

  function renderHeaderMenus(settings) {
    if (!siteHeaderNavEl) return;
    const items = Array.isArray(settings?.custom_menu_items)
      ? settings.custom_menu_items
      : [];
    const visible = items
      .filter((item) => item && item.visibility === "user")
      .filter((item) => {
        const placement = normalizePlacement(item.placement);
        return placement === "home_header" || placement === "both";
      })
      .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0));

    siteHeaderNavEl.innerHTML = visible
      .map((item) => {
        const icon = item.icon_svg
          ? `<span style="display:inline-flex;width:16px;height:16px;align-items:center;justify-content:center;">${item.icon_svg}</span>`
          : "";
        return `<a class="header-link" href="${escapeAttr(buildCustomMenuHref(item))}"${buildCustomMenuTargetAttrs(item)}>${icon}<span>${escapeHtml(item.label || "")}</span></a>`;
      })
      .join("");
  }

  function updateHeader(settings) {
    const text = getText();
    const auth = readAuthUser();
    const siteName =
      typeof settings?.site_name === "string" && settings.site_name.trim()
        ? settings.site_name.trim()
        : DEFAULT_SITE_NAME;
    const siteSubtitle =
      typeof settings?.site_subtitle === "string" && settings.site_subtitle.trim()
        ? settings.site_subtitle.trim()
        : DEFAULT_SITE_SUBTITLE;
    const siteLogo =
      typeof settings?.site_logo === "string" && settings.site_logo.trim()
        ? settings.site_logo.trim()
        : "/logo.png";

    if (siteLogoEl) {
      siteLogoEl.src = siteLogo;
    }
    if (siteNameEl) {
      siteNameEl.textContent = siteName;
    }
    if (siteSubtitleEl) {
      siteSubtitleEl.textContent = siteSubtitle;
    }
    document.title = `${siteName} - ${text.titleSuffix || ""}`;

    if (headerLoginLinkEl) {
      if (auth) {
        headerLoginLinkEl.href =
          auth.role === "admin" || auth.role === "useradmin"
            ? "/admin/dashboard"
            : "/dashboard";
        headerLoginLinkEl.querySelector("[data-i18n]").textContent =
          text.dashboard || "Dashboard";
      } else {
        headerLoginLinkEl.href = "/login";
        headerLoginLinkEl.querySelector("[data-i18n]").textContent =
          text.goLogin || "Login";
      }
      headerLoginLinkEl.title = text.goLogin || "Login";
    }
    renderHeaderMenus(settings);
  }

  function replaceNodeContent(selector, content, mode) {
    const el = document.querySelector(selector);
    if (!el) return;
    if (mode === "html") {
      el.innerHTML = content;
      return;
    }
    el.textContent = content;
  }

  function replaceAllText(selector, values) {
    const nodes = document.querySelectorAll(selector);
    nodes.forEach((node, index) => {
      if (values[index] != null) {
        node.textContent = values[index];
      }
    });
  }

  function replaceAllHtml(selector, values) {
    const nodes = document.querySelectorAll(selector);
    nodes.forEach((node, index) => {
      if (values[index] != null) {
        node.innerHTML = values[index];
      }
    });
  }

  function renderStaticContent() {
    const text = getText();
    document.documentElement.setAttribute("lang", text.lang || currentLocale);
    if (localeSwitcherEl) {
      localeSwitcherEl.setAttribute("aria-label", text.localeLabel || "Language");
    }

    replaceNodeContent(".eyebrow-row .pill.brand", text.hero?.priority || "", "text");
    replaceNodeContent(
      ".eyebrow-row .pill:nth-child(2)",
      `${text.hero?.defaultHostLabel || ""}<span data-default-host-text>${DEFAULT_HOST}</span>`,
      "html",
    );
    replaceNodeContent(
      ".eyebrow-row .pill:nth-child(3)",
      `${text.hero?.docPathLabel || ""}<code>/docs/api.html</code>`,
      "html",
    );
    replaceNodeContent(".hero h1", text.hero?.title || "", "text");
    replaceNodeContent(".hero-copy", text.hero?.copyHtml || "", "html");
    replaceAllText(".hero-actions a", text.hero?.actions || []);
    replaceAllText(".hero-stat .label", text.hero?.statLabels || []);
    replaceNodeContent(
      ".hero-stat:nth-child(2) .value",
      text.hero?.keyPathHtml || "",
      "html",
    );
    replaceNodeContent(
      ".hero-stat:nth-child(3) .value",
      text.hero?.placement || "",
      "text",
    );
    replaceNodeContent(
      ".hero-stat:nth-child(4) .value",
      text.hero?.protocols || "",
      "text",
    );

    replaceNodeContent(".sidebar h2", text.sidebar?.title || "", "text");
    replaceAllText(".sidebar a", text.sidebar?.links || []);

    replaceNodeContent("#quickstart h2", text.quickstart?.title || "", "text");
    replaceNodeContent("#quickstart .section-desc", text.quickstart?.descHtml || "", "html");
    replaceAllText("#quickstart .card h3", (text.quickstart?.cards || []).map((item) => item.title));
    replaceAllHtml("#quickstart .card p", (text.quickstart?.cards || []).map((item) => item.bodyHtml));
    replaceNodeContent("#quickstart .note", text.quickstart?.noteHtml || "", "html");

    replaceNodeContent("#ccswitch h2", text.ccswitch?.title || "", "text");
    replaceNodeContent("#ccswitch > .section-desc", text.ccswitch?.descHtml || "", "html");
    replaceAllText("#ccswitch .grid-2 .card h3", [
      text.ccswitch?.cards?.download?.title || "",
      text.ccswitch?.cards?.oneClick?.title || "",
      text.ccswitch?.cards?.importMap?.title || "",
      text.ccswitch?.cards?.manual?.title || "",
    ]);
    replaceAllHtml(
      "#ccswitch .grid-2:nth-of-type(1) .card:nth-child(1) .plain-list li",
      text.ccswitch?.cards?.download?.items || [],
    );
    replaceAllText(
      "#ccswitch .grid-2:nth-of-type(1) .card:nth-child(1) .meta-row a",
      text.ccswitch?.cards?.download?.actions || [],
    );
    replaceAllHtml(
      "#ccswitch .grid-2:nth-of-type(1) .card:nth-child(2) .plain-list li",
      text.ccswitch?.cards?.oneClick?.items || [],
    );
    replaceNodeContent(
      "#ccswitch .grid-2:nth-of-type(1) .card:nth-child(2) .note",
      text.ccswitch?.cards?.oneClick?.noteHtml || "",
      "html",
    );
    replaceAllText(
      "#ccswitch .grid-2:nth-of-type(2) .card:nth-child(1) th",
      text.ccswitch?.cards?.importMap?.headers || [],
    );
    const importRows = document.querySelectorAll(
      "#ccswitch .grid-2:nth-of-type(2) .card:nth-child(1) tbody tr",
    );
    importRows.forEach((row, rowIndex) => {
      const cells = row.querySelectorAll("td");
      const values = text.ccswitch?.cards?.importMap?.rows?.[rowIndex] || [];
      cells.forEach((cell, colIndex) => {
        if (values[colIndex] != null) {
          cell.innerHTML = values[colIndex];
        }
      });
    });
    replaceAllText(
      "#ccswitch .grid-2:nth-of-type(2) .card:nth-child(2) label",
      text.ccswitch?.cards?.manual?.labels || [],
    );
    replaceNodeContent(
      "#ccswitch .grid-2:nth-of-type(2) .card:nth-child(2) small",
      text.ccswitch?.cards?.manual?.hostHelpHtml || "",
      "html",
    );
    replaceAllText(
      "#ccswitch .grid-2:nth-of-type(2) .card:nth-child(2) .host-actions button",
      text.ccswitch?.cards?.manual?.buttons || [],
    );
    if (apiKeyInput) {
      apiKeyInput.placeholder = text.ccswitch?.cards?.manual?.apiKeyPlaceholder || "";
    }
    if (platformSelect) {
      Array.from(platformSelect.options).forEach((option, index) => {
        if (text.ccswitch?.cards?.manual?.options?.[index]) {
          option.textContent = text.ccswitch.cards.manual.options[index];
        }
      });
    }

    replaceNodeContent("#overview h2", text.overview?.title || "", "text");
    replaceNodeContent("#overview .section-desc", text.overview?.descHtml || "", "html");
    replaceAllText("#overview th", text.overview?.headers || []);
    const overviewRows = document.querySelectorAll("#overview tbody tr");
    overviewRows.forEach((row, rowIndex) => {
      const cells = row.querySelectorAll("td");
      const values = text.overview?.rows?.[rowIndex] || [];
      cells.forEach((cell, colIndex) => {
        if (values[colIndex] != null) {
          cell.innerHTML = values[colIndex];
        }
      });
    });

    replaceNodeContent("#hosts h2", text.hosts?.title || "", "text");
    replaceNodeContent("#hosts .section-desc", text.hosts?.descHtml || "", "html");
    replaceNodeContent("#hosts .note", text.hosts?.noteHtml || "", "html");

    replaceNodeContent("#http h2", text.http?.title || "", "text");
    replaceNodeContent("#http .section-desc", text.http?.descHtml || "", "html");
    replaceNodeContent("#sdk-js h2", text.sdkJs?.title || "", "text");
    replaceNodeContent("#sdk-js .section-desc", text.sdkJs?.descHtml || "", "html");
    replaceNodeContent("#sdk-python h2", text.sdkPython?.title || "", "text");
    replaceNodeContent("#sdk-python .section-desc", text.sdkPython?.descHtml || "", "html");
    replaceNodeContent("#claude-code h2", text.claude?.title || "", "text");
    replaceNodeContent("#claude-code .section-desc", text.claude?.descHtml || "", "html");
    replaceNodeContent("#gemini-cli h2", text.gemini?.title || "", "text");
    replaceNodeContent("#gemini-cli .section-desc", text.gemini?.descHtml || "", "html");
    replaceNodeContent("#codex-cli h2", text.codex?.title || "", "text");
    replaceNodeContent("#codex-cli .section-desc", text.codex?.descHtml || "", "html");
    replaceNodeContent("#codexplusplus h2", text.codexplusplus?.title || "", "text");
    replaceNodeContent("#codexplusplus .section-desc", text.codexplusplus?.descHtml || "", "html");
    replaceAllText(
      "#codexplusplus .grid-2 .card h3",
      [
        text.codexplusplus?.features?.title || "",
        text.codexplusplus?.setup?.title || "",
      ],
    );
    replaceAllHtml(
      "#codexplusplus .grid-2 .card:nth-child(1) .plain-list li",
      text.codexplusplus?.features?.items || [],
    );
    replaceAllText(
      "#codexplusplus .grid-2 .card:nth-child(1) .meta-row a",
      text.codexplusplus?.features?.actions || [],
    );
    replaceAllHtml(
      "#codexplusplus .grid-2 .card:nth-child(2) .plain-list li",
      text.codexplusplus?.setup?.items || [],
    );
    replaceNodeContent(
      "#codexplusplus .grid-2 .card:nth-child(2) .note",
      text.codexplusplus?.setup?.warningHtml || "",
      "html",
    );
    replaceNodeContent(
      "#codexplusplus > .note",
      text.codexplusplus?.noteHtml || "",
      "html",
    );
    replaceNodeContent("#opencode h2", text.opencode?.title || "", "text");
    replaceNodeContent("#opencode .section-desc", text.opencode?.descHtml || "", "html");
    replaceNodeContent("#openclaw h2", text.openclaw?.title || "", "text");
    replaceNodeContent("#openclaw .section-desc", text.openclaw?.descHtml || "", "html");
    replaceNodeContent("#openclaw .note", text.openclaw?.noteHtml || "", "html");
    replaceNodeContent("#hermes h2", text.hermes?.title || "", "text");
    replaceNodeContent("#hermes .section-desc", text.hermes?.descHtml || "", "html");
    replaceNodeContent(
      "#hermes .grid-2 .code-block:nth-child(2) .code-title",
      text.hermes?.interactiveTitle || "",
      "text",
    );
    replaceNodeContent("#hermes .note", text.hermes?.warningHtml || "", "html");
    replaceNodeContent("#faq h2", text.faq?.title || "", "text");
    replaceAllText("#faq th", text.faq?.headers || []);
    const faqRows = document.querySelectorAll("#faq tbody tr");
    faqRows.forEach((row, rowIndex) => {
      const cells = row.querySelectorAll("td");
      const values = text.faq?.rows?.[rowIndex] || [];
      cells.forEach((cell, colIndex) => {
        if (values[colIndex] != null) {
          cell.innerHTML = values[colIndex];
        }
      });
    });

    replaceNodeContent(".footer", text.footerHtml || "", "html");

    const copyButtons = document.querySelectorAll(".copy-btn");
    copyButtons.forEach((button) => {
      button.textContent = text.messages?.copy || "Copy";
    });
    replaceAllText(".code-title", [
      text.codeTitles?.httpChat || "",
      text.codeTitles?.httpModels || "",
      text.codeTitles?.sdkJs || "",
      text.codeTitles?.sdkPython || "",
      text.codeTitles?.claude || "",
      text.codeTitles?.gemini || "",
      text.codeTitles?.codexConfig || "",
      text.codeTitles?.codexAuth || "",
      text.codeTitles?.opencode || "",
      text.codeTitles?.openclaw || "",
      text.codeTitles?.hermes || "",
      text.codeTitles?.hermesInteractive || "",
    ]);

    const codeHttpChat = document.getElementById("code-http-chat");
    const codeHermesInteractive = document.getElementById("code-hermes-interactive");
    if (codeHttpChat && text.codeSamples?.httpChat) {
      codeHttpChat.textContent = text.codeSamples.httpChat;
    }
    if (codeHermesInteractive && text.codeSamples?.hermesInteractive) {
      codeHermesInteractive.textContent = text.codeSamples.hermesInteractive;
    }

    defaultHostEls.forEach((el) => {
      el.textContent = DEFAULT_HOST;
    });
  }

  function syncState(options) {
    const previousLocale = currentLocale;
    currentLocale = resolveLocale();
    currentTheme = resolveTheme();

    if (localeSwitcherEl) {
      localeSwitcherEl.value = currentLocale;
    }

    applyTheme(currentTheme);
    renderStaticContent();
    updateHeader(publicSettings);

    if (options?.reloadHosts || previousLocale !== currentLocale) {
      loadPublicSettings();
    }
  }

  async function loadPublicSettings() {
    const text = getText();
    const builtins = text.hostCard?.builtin || {};
    const seeds = [
      {
        name: builtins.default?.name || "Default primary host",
        root: resolveUrl(DEFAULT_HOST),
        description: builtins.default?.description || "",
      },
    ];

    const currentRoot = resolveUrl(window.location.origin);
    if (currentRoot && currentRoot !== resolveUrl(DEFAULT_HOST)) {
      seeds.push({
        name: builtins.current?.name || "Current site origin",
        root: currentRoot,
        description: builtins.current?.description || "",
      });
    }

    try {
      const response = await fetch("/api/v1/settings/public", {
        credentials: "same-origin",
      });
      const payload = await response.json();
      const settings = payload && payload.data ? payload.data : payload;
      publicSettings = settings;
      updateHeader(settings);

      if (
        settings &&
        typeof settings.site_name === "string" &&
        settings.site_name.trim()
      ) {
        providerNameInput.value = settings.site_name.trim();
      }

      if (settings && settings.api_base_url) {
        const apiRoot = resolveUrl(settings.api_base_url);
        if (apiRoot) {
          seeds.push({
            name: builtins.apiBase?.name || "Public API Base URL",
            root: apiRoot,
            description: builtins.apiBase?.description || "",
          });
        }
      }

      if (settings && Array.isArray(settings.custom_endpoints)) {
        settings.custom_endpoints.forEach((item, index) => {
          const root = resolveUrl(item.endpoint);
          if (!root) return;
          seeds.push({
            name:
              item.name ||
              `${builtins.customPrefix || "Custom endpoint"} ${index + 1}`,
            root: root,
            description:
              item.description || builtins.customDescription || "",
          });
        });
      }
    } catch (error) {
      updateHeader(publicSettings);
    }

    const deduped = uniqueBy(
      seeds.filter((item) => item.root),
      (item) => item.root,
    );

    renderHosts(deduped);
  }

  function initHeaderEvents() {
    if (themeToggleEl) {
      themeToggleEl.addEventListener("click", () => {
        applyTheme(currentTheme === "dark" ? "light" : "dark");
      });
    }
    if (localeSwitcherEl) {
      localeSwitcherEl.addEventListener("change", (event) => {
        const next = event.target.value === "zh" ? "zh" : "en";
        currentLocale = next;
        localStorage.setItem(LOCALE_KEY, next);
        syncState({ reloadHosts: true });
      });
    }
  }

  function initCrossContextSync() {
    window.addEventListener("storage", (event) => {
      if (
        event.key &&
        event.key !== LOCALE_KEY &&
        event.key !== THEME_KEY &&
        event.key !== AUTH_TOKEN_KEY &&
        event.key !== AUTH_USER_KEY
      ) {
        return;
      }

      syncState({
        reloadHosts:
          event.key === null ||
          event.key === LOCALE_KEY ||
          event.key === AUTH_TOKEN_KEY ||
          event.key === AUTH_USER_KEY,
      });
    });
  }

  document.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof HTMLElement)) return;

    const copyTargetId = target.getAttribute("data-copy-target");
    if (copyTargetId) {
      const codeEl = document.getElementById(copyTargetId);
      if (codeEl) {
        copyText(codeEl.textContent || "", target);
      }
      return;
    }

    const copyTextValue = target.getAttribute("data-copy-text");
    if (copyTextValue) {
      copyText(copyTextValue, target);
    }
  });

  if (importBtn) {
    importBtn.addEventListener("click", () => {
      const text = getText();
      try {
        const link = buildCcSwitchLink();
        resultEl.textContent = text.messages?.tryingOpen || "";
        window.open(link, "_self");
        setTimeout(() => {
          if (document.hasFocus()) {
            resultEl.textContent = text.messages?.clientNotOpened || "";
          }
        }, 120);
      } catch (error) {
        resultEl.textContent = error.message || text.messages?.importFailed || "";
      }
    });
  }

  if (copyLinkBtn) {
    copyLinkBtn.addEventListener("click", async () => {
      const text = getText();
      try {
        const link = buildCcSwitchLink();
        await copyText(link, copyLinkBtn);
        resultEl.textContent = text.messages?.deepLinkCopied || "";
      } catch (error) {
        resultEl.textContent = error.message || text.messages?.copyFailed || "";
      }
    });
  }

  renderStaticContent();
  updateHeader(publicSettings);
  initHeaderEvents();
  initCrossContextSync();
  loadPublicSettings();
})();
