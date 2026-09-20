const statusText = document.getElementById("statusText");
const contextText = document.getElementById("contextText");
const scopeSelect = document.getElementById("scopeSelect");
const exportJsonBtn = document.getElementById("exportJsonBtn");

function setStatus(message) {
  statusText.textContent = message;
}

function toTimestampParts(date) {
  const pad = (n) => String(n).padStart(2, "0");
  return `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}_${pad(date.getHours())}${pad(date.getMinutes())}${pad(date.getSeconds())}`;
}

function mapSameSite(value) {
  if (value === "no_restriction") return "None";
  if (value === "lax") return "Lax";
  if (value === "strict") return "Strict";
  return "Lax";
}

function getCurrentTab() {
  return new Promise((resolve, reject) => {
    chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }
      resolve(tabs?.[0] ? tabs[0] : null);
    });
  });
}

function getAllCookieStores() {
  return new Promise((resolve, reject) => {
    chrome.cookies.getAllCookieStores((stores) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }
      resolve(Array.isArray(stores) ? stores : []);
    });
  });
}

async function getCurrentContext() {
  const tab = await getCurrentTab();
  if (!tab || typeof tab.id !== "number") {
    throw new Error("Unable to find the active tab for cookie export.");
  }

  const stores = await getAllCookieStores();
  const matchedStore = stores.find(
    (store) => Array.isArray(store.tabIds) && store.tabIds.includes(tab.id)
  );
  if (!matchedStore?.id) {
    throw new Error("Unable to resolve the cookie store for the active tab.");
  }

  return {
    tab,
    storeId: matchedStore.id,
    incognito: Boolean(tab.incognito || chrome.extension.inIncognitoContext),
  };
}

function getCookies(filter, storeId) {
  return new Promise((resolve, reject) => {
    const nextFilter = { ...(filter || {}) };
    if (storeId) {
      nextFilter.storeId = storeId;
    }
    chrome.cookies.getAll(nextFilter, (cookies) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }
      resolve(Array.isArray(cookies) ? cookies : []);
    });
  });
}

function sanitizeAccountLabel(value) {
  const text = String(value || "")
    .replace(/[\u0000-\u001f\u007f]/g, "")
    .replace(/\s+/g, " ")
    .trim();
  if (!text) return "";
  return text.slice(0, 80);
}

