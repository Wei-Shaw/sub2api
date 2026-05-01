/**
 * English i18n — `nav` + `common` namespaces.
 *
 * T19 Fix B — split out from monolithic `en.ts` so each file stays under the
 * 200-line TS util budget and ownership boundaries are obvious.
 */
export default {
  nav: {
    channels: 'Channels',
  },
  common: {
    refresh: 'Refresh',
    loading: 'Loading...',
    autoRefresh: {
      title: 'Auto Refresh',
      enable: 'Enable auto refresh',
      countdown: 'Auto refresh: {seconds}s',
      seconds: '{n} seconds',
    },
  },
}
