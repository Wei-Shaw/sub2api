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
    ["setting_up_sudoers"]="正在配置 sudoers..."
    ["sudoers_configured"]="sudoers 配置完成"
    ["sudoers_failed"]="sudoers 验证失败，已移除文件"
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

    # Uninstall
    ["uninstall_confirm"]="这将从系统中移除 Sub2API。"
    ["are_you_sure"]="确定要继续吗？(y/N)"
    ["uninstall_cancelled"]="卸载已取消"
    ["removing_files"]="正在移除文件..."
    ["removing_install_dir"]="正在移除安装目录..."
    ["removing_user"]="正在移除用户..."
    ["config_not_removed"]="配置目录未被移除"
    ["remove_manually"]="如不再需要，请手动删除"
    ["uninstall_complete"]="Sub2API 已卸载"

    # Help
    ["usage"]="用法"
    ["cmd_none"]="(无参数)"
    ["cmd_install"]="安装 Sub2API"
    ["cmd_upgrade"]="升级到最新版本"
    ["cmd_uninstall"]="卸载 Sub2API"

    # Server configuration
    ["server_config_title"]="服务器配置"
    ["server_config_desc"]="配置 Sub2API 服务监听地址"
    ["server_host_prompt"]="服务器监听地址"
    ["server_host_hint"]="0.0.0.0 表示监听所有网卡，127.0.0.1 仅本地访问"
    ["server_port_prompt"]="服务器端口"
    ["server_port_hint"]="建议使用 1024-65535 之间的端口"
    ["server_config_summary"]="服务器配置"
    ["invalid_port"]="无效端口号，请输入 1-65535 之间的数字"
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
    ["setting_up_sudoers"]="Setting up sudoers..."
    ["sudoers_configured"]="Sudoers configured"
    ["sudoers_failed"]="Sudoers validation failed, removing file"
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

    # Uninstall
    ["uninstall_confirm"]="This will remove Sub2API from your system."
    ["are_you_sure"]="Are you sure? (y/N)"
    ["uninstall_cancelled"]="Uninstall cancelled"
    ["removing_files"]="Removing files..."
    ["removing_install_dir"]="Removing installation directory..."
    ["removing_user"]="Removing user..."
    ["config_not_removed"]="Config directory was NOT removed."
    ["remove_manually"]="Remove it manually if you no longer need it."
    ["uninstall_complete"]="Sub2API has been uninstalled"

    # Help
    ["usage"]="Usage"
    ["cmd_none"]="(none)"
    ["cmd_install"]="Install Sub2API"
    ["cmd_upgrade"]="Upgrade to the latest version"
    ["cmd_uninstall"]="Remove Sub2API"

    # Server configuration
    ["server_config_title"]="Server Configuration"
    ["server_config_desc"]="Configure Sub2API server listen address"
    ["server_host_prompt"]="Server listen address"
    ["server_host_hint"]="0.0.0.0 listens on all interfaces, 127.0.0.1 for local only"
    ["server_port_prompt"]="Server port"
    ["server_port_hint"]="Recommended range: 1024-65535"
    ["server_config_summary"]="Server configuration"
    ["invalid_port"]="Invalid port number, please enter a number between 1-65535"
)

