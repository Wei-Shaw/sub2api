const params = new URLSearchParams(location.search);
const embeddedToken = params.get('token') || '';
const theme = params.get('theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
document.documentElement.dataset.theme = theme;
document.body.dataset.uiMode = params.get('ui_mode') || 'standalone';

const settingsKey = 'CozyToken_static_online_chat_settings';
const sessionsKey = 'CozyToken_static_online_chat_sessions';
const defaultSettings = {
  apiKey: embeddedToken,
  model: ''
};

let resolvedAPIEndpoint = '/v1';
let settings = sanitizeSettings(loadJson(settingsKey, defaultSettings));
if (!settings.apiKey && embeddedToken) settings.apiKey = embeddedToken;
let draftSettings = { ...settings };
let sessions = loadJson(sessionsKey, []);
let activeSessionId = sessions[0]?.id || '';
let models = [];
let busy = false;
let attachments = [];

const el = {
  sessionList: document.getElementById('sessionList'),
  main: document.querySelector('.main'),
  emptyHero: document.getElementById('emptyHero'),
  messages: document.getElementById('messages'),
  modelPill: document.getElementById('modelPill'),
  prompt: document.getElementById('prompt'),
  mode: document.getElementById('mode'),
  sendBtn: document.getElementById('sendBtn'),
  addBtn: document.getElementById('addBtn'),
  imageInput: document.getElementById('imageInput'),
  attachmentsBar: document.getElementById('attachmentsBar'),
  attachmentHint: document.getElementById('attachmentHint'),
  newSessionBtn: document.getElementById('newSessionBtn'),
  settingsBtn: document.getElementById('settingsBtn'),
  newWindowBtn: document.getElementById('newWindowBtn'),
  modal: document.getElementById('settingsModal'),
  closeSettingsBtn: document.getElementById('closeSettingsBtn'),
  cancelSettingsBtn: document.getElementById('cancelSettingsBtn'),
  saveSettingsBtn: document.getElementById('saveSettingsBtn'),
  loadModelsBtn: document.getElementById('loadModelsBtn'),
  testModelBtn: document.getElementById('testModelBtn'),
  apiKeyInput: document.getElementById('apiKeyInput'),
  apiEndpointInput: document.getElementById('apiEndpointInput'),
  modelSelect: document.getElementById('modelSelect'),
  modeMenu: null,
  notice: document.getElementById('notice')
};

if (sessions.length === 0) createSession();
bindEvents();
loadPublicSettings();
render();

function bindEvents() {
  setupModeMenu();
  el.newSessionBtn.addEventListener('click', () => { createSession(); render(); });
  el.settingsBtn.addEventListener('click', openSettings);
  el.newWindowBtn.addEventListener('click', () => window.open(location.href, '_blank', 'noopener,noreferrer'));
  el.closeSettingsBtn.addEventListener('click', closeSettings);
  el.cancelSettingsBtn.addEventListener('click', closeSettings);
  el.saveSettingsBtn.addEventListener('click', saveSettings);
  el.loadModelsBtn.addEventListener('click', loadModels);
  el.testModelBtn.addEventListener('click', testModel);
  el.sendBtn.addEventListener('click', send);
  el.addBtn.addEventListener('click', (event) => {
    event.stopPropagation();
    el.modeMenu?.classList.toggle('open');
  });
  document.addEventListener('click', () => {
    el.modeMenu?.classList.remove('open');
    closeSessionMenus();
  });
  el.imageInput.addEventListener('change', () => addImageFiles([...el.imageInput.files]));
  el.prompt.addEventListener('paste', handlePaste);
  el.prompt.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      send();
    }
  });
}

