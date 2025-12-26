#!/bin/bash
#
# Sub2API Installation Script
# Sub2API 安装脚本
# Usage: curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/install.sh | bash
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
GITHUB_REPO="Wei-Shaw/sub2api"
INSTALL_DIR="/opt/sub2api"
SERVICE_NAME="sub2api"
SERVICE_USER="sub2api"
CONFIG_DIR="/etc/sub2api"

# Server configuration (will be set by user)
SERVER_HOST="0.0.0.0"
SERVER_PORT="8080"

# Language (default: zh = Chinese)
LANG_CHOICE="zh"

# ============================================================
# Language strings / 语言字符串
# ============================================================

# Chinese strings
declare -A MSG_ZH=(
    # General
    ["info"]="信息"
    ["success"]="成功"
    ["warning"]="警告"
    ["error"]="错误"

    # Language selection
    ["select_lang"]="请选择语言 / Select language"
    ["lang_zh"]="中文"
    ["lang_en"]="English"
    ["enter_choice"]="请输入选择 (默认: 1)"

    # Installation
    ["install_title"]="Sub2API 安装脚本"
    ["run_as_root"]="请使用 root 权限运行 (使用 sudo)"
    ["detected_platform"]="检测到平台"
    ["unsupported_arch"]="不支持的架构"
    ["unsupported_os"]="不支持的操作系统"
    ["missing_deps"]="缺少依赖"
    ["install_deps_first"]="请先安装以下依赖"
    ["fetching_version"]="正在获取最新版本..."
    ["latest_version"]="最新版本"
    ["failed_get_version"]="获取最新版本失败"
    ["downloading"]="正在下载"
    ["download_failed"]="下载失败"
    ["verifying_checksum"]="正在校验文件..."
    ["checksum_verified"]="校验通过"
    ["checksum_failed"]="校验失败"
    ["checksum_not_found"]="无法验证校验和（checksums.txt 未找到）"
    ["extracting"]="正在解压..."
    ["binary_installed"]="二进制文件已安装到"
    ["user_exists"]="用户已存在"
    ["creating_user"]="正在创建系统用户"
    ["user_created"]="用户已创建"
    ["setting_up_dirs"]="正在设置目录..."
    ["dirs_configured"]="目录配置完成"
    ["installing_service"]="正在安装 systemd 服务..."
    ["service_installed"]="systemd 服务已安装"
    ["ready_for_setup"]="准备就绪，可以启动设置向导"

    # Completion
    ["install_complete"]="Sub2API 安装完成！"
    ["install_dir"]="安装目录"
    ["next_steps"]="后续步骤"
    ["step1_check_services"]="确保 PostgreSQL 和 Redis 正在运行："
    ["step2_start_service"]="启动 Sub2API 服务："
    ["step3_enable_autostart"]="设置开机自启："
    ["step4_open_wizard"]="在浏览器中打开设置向导："
    ["wizard_guide"]="设置向导将引导您完成："
    ["wizard_db"]="数据库配置"
    ["wizard_redis"]="Redis 配置"
    ["wizard_admin"]="管理员账号创建"
    ["useful_commands"]="常用命令"
    ["cmd_status"]="查看状态"
    ["cmd_logs"]="查看日志"
    ["cmd_restart"]="重启服务"
    ["cmd_stop"]="停止服务"

    # Upgrade
    ["upgrading"]="正在升级 Sub2API..."
    ["current_version"]="当前版本"
    ["stopping_service"]="正在停止服务..."
    ["backup_created"]="备份已创建"
    ["starting_service"]="正在启动服务..."
    ["upgrade_complete"]="升级完成！"

    # Version install
    ["installing_version"]="正在安装指定版本"
    ["version_not_found"]="指定版本不存在"
    ["same_version"]="已经是该版本，无需操作"
    ["rollback_complete"]="版本回退完成！"
    ["install_version_complete"]="指定版本安装完成！"
    ["validating_version"]="正在验证版本..."
    ["available_versions"]="可用版本列表"
    ["fetching_versions"]="正在获取可用版本..."
    ["not_installed"]="Sub2API 尚未安装，请先执行全新安装"
    ["fresh_install_hint"]="用法"

    # Uninstall
    ["uninstall_confirm"]="这将从系统中移除 Sub2API。"
    ["are_you_sure"]="确定要继续吗？(y/N)"
    ["uninstall_cancelled"]="卸载已取消"
    ["removing_files"]="正在移除文件..."
    ["removing_install_dir"]="正在移除安装目录..."
    ["removing_user"]="正在移除用户..."
    ["config_not_removed"]="配置目录未被移除"
    ["remove_manually"]="如不再需要，请手动删除"
    ["removing_install_lock"]="正在移除安装锁文件..."
    ["install_lock_removed"]="安装锁文件已移除，重新安装时将进入设置向导"
    ["purge_prompt"]="是否同时删除配置目录？这将清除所有配置和数据 [y/N]: "
    ["removing_config_dir"]="正在移除配置目录..."
    ["uninstall_complete"]="Sub2API 已卸载"

    # Help
    ["usage"]="用法"
    ["cmd_none"]="(无参数)"
    ["cmd_install"]="安装 Sub2API"
    ["cmd_upgrade"]="升级到最新版本"
    ["cmd_uninstall"]="卸载 Sub2API"
    ["cmd_install_version"]="安装/回退到指定版本"
    ["cmd_list_versions"]="列出可用版本"
    ["opt_version"]="指定要安装的版本号 (例如: v1.0.0)"

    # Server configuration
    ["server_config_title"]="服务器配置"
    ["server_config_desc"]="配置 Sub2API 服务监听地址"
    ["server_host_prompt"]="服务器监听地址"
    ["server_host_hint"]="0.0.0.0 表示监听所有网卡，127.0.0.1 仅本地访问"
    ["server_port_prompt"]="服务器端口"
    ["server_port_hint"]="建议使用 1024-65535 之间的端口"
    ["server_config_summary"]="服务器配置"
    ["invalid_port"]="无效端口号，请输入 1-65535 之间的数字"

    # Service management
    ["starting_service"]="正在启动服务..."
    ["service_started"]="服务已启动"
    ["service_start_failed"]="服务启动失败，请检查日志"
    ["enabling_autostart"]="正在设置开机自启..."
    ["autostart_enabled"]="开机自启已启用"
    ["getting_public_ip"]="正在获取公网 IP..."
    ["public_ip_failed"]="无法获取公网 IP，使用本地 IP"
)

