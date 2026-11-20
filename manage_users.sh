#!/usr/bin/env bash
# sub2api 用户管理脚本
# 用法见下方 usage() 函数

set -euo pipefail

HOST="http://localhost:8080"
ADMIN_EMAIL=""
ADMIN_PASSWORD=""

usage() {
  cat <<EOF
用法: $0 [全局选项] <子命令> [子命令选项]

全局选项:
  --host        服务地址 (默认: http://localhost:8080)
  --admin-email    管理员邮箱
  --admin-password 管理员密码

子命令:
  create        创建新用户
    --email     新用户邮箱 (必填)
    --password  新用户密码 (必填, 至少6位)
    --username  用户名 (可选)
    --balance   初始余额 (可选)

  set-password  由管理员重置指定用户的密码
    --email     目标用户邮箱 (必填)
    --new-password 新密码 (必填, 至少6位)

  change-password  用户修改自己的密码
    --email        用户邮箱 (必填)
    --old-password 旧密码 (必填)
    --new-password 新密码 (必填, 至少6位)

示例:
  # 创建用户
  $0 --host http://localhost:8080 --admin-email admin@example.com --admin-password adminpass \\
     create --email user@example.com --password mypassword

  # 管理员重置用户密码
  $0 --admin-email admin@example.com --admin-password adminpass \\
     set-password --email user@example.com --new-password newpassword

  # 用户修改自己密码
  $0 change-password --email user@example.com --old-password old123 --new-password new123
EOF
}

# 检查依赖
check_deps() {
  if ! command -v curl &>/dev/null; then
    echo "错误: 需要安装 curl" >&2; exit 1
  fi
  if ! command -v jq &>/dev/null; then
    echo "错误: 需要安装 jq" >&2; exit 1
  fi
}

# 登录并返回 access_token
login() {
  local email="$1" password="$2"
  local resp
  resp=$(curl -s -X POST "${HOST}/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":$(jq -rn --arg v "$email" '$v | @json'),\"password\":$(jq -rn --arg v "$password" '$v | @json')}")
  local token
  token=$(echo "$resp" | jq -r '.data.access_token // empty')
  if [[ -z "$token" ]]; then
    echo "登录失败:" >&2
    echo "$resp" | jq . >&2
    exit 1
  fi
  echo "$token"
}

# 根据邮箱查找用户 ID
find_user_id_by_email() {
  local token="$1" email="$2"
  local encoded_email
  encoded_email=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$email" 2>/dev/null || printf '%s' "$email")
  local resp
  resp=$(curl -s -X GET "${HOST}/api/v1/admin/users?search=${encoded_email}" \
    -H "Authorization: Bearer ${token}")
  local user_id
  user_id=$(echo "$resp" | jq -r --arg email "$email" \
    '.data.items[] | select(.email == $email) | .id // empty' | head -1)
  if [[ -z "$user_id" ]]; then
    echo "未找到邮箱为 ${email} 的用户" >&2
    echo "$resp" | jq . >&2
    exit 1
  fi
  echo "$user_id"
}

cmd_create() {
  local email="" password="" username="" balance=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --email)        email="$2";    shift 2 ;;
      --password)     password="$2"; shift 2 ;;
      --username)     username="$2"; shift 2 ;;
      --balance)      balance="$2";  shift 2 ;;
      *) echo "未知选项: $1" >&2; usage; exit 1 ;;
    esac
  done
  [[ -z "$email" ]]    && { echo "错误: --email 必填" >&2; exit 1; }
  [[ -z "$password" ]] && { echo "错误: --password 必填" >&2; exit 1; }
  [[ -z "$ADMIN_EMAIL" || -z "$ADMIN_PASSWORD" ]] && { echo "错误: 请提供 --admin-email 和 --admin-password" >&2; exit 1; }

  echo "正在以管理员身份登录..."
  local token
  token=$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD")

  # 构造 JSON body
  local body
  body=$(jq -n \
    --arg email "$email" \
    --arg password "$password" \
    --arg username "$username" \
    '{email: $email, password: $password} |
     if $username != "" then . + {username: $username} else . end')
  if [[ -n "$balance" ]]; then
    body=$(echo "$body" | jq --argjson bal "$balance" '. + {balance: $bal}')
  fi

  echo "正在创建用户 ${email}..."
  local resp
  resp=$(curl -s -X POST "${HOST}/api/v1/admin/users" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "$body")
  local created_email
  created_email=$(echo "$resp" | jq -r '.data.email // empty')
  if [[ -n "$created_email" ]]; then
    echo "用户创建成功:"
    echo "$resp" | jq '.data | {id, email, username, role, status}'
  else
    echo "创建失败:" >&2
    echo "$resp" | jq . >&2
    exit 1
  fi
}

