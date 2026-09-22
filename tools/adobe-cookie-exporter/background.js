const ARP_STORAGE_KEY = "arp_session_id";
const TOKEN_STORAGE_KEY = "access_token";
const ARP_HEADER = "x-arp-session-id";
const AUTH_HEADER = "authorization";
const FIREFLY_TABS = ["https://firefly.adobe.com/*"];
const ARP_REQUEST_URLS = [
  "https://*.adobe.io/*",
  "https://*.ff.adobe.io/*",
  "https://*.adobe.com/*",
];

function headerValue(headers, name) {
  if (!Array.isArray(headers)) return "";
  const target = name.toLowerCase();
  for (const item of headers) {
    if (String(item.name || "").toLowerCase() === target) {
      return String(item.value || "").trim();
    }
  }
  return "";
}

function stripBearer(value) {
  const text = String(value || "").trim();
  if (text.slice(0, 7).toLowerCase() === "bearer ") {
    return text.slice(7).trim();
  }
  return text;
}

function isFireflySubmit(url) {
  const value = String(url || "");
  return value.includes("generate-async") || value.includes("firefly-3p");
}

function persistSession(fields) {
  if (!chrome.storage?.session) return;
  const patch = {};
  const arp = String(fields.arp || "").trim();
  const token = stripBearer(fields.token);
  if (arp) patch[ARP_STORAGE_KEY] = arp;
  if (token) patch[TOKEN_STORAGE_KEY] = token;
  if (Object.keys(patch).length === 0) return;
  chrome.storage.session.set(patch);
}

chrome.webRequest.onBeforeSendHeaders.addListener(
  (details) => {
    const arp = headerValue(details.requestHeaders, ARP_HEADER);
    const token = isFireflySubmit(details.url)
      ? headerValue(details.requestHeaders, AUTH_HEADER)
      : "";
    persistSession({ arp, token });
  },
  { urls: ARP_REQUEST_URLS },
  ["requestHeaders", "extraHeaders"]
);

async function injectArpHooks(tabId) {
  if (typeof tabId !== "number") return;
  try {
    await chrome.scripting.executeScript({
      target: { tabId },
      files: ["content-main.js"],
      world: "MAIN",
      injectImmediately: true,
    });
    await chrome.scripting.executeScript({
      target: { tabId },
      files: ["content-isolated.js"],
      injectImmediately: true,
    });
  } catch {
    // Tab may not be a Firefly page, or the user has not granted host access.
  }
}

async function injectIntoOpenFireflyTabs() {
  const tabs = await chrome.tabs.query({ url: FIREFLY_TABS });
  await Promise.all(tabs.map((tab) => injectArpHooks(tab.id)));
}

chrome.runtime.onInstalled.addListener(() => {
  injectIntoOpenFireflyTabs();
});

chrome.runtime.onStartup.addListener(() => {
  injectIntoOpenFireflyTabs();
});
