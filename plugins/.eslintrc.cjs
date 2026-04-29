/**
 * Plan B·D5 — ESLint config for plugin sources.
 *
 * Forbids raw Tailwind semantic colors (emerald/red/blue/amber/green/yellow/
 * primary) in plugin `.vue` template `class` attributes; plugins must use SDK
 * semantic utility classes (.btn-*, .badge-*, .input-required, etc.).
 *
 * Brand color `purple-*` is intentionally NOT forbidden.
 *
 * The rule lives in `<repo>/frontend/.eslint-rules/no-raw-semantic-color.cjs`
 * and is loaded via `eslint-plugin-local-rules` (which looks up the
 * `eslint-local-rules.cjs` index from CWD ancestors).
 *
 * Run from `<repo>/frontend` via:
 *
 *   pnpm run lint:plugins
 *
 * which expands to:
 *
 *   eslint --no-eslintrc --config ../plugins/.eslintrc.cjs \
 *          --resolve-plugins-relative-to . --ext .vue ../plugins
 *
 * The `--no-eslintrc` flag is required so the host's `frontend/.eslintrc.cjs`
 * is not loaded; `--resolve-plugins-relative-to .` makes ESLint find
 * `eslint-plugin-local-rules` and `vue-eslint-parser` in
 * `frontend/node_modules`.
 *
 * This config is intentionally narrow — only the no-raw-semantic-color rule
 * is enabled. Other plugins-side lint concerns are out of scope.
 *
 * Severity: warn (will be promoted to error in a follow-up PR after baseline
 * is confirmed clean).
 */
module.exports = {
  root: true,
  env: {
    browser: true,
    es2021: true,
    node: true,
  },
  parser: "vue-eslint-parser",
  parserOptions: {
    parser: "@typescript-eslint/parser",
    ecmaVersion: "latest",
    sourceType: "module",
    extraFileExtensions: [".vue"],
  },
  plugins: ["local-rules"],
  ignorePatterns: [
    "**/dist/**",
    "**/node_modules/**",
    "**/.cache/**",
  ],
  rules: {
    "local-rules/no-raw-semantic-color": "warn",
  },
};