function setupModeMenu() {
  if (el.modeMenu) return;
  const wrap = document.createElement('div');
  wrap.className = 'mode-menu-wrap';
  el.addBtn.parentNode.insertBefore(wrap, el.addBtn);
  wrap.appendChild(el.addBtn);

  const menu = document.createElement('div');
  menu.className = 'mode-menu';
  const upload = document.createElement('button');
  upload.type = 'button';
  upload.className = 'mode-option';
  upload.textContent = '上传图片';
  upload.addEventListener('click', (event) => {
    event.stopPropagation();
    menu.classList.remove('open');
    el.imageInput.click();
  });
  menu.appendChild(upload);
  const labels = { chat: '对话', image: '生图', video: '生视频' };
  Object.entries(labels).forEach(([mode, label]) => {
    const option = document.createElement('button');
    option.type = 'button';
    option.className = 'mode-option' + (el.mode.value === mode ? ' active' : '');
    option.dataset.mode = mode;
    option.textContent = label;
    option.addEventListener('click', (event) => {
      event.stopPropagation();
      el.mode.value = mode;
      menu.querySelectorAll('.mode-option').forEach(item => {
        item.classList.toggle('active', item.dataset.mode === mode);
      });
      menu.classList.remove('open');
      renderAttachments();
      el.prompt.focus();
    });
    menu.appendChild(option);
  });
  wrap.appendChild(menu);
  el.modeMenu = menu;
}

async function addImageFiles(files) {
  const imageFiles = files.filter(file => file && file.type?.startsWith('image/'));
  if (!imageFiles.length) return;
  const next = await Promise.all(imageFiles.map(fileToAttachment));
  attachments = [...attachments, ...next].slice(0, 6);
  el.imageInput.value = '';
  renderAttachments();
}

function fileToAttachment(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve({
      id: makeId(),
      name: file.name || 'pasted-image',
      type: file.type || 'image/png',
      size: file.size || 0,
      dataUrl: String(reader.result || '')
    });
    reader.onerror = () => reject(reader.error || new Error('read image failed'));
    reader.readAsDataURL(file);
  });
}

function handlePaste(event) {
  const files = [...(event.clipboardData?.files || [])].filter(file => file.type?.startsWith('image/'));
  if (!files.length) return;
  event.preventDefault();
  addImageFiles(files);
}

function renderAttachments() {
  el.attachmentsBar.innerHTML = '';
  el.attachmentsBar.classList.toggle('show', attachments.length > 0);
  attachments.forEach(item => {
    const chip = document.createElement('div');
    chip.className = 'attachment-chip';
    chip.innerHTML = '<img alt=""><span></span><button type="button">×</button>';
    chip.querySelector('img').src = item.dataUrl;
    chip.querySelector('span').textContent = `${item.name} · ${formatBytes(item.size)}`;
    chip.querySelector('button').addEventListener('click', () => {
      attachments = attachments.filter(file => file.id !== item.id);
      renderAttachments();
    });
    el.attachmentsBar.appendChild(chip);
  });

  const mode = el.mode.value;
  let hint = '';
  if (attachments.length && mode === 'chat') {
    hint = '已附加图片：将按 OpenAI Vision 格式发送。请确认当前模型支持图片理解。';
  } else if (attachments.length && mode === 'image') {
    hint = '当前是生图模式：部分模型不支持参考图输入，若失败请切换支持图片编辑/参考图的模型。';
  } else if (attachments.length && mode === 'video') {
    hint = '当前是生视频模式：部分模型不支持图片参考，请以接口返回为准。';
  }
  el.attachmentHint.textContent = hint;
  el.attachmentHint.classList.toggle('show', Boolean(hint));
}

