## ADDED Requirements

### Requirement: 文件暂存 URL 添加素材时移动归档
当 `AddMaterialByUrl` 收到当前 COS `file_uploads` 根目录下的 URL 时，系统 MUST 验证对象属于目标用户，并通过 COS 服务端移动能力将对象迁移到该用户的素材目录。素材记录 SHALL 保存迁移后的 URL 和对象 key，而非暂存 URL。

#### Scenario: 当前用户暂存文件成功归档
- **WHEN** 用户把自己通过文件 API 上传的 URL 传给 `AddMaterialByUrl`
- **THEN** 系统 SHALL 将对象移动到该用户素材目录、删除原暂存对象并登记新的素材 URL 与 key

#### Scenario: 拒绝其他用户暂存文件
- **WHEN** 用户把另一个用户的 `file_uploads` URL 传给 `AddMaterialByUrl`
- **THEN** 系统 SHALL 拒绝请求，且不得移动对象或创建素材记录

#### Scenario: 素材记录写入失败时补偿
- **WHEN** 对象移动成功但素材记录写入失败
- **THEN** 系统 SHALL 尝试将对象移回原暂存 key，并返回错误

#### Scenario: 非暂存 COS URL 保持既有行为
- **WHEN** `AddMaterialByUrl` 收到当前 COS origin 下但不属于 `file_uploads` 的合法 URL
- **THEN** 系统 SHALL 继续按既有逻辑登记 URL，不移动对象
