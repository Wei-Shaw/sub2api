export default {
  qualityTest: {
    title: 'Kiểm tra chất lượng',
    description: 'Chạy cùng một prompt trên tối đa ba tài khoản song song và so sánh kết quả',
    platform: 'Nền tảng',
    reasoningEffort: 'Mức suy luận',
    effortDefault: 'Mặc định của model',
    slot: 'Tài khoản {n}',
    selectAccount: 'Chọn tài khoản',
    selectModel: 'Chọn model',
    prompt: 'Prompt',
    run: 'Chạy',
    stop: 'Dừng',
    duration: 'Thời gian',
    chars: '{n} ký tự',
    preview: 'Xem trước',
    source: 'Mã nguồn',
    loadAccountsFailed: 'Không tải được danh sách tài khoản',
    loadModelsFailed: 'Không tải được danh sách model',
    status: {
      idle: 'Chờ',
      running: 'Đang chạy',
      completed: 'Hoàn thành',
      error: 'Lỗi',
      stopped: 'Đã dừng'
    }
  }
}
