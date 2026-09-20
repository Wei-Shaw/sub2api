(() => {
  if (window.__sub2apiAdobeArpHooked) return;
  window.__sub2apiAdobeArpHooked = true;

  const MSG_TYPE = "sub2api-adobe-arp";

  function headerValue(headers, name) {
    if (!headers) return "";
    const target = name.toLowerCase();
    if (typeof Headers !== "undefined" && headers instanceof Headers) {
      return String(headers.get(name) || headers.get(target) || "").trim();
    }
    if (Array.isArray(headers)) {
      for (const item of headers) {
        if (!item) continue;
        if (Array.isArray(item) && String(item[0] || "").toLowerCase() === target) {
          return String(item[1] || "").trim();
        }
        if (typeof item === "object" && String(item.name || "").toLowerCase() === target) {
          return String(item.value || "").trim();
        }
      }
      return "";
    }
    if (typeof headers === "object") {
      for (const [key, value] of Object.entries(headers)) {
        if (String(key).toLowerCase() === target) {
          return String(value || "").trim();
        }
      }
    }
    return "";
  }

  function requestUrl(input, init) {
    if (typeof input === "string") return input;
    if (typeof URL !== "undefined" && input instanceof URL) return input.href;
    if (input && typeof input.url === "string") return input.url;
    if (input && typeof input.href === "string") return input.href;
    if (init && typeof init.url === "string") return init.url;
    return "";
  }

  function isFireflySubmit(url) {
    const value = String(url || "");
    return value.includes("generate-async") || value.includes("firefly-3p");
  }

  function stripBearer(value) {
    const text = String(value || "").trim();
    if (text.slice(0, 7).toLowerCase() === "bearer ") {
      return text.slice(7).trim();
    }
    return text;
  }

  function publishCapture(arp, token) {
    const payload = { type: MSG_TYPE };
    const session = String(arp || "").trim();
    const access = stripBearer(token);
    if (session) payload.arp_session_id = session;
    if (access) payload.access_token = access;
    if (!payload.arp_session_id && !payload.access_token) return;
    window.postMessage(payload, window.location.origin);
  }

  function captureFromRequest(input, init) {
    const url = requestUrl(input, init);
    let arp = "";
    let token = "";
    if (typeof Request !== "undefined" && input instanceof Request) {
      arp = headerValue(input.headers, "x-arp-session-id");
      token = headerValue(input.headers, "authorization");
    }
    if (init && init.headers) {
      arp = arp || headerValue(init.headers, "x-arp-session-id");
      token = token || headerValue(init.headers, "authorization");
    }
    if (!isFireflySubmit(url)) token = "";
    publishCapture(arp, token);
  }

  const originalFetch = window.fetch;
  window.fetch = function (input, init) {
    try {
      captureFromRequest(input, init);
    } catch {
      // Hook must never break Firefly's own generate-async.
    }
    return originalFetch.apply(this, arguments);
  };

  const originalOpen = XMLHttpRequest.prototype.open;
  const originalSetRequestHeader = XMLHttpRequest.prototype.setRequestHeader;
  const originalSend = XMLHttpRequest.prototype.send;

  XMLHttpRequest.prototype.open = function (method, url) {
    this.__sub2apiAdobeUrl = url;
    this.__sub2apiAdobeArp = "";
    this.__sub2apiAdobeToken = "";
    return originalOpen.apply(this, arguments);
  };

  XMLHttpRequest.prototype.setRequestHeader = function (name, value) {
    const header = String(name || "").toLowerCase();
    if (header === "x-arp-session-id") {
      this.__sub2apiAdobeArp = String(value || "");
    }
    if (header === "authorization") {
      this.__sub2apiAdobeToken = String(value || "");
    }
    return originalSetRequestHeader.apply(this, arguments);
  };

  XMLHttpRequest.prototype.send = function () {
    try {
      const token = isFireflySubmit(this.__sub2apiAdobeUrl)
        ? this.__sub2apiAdobeToken
        : "";
      publishCapture(this.__sub2apiAdobeArp, token);
    } catch {
      // Same as fetch: never interfere with the page request.
    }
    return originalSend.apply(this, arguments);
  };
})();
