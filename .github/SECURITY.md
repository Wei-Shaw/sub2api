# Security Policy

[English](#english) | [中文](#中文)

## English

### Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, pull requests, or discussions.**

Report them privately through GitHub's private vulnerability reporting:

**[Report a vulnerability](https://github.com/Wei-Shaw/sub2api/security/advisories/new)**

(Repository → **Security** tab → **Report a vulnerability**.) Only the maintainers can see the report, and the advisory becomes the place to discuss, fix, and eventually publish the issue.

If you cannot use GitHub private vulnerability reporting, email **dev@sub2api.org** instead, with `[Security]` in the subject line.

A useful report includes:

- The affected component and version (release tag or commit)
- Impact: what an attacker can do and which privileges they need
- Minimal reproduction steps or a proof of concept
- Any non-default configuration required to trigger the issue
- A suggested fix, if you have one

### Supported Versions

Sub2API releases frequently, and fixes ship in a new release rather than being backported.

| Version | Supported |
| ------- | --------- |
| Latest release | ✅ |
| Older releases | ❌ |

Please confirm that the issue reproduces on the latest release or on `main` before reporting.

### Scope

In scope: the Sub2API backend, frontend, official Docker images, and deployment files in this repository.

Usually out of scope:

- Issues that require already-compromised admin credentials or server access
- Risks arising from explicitly weakening documented security settings (for example, disabling `security.url_allowlist`)
- Vulnerabilities in upstream AI providers or third-party dependencies with no Sub2API-specific impact (please report those upstream)
- Missing best-practice headers or scanner output without a demonstrated impact

### Disclosure Process

1. We acknowledge the report and assess its validity and severity.
2. We develop a fix privately in the advisory and may ask you to verify it.
3. We publish a release containing the fix, then publish the GitHub Security Advisory (with a CVE when applicable), crediting you unless you prefer to stay anonymous.

Please keep details private until the advisory is published. This is a volunteer-maintained project, so response times are best effort.

## 中文

### 报告漏洞

**请不要通过公开的 Issue、Pull Request 或 Discussion 报告安全漏洞。**

请使用 GitHub 私密漏洞报告功能提交：

**[提交漏洞报告](https://github.com/Wei-Shaw/sub2api/security/advisories/new)**

（仓库 → **Security** 标签页 → **Report a vulnerability**。）报告仅维护者可见，后续的讨论、修复与公告发布都在该安全公告中进行。

如果无法使用 GitHub 私密漏洞报告，也可以发送邮件至 **dev@sub2api.org**，邮件标题请注明 `[Security]`。

报告中建议包含：

- 受影响的组件与版本（Release tag 或 commit）
- 影响：攻击者能做什么、需要什么权限
- 最小复现步骤或 PoC
- 触发问题所需的非默认配置
- 修复建议（如有）

### 支持的版本

Sub2API 发版频繁，安全修复随新版本发布，不向旧版本回移。

| 版本 | 是否支持 |
| ---- | -------- |
| 最新 Release | ✅ |
| 更早的版本 | ❌ |

报告前请先确认问题在最新 Release 或 `main` 上仍可复现。

### 范围

范围内：本仓库中的 Sub2API 后端、前端、官方 Docker 镜像与部署文件。

通常不在范围内：

- 需要事先已掌握管理员凭据或服务器权限才能利用的问题
- 因主动关闭文档中的安全配置（如关闭 `security.url_allowlist`）而产生的风险
- 上游 AI 服务商或第三方依赖自身的漏洞，且对 Sub2API 无特定影响（请向上游报告）
- 仅缺少最佳实践响应头或扫描器输出，但未证明实际影响

### 披露流程

1. 我们确认收到报告，并评估其有效性与严重程度。
2. 我们在安全公告中私下开发修复，可能请你协助验证。
3. 我们先发布包含修复的版本，再公开 GitHub 安全公告（适用时申请 CVE），并致谢报告者（如你希望匿名则不署名）。

在公告发布前请勿公开漏洞细节。本项目由志愿者维护，响应时间尽力而为。
