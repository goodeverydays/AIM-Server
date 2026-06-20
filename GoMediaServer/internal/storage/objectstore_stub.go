//go:build !s3

// 默认构建的占位实现：未以 `-tags s3` 构建时不引入 minio-go 依赖，
// NewObjectStore 直接报错，保证本地默认构建无需联网即可编译。
package storage

import "fmt"

type ObjectStore struct{}

func NewObjectStore(endpoint, bucket, accessKey, secretKey, region, publicBase string, useSSL bool) (*ObjectStore, error) {
	return nil, fmt.Errorf("对象存储后端未编译：请用 `go build -tags s3` 构建(需联网拉取 minio-go)")
}

func (s *ObjectStore) Save(data []byte, ext string) (string, string, error) {
	return "", "", fmt.Errorf("对象存储后端未编译")
}
