/**
 * Plugin shim — re-references the shared host module declarations from
 * @sub2api/plugin-sdk so vue-tsc resolves @tanstack/vue-virtual and
 * window.__APP_CONFIG__ during type checking.
 */
/// <reference path="../node_modules/@sub2api/plugin-sdk/src/types/host-modules.d.ts" />
