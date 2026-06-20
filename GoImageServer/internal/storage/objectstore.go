//go:build s3

// 对象存储后端(S3兼容)。依赖外部 SDK minio-go，仅在以 `-tags s3` 构建时编入，
// 以便默认构建(本地文件后端)无需联网拉取该依赖。云端构建：
//   go get github.com/minio/minio-go/v7 && go build -tags s3 ./...
package storage

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ============================================================
// ObjectStore — 对象存储后端(S3 兼容)
// ------------------------------------------------------------
// 通过 endpoint+bucket+AK/SK 对接：
//   · 阿里云 OSS   endpoint=oss-cn-hangzhou.aliyuncs.com
//   · 腾讯云 COS   endpoint=cos.ap-guangzhou.myqcloud.com
//   · AWS S3       endpoint=s3.amazonaws.com (或区域 endpoint)
//   · MinIO 自建   endpoint=minio.example.com:9000
// 头像作为对象存储，云端自带多副本持久化；URL 走公网/CDN。
// 实例重建/多实例横向扩展都不影响头像，彻底解耦"文件↔实例"。
// ============================================================
type ObjectStore struct {
	client     *minio.Client
	bucket     string
	publicBase string // 公网/CDN 访问前缀，如 https://bucket.oss-cn-hangzhou.aliyuncs.com 或 https://cdn.example.com
	prefix     string // 对象键目录前缀
	maxSize    int64
	maxW, maxH int
}

func NewObjectStore(endpoint, bucket, accessKey, secretKey, region, publicBase string,
	useSSL bool, maxSizeMB, maxW, maxH int) (*ObjectStore, error) {
	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("对象存储配置不完整(需 S3_ENDPOINT/S3_BUCKET/S3_ACCESS_KEY/S3_SECRET_KEY)")
	}
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化对象存储客户端失败: %w", err)
	}
	// publicBase 缺省时回退到 endpoint+bucket(要求 bucket 公共读)
	if publicBase == "" {
		scheme := "https"
		if !useSSL {
			scheme = "http"
		}
		publicBase = fmt.Sprintf("%s://%s/%s", scheme, endpoint, bucket)
	}
	return &ObjectStore{
		client:     cli,
		bucket:     bucket,
		publicBase: strings.TrimRight(publicBase, "/"),
		prefix:     "avatars",
		maxSize:    int64(maxSizeMB) * 1024 * 1024,
		maxW:       maxW,
		maxH:       maxH,
	}, nil
}

func (s *ObjectStore) key(filename string) string { return s.prefix + "/" + filename }
func (s *ObjectStore) url(filename string) string { return s.publicBase + "/" + s.key(filename) }

func (s *ObjectStore) Save(id int32, data []byte, format string) (string, string, error) {
	pngData, err := processImage(data, s.maxW, s.maxH, s.maxSize)
	if err != nil {
		return "", "", err
	}
	// 删除旧头像对象，避免同一 id 堆积
	s.deleteByPrefix(id)

	filename := avatarFilename(id)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = s.client.PutObject(ctx, s.bucket, s.key(filename),
		bytes.NewReader(pngData), int64(len(pngData)),
		minio.PutObjectOptions{ContentType: "image/png"})
	if err != nil {
		return "", "", fmt.Errorf("上传对象失败: %w", err)
	}
	return filename, s.url(filename), nil
}

func (s *ObjectStore) Delete(id int32) error {
	return s.deleteByPrefix(id)
}

func (s *ObjectStore) deleteByPrefix(id int32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    s.key(avatarPrefix(id)),
		Recursive: true,
	}) {
		if obj.Err != nil {
			continue
		}
		_ = s.client.RemoveObject(ctx, s.bucket, obj.Key, minio.RemoveObjectOptions{})
	}
	return nil
}

func (s *ObjectStore) Find(id int32) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    s.key(avatarPrefix(id)),
		Recursive: true,
	}) {
		if obj.Err != nil {
			continue
		}
		fn := path.Base(obj.Key)
		return fn, s.url(fn)
	}
	return "", ""
}
