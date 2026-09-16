// Generated from the EN → VI locale gap audit.
export default {
  "settings": {
    "features": {
      "channelMonitor": {
        "hideUserRanking": "Ẩn xếp hạng người dùng",
        "hideUserRankingHint": "Khi bật, trang Channel Monitor V2 của người dùng sẽ ẩn tab xếp hạng và API người dùng không trả về các dòng xếp hạng. Quản trị viên vẫn thấy xếp hạng."
      },
      "siteBillingMode": {
        "title": "Chế độ thanh toán của site",
        "description": "Kiểm soát các tùy chọn mua mà người dùng nhìn thấy. Mặc định là “Nạp tiền & Gói đăng ký”.",
        "label": "Tùy chọn mua",
        "options": {
          "rechargeAndSubscription": "Nạp tiền & Gói đăng ký",
          "rechargeOnly": "Chỉ nạp tiền",
          "subscriptionOnly": "Chỉ gói đăng ký"
        },
        "hints": {
          "rechargeAndSubscription": "Người dùng có thể vừa nạp số dư vừa mua các gói đăng ký.",
          "rechargeOnly": "Ẩn “Gói đăng ký của tôi”, tab gói đăng ký trên trang mua, huy hiệu gói đăng ký ở header và bộ lọc loại thanh toán trong phần sử dụng; truy cập trực tiếp vào “Gói đăng ký của tôi” sẽ quay về dashboard. Sidebar quản trị cũng ẩn mục “Quản lý gói đăng ký” (trang vẫn truy cập được bằng URL). Thanh toán gói đăng ký hiện có và gói đăng ký từ mã đổi thưởng không bị ảnh hưởng.",
          "subscriptionOnly": "Trang mua chỉ cung cấp các gói đăng ký và mục trên sidebar hiển thị là “Gói đăng ký”; đơn nạp số dư sẽ bị từ chối. Mã đổi thưởng, khoản chi trả affiliate và các khoản cộng số dư khác không bị ảnh hưởng."
        }
      }
    },
    "gatewayForwarding": {
      "openaiTTFTMode": "Chỉ số token đầu tiên của OpenAI Responses",
      "openaiTTFTModeSemantic": "Tương thích cũ (sự kiện ngữ nghĩa)",
      "openaiTTFTModeVisible": "Nội dung hiển thị thực tế",
      "openaiTTFTModeHint": "Mặc định ghi first_token_ms tại sự kiện ngữ nghĩa đầu tiên không phải preamble. Chế độ nội dung hiển thị thực tế chỉ ghi khi nhận được văn bản không rỗng, đối số tool hoặc nội dung ảnh."
    },
    "customMenu": {
      "hideOpenButton": "Ẩn nút “Mở trong tab mới”"
    },
    "openaiFastPolicy": {
      "tierMissing": "Tier bị bỏ qua",
      "tierUltrafast": "ultrafast"
    },
    "openaiAutoProvision": {
      "title": "Bổ sung tài khoản OpenAI",
      "description": "Duy trì số lượng tài khoản OpenAI OAuth khỏe mạnh mục tiêu bằng cách yêu cầu turb-gpt-free-register đăng ký và tải lên tài khoản thay thế.",
      "enabledTitle": "Bật bổ sung tự động",
      "targetLabel": "Mục tiêu tài khoản OAuth khỏe mạnh",
      "intervalLabel": "Khoảng kiểm tra (giây)",
      "workersLabel": "Worker đăng ký",
      "requestsPerAccountLabel": "Số yêu cầu trong 5 giờ mỗi tài khoản OAuth",
      "tokensPerAccountLabel": "Số token trong 5 giờ mỗi tài khoản OAuth",
      "emailSourceLabel": "Ghi đè nguồn email",
      "emailSourcePlaceholder": "Dùng mặc định của turb",
      "turbURLLabel": "URL turb-gpt-free-register",
      "callbackURLLabel": "URL callback Sub2API",
      "turbAuthCodeLabel": "Mã xác thực Turb WebUI",
      "callbackSecretLabel": "Callback secret",
      "reauthorizationTitle": "Tự động cấp lại quyền cho tài khoản OAuth bị lỗi",
      "reauthorizationDescription": "Bắt đầu luồng OAuth Codex mới và chỉ thay thế token tài khoản hiện có sau khi danh tính được xác nhận trùng khớp."
    }
  }
}
