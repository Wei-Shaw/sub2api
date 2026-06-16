/**
 * Plugin API client wrapper.
 *
 * The core frontend provides an axios instance with auth interceptors. The
 * plugin receives that instance via setClient() (called from install()) and
 * uses it for all HTTP calls so authentication and error handling stay
 * consistent with the host app.
 *
 * All plugin endpoints live under /api/v1/plugin/content-moderation/* — the
 * riskControl API module hardcodes that prefix on each path.
 */
import { createApiClient } from '@sub2api/plugin-sdk'

export const { setClient, getClient } = createApiClient('plugin-content-moderation')
