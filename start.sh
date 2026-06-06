#!/usr/bin/env bash
# sub2api 一键服务控制脚本
#
# 用法:
#   ./start.sh start   [backend|frontend|all]   # 默认 all
#   ./start.sh stop    [backend|frontend|all]
#   ./start.sh restart [backend|frontend|all]
#   ./start.sh status  [backend|frontend|all]
#   ./start.sh logs    [backend|frontend]       # 跟随查看 syslog（fallback 本地日志文件）
#   ./start.sh logs    [backend|frontend] --file # 强制 tail 本地日志文件
#   ./start.sh prod    [info|check]             # 生产环境说明/烟测（默认 info）
#
# 特性:
#   - 启动后与终端解绑（setsid + 重定向 + disown），关闭终端不影响进程。
#   - 热加载：后端用 air（若已安装），前端 vite dev 自带 HMR。
#   - 工具链由 mise 管理：Go / Node / pnpm（Python 项目自动走 uv → poetry，按需启用）。
#   - 标准输出/错误同时落到 本地日志文件 + 系统 syslog（tag=sub2api-<svc>）。
#   - PG / Redis 视为本地已起的基础组件，脚本只做存活探测，不接管其生命周期。

set -euo pipefail

# ============== 基本路径 ==============
PROJECT_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$PROJECT_ROOT/backend"
FRONTEND_DIR="$PROJECT_ROOT/frontend"
RUN_DIR="$PROJECT_ROOT/.run"
mkdir -p "$RUN_DIR"