# English strings
declare -A MSG_EN=(
    # General
    ["info"]="INFO"
    ["success"]="SUCCESS"
    ["warning"]="WARNING"
    ["error"]="ERROR"

    # Language selection
    ["select_lang"]="请选择语言 / Select language"
    ["lang_zh"]="中文"
    ["lang_en"]="English"
    ["enter_choice"]="Enter your choice (default: 1)"

    # Installation
    ["install_title"]="Sub2API Installation Script"
    ["run_as_root"]="Please run as root (use sudo)"
    ["detected_platform"]="Detected platform"
    ["unsupported_arch"]="Unsupported architecture"
    ["unsupported_os"]="Unsupported OS"
    ["missing_deps"]="Missing dependencies"
    ["install_deps_first"]="Please install them first"
    ["fetching_version"]="Fetching latest version..."
    ["latest_version"]="Latest version"
    ["failed_get_version"]="Failed to get latest version"
    ["downloading"]="Downloading"
    ["download_failed"]="Download failed"
    ["verifying_checksum"]="Verifying checksum..."
    ["checksum_verified"]="Checksum verified"
    ["checksum_failed"]="Checksum verification failed"
    ["checksum_not_found"]="Could not verify checksum (checksums.txt not found)"
    ["extracting"]="Extracting..."
    ["binary_installed"]="Binary installed to"
    ["user_exists"]="User already exists"
    ["creating_user"]="Creating system user"
    ["user_created"]="User created"
    ["setting_up_dirs"]="Setting up directories..."
    ["dirs_configured"]="Directories configured"
    ["installing_service"]="Installing systemd service..."
    ["service_installed"]="Systemd service installed"
    ["ready_for_setup"]="Ready for Setup Wizard"

    # Completion
    ["install_complete"]="Sub2API installation completed!"
    ["install_dir"]="Installation directory"
    ["next_steps"]="NEXT STEPS"
    ["step1_check_services"]="Make sure PostgreSQL and Redis are running:"
    ["step2_start_service"]="Start Sub2API service:"
    ["step3_enable_autostart"]="Enable auto-start on boot:"
    ["step4_open_wizard"]="Open the Setup Wizard in your browser:"
    ["wizard_guide"]="The Setup Wizard will guide you through:"
    ["wizard_db"]="Database configuration"
    ["wizard_redis"]="Redis configuration"
    ["wizard_admin"]="Admin account creation"
    ["useful_commands"]="USEFUL COMMANDS"
    ["cmd_status"]="Check status"
    ["cmd_logs"]="View logs"
    ["cmd_restart"]="Restart"
    ["cmd_stop"]="Stop"

    # Upgrade
    ["upgrading"]="Upgrading Sub2API..."
    ["current_version"]="Current version"
    ["stopping_service"]="Stopping service..."
    ["backup_created"]="Backup created"
    ["starting_service"]="Starting service..."
    ["upgrade_complete"]="Upgrade completed!"

    # Version install
    ["installing_version"]="Installing specified version"
    ["version_not_found"]="Specified version not found"
    ["same_version"]="Already at this version, no action needed"
    ["rollback_complete"]="Version rollback completed!"
    ["install_version_complete"]="Specified version installed!"
    ["validating_version"]="Validating version..."
    ["available_versions"]="Available versions"
    ["fetching_versions"]="Fetching available versions..."
    ["not_installed"]="Sub2API is not installed. Please run a fresh install first"
    ["fresh_install_hint"]="Usage"

    # Uninstall
    ["uninstall_confirm"]="This will remove Sub2API from your system."
    ["are_you_sure"]="Are you sure? (y/N)"
    ["uninstall_cancelled"]="Uninstall cancelled"
    ["removing_files"]="Removing files..."
    ["removing_install_dir"]="Removing installation directory..."
    ["removing_user"]="Removing user..."
    ["config_not_removed"]="Config directory was NOT removed."
    ["remove_manually"]="Remove it manually if you no longer need it."
    ["removing_install_lock"]="Removing install lock file..."
    ["install_lock_removed"]="Install lock removed. Setup wizard will appear on next install."
    ["purge_prompt"]="Also remove config directory? This will delete all config and data [y/N]: "
    ["removing_config_dir"]="Removing config directory..."
    ["uninstall_complete"]="Sub2API has been uninstalled"

    # Help
    ["usage"]="Usage"
    ["cmd_none"]="(none)"
    ["cmd_install"]="Install Sub2API"
    ["cmd_upgrade"]="Upgrade to the latest version"
    ["cmd_uninstall"]="Remove Sub2API"
    ["cmd_install_version"]="Install/rollback to a specific version"
    ["cmd_list_versions"]="List available versions"
    ["opt_version"]="Specify version to install (e.g., v1.0.0)"

    # Server configuration
    ["server_config_title"]="Server Configuration"
    ["server_config_desc"]="Configure Sub2API server listen address"
    ["server_host_prompt"]="Server listen address"
    ["server_host_hint"]="0.0.0.0 listens on all interfaces, 127.0.0.1 for local only"
    ["server_port_prompt"]="Server port"
    ["server_port_hint"]="Recommended range: 1024-65535"
    ["server_config_summary"]="Server configuration"
    ["invalid_port"]="Invalid port number, please enter a number between 1-65535"

    # Service management
    ["starting_service"]="Starting service..."
    ["service_started"]="Service started"
    ["service_start_failed"]="Service failed to start, please check logs"
    ["enabling_autostart"]="Enabling auto-start on boot..."
    ["autostart_enabled"]="Auto-start enabled"
    ["getting_public_ip"]="Getting public IP..."
    ["public_ip_failed"]="Failed to get public IP, using local IP"
)

