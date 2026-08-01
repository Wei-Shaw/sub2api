export default {
  home: {
    viewOnGithub: 'Xem trên GitHub',
    viewDocs: 'Xem tài liệu',
    docs: 'Tài liệu',
    switchToLight: 'Chuyển sang chế độ sáng',
    switchToDark: 'Chuyển sang chế độ tối',
    dashboard: 'Bảng điều khiển',
    login: 'Đăng nhập',
    getStarted: 'Bắt đầu',
    goToDashboard: 'Đi tới bảng điều khiển',
    heroSubtitle: 'Một khóa, dùng mọi mô hình AI',
    heroDescription:
      'Không cần quản lý nhiều gói đăng ký. Truy cập Claude, GPT, Gemini và nhiều mô hình khác chỉ với một API key',
    tags: {
      subscriptionToApi: 'Từ đăng ký đến API',
      stickySession: 'Duy trì phiên',
      realtimeBilling: 'Trả theo mức dùng'
    }
  },
  common: {
    loading: 'Đang tải...',
    save: 'Lưu',
    cancel: 'Hủy',
    delete: 'Xóa',
    edit: 'Sửa',
    create: 'Tạo',
    update: 'Cập nhật',
    confirm: 'Xác nhận',
    reset: 'Đặt lại',
    search: 'Tìm kiếm',
    filter: 'Lọc',
    export: 'Xuất',
    import: 'Nhập',
    actions: 'Thao tác',
    status: 'Trạng thái',
    name: 'Tên',
    email: 'Email',
    submit: 'Gửi',
    back: 'Quay lại',
    next: 'Tiếp',
    yes: 'Có',
    no: 'Không',
    all: 'Tất cả',
    none: 'Không có',
    noData: 'Không có dữ liệu',
    success: 'Thành công',
    error: 'Lỗi',
    warning: 'Cảnh báo',
    info: 'Thông tin',
    active: 'Đang hoạt động',
    inactive: 'Không hoạt động',
    more: 'Thêm',
    close: 'Đóng',
    enabled: 'Đã bật',
    disabled: 'Đã tắt'
  },
  auth: {
    login: 'Đăng nhập',
    register: 'Đăng ký',
    forgotPassword: 'Quên mật khẩu?',
    emailPlaceholder: 'Nhập email',
    passwordPlaceholder: 'Nhập mật khẩu',
    invitationCode: 'Mã mời',
    invitationCodePlaceholder: 'Nhập mã mời',
    redeemLogin: 'Đăng nhập bằng mã',
    loginSuccess: 'Đăng nhập thành công',
    loginFailed: 'Đăng nhập thất bại'
  },
  admin: {
    settings: {
      defaults: {
        title: 'Mặc định người dùng mới',
        description: 'Giá trị mặc định áp cho người dùng mới',
        defaultBalance: 'Số dư mặc định',
        defaultBalanceHint: 'Số dư ban đầu cho người dùng mới',
        affiliateRebateRate: 'Tỷ lệ hoa hồng giới thiệu',
        affiliateRebateRateHint:
          'Phần trăm mỗi lần nạp được hoàn lại cho người mời (0-100%, ví dụ 10 nghĩa là hoàn 10%).',
        defaultConcurrency: 'Mức đồng thời mặc định',
        defaultConcurrencyHint: 'Số yêu cầu đồng thời tối đa cho người dùng mới',
        defaultUserRpmLimit: 'Giới hạn RPM mặc định cho người dùng',
        defaultUserRpmLimitHint:
          'Giới hạn request mỗi phút áp cho người dùng mới. Đặt 0 để không giới hạn.',
        deviceClaimBonusBalance: 'Số dư thưởng khi claim thiết bị',
        deviceClaimBonusBalanceHint:
          'Số dư được cộng một lần khi một thiết bị hoàn tất claim V-Claw đầu tiên. Đặt 0 để tắt thưởng.',
        defaultSubscriptions: 'Gói mặc định'
      }
    }
  },
  user: {
    profile: 'Hồ sơ',
    balance: 'Số dư',
    apiKeys: 'API Keys',
    redeem: {
      title: 'Đổi mã',
      placeholder: 'Nhập mã đổi',
      submit: 'Đổi ngay',
      success: 'Đổi mã thành công',
      failed: 'Đổi mã thất bại'
    },
    payment: {
      methods: {
        paddle: 'Paddle'
      },
      paddleLoading: 'Đang tải Paddle…',
      paddleCheckoutReady: 'Paddle checkout đã sẵn sàng.',
      paddleWaitingWebhook: 'Đang chờ Paddle xác nhận thanh toán…',
      paddleLoadFailed: 'Không thể tải Paddle checkout.',
      paddleNotConfigured: 'Paddle chưa được cấu hình.'
    }
  }
}
