const state = {
  adminToken: localStorage.getItem("grok_admin_token") || "",
  apiKey: localStorage.getItem("grok_api_key") || "",
  status: null,
};

const $ = (id) => document.getElementById(id);

function toast(message) {
  const el = $("toast");
  el.textContent = message;
  el.classList.add("show");
  setTimeout(() => el.classList.remove("show"), 2600);
}

function headers(admin = false) {
  const out = { "Content-Type": "application/json" };
  if (admin && state.adminToken) out["X-Admin-Token"] = state.adminToken;
  if (!admin && state.apiKey) out.Authorization = `Bearer ${state.apiKey}`;
  return out;
}

async function api(path, options = {}, admin = false) {
  const res = await fetch(path, {
    ...options,
    headers: { ...headers(admin), ...(options.headers || {}) },
  });
  const ct = res.headers.get("content-type") || "";
  const data = ct.includes("application/json") ? await res.json() : await res.text();
  if (!res.ok) {
    const msg = data?.error?.message || data?.message || data || `HTTP ${res.status}`;
    throw new Error(msg);
  }
  return data;
}

function formJSON(form) {
  const data = new FormData(form);
  const out = {};
  for (const [key, value] of data.entries()) {
    const input = form.elements[key];
    if (input?.type === "checkbox") out[key] = input.checked;
    else if (input?.type === "number") out[key] = value === "" ? 0 : Number(value);
    else out[key] = String(value).trim();
  }
  return out;
}

async function loadStatus() {
  try {
    state.status = await api("/api/admin/status", { method: "GET", headers: {} }, true);
    renderStatus();
  } catch (err) {
    toast(err.message);
  }
}

function renderStatus() {
  const accounts = state.status?.accounts || [];
  $("accountCount").textContent = `${accounts.length} 个账号`;
  $("accountsBody").innerHTML = accounts.map((a) => {
    const cooled = a.cooldown_until && new Date(a.cooldown_until) > new Date();
    const status = !a.enabled ? "停用" : cooled ? "冷却中" : "可用";
    const cls = !a.enabled ? "status-bad" : cooled ? "status-warn" : "status-ok";
    return `<tr>
      <td>${a.id}</td>
      <td>${escapeHTML(a.name || "")}</td>
      <td class="${cls}">${status}</td>
      <td>${escapeHTML(a.email || "")}</td>
      <td>${a.in_flight || 0}/${a.concurrency || 1}</td>
      <td>${a.cooldown_until ? formatTime(a.cooldown_until) : ""}</td>
      <td>${a.has_access_token ? "access" : ""} ${a.has_refresh_token ? "refresh" : ""}</td>
      <td>
        <button class="secondary" onclick="refreshAccount(${a.id})">刷新</button>
        <button class="secondary" onclick="probeAccount(${a.id})">探测</button>
        <button class="secondary" onclick="clearCooldown(${a.id})">解除冷却</button>
        <button class="danger" onclick="deleteAccount(${a.id})">删除</button>
      </td>
    </tr>`;
  }).join("");

  const keys = state.status?.api_keys || [];
  $("keysBody").innerHTML = keys.map((k) => `<tr>
    <td>${k.id}</td>
    <td>${escapeHTML(k.name || "")}</td>
    <td>${escapeHTML(k.prefix || "")}</td>
    <td>${formatTime(k.created_at)}</td>
    <td><button class="danger" onclick="deleteKey(${k.id})">删除</button></td>
  </tr>`).join("");

  const logs = state.status?.logs || [];
  $("logsBody").innerHTML = logs.map((l) => `<tr>
    <td>${formatTime(l.time)}</td>
    <td>${escapeHTML(l.endpoint || "")}</td>
    <td>${l.account_id || ""}</td>
    <td>${l.status_code || ""}</td>
    <td>${escapeHTML(l.message || "")}</td>
  </tr>`).join("");
}

async function refreshAccount(id) {
  await runButton(async () => {
    await api(`/api/admin/accounts/${id}/refresh`, { method: "POST", body: "{}" }, true);
    await loadStatus();
  });
}

async function probeAccount(id) {
  await runButton(async () => {
    const result = await api(`/api/admin/accounts/${id}/probe`, { method: "POST", body: "{}" }, true);
    toast(`探测完成：${result.status_code}`);
    await loadStatus();
  });
}

async function clearCooldown(id) {
  await api(`/api/admin/accounts/${id}`, { method: "PATCH", body: JSON.stringify({ clear_cooldown: true }) }, true);
  await loadStatus();
}

async function deleteAccount(id) {
  if (!confirm(`删除账号 ${id}？`)) return;
  await api(`/api/admin/accounts/${id}`, { method: "DELETE" }, true);
  await loadStatus();
}

async function deleteKey(id) {
  if (!confirm(`删除 API Key ${id}？`)) return;
  await api(`/api/admin/api-keys/${id}`, { method: "DELETE" }, true);
  await loadStatus();
}

async function runButton(fn) {
  const btn = event?.target;
  if (btn) btn.disabled = true;
  try {
    await fn();
  } catch (err) {
    toast(err.message);
  } finally {
    if (btn) btn.disabled = false;
  }
}

