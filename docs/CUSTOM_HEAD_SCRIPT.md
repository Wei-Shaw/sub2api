# Custom Head Script

Administrators can configure `custom_head_script` under **Settings > General >
Site Settings**. The setting accepts JavaScript or complete `<script>` tags,
including multiple inline/external scripts and HTML comments, up to 64 KiB.
Non-script HTML and unclosed script tags are rejected before saving.

```html
<script async src="https://analytics.example.com/script.js" data-site="example"></script>
<script>window.siteCustomization = true;</script>
```

The embedded frontend inserts the scripts before `</head>` on every application
page, including public, authentication, user and admin routes. They execute once
per document load, not on each Vue route change. Standalone and development
frontends use the public settings to insert the same scripts into `document.head`.
Saving invalidates the existing HTML cache; reload to apply changes. Clear and
save the field to disable it. Existing loaded pages retain already executed code
until reloaded. API responses and third-party iframe documents are not modified.

Every injected script receives the response's CSP nonce; supplied nonces are
replaced. Other CSP restrictions remain unchanged. Analytics network requests,
images, frames, or dynamically loaded scripts may require explicit origins in
the deployment's `security.csp.policy`. Do not disable CSP to enable a snippet.

This is intentionally trusted administrator code, with access to the current
user's session and page. Only use reviewed code. The source is publicly visible
in HTML and `/api/v1/settings/public`; never include API keys or other secrets.
The setting uses the existing settings table and requires no schema migration.
Older clients that omit the field preserve the stored value.