# 本地 env 覆盖文件（gitignore，放代理/凭据/端口覆盖等）
# 在所有子命令之前 source，所有 KEY=value 自动 export 给 backend/frontend
ENV_LOCAL="$RUN_DIR/env.local"
if [[ -f "$ENV_LOCAL" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_LOCAL"
  set +a
fi

# 默认端口（可被环境变量覆盖）；若端口被占用会自动顺延找下一个空闲端口
BACKEND_PORT="${SERVER_PORT:-8080}"
FRONTEND_PORT="${VITE_DEV_PORT:-3000}"

# 基础组件（本地已起，仅探测）
PG_HOST="${DATABASE_HOST:-127.0.0.1}"
PG_PORT="${DATABASE_PORT:-5432}"
REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
REDIS_PORT="${REDIS_PORT:-6379}"

# ============== 颜色 ==============
if [[ -t 1 ]]; then
  C_R=$'\033[31m'; C_G=$'\033[32m'; C_Y=$'\033[33m'; C_B=$'\033[34m'; C_D=$'\033[2m'; C_0=$'\033[0m'
else
  C_R=""; C_G=""; C_Y=""; C_B=""; C_D=""; C_0=""
fi
info()  { printf "${C_B}[*]${C_0} %s\n" "$*"; }
ok()    { printf "${C_G}[+]${C_0} %s\n" "$*"; }
warn()  { printf "${C_Y}[!]${C_0} %s\n" "$*"; }
err()   { printf "${C_R}[x]${C_0} %s\n" "$*" >&2; }

# ============== mise 封装 ==============
MISE_BIN="${MISE_BIN:-/usr/local/bin/mise}"
if [[ ! -x "$MISE_BIN" ]]; then
  if command -v mise >/dev/null 2>&1; then
    MISE_BIN="$(command -v mise)"
  else
    err "未找到 mise，请先安装：curl https://mise.run | sh"
    exit 1
  fi
fi

# 在项目目录下用 mise 执行命令（自动加载 mise.toml 声明的版本）
# 用法: mise_run <workdir> <cmd...>
mise_run() {
  local wd="$1"; shift
  ( cd "$wd" && "$MISE_BIN" exec -- "$@" )
}

ensure_tools() {
  info "检查 mise 工具链..."
  ( cd "$PROJECT_ROOT" && "$MISE_BIN" install ) >/dev/null
  ok "mise: $(cd "$PROJECT_ROOT" && "$MISE_BIN" current 2>/dev/null | paste -sd' ' -)"
}

# ============== 基础组件探测 ==============
probe_deps() {
  local fail=0
  if (echo > "/dev/tcp/${PG_HOST}/${PG_PORT}") 2>/dev/null; then
    ok "PostgreSQL 可达 ${PG_HOST}:${PG_PORT}"
  else
    warn "PostgreSQL 不可达 ${PG_HOST}:${PG_PORT}（如未启动，请先启动本地 PG）"
    fail=1
  fi
  if (echo > "/dev/tcp/${REDIS_HOST}/${REDIS_PORT}") 2>/dev/null; then
    ok "Redis 可达 ${REDIS_HOST}:${REDIS_PORT}"
  else
    warn "Redis 不可达 ${REDIS_HOST}:${REDIS_PORT}（如未启动，请先启动本地 Redis）"
    fail=1
  fi
  return $fail
}

# ============== PID / 进程辅助 ==============
pidfile()  { echo "$RUN_DIR/$1.pid"; }
portfile() { echo "$RUN_DIR/$1.port"; }
logfile()  { echo "$RUN_DIR/$1.log"; }
syslog_tag() { echo "sub2api-$1"; }

read_pid() {
  local f; f="$(pidfile "$1")"
  [[ -f "$f" ]] || return 1
  local pid; pid="$(<"$f")"
  [[ -n "${pid:-}" ]] || return 1
  kill -0 "$pid" 2>/dev/null || return 1
  echo "$pid"
}

# 检查端口是否被占用
port_in_use() {
  local p="$1"
  ss -ltn 2>/dev/null | awk '{print $4}' | grep -qE ":${p}$"
}

# 从 $1 起步找第一个空闲端口（最多向上找 50 个）
find_free_port() {
  local start="$1"
  local p="$start"
  local i
  for ((i=0; i<50; i++)); do
    if ! port_in_use "$p"; then
      echo "$p"; return 0
    fi
    p=$((p+1))
  done
  echo "$start"  # fallback：返回原值，让上层服务自己报错
  return 1
}

# 给定 PID，递归收集进程子树（用于"看一眼实际跑了啥"）
proc_tree() {
  local root="$1"
  local pids=("$root")
  local children
  children="$(pgrep -P "$root" 2>/dev/null || true)"
  for c in $children; do
    pids+=( "$(proc_tree "$c")" )
  done
  printf '%s\n' "${pids[@]}"
}

# ============== 启动一个守护进程 ==============
# launch <name> <workdir> <log_tag> <cmd...>
# 用 setsid 新建会话/进程组，stdin/stdout/stderr 全部脱钩当前 tty，
# 通过 tee 同时落本地日志和 logger（syslog）。
launch() {
  local name="$1"; shift
  local wd="$1"; shift
  local tag="$1"; shift
  local pf; pf="$(pidfile "$name")"
  local lf; lf="$(logfile "$name")"

  # 用 bash -c 包一层，便于 cd + 管道；setsid 让其成为新会话首进程
  # 关键：</dev/null 切断 stdin；外层 & 后立刻 disown
  setsid bash -c "
    cd '$wd'
    exec > >(tee -a '$lf' | logger -t '$tag') 2>&1
    exec $(printf '%q ' "$@")
  " </dev/null >/dev/null 2>&1 &
  local pid=$!
  disown "$pid" 2>/dev/null || true
  echo "$pid" > "$pf"

  # 短暂等待，确认没有秒退
  sleep 1
  if ! kill -0 "$pid" 2>/dev/null; then
    err "[$name] 启动失败，请查看日志：$lf"
    rm -f "$pf"
    return 1
  fi
  return 0
}

# ============== 后端 ==============
start_backend() {
  if read_pid backend >/dev/null; then
    ok "[backend] 已在运行 (PID $(<"$(pidfile backend)"))"
    return 0
  fi

  # 后端端口被占用时不自动换：很多配置/前端代理都写死了 8080，换了反而更难排查
  if port_in_use "$BACKEND_PORT"; then
    err "[backend] 端口 $BACKEND_PORT 已被占用：$(ss -ltnp 2>/dev/null | awk -v p=":$BACKEND_PORT" '$4 ~ p {print $0; exit}')"
    err "请先释放或设置 SERVER_PORT=<其他端口> ./start.sh start backend"
    return 1
  fi
  echo "$BACKEND_PORT" > "$(portfile backend)"

  # 首次启动：config.yaml 不存在时走 AUTO_SETUP 路径，
  # 自动建库 + 跑迁移 + 创建管理员 + 生成 config.yaml + 写 install lock。
  # 所有变量均可通过宿主 env 覆盖（DATABASE_USER 等）。
  local -a setup_env=()
  if [[ ! -f "$BACKEND_DIR/config.yaml" ]]; then
    info "[backend] 未发现 config.yaml，启用 AUTO_SETUP 自动初始化（建库 + 迁移 + 管理员）"
    setup_env=(
      AUTO_SETUP=true
      DATABASE_HOST="${DATABASE_HOST:-$PG_HOST}"
      DATABASE_PORT="${DATABASE_PORT:-$PG_PORT}"
      DATABASE_USER="${DATABASE_USER:-postgres}"
      DATABASE_PASSWORD="${DATABASE_PASSWORD:-postgres}"
      DATABASE_DBNAME="${DATABASE_DBNAME:-sub2api}"
      DATABASE_SSLMODE="${DATABASE_SSLMODE:-disable}"
      REDIS_HOST="${REDIS_HOST:-$REDIS_HOST}"
      REDIS_PORT="${REDIS_PORT:-$REDIS_PORT}"
      REDIS_PASSWORD="${REDIS_PASSWORD:-}"
      REDIS_DB="${REDIS_DB:-0}"
      ADMIN_EMAIL="${ADMIN_EMAIL:-admin@sub2api.local}"
      ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123456}"
      SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
      SERVER_PORT="$BACKEND_PORT"
      SERVER_MODE="${SERVER_MODE:-debug}"
      TZ="${TZ:-Asia/Shanghai}"
    )
    # 没显式给 JWT_SECRET 就让后端自己随机生成
    [[ -n "${JWT_SECRET:-}" ]] && setup_env+=( JWT_SECRET="$JWT_SECRET" )
  fi

  # 选择启动器：air（热加载，优先，需要 backend/.air.toml） → go run（兜底）
  local launcher_desc
  if [[ -f "$BACKEND_DIR/.air.toml" ]] && { command -v air >/dev/null 2>&1 || [[ -x /data/go/bin/air ]]; }; then
    local air_bin; air_bin="$(command -v air || echo /data/go/bin/air)"
    launcher_desc="air (hot reload)"
    info "[backend] 启动: $launcher_desc，端口 $BACKEND_PORT"
    launch backend "$BACKEND_DIR" "$(syslog_tag backend)" \
      env "${setup_env[@]}" "$MISE_BIN" exec -- "$air_bin"
  else
    launcher_desc="go run ./cmd/server (无 air 或缺 .air.toml，回退)"
    info "[backend] 启动: $launcher_desc，端口 $BACKEND_PORT"
    launch backend "$BACKEND_DIR" "$(syslog_tag backend)" \
      env "${setup_env[@]}" "$MISE_BIN" exec -- go run ./cmd/server
  fi
  ok "[backend] 已启动 (PID $(<"$(pidfile backend)"))  日志: $(logfile backend)"
}

# ============== 前端 ==============
start_frontend() {
  if read_pid frontend >/dev/null; then
    ok "[frontend] 已在运行 (PID $(<"$(pidfile frontend)"))"
    return 0
  fi

  # 前端端口冲突自动顺延（开发服务端口换了影响面小）
  if port_in_use "$FRONTEND_PORT"; then
    local newp; newp="$(find_free_port "$FRONTEND_PORT")"
    warn "[frontend] 端口 $FRONTEND_PORT 已被占用，自动改用 $newp"
    FRONTEND_PORT="$newp"
  fi
  echo "$FRONTEND_PORT" > "$(portfile frontend)"

  # 确保依赖已装（lockfile 模式，避免 CI 漂移）
  if [[ ! -d "$FRONTEND_DIR/node_modules" ]]; then
    info "[frontend] 安装依赖 (pnpm install --frozen-lockfile)..."
    ( cd "$FRONTEND_DIR" && "$MISE_BIN" exec -- pnpm install --frozen-lockfile ) \
      || ( cd "$FRONTEND_DIR" && "$MISE_BIN" exec -- pnpm install )
  fi

  info "[frontend] 启动: pnpm dev (vite HMR)，端口 $FRONTEND_PORT"
  # 关掉 pnpm 11 的 verify-deps-before-run，否则 dev 之前会再跑一次 install
  # 并因 ERR_PNPM_IGNORED_BUILDS 退出 1，导致 vite 根本起不来。
  # vite 走 --port 显式声明；--host 让局域网也可访问，便于联调
  launch frontend "$FRONTEND_DIR" "$(syslog_tag frontend)" \
    env npm_config_verify_deps_before_run=false \
    "$MISE_BIN" exec -- pnpm dev --host --port "$FRONTEND_PORT"
  ok "[frontend] 已启动 (PID $(<"$(pidfile frontend)"))  日志: $(logfile frontend)"
}

# ============== Python 占位（项目当前未使用）==============
start_python() {
  # 留个钩子，后续若引入 Python 子项目可填入：
  #   - 优先 uv:    mise exec -- uv sync && uv run <entry>
  #   - 退化 poetry: mise exec -- poetry install && poetry run <entry>
  warn "本项目当前未使用 Python，跳过。"
}

# ============== 停止 ==============
stop_one() {
  local name="$1"
  local pf; pf="$(pidfile "$name")"
  local pid
  if ! pid="$(read_pid "$name")"; then
    warn "[$name] 未在运行"
    rm -f "$pf"
    return 0
  fi
  info "[$name] 停止 PID=$pid (含子进程)..."
  # 负 PID = 杀整个进程组；setsid 让 PGID == PID
  kill -TERM -"$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
  local i
  for ((i=0; i<20; i++)); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.5
  done
  if kill -0 "$pid" 2>/dev/null; then
    warn "[$name] TERM 超时，发送 KILL"
    kill -KILL -"$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null || true
  fi
  rm -f "$pf"
  ok "[$name] 已停止"
}

# ============== 状态 ==============
status_one() {
  local name="$1"
  local pf; pf="$(pidfile "$name")"
  local pid
  if ! pid="$(read_pid "$name")"; then
    printf "${C_R}[%s] STOPPED${C_0}\n" "$name"
    return 0
  fi
  local port; port="$(cat "$(portfile "$name")" 2>/dev/null || echo '?')"
  local started user cmd
  started="$(ps -o lstart= -p "$pid" 2>/dev/null | sed 's/^ *//')"
  user="$(ps -o user= -p "$pid" 2>/dev/null | sed 's/^ *//')"
  cmd="$(ps -o args= -p "$pid" 2>/dev/null | sed 's/^ *//')"

  # 真实监听端口（看子进程，因为 setsid 包装层不会监听）
  local listen=""
  if command -v ss >/dev/null 2>&1; then
    listen="$(ss -ltnp 2>/dev/null | awk -v p=":$port" '$4 ~ p {print $4; exit}')"
  fi

  printf "${C_G}[%s] RUNNING${C_0}\n" "$name"
  printf "  ${C_D}PID       :${C_0} %s\n" "$pid"
  printf "  ${C_D}声明端口  :${C_0} %s\n" "$port"
  printf "  ${C_D}实际监听  :${C_0} %s\n" "${listen:-（未检测到，可能还在初始化）}"
  printf "  ${C_D}启动时间  :${C_0} %s\n" "$started"
  printf "  ${C_D}启动用户  :${C_0} %s\n" "$user"
  printf "  ${C_D}启动命令  :${C_0} %s\n" "$cmd"
  printf "  ${C_D}进程树    :${C_0}\n"
  pstree -ap "$pid" 2>/dev/null | sed 's/^/    /' \
    || ps -o pid,ppid,user,args --forest -g "$pid" 2>/dev/null | sed 's/^/    /' \
    || true
  printf "  ${C_D}日志文件  :${C_0} %s\n" "$(logfile "$name")"
  printf "  ${C_D}Syslog tag:${C_0} %s  (journalctl -t %s -f)\n" \
    "$(syslog_tag "$name")" "$(syslog_tag "$name")"
}

# ============== 日志 ==============
logs_one() {
  local name="$1"; shift || true
  local mode="${1:-auto}"
  local tag; tag="$(syslog_tag "$name")"
  local lf; lf="$(logfile "$name")"

  case "$mode" in
    --file|file)
      info "tail 本地日志: $lf  (Ctrl-C 退出)"
      exec tail -F -n 200 "$lf"
      ;;
    *)
      if command -v journalctl >/dev/null 2>&1; then
        info "journalctl -t $tag -f  (无权限时自动回退本地日志)"
        # 先做一次性 dry-run 探一下权限；不行就 fallback
        if journalctl -t "$tag" -n 0 --no-pager >/dev/null 2>&1; then
          exec journalctl -t "$tag" -f -n 200 --no-pager
        else
          warn "journalctl 无权限读取 syslog（需 adm/systemd-journal 组）。回退本地文件。"
          info "如需 syslog 视图，请: sudo usermod -aG systemd-journal $USER  （重登生效）"
          exec tail -F -n 200 "$lf"
        fi
      else
        exec tail -F -n 200 "$lf"
      fi
      ;;
  esac
}

