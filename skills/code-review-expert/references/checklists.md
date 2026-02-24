# Sub-Agent Review Checklists

5 个子 Agent 的完整检查清单。每个子 Agent 在独立 git worktree 中工作。

---

## Agent 1: Security & Compliance (安全与合规)

### 1.1 Injection (注入漏洞)
- SQL 注入：字符串拼接 SQL、未使用参数化查询
- 命令注入：exec/system/os.Command/subprocess 拼接用户输入
- XSS：未转义的用户输入写入 HTML/DOM
- XXE：XML 解析器未禁用外部实体
- SSRF：用户可控 URL 用于服务端请求，缺少白名单
- LDAP 注入：LDAP 查询拼接用户输入
- SSTI：用户输入直接传入模板引擎
- 路径穿越：文件操作中未校验 `../`
- Header 注入：HTTP 响应头拼接用户输入 (CRLF)
- Log 注入：日志中拼接未净化的用户输入

### 1.2 Authentication & Authorization
- 缺少认证：敏感 API 端点未要求身份验证
- 越权访问：缺少资源归属校验（水平越权）
- 权限提升：普通用户可执行管理员操作（垂直越权）
- 会话管理：Session fixation、不安全 cookie、缺少超时
- JWT：弱签名算法 (none/HS256)、未验证签名、token 泄露
- OAuth：开放重定向、state 缺失、token 存储不安全
- 默认凭证：代码中预设的用户名密码

### 1.3 Secrets & Sensitive Data
- 硬编码密钥：API key、密码、token、连接字符串写在源码
- 密钥泄露：.env 提交版本控制、明文密码
- 日志泄露：敏感数据出现在日志/错误信息中
- API 响应泄露：接口返回超出必要范围的用户数据
- 错误信息泄露：堆栈、内部路径、数据库结构暴露

### 1.4 Cryptography
- 弱哈希：MD5/SHA1 用于密码或安全场景
- 不安全随机数：math/rand 替代 CSPRNG
- ECB 模式：AES-ECB 等不安全加密模式
- 硬编码 IV/Salt
- 缺少完整性校验：加密但未做 HMAC/AEAD

### 1.5 Dependency Security
- 已知漏洞：依赖清单中的 CVE
- 过时依赖：已停止维护的库
- 依赖来源：非官方源、typosquatting
- 许可证合规：GPL 等传染性许可证混入商业项目

### 1.6 Privacy & Data Protection
- PII 未加密存储或传输
- 缺少数据过期/删除机制
- 跨境传输未考虑地域合规

---

## Agent 2: Architecture & Design (架构与设计)

### 2.1 Design Principles
- SRP：类/函数/模块承担过多职责
- OCP：修改核心逻辑而非通过扩展点添加
- LSP：子类/实现违反父类/接口契约
- ISP：接口过大，强迫实现不需要的方法
- DIP：高层模块直接依赖低层实现

### 2.2 Architectural Patterns
- 分层违规：跨层直接调用
- 循环依赖：包/模块间循环引用
- 上帝对象：单类承载过多数据和行为
- 过度抽象：不必要的工厂/策略/装饰器
- 模式误用：强行套用不适合的设计模式
- 配置管理：硬编码环境相关值

### 2.3 API Design
- 一致性：同系统 API 风格不一致
- 向后兼容：破坏性变更未版本控制
- 幂等性：写操作缺少幂等保证
- 批量操作：逐条处理导致 N+1 网络请求
- 分页：大列表缺少分页/游标
- 错误响应：格式不统一、缺少错误码

### 2.4 Error Handling Strategy
- 错误传播：底层错误未包装丢失上下文
- 错误类型：字符串替代结构化错误
- 恢复策略：缺少重试/降级/断路器
- 边界处理：系统边界缺少防御性检查

### 2.5 Module Boundaries
- 接口定义：模块间通过实现而非接口通信
- 数据共享：模块间共享可变数据结构
- 事件/消息：同步调用链过长
- 领域模型：贫血模型、逻辑散落 Service 层

---

## Agent 3: Performance & Resource (性能与资源)

### 3.1 Algorithm & Data Structure
- 热路径上 O(n^2) 或更高复杂度
- 不当数据结构：线性查找替代哈希
- 循环内重复计算
- 不必要的排序/遍历

### 3.2 Database Performance
- N+1 查询：循环内逐条查询
- 缺少索引：WHERE/JOIN 字段未建索引
- 全表扫描
- 大事务持锁过久
- 连接池未配置或配置不当
- SELECT * 替代指定字段

