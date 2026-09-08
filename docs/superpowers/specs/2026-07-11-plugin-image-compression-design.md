# Plugin Image Compression Design

## Scope

Optimize only the image-generation plugin service. Compress uploaded reference images and generated result previews while preserving the original bytes for full-size viewing and download.

## Image Variants

Each persisted image may have two variants:

- Original: unchanged source bytes, content type, dimensions, and file name where available.
- Preview: JPEG at quality 82 for opaque images and lossless WebP for images with transparency, with the longest edge limited to 1600 pixels. The browser prefers WebP at quality 82 for local previews.

Compression must never replace the original object. If preview generation fails, the operation continues with the original as the display fallback.

## Reference Image Flow

The browser keeps the selected original file and creates a compressed preview for immediate UI display. The request sends the original image to the plugin once. The plugin backend persists those bytes, creates the compressed variant, and forwards that variant to the generation provider. This preserves an original download without sending both variants across the browser-to-plugin boundary.

Reference images shown in the composer and conversation use the preview URL after persistence. Before persistence, the browser-generated WebP data URL is used. Clicking a reference image opens the original.

## Generated Image Flow

The backend archives the provider result as the original object, then generates and stores a compressed preview object. History result metadata exposes both URLs:

- `url`: original authenticated media URL, retained for backward compatibility.
- `preview_url`: compressed authenticated media URL.

The frontend uses `preview_url` for cards and message lists, falling back to `url` or `b64_json` for old records and compression failures. Actions that reuse an image as a model reference use the original URL.

## Media Routes

Existing original routes remain valid:

- `/assets/{historyID}/result/{index}`
- `/assets/{historyID}/reference/{index}`

Preview routes add a `preview` suffix:

- `/assets/{historyID}/result/{index}/preview`
- `/assets/{historyID}/reference/{index}/preview`

All routes retain the existing ownership and authentication checks. Original downloads set a useful `Content-Disposition` file name when requested through the download action.

## User Interface

Generated image cards display the compressed preview. Clicking the image opens a modal containing the original image URL. The modal provides a download-original command and closes by its close control, backdrop click, or Escape.

Reference thumbnails are clickable and use the same original-view modal. Generated cards also expose a direct download-original command. Existing reference, refine, and repeat actions remain unchanged.

## Compatibility

Historical records without `preview_url` display their original URL. Records whose preview object is missing also fall back to the original after an image load error. No database migration is required because image metadata is stored in existing JSON fields.

## Error Handling

- Unsupported or undecodable image input returns the existing validation error when the original itself is invalid.
- Preview encoding or preview storage failure is non-fatal and leaves only the original metadata.
- Original-view and download failures surface an accessible frontend error without breaking the conversation.
- Object cleanup deletes both original and preview storage keys when present.

## Testing

Backend tests cover JPEG/WebP selection, dimensions, original byte preservation, preview metadata, preview fallback, authenticated preview serving, and deletion of both variants. Frontend tests cover preview selection, legacy fallback, opening original images, downloading originals, Escape/backdrop behavior, and reference image compression before display. Production assets are rebuilt and Go hosting tests verify the generated bundle.