# ============== 子命令分发 ==============
TARGETS_DEFAULT=(backend frontend)

expand_targets() {
  local t="${1:-all}"
  case "$t" in
    all|"")    printf '%s\n' "${TARGETS_DEFAULT[@]}" ;;
    backend|frontend|python) echo "$t" ;;
    *) err "未知目标: $t (可选: backend | frontend | all)"; exit 2 ;;
  esac
}

cmd_start() {
  ensure_tools
  probe_deps || warn "基础组件探测有失败项，仍继续启动（如服务自检失败请检查依赖）"
  local t
  for t in $(expand_targets "${1:-all}"); do
    case "$t" in
      backend)  start_backend ;;
      frontend) start_frontend ;;
      python)   start_python ;;
    esac
  done
}

cmd_stop() {
  local t
  for t in $(expand_targets "${1:-all}"); do
    stop_one "$t"
  done
}

cmd_status() {
  local t
  for t in $(expand_targets "${1:-all}"); do
    status_one "$t"
  done
}

cmd_restart() {
  cmd_stop "${1:-all}"
  sleep 1
  cmd_start "${1:-all}"
}

cmd_logs() {
  local t="${1:-}"
  if [[ -z "$t" || "$t" == "all" ]]; then
    err "logs 需要指定单个目标：./start.sh logs backend | frontend"
    exit 2
  fi
  shift
  logs_one "$t" "${1:---auto}"
}

