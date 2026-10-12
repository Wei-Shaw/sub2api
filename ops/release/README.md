# 唯一发布链路

## 2026-10-09：镜像统一提供前后端

当前主站镜像 `sub2api:custom-v0215-custom-20261009-r3` 已包含后端、内嵌前端及 `/app/.buaga-release` 检查副本。
镜像 ID：`sha256:921f38d417fd45c7c57421b65a010d259be313fffc287b98c51fd2f755b6193d`。
对应源码标签 `rel/buaga-v0215-custom-20261009-r3`。修复白屏只调整了 Nginx，不需要重建或 docker commit 在线容器。

从本次起统一使用版本名 `BUAGA 定制-vX.Y.Z`，Docker 标签使用不含中文的 `sub2api:buaga-custom-vX.Y.Z`。
当前镜像的规范标签为 `sub2api:buaga-custom-v0.2.15`；旧的日期/轮次标签只作为历史别名，不再用于新发布。

Nginx `/assets/` 必须与 HTML 使用同一 `sub2api_backend`，直接代理当前镜像；不得使用独立静态目录。配置模板为本目录 `sub2api-static-assets.conf`。
禁止恢复旧的静态 root/try_files 或另行拷贝资源发布。数据库、上传文件和机密配置继续使用原持久存储，不打进镜像。
后续修改从权威 production 的源码继续；部署和回滚以清单内不可变镜像 ID 为准，不从容器或历史目录反推源码。

每次候选验证及切换后都要从 HTML 提取入口 CSS/JS 并递归验证动态 import 返回 200 且非 HTML 兜底；再用真实浏览器检查页面和控制台。
资源校验脚本 `verify_frontend_http.py` 会检查 HTML 入口资源，并继续检查实际的 JavaScript 动态导入；普通字符串不会被误判为依赖。
任何失败均禁止标记发布完成或清理回滚版本。只测容器健康或候选直连资源不构成公网验收。
当前版本和一个已验证回滚版本均须保留，清理前核查 Nginx 实际引用，禁止删数据库卷。

1. 从 `production` 创建功能分支；不得修改历史快照。
2. 改动后执行定制守卫及与变更相关的测试。清单中的源码锚点是保留检查，不替代行为测试。
3. 审核后合回 `production`，创建不可移动的 `rel/buaga-*` 标签。
4. 在干净工作区执行 `python3 ops/release/package.py <tag> <绝对输出路径.tar.gz>`。
5. 上传包和独立 JSON 清单。服务器运行已安装的 `verify_package.py`，校验 SHA256、Git commit、定制功能，再解包。
6. 主站构建使用 `build-bluegreen.sh`；镜像必须继承当前定制运行时，前端从同一 Git 包重新构建，并使用 `sub2api:buaga-custom-vX.Y.Z` 标签。同一产品版本的修复候选使用不可覆盖现有镜像的 `-rN` 后缀。
7. 镜像统一提供后端、入口 HTML 和全部前端懒加载资源；Nginx `/assets/` 只代理当前镜像，不再单独同步静态目录。
8. 按现有流程隔离测试、验证定制界面和健康检查，再平滑切换。打包或构建不等于上线。

主站切换验收必须包含价格监控容器内的健康检查与管理 API 只读认证检查。
不能只看主站 HTTP 200：Docker 网络别名可能在重建后丢失。
监控的 `SUB2API_API_BASE_URL` 及 `.env` 必须指向实际承接流量的当前容器；
采用容器名绑定时，每次切换须同步更新监控，不能遗留在已退役实例上。

部署后必须更新 `/home/serveradmin/releases/buaga-current.json` 的实际镜像、源码 commit、tag 和源码包校验和。
生产主站与价格监控是独立服务：不能用监控发布标签冒充主站部署记录。

历史 `release.py` / `verify-backend.sh` 仅保留为原发布证据，不得再作为候选创建/构建入口。
服务器 `prepare-custom-upgrade.py --check` 继续校验线上镜像；旧的目录复制方式已禁用。
服务器未安装 Git，权威 Git 操作都在 Mac 完成。

迁移基线保留 R3 原始源码包与其 SHA256，另记录 Git 导出包；二者压缩格式不同，校验和不会相同，但每个源码文件内容一致。
旧目录只读化保留已有未提交修改、测试依赖和数据，不删除文件。