# Get message based on current language
msg() {
    local key="$1"
    if [ "$LANG_CHOICE" = "en" ]; then
        echo "${MSG_EN[$key]}"
    else
        echo "${MSG_ZH[$key]}"
    fi
}

# Print functions
print_info() {
    echo -e "${BLUE}[$(msg 'info')]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[$(msg 'success')]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[$(msg 'warning')]${NC} $1"
}

print_error() {
    echo -e "${RED}[$(msg 'error')]${NC} $1"
}

# Check if running interactively (can access terminal)
# When piped (curl | bash), stdin is not a terminal, but /dev/tty may still be available
is_interactive() {
    # Check if /dev/tty is available (works even when piped)
    [ -e /dev/tty ] && [ -r /dev/tty ] && [ -w /dev/tty ]
}

# Select language
select_language() {
    # If not interactive (piped), use default language
    if ! is_interactive; then
        LANG_CHOICE="zh"
        return
    fi

    echo ""
    echo -e "${CYAN}=============================================="
    echo "  $(msg 'select_lang')"
    echo "==============================================${NC}"
    echo ""
    echo "  1) $(msg 'lang_zh') (默认/default)"
    echo "  2) $(msg 'lang_en')"
    echo ""

    read -p "$(msg 'enter_choice'): " lang_input < /dev/tty

    case "$lang_input" in
        2|en|EN|english|English)
            LANG_CHOICE="en"
            ;;
        *)
            LANG_CHOICE="zh"
            ;;
    esac

    echo ""
}

