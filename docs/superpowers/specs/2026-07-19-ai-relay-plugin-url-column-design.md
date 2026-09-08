# AI Relay Plugin URL Column Design

## Scope

Update only the standalone AI Relay plugin UI and plugin-service runtime configuration. Do not restore or modify the removed main-site AI Relay page.

## Route Table

The route table follows the last main-site AI Relay page while retaining the plugin-only mapping count:

1. Selection
2. Name
3. Platform
4. Target URL
5. Plugin URL
6. Mappings
7. Actions

The Name cell displays only `route.name`, falling back to `route.slug` when the name is empty. It does not render the slug as secondary text.

The Plugin URL cell displays the complete relay base route:

`{relay_base_url}/plugins/ai-relay/{platform}/{slug}`

The existing copy action copies exactly the URL shown in this column. Edit and delete actions remain unchanged.

## Runtime URL Resolution

The plugin-service exposes its configured AI Relay base URL through an authenticated plugin API endpoint. The frontend loads this value with the platform and route data and uses it for every displayed or copied Plugin URL.

The local source-run default is:

`http://127.0.0.1:8091`

Docker Compose explicitly configures:

`http://plugin-server:8091`

The configured port follows `PLUGIN_SERVER_PORT`. A dedicated environment variable can override the complete base URL so non-default deployments do not require frontend rebuilds.

## Failure Behavior

If runtime configuration cannot be loaded, route management remains available and Plugin URL generation falls back to the local source-run URL. The existing page-level error handling continues to report required API failures.

## Verification

- Frontend component tests cover Name rendering without secondary slug text.
- Frontend tests cover complete local and Docker Plugin URL rendering and copying.
- Go handler/configuration tests cover default and configured runtime base URLs.
- Plugin frontend tests, typecheck, and production build pass.
- Plugin-service Go tests pass.
- Git status confirms no main-site frontend files changed.
