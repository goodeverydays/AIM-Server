#pragma once
#include "net/EventLoop.h"
#include "net/EventLoopThreadPool.h"
#include "net/EventLoopThread.h"
#include "net/TcpServer.h"
#include "base/Logging.h"
#include <boost/uuid/uuid.hpp>
#include <boost/uuid/uuid_io.hpp>
#include <boost/uuid/uuid_generators.hpp>
#include <iostream>
#include <memory>
#include <string>
#include "UserManager.h"
#include "MsgCacheManager.h"
#include "IMSer.h"

// 前向声明protobuf生成的类，避免头文件污染
namespace im {
class MessageContainer;
}

namespace muduo {
namespace net {
class IMCodec;
}
}

using namespace muduo;
using namespace muduo::net;
using namespace boost::uuids;
using namespace std::placeholders;

#define GROUPID_BOUNDARY 0xFFFFFFF  // 群ID与用户ID的分界线，大于等于此值的为群ID

// 消息类型枚举（保持不变，与im.proto中的cmd值对应）
enum {
    msg_type_unknown,
    // 用户消息
    msg_type_heartbeart = 1000,
    msg_type_register,
    msg_type_login,
    msg_type_getofriendlist,
    msg_type_finduser,
    msg_type_operatefriend,
    msg_type_userstatuschange,
    msg_type_updateuserinfo,
    msg_type_modifypassword,
    msg_type_creategroup,
    msg_type_getgroupmembers,
    msg_type_getchathistory,
    msg_type_avatarupload,     // 1012: 上传头像
    msg_type_sendemailcode,    // 1013: 发送邮箱验证码
    msg_type_upgradevip,       // 1014: 升级VIP
    msg_type_sendrequest,      // 1015: 发起好友/入群请求
    msg_type_pendingrequests,  // 1016: 拉取待处理请求
    msg_type_handlerequest,    // 1017: 处理请求(接受/拒绝)
    msg_type_newrequestpush,   // 1018: S->C 推送新请求
    msg_type_kickgroupmember,  // 1019: 群主踢出成员
    msg_type_renamegroup,      // 1020: 群主/管理员重命名群
    msg_type_setgroupadmin,    // 1021: 群主设置/取消管理员
    // 聊天消息
    msg_type_chat = 1100,   // 单聊消息
    msg_type_multichat,     // 群发消息
};

// TcpSession —— 会话基类，消息编解码由 IMCodec+protobuf 处理
class TcpSession
{
public:
    TcpSession() = default;
    ~TcpSession() = default;
};

// ClientSession —— 表示一个客户端会话，管理与客户端连接相关的信息和操作
// 使用protobuf协议替代原有的手写二进制协议
class ClientSession : public TcpSession
{
public:
    ClientSession(const TcpConnectionPtr& conn);
    // 禁止拷贝构造和赋值操作，防止生命周期管理混乱
    ClientSession(const ClientSession&) = delete;
    ClientSession& operator=(const ClientSession&) = delete;
    ~ClientSession();

    operator std::string()
    {
        return m_sessionid;
    }

    /// TCP数据到达回调，委托给IMCodec处理帧解析
    void OnRead(const muduo::net::TcpConnectionPtr& conn, Buffer* buf, Timestamp time);

    int32_t UserID() const { return (m_user != NULL) ? m_user->userid : -1; }

    /// 向此会话对应的连接发送消息（供其他会话转发消息使用）
    /// @param msg 要发送的MessageContainer
    void SendContainer(const im::MessageContainer& msg);

protected:
    /// IMCodec解码完成后的回调，根据cmd分派到各业务处理函数
    void OnMessageReceived(const TcpConnectionPtr& conn,
                           const im::MessageContainer& msg,
                           Timestamp time);

    // --- 业务处理函数 ---
    // 所有函数统一接收已解码的MessageContainer，内部按需解析payload子消息
    void OnHeartbeatResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnRegisterResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnLoginResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnGetFriendListResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnFindUserResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnOperateFriendResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnUpdateUserInfoResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnModifyPasswordResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnCreateGroupResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnGetGroupMembersResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnGetChatHistoryResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnAvatarUploadResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnSendEmailCodeResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnUpgradeVipResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnSendRequestResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnPendingRequestsResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnHandleRequestResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    // 群设置：群主踢出成员(1019) / 重命名群(1020) / 设置管理员(1021)
    void OnKickGroupMemberResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnRenameGroupResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnSetGroupAdminResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnChatResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);
    void OnMultiChatResponse(const TcpConnectionPtr& conn, const im::MessageContainer& msg);

    void DeleteFriend(const TcpConnectionPtr& conn, int32_t friendid);
    void OnAddGroupResponse(const TcpConnectionPtr& conn, int32_t groupid);
    void SendUserStatusChangeMsg(int32_t userid, int type, const std::string& customface = "");
    // 向本会话用户推送一条新的好友/入群请求（cmd=1018）
    void PushNewRequest(int64_t reqid, int32_t fromid, const std::string& fromname,
                        const std::string& fromface, int32_t reqtype,
                        int32_t groupid, const std::string& groupname,
                        const std::string& message);

private:
    std::string m_sessionid;                            // 会话唯一标识
    int m_seq;                                          // 消息序号，用于请求响应匹配
    UserPtr m_user;                                     // 会话关联的用户信息
    int32_t m_target;                                   // 单聊目标ID
    std::string m_targets;                              // 群发目标列表(JSON数组)
    TcpConnectionPtr m_conn;                            // TCP连接对象
    std::unique_ptr<muduo::net::IMCodec> m_codec;       // protobuf编解码器（每个会话独立实例）
};

typedef std::shared_ptr<ClientSession> ClientSessionPtr;