function formatBytes(size) {
  if (!size) return '0 B';
  const units = ['B', 'KB', 'MB'];
  let value = size;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(index ? 1 : 0)} ${units[index]}`;
}

function loadJson(key, fallback) {
  try {
    const value = JSON.parse(localStorage.getItem(key) || '');
    return value || fallback;
  } catch {
    return fallback;
  }
}

function saveJson(key, value) {
  localStorage.setItem(key, JSON.stringify(value));
}

function makeId() {
  const randomUUID = globalThis.crypto?.randomUUID;
  if (typeof randomUUID === 'function') {
    return randomUUID.call(globalThis.crypto);
  }
  const randomValues = globalThis.crypto?.getRandomValues;
  if (typeof randomValues === 'function') {
    const bytes = new Uint8Array(16);
    randomValues.call(globalThis.crypto, bytes);
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    const hex = [...bytes].map(value => value.toString(16).padStart(2, '0'));
    return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`;
  }
  return `id-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

function sanitizeSettings(value) {
  return {
    apiKey: value?.apiKey || embeddedToken || '',
    model: value?.model || ''
  };
}

async function loadPublicSettings() {
  const injected = window.__APP_CONFIG__;
  if (injected?.api_base_url) {
    applyAPIBaseURL(injected.api_base_url);
    return;
  }
  try {
    const res = await fetch('/api/v1/settings/public', { credentials: 'same-origin' });
    if (!res.ok) return;
    const payload = await res.json();
    const data = payload?.data || payload;
    if (data?.api_base_url) applyAPIBaseURL(data.api_base_url);
  } catch {
    applyAPIBaseURL('');
  }
}

function applyAPIBaseURL(value) {
  resolvedAPIEndpoint = resolveSameOriginEndpoint(value);
  if (el.apiEndpointInput) {
    el.apiEndpointInput.value = resolvedAPIEndpoint;
    el.apiEndpointInput.readOnly = true;
  }
}

function resolveSameOriginEndpoint(value) {
  const fallback = '/v1';
  const raw = String(value || '').trim();
  if (!raw) return fallback;
  try {
    const url = new URL(raw, location.origin);
    if (url.origin !== location.origin) {
      return fallback;
    }
    const path = url.pathname.replace(/\/+$/, '');
    return path || fallback;
  } catch {
    return raw.startsWith('/') ? raw.replace(/\/+$/, '') || fallback : fallback;
  }
}

function createSession() {
  const session = {
    id: makeId(),
    title: '新对话',
    updatedAt: Date.now(),
    messages: []
  };
  sessions.unshift(session);
  activeSessionId = session.id;
  persistSessions();
  return session;
}

function activeSession() {
  return sessions.find(item => item.id === activeSessionId) || sessions[0] || createSession();
}

function persistSessions() {
  saveJson(sessionsKey, sessions);
}

function render() {
  const session = activeSession();
  el.main.classList.toggle('empty-state', session.messages.length === 0);
  el.modelPill.textContent = settings.model || '未选择模型';
  renderSessions();
  renderMessages();
}

function renderSessions() {
  el.sessionList.innerHTML = '';
  sessions.forEach(session => {
    const item = document.createElement('div');
    item.className = 'session-item' + (session.id === activeSessionId ? ' active' : '');
    item.innerHTML = `
      <button class="session-main" type="button">
        <span class="session-name"></span>
        <span class="session-meta"></span>
      </button>
      <button class="session-more" type="button" title="更多">⋯</button>
      <div class="session-menu">
        <button class="rename-session" type="button">重命名</button>
        <button class="delete-session danger" type="button">删除</button>
      </div>
    `;
    item.querySelector('.session-name').textContent = session.title;
    item.querySelector('.session-meta').textContent = new Date(session.updatedAt).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
    item.querySelector('.session-main').addEventListener('click', () => {
      activeSessionId = session.id;
      render();
    });
    const menu = item.querySelector('.session-menu');
    item.querySelector('.session-more').addEventListener('click', (event) => {
      event.stopPropagation();
      closeSessionMenus(menu);
      menu.classList.toggle('open');
    });
    item.querySelector('.rename-session').addEventListener('click', (event) => {
      event.stopPropagation();
      renameSession(session.id);
    });
    item.querySelector('.delete-session').addEventListener('click', (event) => {
      event.stopPropagation();
      deleteSession(session.id);
    });
    el.sessionList.appendChild(item);
  });
}

function closeSessionMenus(except) {
  el.sessionList.querySelectorAll('.session-menu.open').forEach(menu => {
    if (menu !== except) menu.classList.remove('open');
  });
}

function renameSession(id) {
  const session = sessions.find(item => item.id === id);
  if (!session) return;
  const title = prompt('重命名会话', session.title || '');
  if (title === null) return;
  const next = title.trim();
  if (!next) return;
  session.title = next.slice(0, 80);
  session.updatedAt = Date.now();
  persistSessions();
  render();
}

function deleteSession(id) {
  if (sessions.length <= 1) {
    sessions = [];
    createSession();
    render();
    return;
  }
  if (!confirm('删除这个会话？')) return;
  const index = sessions.findIndex(item => item.id === id);
  sessions = sessions.filter(item => item.id !== id);
  if (activeSessionId === id) {
    activeSessionId = sessions[Math.max(0, index - 1)]?.id || sessions[0]?.id || '';
  }
  persistSessions();
  render();
}

function renderMessages() {
  const session = activeSession();
  el.messages.innerHTML = '';
  if (session.messages.length === 0) {
    el.messages.innerHTML = '<div class="empty"><div class="empty-mark">💬</div><div>发送消息开始对话</div></div>';
    return;
  }
  session.messages.forEach(message => {
    const box = document.createElement('div');
    box.className = `msg ${message.role}${message.error ? ' error' : ''}`;
    if (message.mediaUrl) {
      const media = message.kind === 'video' ? document.createElement('video') : document.createElement('img');
      media.className = 'generated';
      media.src = message.mediaUrl;
      if (message.kind === 'video') media.controls = true;
      box.appendChild(media);
    }
    if (Array.isArray(message.attachments)) {
      message.attachments.forEach(item => {
        const image = document.createElement('img');
        image.className = 'generated';
        image.src = item.dataUrl;
        image.alt = item.name || 'image';
        box.appendChild(image);
      });
    }
    const text = document.createElement('div');
    text.textContent = message.pending ? '正在生成...' : message.content;
    box.appendChild(text);
    el.messages.appendChild(box);
  });
  requestAnimationFrame(() => {
    el.messages.scrollTop = el.messages.scrollHeight;
  });
}

function openSettings() {
  draftSettings = { ...settings };
  el.apiKeyInput.value = draftSettings.apiKey || embeddedToken;
  el.apiEndpointInput.value = resolvedAPIEndpoint;
  el.apiEndpointInput.readOnly = true;
  syncModelSelect(draftSettings.model);
  showNotice('');
  el.modal.classList.add('open');
  if (!models.length) loadModels();
}

function closeSettings() {
  el.modal.classList.remove('open');
}

function saveSettings() {
  settings = readSettingsForm();
  saveJson(settingsKey, settings);
  closeSettings();
  render();
}

function readSettingsForm() {
  return {
    apiKey: el.apiKeyInput.value.trim(),
    model: el.modelSelect.value.trim()
  };
}

function showNotice(text, error = false) {
  el.notice.textContent = text;
  el.notice.className = 'notice' + (text ? ' show' : '') + (error ? ' error' : '');
}

function syncModelSelect(selected) {
  el.modelSelect.innerHTML = '<option value="">请选择模型</option>';
  models.forEach(model => {
    const option = document.createElement('option');
    option.value = model;
    option.textContent = model;
    el.modelSelect.appendChild(option);
  });
  el.modelSelect.value = selected || '';
}

function endpoint(path) {
  return normalizeEndpoint(resolvedAPIEndpoint || '/v1') + path;
}

function normalizeEndpoint(value) {
  const normalized = (value || '/v1').trim().replace(/\/+$/, '');
  return normalized || '/v1';
}

function headers(cfg = settings) {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${cfg.apiKey || embeddedToken}`
  };
}