cmd_set_password() {
  local email="" new_password=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --email)        email="$2";        shift 2 ;;
      --new-password) new_password="$2"; shift 2 ;;
      *) echo "未知选项: $1" >&2; usage; exit 1 ;;
    esac
  done
  [[ -z "$email" ]]        && { echo "错误: --email 必填" >&2; exit 1; }
  [[ -z "$new_password" ]] && { echo "错误: --new-password 必填" >&2; exit 1; }
  [[ -z "$ADMIN_EMAIL" || -z "$ADMIN_PASSWORD" ]] && { echo "错误: 请提供 --admin-email 和 --admin-password" >&2; exit 1; }

  echo "正在以管理员身份登录..."
  local token
  token=$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD")

  echo "正在查找用户 ${email}..."
  local user_id
  user_id=$(find_user_id_by_email "$token" "$email")
  echo "找到用户 ID: ${user_id}"

  local body
  body=$(jq -n --arg pwd "$new_password" '{password: $pwd}')

  echo "正在更新密码..."
  local resp
  resp=$(curl -s -X PUT "${HOST}/api/v1/admin/users/${user_id}" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "$body")
  local updated_email
  updated_email=$(echo "$resp" | jq -r '.data.email // empty')
  if [[ -n "$updated_email" ]]; then
    echo "密码更新成功 (用户: ${updated_email})"
  else
    echo "更新失败:" >&2
    echo "$resp" | jq . >&2
    exit 1
  fi
}

cmd_change_password() {
  local email="" old_password="" new_password=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --email)        email="$2";        shift 2 ;;
      --old-password) old_password="$2"; shift 2 ;;
      --new-password) new_password="$2"; shift 2 ;;
      *) echo "未知选项: $1" >&2; usage; exit 1 ;;
    esac
  done
  [[ -z "$email" ]]        && { echo "错误: --email 必填" >&2; exit 1; }
  [[ -z "$old_password" ]] && { echo "错误: --old-password 必填" >&2; exit 1; }
  [[ -z "$new_password" ]] && { echo "错误: --new-password 必填" >&2; exit 1; }

  echo "正在登录 ${email}..."
  local token
  token=$(login "$email" "$old_password")

  local body
  body=$(jq -n --arg old "$old_password" --arg new "$new_password" \
    '{old_password: $old, new_password: $new}')

  echo "正在修改密码..."
  local resp
  resp=$(curl -s -X PUT "${HOST}/api/v1/user/password" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "$body")
  local code
  code=$(echo "$resp" | jq -r '.code // empty')
  if [[ "$code" == "0" ]] || echo "$resp" | jq -e '.data' &>/dev/null; then
    echo "密码修改成功"
  else
    echo "修改失败:" >&2
    echo "$resp" | jq . >&2
    exit 1
  fi
}

# ── 解析全局参数 ──
if [[ $# -eq 0 ]]; then usage; exit 0; fi

check_deps

SUBCMD=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --host)           HOST="$2";           shift 2 ;;
    --admin-email)    ADMIN_EMAIL="$2";    shift 2 ;;
    --admin-password) ADMIN_PASSWORD="$2"; shift 2 ;;
    create|set-password|change-password)
      SUBCMD="$1"; shift; break ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知选项或子命令: $1" >&2; usage; exit 1 ;;
  esac
done

case "$SUBCMD" in
  create)          cmd_create "$@" ;;
  set-password)    cmd_set_password "$@" ;;
  change-password) cmd_change_password "$@" ;;
  *) echo "请指定子命令: create / set-password / change-password" >&2; usage; exit 1 ;;
esac
