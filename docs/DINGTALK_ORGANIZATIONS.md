# 钉钉多应用、组织负责人和额度分配

入口：侧栏 **钉钉组织**（`/organization/dingtalk`）。

## 角色与额度规则

- 系统现有的全局管理员（`admin`）承担超级管理员职责，可以维护钉钉应用、同步组织、指定负责人和调整最大可分配额度。
- 部门负责人使用普通用户账号，无需授予全局管理员权限。可以负责多个应用中的多个部门，权限包含子部门，不能给自己或全局管理员分配额度。
- 每位负责人有一个跨应用、跨部门共用的累计预算。例如总额度 1,000，已经分配 300，则还可分配 700。**0 表示不能分配，不是无限额度**。
- 增加额度会增加成员现有账户余额，单位与平台余额一致（USD，支持两位小数），不覆盖余额，也不改变 API Key 限额或平台每日/每月限额。
- 最大额度不能低于已分配金额。撤销负责人、调整负责部门、停用应用均不会重置已分配金额。需要追加预算时，由全局管理员增加总额度。
- 每笔额度分配均留下持久化记录。预算扣减、余额增加和记录写入在同一 PostgreSQL 事务内完成；请求 ID 防止超时重试导致重复入账。

## 配置步骤

1. 以全局管理员进入「钉钉组织」，展开「钉钉应用配置」，填写应用标识、名称、Client ID、Client Secret 和回调地址。
2. 使用钉钉**企业内部应用**。在钉钉开发者后台配置登录回调地址，例如 `https://your-domain/api/v1/auth/oauth/dingtalk/callback`。同一个回调端点支持多个应用，OAuth state 会绑定所选应用。
3. 为每个应用开通用户登录、企业应用 access token、部门详情/子部门列表和部门成员读取权限，并配置需要管理的通讯录可见范围。同步使用 `/topapi/v2/department/get`、`/topapi/v2/department/listsub`、`/topapi/v2/user/list`，登录也需要通过 union ID 查询员工的权限。
4. 保存应用，选择该应用，点击「同步钉钉组织」。部门父子关系和成员来自钉钉，平台不允许自行编辑组织树。所有部门和成员分页读取成功后才会替换旧快照，失败不会提交部分数据。
5. 用户通过对应钉钉应用登录，或者在「钉钉组织」的「绑定我的钉钉应用」入口，将钉钉身份绑定到已有平台账号。仅使用经过 OAuth 验证的身份关联成员，**不会根据姓名、邮箱或可编辑用户属性自动授予组织权限**。
6. 管理员点击「添加负责人」，填写其平台用户 ID、最大累计可分配额度，并勾选所管部门。再次编辑可以增加其它应用的部门权限，原有应用权限会保留。
7. 负责人进入相同页面，选择所管部门与成员，点击「增加额度」，填写金额并确认。

## 同步与兼容

- 组织同步是管理员手动触发的。为避免长期使用离职、调岗之前的关系，额度分配要求快照在 **24 小时内**完成同步；过期后需先重新同步。同步后的成员移除、部门移动会影响后续分配权限。
- 一个成员可以属于多个部门；不同应用的部门 ID、身份和负责人授权相互隔离。
- 原来的单应用系统设置保持有效，在组织页面显示为「默认钉钉应用」。原有身份标识和登录路径不变；新增应用使用独立身份命名空间。
- 新增应用遵守系统注册开关，不继承默认应用的注册豁免。绑定已有账号可使用「绑定我的钉钉应用」入口。
- 额外应用的标识、Client ID 和企业 ID 保存后不可修改，也不可删除；可以停用并添加另一个应用。这避免新应用继承旧组织的权限和成员。Client Secret 可以轮换，编辑时留空保留原值，读取接口不返回密钥。
- 全局管理员仍保有原有的用户管理与余额调整权限；不要将普通部门负责人提升为全局管理员。

## 数据与接口

服务启动会自动执行 `239_dingtalk_organizations.sql` 迁移。应用配置存于现有 `settings` 表的 `dingtalk_connect_apps`，组织快照、成员、负责人预算、部门授权和分配流水存于新增的 `dingtalk_*` 表。

管理员接口以 `/api/v1/admin/dingtalk` 为前缀：

| 方法 / 路径 | 功能 |
| --- | --- |
| GET / PUT `/apps` | 读取 / 保存额外应用配置 |
| POST `/apps/:app/sync` | 同步组织与成员 |
| GET `/apps/:app/directory` | 部门与成员 |
| GET / PUT `/managers` | 读取 / 保存负责人和累计预算 |
| GET / POST `/grants` | 最近 100 笔记录 / 分配额度 |

普通负责人通过 `/api/v1/organization/dingtalk` 的对应读取接口和 `POST /grants` 操作。服务端强制按当前用户检查部门权限和累计预算。普通负责人不能修改应用或授权。

## 开发验证

```sh
cd backend
# 可选：提供独立 PostgreSQL 测试数据库，测试在独立 schema 内验证真实事务。
DINGTALK_TEST_DATABASE_URL='postgres://postgres:password@localhost:5432/test?sslmode=disable' \
  go test -tags unit ./internal/config ./internal/service ./internal/handler -run 'DingTalk|Dingtalk'

cd ../frontend
./node_modules/.bin/vue-tsc --noEmit
./node_modules/.bin/vitest run src/views/user/__tests__/DingTalkOrganizationView.spec.ts src/components/auth/__tests__/OAuthLoginSections.spec.ts
```
