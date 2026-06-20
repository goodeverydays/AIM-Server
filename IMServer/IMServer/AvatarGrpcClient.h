#pragma once

#include <memory>
#include <string>

namespace avatar {
class AvatarService;
class UploadReq;
class UploadRsp;
}

namespace grpc {
class Channel;
}

/// AvatarGrpcClient — gRPC 客户端，连接 GoImageServer 头像微服务
class AvatarGrpcClient {
public:
    explicit AvatarGrpcClient(const std::string& target);
    ~AvatarGrpcClient();

    /// 上传头像 → 返回访问 URL
    /// @param userId   用户ID
    /// @param imgData  图片二进制
    /// @param format   格式 "png"/"jpg"
    /// @param urlOut   输出：头像访问URL
    struct UploadResult {
        bool   ok = false;
        std::string url;
        std::string errMsg;
    };
    UploadResult Upload(int32_t userId, const std::string& imgData, const std::string& format);

private:
    struct Impl;
    std::unique_ptr<Impl> m_impl;
};
