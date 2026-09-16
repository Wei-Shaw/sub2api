// Generated from the EN → VI locale gap audit.
export default {
  "users": {
    "bulkDelete": {
      "action": "Xóa đã chọn ({count})",
      "title": "Xóa người dùng đã chọn",
      "confirm": "Xóa {count} người dùng đã chọn? Hành động này không thể hoàn tác. Không thể xóa tài khoản quản trị viên.",
      "success": "Đã xóa {count} người dùng",
      "failed": "Không thể xóa {count} người dùng. Các người dùng này vẫn được chọn để thử lại."
    },
    "platformQuota": {
      "reset": {
        "unavailable": "Nền tảng này chưa cấu hình hạn mức nên không có cửa sổ sử dụng để đặt lại"
      }
    }
  },
  "groups": {
    "form": {
      "maxReasoningEffortOverLimit": "Kiểm soát quyền truy cập khi vượt hạn mức",
      "maxReasoningEffortOverLimitDowngrade": "Tự động hạ mức khi vượt hạn mức",
      "maxReasoningEffortOverLimitDeny": "Từ chối truy cập",
      "maxReasoningEffortOverLimitHint": "Áp dụng sau khi đặt mức trần. Hạ mức sẽ đổi các giá trị vượt trần về mức trần. Từ chối sẽ chặn yêu cầu.",
      "reasoningEffortMappingsHint": "Có thể để trống cả loại và model để khớp mọi model. Một loại và model có thể có nhiều ánh xạ yêu cầu, ví dụ tiền tố gpt ánh xạ cả high và xhigh thành medium. Chọn Từ chối làm giá trị chuyển tiếp để từ chối giá trị yêu cầu đó. Khớp chính xác được ưu tiên hơn tiền tố/hậu tố, và tiền tố/hậu tố dài hơn được ưu tiên hơn loại ngắn hơn.",
      "addReasoningEffortPair": "Thêm giá trị yêu cầu",
      "removeReasoningEffortPair": "Xóa giá trị yêu cầu",
      "reasoningEffortMatchType": "Loại",
      "reasoningEffortModel": "Model",
      "reasoningEffortMatchExact": "Chính xác",
      "reasoningEffortMatchPrefix": "Tiền tố",
      "reasoningEffortMatchSuffix": "Hậu tố",
      "reasoningEffortMatchTypePlaceholder": "Tất cả model",
      "reasoningEffortModelPlaceholder": "Trống = tất cả / gpt / gpt-5.4",
      "reasoningEffortToDeny": "Từ chối",
      "unsupportedMatchType": "Loại khớp phải là chính xác, tiền tố hoặc hậu tố",
      "duplicateScope": "Tổ hợp loại và model phải là duy nhất"
    },
    "platforms": {
      "minimax": "MiniMax",
      "opencode_go": "OpenCode"
    },
    "modelAllowlist": {
      "title": "Danh sách cho phép model",
      "hint": "Khi bật, các model ngoài danh sách cho phép sẽ bị từ chối với lỗi 404 model_not_found và các endpoint liệt kê model chỉ hiển thị model được cho phép. Mục nhập hỗ trợ ID model chính xác và ký tự đại diện * ở cuối. Lưu ý: Claude Code thăm dò các model thuộc dòng haiku để lấy tiêu đề/tóm tắt và /messages/count_tokens cũng được kiểm soát bởi danh sách cho phép, nên hãy chọn cả các model nhỏ cần dùng.",
      "loading": "Đang tải các model ứng viên...",
      "empty": "Không có model ứng viên; hãy thêm mục tùy chỉnh bên dưới",
      "selectedSummary": "Đã chọn {selected} / {total}",
      "selectAll": "Chọn tất cả",
      "invertSelection": "Đảo ngược",
      "wildcardTag": "ký tự đại diện",
      "customPlaceholder": "Mục tùy chỉnh, ví dụ claude-* hoặc gpt-5.5-codex",
      "addCustom": "Thêm",
      "emptySelectionError": "Danh sách cho phép model đang bật; hãy chọn hoặc thêm ít nhất một mục model",
      "errors": {
        "empty": "Vui lòng nhập một mục model",
        "invalidWildcard": "Ký tự đại diện * chỉ được phép ở cuối mục nhập",
        "duplicate": "Mục nhập này đã tồn tại"
      }
    },
    "codexModelsManifest": {
      "title": "Tài khoản ghim cho danh sách model",
      "hint": "Khi bật, danh sách model thông thường và Codex Model Manifest sẽ được lấy từ các tài khoản ghim trước, sau đó hợp nhất và lọc theo ánh xạ tài khoản và danh sách model của nhóm. Các tài khoản ghim bị giới hạn tốc độ hoặc quá tải vẫn được sử dụng.",
      "enable": "Lấy danh sách model bằng các tài khoản cụ thể",
      "enabledHint": "Tài khoản được giới hạn ở các tài khoản OpenAI gắn với nhóm này, tối đa 10 tài khoản.",
      "disabledHint": "Đã tắt: danh sách thông thường dùng ánh xạ hoặc mặc định cục bộ; Codex dùng danh mục cục bộ nếu đã cấu hình, nếu không sẽ dùng cơ chế khám phá của bộ lập lịch.",
      "accounts": "Tài khoản ghim",
      "searchPlaceholder": "Tìm tài khoản (tài khoản OpenAI trong nhóm này)",
      "searchEmpty": "Không có tài khoản phù hợp",
      "fallback": "Chuyển về bộ lập lịch khi tất cả tài khoản ghim không khả dụng",
      "fallbackHint": "Tắt: trả về 503 / lỗi từ upstream. Bật: chuyển về luồng bộ lập lịch hiện có.",
      "selectAtLeastOne": "Sau khi bật tài khoản ghim, hãy chọn ít nhất một tài khoản"
    },
    "openaiFast": {
      "title": "Chế độ OpenAI Fast",
      "force": "Bắt buộc Fast (ưu tiên)",
      "hint": "Buộc service_tier=priority cho các yêu cầu OpenAI trong nhóm này. Chính sách Fast/Flex toàn cục vẫn có thể lọc hoặc chặn tùy chọn này. Yêu cầu mới cập nhật ngay sau khi lưu; các phiên WebSocket hiện tại phải kết nối lại.",
      "free": "Fast miễn phí",
      "freeHint": "Các yêu cầu Fast trong nhóm này vẫn dùng tier priority, nhưng khách hàng chỉ bị tính mức giá tương đương Standard."
    }
  }
}
