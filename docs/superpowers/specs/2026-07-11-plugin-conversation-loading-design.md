# Plugin Conversation Loading Design

## Goal

Make the image-generation plugin render its dependencies and conversation list first, then load only the newest messages for the selected conversation.

## API

- `GET /conversations?limit=20&cursor=<updated_at,id>` returns conversation summaries ordered newest first.
- `GET /conversations/{id}/messages?limit=20&before=<created_at,id>` returns the newest message records first plus a cursor for older records.
- Conversation summaries contain ID, title, latest prompt/result preview, status, updated time, and optional preview thumbnail URL. They do not contain full requests or results.
- Message pages contain the complete history records required to project user and assistant messages.
- Cursors are opaque URL-safe values. Limits default to 20 and are capped at 100.

## Startup

The frontend starts `me`, config, key, and conversation-summary requests concurrently. It renders the shell immediately. When summaries arrive it selects the newest conversation and requests its newest message page. Key or config latency does not block the sidebar.

## Interaction

Selecting a conversation cancels or ignores stale detail responses and loads its newest messages. Scrolling to the top requests the prior page and prepends it without changing scroll position. New local conversations appear immediately. Successful generation updates the active messages and moves its summary to the top without reloading all conversations.

## Cleanup

The plugin frontend stops using `/history`. Remove `projectHistory`, `HistoryList`, `listHistory`, and their obsolete tests. The backend may remove the old list route because compatibility is not required; record-specific status, retry, cancel, delete, and asset routes remain.

## Testing

Repository tests cover user scoping, stable cursor order, summary grouping, newest message pages, and older-page cursors. Handler tests cover parameter validation and response envelopes. Frontend tests cover concurrent startup, summary-first rendering, stale detail suppression, latest-page selection, and loading older messages.
