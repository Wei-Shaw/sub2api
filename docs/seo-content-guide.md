# SEO Content Guide

This document explains how to maintain SEO/GEO content in this repository.

## Single source of truth: `shared/seo/`

| File | What it controls |
|---|---|
| `seo.json` | Per-route titles, descriptions, keywords, OG image, JSON-LD types |
| `faq.zh.json` / `faq.en.json` | FAQ page content + `FAQPage` JSON-LD + `/llms.txt` excerpts |
| `seo.schema.json` | Validation schema (do not edit unless adding new SEO fields) |

These four files are the single source of truth for SEO content. The Go backend reads them at build time (`make prepare-seo`) and embeds them; the Vue frontend imports them via the `@shared` alias.

## Build flow

`shared/seo/` is the source. The backend embeds JSON via `//go:embed` from `backend/internal/web/seo/data/`, which is populated by `make prepare-seo`:

```
make prepare-seo            # Copies shared/seo/*.json into backend/internal/web/seo/data/
make build                  # Implicitly runs prepare-seo first
```

The frontend imports the files directly from `shared/seo/` via the Vite `@shared` alias — no copy step needed.

On Windows where `make` is unavailable:

```powershell
cd backend
Copy-Item ../shared/seo/seo.json internal/web/seo/data/seo.json -Force
Copy-Item ../shared/seo/faq.zh.json internal/web/seo/data/faq.zh.json -Force
Copy-Item ../shared/seo/faq.en.json internal/web/seo/data/faq.en.json -Force
```

## Adding or editing a route's SEO

1. Open `shared/seo/seo.json`
2. Add or edit the route under `routes`:

```json
"/new-page": {
  "indexable": true,
  "priority": 0.7,
  "changefreq": "monthly",
  "jsonLd": ["BreadcrumbList"],
  "zh": {
    "title": "...",
    "description": "...",
    "keywords": ["..."]
  },
  "en": {
    "title": "...",
    "description": "...",
    "keywords": ["..."]
  }
}
```

3. **Required**: every route MUST have both `zh` and `en` blocks with non-empty `title` and `description`. CI will reject otherwise.
4. **Title**: ≤ 70 chars. **Description**: ≤ 200 chars (Google truncates ~160; AI engines tolerate longer).
5. Run `make prepare-seo` from `backend/` to refresh the embedded copy.

## Adding a FAQ entry

1. Edit BOTH `shared/seo/faq.zh.json` and `shared/seo/faq.en.json` (always bilingual, keep `id` in sync).
2. Each entry needs `id`, `q`, `a`, and optional `details` (markdown).
3. **Answer `a`** ≤ 160 chars — AI engines like Perplexity / ChatGPT often use the first sentence as the cited summary. Lead with the answer, then context.

## Feature flag

`seo_enabled` in admin settings controls whether the SEO `<head>` is injected. Default `false`. Enable from admin UI after deploying to verify production behavior; toggle off for an instant rollback.

## Verification

After changes, run:

```bash
cd backend
make prepare-seo
go test -tags=embed ./internal/web/seo/...
go test -tags=embed ./internal/handler/seo_handler_test.go
go test -tags=embed -run TestFrontendServer_seoInjection ./internal/web/
```

Start the server with `seo_enabled=true` (toggle via admin settings UI) and run:

```bash
node scripts/verify-seo.mjs http://localhost:8080
```

## OG images

Place 1200×630 PNGs at `frontend/public/og/`. Filenames must match `ogImage` paths in `seo.json` (e.g. `/og/home-zh.png` → `frontend/public/og/home-zh.png`). If a file is missing, the backend simply omits the OG image (no broken-image fallback).

## Don'ts

- ❌ Don't put per-route SEO in `frontend/src/i18n/locales/{zh,en}.ts` — those are UI strings, not SEO content.
- ❌ Don't edit the JSON files via admin UI / DB — they're build artifacts.
- ❌ Don't add new `schema.org` types without updating `seo.schema.json` `jsonLd` enum AND the `RenderGraph` switch in `backend/internal/web/seo/jsonld.go`.
- ❌ Don't commit anything to `backend/internal/web/seo/data/*.json` — those are gitignored, generated from `shared/seo/`.
