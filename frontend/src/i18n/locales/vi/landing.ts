import { mergeLocaleMessages } from './merge'

const base = {
  // Home Page
home: {
  viewOnGithub: 'Xem trên GitHub',
  viewDocs: 'Xem tài liệu',
  docs: 'Tài liệu',
  switchToLight: 'Chuyển sang chế độ sáng',
  switchToDark: 'Chuyển sang chế độ tối',
  dashboard: 'Bảng điều khiển',
  login: 'Đăng nhập',
  getStarted: 'Bắt đầu ngay',
  goToDashboard: 'Vào bảng điều khiển',
  legal: {
    title: 'Tài liệu pháp lý',
    description: 'Xem điều khoản dịch vụ, chính sách sử dụng, khu vực hỗ trợ và điều khoản riêng trước khi đăng nhập.'
  },
  // Mới thêm: điểm nhấn giá trị cho người dùng
  heroSubtitle: 'Một khóa, dùng nhiều mô hình AI',
  heroDescription: 'Không cần quản lý nhiều tài khoản thuê bao, truy cập một chỗ tới Claude, GPT, Gemini và các dịch vụ AI phổ biến khác',
  tags: {
    subscriptionToApi: 'Từ đăng ký đến API',
    stickySession: 'Duy trì phiên',
    realtimeBilling: 'Tính phí theo mức dùng'
  },
  // Khu vực nêu vấn đề của người dùng
  painPoints: {
    title: 'Bạn cũng đang gặp những vấn đề này?',
    items: {
      expensive: {
        title: 'Chi phí thuê bao cao',
        desc: 'Mỗi dịch vụ AI đều phải đăng ký riêng, chi phí hàng tháng ngày càng tăng'
      },
      complex: {
        title: 'Khó quản lý nhiều tài khoản',
        desc: 'Tài khoản và khóa của nhiều nền tảng bị phân tán khắp nơi, rất phiền khi quản lý'
      },
      unstable: {
        title: 'Dịch vụ thiếu ổn định',
        desc: 'Một tài khoản đơn lẻ rất dễ chạm giới hạn, ảnh hưởng tới việc sử dụng bình thường'
      },
      noControl: {
        title: 'Không kiểm soát được mức sử dụng',
        desc: 'Không biết tiền đang bị tiêu vào đâu, cũng không thể giới hạn mức dùng của từng thành viên trong nhóm'
      }
    }
  },
  // Khu vực giải pháp
  solutions: {
    title: 'Chúng tôi giúp bạn xử lý',
    subtitle: 'Chỉ 3 bước đơn giản để bắt đầu dùng AI nhẹ đầu hơn'
  },
  features: {
    unifiedGateway: 'Kết nối chỉ với một lần cấu hình',
    unifiedGatewayDesc: 'Chỉ cần lấy một API key là có thể gọi toàn bộ mô hình AI đã được tích hợp, không cần đăng ký riêng lẻ.',
    multiAccount: 'Ổn định và đáng tin cậy',
    multiAccountDesc: 'Điều phối thông minh nhiều tài khoản upstream, tự động chuyển đổi và cân bằng tải để giảm lỗi lặp lại.',
    balanceQuota: 'Dùng bao nhiêu trả bấy nhiêu',
    balanceQuotaDesc: 'Tính tiền theo mức dùng thực tế, hỗ trợ đặt trần quota và theo dõi mức dùng của cả nhóm rõ ràng.'
  },
  // So sánh ưu thế
  comparison: {
    title: 'Vì sao nên chọn chúng tôi?',
    headers: {
      feature: 'Hạng mục',
      official: 'Thuê bao chính hãng',
      us: 'Nền tảng này'
    },
    items: {
      pricing: {
        feature: 'Hình thức thanh toán',
        official: 'Phí tháng cố định, không dùng hết vẫn phải trả',
        us: 'Trả theo mức dùng, dùng bao nhiêu trả bấy nhiêu'
      },
      models: {
        feature: 'Lựa chọn mô hình',
        official: 'Một nhà cung cấp duy nhất',
        us: 'Chuyển đổi linh hoạt giữa nhiều mô hình'
      },
      management: {
        feature: 'Quản lý tài khoản',
        official: 'Mỗi dịch vụ phải quản lý riêng',
        us: 'Một khóa thống nhất, quản lý tập trung'
      },
      stability: {
        feature: 'Độ ổn định dịch vụ',
        official: 'Một tài khoản đơn lẻ dễ chạm giới hạn',
        us: 'Pool nhiều tài khoản, tự động chuyển đổi'
      },
      control: {
        feature: 'Kiểm soát mức dùng',
        official: 'Không thể giới hạn',
        us: 'Có thể đặt quota, xem chi tiết'
      }
    }
  },
  providers: {
    title: 'Các mô hình AI đã hỗ trợ',
    description: 'Một API, nhiều lựa chọn',
    supported: 'Đã hỗ trợ',
    soon: 'Sắp ra mắt',
    claude: 'Claude',
    gemini: 'Gemini',
    antigravity: 'Antigravity',
    more: 'Thêm'
  },
  // Khu vực CTA
  cta: {
    title: 'Sẵn sàng bắt đầu chưa?',
    description: 'Đăng ký để nhận hạn mức dùng thử miễn phí và trải nghiệm dịch vụ AI một cửa',
    button: 'Đăng ký miễn phí'
  },
  footer: {
    allRightsReserved: 'Bảo lưu mọi quyền.'
  }
  },

  // Key Usage Query Page
keyUsage: {
  title: 'Tra cứu mức dùng API Key',
  subtitle: 'Nhập API Key của bạn để xem chi phí phát sinh và trạng thái sử dụng theo thời gian thực',
  placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
  query: 'Tra cứu',
  querying: 'Đang tra cứu...',
  privacyNote: 'Key của bạn chỉ được xử lý cục bộ trong trình duyệt và sẽ không bị lưu lại',
  dateRange: 'Phạm vi thống kê:',
  dateRangeToday: 'Hôm nay',
  dateRange7d: '7 ngày',
  dateRange30d: '30 ngày',
  dateRangeCustom: 'Tùy chỉnh',
  apply: 'Áp dụng',
  used: 'Đã sử dụng',
  detailInfo: 'Thông tin chi tiết',
  tokenStats: 'Thống kê token',
  modelStats: 'Thống kê mức dùng theo mô hình',
  // Table headers
  model: 'Mô hình',
  requests: 'Số yêu cầu',
  inputTokens: 'Token đầu vào',
  outputTokens: 'Token đầu ra',
  cacheCreationTokens: 'Tạo bộ nhớ đệm',
  cacheReadTokens: 'Đọc bộ nhớ đệm',
  totalTokens: 'Tổng token',
  cost: 'Chi phí',
  // Status
  quotaMode: 'Chế độ hạn mức của key',
  walletBalance: 'Số dư ví',
  // Ring card titles
  totalQuota: 'Tổng hạn mức',
  limit5h: 'Hạn mức 5 giờ',
  limitDaily: 'Hạn mức theo ngày',
  limit7d: 'Hạn mức 7 ngày',
  limitWeekly: 'Hạn mức theo tuần',
  limitMonthly: 'Hạn mức theo tháng',
  // Detail rows
  remainingQuota: 'Hạn mức còn lại',
  expiresAt: 'Hết hạn lúc',
  todayExpires: '(Hết hạn hôm nay)',
  daysLeft: '(còn {days} ngày)',
  usedQuota: 'Hạn mức đã dùng',
  resetNow: 'Sắp đặt lại',
  subscriptionType: 'Loại đăng ký',
  subscriptionExpires: 'Đăng ký hết hạn',
  // Usage stat cells
  todayRequests: 'Yêu cầu hôm nay',
  todayInputTokens: 'Token đầu vào hôm nay',
  todayOutputTokens: 'Token đầu ra hôm nay',
  todayTokens: 'Token hôm nay',
  todayCacheCreation: 'Tạo bộ nhớ đệm hôm nay',
  todayCacheRead: 'Đọc bộ nhớ đệm hôm nay',
  todayCost: 'Chi phí hôm nay',
  rpmTpm: 'RPM / TPM',
  totalRequests: 'Tổng số yêu cầu',
  totalInputTokens: 'Tổng token đầu vào',
  totalOutputTokens: 'Tổng token đầu ra',
  totalTokensLabel: 'Tổng token tích lũy',
  totalCacheCreation: 'Tổng lượt tạo bộ nhớ đệm',
  totalCacheRead: 'Tổng lượt đọc bộ nhớ đệm',
  totalCost: 'Tổng chi phí',
  avgDuration: 'Thời gian trung bình',
  // Messages
  enterApiKey: 'Vui lòng nhập API Key',
  querySuccess: 'Tra cứu thành công',
  queryFailed: 'Tra cứu thất bại',
  windowUnits: {
    day: 'D',
    week: 'W',
    month: 'M'
  },
  queryFailedRetry: 'Tra cứu thất bại, vui lòng thử lại sau',
  },

  // Setup Wizard
setup: {
  title: 'Trình hướng dẫn cài đặt Sub2API',
  description: 'Cấu hình instance Sub2API của bạn',
  database: {
    title: 'Cấu hình cơ sở dữ liệu',
    description: 'Kết nối tới cơ sở dữ liệu PostgreSQL của bạn',
    host: 'Máy chủ',
    port: 'Cổng',
    username: 'Tên người dùng',
    password: 'Mật khẩu',
    databaseName: 'Tên cơ sở dữ liệu',
    sslMode: 'Chế độ SSL',
    passwordPlaceholder: 'Mật khẩu',
    ssl: {
      disable: 'Tắt',
      require: 'Yêu cầu',
      verifyCa: 'Xác minh CA',
      verifyFull: 'Xác minh đầy đủ'
    }
  },
  redis: {
    title: 'Cấu hình Redis',
    description: 'Kết nối tới máy chủ Redis của bạn',
    host: 'Máy chủ',
    port: 'Cổng',
    password: 'Mật khẩu (không bắt buộc)',
    database: 'Cơ sở dữ liệu',
    passwordPlaceholder: 'Mật khẩu',
    enableTls: 'Bật TLS',
    enableTlsHint: 'Sử dụng TLS khi kết nối Redis (chứng chỉ CA công khai)'
  },
  admin: {
    title: 'Tài khoản quản trị viên',
    description: 'Tạo tài khoản quản trị viên của bạn',
    email: 'Email',
    password: 'Mật khẩu',
    confirmPassword: 'Xác nhận mật khẩu',
    passwordPlaceholder: 'Ít nhất 8 ký tự',
    confirmPasswordPlaceholder: 'Nhập lại mật khẩu',
    passwordMismatch: 'Mật khẩu không khớp'
  },
  ready: {
    autoRefreshRemaining: 'Còn {seconds}s đến lần làm mới tiếp theo',
    title: 'Sẵn sàng cài đặt',
    description: 'Kiểm tra cấu hình của bạn và hoàn tất cài đặt',
    database: 'Cơ sở dữ liệu',
    redis: 'Redis',
    adminEmail: 'Email quản trị viên'
  },
  status: {
    testing: 'Đang kiểm tra...',
    success: 'Kết nối thành công',
    testConnection: 'Kiểm tra kết nối',
    installing: 'Đang cài đặt...',
    completeInstallation: 'Hoàn tất cài đặt',
    completed: 'Cài đặt hoàn tất!',
    redirecting: 'Đang chuyển đến trang đăng nhập...',
    restarting: 'Dịch vụ đang khởi động lại, vui lòng chờ...',
    timeout: 'Thời gian khởi động lại dịch vụ lâu hơn dự kiến, vui lòng tự làm mới trang.'
  }
  }
}

const additionsA = {
  batchImageGuide: {
  title: 'Tạo hình ảnh hàng loạt',
  description: 'Gửi nhiều lời nhắc trong một công việc và tải xuống các hình ảnh được tạo khi hoàn thành',
  },

  keyUsage: {
  dateRange90d: '90 ngày',
  dailyDetail: 'Chi tiết hàng ngày',
  date: 'Ngày',
  cacheWriteTokens: 'Ghi bộ nhớ đệm',
  noDailyUsage: 'Không có dữ liệu sử dụng hàng ngày',
  },

  setup: {
  redis: {
    username: 'Tên người dùng (tùy chọn)',
    usernamePlaceholder: 'Để trống cho người dùng mặc định',
  },
  }
}

export default mergeLocaleMessages(base, additionsA)