function setPre(id, value) {
  $(id).textContent = typeof value === "string" ? value : JSON.stringify(value, null, 2);
}

function extractImages(payload) {
  const found = [];
  const walk = (v) => {
    if (!v) return;
    if (typeof v === "string") {
      if (v.startsWith("http") || v.startsWith("data:image/")) found.push(v);
      return;
    }
    if (Array.isArray(v)) return v.forEach(walk);
    if (typeof v === "object") {
      if (v.url) found.push(v.url);
      if (v.b64_json) found.push(`data:image/png;base64,${v.b64_json}`);
      Object.values(v).forEach(walk);
    }
  };
  walk(payload);
  return [...new Set(found)].filter((v) => /^https?:\/\//.test(v) || v.startsWith("data:image/"));
}

function extractVideoID(payload) {
  return payload?.request_id || payload?.id || payload?.data?.request_id || payload?.data?.id || payload?.video?.request_id || payload?.video?.id || "";
}

function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

function formatTime(value) {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString();
}

document.querySelectorAll(".tab").forEach((tab) => {
  tab.addEventListener("click", () => {
    document.querySelectorAll(".tab").forEach((el) => el.classList.remove("active"));
    document.querySelectorAll(".view").forEach((el) => el.classList.remove("active"));
    tab.classList.add("active");
    $(tab.dataset.tab).classList.add("active");
  });
});

$("adminToken").value = state.adminToken;
$("apiKey").value = state.apiKey;
$("saveTokens").addEventListener("click", () => {
  state.adminToken = $("adminToken").value.trim();
  state.apiKey = $("apiKey").value.trim();
  localStorage.setItem("grok_admin_token", state.adminToken);
  localStorage.setItem("grok_api_key", state.apiKey);
  toast("已保存");
});
$("refreshStatus").addEventListener("click", loadStatus);

$("accountForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    await api("/api/admin/accounts", { method: "POST", body: JSON.stringify(formJSON(e.target)) }, true);
    e.target.reset();
    e.target.elements.base_url.value = "https://api.x.ai/v1";
    e.target.elements.concurrency.value = "1";
    await loadStatus();
  });
});

$("keyForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    const key = await api("/api/admin/api-keys", { method: "POST", body: JSON.stringify(formJSON(e.target)) }, true);
    $("newKey").textContent = `新 Key：${key.key}`;
    $("apiKey").value = key.key;
    state.apiKey = key.key;
    localStorage.setItem("grok_api_key", key.key);
    e.target.reset();
    await loadStatus();
  });
});

$("authUrlForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    const result = await api("/api/admin/grok/auth-url", { method: "POST", body: JSON.stringify(formJSON(e.target)) }, true);
    $("authResult").innerHTML = `<a href="${result.auth_url}" target="_blank" rel="noreferrer">打开 Grok 授权链接</a>\nSession ID: ${result.session_id}\nState: ${result.state}`;
    $("exchangeForm").elements.session_id.value = result.session_id;
    $("exchangeForm").elements.state.value = result.state;
  });
});

$("exchangeForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    const result = await api("/api/admin/grok/exchange-code", { method: "POST", body: JSON.stringify(formJSON(e.target)) }, true);
    setPre("oauthOutput", result);
    await loadStatus();
  });
});

$("refreshTokenForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    const result = await api("/api/admin/grok/refresh-token", { method: "POST", body: JSON.stringify(formJSON(e.target)) }, true);
    setPre("oauthOutput", result);
    await loadStatus();
  });
});

$("chatForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    const data = formJSON(e.target);
    const result = await api("/v1/chat/completions", {
      method: "POST",
      body: JSON.stringify({ model: data.model, messages: [{ role: "user", content: data.prompt }] }),
    });
    const text = result?.choices?.[0]?.message?.content || JSON.stringify(result, null, 2);
    setPre("chatOutput", text);
  });
});

$("imageForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    const data = formJSON(e.target);
    const result = await api("/v1/images/generations", {
      method: "POST",
      body: JSON.stringify({ model: data.model, prompt: data.prompt, size: data.size, n: data.n }),
    });
    setPre("imageOutput", result);
    const images = extractImages(result);
    $("imageGallery").innerHTML = images.map((src) => `<img src="${src}" alt="generated image" />`).join("");
  });
});

$("videoForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    const data = formJSON(e.target);
    const result = await api("/v1/videos/generations", {
      method: "POST",
      body: JSON.stringify({ model: data.model, prompt: data.prompt }),
    });
    setPre("videoOutput", result);
    const id = extractVideoID(result);
    if (id) $("videoStatusForm").elements.request_id.value = id;
  });
});

$("videoStatusForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  await runButton(async () => {
    const id = e.target.elements.request_id.value.trim();
    const result = await api(`/v1/videos/${encodeURIComponent(id)}`, { method: "GET", headers: { Authorization: `Bearer ${state.apiKey}` } });
    setPre("videoOutput", result);
  });
});

loadStatus();