function getFireflyAccountLabel(tabId) {
  return new Promise((resolve) => {
    if (typeof tabId !== "number" || !chrome.scripting) {
      resolve("");
      return;
    }

    chrome.scripting.executeScript(
      {
        target: { tabId },
        func: () => {
          const emailRe = /[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i;
          const interestingKey = /email|user|profile|ims|adobe|account|identity/i;
          const pickFromObject = (value) => {
            if (!value || typeof value !== "object") return { email: "", name: "" };
            const record = value;
            const emailKeys = ["email", "userEmail", "mail", "emailAddress"];
            const nameKeys = ["displayName", "fullName", "name", "userName"];
            let email = "";
            let name = "";
            for (const key of emailKeys) {
              const item = record[key];
              if (typeof item === "string" && emailRe.test(item.trim())) {
                email = item.trim();
                break;
              }
            }
            for (const key of nameKeys) {
              const item = record[key];
              if (typeof item === "string" && item.trim()) {
                name = item.trim();
                break;
              }
            }
            if (!email) {
              const match = JSON.stringify(record).match(emailRe);
              if (match) email = match[0];
            }
            return { email, name };
          };
          const pickFromRaw = (raw) => {
            if (!raw) return { email: "", name: "" };
            try {
              return pickFromObject(JSON.parse(raw));
            } catch {
              const match = String(raw).match(emailRe);
              return { email: match ? match[0] : "", name: "" };
            }
          };

          let email = "";
          let name = "";
          const consider = (candidate) => {
            if (!email && candidate.email) email = candidate.email;
            if (!name && candidate.name) name = candidate.name;
          };

          for (const store of [localStorage, sessionStorage]) {
            for (let i = 0; i < store.length; i += 1) {
              const key = store.key(i);
              if (!key || !interestingKey.test(key)) continue;
              consider(pickFromRaw(store.getItem(key)));
              if (email) break;
            }
            if (email) break;
          }

          return { email, name };
        },
      },
      (results) => {
        if (chrome.runtime.lastError) {
          resolve("");
          return;
        }
        const value =
          Array.isArray(results) && results[0] ? results[0].result : null;
        const email =
          value && typeof value.email === "string"
            ? sanitizeAccountLabel(value.email)
            : "";
        const name =
          value && typeof value.name === "string"
            ? sanitizeAccountLabel(value.name)
            : "";
        resolve(email || name);
      }
    );
  });
}

async function collectCookiesByScope(scope) {
  const context = await getCurrentContext();
  const { tab, storeId, incognito } = context;

  if (scope === "current") {
    const url = tab?.url ? tab.url : "";
    if (!url.startsWith("http://") && !url.startsWith("https://")) {
      throw new Error("The current tab is not a regular web page.");
    }
    const cookies = await getCookies({ url }, storeId);
    return { cookies, sourceUrl: url, storeId, incognito };
  }

  const domains = [".adobe.com", "firefly.adobe.com", "account.adobe.com"];
  const all = [];
  for (const domain of domains) {
    const cookies = await getCookies({ domain }, storeId);
    all.push(...cookies);
  }

  const unique = [];
  const seen = new Set();
  for (const item of all) {
    const key = `${item.domain}|${item.path}|${item.name}`;
    if (seen.has(key)) continue;
    seen.add(key);
    unique.push(item);
  }

  return {
    cookies: unique,
    sourceUrl: "https://firefly.adobe.com/",
    storeId,
    incognito,
  };
}

function toPlaywrightLikeCookies(cookies) {
  return cookies.map((item) => ({
    name: item.name,
    value: item.value,
    domain: item.domain,
    path: item.path || "/",
    expires: typeof item.expirationDate === "number" ? item.expirationDate : -1,
    httpOnly: Boolean(item.httpOnly),
    secure: Boolean(item.secure),
    sameSite: mapSameSite(item.sameSite),
  }));
}

function buildCookieHeader(cookies) {
  const parts = [];
  for (const item of cookies) {
    const name = String(item.name || "").trim();
    if (!name) continue;
    parts.push(`${name}=${String(item.value || "")}`);
  }
  return parts.join("; ");
}

function downloadJson(filename, data) {
  const blob = new Blob([JSON.stringify(data, null, 2)], {
    type: "application/json",
  });
  const url = URL.createObjectURL(blob);
  chrome.downloads.download({
    url,
    filename,
    saveAs: true,
  });
  setTimeout(() => URL.revokeObjectURL(url), 3000);
}

async function generatePayload() {
  const scope = scopeSelect.value;
  const { tab } = await getCurrentContext();
  const { cookies, incognito, storeId } = await collectCookiesByScope(scope);
  const normalizedCookies = toPlaywrightLikeCookies(cookies);
  const cookieHeader = buildCookieHeader(normalizedCookies);
  const now = new Date();
  const fileTs = toTimestampParts(now);
  const pageLabel =
    tab && String(tab.url || "").startsWith("https://firefly.adobe.com/")
      ? await getFireflyAccountLabel(tab.id)
      : "";
  const accountName = pageLabel ? `adobe-${pageLabel}` : `adobe-${fileTs}`;
  const payload = {
    type: "sub2api-data",
    version: 1,
    exported_at: now.toISOString(),
    proxies: [],
    accounts: [
      {
        name: accountName,
        platform: "adobe",
        type: "oauth",
        credentials: { cookie: cookieHeader },
        concurrency: 10,
        priority: 1,
      },
    ],
  };
  const fileName = `sub2api-account-adobe-${fileTs}.json`;
  return {
    payload,
    fileName,
    cookieCount: normalizedCookies.length,
    incognito,
    storeId,
  };
}

function renderContext(context) {
  const modeText = context.incognito ? "Incognito" : "Regular";
  contextText.textContent = `Browser context: ${modeText} window | store: ${context.storeId}`;
  if (context.incognito) {
    setStatus(
      "Incognito cookie store detected. Export will use the isolated incognito cookie jar."
    );
  } else {
    setStatus("Regular browser context detected.");
  }
}

async function initContext() {
  try {
    const context = await getCurrentContext();
    renderContext(context);
  } catch (error) {
    contextText.textContent = "Browser context: unavailable";
    setStatus(`Unable to detect the cookie store: ${error.message || error}`);
    exportJsonBtn.disabled = true;
  }
}

exportJsonBtn.addEventListener("click", async () => {
  try {
    setStatus("Reading cookies...");
    const { payload, fileName, cookieCount, incognito } =
      await generatePayload();
    if (!cookieCount) {
      setStatus("No cookies were found. Log in to Adobe or Firefly first.");
      return;
    }
    downloadJson(fileName, payload);
    const modeText = incognito ? "incognito" : "regular";
    setStatus(
      `Exported ${cookieCount} cookies from the ${modeText} browser store.`
    );
  } catch (error) {
    setStatus(`Export failed: ${error.message || error}`);
  }
});

initContext();
