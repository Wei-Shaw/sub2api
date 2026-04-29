/**
 * Tailwind v3 preflight 中 `--tw-*` 自定义属性初始值.
 *
 * 为什么单独一份:
 *   Tailwind utility (gradient / shadow / ring / transform 等) 的 CSS 是
 *   `var(--tw-gradient-stops)` 这种引用形式, 真正的初始值靠 preflight 注入到
 *   `*, ::before, ::after` 选择器上. 没有这层初始值, `var()` 会 invalid 整条
 *   声明, 渐变/阴影直接失效 (实际表现: .btn-primary 变成纯色, 没有渐变).
 *
 *   Shadow DOM 边界规则:
 *     - `--accent-color` 这种 `:root { ... }` 上定义的变量, 因为是 inheritance,
 *       会自然穿透到 shadow host 再下沉到 shadow tree
 *     - 但 Tailwind 的 `--tw-*` 是 `*` 选择器在每个元素上单独设置的, 不是
 *       inheritance, 不会跨 shadow boundary
 *   所以必须在 shadow root 内重新注入一份.
 *
 * 内容来源:
 *   抄自 Tailwind v3 编译产物中 `*, ::before, ::after { ... }` 那一块的
 *   `--tw-*` 默认值. 不包含 `box-sizing` 等 reset, 调用方自己决定要不要.
 *
 * 维护规则:
 *   升级 Tailwind 大版本时需要重新对照新版 preflight 输出, 把新增的
 *   `--tw-*` 变量补进来.
 */
export const TAILWIND_SHADOW_PREFLIGHT = `
*, ::before, ::after {
  --tw-border-spacing-x: 0;
  --tw-border-spacing-y: 0;
  --tw-translate-x: 0;
  --tw-translate-y: 0;
  --tw-rotate: 0;
  --tw-skew-x: 0;
  --tw-skew-y: 0;
  --tw-scale-x: 1;
  --tw-scale-y: 1;
  --tw-pan-x:  ;
  --tw-pan-y:  ;
  --tw-pinch-zoom:  ;
  --tw-scroll-snap-strictness: proximity;
  --tw-gradient-from-position:  ;
  --tw-gradient-via-position:  ;
  --tw-gradient-to-position:  ;
  --tw-ordinal:  ;
  --tw-slashed-zero:  ;
  --tw-numeric-figure:  ;
  --tw-numeric-spacing:  ;
  --tw-numeric-fraction:  ;
  --tw-ring-inset:  ;
  --tw-ring-offset-width: 0px;
  --tw-ring-offset-color: #fff;
  --tw-ring-color: rgb(59 130 246 / 0.5);
  --tw-ring-offset-shadow: 0 0 #0000;
  --tw-ring-shadow: 0 0 #0000;
  --tw-shadow: 0 0 #0000;
  --tw-shadow-colored: 0 0 #0000;
  --tw-blur:  ;
  --tw-brightness:  ;
  --tw-contrast:  ;
  --tw-grayscale:  ;
  --tw-hue-rotate:  ;
  --tw-invert:  ;
  --tw-saturate:  ;
  --tw-sepia:  ;
  --tw-drop-shadow:  ;
  --tw-backdrop-blur:  ;
  --tw-backdrop-brightness:  ;
  --tw-backdrop-contrast:  ;
  --tw-backdrop-grayscale:  ;
  --tw-backdrop-hue-rotate:  ;
  --tw-backdrop-invert:  ;
  --tw-backdrop-opacity:  ;
  --tw-backdrop-saturate:  ;
  --tw-backdrop-sepia:  ;
  --tw-contain-size:  ;
  --tw-contain-layout:  ;
  --tw-contain-paint:  ;
  --tw-contain-style:  ;
}
::backdrop {
  --tw-border-spacing-x: 0;
  --tw-border-spacing-y: 0;
  --tw-translate-x: 0;
  --tw-translate-y: 0;
  --tw-rotate: 0;
  --tw-skew-x: 0;
  --tw-skew-y: 0;
  --tw-scale-x: 1;
  --tw-scale-y: 1;
  --tw-pan-x:  ;
  --tw-pan-y:  ;
  --tw-pinch-zoom:  ;
  --tw-scroll-snap-strictness: proximity;
  --tw-gradient-from-position:  ;
  --tw-gradient-via-position:  ;
  --tw-gradient-to-position:  ;
  --tw-ordinal:  ;
  --tw-slashed-zero:  ;
  --tw-numeric-figure:  ;
  --tw-numeric-spacing:  ;
  --tw-numeric-fraction:  ;
  --tw-ring-inset:  ;
  --tw-ring-offset-width: 0px;
  --tw-ring-offset-color: #fff;
  --tw-ring-color: rgb(59 130 246 / 0.5);
  --tw-ring-offset-shadow: 0 0 #0000;
  --tw-ring-shadow: 0 0 #0000;
  --tw-shadow: 0 0 #0000;
  --tw-shadow-colored: 0 0 #0000;
}
`
