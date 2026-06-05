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
  C_R="\033[31m"; C_G="\033[32m"; C_Y="\033[33m"; C_B="\033[34m"; C_D="\033[2m"; C_0="\033[0m"
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
    ""|-h|--help|help) usage ;;
    *) err "未知命令: $action"; usage; exit 2 ;;
  esac
}

main "$@"
