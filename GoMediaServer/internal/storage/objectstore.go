//go:build s3

// 对象存储后端(S3兼容)。依赖 minio-go，仅 `-tags s3` 构建时编入。
// 云端：go get github.com/minio/minio-go/v7 && go build -tags s3 ./...
package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type ObjectStore struct {
	client     *minio.Client
	bucket     string
	publicBase string
	prefix     string
}

func NewObjectStore(endpoint, bucket, accessKey, secretKey, region, publicBase string, useSSL bool) (*ObjectStore, error) {
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
		prefix:     "media",
	}, nil
}

func (s *ObjectStore) Save(data []byte, ext string) (string, string, error) {
	filename := uniqueFilename(ext)
	key := s.prefix + "/" + filename
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err := s.client.PutObject(ctx, s.bucket, key,
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentTypeOf(ext)})
	if err != nil {
		return "", "", fmt.Errorf("上传对象失败: %w", err)
	}
	return filename, s.publicBase + "/" + key, nil
}
