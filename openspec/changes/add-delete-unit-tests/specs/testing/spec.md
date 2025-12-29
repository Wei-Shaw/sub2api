## ADDED Requirements
### Requirement: Delete path unit coverage
服务层删除流程 SHALL 具备单元测试覆盖用户、分组、代理、兑换码等资源的关键分支，且覆盖 AdminService 删除入口的权限保护、幂等删除与错误传播。

#### Scenario: User delete success
- **WHEN** 删除存在的用户
- **THEN** 返回成功且仓储删除被调用

#### Scenario: User delete not found
- **WHEN** 删除不存在的用户
- **THEN** 返回未找到错误

#### Scenario: User delete propagates errors
- **WHEN** 删除用户时仓储返回错误
- **THEN** 错误被向上返回且不吞掉

#### Scenario: User delete rejects admin accounts
- **WHEN** 删除管理员用户
- **THEN** 返回拒绝删除的错误

#### Scenario: Group delete success
- **WHEN** 删除存在的分组
- **THEN** 返回成功且仓储级联删除被调用

#### Scenario: Group delete not found
- **WHEN** 删除不存在的分组
- **THEN** 返回 ErrGroupNotFound

#### Scenario: Group delete propagates errors
- **WHEN** 删除分组时仓储返回错误
- **THEN** 错误被向上返回且不吞掉

#### Scenario: Proxy delete success
- **WHEN** 删除存在的代理
- **THEN** 返回成功且仓储删除被调用

#### Scenario: Proxy delete is idempotent
- **WHEN** 删除不存在的代理
- **THEN** 不返回错误且调用删除流程

#### Scenario: Proxy delete propagates errors
- **WHEN** 删除代理时仓储返回错误
- **THEN** 错误被向上返回且不吞掉

#### Scenario: Redeem code delete success
- **WHEN** 删除存在的兑换码
- **THEN** 返回成功且仓储删除被调用

#### Scenario: Redeem code delete is idempotent
- **WHEN** 删除不存在的兑换码
- **THEN** 不返回错误且调用删除流程

#### Scenario: Redeem code delete propagates errors
- **WHEN** 删除兑换码时仓储返回错误
- **THEN** 错误被向上返回且不吞掉

#### Scenario: Batch redeem code delete success
- **WHEN** 批量删除兑换码且全部成功
- **THEN** 返回删除数量等于输入数量且不返回错误

#### Scenario: Batch redeem code delete partial failures
- **WHEN** 批量删除兑换码且部分失败
- **THEN** 返回删除数量小于输入数量且不返回错误
