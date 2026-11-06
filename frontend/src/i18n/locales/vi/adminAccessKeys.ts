export default {
  "title": "Khóa truy cập quản trị",
  "authorizationHint": "Cấp quyền theo thao tác trong phạm vi tài nguyên. Các thao tác chưa chọn sẽ bị từ chối.",
  "sharedAccountWarning": "Thay đổi trên tài khoản dùng chung ảnh hưởng đến tất cả nhóm liên kết. Quyền toàn cục được áp dụng độc lập với phạm vi nhóm.",
  "secretWarning": "Khóa đầy đủ chỉ hiển thị một lần. Hãy sao chép và lưu lại trước khi đóng.",
  "copySecret": "Sao chép khóa",
  "dismissSecret": "Đã lưu, đóng",
  "create": "Tạo khóa có phạm vi",
  "refresh": "Làm mới",
  "loading": "Đang tải khóa và quyền…",
  "noDescription": "Không có mô tả",
  "never": "Không bao giờ",
  "neverUsed": "Chưa từng sử dụng",
  "summaryLabel": "Tóm tắt quyền truy cập",
  "editPermissions": "Sửa quyền",
  "disable": "Vô hiệu hóa",
  "enable": "Bật",
  "rotate": "Đổi khóa",
  "revoke": "Thu hồi vĩnh viễn",
  "deleteRecord": "Xóa bản ghi",
  "editTitle": "Sửa khóa",
  "createTitle": "Tạo khóa",
  "name": "Tên",
  "description": "Mô tả",
  "expiration": "Thời hạn (giờ địa phương; để trống nếu không hết hạn)",
  "allowedIps": "IP / CIDR được phép (mỗi dòng một mục; để trống thì chấp nhận mọi IP)",
  "enabled": "Bật khóa",
  "allGroups": "Tất cả nhóm, bao gồm cả nhóm được tạo sau này",
  "groups": "Nhóm (có thể chọn nhiều)",
  "includeUngrouped": "Bao gồm tài khoản chưa được phân nhóm",
  "accountTypes": "Loại tài khoản (chỉ áp dụng cho thao tác tài khoản; không chọn loại nào nghĩa là không có quyền truy cập tài khoản)",
  "platforms": "Nền tảng (có thể chọn nhiều; để trống cho phép tất cả)",
  "accountOperations": "Các thao tác tài khoản được phép",
  "groupOperations": "Các thao tác mô hình nhóm được phép (vẫn theo phạm vi nhóm và nền tảng bên trên)",
  "modelAllowlistHint": "Danh sách model được phép của nhóm giới hạn các model mà nhóm có thể sử dụng.",
  "removeRule": "Xóa quy tắc",
  "addRule": "Thêm quy tắc tài nguyên",
  "globalPermissions": "Quyền toàn cục (mặc định tắt tất cả)",
  "processing": "Đang xử lý…",
  "save": "Lưu khóa",
  "cancel": "Hủy",
  "other": "Khác",
  "allGroupsSummary": "Tất cả nhóm, bao gồm cả nhóm được tạo sau này",
  "noGroups": "Không có nhóm",
  "ungroupedSummary": ", bao gồm tài khoản chưa được phân nhóm",
  "noAccountTypes": "Chưa chọn loại tài khoản (không có quyền truy cập tài khoản)",
  "anyPlatform": "Mọi nền tảng",
  "operationFailed": "Thao tác thất bại. Vui lòng thử lại.",
  "nameRequired": "Nhập tên khóa",
  "expirationInvalid": "Nhập thời hạn hợp lệ",
  "saved": "Đã lưu khóa",
  "statusUpdated": "Đã cập nhật trạng thái khóa",
  "rotated": "Đã đổi khóa",
  "revoked": "Đã thu hồi khóa vĩnh viễn",
  "removed": "Đã xóa khóa khỏi danh sách. Lịch sử thao tác vẫn được lưu.",
  "copied": "Đã sao chép khóa",
  "copyFailed": "Không thể sao chép. Vui lòng sao chép khóa thủ công.",
  "statusRevoked": "Đã thu hồi",
  "statusDisabled": "Đã vô hiệu hóa",
  "statusExpired": "Đã hết hạn",
  "statusEnabled": "Đã bật",
  "summary": "Quyền toàn cục: {globals} · Quy tắc tài nguyên: {rules}",
  "ruleLine": "Quy tắc {index}: {summary}",
  "expires": "Hết hạn: {value}",
  "lastUsed": "Sử dụng gần nhất: {value}",
  "ruleLegend": "Quy tắc tài nguyên tài khoản và nhóm {index}",
  "groupFallback": "Nhóm #{id}",
  "ruleSummary": "{groups}; Loại tài khoản: {types} (chỉ thao tác tài khoản); Nền tảng: {platforms}; Số thao tác: {count}",
  "rotateConfirm": "Đổi khóa “{name}” sẽ làm mất hiệu lực khóa hiện tại ngay lập tức. Tiếp tục?",
  "revokeConfirm": "Thu hồi vĩnh viễn “{name}” không thể hoàn tác. Tiếp tục?",
  "deleteConfirm": "Xóa bản ghi khóa “{name}”? Thao tác này chỉ xóa khóa khỏi danh sách. Lịch sử thao tác vẫn được lưu.",
  "legacy": "Khóa cũ có toàn quyền (khuyến nghị chuyển sang khóa có phạm vi)",
  "fullAccessLabel": "Toàn quyền truy cập (tương thích với khóa quản trị viên cũ)",
  "fullAccessWarning": "Khóa này có thể truy cập mọi tài nguyên quản trị, đọc dữ liệu nhạy cảm và thay đổi cài đặt tài khoản, tài chính, kể cả qua các endpoint quản trị được bổ sung sau này. Các quy tắc về nhóm, nền tảng, loại tài khoản và thao tác không còn giới hạn khóa này. Các endpoint yêu cầu phiên trình duyệt của quản trị viên hoặc xác minh tăng cường vẫn giữ nguyên những yêu cầu đó.",
  "rulesSuspended": "Các quy tắc tài nguyên tạm ngừng áp dụng và sẽ có hiệu lực trở lại khi tắt toàn quyền truy cập. Thời hạn, giới hạn IP, giới hạn tần suất và ghi nhật ký kiểm toán vẫn được áp dụng.",
  "fullAccessConfirmTitle": "Xác nhận toàn quyền truy cập",
  "fullAccessPhrase": "BẬT TOÀN QUYỀN TRUY CẬP",
  "fullAccessPhraseHint": "Nhập “{phrase}” để xác nhận:",
  "unknownPermission": "Quyền không xác định ({id})",
  "permissions": {
    "accounts_didentity_dread": {
      "label": "Xem email, gói dịch vụ và thời gian tạo tài khoản (không bao gồm token)",
      "category": "Tài khoản"
    },
    "accounts_dusage_dread": {
      "label": "Xem và làm mới hạn mức cùng bản chụp trạng thái của tài khoản được cấp quyền",
      "category": "Tài khoản"
    },
    "accounts_dstats_dread": {
      "label": "Xem thống kê sử dụng và chi phí của tài khoản",
      "category": "Tài khoản"
    },
    "openai_dquota_dread": {
      "label": "Xem hạn mức từ OpenAI và số lượt đặt lại còn lại",
      "category": "OpenAI"
    },
    "openai_dquota_dreset": {
      "label": "Dùng một lượt đặt lại của OpenAI để khôi phục hạn mức",
      "category": "OpenAI"
    },
    "openai_dreferrals_dread": {
      "label": "Xem điều kiện giới thiệu của OpenAI và số lời mời còn lại",
      "category": "OpenAI"
    },
    "openai_dreferrals_dinvite": {
      "label": "Gửi email mời sử dụng OpenAI",
      "category": "OpenAI"
    },
    "accounts_doptions_dwrite": {
      "label": "Ghi thông tin định danh OAuth và tùy chọn kết nối OpenAI",
      "category": "Tài khoản"
    },
    "accounts_drate_dwrite": {
      "label": "Đặt hệ số tính phí của tài khoản",
      "category": "Tài khoản"
    },
    "groups_dlist_dread": {
      "label": "Xem chi tiết nhóm đã chọn (tên, nền tảng và trạng thái)",
      "category": "Nhóm"
    },
    "groups_dcapacity_dread": {
      "label": "Xem năng lực phục vụ của nhóm đã chọn (toàn bộ nhóm, không phân biệt loại tài khoản)",
      "category": "Nhóm"
    },
    "openai_dtokens_drefresh": {
      "label": "Làm mới token OpenAI do bên gọi cung cấp (trả về token; cho phép dùng proxy đã chọn)",
      "category": "OAuth"
    },
    "accounts_dread": {
      "label": "Xem tài khoản (đã ẩn thông tin nhạy cảm)",
      "category": "Tài khoản"
    },
    "accounts_dcreate": {
      "label": "Tạo tài khoản",
      "category": "Tài khoản"
    },
    "accounts_dupdate": {
      "label": "Sửa tên và ghi chú của tài khoản",
      "category": "Tài khoản"
    },
    "accounts_dcredentials": {
      "label": "Ghi thông tin xác thực (không có quyền đọc)",
      "category": "Tài khoản"
    },
    "accounts_dmodels_dwrite": {
      "label": "Sửa ID trong danh sách mô hình được phép và các ánh xạ của tài khoản",
      "category": "Tài khoản"
    },
    "accounts_dnetwork": {
      "label": "Thay đổi endpoint, proxy và tùy chọn kết nối",
      "category": "Tài khoản"
    },
    "accounts_dstatus": {
      "label": "Bật, vô hiệu hóa tài khoản và bật/tắt điều phối tài khoản",
      "category": "Tài khoản"
    },
    "accounts_dschedule": {
      "label": "Thay đổi số yêu cầu đồng thời, độ ưu tiên và thời hạn",
      "category": "Tài khoản"
    },
    "accounts_drefresh": {
      "label": "Làm mới thông tin xác thực OAuth",
      "category": "Tài khoản"
    },
    "accounts_drecover": {
      "label": "Xóa lỗi tài khoản và trạng thái giới hạn tần suất",
      "category": "Tài khoản"
    },
    "accounts_ddelete": {
      "label": "Xóa tài khoản (ảnh hưởng đến các nhóm dùng chung)",
      "category": "Tài khoản"
    },
    "accounts_dgroups": {
      "label": "Thay đổi liên kết với các nhóm được cấp quyền",
      "category": "Tài khoản"
    },
    "groups_dmodels_dread": {
      "label": "Xem danh sách mô hình được phép và các ID ứng viên của nhóm đã chọn",
      "category": "Mô hình của nhóm"
    },
    "groups_dmodels_dwrite": {
      "label": "Sửa danh sách mô hình được phép của nhóm đã chọn (ảnh hưởng đến toàn bộ nhóm)",
      "category": "Mô hình của nhóm"
    },
    "proxies_dread": {
      "label": "Xem các IP proxy có thể chọn (có phân trang, không bao gồm thông tin xác thực)",
      "category": "Proxy"
    },
    "models_ddefaults_dread": {
      "label": "Xem ID trong danh sách mô hình được phép mặc định và các ánh xạ",
      "category": "Mô hình"
    },
    "dashboard_dread": {
      "label": "Xem báo cáo kinh doanh toàn hệ thống",
      "category": "Báo cáo"
    },
    "users_dread": {
      "label": "Xem hồ sơ của tất cả người dùng (bao gồm thông tin liên hệ)",
      "category": "Người dùng"
    },
    "users_dbalance_dread": {
      "label": "Xem số dư và lịch sử giao dịch của tất cả người dùng",
      "category": "Tài chính"
    },
    "users_dbalance_dwrite": {
      "label": "Cộng hoặc trừ số dư của bất kỳ người dùng nào",
      "category": "Tài chính"
    },
    "users_dquotas_dread": {
      "label": "Xem hạn mức theo nền tảng của tất cả người dùng",
      "category": "Người dùng"
    },
    "users_dquotas_dwrite": {
      "label": "Thay đổi và đặt lại hạn mức theo nền tảng của tất cả người dùng",
      "category": "Người dùng"
    },
    "groups_dread": {
      "label": "Xem cấu hình của tất cả nhóm",
      "category": "Nhóm"
    },
    "usage_dread": {
      "label": "Xem chi tiết sử dụng toàn hệ thống và mức sử dụng của người dùng",
      "category": "Báo cáo"
    },
    "payment_dread": {
      "label": "Xem tất cả đơn thanh toán và báo cáo",
      "category": "Tài chính"
    },
    "payment_dcancel": {
      "label": "Hủy đơn thanh toán",
      "category": "Tài chính"
    },
    "payment_dfulfill": {
      "label": "Thử lại việc xử lý quyền lợi cho đơn thanh toán",
      "category": "Tài chính"
    },
    "payment_drefund": {
      "label": "Hoàn tiền và xác nhận kết quả hoàn tiền",
      "category": "Tài chính"
    },
    "payment_dplans_dread": {
      "label": "Xem tất cả gói thanh toán",
      "category": "Tài chính"
    },
    "payment_dplans_dwrite": {
      "label": "Tạo, sửa và xóa gói thanh toán",
      "category": "Tài chính"
    },
    "subscriptions_dread": {
      "label": "Xem tất cả bản ghi gói đăng ký",
      "category": "Gói đăng ký"
    },
    "subscriptions_dassign": {
      "label": "Cấp gói đăng ký cho bất kỳ người dùng nào",
      "category": "Gói đăng ký"
    },
    "subscriptions_dextend": {
      "label": "Gia hạn gói đăng ký",
      "category": "Gói đăng ký"
    },
    "subscriptions_drevoke": {
      "label": "Thu hồi gói đăng ký",
      "category": "Gói đăng ký"
    },
    "subscriptions_drestore": {
      "label": "Khôi phục gói đăng ký",
      "category": "Gói đăng ký"
    },
    "subscriptions_dreset": {
      "label": "Đặt lại hạn mức gói đăng ký",
      "category": "Gói đăng ký"
    },
    "announcements_dread": {
      "label": "Xem tất cả thông báo và xác nhận đã đọc",
      "category": "Nội dung"
    },
    "announcements_dwrite": {
      "label": "Tạo, xuất bản và sửa thông báo",
      "category": "Nội dung"
    },
    "announcements_ddelete": {
      "label": "Xóa thông báo",
      "category": "Nội dung"
    },
    "promo_dread": {
      "label": "Xem mã khuyến mãi và lịch sử sử dụng",
      "category": "Tài chính"
    },
    "promo_dwrite": {
      "label": "Tạo và sửa quyền lợi của mã khuyến mãi",
      "category": "Tài chính"
    },
    "promo_ddelete": {
      "label": "Xóa mã khuyến mãi",
      "category": "Tài chính"
    },
    "ops_dread": {
      "label": "Xem chỉ số vận hành toàn hệ thống",
      "category": "Giám sát"
    },
    "ops_derrors_dread": {
      "label": "Xem chi tiết lỗi yêu cầu toàn hệ thống (nhạy cảm)",
      "category": "Giám sát"
    },
    "ops_derrors_dresolve": {
      "label": "Đánh dấu trạng thái xử lý lỗi yêu cầu",
      "category": "Giám sát"
    },
    "ops_dalerts_dread": {
      "label": "Xem quy tắc và sự kiện cảnh báo toàn hệ thống",
      "category": "Giám sát"
    },
    "ops_dalerts_dwrite": {
      "label": "Thay đổi quy tắc, trạng thái và thiết lập tạm ngưng cảnh báo",
      "category": "Giám sát"
    },
    "system_dread": {
      "label": "Xem phiên bản hệ thống hiện tại và các phiên bản có sẵn",
      "category": "Hệ thống"
    }
  },
  "listSeparator": ", "
}