# ============== 生产环境探活 ==============
# 配置（可被环境变量覆盖，方便切到 staging / 不同机房）
PROD_API_HOST="${PROD_API_HOST:-10.36.32.221}"
PROD_API_PORT="${PROD_API_PORT:-8001}"
PROD_WEB_HOST="${PROD_WEB_HOST:-10.36.32.221}"
PROD_WEB_PORT="${PROD_WEB_PORT:-3001}"
PROD_PUBLIC_URL="${PROD_PUBLIC_URL:-https://sub2api.p1.cn}"

# 单条 check 用同一个工具栈以保持输出整齐
__prod_pass=0
__prod_fail=0
__prod_warn=0

_p_pass() { printf "  ${C_G}✓${C_0} %s\n" "$*"; __prod_pass=$((__prod_pass+1)); }
_p_fail() { printf "  ${C_R}✗${C_0} %s\n" "$*"; __prod_fail=$((__prod_fail+1)); }
_p_warn() { printf "  ${C_Y}!${C_0} %s\n" "$*"; __prod_warn=$((__prod_warn+1)); }
_p_kv()   { printf "    ${C_D}%s${C_0} %s\n" "$1" "$2"; }

# TCP 探活: _p_tcp <label> <host> <port>
_p_tcp() {
  local label="$1" host="$2" port="$3"
  if timeout 3 bash -c "echo > /dev/tcp/$host/$port" 2>/dev/null; then
    _p_pass "$label  TCP $host:$port"
  else
    _p_fail "$label  TCP $host:$port 不通（超时或被拒）"
    return 1
  fi
}

