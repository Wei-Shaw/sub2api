// Generated from the EN → VI locale gap audit.
export default {
  "keys": {
    "bulkEdit": {
      "title": "Chỉnh sửa hàng loạt",
      "selectedCount": "Đã chọn {count} khóa",
      "selectKey": "Chọn khóa {name}",
      "clearSelection": "Bỏ chọn",
      "hint": "Chọn các trường cần cập nhật. Trường không chọn sẽ giữ nguyên giá trị hiện tại.",
      "limitHint": "Nhập 0 để không giới hạn. Mức sử dụng hiện tại vẫn được giữ nguyên.",
      "ipHint": "Mỗi dòng một IP hoặc CIDR. Để trống để xóa danh sách này trên các khóa đã chọn.",
      "invalidLimit": "Nhập số tiền hợp lệ lớn hơn hoặc bằng 0.",
      "invalidExpiration": "Chọn ngày hết hạn hợp lệ hoặc chọn Không hết hạn.",
      "apply": "Áp dụng cho {count} khóa",
      "success": "Đã cập nhật {count} khóa",
      "partialFailure": "Đã cập nhật {success} khóa; {failed} khóa thất bại",
      "failureHint": "Không thể cập nhật các khóa này. Hãy điều chỉnh cài đặt và thử lại. Chỉ các khóa thất bại sẽ được thử lại."
    },
    "useKeyModal": {
      "minimax": {
        "description": "Cấu hình Claude Code, Codex hoặc OpenCode thông qua nhóm MiniMax hiện tại.",
        "codexDescription": "Cấu hình Codex bằng xác thực API key thông qua nhóm MiniMax hiện tại.",
        "codexConfigTomlHint": "Tải danh mục model bên dưới, lưu cả hai tệp vào thư mục cấu hình Codex rồi khởi động lại Codex.",
        "codexNote": "Xuất SUB2API_API_KEY trước khi khởi động Codex. Danh mục đã tải chỉ chứa metadata của model, không chứa API key của bạn."
      }
    }
  },
  "usage": {
    "nativeCompactionV2": "Nén",
    "compactionFilter": "Loại yêu cầu",
    "allCompactionTypes": "Tất cả yêu cầu",
    "compactionOnly": "Chỉ yêu cầu nén",
    "serviceTierUltrafast": "Siêu nhanh"
  },
  "monitorCommon": {
    "providers": {
      "minimax": "MiniMax",
      "opencode_go": "OpenCode"
    },
    "quota": {
      "windows": {
        "monthly": "Hàng tháng"
      }
    }
  },
  "availableChannels": {
    "pricing": {
      "cacheWrite5mPrice": "Ghi cache (5 phút)",
      "cacheWrite1hPrice": "Ghi cache (1 giờ)"
    }
  },
  "modelPlaza": {
    "table": {
      "maxReasoningMultiplierBadge": "Tối đa ×{multiplier}",
      "maxReasoningMultiplierHint": "Khi mức suy luận được chuyển tiếp là max, chi phí và hạn ngạch của yêu cầu được nhân với {multiplier}",
      "reasoningMultiplierBadge": "{effort} ×{multiplier}",
      "reasoningMultiplierHint": "Khi mức suy luận được chuyển tiếp là {effort}, chi phí và hạn ngạch của yêu cầu được nhân với {multiplier}. Các mức chưa cấu hình sử dụng ×1"
    }
  },
  "redeem": {
    "userRefreshFailed": "Đổi mã thành công nhưng không thể làm mới thông tin tài khoản."
  }
}