async function loadModels() {
  const cfg = readSettingsForm();
  el.loadModelsBtn.disabled = true;
  showNotice('正在加载模型列表...');
  try {
    const res = await fetch(endpoint('/models'), { headers: headers(cfg) });
    if (!res.ok) throw new Error(await errorText(res));
    const data = await res.json();
    const rows = Array.isArray(data.data) ? data.data : Array.isArray(data.models) ? data.models : [];
    models = [...new Set(rows.map(item => item.id || item.name).filter(Boolean))].sort();
    syncModelSelect(cfg.model || models[0] || '');
    showNotice(models.length ? `已加载 ${models.length} 个模型` : '接口已响应，但未解析到模型列表');
  } catch (error) {
    showNotice(error.message || '加载模型列表失败', true);
  } finally {
    el.loadModelsBtn.disabled = false;
  }
}

async function testModel() {
  const cfg = readSettingsForm();
  if (!cfg.model) {
    showNotice('请先选择模型', true);
    return;
  }
  el.testModelBtn.disabled = true;
  showNotice('正在测试模型...');
  try {
    await chat([{ role: 'user', content: 'ping' }], cfg);
    showNotice('模型测试成功');
  } catch (error) {
    showNotice(error.message || '模型测试失败', true);
  } finally {
    el.testModelBtn.disabled = false;
  }
}