# HTTP 检查: _p_http <label> <url> <expect_code> [<expect_body_grep_pattern>]
# expect_code 支持: 200 | 2xx | 3xx | 4xx | 401|302（多个允许码竖线分隔）
_p_http() {
  local label="$1" url="$2" expect="$3" needle="${4:-}"
  local tmp err; tmp="$(mktemp)"; err="$(mktemp)"
  local out; out="$(curl -sS -o "$tmp" -w '%{http_code}|%{time_total}|%{size_download}' \
    --max-time 10 -k "$url" 2>"$err")" || true
  local code time size
  IFS='|' read -r code time size <<< "$out"
  local pass=0
  if [[ "$code" == "000" || -z "$code" ]]; then
    _p_fail "$label  $url  ← 连接失败 / 超时"
    _p_kv "↳ curl" "$(head -c 200 "$err" | tr '\n' ' ')"
    rm -f "$tmp" "$err"; return 1
  fi
  # 期望码匹配
  if [[ ",${expect//|/,}," == *",${code},"* ]]; then
    pass=1
  elif [[ "$expect" == "2xx" && "$code" =~ ^2 ]]; then pass=1
  elif [[ "$expect" == "3xx" && "$code" =~ ^3 ]]; then pass=1
  elif [[ "$expect" == "4xx" && "$code" =~ ^4 ]]; then pass=1
  fi
  if (( pass == 0 )); then
    _p_fail "$label  HTTP $code（期望 $expect）  ${time}s  $url"
    _p_kv "↳ body" "$(head -c 200 "$tmp" | tr '\n' ' ')"
    rm -f "$tmp" "$err"; return 1
  fi
  # body 关键字校验
  if [[ -n "$needle" ]] && ! grep -q "$needle" "$tmp"; then
    _p_warn "$label  HTTP $code 但 body 未含 \"$needle\"  ${time}s"
    _p_kv "↳ body" "$(head -c 200 "$tmp" | tr '\n' ' ')"
    rm -f "$tmp" "$err"; return 0
  fi
  _p_pass "$label  HTTP $code  ${time}s  ${size}B"
  rm -f "$tmp" "$err"
}

