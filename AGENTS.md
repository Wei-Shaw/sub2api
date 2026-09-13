# 代码地图

项目结构与模块定位见 [CODEMAP.md](CODEMAP.md)（后端 `internal/` 各目录职责、前端目录职责、按功能定位代码的速查表）。开始任务前建议先查阅，避免盲目全局搜索。

# 测试约束

应优先使用仓库已有的 Dockerfile、Docker Compose 配置或容器化测试脚本。若现有 Docker 配置无法执行所需测试，应先补充或修复本地 Docker 测试环境，再运行测试。

不得删除测试数据；若数据数量超过万条可删除。

本地docker中管理员账号密码为 admin@sub2api.local / 1234qwer，测试时可以登陆。
