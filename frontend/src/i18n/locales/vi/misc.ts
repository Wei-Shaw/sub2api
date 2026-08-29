import { mergeLocaleMessages } from './merge'

const base = {
  // Subscription Progress (Header component)
subscriptionProgress: {
  title: 'Gói đăng ký của tôi',
  viewDetails: 'Xem chi tiết gói đăng ký',
  activeCount: '{count} gói đăng ký còn hiệu lực',
  daily: 'Hàng ngày',
  weekly: 'Hàng tuần',
  monthly: 'Hàng tháng',
  daysRemaining: 'Còn {days} ngày',
  expired: 'Đã hết hạn',
  expiresToday: 'Hết hạn hôm nay',
  expiresTomorrow: 'Hết hạn ngày mai',
  viewAll: 'Xem tất cả gói đăng ký',
  noSubscriptions: 'Chưa có gói đăng ký nào còn hiệu lực',
  unlimited: 'Không giới hạn'
  },

  // Version Badge
version: {
  currentVersion: 'Phiên bản hiện tại',
  latestVersion: 'Phiên bản mới nhất',
  upToDate: 'Đã là phiên bản mới nhất',
  updateAvailable: 'Có phiên bản mới khả dụng!',
  releaseNotes: 'Ghi chú phát hành',
  noReleaseNotes: 'Chưa có ghi chú phát hành',
  viewUpdate: 'Xem bản cập nhật',
  viewRelease: 'Xem bản phát hành',
  viewChangelog: 'Xem changelog',
  refresh: 'Làm mới',
  sourceMode: 'Bản dựng từ mã nguồn',
  sourceModeHint: 'Với bản dựng từ mã nguồn, hãy dùng git pull để cập nhật',
  updateNow: 'Cập nhật ngay',
  updating: 'Đang cập nhật...',
  updateComplete: 'Cập nhật hoàn tất',
  updateFailed: 'Cập nhật thất bại',
  restartRequired: 'Vui lòng khởi động lại dịch vụ để áp dụng bản cập nhật',
  restartNow: 'Khởi động lại ngay',
  restarting: 'Đang khởi động lại...',
  retry: 'Thử lại'
  },

  // Recharge / Subscription Page
purchase: {
  title: 'Nạp tiền / Đăng ký',
  description: 'Hoàn tất nạp tiền / đăng ký qua trang nhúng',
  openInNewTab: 'Mở ở tab mới',
  notEnabledTitle: 'Tính năng này chưa được bật',
  notEnabledDesc: 'Quản trị viên hiện chưa bật cổng nạp tiền / đăng ký, vui lòng liên hệ quản trị viên.',
  notConfiguredTitle: 'Liên kết nạp tiền / đăng ký chưa được cấu hình',
  notConfiguredDesc: 'Quản trị viên đã bật lối vào nhưng chưa cấu hình URL nạp tiền / đăng ký. Vui lòng liên hệ quản trị viên.'
  },

  // Custom Page (iframe embed)
customPage: {
  title: 'Trang tùy chỉnh',
  openInNewTab: 'Mở ở tab mới',
  notFoundTitle: 'Trang không tồn tại',
  notFoundDesc: 'Trang tùy chỉnh này không tồn tại hoặc đã bị xóa.',
  notConfiguredTitle: 'Liên kết trang chưa được cấu hình',
  notConfiguredDesc: 'URL của trang tùy chỉnh này chưa được cấu hình đúng.',
  tableOfContents: 'Mục lục',
  markdownNotFound: 'Không tìm thấy trang',
  markdownLoadFailed: 'Tải trang thất bại',
  },

  // Legal Document Page
legalDocument: {
  loadFailedTitle: 'Tải tài liệu thất bại',
  loadFailedDesc: 'Vui lòng làm mới trang và thử lại sau.',
  notFoundTitle: 'Không tìm thấy tài liệu',
  notFoundDesc: 'Tài liệu điều khoản hiện tại không tồn tại hoặc đã bị quản trị viên xóa.',
  loginTerms: 'Điều khoản đăng nhập',
  updatedAt: 'Cập nhật vào: {date}',
  emptyContent: 'Chưa có nội dung'
  },

  // Announcements Page
announcements: {
  title: 'Thông báo',
  description: 'Xem thông báo hệ thống',
  unreadOnly: 'Chỉ hiển thị chưa đọc',
  markRead: 'Đánh dấu đã đọc',
  markAllRead: 'Đánh dấu tất cả đã đọc',
  viewAll: 'Xem tất cả thông báo',
  markedAsRead: 'Đã đánh dấu là đã đọc',
  allMarkedAsRead: 'Tất cả thông báo đã được đánh dấu là đã đọc',
  newCount: '{count} thông báo mới | {count} thông báo mới',
  readAt: 'Thời gian đã đọc',
  read: 'Đã đọc',
  unread: 'Chưa đọc',
  startsAt: 'Thời gian bắt đầu',
  endsAt: 'Thời gian kết thúc',
  empty: 'Chưa có thông báo nào',
  emptyUnread: 'Chưa có thông báo chưa đọc nào',
  total: 'thông báo',
  emptyDescription: 'Hiện chưa có thông báo hệ thống nào',
  readStatus: 'Bạn đã đọc thông báo này',
  markReadHint: 'Bấm "Đã đọc" để đánh dấu thông báo này'
  },

  // User Subscriptions Page
userSubscriptions: {
  title: 'Gói đăng ký của tôi',
  description: 'Xem gói đăng ký và mức sử dụng của bạn',
  noActiveSubscriptions: 'Chưa có gói đăng ký nào còn hiệu lực',
  noActiveSubscriptionsDesc: 'Bạn hiện không có gói đăng ký nào còn hiệu lực. Vui lòng liên hệ quản trị viên để được cấp gói đăng ký.',
  failedToLoad: 'Tải gói đăng ký thất bại',
  status: {
    active: 'Còn hiệu lực',
    expired: 'Đã hết hạn',
    revoked: 'Đã thu hồi'
  },
  usage: 'Mức sử dụng',
  expires: 'Thời gian hết hạn',
  noExpiration: 'Không có thời gian hết hạn',
  unlimited: 'Không giới hạn',
  unlimitedDesc: 'Gói đăng ký này không có giới hạn sử dụng',
  daily: 'Hàng ngày',
  weekly: 'Hàng tuần',
  monthly: 'Hàng tháng',
  daysRemaining: 'Còn {days} ngày',
  expiresOn: 'Hết hạn vào {date}',
  resetIn: 'Đặt lại sau {time}',
  windowNotActive: 'Đang chờ lần sử dụng đầu tiên',
  usageOf: 'Đã dùng {used} / {limit}'
  },

  // Onboarding Tour
onboarding: {
  restartTour: 'Xem lại hướng dẫn cho người mới',
  dontShowAgain: 'Không nhắc lại nữa',
  dontShowAgainTitle: 'Tắt vĩnh viễn hướng dẫn cho người mới',
  confirmDontShow: 'Bạn có chắc muốn ngừng hiển thị hướng dẫn cho người mới không?\n\nBạn có thể bật lại bất cứ lúc nào từ menu ảnh đại diện ở góc trên bên phải.',
  confirmExit: 'Bạn có chắc muốn thoát hướng dẫn cho người mới không? Bạn có thể bắt đầu lại bất cứ lúc nào từ menu góc trên bên phải.',
  interactiveHint: 'Nhấn Enter hoặc bấm để tiếp tục',
  navigation: {
    flipPage: 'Lật trang',
    exit: 'Thoát'
  },
  // Admin tour steps
  admin: {
    welcome: {
      title: '👋 Chào mừng bạn đến với Sub2API',
      description:
        '<div style="line-height: 1.8;"><p style="margin-bottom: 16px;">Sub2API là một nền tảng trung chuyển dịch vụ AI mạnh mẽ, giúp bạn dễ dàng quản lý và phân phối các dịch vụ AI.</p><p style="margin-bottom: 12px;"><b>🎯 Chức năng cốt lõi:</b></p><ul style="margin-left: 20px; margin-bottom: 16px;"><li>📦 <b>Quản lý nhóm</b> - tạo các gói dịch vụ khác nhau (VIP, dùng thử miễn phí, v.v.)</li><li>🔗 <b>Pool tài khoản</b> - kết nối nhiều tài khoản nhà cung cấp AI upstream</li><li>🔑 <b>Phân phối khóa</b> - tạo API Key độc lập cho người dùng</li><li>💰 <b>Quản lý tính phí</b> - kiểm soát linh hoạt hệ số giá và hạn ngạch</li></ul><p style="color: #10b981; font-weight: 600;">Tiếp theo, chúng tôi sẽ giúp bạn hoàn tất cấu hình lần đầu chỉ trong 3 phút →</p></div>',
      nextBtn: 'Bắt đầu cấu hình 🚀',
      prevBtn: 'Bỏ qua'
    },
    groupManage: {
      title: '📦 Bước 1: Quản lý nhóm',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;"><b>Nhóm là gì?</b></p><p style="margin-bottom: 12px;">Nhóm là khái niệm cốt lõi của Sub2API, giống như một "gói dịch vụ":</p><ul style="margin-left: 20px; margin-bottom: 12px; font-size: 13px;"><li>🎯 Mỗi nhóm có thể chứa nhiều tài khoản upstream</li><li>💰 Mỗi nhóm có hệ số tính phí riêng</li><li>👥 Có thể đặt thành nhóm công khai hoặc nhóm riêng</li></ul><p style="margin-top: 12px; padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Ví dụ:</b> Bạn có thể tạo hai nhóm là "Tuyến VIP" (hệ số cao) và "Dùng thử miễn phí" (hệ số thấp)</p><p style="margin-top: 16px; color: #10b981; font-weight: 600;">👉 Bấm "Quản lý nhóm" ở bên trái để bắt đầu</p></div>'
    },
    createGroup: {
      title: '➕ Tạo nhóm mới',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Bây giờ hãy tạo nhóm đầu tiên của bạn.</p><p style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📝 Gợi ý:</b> Nên tạo một nhóm thử nghiệm trước, làm quen quy trình rồi mới tạo nhóm chính thức</p><p style="color: #10b981; font-weight: 600;">👉 Bấm nút "Tạo nhóm"</p></div>'
    },
    groupName: {
      title: '✏️ 1. Tên nhóm',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Đặt cho nhóm của bạn một tên dễ nhận biết.</p><div style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>💡 Gợi ý đặt tên:</b><ul style="margin: 8px 0 0 16px;"><li>"Nhóm thử nghiệm" - dùng để test</li><li>"Tuyến VIP" - dịch vụ chất lượng cao</li><li>"Dùng thử miễn phí" - bản trải nghiệm</li></ul></div><p style="font-size: 13px; color: #6b7280;">Sau khi điền xong, bấm "Tiếp theo" để tiếp tục</p></div>',
      nextBtn: 'Tiếp theo'
    },
    groupPlatform: {
      title: '🤖 2. Chọn nền tảng',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Chọn nền tảng AI mà nhóm này hỗ trợ.</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📌 Giải thích nền tảng:</b><ul style="margin: 8px 0 0 16px;"><li><b>Anthropic</b> - dòng model Claude</li><li><b>OpenAI</b> - dòng model GPT</li><li><b>Google</b> - dòng model Gemini</li></ul></div><p style="font-size: 13px; color: #6b7280;">Mỗi nhóm chỉ có thể chọn một nền tảng</p></div>',
      nextBtn: 'Tiếp theo'
    },
    groupMultiplier: {
      title: '💰 3. Hệ số giá',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Thiết lập hệ số tính phí cho nhóm này để kiểm soát mức khấu trừ thực tế của người dùng.</p><div style="padding: 8px 12px; background: #fef3c7; border-left: 3px solid #f59e0b; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚙️ Quy tắc tính phí:</b><ul style="margin: 8px 0 0 16px;"><li><b>1.0</b> - tính giá gốc (giá vốn)</li><li><b>1.5</b> - người dùng tiêu tốn $1, hệ thống khấu trừ $1.5</li><li><b>2.0</b> - người dùng tiêu tốn $1, hệ thống khấu trừ $2</li><li><b>0.8</b> - chế độ trợ giá (vận hành lỗ)</li></ul></div><p style="font-size: 13px; color: #6b7280;">Khuyến nghị đặt nhóm thử nghiệm ở mức 1.0</p></div>',
      nextBtn: 'Tiếp theo'
    },
    groupExclusive: {
      title: '🔒 4. Nhóm riêng (tùy chọn)',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Kiểm soát khả năng hiển thị và quyền truy cập của nhóm.</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>🔐 Giải thích quyền hạn:</b><ul style="margin: 8px 0 0 16px;"><li><b>Tắt</b> - nhóm công khai, mọi người dùng đều thấy</li><li><b>Bật</b> - nhóm riêng, chỉ người dùng được chỉ định mới thấy</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Trường hợp sử dụng:</b> người dùng VIP, kiểm thử nội bộ, khách hàng đặc biệt...</p></div>',
      nextBtn: 'Tiếp theo'
    },
    groupSubmit: {
      title: '✅ Lưu nhóm',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Sau khi xác nhận thông tin là chính xác, bấm nút tạo để lưu nhóm.</p><p style="padding: 8px 12px; background: #fef3c7; border-left: 3px solid #f59e0b; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚠️ Lưu ý:</b> sau khi nhóm được tạo, loại nền tảng sẽ không thể chỉnh sửa, nhưng các thông tin khác vẫn có thể sửa bất cứ lúc nào</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>📌 Bước tiếp theo:</b> sau khi tạo thành công, chúng ta sẽ thêm tài khoản upstream vào nhóm này</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 Bấm nút "Tạo"</p></div>'
    },
    accountManage: {
      title: '🔗 Bước 2: Thêm tài khoản',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;"><b>Tuyệt vời! Nhóm đã được tạo thành công 🎉</b></p><p style="margin-bottom: 12px;">Bây giờ bạn cần thêm tài khoản của nhà cung cấp AI upstream để nhóm có thể thực sự cung cấp dịch vụ.</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>🔑 Vai trò của tài khoản:</b><ul style="margin: 8px 0 0 16px;"><li>Kết nối tới các dịch vụ AI upstream (Claude, GPT, v.v.)</li><li>Một nhóm có thể chứa nhiều tài khoản để cân bằng tải</li><li>Hỗ trợ cả OAuth và Session Key</li></ul></div><p style="margin-top: 16px; color: #10b981; font-weight: 600;">👉 Bấm "Quản lý tài khoản" ở bên trái</p></div>'
    },
    createAccount: {
      title: '➕ Thêm tài khoản mới',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Bấm nút để bắt đầu thêm tài khoản upstream đầu tiên của bạn.</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Gợi ý:</b> khuyến nghị dùng OAuth để an toàn hơn và không cần trích xuất khóa thủ công</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 Bấm nút "Thêm tài khoản"</p></div>'
    },
    accountName: {
      title: '✏️ 1. Tên tài khoản',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Đặt cho tài khoản một tên dễ nhận biết.</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Gợi ý đặt tên:</b> "Tài khoản Claude chính", "GPT dự phòng 1", "Tài khoản thử nghiệm", v.v.</p></div>',
      nextBtn: 'Tiếp theo'
    },
    accountPlatform: {
      title: '🤖 2. Chọn nền tảng',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Chọn nền tảng nhà cung cấp tương ứng với tài khoản này.</p><p style="padding: 8px 12px; background: #fef3c7; border-left: 3px solid #f59e0b; border-radius: 4px; font-size: 13px;"><b>⚠️ Quan trọng:</b> nền tảng phải trùng với nền tảng của nhóm vừa tạo</p></div>',
      nextBtn: 'Tiếp theo'
    },
    accountType: {
      title: '🔐 3. Phương thức ủy quyền',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Chọn phương thức ủy quyền cho tài khoản.</p><div style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>✅ Khuyến nghị: OAuth</b><ul style="margin: 8px 0 0 16px;"><li>Không cần trích xuất khóa thủ công</li><li>An toàn hơn, hỗ trợ tự động làm mới</li><li>Phù hợp với Claude Code, ChatGPT OAuth</li></ul></div><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px;"><b>📌 Session Key</b><ul style="margin: 8px 0 0 16px;"><li>Cần tự trích xuất từ trình duyệt</li><li>Có thể phải cập nhật định kỳ</li><li>Phù hợp với các nền tảng không hỗ trợ OAuth</li></ul></div></div>',
      nextBtn: 'Tiếp theo'
    },
    accountPriority: {
      title: '⚖️ 4. Mức ưu tiên (tùy chọn)',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Thiết lập mức ưu tiên gọi cho tài khoản.</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📊 Quy tắc ưu tiên:</b><ul style="margin: 8px 0 0 16px;"><li>Số càng nhỏ, ưu tiên càng cao</li><li>Hệ thống sẽ ưu tiên dùng tài khoản có giá trị thấp hơn</li><li>Nếu cùng mức ưu tiên thì chọn ngẫu nhiên</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Trường hợp sử dụng:</b> đặt tài khoản chính ở giá trị thấp, tài khoản dự phòng ở giá trị cao</p></div>',
      nextBtn: 'Tiếp theo'
    },
    accountGroups: {
      title: '🎯 5. Gán nhóm',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;"><b>Bước then chốt!</b> Gán tài khoản vào nhóm bạn vừa tạo.</p><div style="padding: 8px 12px; background: #fee2e2; border-left: 3px solid #ef4444; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚠️ Nhắc nhở quan trọng:</b><ul style="margin: 8px 0 0 16px;"><li>Phải chọn ít nhất một nhóm</li><li>Tài khoản chưa được gán nhóm sẽ không thể sử dụng</li><li>Một tài khoản có thể được gán cho nhiều nhóm</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Gợi ý:</b> hãy chọn nhóm thử nghiệm bạn vừa tạo</p></div>',
      nextBtn: 'Tiếp theo'
    },
    accountSubmit: {
      title: '✅ Lưu tài khoản',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Sau khi xác nhận thông tin là chính xác, bấm nút lưu.</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📌 Quy trình ủy quyền OAuth:</b><ul style="margin: 8px 0 0 16px;"><li>Sau khi bấm lưu, bạn sẽ được chuyển tới trang của nhà cung cấp</li><li>Hoàn tất đăng nhập và ủy quyền tại đó</li><li>Sau khi ủy quyền thành công, hệ thống sẽ tự động quay lại</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>📌 Bước tiếp theo:</b> sau khi thêm tài khoản thành công, chúng ta sẽ tạo API Key</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 Bấm nút "Lưu"</p></div>'
    },
    keyManage: {
      title: '🔑 Bước 3: Tạo khóa',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;"><b>Chúc mừng! Việc cấu hình tài khoản đã hoàn tất 🎉</b></p><p style="margin-bottom: 12px;">Bước cuối cùng là tạo API Key để kiểm tra xem dịch vụ có hoạt động bình thường hay không.</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>🔑 Vai trò của API Key:</b><ul style="margin: 8px 0 0 16px;"><li>Là thông tin xác thực để gọi dịch vụ AI</li><li>Mỗi Key được gắn với một nhóm</li><li>Có thể thiết lập hạn ngạch và thời hạn hiệu lực</li><li>Hỗ trợ thống kê sử dụng độc lập</li></ul></div><p style="margin-top: 16px; color: #10b981; font-weight: 600;">👉 Bấm "API Key" ở bên trái</p></div>'
    },
    createKey: {
      title: '➕ Tạo khóa',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Bấm nút để tạo API Key đầu tiên của bạn.</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Gợi ý:</b> sau khi tạo, hãy sao chép và lưu lại ngay vì khóa chỉ hiển thị một lần</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 Bấm nút "Tạo khóa"</p></div>'
    },
    keyName: {
      title: '✏️ 1. Tên khóa',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Đặt cho khóa một tên dễ quản lý.</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Gợi ý đặt tên:</b> "Khóa thử nghiệm", "Môi trường production", "Thiết bị di động", v.v.</p></div>',
      nextBtn: 'Tiếp theo'
    },
    keyGroup: {
      title: '🎯 2. Chọn nhóm',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Chọn nhóm mà bạn vừa cấu hình.</p><div style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>📌 Nhóm quyết định:</b><ul style="margin: 8px 0 0 16px;"><li>Khóa này có thể dùng những tài khoản nào</li><li>Hệ số tính phí là bao nhiêu</li><li>Có phải là khóa riêng hay không</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Gợi ý:</b> chọn nhóm thử nghiệm mà bạn vừa tạo</p></div>',
      nextBtn: 'Tiếp theo'
    },
    keySubmit: {
      title: '🎉 Tạo và sao chép',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Sau khi bấm tạo, hệ thống sẽ sinh ra API Key hoàn chỉnh.</p><div style="padding: 8px 12px; background: #fee2e2; border-left: 3px solid #ef4444; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚠️ Nhắc nhở quan trọng:</b><ul style="margin: 8px 0 0 16px;"><li>Khóa chỉ hiển thị một lần, hãy sao chép ngay</li><li>Nếu làm mất thì phải tạo lại</li><li>Hãy bảo quản cẩn thận, không chia sẻ cho người khác</li></ul></div><div style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>🚀 Bước tiếp theo:</b><ul style="margin: 8px 0 0 16px;"><li>Sao chép khóa sk-xxx vừa tạo</li><li>Dùng trong các client hỗ trợ giao diện OpenAI</li><li>Bắt đầu trải nghiệm dịch vụ AI!</li></ul></div><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 Bấm nút "Tạo"</p></div>'
    }
  },
  // User tour steps
  user: {
    welcome: {
      title: '👋 Chào mừng bạn đến với Sub2API',
      description:
        '<div style="line-height: 1.8;"><p style="margin-bottom: 16px;">Xin chào! Chào mừng bạn đến với nền tảng dịch vụ AI Sub2API.</p><p style="margin-bottom: 12px;"><b>🎯 Bắt đầu nhanh:</b></p><ul style="margin-left: 20px; margin-bottom: 16px;"><li>🔑 Tạo API Key</li><li>📋 Sao chép khóa vào ứng dụng của bạn</li><li>🚀 Bắt đầu sử dụng dịch vụ AI</li></ul><p style="color: #10b981; font-weight: 600;">Chỉ cần 1 phút, bắt đầu thôi →</p></div>',
      nextBtn: 'Bắt đầu 🚀',
      prevBtn: 'Bỏ qua'
    },
    keyManage: {
      title: '🔑 Quản lý API Key',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Tại đây bạn có thể quản lý tất cả API Key truy cập của mình.</p><p style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px;"><b>📌 API Key là gì?</b><br/>API Key là thông tin xác thực để truy cập dịch vụ AI, giống như một chiếc chìa khóa giúp ứng dụng của bạn gọi được các khả năng AI.</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 Bấm để vào trang khóa</p></div>'
    },
    createKey: {
      title: '➕ Tạo khóa mới',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Bấm nút để tạo API Key đầu tiên của bạn.</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Gợi ý:</b> sau khi tạo, khóa chỉ hiển thị một lần nên nhớ sao chép và lưu lại</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 Bấm "Tạo khóa"</p></div>'
    },
    keyName: {
      title: '✏️ Tên khóa',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Đặt cho khóa một tên dễ nhận biết.</p><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>💡 Ví dụ:</b> "Khóa đầu tiên của tôi", "Dùng để test", v.v.</p></div>',
      nextBtn: 'Tiếp theo'
    },
    keyGroup: {
      title: '🎯 Chọn nhóm',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Chọn nhóm dịch vụ mà quản trị viên đã cấp cho bạn.</p><p style="padding: 8px 12px; background: #eff6ff; border-left: 3px solid #3b82f6; border-radius: 4px; font-size: 13px;"><b>📌 Giải thích về nhóm:</b><br/>Mỗi nhóm có thể có chất lượng dịch vụ và tiêu chuẩn tính phí khác nhau, hãy chọn theo nhu cầu của bạn.</p></div>',
      nextBtn: 'Tiếp theo'
    },
    keySubmit: {
      title: '🎉 Hoàn tất tạo',
      description:
        '<div style="line-height: 1.7;"><p style="margin-bottom: 12px;">Bấm xác nhận để tạo API Key của bạn.</p><div style="padding: 8px 12px; background: #fee2e2; border-left: 3px solid #ef4444; border-radius: 4px; font-size: 13px; margin-bottom: 12px;"><b>⚠️ Quan trọng:</b><ul style="margin: 8px 0 0 16px;"><li>Sau khi tạo, hãy sao chép ngay khóa (sk-xxx)</li><li>Khóa chỉ hiển thị một lần, nếu mất phải tạo lại</li></ul></div><p style="padding: 8px 12px; background: #f0fdf4; border-left: 3px solid #10b981; border-radius: 4px; font-size: 13px;"><b>🚀 Cách sử dụng:</b><br/>Cấu hình khóa vào bất kỳ client nào hỗ trợ giao diện OpenAI (như ChatBox, OpenCat, v.v.) là bạn có thể bắt đầu dùng ngay!</p><p style="margin-top: 12px; color: #10b981; font-weight: 600;">👉 Bấm nút "Tạo"</p></div>'
    }
  }
  },

  // Payment System
payment: {
  title: 'Nạp tiền / Đăng ký',
  amountLabel: 'Số tiền nạp',
  paymentAmount: 'Số tiền thanh toán',
  paymentCurrency: 'Đồng tiền thanh toán',
  creditedBalance: 'Số dư nhận được',
  quickAmounts: 'Mức tiền nhanh',
  customAmount: 'Số tiền tùy chỉnh',
  enterAmount: 'Nhập số tiền',
  airwallexPay: 'Thanh toán Airwallex',
  airwallexLoadFailed: 'Không tải được Airwallex',
  airwallexMissingParams: 'Thiếu tham số thanh toán Airwallex',
  paymentMethod: 'Phương thức thanh toán',
  fee: 'Phí xử lý',
  actualPay: 'Số tiền thực trả',
  createOrder: 'Xác nhận thanh toán',
  methods: {
    easypay: 'YiPay',
    alipay: 'Alipay',
    wxpay: 'WeChat Pay',
    stripe: 'Stripe',
    airwallex: 'Airwallex',
    sepay: 'SePay',
    paddle: 'Paddle',
    card: 'Thẻ ngân hàng',
    link: 'Link',
    alipay_direct: 'Alipay (direct)',
    wxpay_direct: 'WeChat Pay (direct)',
  },
  status: {
    pending: 'Chờ thanh toán',
    paid: 'Đã thanh toán',
    recharging: 'Đang nạp tiền',
    completed: 'Đã hoàn tất',
    expired: 'Đã hết hạn',
    cancelled: 'Đã hủy',
    failed: 'Thất bại',
    refund_requested: 'Đang yêu cầu hoàn tiền',
    refunding: 'Đang hoàn tiền',
    refunded: 'Đã hoàn tiền',
    partially_refunded: 'Hoàn tiền một phần',
    refund_failed: 'Hoàn tiền thất bại',
  },
  qr: {
    scanToPay: 'Vui lòng quét mã để thanh toán',
    scanAlipay: 'Quét mã Alipay để thanh toán',
    scanWxpay: 'Quét mã WeChat Pay để thanh toán',
    scanAlipayHint: 'Vui lòng mở Alipay trên điện thoại và quét mã QR để hoàn tất thanh toán',
    scanWxpayHint: 'Vui lòng mở WeChat trên điện thoại và quét mã QR để hoàn tất thanh toán',
    payInNewWindow: 'Vui lòng hoàn tất thanh toán ở cửa sổ mới',
    payInNewWindowHint: 'Trang thanh toán đã được mở ở cửa sổ mới, vui lòng hoàn tất thanh toán rồi quay lại trang này',
    openPayWindow: 'Mở lại trang thanh toán',
    expiresIn: 'Thời gian thanh toán còn lại',
    expired: 'Đơn hàng đã hết hạn',
    expiredDesc: 'Đơn hàng đã hết thời gian chờ, vui lòng tạo lại đơn mới',
    cancelled: 'Đơn hàng đã bị hủy',
    cancelledDesc: 'Bạn đã hủy lần thanh toán này',
    waitingPayment: 'Đang chờ thanh toán...',
    cancelOrder: 'Hủy đơn hàng',
  },
  orders: {
    title: 'Đơn hàng của tôi',
    empty: 'Chưa có đơn hàng nào',
    orderId: 'ID đơn hàng',
    orderNo: 'Mã đơn hàng',
    amount: 'Số tiền',
    creditedAmount: 'Số tiền ghi có',
    fee: 'Phí xử lý',
    baseAmount: 'Số tiền nạp',
    includedInPayAmount: 'Đã bao gồm trong số tiền thực trả',
    status: 'Trạng thái',
    paymentMethod: 'Phương thức thanh toán',
    createdAt: 'Thời gian tạo',
    cancel: 'Hủy đơn hàng',
    userId: 'ID người dùng',
    deviceCode: 'Mã thiết bị',
    orderType: 'Loại đơn hàng',
    actions: 'Thao tác',
    requestRefund: 'Yêu cầu hoàn tiền',
  },
  result: {
    success: 'Thanh toán thành công',
    subscriptionSuccess: 'Đăng ký thành công',
    processing: 'Thanh toán đang được xử lý',
    processingHint: 'Kết quả thanh toán vẫn đang được xác nhận, trang sẽ tự động làm mới.',
    failed: 'Thanh toán thất bại',
    backToRecharge: 'Quay lại nạp tiền',
    viewOrders: 'Xem đơn hàng',
  },
  currentBalance: 'Số dư hiện tại',
  groupFallback: 'Nhóm #{id}',
  rechargeAccount: 'Tài khoản nạp tiền',
  activeSubscription: 'Gói đăng ký hiện tại',
  noActiveSubscription: 'Chưa có gói đăng ký nào còn hiệu lực',
  tabTopUp: 'Nạp tiền',
  tabSubscribe: 'Đăng ký',
  noPlans: 'Chưa có gói đăng ký nào khả dụng',
  notAvailable: 'Tính năng nạp tiền hiện chưa được mở',
  confirmSubscription: 'Xác nhận đăng ký',
  confirmCancel: 'Bạn có chắc muốn hủy đơn hàng này không?',
  amountTooLow: 'Số tiền tối thiểu là {min}',
  amountTooHigh: 'Số tiền tối đa là {max}',
  amountNoMethod: 'Không có phương thức thanh toán khả dụng cho số tiền này',
  rechargeRatePreview: 'Tỷ giá hiện tại: 1 CNY = {usd} USD',
  fxRateMissing: 'Đang thiếu tỷ giá cho: {currencies}. Vui lòng liên hệ hỗ trợ trước khi thanh toán.',
  fxRateStale: 'Tỷ giá từ {source} có thể đã cũ; hệ thống sẽ khóa quote tại thời điểm thanh toán.',
  fxRateUpdated: 'Tỷ giá từ {source}, cập nhật lúc {time}.',
  refundReason: 'Lý do hoàn tiền',
  refundReasonPlaceholder: 'Vui lòng mô tả lý do hoàn tiền của bạn',
  stripeLoadFailed: 'Tải thành phần thanh toán thất bại, vui lòng làm mới trang và thử lại',
  stripeMissingParams: 'Thiếu ID đơn hàng hoặc khóa thanh toán',
  stripeNotConfigured: 'Stripe chưa được cấu hình',
  paddleLoading: 'Đang tải cổng thanh toán Paddle...',
  paddleCheckoutReady: 'Cổng thanh toán Paddle đã sẵn sàng. Nếu chưa tự mở, hãy bấm nút bên dưới.',
  paddleWaitingWebhook: 'Thanh toán đã được gửi. Hệ thống đang chờ Paddle xác nhận webhook...',
  paddleLoadFailed: 'Không thể tải cổng thanh toán Paddle, vui lòng làm mới trang và thử lại.',
  paddleNotConfigured: 'Paddle chưa được cấu hình',
  hostedCheckout: {
    provider: 'Thanh toán bảo mật qua {provider}',
    title: 'Xem lại đơn hàng trước khi thanh toán',
    orderSummary: 'Thông tin đơn hàng',
    product: 'Nội dung thanh toán',
    amountDue: 'Số tiền cần thanh toán',
    creditedBalance: 'Số dư sẽ được ghi có',
    orderNo: 'Mã đơn hàng',
    expiresAt: 'Hết hạn lúc',
    openButton: 'Mở cổng thanh toán Paddle',
    reopenButton: 'Mở lại cổng thanh toán Paddle',
    secureHint: 'Bạn sẽ nhập thông tin thẻ và hoàn tất thanh toán trong khung checkout bảo mật của Paddle. Sau khi thanh toán, hệ thống sẽ tự xác nhận đơn hàng.',
    missingOrderDetails: 'Đang thiếu thông tin chi tiết đơn hàng. Bạn vẫn có thể mở Paddle, nhưng nên kiểm tra lại mã đơn hoặc liên hệ hỗ trợ nếu số tiền không rõ ràng.',
    topUpProduct: 'Nạp số dư tài khoản V-Claw',
    subscriptionProduct: 'Đăng ký gói dịch vụ V-Claw',
    orderTypes: {
      balance: 'Nạp số dư',
      subscription: 'Đăng ký',
    },
    status: {
      loading: 'Đang tải',
      ready: 'Sẵn sàng',
      completed: 'Đang xử lý',
      failed: 'Lỗi',
    },
  },
  errors: {
    tooManyPending: 'Có quá nhiều đơn hàng đang chờ thanh toán (tối đa {max}), vui lòng hoàn tất hoặc hủy các đơn hiện có trước',
    cancelRateLimited: 'Bạn đang hủy đơn quá thường xuyên, vui lòng thử lại sau',
    wechatH5NotAuthorized: 'Merchant hiện tại chưa bật thanh toán WeChat H5, vui lòng mở trang này trong WeChat để tiếp tục thanh toán.',
    wechatPaymentMpNotConfigured: 'Trang hiện tại chưa hoàn tất cấu hình thanh toán Official Account/JSAPI nên tạm thời không thể gọi thanh toán trực tiếp trong WeChat.',
    wechatJsapiUnavailable: 'Môi trường hiện tại không thể gọi WeChat Pay, vui lòng xác nhận bạn đang mở trang này trong WeChat rồi thử lại.',
    wechatJsapiFailed: 'Thanh toán WeChat chưa hoàn tất, vui lòng gọi lại thanh toán hoặc chuyển sang quét mã.',
    wechatUnavailable: 'WeChat Pay hiện tạm thời không khả dụng, vui lòng thử lại sau.',
    wechatOpenInWeChatHint: 'Vui lòng sao chép liên kết trang này để mở trong WeChat, hoặc chuyển sang quét mã bằng WeChat trên máy tính.',
    wechatScanOnDesktopHint: 'Trên máy tính hãy dùng WeChat Scan trực tiếp để thanh toán; trên di động hãy mở trang này trong WeChat.',
    wechatSwitchBrowserHint: 'Vui lòng chuyển sang quét mã bằng WeChat trên máy tính, hoặc mở lại trang này trong trình duyệt ngoài rồi thử lại.',
    mobilePaymentFallbackToQr: 'Merchant hiện tại chưa bật thanh toán di động, hệ thống đã tự động chuyển sang thanh toán bằng mã QR.',
    alipayDesktopUnavailable: 'Thanh toán Alipay trên desktop hiện không tạo được mã QR thành công.',
    alipayDesktopQrHint: 'Alipay trên máy tính lẽ ra phải hiển thị mã quét, vui lòng làm mới rồi thử lại, hoặc kiểm tra xem trình duyệt có đang chặn trang thanh toán hiện tại hay không.',
    alipayMobileUnavailable: 'Trang hiện tại chưa chuyển sang Alipay thành công.',
    alipayMobileOpenHint: 'Vui lòng cho phép trang này mở ứng dụng Alipay, hoặc dùng trình duyệt hệ thống để khởi tạo thanh toán lại.',
    // Structured error codes (reason strings from backend ApplicationError)
    PAYMENT_DISABLED: 'Hệ thống thanh toán đã bị tắt',
    USER_INACTIVE: 'Tài khoản đã bị vô hiệu hóa',
    BALANCE_PAYMENT_DISABLED: 'Tính năng nạp số dư đã bị tắt',
    INVALID_AMOUNT: 'Số tiền không hợp lệ',
    INVALID_INPUT: 'Tham số không hợp lệ',
    PLAN_NOT_AVAILABLE: 'Gói không tồn tại hoặc đã bị gỡ khỏi bán',
    GROUP_NOT_FOUND: 'Nhóm đăng ký không khả dụng',
    GROUP_TYPE_MISMATCH: 'Loại nhóm không phải nhóm đăng ký',
    TOO_MANY_PENDING: 'Có quá nhiều đơn hàng đang chờ thanh toán (tối đa {max}), vui lòng hoàn tất hoặc hủy các đơn hiện có trước',
    DAILY_LIMIT_EXCEEDED: 'Hôm nay đã đạt giới hạn nạp tiền, hạn mức còn lại là {remaining}',
    PAYMENT_GATEWAY_ERROR: 'Phương thức thanh toán hiện không khả dụng',
    NO_AVAILABLE_INSTANCE: 'Hiện không có kênh thanh toán nào khả dụng',
    PAYMENT_PROVIDER_MISCONFIGURED: 'Kênh thanh toán đang cấu hình sai, vui lòng liên hệ quản trị viên',
    WXPAY_CONFIG_MISSING_KEY: 'Cấu hình WeChat Pay thiếu trường bắt buộc: {key}',
    WXPAY_CONFIG_INVALID_KEY_LENGTH: 'Độ dài của {key} trong WeChat Pay không đúng, phải là {expected} byte (thực tế {actual})',
    WXPAY_CONFIG_INVALID_KEY: 'Định dạng {key} của WeChat Pay không đúng, vui lòng xác nhận bạn đã sao chép đầy đủ nội dung PEM',
    PENDING_ORDERS: 'Nhà cung cấp này vẫn có đơn hàng chưa hoàn tất, vui lòng đợi xử lý xong rồi thao tác tiếp',
    PAYMENT_PROVIDER_CONFLICT: 'Phương thức thanh toán này đã có instance provider khác đang bật, vui lòng tắt instance đó trước khi tiếp tục.',
    CANCEL_RATE_LIMITED: 'Bạn đang hủy đơn quá thường xuyên, vui lòng thử lại sau',
    NOT_FOUND: 'Đơn hàng không tồn tại',
    FORBIDDEN: 'Bạn không có quyền thao tác với đơn hàng này',
    CONFLICT: 'Trạng thái đơn hàng đã thay đổi, vui lòng làm mới',
    INVALID_ORDER_TYPE: 'Chỉ đơn hàng nạp số dư mới có thể yêu cầu hoàn tiền',
    INVALID_STATUS: 'Trạng thái đơn hàng hiện tại không cho phép thao tác này',
    BALANCE_NOT_ENOUGH: 'Số tiền hoàn vượt quá số dư',
    REFUND_AMOUNT_EXCEEDED: 'Số tiền hoàn vượt quá số tiền đã nạp',
    REFUND_FAILED: 'Hoàn tiền thất bại',
  },
  stripePay: 'Thanh toán ngay',
  stripeSuccessProcessing: 'Thanh toán thành công, đang xử lý đơn hàng...',
  stripePopup: {
    redirecting: 'Đang chuyển đến trang thanh toán...',
    loadingQr: 'Đang lấy mã QR WeChat Pay...',
    timeout: 'Hết thời gian chờ chứng từ thanh toán, vui lòng thử lại',
    qrFailed: 'Không thể lấy mã QR WeChat Pay',
  },
  subscribeNow: 'Mở ngay',
  renewNow: 'Gia hạn',
  selectPlan: 'Chọn gói',
  planFeatures: 'Tính năng gói',
  packageCard: {
    credits: 'Số dư',
    group: 'Nhóm',
    select: 'Chọn',
    selected: 'Đã chọn',
    noPackages: 'Không có gói nào',
  },
  planCard: {
    rate: 'Hệ số',
    dailyLimit: 'Hạn mức ngày',
    weeklyLimit: 'Hạn mức tuần',
    monthlyLimit: 'Hạn mức tháng',
    quota: 'Hạn ngạch',
    unlimited: 'Không giới hạn',
    models: 'Mô hình',
  },
  days: 'ngày',
  months: 'tháng',
  years: 'năm',
  oneMonth: '1 tháng',
  oneYear: '1 năm',
  perMonth: '/tháng',
  perYear: '/năm',
  admin: {
    tabs: {
      overview: 'Tổng quan',
      orders: 'Quản lý đơn hàng',
      channels: 'Kênh thanh toán',
      plans: 'Gói đăng ký',
    },
    todayRevenue: 'Doanh thu hôm nay',
    totalRevenue: 'Tổng doanh thu',
    todayOrders: 'Đơn hàng hôm nay',
    totalOrders: 'Tổng đơn hàng',
    pendingOrders: 'Đơn chờ xử lý',
    today: 'Hôm nay',
    orderCount: 'Số đơn hàng',
    avgAmount: 'Giá trị trung bình',
    revenue: 'Doanh thu',
    dailyRevenue: 'Doanh thu theo ngày',
    paymentDistribution: 'Phân bố phương thức thanh toán',
    colUser: 'Người dùng',
    colDeviceCode: 'Device Code',
    topUsers: 'Xếp hạng chi tiêu',
    noData: 'Chưa có dữ liệu',
    depositOverview: 'Thống kê nạp / cấp gói',
    depositOverviewDesc: 'Theo dõi admin nạp số dư, mã nạp, đơn thanh toán và các gói được cấp tự động/thủ công cho người dùng.',
    totalDepositEvents: 'Sự kiện nạp',
    depositLedgerAmount: 'Số dư cộng',
    depositCredits: 'Credit cộng',
    depositPackageAssignments: 'Gói đã cấp',
    depositAdminAutoBreakdown: 'Admin / tự động',
    adminAdjustments: 'Admin nạp',
    manualAssignments: 'Cấp thủ công',
    autoAssignments: 'Cấp tự động',
    depositSources: 'Nguồn nạp',
    depositRecipients: 'Người nhận nạp nhiều nhất',
    recentDeposits: 'Lịch sử nạp gần đây',
    lastDeposit: 'Lần nạp cuối',
    colSource: 'Nguồn',
    colAmount: 'Số tiền',
    colPackage: 'Gói',
    unknownPackage: 'Gói không xác định',
    depositSourceLabels: {
      paid_balance_order: 'Đơn nạp số dư',
      paid_subscription_order: 'Đơn mua gói',
      redeem_balance: 'Mã nạp số dư',
      redeem_subscription: 'Mã cấp gói',
      redeem_affiliate_balance: 'Credit affiliate',
      admin_balance_adjustment: 'Admin nạp số dư',
      manual_subscription_assignment: 'Admin cấp gói',
      auto_subscription_assignment: 'Tự động cấp gói',
    },
    days: 'ngày',
    weeks: 'tuần',
    months: 'tháng',
    searchOrders: 'Tìm kiếm đơn hàng...',
    allStatuses: 'Tất cả trạng thái',
    allPaymentTypes: 'Tất cả phương thức thanh toán',
    allOrderTypes: 'Tất cả loại đơn hàng',
    orderDetail: 'Chi tiết đơn hàng',
    orderType: 'Loại đơn hàng',
    orders: 'Đơn hàng',
    balanceOrder: 'Nạp số dư',
    subscriptionOrder: 'Đăng ký',
    paidAt: 'Thời gian thanh toán',
    completedAt: 'Thời gian hoàn tất',
    expiresAt: 'Hết hạn lúc',
    feeRate: 'Tỷ lệ phí xử lý',
    refund: 'Hoàn tiền',
    refundOrder: 'Đơn hoàn tiền',
    refundAmount: 'Số tiền hoàn',
    maxRefundable: 'Số tiền tối đa có thể hoàn',
    refundReason: 'Lý do hoàn tiền',
    refundReasonPlaceholder: 'Vui lòng nhập lý do hoàn tiền',
    confirmRefund: 'Xác nhận hoàn tiền',
    refundSuccess: 'Hoàn tiền thành công',
    refundInfo: 'Thông tin hoàn tiền',
    refundEnabled: 'Cho phép hoàn tiền',
    alreadyRefunded: 'Đã hoàn tiền',
    deductBalance: 'Khấu trừ số dư',
    deductBalanceHint: 'Khấu trừ lại số tiền đã nạp khỏi số dư người dùng',
    userBalance: 'Số dư người dùng',
    orderAmount: 'Giá trị đơn hàng',
    insufficientBalance: 'Số dư không đủ, sẽ khấu trừ về $0',
    noDeduction: 'Sẽ không khấu trừ số dư người dùng',
    forceRefund: 'Buộc hoàn tiền (bỏ qua kiểm tra số dư)',
    orderCancelled: 'Đơn hàng đã bị hủy',
    retry: 'Thử lại',
    retrySuccess: 'Thử lại thành công',
    approveRefund: 'Phê duyệt hoàn tiền',
    retryRefund: 'Thử hoàn tiền lại',
    confirmManualPayment: 'Xác nhận thanh toán',
    confirmManualPaymentPrompt: 'Xác nhận đã nhận tiền cho đơn {order} (số tiền {amount})? Hành động này sẽ hoàn tất đơn.',
    manualPaymentConfirmed: 'Đã xác nhận thanh toán thủ công',
    refundRequestInfo: 'Thông tin yêu cầu hoàn tiền',
    refundRequestedAt: 'Thời gian yêu cầu',
    refundRequestedBy: 'Người yêu cầu',
    refundRequestReason: 'Lý do yêu cầu',
    auditLogs: 'Nhật ký thao tác',
    operator: 'Người thao tác',
    channelName: 'Tên kênh',
    channelDescription: 'Mô tả kênh',
    createChannel: 'Tạo kênh',
    editChannel: 'Chỉnh sửa kênh',
    deleteChannel: 'Xóa kênh',
    deleteChannelConfirm: 'Bạn có chắc muốn xóa kênh này không?',
    planName: 'Tên gói',
    planDescription: 'Mô tả gói',
    createPlan: 'Tạo gói',
    editPlan: 'Chỉnh sửa gói',
    deletePlan: 'Xóa gói',
    deletePlanConfirm: 'Bạn có chắc muốn xóa gói này không?',
    originalPrice: 'Giá gốc',
    price: 'Giá',
    validityDays: 'Thời hạn hiệu lực (ngày)',
    validityUnit: 'Đơn vị thời hạn',
    sortOrder: 'Thứ tự sắp xếp',
    forSale: 'Trạng thái mở bán',
    onSale: 'Đang bán',
    offSale: 'Ngừng bán',
    group: 'Nhóm',
    groupId: 'ID nhóm',
    features: 'Tính năng',
    featuresHint: 'Mỗi dòng một tính năng',
    featuresPlaceholder: 'Nhập tính năng gói...',
    providerManagement: 'Quản lý provider',
    providerManagementDesc: 'Quản lý các instance provider thanh toán',
    createProvider: 'Tạo provider',
    editProvider: 'Chỉnh sửa provider',
    deleteProvider: 'Xóa provider',
    deleteProviderConfirm: 'Bạn có chắc muốn xóa provider này không?',
    providerName: 'Tên provider',
    providerKey: 'Định danh provider',
    selectProviderKey: 'Chọn định danh provider',
    providerConfig: 'Cấu hình provider',
    noProviders: 'Chưa có provider nào',
    noProvidersHint: 'Tạo một instance provider để bắt đầu nhận thanh toán',
    supportedTypes: 'Phương thức thanh toán được hỗ trợ',
    supportedTypesHint: 'Chọn các phương thức thanh toán mà provider này hỗ trợ',
    rateMultiplier: 'Hệ số giá',
    dashboardTitle: 'Tổng quan thanh toán',
    dashboardDesc: 'Thống kê và phân tích đơn nạp tiền',
    daySuffix: 'ngày',
    paymentConfigTitle: 'Cấu hình thanh toán',
    paymentConfigDesc: 'Quản lý provider thanh toán và các cài đặt liên quan',
    plansPageTitle: 'Quản lý gói đăng ký',
    plansPageDesc: 'Quản lý cấu hình gói đăng ký',
    tabPlanConfig: 'Cấu hình gói',
    tabUserSubs: 'Đăng ký người dùng',
    selectGroup: 'Vui lòng chọn nhóm',
    groupRequired: 'Vui lòng chọn nhóm đăng ký',
    priceRequired: 'Giá phải lớn hơn 0',
    validityDaysRequired: 'Số ngày hiệu lực phải lớn hơn 0',
    groupMissing: 'Thiếu',
    groupInfo: 'Thông tin nhóm',
    platform: 'Nền tảng',
    rateMultiplierLabel: 'Hệ số',
    dailyLimit: 'Hạn mức ngày',
    weeklyLimit: 'Hạn mức tuần',
    monthlyLimit: 'Hạn mức tháng',
    unlimited: 'Không giới hạn',
    searchUserSubs: 'Tìm kiếm đăng ký người dùng...',
    daily: 'ngày',
    weekly: 'tuần',
    monthly: 'tháng',
    subsStatus: {
      active: 'Đang hiệu lực',
      expired: 'Đã hết hạn',
      revoked: 'Đã thu hồi',
    },


    balancePackagesPageTitle: 'Quản lý gói nạp credit',
    balancePackagesPageDesc: 'Quản lý gói top-up bằng form, không cần sửa JSON. Người dùng trả số tiền nạp và nhận số credit đã cấu hình.',
    createBalancePackage: 'Tạo gói nạp',
    editBalancePackage: 'Chỉnh sửa gói nạp',
    deleteBalancePackage: 'Xóa gói nạp',
    deleteBalancePackageConfirm: 'Bạn có chắc muốn xóa gói nạp credit này không?',
    packageCode: 'Mã gói',
    packageLabel: 'Tên gói',
    packageDescription: 'Mô tả gói',
    packageAmounts: 'Số tiền / credit',
    amountLedger: 'Số tiền thu',
    creditLedger: 'Credit cộng',
    bonusLedger: 'Credit thưởng',
    creditMultiplier: 'Hệ số credit',
    balanceGroup: 'Nhóm balance',
    selectBalanceGroup: 'Chọn nhóm balance standard',
    balanceGroupHelp: 'Chỉ nhóm active có subscription_type=standard được dùng. Nhóm subscription thuộc gói thuê bao, không dùng cho gói nạp balance.',
    balanceGroupRequired: 'Vui lòng chọn nhóm balance',
    balanceGroupMissing: 'Thiếu nhóm balance',
    payAmount: 'Trả',
    creditAmount: 'Nhận',
    badge: 'Nhãn',
    popular: 'Phổ biến',
    balancePackageFormula: 'Công thức: người dùng trả Số tiền thu; hệ thống cộng Credit cộng. Hệ số = Credit / Tiền thu, credit thưởng = Credit - Tiền thu.',
    packageRequired: 'Vui lòng nhập mã gói và tên gói',
    packageAmountRequired: 'Số tiền thu và credit cộng phải lớn hơn 0',
    allowUserRefund: 'Cho phép người dùng hoàn tiền',
  },
  }
}