# Validate port number
validate_port() {
    local port="$1"
    if [[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; then
        return 0
    fi
    return 1
}

# Configure server settings
configure_server() {
    # If not interactive (piped), use default settings
    if ! is_interactive; then
        print_info "$(msg 'server_config_summary'): ${SERVER_HOST}:${SERVER_PORT} (default)"
        return
    fi

    echo ""
    echo -e "${CYAN}=============================================="
    echo "  $(msg 'server_config_title')"
    echo "==============================================${NC}"
    echo ""
    echo -e "${BLUE}$(msg 'server_config_desc')${NC}"
    echo ""

    # Server host
    echo -e "${YELLOW}$(msg 'server_host_hint')${NC}"
    read -p "$(msg 'server_host_prompt') [${SERVER_HOST}]: " input_host < /dev/tty
    if [ -n "$input_host" ]; then
        SERVER_HOST="$input_host"
    fi

    echo ""

    # Server port
    echo -e "${YELLOW}$(msg 'server_port_hint')${NC}"
    while true; do
        read -p "$(msg 'server_port_prompt') [${SERVER_PORT}]: " input_port < /dev/tty
        if [ -z "$input_port" ]; then
            # Use default
            break
        elif validate_port "$input_port"; then
            SERVER_PORT="$input_port"
            break
        else
            print_error "$(msg 'invalid_port')"
        fi
    done

    echo ""
    print_info "$(msg 'server_config_summary'): ${SERVER_HOST}:${SERVER_PORT}"
    echo ""
}

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_error "$(msg 'run_as_root')"
        exit 1
    fi
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$ARCH" in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            print_error "$(msg 'unsupported_arch'): $ARCH"
            exit 1
            ;;
    esac

    case "$OS" in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        *)
            print_error "$(msg 'unsupported_os'): $OS"
            exit 1
            ;;
    esac

    print_info "$(msg 'detected_platform'): ${OS}_${ARCH}"
}

# Check dependencies
check_dependencies() {
    local missing=()

    if ! command -v curl &> /dev/null; then
        missing+=("curl")
    fi

    if ! command -v tar &> /dev/null; then
        missing+=("tar")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        print_error "$(msg 'missing_deps'): ${missing[*]}"
        print_info "$(msg 'install_deps_first')"
        exit 1
    fi
}

# Get latest release version
get_latest_version() {
    print_info "$(msg 'fetching_version')"
    LATEST_VERSION=$(curl -s --connect-timeout 10 --max-time 30 "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$LATEST_VERSION" ]; then
        print_error "$(msg 'failed_get_version')"
        print_info "Please check your network connection or try again later."
        exit 1
    fi

    print_info "$(msg 'latest_version'): $LATEST_VERSION"
}

# List available versions
list_versions() {
    print_info "$(msg 'fetching_versions')"

    local versions
    versions=$(curl -s --connect-timeout 10 --max-time 30 "https://api.github.com/repos/${GITHUB_REPO}/releases" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | head -20)

    if [ -z "$versions" ]; then
        print_error "$(msg 'failed_get_version')"
        print_info "Please check your network connection or try again later."
        exit 1
    fi

    echo ""
    echo "$(msg 'available_versions'):"
    echo "----------------------------------------"
    echo "$versions" | while read -r version; do
        echo "  $version"
    done
    echo "----------------------------------------"
    echo ""
}

# Validate if a version exists
validate_version() {
    local version="$1"

    # Check for empty version
    if [ -z "$version" ]; then
        print_error "$(msg 'opt_version')" >&2
        exit 1
    fi

    # Ensure version starts with 'v'
    if [[ ! "$version" =~ ^v ]]; then
        version="v$version"
    fi

    print_info "$(msg 'validating_version') $version" >&2

    # Check if the release exists
    local http_code
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 --max-time 30 "https://api.github.com/repos/${GITHUB_REPO}/releases/tags/${version}" 2>/dev/null)

    # Check for network errors (empty or non-numeric response)
    if [ -z "$http_code" ] || ! [[ "$http_code" =~ ^[0-9]+$ ]]; then
        print_error "Network error: Failed to connect to GitHub API" >&2
        exit 1
    fi

    if [ "$http_code" != "200" ]; then
        print_error "$(msg 'version_not_found'): $version" >&2
        echo "" >&2
        list_versions >&2
        exit 1
    fi

    # Return the normalized version (to stdout)
    echo "$version"
}

# Get current installed version
get_current_version() {
    if [ -f "$INSTALL_DIR/sub2api" ]; then
        # Use grep -E for better compatibility (works on macOS and Linux)
        "$INSTALL_DIR/sub2api" --version 2>/dev/null | grep -oE 'v?[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown"
    else
        echo "not_installed"
    fi
}

