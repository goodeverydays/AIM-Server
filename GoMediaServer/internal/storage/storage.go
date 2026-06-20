package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ============================================================
// Storage — 媒体存储后端抽象(本地文件 / 对象存储)
// ------------------------------------------------------------
// 与头像不同：聊天媒体文件【唯一且永久】，不按用户ID覆盖、不删除，
// 因此接口只需 Save。两种实现：FileStore(本地) / ObjectStore(S3兼容,-tags s3)。
// ============================================================
type Storage interface {
	// Save 保存原始字节，返回 文件名 与 可访问URL；ext 为不带点的扩展名。
	Save(data []byte, ext string) (filename, url string, err error)
}

// uniqueFilename 生成唯一文件名：{纳秒时间戳}_{随机hex}.{ext}
func uniqueFilename(ext string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d_%s.%s", time.Now().UnixNano(), hex.EncodeToString(b), sanitizeExt(ext))
}

// sanitizeExt 规范化扩展名(小写去点)，非白名单回退 bin。
func sanitizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	switch ext {
	case "m4a", "aac", "mp3", "ogg", "opus", "wav", "amr", // 音频
		"mp4", "webm", "mov", "m4v", // 视频
		"png", "jpg", "jpeg", "gif", "webp", // 图片
		"pdf", "txt", "zip", "doc", "docx": // 文件
		return ext
	}
	return "bin"
}

// contentTypeOf 由扩展名推断 MIME(对象存储设置 Content-Type 用)。
func contentTypeOf(ext string) string {
	switch sanitizeExt(ext) {
	case "m4a", "aac":
		return "audio/mp4"
	case "mp3":
		return "audio/mpeg"
	case "ogg", "opus":
		return "audio/ogg"
	case "wav":
		return "audio/wav"
	case "amr":
		return "audio/amr"
	case "mp4", "m4v":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "mov":
		return "video/quicktime"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	}
	return "application/octet-stream"
}
