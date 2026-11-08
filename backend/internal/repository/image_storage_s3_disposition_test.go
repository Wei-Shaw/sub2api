package repository

import (
	"os"
	"strings"
	"testing"
)

// mustReadFile 读同目录下的源文件，用来做「这一行还在不在」的守卫。
func mustReadFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", name, err)
	}
	return string(data)
}

// 2026-10-24：图片直链**不能**再带 Content-Disposition: attachment。
//
// 来由：2026-10-22 为了让「点下载直接落盘」加了这个头（当时图片域名没 CORS，
// 只能靠「导航 + attachment」）。2026-10-23 有了同源反代 /img-proxy 之后，
// 下载改用 blob，不再依赖它；而主人反馈 Ozon 上架粘链接老是识别不了，
// 响应里唯一不正常的就只有这个头（图片直链不该是「文件下载」语义）。
//
// 这条守卫就是钉子：谁要是为了下载方便又把它加回来，测试会挂。
func TestImageStorageHasNoContentDisposition(t *testing.T) {
	src := mustReadFile(t, "image_storage_s3.go")
	if strings.Contains(src, "ContentDisposition:") {
		t.Fatal("image_storage_s3.go 又给图片对象加了 ContentDisposition —— " +
			"图片直链要按图片的正常语义返回，下载请走 /img-proxy")
	}
	// Content-Type 还是要设的，不然第三方拿不到图片类型。
	if !strings.Contains(src, "ContentType: &contentType") {
		t.Fatal("image_storage_s3.go 丢了 ContentType —— 图片直链必须带 image/* 类型")
	}
}

// 数据库备份那条线上的 ResponseContentDisposition 是**另一回事**，别一起删。
//
//   - `ContentDisposition`         → 上传时写死在对象上的属性（图片这边已经不用了）
//   - `ResponseContentDisposition` → presigned 链接上的临时参数（备份下载用它，属正常）
//
// 备份文件是给人下载的，保持 attachment 语义是对的。
func TestBackupStoreKeepsAttachmentOnPresignedURL(t *testing.T) {
	backup := mustReadFile(t, "backup_s3_store.go")
	if !strings.Contains(backup, "ResponseContentDisposition") {
		t.Fatal("backup_s3_store.go 丢了 ResponseContentDisposition —— 备份下载会变成在浏览器里打开而不是下载")
	}
}
