## ADDED Requirements

### Requirement: Chat Widget Handoff to Ticket Form

The ticket creation page (`/support/tickets/new`) SHALL accept the URL query parameters `from=chat` and `session=<localStorage-key>`. When **both** parameters are present, the page SHALL:

1. Read the value at `localStorage[<session>]`. The expected schema is `{ messages: [...], updated_at: string }` (matching the `support-chat` widget's persistence format).
2. Render each message into a Markdown block (with role labels such as `**User:**` / `**Assistant:**`).
3. Pre-fill the `content` Markdown field with the rendered Markdown as an editable draft (the user MAY edit it before submitting).
4. Submit the same rendered Markdown verbatim as the request's `chat_context` field (in addition to whatever final `content` value the user sets).

When the URL query is present but `localStorage[<session>]` is missing, malformed, or empty, the page SHALL silently fall back to an empty draft (logging a `console.warn` for debuggability) and SHALL NOT display an error toast — the URL-driven handoff is best-effort and SHALL never block ticket creation.

When `support_ticket_enabled = false`, this handoff is unreachable because the ticket creation route itself is hidden / 404s.

#### Scenario: Handoff fills both content and chat_context

- **GIVEN** `localStorage.support_chat_session_v1 = {messages:[{role:"user",content:"怎么充值？"},{role:"assistant",content:"...步骤..."}], updated_at:"..."}` and the ticket creation page is opened at `/support/tickets/new?from=chat&session=support_chat_session_v1`
- **WHEN** the page renders
- **THEN** the `content` textarea is pre-filled with a Markdown rendering of the two messages, the user can edit it, AND clicking submit results in `POST /api/v1/support/tickets` with `chat_context` equal to the original Markdown rendering (not the user-edited content)

#### Scenario: Missing localStorage falls back to empty draft

- **GIVEN** `localStorage.support_chat_session_v1` is undefined (or empty messages) AND the page is opened at `/support/tickets/new?from=chat&session=support_chat_session_v1`
- **WHEN** the page renders
- **THEN** the `content` textarea is empty, no error toast appears, and a `console.warn` is emitted; submission proceeds normally without `chat_context`

#### Scenario: Handoff URL absent leaves form blank

- **GIVEN** the page is opened at `/support/tickets/new` (no query parameters)
- **WHEN** the page renders
- **THEN** the form is fully blank and `localStorage` is not read at all