const additionsA = {
  version: {
  rollback: 'Khôi phục phiên bản',
  rollbackSelectVersion: 'Chọn một phiên bản để quay lại (3 phiên bản gần đây nhất)',
  rollbackConfirm: 'Quay trở lại {version}',
  rollbackWarning: 'Rollback tải xuống phiên bản đã chọn và thay thế tệp nhị phân hiện tại. Yêu cầu khởi động lại dịch vụ sau đó.',
  rollingBack: 'Quay lại...',
  rollbackComplete: 'Hoàn tất khôi phục',
  rollbackFailed: 'Khôi phục không thành công',
  manualRollbackCommand: 'Khôi phục thủ công',
  copyCommand: 'Sao chép',
  copied: 'Đã sao chép',
  noRollbackVersions: 'Không có phiên bản nào có thể khôi phục',
  loadVersionsFailed: 'Không tải được phiên bản',
  rollbackSourceHint: 'Khôi phục trực tuyến không có sẵn cho các bản dựng nguồn',
  deployScript: 'Kịch bản',
  deployDocker: 'Docker',
  dockerEditCompose: 'Chỉnh sửa thẻ hình ảnh trong docker-compose.yml',
  dockerRecreate: 'Tạo lại vùng chứa',
  },

  customPage: {
  copyCode: 'Sao chép',
  copiedCode: 'Đã sao chép',
  copyCodeFailed: 'Thất bại',
  },

  userSubscriptions: {
  quotaEndsIn: 'Hạn ngạch kết thúc sau {time}',
  },

  payment: {
  status: {
    refund_pending: 'Đang chờ hoàn tiền',
  },
  qr: {
    alipayOpening: 'Khai trương Alipay',
    alipayContinueInApp: 'Hoàn tất thanh toán trong Alipay',
    alipayWaitingHint: 'Máy chủ sẽ xác nhận thanh toán và tự động cập nhật trang này',
    alipayFallbackTitle: 'Alipay chưa mở',
    alipayFallbackHint: 'Hãy thử mở lại Alipay hoặc lưu mã QR và quét mã từ album ảnh Alipay của bạn',
    reopenAlipay: 'Mở lại Alipay',
    saveQRCode: 'Lưu mã QR',
    alipaySaveAndScanHint: 'Lưu mã QR, mở Alipay Scan, sau đó chọn mã đó từ album ảnh của bạn',
  },
  orders: {
    payAmount: 'Trả',
  },
  planCard: {
    peakRate: 'Tỷ lệ cao điểm',
  },
  weeks: 'tuần',
  admin: {
    refundPending: 'Hoàn tiền đang chờ xác nhận cổng',
    queryRefundStatus: 'Truy vấn trạng thái hoàn tiền',
    currency: 'Nhãn tiền tệ',
    currencyPlaceholder: 'ví dụ. USD / NZD / CNY',
    currencyHint: 'Mã tiền tệ ISO gồm 3 chữ cái chỉ hiển thị bên cạnh giá; để trống để ẩn, không ảnh hưởng đến việc thanh toán',
    subscriptionCnyPayPreview: 'Xem trước phí kênh CNY: {amount}',
    subscriptionCnyPayPreviewWithFee: '(Đã bao gồm phí {feeRate}%: {total})',
    validity: 'hiệu lực',
    validityRequired: 'Hiệu lực phải lớn hơn 0',
  },
  }
}

export default mergeLocaleMessages(base, additionsA)
