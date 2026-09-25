// Generated from the EN → VI locale gap audit.
export default {
  "accounts": {
    "openaiProvision": {
      "statusTitle": "Trạng thái bổ sung OpenAI",
      "loading": "Đang tải trạng thái OpenAI...",
      "healthySummary": "OpenAI khỏe mạnh: {healthy}/{target}",
      "checkingSummary": "Đang kiểm tra tài khoản OpenAI...",
      "provisioningSummary": "Đang tạo tài khoản: {count} tài khoản chờ callback",
      "reauthorizationSummary": "Đang cấp lại quyền: {count}",
      "healthy": "OAuth khỏe mạnh",
      "pending": "Đang chờ bổ sung",
      "requested": "Yêu cầu gần nhất",
      "reauthorization": "Đang chờ cấp lại quyền",
      "lastCallback": "Callback gần nhất",
      "nextCheck": "Lần kiểm tra tiếp theo",
      "lastCheck": "Lần kiểm tra gần nhất",
      "reset": "Đặt lại yêu cầu đang chờ",
      "resetting": "Đang đặt lại...",
      "resetSuccess": "Đã đặt lại trạng thái bổ sung",
      "resetFailed": "Không thể đặt lại trạng thái bổ sung",
      "notAvailable": "Không khả dụng",
      "noCallback": "Chưa nhận callback",
      "received": "Đã nhận",
      "phase": {
        "disabled": "Đã tắt",
        "idle": "Đang chờ chu kỳ tiếp theo",
        "checking": "Đang kiểm tra số lượng tài khoản",
        "sending_provision_request": "Đang gửi yêu cầu tạo tài khoản",
        "waiting_for_provision_callback": "Đang chờ callback bổ sung",
        "waiting_for_reauthorization_callback": "Đang chờ callback cấp lại quyền",
        "error": "Lỗi tự động hóa",
        "unknown": "Không xác định"
      }
    },
    "platforms": {
      "minimax": "MiniMax",
      "opencode_go": "OpenCode"
    },
    "cnProviders": {
      "zhipuTeam": {
        "title": "Organization ID / Project ID của gói nhóm",
        "organization": "Organization ID (gói nhóm, tùy chọn)",
        "organizationPlaceholder": "Organization ID của Coding Plan nhóm",
        "project": "Project ID (gói nhóm, tùy chọn)",
        "projectPlaceholder": "Project ID của Coding Plan nhóm",
        "hint": "Chỉ bắt buộc với GLM Coding Plan nhóm; khi đặt, truy vấn sử dụng sẽ đi qua endpoint nhóm. Để trống với gói cá nhân. Nhấp dấu hỏi để xem cách lấy ID.",
        "help": {
          "title": "Cách lấy Organization ID / Project ID",
          "step1": "Đăng nhập nền tảng mở Zhipu (bigmodel.cn) bằng tài khoản nhóm và mở “Coding Plan → Team → My Plan”.",
          "step2": "Nhấn F12 để mở DevTools của trình duyệt, chuyển sang tab Network rồi tải lại trang.",
          "step3": "Nhập /api/biz/v1/organization vào ô lọc Network và nhấp yêu cầu tương ứng (ví dụ api_keys).",
          "step4": "Trong URL yêu cầu, đoạn org-… là Organization ID và đoạn proj_… là Project ID (cũng hiển thị trong header yêu cầu bigmodel-organization / bigmodel-project). Điền chúng vào các trường bên trên.",
          "example": "Ví dụ: …/organization/org-0610bE2D…/projects/proj_0798F20…/api_keys → org-0610bE2D… điền vào “Organization ID”, proj_0798F20… điền vào “Project ID”"
        }
      },
      "windowMonthly": "30 ngày"
    },
    "opencodeGo": {
      "accountMode": {
        "zen": "Zen",
        "zenDesc": "Gateway trả theo mức sử dụng. Tiêu hao credit tài khoản và tính phí theo token.",
        "go": "GO",
        "goDesc": "Gateway theo gói đăng ký, bị giới hạn theo các cửa sổ sử dụng 5 giờ / hàng tuần / hàng tháng."
      },
      "protocolRules": {
        "title": "Định tuyến giao thức model",
        "hint": "Ở chế độ thích ứng, mỗi model được gửi đến giao thức upstream gốc tương ứng. Dùng ID chính xác hoặc ký tự đại diện * ở cuối (ví dụ grok-*, qwen*). Quy tắc khớp đầu tiên được áp dụng; model không khớp dùng Chat Completions.",
        "patternPlaceholder": "grok-* hoặc deepseek-v4-flash",
        "add": "Thêm quy tắc",
        "remove": "Xóa quy tắc",
        "restoreDefaults": "Khôi phục mặc định",
        "fallback": "Model không khớp → Chat Completions (/v1/chat/completions)"
      },
      "title": "Mức sử dụng OpenCode Go",
      "panelHint": "Cửa sổ sử dụng do tài khoản OpenCode Go upstream báo cáo. Làm mới theo yêu cầu hoặc tự động khi được bật.",
      "notRefreshed": "Chưa làm mới",
      "refreshNow": "Làm mới mức sử dụng",
      "autoRefresh": "Tự động làm mới mức sử dụng",
      "autoRefreshHint": "Chỉ chạy khi cả công tắc của tài khoản và công tắc toàn cục đều được bật.",
      "rolling": "5 giờ",
      "rollingShort": "5h",
      "weekly": "Tuần",
      "weeklyShort": "7 ngày",
      "monthly": "Tháng",
      "monthlyShort": "1 tháng",
      "status": "Trạng thái",
      "updatedAt": "Đã cập nhật",
      "ok": "Hiện tại",
      "unauthorized": "Phiên đã hết hạn",
      "failed": "Làm mới thất bại",
      "windowWithReset": "Đã dùng {percent}, đặt lại lúc {reset}",
      "loadFailed": "Không thể tải cài đặt mức sử dụng OpenCode Go",
      "autoRefreshFailed": "Không thể cập nhật tự động làm mới mức sử dụng",
      "refreshSuccess": "Đã làm mới mức sử dụng OpenCode Go",
      "refreshFailed": "Không thể làm mới mức sử dụng OpenCode Go",
      "errors": {
        "OPENCODE_GO_USAGE_REFRESH_RATE_LIMITED": "Đang bị giới hạn làm mới. Hãy thử lại sau {retry_after_seconds} giây."
      }
    },
    "upstreamRequestIdHeader": "ID upstream",
    "upstreamRequestIdHeaderPlaceholder": "Để trống để không ghi nhận",
    "upstreamRequestIdHeaderHelp": {
      "intro": "Tên header phản hồi trong đó upstream trực tiếp khai báo request ID. Giá trị được ghi vào cột “Upstream ID” của nhật ký sử dụng; để trống để không ghi nhận.",
      "examplesTitle": "Giá trị thường dùng",
      "sub2apiNote": "Khớp với cột request ID trong nhật ký sử dụng",
      "official": "API chính thức của {platform}"
    },
    "openai": {
      "wsModeCtxPoolHint": "Gateway lấy và tái sử dụng kết nối WS upstream từ một pool; giới hạn pool do cấu hình gateway quyết định.",
      "wsModeHttpBridgeHint": "Gateway chuyển đổi yêu cầu WS của client thành yêu cầu HTTP upstream, sau đó chuyển đổi phản hồi streaming SSE trở lại thành thông điệp WS.",
      "imagesUrlToB64Json": "Chuyển URL kết quả ảnh thành base64",
      "imagesUrlToB64JsonDesc": "Chỉ áp dụng cho phản hồi Images không streaming của tài khoản OpenAI API Key. Khi một mục ảnh upstream có url nhưng không có b64_json, gateway sẽ tải URL và điền b64_json bằng nội dung base64 (vẫn giữ url) cho client dùng API chính thức; nếu tải thất bại, phản hồi được trả về nguyên trạng."
    },
    "syncUpstreamModelsMetadataPartial": "Một số khả năng của model đã được cập nhật; các model còn lại vẫn chưa đầy đủ.",
    "grokMediaEligibility": {
      "title": "Điều kiện tạo nội dung đa phương tiện",
      "hint": "Kiểm soát việc tài khoản Grok OAuth này có được chọn để tạo ảnh và video hay không.",
      "auto": "Tự động phát hiện",
      "enabled": "Bắt buộc bật",
      "disabled": "Bắt buộc tắt",
      "current": "Quyết định hiện tại:",
      "eligible": "Đủ điều kiện",
      "ineligible": "Không đủ điều kiện",
      "loading": "Đang tải điều kiện…",
      "loadFailed": "Không thể tải điều kiện tạo nội dung đa phương tiện",
      "autoHint": "Tự động phát hiện chỉ xóa ghi đè thủ công; không tự kích hoạt yêu cầu đa phương tiện.",
      "forceEnableWarning": "Bắt buộc bật sẽ bỏ qua kiểm tra điều kiện tự động. Chỉ sử dụng với tài khoản đã xác nhận hỗ trợ tạo ảnh/video.",
      "partialSave": "Các cài đặt tài khoản khác có thể đã được lưu, nhưng điều kiện đa phương tiện chưa được cập nhật. Vui lòng thử lại.",
      "reasons": {
        "eligible": "Đã xác nhận quyền lợi trả phí",
        "billing_inconclusive": "Thông tin thanh toán chưa đủ kết luận",
        "billing_forbidden": "Endpoint thanh toán bị từ chối",
        "billing_free_tier": "Tài khoản gói miễn phí",
        "billing_unobserved": "Chưa quan sát được thông tin thanh toán",
        "override_enabled": "Đã thủ công buộc bật",
        "override_disabled": "Đã thủ công buộc tắt"
      }
    },
    "expiresAtTimezoneHint": "Giá trị được hiểu theo múi giờ trình duyệt của bạn ({timezone}).",
    "usageWindow": {
      "estimatedTotalCost": "Tổng ước tính ${cost}",
      "estimatedTotalCostTooltip": "Chi phí ước tính ở mức sử dụng 100%, dựa trên chi phí và mức sử dụng của cửa sổ hiện tại"
    }
  }
}
