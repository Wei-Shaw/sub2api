# ModelTrace

Source: https://github.com/xqy2006/ModelTrace (MIT)

The scoring algorithm and challenge generator are Go ports of upstream
`static/fingerprint-core.js` and `static/challenge-browser.js`, pinned at
`df3a0f9d3e054c0dc02d6d586686db8daf8fa7c8`. The license is embedded in the plugin.

`unified_bank.json` contains the reference profiles, feature transforms and
calibration for all candidates. Updating candidates requires refitting these
artifacts together; adding a model name alone does not provide a baseline.
History records identify the bank by its SHA-256 digest.

Probabilities compare only the included candidates. Collection conditions and
similar model behavior can affect attribution; results do not prove identity.
