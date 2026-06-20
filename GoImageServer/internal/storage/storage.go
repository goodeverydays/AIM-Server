package storage

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // 注册 GIF 解码器
	_ "image/jpeg" // 注册 JPEG 解码器
	"image/png"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // 注册 WebP 解码器
)

// ============================================================
// Storage — 头像存储后端抽象
// ------------------------------------------------------------
// 两种实现：
//   · FileStore   本地文件(开发/单机)
//   · ObjectStore 对象存储(阿里云OSS/腾讯云COS/AWS S3/MinIO，云上推荐)
// 由 config.Storage.Backend 选择，业务层只依赖此接口，互不感知。
// 头像以 id(用户ID或群ID，int32，范围不重叠) 为 key，文件名 {id}_{时间戳}.png。
// ============================================================
type Storage interface {
	// Save 校验+缩放为PNG后保存 id 的头像，返回 文件名 与 可访问URL。
	Save(id int32, data []byte, format string) (filename, url string, err error)
	// Delete 删除 id 的头像(幂等)。
	Delete(id int32) error
	// Find 返回 id 当前头像的 文件名 与 URL；不存在则返回空字符串。
	Find(id int32) (filename, url string)
}

// avatarPrefix 返回某 id 头像文件名/对象键的前缀(用于查找与删除旧头像)。
func avatarPrefix(id int32) string {
	return fmt.Sprintf("%d_", id)
}

// avatarFilename 生成带时间戳的头像文件名 {id}_{unix}.png；
// 时间戳保证每次上传URL不同，天然规避客户端图片缓存。
func avatarFilename(id int32) string {
	return fmt.Sprintf("%d_%d.png", id, time.Now().Unix())
}

// processImage 解码 → (超出尺寸才)缩放 → 统一编码为 PNG 字节。本地与对象存储共用。
func processImage(data []byte, maxW, maxH int, maxSize int64) ([]byte, error) {
	if maxSize > 0 && int64(len(data)) > maxSize {
		return nil, fmt.Errorf("文件过大，最大 %dMB", maxSize/(1024*1024))
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片解码失败: %w", err)
	}
	resized := resizeToFit(img, maxW, maxH)

	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return nil, fmt.Errorf("编码PNG失败: %w", err)
	}
	return buf.Bytes(), nil
}

// resizeToFit 缩放图片到指定尺寸（保持比例，适应不裁剪）。
func resizeToFit(src image.Image, maxW, maxH int) image.Image {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= maxW && srcH <= maxH {
		return src // 不需要缩放
	}

	scale := float64(maxW) / float64(srcW)
	if hScale := float64(maxH) / float64(srcH); hScale < scale {
		scale = hScale
	}
	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}