async function send() {
  const content = el.prompt.value.trim();
  if ((!content && attachments.length === 0) || busy) return;
  if (!settings.apiKey && !embeddedToken) {
    openSettings();
    showNotice('请先配置 API Key', true);
    return;
  }
  if (!settings.model) {
    openSettings();
    showNotice('请先选择模型', true);
    return;
  }

  busy = true;
  el.sendBtn.disabled = true;
  el.prompt.value = '';
  const outgoingAttachments = attachments;
  attachments = [];
  renderAttachments();

  const session = activeSession();
  session.messages.push({
    role: 'user',
    content: content || '[图片]',
    attachments: outgoingAttachments
  });
  if (session.title === '新对话') session.title = content.slice(0, 18);
  const assistant = { role: 'assistant', content: '', pending: true, kind: el.mode.value };
  session.messages.push(assistant);
  session.updatedAt = Date.now();
  render();

  try {
    if (el.mode.value === 'image') {
      const image = await imageGeneration(content, outgoingAttachments);
      assistant.mediaUrl = image.url;
      assistant.content = image.text;
    } else if (el.mode.value === 'video') {
      const video = await videoGeneration(content, outgoingAttachments);
      assistant.mediaUrl = video.url;
      assistant.content = video.text;
    } else {
      assistant.content = await chat(session.messages.filter(item => !item.pending).map(toOpenAIMessage));
    }
  } catch (error) {
    assistant.error = true;
    assistant.content = error.message || '请求失败';
  } finally {
    assistant.pending = false;
    session.updatedAt = Date.now();
    persistSessions();
    busy = false;
    el.sendBtn.disabled = false;
    render();
  }
}

async function chat(messages, cfg = settings) {
  const res = await fetch(endpoint('/chat/completions'), {
    method: 'POST',
    headers: headers(cfg),
    body: JSON.stringify({
      model: cfg.model,
      messages
    })
  });
  if (!res.ok) throw new Error(await errorText(res));
  const data = await res.json();
  return data.choices?.[0]?.message?.content || data.output_text || JSON.stringify(data);
}

function toOpenAIMessage(message) {
  const imageParts = Array.isArray(message.attachments)
    ? message.attachments.map(item => ({
        type: 'image_url',
        image_url: { url: item.dataUrl }
      }))
    : [];
  if (!imageParts.length) {
    return { role: message.role, content: message.content };
  }
  return {
    role: message.role,
    content: [
      { type: 'text', text: message.content || '请分析这张图片' },
      ...imageParts
    ]
  };
}

async function imageGeneration(prompt, refs = []) {
  const res = await fetch(endpoint('/images/generations'), {
    method: 'POST',
    headers: headers(),
    body: JSON.stringify({
      model: settings.model,
      prompt,
      image: refs.map(item => item.dataUrl),
      image_url: refs.map(item => item.dataUrl),
      size: '1024x1024',
      n: 1
    })
  });
  if (!res.ok) throw new Error(await errorText(res));
  const data = await res.json();
  const first = data.data?.[0] || data.output?.[0] || data;
  const url = first.url || first.image_url || (first.b64_json ? `data:image/png;base64,${first.b64_json}` : '');
  return { url, text: url ? '图片生成完成' : JSON.stringify(data) };
}

async function videoGeneration(prompt, refs = []) {
  const res = await fetch(endpoint('/videos/generations'), {
    method: 'POST',
    headers: headers(),
    body: JSON.stringify({
      model: settings.model,
      prompt,
      image: refs.map(item => item.dataUrl),
      image_url: refs.map(item => item.dataUrl)
    })
  });
  if (!res.ok) throw new Error(await errorText(res));
  const data = await res.json();
  const first = data.data?.[0] || data.output?.[0] || data;
  const url = first.url || first.video_url || first.asset_url || '';
  return { url, text: url ? '视频生成完成' : JSON.stringify(data) };
}

async function errorText(res) {
  try {
    const data = await res.json();
    return data.error?.message || data.message || `${res.status} ${res.statusText}`;
  } catch {
    return `${res.status} ${res.statusText}`;
  }
}
