package service

import (
	"context"
	"fmt"

	"GoImageServer/internal/storage"
	"GoImageServer/pkg/pb"
)

// AvatarService 实现 pb.AvatarServiceServer
type AvatarService struct {
	pb.UnimplementedAvatarServiceServer

	store   storage.Storage // 后端抽象：本地文件 / 对象存储
	version string
}

func NewAvatarService(store storage.Storage, version string) *AvatarService {
	return &AvatarService{
		store:   store,
		version: version,
	}
}

func (s *AvatarService) Upload(ctx context.Context, req *pb.UploadReq) (*pb.UploadRsp, error) {
	if len(req.ImageData) == 0 {
		return &pb.UploadRsp{Code: 1, Msg: "图片数据为空"}, nil
	}

	filename, url, err := s.store.Save(req.UserId, req.ImageData, req.Format)
	if err != nil {
		return &pb.UploadRsp{Code: 2, Msg: fmt.Sprintf("保存失败: %s", err)}, nil
	}

	return &pb.UploadRsp{
		Code:     0,
		Msg:      "上传成功",
		Filename: filename,
		Url:      url,
	}, nil
}

func (s *AvatarService) Delete(ctx context.Context, req *pb.DeleteReq) (*pb.CommonRsp, error) {
	_ = s.store.Delete(req.UserId)
	return &pb.CommonRsp{Code: 0, Msg: "已删除"}, nil
}

func (s *AvatarService) GetUrl(ctx context.Context, req *pb.GetUrlReq) (*pb.GetUrlRsp, error) {
	filename, url := s.store.Find(req.UserId)
	if filename == "" {
		return &pb.GetUrlRsp{Code: 1, Msg: "未找到头像", Filename: "", Url: ""}, nil
	}
	return &pb.GetUrlRsp{Code: 0, Filename: filename, Url: url}, nil
}

func (s *AvatarService) HealthCheck(ctx context.Context, req *pb.HealthCheckReq) (*pb.HealthCheckRsp, error) {
	return &pb.HealthCheckRsp{Healthy: true, Version: s.version}, nil
}