### 3.3 Memory Management
- 内存泄漏：未释放引用、全局缓存无上限
- 循环内创建大对象/切片
- 未使用缓冲 I/O、一次性读取大文件
- 循环内字符串拼接
- 高频对象未使用池化

### 3.4 Concurrency Performance
- 全局锁替代细粒度锁
- 热点资源锁竞争
- 无限制创建 goroutine/线程
- 对只读数据加锁
- 无缓冲通道导致阻塞

### 3.5 I/O Performance
- 异步上下文中阻塞调用
- HTTP 客户端未复用连接
- 大响应未压缩
- 大数据一次性加载替代流式

### 3.6 Caching
- 频繁重复计算/查询未缓存
- 缓存穿透：不存在 key 反复查 DB
- 缓存雪崩：大量 key 同时过期
- 更新后未失效缓存
- 无界缓存导致 OOM

### 3.7 Resource Leaks
- 文件句柄：打开未关闭
- HTTP response body 未关闭
- 数据库查询结果集未关闭
- Timer/Ticker/订阅未取消
- Goroutine/线程启动后永不退出

---

## Agent 4: Reliability & Data Integrity (可靠性与数据完整性)

### 4.1 Error Handling
- 静默吞错：空 catch、忽略返回 error
- 泛型 catch：catch(Exception e)
- 错误消息缺少上下文 (who/what/why)
- 库代码中 panic/os.Exit
- 关键路径缺少 recover/降级

### 4.2 Null Safety
- 空指针解引用：未检查 nil/null
- Optional/Maybe 未正确解包
- 空集合直接取下标
- 长链式调用中环节返回 null

### 4.3 Concurrency Safety
- 数据竞争：无保护读写共享变量
- 死锁：多锁嵌套、不一致加锁顺序
- check-then-act 未加锁
- 非线程安全 Map 并发使用
- 向已关闭 channel 发送数据

### 4.4 Transaction & Consistency
- 多步数据库操作未包裹事务
- 不恰当的事务隔离级别
- 跨服务缺少补偿/Saga
- 异步处理缺少确认/重试
- 重试产生重复数据

### 4.5 Timeout & Retry
- HTTP/DB/RPC 调用未设超时
- 无限重试或缺少退避
- 调用链超时未传递/收缩
- 缺少断路器保护

### 4.6 Boundary Conditions
- 整数溢出：大数、类型截断
- 浮点精度：金额用浮点数
- 时区未明确
- UTF-8 多字节未处理
- 空集合边界
- 并发 first/last、空队列竞态

### 4.7 Graceful Shutdown
- 缺少 SIGTERM/SIGINT 处理
- 关闭时未等待进行中请求
- 未释放 DB 连接、文件句柄
- 内存中待写数据丢失

---

## Agent 5: Code Quality & Observability (代码质量与可观测性)

### 5.1 Complexity
- 函数圈复杂度 > 15
- 深层嵌套 > 4 层
- 函数超过 100 行
- 参数超过 5 个
- 单文件超过 500 行

### 5.2 Duplication
- 大段相似代码 > 10 行
- 相同业务逻辑多处独立实现
- 魔法数字/字符串多处出现

### 5.3 Naming & Readability
- 不符合语言惯例的命名
- 含义模糊：data/info/temp/result
- 同一概念不同命名
- 布尔命名不是 is/has/can/should
- 不通用缩写降低可读性

### 5.4 Dead Code & Tech Debt
- 未调用的函数、未使用的变量/导入
- 被注释的代码块
- TODO/FIXME/HACK 遗留
- 使用 deprecated API

### 5.5 Test Quality
- 关键业务路径缺少测试
- 断言仅检查"不报错"
- 缺少边界和异常路径测试
- 测试间隐式依赖
- 过度 mock
- 依赖时间/网络等外部状态

### 5.6 Logging
- 关键决策点缺少日志
- ERROR 级别用于非错误场景
- 字符串拼接而非结构化日志
- 日志含密码/token/PII
- 热路径过度日志

### 5.7 Observability
- 缺少业务指标（请求量、延迟、错误率）
- 跨服务缺少 trace ID
- 缺少 liveness/readiness 探针
- 关键故障路径缺少告警

### 5.8 Build & Deploy
- 构建结果依赖环境状态
- 缺少 lock 文件
- 开发/生产配置差异未文档化
- 迁移脚本缺少回滚方案
- 大功能上线缺少 feature flag
