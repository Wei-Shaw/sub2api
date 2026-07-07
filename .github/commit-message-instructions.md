# Git Commit Message Rules

- 所有 git commit message 必须使用简体中文。
- 严格使用以下格式输出：`<emoji> <type>: <中文简短描述>`
- 只允许使用以下类型与前缀：

  - `✨ feat: 添加新功能`
  - `🐛 fix: 修复 bug`
  - `📝 docs: 对文档进行修改`
  - `♻️ refactor: 代码重构`
  - `⚡ perf: 提高性能的代码修改`
  - `🧑‍💻 dx: 优化开发体验`
  - `🔨 workflow: 工作流变动`
  - `🏷️ types: 类型声明修改`
  - `🚧 wip: 工作进行中`
  - `✅ test: 测试用例添加及修改`
  - `🔨 build: 构建系统或依赖变更`
  - `👷 ci: CI 配置变更`
  - `❓ chore: 其它杂项修改`
  - `⬆️ deps: 依赖项修改`
  - `🔖 release: 发布新版本`

- 只输出最终 commit message，不要附加解释。
- 优先根据本次暂存改动的主要目的选择 type，不要罗列多个 type。
- 描述要短、准、清晰，避免“更新代码”“调整内容”这类空话。
- 默认单行输出。
