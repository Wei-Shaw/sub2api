// Generated from the EN → VI locale gap audit.
export default {
  "ops": {
    "outputTps": "TPS đầu ra theo từng yêu cầu",
    "outputTpsSamples": "Mẫu hợp lệ: {count}",
    "tooltips": {
      "outputTps": "Phân vị của token đầu ra / tổng thời gian trên mỗi bản ghi sử dụng hợp lệ, bao gồm thời gian chờ token đầu tiên, trong khoảng thời gian, nền tảng và nhóm đã chọn. Đầu ra có thể gồm token suy luận và không cộng lại lần nữa. P50 là trung vị; P5/P10 phản ánh các yêu cầu chậm hơn. Cao hơn là nhanh hơn. Loại trừ ảnh, Live và bản ghi không có đầu ra hoặc thời lượng dương. Mẫu lấy từ nhật ký sử dụng còn lưu; — nghĩa là không có mẫu hoặc thống kê tạm thời chưa khả dụng.",
    },
    "systemLogs": {
      "requestRetentionDays": "Số ngày giữ yêu cầu",
      "requestRetentionDaysHint": "Giữ nhật ký yêu cầu trong số ngày này; 0 = vĩnh viễn",
      "retentionDaysInvalid": "Số ngày giữ phải là số nguyên không âm",
      "retentionDaysOption": "{days} ngày",
      "retentionDaysCustom": "Tùy chỉnh (ngày)",
      "retentionForever": "Vĩnh viễn",
      "runtimeConfigLoadFailed": "Tải cấu hình runtime thất bại",
      "retentionDaysHint": "Được áp dụng bởi tác vụ dọn dẹp dữ liệu theo lịch.",
      "persistAccessLogs": "Lưu nhật ký truy cập vào cơ sở dữ liệu",
      "persistAccessLogsHint": "Mặc định tắt vì nhật ký truy cập thêm một dòng cơ sở dữ liệu có chỉ mục cho mỗi yêu cầu. Nhật ký cảnh báo, lỗi và kiểm toán luôn được lưu."
    }
  }
}