# Get message based on current language
msg() {
    local key="$1"
    if [ "$LANG_CHOICE" = "en" ]; then
        echo "${MSG_EN[$key]REDACTED"
    else
        echo "${MSG_ZH[$key]REDACTED"
    fi
REDACTED

# Print functions
print_info() {
    echo -e "${BLUEREDACTED[$(msg 'info')]${NCREDACTED $1"
REDACTED

print_success() {
    echo -e "${GREENREDACTED[$(msg 'success')]${NCREDACTED $1"
REDACTED

print_warning() {
    echo -e "${YELLOWREDACTED[$(msg 'warning')]${NCREDACTED $1"
REDACTED

print_error() {
    echo -e "${REDREDACTED[$(msg 'error')]${NCREDACTED $1"
REDACTED

# Select language
select_language() {
    echo ""
    echo -e "${CYANREDACTED=============================================="
    echo "  $(msg 'select_lang')"
    echo "==============================================${NCREDACTED"
    echo ""
    echo "  1) $(msg 'lang_zh') (默认/default)"
    echo "  2) $(msg 'lang_en')"
    echo ""

    # Read with timeout for piped input
    read -t 10 -p "$(msg 'enter_choice'): " lang_input 2>/dev/null || lang_input=""

    case "$lang_input" in
        2|en|EN|english|English)
            LANG_CHOICE="en"
            ;;
        *)
            LANG_CHOICE="zh"
            ;;
    esac

    echo ""
REDACTED

# Validate port number
validate_port() {
    local port="$1"
    if [[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; then
        return 0
    fi
    return 1
REDACTED

# Configure server settings
configure_server() {
    echo ""
    echo -e "${CYANREDACTED=============================================="
    echo "  $(msg 'server_config_title')"
    echo "==============================================${NCREDACTED"
    echo ""
    echo -e "${BLUEREDACTED$(msg 'server_config_desc')${NCREDACTED"
    echo ""

    # Server host
    echo -e "${YELLOWREDACTED$(msg 'server_host_hint')${NCREDACTED"
    read -p "$(msg 'server_host_prompt') [${SERVER_HOSTREDACTED]: " input_host
    if [ -n "$input_host" ]; then
        SERVER_HOST="$input_host"
    fi

    echo ""

    # Server port
    echo -e "${YELLOWREDACTED$(msg 'server_port_hint')${NCREDACTED"
    while true; do
        read -p "$(msg 'server_port_prompt') [${SERVER_PORTREDACTED]: " input_port
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
    print_info "$(msg 'server_config_summary'): ${SERVER_HOSTREDACTED:${SERVER_PORTREDACTED"
    echo ""
REDACTED

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_error "$(msg 'run_as_root')"
        exit 1
    fi
REDACTED

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

    print_info "$(msg 'detected_platform'): ${OSREDACTED_${ARCHREDACTED"
REDACTED

# Check dependencies
check_dependencies() {
    local missing=()

    if ! command -v curl &> /dev/null; then
        missing+=("curl")
    fi

    if ! command -v tar &> /dev/null; then
        missing+=("tar")
    fi

    if [ ${#missing[@]REDACTED -gt 0 ]; then
        print_error "$(msg 'missing_deps'): ${missing[*]REDACTED"
        print_info "$(msg 'install_deps_first')"
        exit 1
    fi
REDACTED

# Get latest release version
get_latest_version() {
    print_info "$(msg 'fetching_version')"
    LATEST_VERSION=$(curl -s "https://api.github.com/repos/${GITHUB_REPOREDACTED/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$LATEST_VERSION" ]; then
        print_error "$(msg 'failed_get_version')"
        exit 1
    fi

    print_info "$(msg 'latest_version'): $LATEST_VERSION"
REDACTED

# Download and extract
download_and_extract() {
    local version_num=${LATEST_VERSION#vREDACTED
    local archive_name="sub2api_${version_numREDACTED_${OSREDACTED_${ARCHREDACTED.tar.gz"
    local download_url="https://github.com/${GITHUB_REPOREDACTED/releases/download/${LATEST_VERSIONREDACTED/${archive_nameREDACTED"
    local checksum_url="https://github.com/${GITHUB_REPOREDACTED/releases/download/${LATEST_VERSIONREDACTED/checksums.txt"

    print_info "$(msg 'downloading') ${archive_nameREDACTED..."

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
        local expected_checksum=$(grep "$archive_name" "$TEMP_DIR/checksums.txt" | awk '{print $1REDACTED')
        local actual_checksum=$(sha256sum "$TEMP_DIR/$archive_name" | awk '{print $1REDACTED')

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
REDACTED

# Create system user
create_user() {
    if id "$SERVICE_USER" &>/dev/null; then
        print_info "$(msg 'user_exists'): $SERVICE_USER"
    else
        print_info "$(msg 'creating_user') $SERVICE_USER..."
        useradd -r -s /bin/false -d "$INSTALL_DIR" "$SERVICE_USER"
        print_success "$(msg 'user_created')"
    fi
REDACTED

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
REDACTED

# Setup sudoers for service restart
setup_sudoers() {
    print_info "$(msg 'setting_up_sudoers')"

    # Check if sudoers file exists in install dir
    if [ -f "$INSTALL_DIR/sub2api-sudoers" ]; then
        cp "$INSTALL_DIR/sub2api-sudoers" /etc/sudoers.d/sub2api
    else
        # Create sudoers file
        cat > /etc/sudoers.d/sub2api << 'EOF'
# Sudoers configuration for Sub2API
sub2api ALL=(ALL) NOPASSWD: /bin/systemctl restart sub2api
sub2api ALL=(ALL) NOPASSWD: /bin/systemctl stop sub2api
sub2api ALL=(ALL) NOPASSWD: /bin/systemctl start sub2api
EOF
    fi

    # Set correct permissions (required for sudoers files)
    chmod 440 /etc/sudoers.d/sub2api

    # Validate sudoers file
    if visudo -c -f /etc/sudoers.d/sub2api &>/dev/null; then
        print_success "$(msg 'sudoers_configured')"
    else
        print_warning "$(msg 'sudoers_failed')"
        rm -f /etc/sudoers.d/sub2api
    fi
REDACTED

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
Environment=SERVER_HOST=${SERVER_HOSTREDACTED
Environment=SERVER_PORT=${SERVER_PORTREDACTED

[Install]
WantedBy=multi-user.target
EOF

    # Reload systemd
    systemctl daemon-reload

    print_success "$(msg 'service_installed')"
REDACTED

# Prepare for setup wizard (no config file needed - setup wizard will create it)
prepare_for_setup() {
    print_success "$(msg 'ready_for_setup')"
REDACTED

# Print completion message
print_completion() {
    local ip_addr
    ip_addr=$(hostname -I 2>/dev/null | awk '{print $1REDACTED' || echo "YOUR_SERVER_IP")

    # Determine display address
    local display_host="$ip_addr"
    if [ "$SERVER_HOST" = "127.0.0.1" ]; then
        display_host="127.0.0.1"
    fi

    echo ""
    echo "=============================================="
    print_success "$(msg 'install_complete')"
    echo "=============================================="
    echo ""
    echo "$(msg 'install_dir'): $INSTALL_DIR"
    echo "$(msg 'server_config_summary'): ${SERVER_HOSTREDACTED:${SERVER_PORTREDACTED"
    echo ""
    echo "=============================================="
    echo "  $(msg 'next_steps')"
    echo "=============================================="
    echo ""
    echo "  1. $(msg 'step1_check_services')"
    echo "     sudo systemctl status postgresql"
    echo "     sudo systemctl status redis"
    echo ""
    echo "  2. $(msg 'step2_start_service')"
    echo "     sudo systemctl start sub2api"
    echo ""
    echo "  3. $(msg 'step3_enable_autostart')"
    echo "     sudo systemctl enable sub2api"
    echo ""
    echo "  4. $(msg 'step4_open_wizard')"
    echo ""
    print_info "     http://${display_hostREDACTED:${SERVER_PORTREDACTED"
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
REDACTED

# Upgrade function
upgrade() {
    print_info "$(msg 'upgrading')"

    # Get current version
    if [ -f "$INSTALL_DIR/sub2api" ]; then
        CURRENT_VERSION=$("$INSTALL_DIR/sub2api" --version 2>/dev/null | grep -oP 'v?\d+\.\d+\.\d+' || echo "unknown")
        print_info "$(msg 'current_version'): $CURRENT_VERSION"
    fi

    # Stop service
    if systemctl is-active --quiet sub2api; then
        print_info "$(msg 'stopping_service')"
        systemctl stop sub2api
    fi

    # Backup current binary
    if [ -f "$INSTALL_DIR/sub2api" ]; then
        cp "$INSTALL_DIR/sub2api" "$INSTALL_DIR/sub2api.backup"
        print_info "$(msg 'backup_created'): $INSTALL_DIR/sub2api.backup"
    fi

    # Download and install new version
    get_latest_version
    download_and_extract

    # Set permissions
    chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/sub2api"

    # Start service
    print_info "$(msg 'starting_service')"
    systemctl start sub2api

    print_success "$(msg 'upgrade_complete')"
REDACTED

# Uninstall function
uninstall() {
    print_warning "$(msg 'uninstall_confirm')"
    read -p "$(msg 'are_you_sure') " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_info "$(msg 'uninstall_cancelled')"
        exit 0
    fi

    print_info "$(msg 'stopping_service')"
    systemctl stop sub2api 2>/dev/null || true
    systemctl disable sub2api 2>/dev/null || true

    print_info "$(msg 'removing_files')"
    rm -f /etc/systemd/system/sub2api.service
    rm -f /etc/sudoers.d/sub2api
    systemctl daemon-reload

    print_info "$(msg 'removing_install_dir')"
    rm -rf "$INSTALL_DIR"

    print_info "$(msg 'removing_user')"
    userdel "$SERVICE_USER" 2>/dev/null || true

    print_warning "$(msg 'config_not_removed'): $CONFIG_DIR"
    print_warning "$(msg 'remove_manually')"

    print_success "$(msg 'uninstall_complete')"
REDACTED

# Main
main() {
    # Select language first
    select_language

    echo ""
    echo "=============================================="
    echo "       $(msg 'install_title')"
    echo "=============================================="
    echo ""

    # Parse arguments
    case "${1:-REDACTED" in
        upgrade|update)
            check_root
            detect_platform
            upgrade
            exit 0
            ;;
        uninstall|remove)
            check_root
            uninstall
            exit 0
            ;;
        --help|-h)
            echo "$(msg 'usage'): $0 [command]"
            echo ""
            echo "Commands:"
            echo "  $(msg 'cmd_none')     $(msg 'cmd_install')"
            echo "  upgrade        $(msg 'cmd_upgrade')"
            echo "  uninstall      $(msg 'cmd_uninstall')"
            echo ""
            exit 0
            ;;
    esac

    # Fresh install
    check_root
    detect_platform
    check_dependencies
    configure_server
    get_latest_version
    download_and_extract
    create_user
    setup_directories
    install_service
    setup_sudoers
    prepare_for_setup
    print_completion
REDACTED

main "$@"