prod_info() {
  cat <<EOF
${C_B}sub2api 生产环境${C_0}

公网入口（需经过公司网络 / VPN）：
  Web   ${C_G}https://sub2api.p1.cn${C_0}        ← 需 OSS 认证（浏览器登录公司 SSO）
  API   ${C_G}https://sub2api.p1.cn/api${C_0}    ← 无认证，curl / SDK 直接打

内网直连（绕过 Nginx + OSS，调试 / CI 用）：
  Backend  http://${PROD_API_HOST}:${PROD_API_PORT}   ← systemd: infra-sub2api-rest
  Frontend http://${PROD_WEB_HOST}:${PROD_WEB_PORT}   ← 本机 Nginx 服务静态文件

链路：浏览器 → sub2api.p1.cn (443) → OSS 网关 → Nginx
                                                ├── /        → ${PROD_WEB_HOST}:${PROD_WEB_PORT}  (静态)
                                                └── /api/*   → ${PROD_API_HOST}:${PROD_API_PORT}  (Go 后端)

部署目录（目标机上）：/app/infra-sub2api-rest/current/
配置文件                /app/infra-sub2api-rest/current/conf/config.yaml
敏感值                  /etc/sub2api/secrets.env  (EnvironmentFile)
systemd 单元           infra-sub2api-rest.service
日志                    journalctl -u infra-sub2api-rest -f

更多见 conf/README.md
EOF
}

prod_check() {
  __prod_pass=0; __prod_fail=0; __prod_warn=0
  printf "${C_B}sub2api 生产环境烟测${C_0}  %s\n\n" "$(date -Iseconds)"

  printf "${C_B}[1] 内网直连后端 (%s:%s)${C_0}\n" "$PROD_API_HOST" "$PROD_API_PORT"
  _p_tcp  "TCP "                                "$PROD_API_HOST" "$PROD_API_PORT" || true
  _p_http "/health           " "http://$PROD_API_HOST:$PROD_API_PORT/health"               "200" || true
  _p_http "/api/v1/settings/public" "http://$PROD_API_HOST:$PROD_API_PORT/api/v1/settings/public" "200" '"code":' || true
  _p_http "/api/v1/admin/* (未授权应 401)" "http://$PROD_API_HOST:$PROD_API_PORT/api/v1/admin/accounts" "401|403" || true
  echo

  printf "${C_B}[2] 内网直连前端 (%s:%s)${C_0}\n" "$PROD_WEB_HOST" "$PROD_WEB_PORT"
  _p_tcp  "TCP "                                "$PROD_WEB_HOST" "$PROD_WEB_PORT" || true
  _p_http "/                 " "http://$PROD_WEB_HOST:$PROD_WEB_PORT/" "200" '<div id="app"' || true
  echo

  printf "${C_B}[3] 公网 API 入口 (%s)${C_0}\n" "$PROD_PUBLIC_URL"
  if command -v dig >/dev/null 2>&1; then
    local host="${PROD_PUBLIC_URL#https://}"; host="${host%%/*}"
    local ip; ip="$(dig +short +time=3 +tries=1 "$host" | tail -1)"
    if [[ -n "$ip" ]]; then _p_pass "DNS  $host → $ip"; else _p_fail "DNS 解析失败 $host"; fi
  fi
  _p_http "/api/v1/settings/public (不走 OSS)" "$PROD_PUBLIC_URL/api/v1/settings/public" "200" '"code":' || true
  _p_http "/api/v1/admin/* (无 token 应 401)" "$PROD_PUBLIC_URL/api/v1/admin/accounts" "401|403" || true
  echo

  printf "${C_B}[4] 公网 Web 入口 (OSS 拦截预期)${C_0}\n"
  # OSS 通常 302 跳转到登录页，或返回 401；200 直出前端就是 OSS 没生效 → 警告
  _p_http "/   (期望 302/401)" "$PROD_PUBLIC_URL/" "302|401|307|303" || true
  echo

  printf "${C_B}汇总${C_0}: "
  printf "${C_G}%d pass${C_0}  " "$__prod_pass"
  (( __prod_warn > 0 )) && printf "${C_Y}%d warn${C_0}  " "$__prod_warn"
  if (( __prod_fail > 0 )); then
    printf "${C_R}%d fail${C_0}\n" "$__prod_fail"
    echo
    echo "排查建议："
    echo "  - TCP 不通  → 在生产机本地 \`ss -ltn | grep -E ':800[01]|:3001'\` 看是否在监听；防火墙 / SG 规则"
    echo "  - /health 200 但 /api/v1/settings/public 500 → DB / Redis 未连上，journalctl -u infra-sub2api-rest -n 100"
    echo "  - 公网 /api 失败但内网直连成功 → Nginx upstream / OSS 配置；nginx -T 看 location /api"
    echo "  - 公网 / 返回 200 而不是 302 → OSS 没拦截到本站，登录认证形同虚设，立即检查 OSS 接入"
    return 1
  fi
  printf "${C_G}all ok${C_0}\n"
}

cmd_prod() {
  local sub="${1:-info}"
  case "$sub" in
    info|help|"")  prod_info ;;
    check|test)    prod_check ;;
    *) err "未知 prod 子命令: $sub（支持 check | info）"; exit 2 ;;
  esac
}

usage() {
  sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
}

main() {
  local action="${1:-}"; shift || true
  case "$action" in
    start)   cmd_start   "${1:-all}" ;;
    stop)    cmd_stop    "${1:-all}" ;;
    restart) cmd_restart "${1:-all}" ;;
    status)  cmd_status  "${1:-all}" ;;
    logs)    cmd_logs    "$@" ;;
    prod)    cmd_prod    "$@" ;;
    ""|-h|--help|help) usage ;;
    *) err "未知命令: $action"; usage; exit 2 ;;
  esac
}

main "$@"