# Download and extract
download_and_extract() {
    local version_num=${LATEST_VERSION#v}
    local archive_name="sub2api_${version_num}_${OS}_${ARCH}.tar.gz"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_VERSION}/${archive_name}"
    local checksum_url="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_VERSION}/checksums.txt"

    print_info "$(msg 'downloading') ${archive_name}..."

    # Create temp directory
    TEMP_DIR=$(mktemp -d)
    trap "rm -rf $TEMP_DIR" EXIT

    # Download archive
    if ! curl -sL "$download_url" -o "$TEMP_DIR/$archive_name"; then
        print_error "$(msg 'download_failed')"
        exit 1
    fi

    # Download and verify checksum
    print_info "$(msg 'verifying_checksum')"
    if curl -sL "$checksum_url" -o "$TEMP_DIR/checksums.txt" 2>/dev/null; then
        local expected_checksum=$(grep "$archive_name" "$TEMP_DIR/checksums.txt" | awk '{print $1}')
        local actual_checksum=$(sha256sum "$TEMP_DIR/$archive_name" | awk '{print $1}')

        if [ "$expected_checksum" != "$actual_checksum" ]; then
            print_error "$(msg 'checksum_failed')"
            print_error "Expected: $expected_checksum"
            print_error "Actual: $actual_checksum"
            exit 1
        fi
        print_success "$(msg 'checksum_verified')"
    else
        print_warning "$(msg 'checksum_not_found')"
    fi

    # Extract
    print_info "$(msg 'extracting')"
    tar -xzf "$TEMP_DIR/$archive_name" -C "$TEMP_DIR"

    # Create install directory
    mkdir -p "$INSTALL_DIR"

    # Copy binary
    cp "$TEMP_DIR/sub2api" "$INSTALL_DIR/sub2api"
    chmod +x "$INSTALL_DIR/sub2api"

    # Copy deploy files if they exist in the archive
    if [ -d "$TEMP_DIR/deploy" ]; then
        cp -r "$TEMP_DIR/deploy/"* "$INSTALL_DIR/" 2>/dev/null || true
    fi

    print_success "$(msg 'binary_installed') $INSTALL_DIR/sub2api"
}

# Create system user
create_user() {
    if id "$SERVICE_USER" &>/dev/null; then
        print_info "$(msg 'user_exists'): $SERVICE_USER"
        # Fix: Ensure existing user has /bin/sh shell for sudo to work
        # Previous versions used /bin/false which prevents sudo execution
        local current_shell
        current_shell=$(getent passwd "$SERVICE_USER" 2>/dev/null | cut -d: -f7)
        if [ "$current_shell" = "/bin/false" ] || [ "$current_shell" = "/sbin/nologin" ]; then
            print_info "Fixing user shell for sudo compatibility..."
            if usermod -s /bin/sh "$SERVICE_USER" 2>/dev/null; then
                print_success "User shell updated to /bin/sh"
            else
                print_warning "Failed to update user shell. Service restart may not work automatically."
                print_warning "Manual fix: sudo usermod -s /bin/sh $SERVICE_USER"
            fi
        fi
    else
        print_info "$(msg 'creating_user') $SERVICE_USER..."
        # Use /bin/sh instead of /bin/false to allow sudo execution
        # The user still cannot login interactively (no password set)
        useradd -r -s /bin/sh -d "$INSTALL_DIR" "$SERVICE_USER"
        print_success "$(msg 'user_created')"
    fi
}

# Setup directories and permissions
setup_directories() {
    print_info "$(msg 'setting_up_dirs')"

    # Create directories
    mkdir -p "$INSTALL_DIR"
    mkdir -p "$INSTALL_DIR/data"
    mkdir -p "$CONFIG_DIR"

    # Set ownership
    chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
    chown -R "$SERVICE_USER:$SERVICE_USER" "$CONFIG_DIR"

    print_success "$(msg 'dirs_configured')"
}

# Install systemd service
install_service() {
    print_info "$(msg 'installing_service')"

    # Create service file with configured host and port
    cat > /etc/systemd/system/sub2api.service << EOF
[Unit]
Description=Sub2API - AI API Gateway Platform
Documentation=https://github.com/Wei-Shaw/sub2api
After=network.target postgresql.service redis.service
Wants=postgresql.service redis.service

[Service]
Type=simple
User=sub2api
Group=sub2api
WorkingDirectory=/opt/sub2api
ExecStart=/opt/sub2api/sub2api
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=sub2api

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/sub2api

# Environment - Server configuration
Environment=GIN_MODE=release
Environment=SERVER_HOST=${SERVER_HOST}
Environment=SERVER_PORT=${SERVER_PORT}

[Install]
WantedBy=multi-user.target
EOF

    # Reload systemd
    systemctl daemon-reload

    print_success "$(msg 'service_installed')"
}

