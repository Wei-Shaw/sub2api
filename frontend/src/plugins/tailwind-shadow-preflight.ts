/**
 * Tailwind v3 preflight 中 `--tw-*` 自定义属性初始值 + 关键 form/border reset.
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
 *   Shadow DOM 也不会继承 Tailwind preflight 的 `border-width: 0; border-style:
 *   solid` 通用重置, 导致 `<button>` `<input>` `<select>` 的浏览器默认 outset/
 *   inset 边框漏出来 (现象: 按钮/输入框出现一圈深灰"黑边"). 因此除了 `--tw-*`
 *   变量, 必须把以下两类关键 reset 也注入:
 *     1. 通用 border 重置 (border-width:0; border-style:solid)
 *     2. form 元素 reset (button/input/select/textarea 字体继承 + button 移除
 *        默认 outset 外观)
 *   不注入完整 @tailwind base 是因为它包含大量 element-level reset (h1~h6,
 *   list-style 等) 会改变 plugin 业务 markup 的渲染语义, 仅注入这两类视觉关键
 *   项足以解决问题, 同时保留 plugin SFC 的语义自由度.
 *
 * 内容来源:
 *   抄自 Tailwind v3 编译产物中 `*, ::before, ::after { ... }` 那一块的
 *   `--tw-*` 默认值, 加 form/border 关键重置.
 *
 * 维护规则:
 *   升级 Tailwind 大版本时需要重新对照新版 preflight 输出, 把新增的
 *   `--tw-*` 变量补进来.
 */
export const TAILWIND_SHADOW_PREFLIGHT = `
*, ::before, ::after {
  border-width: 0;
  border-style: solid;
  border-color: #e5e7eb;
}
button, input, optgroup, select, textarea {
  font-family: inherit;
  font-feature-settings: inherit;
  font-variation-settings: inherit;
  font-size: 100%;
  font-weight: inherit;
  line-height: inherit;
  letter-spacing: inherit;
  color: inherit;
  margin: 0;
  padding: 0;
}
button, [type='button'], [type='reset'], [type='submit'] {
  -webkit-appearance: button;
  background-color: transparent;
  background-image: none;
}
button, [role="button"] {
  cursor: pointer;
}
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
