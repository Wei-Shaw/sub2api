export default {
  batchImageGuide: {
    title: 'Tạo ảnh hàng loạt',
    description: 'Gửi nhiều prompt trong một tác vụ và tải xuống ảnh đã tạo khi hoàn tất'
  },
  // Home Page
  home: {
    viewOnGithub: 'Xem trên GitHub',
    viewDocs: 'Xem tài liệu',
    docs: 'Tài liệu',
    switchToLight: 'Chuyển sang chế độ sáng',
    switchToDark: 'Chuyển sang chế độ tối',
    dashboard: 'Bảng điều khiển',
    login: 'Đăng nhập',
    getStarted: 'Bắt đầu',
    goToDashboard: 'Vào bảng điều khiển',
    // User-focused value proposition
    heroSubtitle: 'Một khóa, mọi mô hình AI',
    heroDescription: 'Không cần quản lý nhiều gói đăng ký. Truy cập Claude, GPT, Gemini và nhiều hơn nữa chỉ với một API Key',
    tags: {
      subscriptionToApi: 'Gói đăng ký thành API',
      stickySession: 'Duy trì phiên',
      realtimeBilling: 'Dùng bao nhiêu trả bấy nhiêu'
    },
    // Pain points section
    painPoints: {
      title: 'Nghe quen không?',
      items: {
        expensive: {
          title: 'Chi phí đăng ký cao',
          desc: 'Trả tiền cho nhiều gói đăng ký AI cộng dồn mỗi tháng'
        },
        complex: {
          title: 'Tài khoản hỗn loạn',
          desc: 'Quản lý tài khoản và API Key rải rác trên nhiều nền tảng khác nhau'
        },
        unstable: {
          title: 'Gián đoạn dịch vụ',
          desc: 'Tài khoản đơn lẻ chạm giới hạn tốc độ và làm gián đoạn công việc'
        },
        noControl: {
          title: 'Không kiểm soát được mức dùng',
          desc: 'Không theo dõi được tiền đi đâu hoặc không giới hạn được mức dùng của thành viên'
        }
      }
    },
    // Solutions section
    solutions: {
      title: 'Chúng tôi giải quyết những vấn đề này',
      subtitle: 'Ba bước đơn giản để truy cập AI không lo lắng'
    },
    features: {
      unifiedGateway: 'Truy cập một chạm',
      unifiedGatewayDesc: 'Nhận một API Key duy nhất để gọi mọi mô hình AI đã kết nối. Không cần đăng ký riêng lẻ.',
      multiAccount: 'Luôn ổn định',
      multiAccountDesc: 'Định tuyến thông minh qua nhiều tài khoản thượng nguồn với tự động chuyển đổi dự phòng. Tạm biệt lỗi.',
      balanceQuota: 'Dùng bao nhiêu trả bấy nhiêu',
      balanceQuotaDesc: 'Tính phí theo mức sử dụng với giới hạn hạn mức. Nắm rõ toàn bộ mức tiêu thụ của nhóm.'
    },
    // Comparison section
    comparison: {
      title: 'Vì sao chọn chúng tôi?',
      headers: {
        feature: 'So sánh',
        official: 'Gói đăng ký chính thức',
        us: 'Nền tảng của chúng tôi'
      },
      items: {
        pricing: {
          feature: 'Giá cả',
          official: 'Phí cố định hằng tháng, không dùng vẫn phải trả',
          us: 'Chỉ trả cho những gì bạn dùng'
        },
        models: {
          feature: 'Lựa chọn mô hình',
          official: 'Chỉ một nhà cung cấp',
          us: 'Tự do chuyển đổi giữa các mô hình'
        },
        management: {
          feature: 'Quản lý tài khoản',
          official: 'Quản lý từng dịch vụ riêng lẻ',
          us: 'Một khóa thống nhất, một bảng điều khiển'
        },
        stability: {
          feature: 'Độ ổn định',
          official: 'Giới hạn tốc độ của tài khoản đơn lẻ',
          us: 'Kho nhiều tài khoản, tự động chuyển dự phòng'
        },
        control: {
          feature: 'Kiểm soát mức dùng',
          official: 'Không có',
          us: 'Hạn mức và phân tích chi tiết'
        }
      }
    },
    providers: {
      title: 'Các mô hình AI được hỗ trợ',
      description: 'Một API, nhiều lựa chọn',
      supported: 'Đã hỗ trợ',
      soon: 'Sắp có',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: 'Thêm nữa'
    },
    // CTA section
    cta: {
      title: 'Sẵn sàng bắt đầu?',
      description: 'Đăng ký ngay và nhận tín dụng dùng thử miễn phí để trải nghiệm truy cập AI liền mạch',
      button: 'Đăng ký miễn phí'
    },
    connect: {
      title: 'Trỏ CLI của bạn tới relay này',
      hint: 'Tạo API Key trong bảng điều khiển, sau đó đặt base URL này trong công cụ của bạn. Hướng dẫn thiết lập đầy đủ cho từng hệ điều hành được hiển thị cạnh mỗi khóa.'
    },
    billing: {
      title: 'Cách tính phí',
      description: 'Bạn nạp trước vào số dư. Mỗi yêu cầu được đo bằng token, tính giá theo đơn giá mô hình và trừ vào số dư đó.',
      meter: {
        usage: 'Chỉ số',
        usageValue: 'Số token dùng mỗi yêu cầu',
        rate: 'Đơn giá',
        rateValue: 'Giá mô hình mỗi 1M token',
        multiplier: 'Hệ số',
        multiplierValue: 'Tỷ lệ nhóm của khóa',
        charge: 'Phí',
        chargeValue: 'Trừ vào số dư'
      },
      table: {
        item: 'Mục tính phí',
        unit: 'Đơn vị',
        basis: 'Tính theo',
        perMillion: 'mỗi 1M token'
      },
      lines: {
        input: { item: 'Token đầu vào', basis: 'Giá đầu vào của mô hình' },
        output: { item: 'Token đầu ra', basis: 'Giá đầu ra của mô hình' },
        cacheWrite: { item: 'Token ghi cache', basis: 'Giá ghi cache của mô hình' },
        cacheRead: { item: 'Token đọc cache', basis: 'Giá đọc cache của mô hình' },
        total: { item: 'Phí yêu cầu', basis: 'Tổng các mục × hệ số nhóm' }
      },
      note: 'Đơn giá khác nhau theo mô hình và nhóm. Mọi khoản phí đều được liệt kê chi tiết trong nhật ký sử dụng.',
      rateCardLink: 'Xem bảng giá mô hình'
    },
    footer: {
      allRightsReserved: 'Bảo lưu mọi quyền.'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'Mức sử dụng API Key',
    subtitle: 'Nhập API Key để xem chi tiêu và trạng thái sử dụng theo thời gian thực',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: 'Tra cứu',
    querying: 'Đang tra cứu...',
    privacyNote: 'Khóa của bạn được xử lý cục bộ trong trình duyệt và sẽ không được lưu trữ',
    dateRange: 'Khoảng thời gian:',
    dateRangeToday: 'Hôm nay',
    dateRange7d: '7 ngày',
    dateRange30d: '30 ngày',
    dateRange90d: '90 ngày',
    dateRangeCustom: 'Tùy chỉnh',
    apply: 'Áp dụng',
    used: 'Đã dùng',
    detailInfo: 'Thông tin chi tiết',
    tokenStats: 'Thống kê token',
    dailyDetail: 'Chi tiết theo ngày',
    modelStats: 'Thống kê sử dụng theo mô hình',
    // Table headers
    date: 'Ngày',
    model: 'Mô hình',
    requests: 'Yêu cầu',
    inputTokens: 'Token đầu vào',
    outputTokens: 'Token đầu ra',
    cacheCreationTokens: 'Tạo cache',
    cacheReadTokens: 'Đọc cache',
    cacheWriteTokens: 'Ghi cache',
    totalTokens: 'Tổng token',
    cost: 'Chi phí',
    // Status
    quotaMode: 'Chế độ hạn mức khóa',
    walletBalance: 'Số dư ví',
    statusActive: 'Hoạt động',
    statusQuotaExhausted: 'Hết hạn mức',
    statusExpired: 'Đã hết hạn',
    statusUnknown: 'Không xác định',
    windowDay: 'Ngày',
    windowWeek: 'Tuần',
    windowMonth: 'Tháng',
    // Ring card titles
    totalQuota: 'Tổng hạn mức',
    limit5h: 'Giới hạn 5 giờ',
    limitDaily: 'Giới hạn ngày',
    limit7d: 'Giới hạn 7 ngày',
    limitWeekly: 'Giới hạn tuần',
    limitMonthly: 'Giới hạn tháng',
    // Detail rows
    remainingQuota: 'Hạn mức còn lại',
    expiresAt: 'Hết hạn lúc',
    todayExpires: '(hết hạn hôm nay)',
    daysLeft: '({days} ngày)',
    usedQuota: 'Hạn mức đã dùng',
    resetNow: 'Sắp đặt lại',
    subscriptionType: 'Loại gói đăng ký',
    billingType: 'Loại tính phí',
    subscriptionExpires: 'Gói đăng ký hết hạn',
    // Usage stat cells
    todayRequests: 'Yêu cầu hôm nay',
    todayInputTokens: 'Đầu vào hôm nay',
    todayOutputTokens: 'Đầu ra hôm nay',
    todayTokens: 'Token hôm nay',
    todayCacheCreation: 'Tạo cache hôm nay',
    todayCacheRead: 'Đọc cache hôm nay',
    todayCost: 'Chi phí hôm nay',
    rpmTpm: 'RPM / TPM',
    totalRequests: 'Tổng yêu cầu',
    totalInputTokens: 'Tổng đầu vào',
    totalOutputTokens: 'Tổng đầu ra',
    totalTokensLabel: 'Tổng token',
    totalCacheCreation: 'Tổng tạo cache',
    totalCacheRead: 'Tổng đọc cache',
    totalCost: 'Tổng chi phí',
    avgDuration: 'Thời gian TB',
    // Messages
    enterApiKey: 'Vui lòng nhập API Key',
    querySuccess: 'Tra cứu thành công',
    queryFailed: 'Tra cứu thất bại',
    queryFailedRetry: 'Tra cứu thất bại, vui lòng thử lại sau',
    noDailyUsage: 'Không có dữ liệu sử dụng theo ngày',
  },

  // Setup Wizard
  setup: {
    title: 'Thiết lập Sub2API',
    description: 'Cấu hình phiên bản Sub2API của bạn',
    database: {
      title: 'Cấu hình cơ sở dữ liệu',
      description: 'Kết nối tới cơ sở dữ liệu PostgreSQL',
      host: 'Host',
      port: 'Cổng',
      username: 'Tên người dùng',
      password: 'Mật khẩu',
      databaseName: 'Tên cơ sở dữ liệu',
      sslMode: 'Chế độ SSL',
      passwordPlaceholder: 'Mật khẩu',
      ssl: {
        disable: 'Tắt',
        require: 'Bắt buộc',
        verifyCa: 'Xác minh CA',
        verifyFull: 'Xác minh đầy đủ'
      }
    },
    redis: {
      title: 'Cấu hình Redis',
      description: 'Kết nối tới máy chủ Redis',
      host: 'Host',
      port: 'Cổng',
      username: 'Tên người dùng (tùy chọn)',
      password: 'Mật khẩu (tùy chọn)',
      database: 'Cơ sở dữ liệu',
      usernamePlaceholder: 'Để trống để dùng người dùng mặc định',
      passwordPlaceholder: 'Mật khẩu',
      enableTls: 'Bật TLS',
      enableTlsHint: 'Dùng TLS khi kết nối tới Redis (chứng chỉ CA công khai)'
    },
    admin: {
      title: 'Tài khoản quản trị',
      description: 'Tạo tài khoản quản trị viên',
      email: 'Email',
      password: 'Mật khẩu',
      confirmPassword: 'Xác nhận mật khẩu',
      passwordPlaceholder: 'Tối thiểu 8 ký tự',
      confirmPasswordPlaceholder: 'Xác nhận mật khẩu',
      passwordMismatch: 'Mật khẩu không khớp'
    },
    ready: {
      title: 'Sẵn sàng cài đặt',
      description: 'Xem lại cấu hình và hoàn tất thiết lập',
      database: 'Cơ sở dữ liệu',
      redis: 'Redis',
      adminEmail: 'Email quản trị'
    },
    status: {
      testing: 'Đang kiểm tra...',
      success: 'Kết nối thành công',
      testConnection: 'Kiểm tra kết nối',
      installing: 'Đang cài đặt...',
      completeInstallation: 'Hoàn tất cài đặt',
      completed: 'Cài đặt hoàn tất!',
      redirecting: 'Đang chuyển tới trang đăng nhập...',
      restarting: 'Dịch vụ đang khởi động lại, vui lòng chờ...',
      timeout: 'Khởi động lại dịch vụ lâu hơn dự kiến. Vui lòng làm mới trang thủ công.'
    }
  },

  // Common
}