# Prepare for setup wizard (no config file needed - setup wizard will create it)
prepare_for_setup() {
    print_success "$(msg 'ready_for_setup')"
}

# Get public IP address
get_public_ip() {
    print_info "$(msg 'getting_public_ip')"

    # Try to get public IP from ipinfo.io
    local response
    response=$(curl -s --connect-timeout 5 --max-time 10 "https://ipinfo.io/json" 2>/dev/null)

    if [ -n "$response" ]; then
        # Extract IP from JSON response using grep and sed (no jq dependency)
        PUBLIC_IP=$(echo "$response" | grep -o '"ip": *"[^"]*"' | sed 's/"ip": *"\([^"]*\)"/\1/')
        if [ -n "$PUBLIC_IP" ]; then
            print_success "Public IP: $PUBLIC_IP"
            return 0
        fi
    fi

    # Fallback to local IP
    print_warning "$(msg 'public_ip_failed')"
    PUBLIC_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "YOUR_SERVER_IP")
    return 1
}

# Start service
start_service() {
    print_info "$(msg 'starting_service')"

    if systemctl start sub2api; then
        print_success "$(msg 'service_started')"
        return 0
    else
        print_error "$(msg 'service_start_failed')"
        print_info "sudo journalctl -u sub2api -n 50"
        return 1
    fi
}

# Enable service auto-start
enable_autostart() {
    print_info "$(msg 'enabling_autostart')"

    if systemctl enable sub2api 2>/dev/null; then
        print_success "$(msg 'autostart_enabled')"
        return 0
    else
        print_warning "Failed to enable auto-start"
        return 1
    fi
}

# Print completion message
print_completion() {
    # Use PUBLIC_IP which was set by get_public_ip()
    # Determine display address
    local display_host="${PUBLIC_IP:-YOUR_SERVER_IP}"
    if [ "$SERVER_HOST" = "127.0.0.1" ]; then
        display_host="127.0.0.1"
    fi

    echo ""
    echo "=============================================="
    print_success "$(msg 'install_complete')"
    echo "=============================================="
    echo ""
    echo "$(msg 'install_dir'): $INSTALL_DIR"
    echo "$(msg 'server_config_summary'): ${SERVER_HOST}:${SERVER_PORT}"
    echo ""
    echo "=============================================="
    echo "  $(msg 'step4_open_wizard')"
    echo "=============================================="
    echo ""
    print_info "     http://${display_host}:${SERVER_PORT}"
    echo ""
    echo "     $(msg 'wizard_guide')"
    echo "     - $(msg 'wizard_db')"
    echo "     - $(msg 'wizard_redis')"
    echo "     - $(msg 'wizard_admin')"
    echo ""
    echo "=============================================="
    echo "  $(msg 'useful_commands')"
    echo "=============================================="
    echo ""
    echo "  $(msg 'cmd_status'):   sudo systemctl status sub2api"
    echo "  $(msg 'cmd_logs'):     sudo journalctl -u sub2api -f"
    echo "  $(msg 'cmd_restart'):  sudo systemctl restart sub2api"
    echo "  $(msg 'cmd_stop'):     sudo systemctl stop sub2api"
    echo ""
    echo "=============================================="
}

