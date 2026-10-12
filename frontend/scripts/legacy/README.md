# Archived Scheduler Overlay

This script patches the specific historical SchedulerMonitorView-DJKX4gd_.js
bundle. Current SchedulerMonitorView.vue already imports schedulerRecovery.ts.
It is not part of current builds and must not be run against current assets.

The original entry point now fails without touching files. The archived script
also fails unless BUAGA_ALLOW_LEGACY_OVERLAY=1 is explicitly set for historical
reproduction from the frontend working directory.

Use pnpm verify:release for current source-based builds. The candidate Dockerfile
requires the same complete frontend test/build stage before packaging its assets.
