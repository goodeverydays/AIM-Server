//go:build !s3

// 对象存储后端的占位实现(默认构建)。
// 未以 `-tags s3` 构建时，NewObjectStore 直接返回错误，避免引入 minio-go 依赖；
// 同时保留 *ObjectStore 的接口方法，使 main.go 在两种构建下都能编译。
package storage

import "fmt"

type ObjectStore struct{}

func NewObjectStore(endpoint, bucket, accessKey, secretKey, region, publicBase string,
	useSSL bool, maxSizeMB, maxW, maxH int) (*ObjectStore, error) {
	return nil, fmt.Errorf("对象存储后端未编译：请用 `go build -tags s3` 构建(需联网拉取 minio-go)")
}

func (s *ObjectStore) Save(id int32, data []byte, format string) (string, string, error) {
	return "", "", fmt.Errorf("对象存储后端未编译")
}
func (s *ObjectStore) Delete(id int32) error { return fmt.Errorf("对象存储后端未编译") }
func (s *ObjectStore) Find(id int32) (string, string) { return "", "" }