# Upgrade function
upgrade() {
    # Check if Sub2API is installed
    if [ ! -f "$INSTALL_DIR/sub2api" ]; then
        print_error "$(msg 'not_installed')"
        print_info "$(msg 'fresh_install_hint'): $0 install"
        exit 1
    fi

    print_info "$(msg 'upgrading')"

    # Get current version
    CURRENT_VERSION=$("$INSTALL_DIR/sub2api" --version 2>/dev/null | grep -oE 'v?[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
    print_info "$(msg 'current_version'): $CURRENT_VERSION"

    # Stop service
    if systemctl is-active --quiet sub2api; then
        print_info "$(msg 'stopping_service')"
        systemctl stop sub2api
    fi

    # Backup current binary
    cp "$INSTALL_DIR/sub2api" "$INSTALL_DIR/sub2api.backup"
    print_info "$(msg 'backup_created'): $INSTALL_DIR/sub2api.backup"

    # Download and install new version
    get_latest_version
    download_and_extract

    # Set permissions
    chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/sub2api"

    # Start service
    print_info "$(msg 'starting_service')"
    systemctl start sub2api

    print_success "$(msg 'upgrade_complete')"
}

# Install specific version (for upgrade or rollback)
# Requires: Sub2API must already be installed
install_version() {
    local target_version="$1"

    # Check if Sub2API is installed
    if [ ! -f "$INSTALL_DIR/sub2api" ]; then
        print_error "$(msg 'not_installed')"
        print_info "$(msg 'fresh_install_hint'): $0 install -v $target_version"
        exit 1
    fi

    # Validate and normalize version
    target_version=$(validate_version "$target_version")

    print_info "$(msg 'installing_version'): $target_version"

    # Get current version
    local current_version
    current_version=$(get_current_version)
    print_info "$(msg 'current_version'): $current_version"

    # Check if same version
    if [ "$current_version" = "$target_version" ] || [ "$current_version" = "${target_version#v}" ]; then
        print_warning "$(msg 'same_version')"
        exit 0
    fi

    # Stop service if running
    if systemctl is-active --quiet sub2api; then
        print_info "$(msg 'stopping_service')"
        systemctl stop sub2api
    fi

    # Backup current binary (for potential recovery)
    if [ -f "$INSTALL_DIR/sub2api" ]; then
        local backup_name
        if [ "$current_version" != "unknown" ] && [ "$current_version" != "not_installed" ]; then
            backup_name="sub2api.backup.${current_version}"
        else
            backup_name="sub2api.backup.$(date +%Y%m%d%H%M%S)"
        fi
        cp "$INSTALL_DIR/sub2api" "$INSTALL_DIR/$backup_name"
        print_info "$(msg 'backup_created'): $INSTALL_DIR/$backup_name"
    fi

    # Set LATEST_VERSION to the target version for download_and_extract
    LATEST_VERSION="$target_version"

    # Download and install
    download_and_extract

    # Set permissions
    chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/sub2api"

    # Start service
    print_info "$(msg 'starting_service')"
    if systemctl start sub2api; then
        print_success "$(msg 'service_started')"
    else
        print_error "$(msg 'service_start_failed')"
        print_info "sudo journalctl -u sub2api -n 50"
    fi

    # Print completion message
    local new_version
    new_version=$(get_current_version)
    echo ""
    echo "=============================================="
    print_success "$(msg 'install_version_complete')"
    echo "=============================================="
    echo ""
    echo "  $(msg 'current_version'): $new_version"
    echo ""
}

# Uninstall function
uninstall() {
    print_warning "$(msg 'uninstall_confirm')"

    # If not interactive (piped), require -y flag or skip confirmation
    if ! is_interactive; then
        if [ "${FORCE_YES:-}" != "true" ]; then
            print_error "Non-interactive mode detected. Use 'curl ... | bash -s -- uninstall -y' to confirm."
            exit 1
        fi
    else
        read -p "$(msg 'are_you_sure') " -n 1 -r < /dev/tty
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_info "$(msg 'uninstall_cancelled')"
            exit 0
        fi
    fi

    print_info "$(msg 'stopping_service')"
    systemctl stop sub2api 2>/dev/null || true
    systemctl disable sub2api 2>/dev/null || true

    print_info "$(msg 'removing_files')"
    rm -f /etc/systemd/system/sub2api.service
    systemctl daemon-reload

    print_info "$(msg 'removing_install_dir')"
    rm -rf "$INSTALL_DIR"

    print_info "$(msg 'removing_user')"
    userdel "$SERVICE_USER" 2>/dev/null || true

    # Remove install lock file (.installed) to allow fresh setup on reinstall
    print_info "$(msg 'removing_install_lock')"
    rm -f "$CONFIG_DIR/.installed" 2>/dev/null || true
    rm -f "$INSTALL_DIR/.installed" 2>/dev/null || true
    print_success "$(msg 'install_lock_removed')"

    # Ask about config directory removal (interactive mode only)
    local remove_config=false
    if [ "${PURGE:-}" = "true" ]; then
        remove_config=true
    elif is_interactive; then
        read -p "$(msg 'purge_prompt')" -n 1 -r < /dev/tty
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            remove_config=true
        fi
    fi

    if [ "$remove_config" = true ]; then
        print_info "$(msg 'removing_config_dir')"
        rm -rf "$CONFIG_DIR"
    else
        print_warning "$(msg 'config_not_removed'): $CONFIG_DIR"
        print_warning "$(msg 'remove_manually')"
    fi

    print_success "$(msg 'uninstall_complete')"
}

# Main
main() {
    # Parse flags first
    local target_version=""
    local positional_args=()

    while [[ $# -gt 0 ]]; do
        case "$1" in
            -y|--yes)
                FORCE_YES="true"
                shift
                ;;
            --purge)
                PURGE="true"
                shift
                ;;
            -v|--version)
                if [ -n "${2:-}" ] && [[ ! "$2" =~ ^- ]]; then
                    target_version="$2"
                    shift 2
                else
                    echo "Error: --version requires a version argument"
                    exit 1
                fi
                ;;
            --version=*)
                target_version="${1#*=}"
                if [ -z "$target_version" ]; then
                    echo "Error: --version requires a version argument"
                    exit 1
                fi
                shift
                ;;
            *)
                positional_args+=("$1")
                shift
                ;;
        esac
    done

    # Restore positional arguments
    set -- "${positional_args[@]}"

    # Select language first
    select_language

    echo ""
    echo "=============================================="
    echo "       $(msg 'install_title')"
    echo "=============================================="
    echo ""

    # Parse commands
    case "${1:-}" in
        upgrade|update)
            check_root
            detect_platform
            check_dependencies
            if [ -n "$target_version" ]; then
                # Upgrade to specific version
                install_version "$target_version"
            else
                # Upgrade to latest
                upgrade
            fi
            exit 0
            ;;
        install)
            # Install with optional version
            check_root
            detect_platform
            check_dependencies
            if [ -n "$target_version" ]; then
                # Install specific version (fresh install or rollback)
                if [ -f "$INSTALL_DIR/sub2api" ]; then
                    # Already installed, treat as version change
                    install_version "$target_version"
                else
                    # Fresh install with specific version
                    configure_server
                    LATEST_VERSION=$(validate_version "$target_version")
                    download_and_extract
                    create_user
                    setup_directories
                    install_service
                    prepare_for_setup
                    get_public_ip
                    start_service
                    enable_autostart
                    print_completion
                fi
            else
                # Fresh install with latest version
                configure_server
                get_latest_version
                download_and_extract
                create_user
                setup_directories
                install_service
                prepare_for_setup
                get_public_ip
                start_service
                enable_autostart
                print_completion
            fi
            exit 0
            ;;
        rollback)
            # Rollback to a specific version (alias for install with version)
            if [ -z "$target_version" ] && [ -n "${2:-}" ]; then
                target_version="$2"
            fi
            if [ -z "$target_version" ]; then
                print_error "$(msg 'opt_version')"
                echo ""
                echo "Usage: $0 rollback -v <version>"
                echo "       $0 rollback <version>"
                echo ""
                list_versions
                exit 1
            fi
            check_root
            detect_platform
            check_dependencies
            install_version "$target_version"
            exit 0
            ;;
        list-versions|versions)
            list_versions
            exit 0
            ;;
        uninstall|remove)
            check_root
            uninstall
            exit 0
            ;;
        --help|-h)
            echo "$(msg 'usage'): $0 [command] [options]"
            echo ""
            echo "Commands:"
            echo "  $(msg 'cmd_none')            $(msg 'cmd_install')"
            echo "  install              $(msg 'cmd_install')"
            echo "  upgrade              $(msg 'cmd_upgrade')"
            echo "  rollback <version>   $(msg 'cmd_install_version')"
            echo "  list-versions        $(msg 'cmd_list_versions')"
            echo "  uninstall            $(msg 'cmd_uninstall')"
            echo ""
            echo "Options:"
            echo "  -v, --version <ver>  $(msg 'opt_version')"
            echo "  -y, --yes            Skip confirmation prompts (for uninstall)"
            echo ""
            echo "Examples:"
            echo "  $0                        # Install latest version"
            echo "  $0 install -v v0.1.0      # Install specific version"
            echo "  $0 upgrade                # Upgrade to latest"
            echo "  $0 upgrade -v v0.2.0      # Upgrade to specific version"
            echo "  $0 rollback v0.1.0        # Rollback to v0.1.0"
            echo "  $0 list-versions          # List available versions"
            echo ""
            exit 0
            ;;
    esac

    # Default: Fresh install with latest version
    check_root
    detect_platform
    check_dependencies

    if [ -n "$target_version" ]; then
        # Install specific version
        if [ -f "$INSTALL_DIR/sub2api" ]; then
            install_version "$target_version"
        else
            configure_server
            LATEST_VERSION=$(validate_version "$target_version")
            download_and_extract
            create_user
            setup_directories
            install_service
            prepare_for_setup
            get_public_ip
            start_service
            enable_autostart
            print_completion
        fi
    else
        # Install latest version
        configure_server
        get_latest_version
        download_and_extract
        create_user
        setup_directories
        install_service
        prepare_for_setup
        get_public_ip
        start_service
        enable_autostart
        print_completion
    fi
}

main "$@"
