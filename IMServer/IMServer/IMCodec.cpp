// IMCodec实现
#include "IMCodec.h"
#include "im.pb.h"                     // protoc 生成的 MessageContainer 头文件

#include "net/TcpConnection.h"
#include "net/Buffer.h"
#include "base/Logging.h"

namespace muduo {
namespace net {

IMCodec::IMCodec(const MessageCallback& messageCb,
                 const ErrorCallback& errorCb)
    : codec_(&im::MessageContainer::default_instance(),  // 原型消息
             "IM01",                                      // 协议魔数tag
             std::bind(&IMCodec::onProtobufMessage, this, _1, _2, _3),
             ProtobufCodecLite::RawMessageCallback(),     // 不使用原始消息回调
             errorCb),
      messageCallback_(messageCb)
{
    LOG_INFO << "IMCodec initialized, tag=" << codec_.tag();
}

void IMCodec::send(const TcpConnectionPtr& conn, const im::MessageContainer& msg)
{
    codec_.send(conn, msg);
}

void IMCodec::sendRaw(const TcpConnectionPtr& conn, const std::string& data)
{
    // 从缓存中解析预序列化的MessageContainer，然后走标准send流程
    im::MessageContainer container;
    if (!container.ParseFromString(data))
    {
        LOG_ERROR << "IMCodec::sendRaw - failed to parse cached MessageContainer";
        return;
    }
    codec_.send(conn, container);
}

void IMCodec::onMessage(const TcpConnectionPtr& conn, Buffer* buf, Timestamp time)
{
    codec_.onMessage(conn, buf, time);
}

const std::string& IMCodec::tag() const
{
    return codec_.tag();
}

void IMCodec::onProtobufMessage(const TcpConnectionPtr& conn,
                                const MessagePtr& msg,
                                Timestamp time)
{
    // 将通用MessagePtr向下转型为具体的MessageContainer
    auto container = muduo::down_pointer_cast<im::MessageContainer>(msg);
    if (container)
    {
        messageCallback_(conn, *container, time);
    }
    else
    {
        LOG_ERROR << "IMCodec::onProtobufMessage - failed to cast to MessageContainer";
    }
}

}  // namespace net
}  // namespace muduo
