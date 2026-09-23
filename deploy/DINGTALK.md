# 企业钉钉首次登录自动注册

已配置钉钉 OAuth 的企业内部应用可选择开启首次登录自动注册。默认关闭，不改变现有注册和绑定流程。

在后端 `config.yaml` 的现有钉钉配置中添加：

```yaml
dingtalk_connect:
  enabled: true
  dingtalk_app_kind: internal_app
  app_type: internal
  corp_restriction_policy: internal_only
  require_email: true
  auto_register: true
  # 保留已有的 client_id、client_secret 和回调地址等配置。
```

也可向后端进程传入环境变量 `DINGTALK_CONNECT_AUTO_REGISTER=true`。Docker Compose 部署需将该变量显式加入服务的 `environment`，仅写入未被 Compose 引用的 `.env` 不会传给容器。修改后重启后端服务。

钉钉应用需要有通过 UnionID 查询企业成员及读取成员企业邮箱的权限。自动注册只信任企业通讯录返回的 `org_email`，不使用个人邮箱或扩展属性邮箱替代。管理员应确保企业邮箱的归属和唯一性。

登录行为：

- 已绑定的钉钉身份继续登录对应账号。
- 未绑定的企业成员有 `org_email` 且邮箱未占用时，自动创建普通用户、绑定钉钉身份并登录，无需手动设置本地密码或再次验证企业邮箱。
- 新用户使用系统生成的随机密码，并沿用钉钉来源的默认注册权益。之后可继续用钉钉登录；需要本地密码时可走现有密码找回流程。
- 企业邮箱已对应本地账号时，进入现有账号选择和验证绑定流程，不根据邮箱直接登录或自动绑定已有账号（包括管理员）。
- 未开启开关、不是内部应用、没有企业邮箱、要求第三方注册强制填写邮箱或开启邀请码时，沿用原有流程。

该开关不绕过注册策略。关闭注册时仍需现有的 `bypass_registration` 策略允许钉钉注册；邮箱域名限制及后端运行模式限制继续生效。企业成员校验失败时拒绝登录。

绑定已有账号时，如果本地邮箱或密码错误，回调页会保留并显示错误，不再因绑定接口的 HTTP 401 自动跳回登录页或刷新应用令牌。
