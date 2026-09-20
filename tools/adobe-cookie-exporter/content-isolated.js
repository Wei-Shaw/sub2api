(() => {
  if (globalThis.__sub2apiAdobeArpListener) return;
  globalThis.__sub2apiAdobeArpListener = true;

  const MSG_TYPE = "sub2api-adobe-arp";
  const ARP_STORAGE_KEY = "arp_session_id";
  const TOKEN_STORAGE_KEY = "access_token";

  window.addEventListener("message", (event) => {
    if (event.source !== window) return;
    const data = event.data;
    if (!data || data.type !== MSG_TYPE) return;
    const patch = {};
    const arp = String(data.arp_session_id || "").trim();
    const token = String(data.access_token || "").trim();
    if (arp) patch[ARP_STORAGE_KEY] = arp;
    if (token) patch[TOKEN_STORAGE_KEY] = token;
    if (Object.keys(patch).length === 0) return;
    chrome.storage.session.set(patch);
  });
})();
