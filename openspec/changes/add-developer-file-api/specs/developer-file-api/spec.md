## ADDED Requirements

### Requirement: 开发者密钥文件上传
系统 SHALL 在 `POST /api/v1/file/` 接受开发者密钥 Bearer 鉴权和 `multipart/form-data` 的 `file` 字段，将非空且不超过 512 MiB 的文件流式上传到 COS，并返回公开访问 URL、文件大小和内容类型。

#### Scenario: 上传文件成功
- **WHEN** 调用方使用有效开发者密钥上传符合限制的文件
- **THEN** 系统 SHALL 将文件写入该密钥所属用户的暂存目录并返回可访问 URL

#### Scenario: 密钥无效
- **WHEN** 调用方未提供开发者密钥、密钥格式无效或密钥已删除
- **THEN** 系统 SHALL 返回未认证且不得写入对象存储

#### Scenario: 文件不符合限制
- **WHEN** 上传缺少文件、文件为空或文件超过 512 MiB
- **THEN** 系统 SHALL 返回客户端错误且不得保留不完整对象

### Requirement: 暂存对象目录约束
文件 API 写入的每个对象 MUST 位于 COS 配置 prefix 下的 `file_uploads` 根目录，并位于根据密钥所属用户派生的不可枚举子目录。客户端提供的文件名 MUST NOT 改变该目录边界。

#### Scenario: 配置了 COS prefix
- **WHEN** COS prefix 为 `assets/` 且用户上传文件
- **THEN** 对象 key SHALL 以 `assets/file_uploads/<opaque-user-prefix>/` 开头

#### Scenario: 恶意文件名不能穿越目录
- **WHEN** 上传文件名包含 `../`、绝对路径或路径分隔符
- **THEN** 系统 SHALL 忽略这些路径成分并生成服务端随机对象名

### Requirement: 开发者密钥文件删除
系统 SHALL 在 `DELETE /api/v1/file/` 接受 JSON `url`，并且只删除该开发者密钥所属用户在 `file_uploads` 中的暂存对象。

#### Scenario: 删除自己的暂存文件
- **WHEN** 调用方使用有效开发者密钥提交自己上传文件的 URL
- **THEN** 系统 SHALL 删除对应 COS 对象并返回成功

#### Scenario: 拒绝跨用户或非暂存对象
- **WHEN** URL 指向其他用户暂存目录、素材目录、其他 COS 对象或非当前 COS origin
- **THEN** 系统 SHALL 拒绝删除且不得修改任何对象
