#include "ClientSession.h"
#include <sstream>
#include "IMCodec.h"
#include "im.pb.h"                     // protoc生成的消息头文件
#include "UserManager.h"
#include "PasswordUtil.h"
#ifdef HAVE_RABBITMQ
#include "IMSer.h"                     // 取 MQ 发布者
#include "base/Singleton.h"
#include "json/json.h"                 // 构造发往 RabbitMQ 的聊天事件 JSON
#endif

namespace {
    // 服务端用户名格式校验：3-20 位，仅字母/数字/下划线。
    // 客户端已校验，服务端必须再校验一次（不信任客户端），同时收窄 SQL 注入面。
    bool IsValidUsername(const std::string& name) {
        if (name.size() < 3 || name.size() > 20) return false;
        for (char c : name) {
            bool ok = (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
                   || (c >= '0' && c <= '9') || c == '_';
            if (!ok) return false;
        }
        return true;
    }
}
#ifdef HAVE_AGENT_GRPC
#include "AgentGrpcClient.h"
#include "AvatarGrpcClient.h"
#include "MailGrpcClient.h"
#endif
// 注意: jsoncpp依赖已由protobuf替代，此文件不再直接使用jsoncpp

using namespace muduo::net;

ClientSession::ClientSession(const TcpConnectionPtr& conn)
    : m_seq(0)
    , m_target(0)
{
    std::stringstream ss;
    ss << (void*)conn.get();
    m_sessionid = ss.str();

    // 初始化protobuf编解码器，设置解码完成后的回调
    m_codec.reset(new IMCodec(
        std::bind(&ClientSession::OnMessageReceived, this, _1, _2, _3)));

    // 设置连接的数据到达回调，委托给IMCodec处理
    TcpConnectionPtr* client = const_cast<TcpConnectionPtr*>(&conn);
    (*client)->setMessageCallback(
        std::bind(&ClientSession::OnRead, this, _1, _2, std::placeholders::_3));

    m_conn = conn;
}

ClientSession::~ClientSession()
{
}

// 发送消息容器（供其他会话转发消息时调用）
void ClientSession::SendContainer(const im::MessageContainer& msg)
{
    if (m_codec && m_conn)
    {
        m_codec->send(m_conn, msg);
    }
}

// TCP数据到达回调 —— 委托给IMCodec处理帧解析和protobuf反序列化
void ClientSession::OnRead(const TcpConnectionPtr& conn, Buffer* buf, Timestamp time)
{
    // IMCodec内部完成：
    // 1. 从Buffer中读取完整帧（按total_len分帧）
    // 2. 验证Adler32校验和
    // 3. 反序列化protobuf MessageContainer
    // 4. 触发OnMessageReceived回调
    m_codec->onMessage(conn, buf, time);
}

// IMCodec解码完成回调 —— 根据cmd分派到各业务处理函数
void ClientSession::OnMessageReceived(const TcpConnectionPtr& conn,
                                      const im::MessageContainer& msg,
                                      Timestamp time)
{
    m_seq = msg.seq();  // 同步当前序号

    switch (msg.cmd())
    {
    case msg_type_heartbeart:
        OnHeartbeatResponse(conn, msg);
        break;
    case msg_type_register:
        OnRegisterResponse(conn, msg);
        break;
    case msg_type_login:
        OnLoginResponse(conn, msg);
        break;
    case msg_type_getofriendlist:
        OnGetFriendListResponse(conn, msg);
        break;
    case msg_type_finduser:
        OnFindUserResponse(conn, msg);
        break;
    case msg_type_operatefriend:
        OnOperateFriendResponse(conn, msg);
        break;
    case msg_type_updateuserinfo:
        OnUpdateUserInfoResponse(conn, msg);
        break;
    case msg_type_modifypassword:
        OnModifyPasswordResponse(conn, msg);
        break;
    case msg_type_creategroup:
        OnCreateGroupResponse(conn, msg);
        break;
    case msg_type_getgroupmembers:
        OnGetGroupMembersResponse(conn, msg);
        break;
    case msg_type_getchathistory:
        OnGetChatHistoryResponse(conn, msg);
        break;
    case msg_type_avatarupload:
        OnAvatarUploadResponse(conn, msg);
        break;
    case msg_type_sendemailcode:
        OnSendEmailCodeResponse(conn, msg);
        break;
    case msg_type_upgradevip:
        OnUpgradeVipResponse(conn, msg);
        break;
    case msg_type_sendrequest:
        OnSendRequestResponse(conn, msg);
        break;
    case msg_type_pendingrequests:
        OnPendingRequestsResponse(conn, msg);
        break;
    case msg_type_handlerequest:
        OnHandleRequestResponse(conn, msg);
        break;
    case msg_type_kickgroupmember:
        OnKickGroupMemberResponse(conn, msg);
        break;
    case msg_type_renamegroup:
        OnRenameGroupResponse(conn, msg);
        break;
    case msg_type_setgroupadmin:
        OnSetGroupAdminResponse(conn, msg);
        break;
    case msg_type_chat:
        OnChatResponse(conn, msg);
        break;
    case msg_type_multichat:
        OnMultiChatResponse(conn, msg);
        break;
    default:
        LOG_WARN << "Unknown message cmd=" << msg.cmd()
                 << " from " << conn->peerAddress().toIpPort();
        break;
    }
}

// ============================================================
// 心跳响应 (cmd=1000)
// ============================================================
void ClientSession::OnHeartbeatResponse(const TcpConnectionPtr& conn,
                                        const im::MessageContainer& msg)
{
    im::MessageContainer response;
    response.set_cmd(msg_type_heartbeart);
    response.set_seq(m_seq);
    // 心跳无需payload

    m_codec->send(conn, response);
    printf("%s(%d): %s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 注册响应 (cmd=1001)
// 请求: RegisterReq { username, nickname, password }
// 响应: CommonRsp { code, msg }
// ============================================================
void ClientSession::OnRegisterResponse(const TcpConnectionPtr& conn,
                                       const im::MessageContainer& msg)
{
    im::RegisterReq req;
    im::CommonRsp rsp;

    // 解析注册请求
    if (!req.ParseFromString(msg.payload()))
    {
        LOG_ERROR << "Failed to parse RegisterReq";
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else if (req.username().empty() || req.nickname().empty() || req.password().empty()
             || req.email().empty() || req.code().empty())
    {
        rsp.set_code(102);
        rsp.set_msg("data field missing!");
    }
    else if (!IsValidUsername(req.username()))
    {
        rsp.set_code(106);
        rsp.set_msg("用户名为3-20位字母、数字或下划线");
    }
    else if (req.nickname().size() > 60)   // 昵称长度上限（UTF-8 字节，约20汉字）
    {
        rsp.set_code(106);
        rsp.set_msg("昵称过长");
    }
    else
    {
        bool emailVerified = false;
#ifdef HAVE_AGENT_GRPC
        IMSer& imserver = Singleton<IMSer>::instance();
        MailGrpcClient* mail = imserver.GetMailClient();
        if (mail) {
            auto result = mail->VerifyCode(req.email(), req.code());
            emailVerified = result.ok;
            if (!emailVerified) {
                rsp.set_code(104);
                rsp.set_msg(result.msg.empty() ? "验证码错误或已过期" : result.msg);
            }
        } else {
            rsp.set_code(104);
            rsp.set_msg("邮件服务未配置");
        }
#else
        rsp.set_code(104);
        rsp.set_msg("邮件服务未编译 (需gRPC)");
#endif

        if (emailVerified)
        {
            // 构建User对象并尝试注册
            User user;
            user.username = req.username();
            user.nickname = req.nickname();
            user.password = req.password();
            user.mail = req.email();

            int addResult = Singleton<UserManager>::instance().AddUser(user);
            if (addResult == 1)
            {
                rsp.set_code(105);
                rsp.set_msg("用户名已被占用");
            }
            else if (addResult == 2)
            {
                rsp.set_code(107);
                rsp.set_msg("该邮箱已注册");
            }
            else if (addResult != 0)
            {
                rsp.set_code(100);
                rsp.set_msg("register failed!");
                printf("%s(%d): %s - add user failed\r\n", __FILE__, __LINE__, __FUNCTION__);
            }
            else
            {
                rsp.set_code(0);
                rsp.set_msg("ok");
                printf("%s(%d): %s - register success\r\n", __FILE__, __LINE__, __FUNCTION__);
            }
        }
    }

    // 构建并发送响应
    im::MessageContainer response;
    response.set_cmd(msg_type_register);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
}

// ============================================================
// 登录响应 (cmd=1002)
// 请求: LoginReq { username, password, clienttype, status }
// 响应: LoginRsp { code, msg, UserInfo }
// ============================================================
void ClientSession::OnLoginResponse(const TcpConnectionPtr& conn,
                                    const im::MessageContainer& msg)
{
    im::LoginReq req;
    im::LoginRsp rsp;

    if (!req.ParseFromString(msg.payload()))
    {
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else if (req.username().empty() || req.password().empty())
    {
        rsp.set_code(102);
        rsp.set_msg("data field missing!");
    }
    else
    {
        std::string username = req.username();
        std::string password = req.password();

        // 查找用户
        if (!Singleton<UserManager>::instance().GetUserInfoUsername(username, m_user))
        {
            rsp.set_code(103);
            rsp.set_msg("user is not exist or password is incorrect!");
        }
        else if (!pwd::VerifyPassword(password, m_user->password))
        {
            rsp.set_code(104);
            rsp.set_msg("user is not exist or password is incorrect!");
        }
        else
        {
            // 登录成功，填充用户信息
            rsp.set_code(0);
            rsp.set_msg("ok");

            im::UserInfo* userInfo = rsp.mutable_user();
            userInfo->set_userid(m_user->userid);
            userInfo->set_username(m_user->username);
            userInfo->set_nickname(m_user->nickname);
            userInfo->set_facetype(m_user->facetype);
            userInfo->set_customface(m_user->customface);
            userInfo->set_gender(m_user->gender);
            userInfo->set_birthday(m_user->birthday);
            userInfo->set_signature(m_user->signature);
            userInfo->set_address(m_user->address);
            userInfo->set_phonenumber(m_user->phonenumber);
            userInfo->set_mail(m_user->mail);

            // 查询并回填 VIP 状态，使客户端重登后恢复会员身份（goagent 持有真值）
#ifdef HAVE_AGENT_GRPC
            {
                AgentGrpcClient* agent = Singleton<IMSer>::instance().GetAgentClient();
                bool isVip = false;
                if (agent && agent->queryVipStatus(m_user->userid, isVip)) {
                    rsp.set_is_vip(isVip);
                }
                // gRPC 不可用时保持默认 false，不影响登录本身
            }
#endif

            // m_user 现为会话本地快照(GetUserInfoUsername 返回深拷贝)。
            // 在线状态须经加锁方法写回缓存，否则多 IO 线程下与"读好友列表"竞争。
            m_user->status = 1;  // 更新本地快照(供本次响应)
            Singleton<UserManager>::instance().SetUserStatus(m_user->userid, 1);  // 线程安全写缓存
        }
    }

    // 发送登录响应
    im::MessageContainer response;
    response.set_cmd(msg_type_login);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);

    // 登录成功后推送离线消息
    if (rsp.code() == 0)
    {
        // 推送通知消息（缓存的序列化数据通过sendRaw发送）
        std::list<NotifyMsgCache> listNotifyCache;
        Singleton<MsgCacheManager>::instance().GetNotifyMsgCache(m_user->userid, listNotifyCache);
        for (const auto& iter : listNotifyCache)
        {
            // 缓存的notifymsg是MessageContainer序列化后的字节串
            m_codec->sendRaw(conn, iter.notifymsg);
        }

        // 推送聊天消息
        std::list<ChatMsgCache> listChatCache;
        Singleton<MsgCacheManager>::instance().GetChatMsgCache(m_user->userid, listChatCache);
        for (const auto& iter : listChatCache)
        {
            m_codec->sendRaw(conn, iter.chatmsg);
        }

        // 推送用户上线状态给所有好友
        std::list<UserPtr> friends;
        Singleton<UserManager>::instance().GetFriendInfoByUserID(m_user->userid, friends);
        IMSer& imserver = Singleton<IMSer>::instance();
        for (const auto& iter : friends)
        {
            ClientSessionPtr targetSession = imserver.GetSessionByID(iter->userid);
            if (targetSession)
            {
                printf("%s(%d): %s userid %d target %d\r\n",
                    __FILE__, __LINE__, __FUNCTION__, m_user->userid, targetSession->UserID());
                targetSession->SendUserStatusChangeMsg(m_user->userid, 1);
            }
        }
    }

    printf("%s(%d) : %s userid %d\r\n", __FILE__, __LINE__, __FUNCTION__, m_user->userid);
}

// ============================================================
// 获取好友列表 (cmd=1003)
// 响应: FriendListRsp { code, msg, friends[] }
// ============================================================
void ClientSession::OnGetFriendListResponse(const TcpConnectionPtr& conn,
                                            const im::MessageContainer& msg)
{
    im::FriendListRsp rsp;
    rsp.set_code(0);
    rsp.set_msg("ok");

    std::list<UserPtr> lstFriend;
    Singleton<UserManager>::instance().GetFriendInfoByUserID(m_user->userid, lstFriend);

    for (const auto& iter : lstFriend)
    {
        im::UserInfo* f = rsp.add_friends();
        f->set_userid(iter->userid);
        f->set_username(iter->username);
        f->set_nickname(iter->nickname);
        f->set_facetype(iter->facetype);
        f->set_customface(iter->customface);
        f->set_gender(iter->gender);
        f->set_birthday(iter->birthday);
        f->set_signature(iter->signature);
        f->set_address(iter->address);
        f->set_phonenumber(iter->phonenumber);
        f->set_mail(iter->mail);
        f->set_clienttype(1);
        f->set_status(iter->status ? 1 : 0);
        f->set_ownerid(iter->ownerid);   // 群条目携带群主ID，便于客户端判断群主身份
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_getofriendlist);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
    printf("%s(%d) : %s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 查找用户 (cmd=1004)
// 请求: FindUserReq { type, keyword }
// 响应: FindUserRsp { code, msg, UserInfo }
// ============================================================
void ClientSession::OnFindUserResponse(const TcpConnectionPtr& conn,
                                       const im::MessageContainer& msg)
{
    im::FindUserReq req;
    im::FindUserRsp rsp;

    if (!req.ParseFromString(msg.payload()))
    {
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else
    {
        UserPtr user;

        if (req.type() == "username")
        {
            Singleton<UserManager>::instance().GetUserInfoUsername(req.keyword(), user);
        }
        else if (req.type() == "userid")
        {
            int32_t userid = std::stoi(req.keyword());
            user = Singleton<UserManager>::instance().GetUserByID(userid);
        }

        if (!user)
        {
            rsp.set_code(103);
            rsp.set_msg("user not found!");
        }
        else if (user->userid >= GROUPID_BOUNDARY)
        {
            rsp.set_code(105);
            rsp.set_msg("cannot search for group!");
        }
        else
        {
            rsp.set_code(0);
            rsp.set_msg("ok");

            im::UserInfo* userInfo = rsp.mutable_user();
            userInfo->set_userid(user->userid);
            userInfo->set_username(user->username);
            userInfo->set_nickname(user->nickname);
            userInfo->set_facetype(user->facetype);
            userInfo->set_customface(user->customface);
            userInfo->set_gender(user->gender);
            userInfo->set_birthday(user->birthday);
            userInfo->set_signature(user->signature);
            userInfo->set_address(user->address);
            userInfo->set_phonenumber(user->phonenumber);
            userInfo->set_mail(user->mail);
            userInfo->set_status(user->status);
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_finduser);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
    printf("%s(%d): %s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 操作好友 (cmd=1005)
// 请求: OperateFriendReq { type(1=add,2=delete), friendid }
// 响应: CommonRsp { code, msg }
// ============================================================
void ClientSession::OnOperateFriendResponse(const TcpConnectionPtr& conn,
                                            const im::MessageContainer& msg)
{
    im::OperateFriendReq req;
    im::CommonRsp rsp;

    if (!req.ParseFromString(msg.payload()))
    {
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else
    {
        int32_t friendid = req.friendid();
        UserManager& userMgr = Singleton<UserManager>::instance();

        if (req.type() == 1)  // 添加好友
        {
            // 不允许将群组添加为好友
            if (friendid >= GROUPID_BOUNDARY)
            {
                rsp.set_code(106);
                rsp.set_msg("cannot add group as friend!");
            }
            else if (userMgr.MakeFriendRelationship(
                (m_user->userid < friendid) ? m_user->userid : friendid,
                (m_user->userid < friendid) ? friendid : m_user->userid))
            {
                rsp.set_code(0);
                rsp.set_msg("ok");
                // 通知被添加方刷新好友列表
                IMSer& imserver = Singleton<IMSer>::instance();
                ClientSessionPtr targetSession = imserver.GetSessionByID(friendid);
                if (targetSession)
                {
                    targetSession->SendUserStatusChangeMsg(m_user->userid, 4);  // type=4: 被添加好友
                }
            }
            else
            {
                rsp.set_code(100);
                rsp.set_msg("add friend failed!");
            }
        }
        else if (req.type() == 2)  // 删除好友
        {
            if (userMgr.DeleteFriendToUser(m_user->userid, friendid))
            {
                rsp.set_code(0);
                rsp.set_msg("ok");
                // 通知被删除方
                DeleteFriend(conn, friendid);
            }
            else
            {
                rsp.set_code(100);
                rsp.set_msg("delete friend failed!");
            }
        }
        else if (req.type() == 3)  // 邀请入群
        {
            int32_t groupid = msg.target_id();
            if (groupid >= GROUPID_BOUNDARY)
            {
                int32_t smallid = (friendid < groupid) ? friendid : groupid;
                int32_t greatid = (friendid < groupid) ? groupid : friendid;
                if (userMgr.MakeFriendRelationship(smallid, greatid))
                {
                    rsp.set_code(0);
                    rsp.set_msg("ok");
                }
                else
                {
                    rsp.set_code(100);
                    rsp.set_msg("invite to group failed!");
                }
            }
            else
            {
                rsp.set_code(104);
                rsp.set_msg("invalid group id!");
            }
        }
        else
        {
            rsp.set_code(103);
            rsp.set_msg("unknown operation type!");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_operatefriend);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
    printf("%s(%d): %s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 更新用户信息 (cmd=1007)
// 请求: UpdateUserInfoReq { nickname, facetype, customface, gender, birthday, ... }
// 响应: CommonRsp { code, msg }
// ============================================================
void ClientSession::OnUpdateUserInfoResponse(const TcpConnectionPtr& conn,
                                             const im::MessageContainer& msg)
{
    im::UpdateUserInfoReq req;
    im::CommonRsp rsp;

    if (!req.ParseFromString(msg.payload()))
    {
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else
    {
        // 构造新用户信息，protobuf默认值表示未传（保留原值）
        // 注意：protobuf3中基础类型默认值为0/空字符串，需区分"未设置"和"设置为空"
        User newinfo;
        newinfo.nickname    = req.nickname().empty()    ? m_user->nickname    : req.nickname();
        newinfo.facetype    = req.facetype()            ? req.facetype()     : m_user->facetype;
        newinfo.customface  = req.customface().empty()  ? m_user->customface  : req.customface();
        newinfo.gender      = req.gender()              ? req.gender()       : m_user->gender;
        newinfo.birthday    = req.birthday()            ? req.birthday()     : m_user->birthday;
        newinfo.signature   = req.signature().empty()   ? m_user->signature   : req.signature();
        newinfo.address     = req.address().empty()     ? m_user->address     : req.address();
        newinfo.phonenumber = req.phonenumber().empty() ? m_user->phonenumber : req.phonenumber();
        newinfo.mail        = req.mail().empty()        ? m_user->mail        : req.mail();

        if (Singleton<UserManager>::instance().UpdateUserInfo(m_user->userid, newinfo))
        {
            // 同步更新会话缓存
            m_user->nickname    = newinfo.nickname;
            m_user->facetype    = newinfo.facetype;
            m_user->customface  = newinfo.customface;
            m_user->gender      = newinfo.gender;
            m_user->birthday    = newinfo.birthday;
            m_user->signature   = newinfo.signature;
            m_user->address     = newinfo.address;
            m_user->phonenumber = newinfo.phonenumber;
            m_user->mail        = newinfo.mail;

            rsp.set_code(0);
            rsp.set_msg("ok");
        }
        else
        {
            rsp.set_code(100);
            rsp.set_msg("update user info failed!");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_updateuserinfo);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
    printf("%s(%d): %s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 修改密码 (cmd=1008)
// 请求: ModifyPasswordReq { oldpassword, newpassword }
// 响应: CommonRsp { code, msg }
// ============================================================
void ClientSession::OnModifyPasswordResponse(const TcpConnectionPtr& conn,
                                             const im::MessageContainer& msg)
{
    im::ModifyPasswordReq req;
    im::CommonRsp rsp;

    if (!req.ParseFromString(msg.payload()))
    {
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else if (req.oldpassword().empty() || req.newpassword().empty())
    {
        rsp.set_code(102);
        rsp.set_msg("invalid parameter!");
    }
    else if (!pwd::VerifyPassword(req.oldpassword(), m_user->password))
    {
        rsp.set_code(103);
        rsp.set_msg("old password is incorrect!");
    }
    else if (Singleton<UserManager>::instance().ModifyUserPassword(m_user->userid, req.newpassword()))
    {
        // 缓存已由 ModifyUserPassword 更新为新哈希（m_user 与缓存共享同一对象），
        // 此处无需再写，避免把哈希覆盖回明文。
        rsp.set_code(0);
        rsp.set_msg("ok");
    }
    else
    {
        rsp.set_code(100);
        rsp.set_msg("modify password failed!");
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_modifypassword);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
    printf("%s(%d): %s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 创建群组 (cmd=1009)
// 请求: CreateGroupReq { groupname }
// 响应: CreateGroupRsp { code, msg, groupid, groupname }
// ============================================================
void ClientSession::OnCreateGroupResponse(const TcpConnectionPtr& conn,
                                          const im::MessageContainer& msg)
{
    im::CreateGroupReq req;
    im::CreateGroupRsp rsp;

    if (!req.ParseFromString(msg.payload()))
    {
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else if (req.groupname().empty())
    {
        rsp.set_code(102);
        rsp.set_msg("invalid parameter!");
    }
    else
    {
        int32_t groupid = 0;
        UserManager& userMgr = Singleton<UserManager>::instance();

        if (userMgr.AddGroup(req.groupname().c_str(), m_user->userid, groupid))
        {
            // 群主自动加入群组
            OnAddGroupResponse(conn, groupid);

            rsp.set_code(0);
            rsp.set_msg("ok");
            rsp.set_groupid(groupid);
            rsp.set_groupname(req.groupname());
        }
        else
        {
            rsp.set_code(100);
            rsp.set_msg("create group failed!");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_creategroup);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
    printf("%s(%d): %s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 获取群成员 (cmd=1010)
// 请求: GetGroupMembersReq { groupid }
// 响应: GetGroupMembersRsp { code, msg, members[] }
// ============================================================
void ClientSession::OnGetGroupMembersResponse(const TcpConnectionPtr& conn,
                                              const im::MessageContainer& msg)
{
    im::GetGroupMembersReq req;
    im::GetGroupMembersRsp rsp;

    if (!req.ParseFromString(msg.payload()))
    {
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else
    {
        int32_t groupid = req.groupid();

        std::list<UserPtr> members;
        UserManager& userMgr = Singleton<UserManager>::instance();
        UserPtr group = userMgr.GetUserByID(groupid);
        int32_t ownerId = group ? group->ownerid : 0;
        if (userMgr.GetFriendInfoByUserID(groupid, members))
        {
            rsp.set_code(0);
            rsp.set_msg("ok");

            for (const auto& member : members)
            {
                im::UserInfo* m = rsp.add_members();
                m->set_userid(member->userid);
                m->set_username(member->username);
                m->set_nickname(member->nickname);
                m->set_facetype(member->facetype);
                m->set_customface(member->customface);
                m->set_gender(member->gender);
                m->set_birthday(member->birthday);
                m->set_signature(member->signature);
                m->set_address(member->address);
                m->set_phonenumber(member->phonenumber);
                m->set_mail(member->mail);
                m->set_status(member->status);
                // 群内角色：群主=2, 管理员=1, 普通成员=0
                int32_t role = (member->userid == ownerId) ? 2
                             : (userMgr.IsGroupAdmin(groupid, member->userid) ? 1 : 0);
                m->set_grouprole(role);
            }
        }
        else
        {
            rsp.set_code(100);
            rsp.set_msg("get group members failed!");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_getgroupmembers);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
    printf("%s(%d): %s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 获取聊天历史 (cmd=1011)
// 请求: ChatHistoryReq { targetid }
// 响应: ChatHistoryRsp { code, msg, targetid, messages[] }
// ============================================================
void ClientSession::OnGetChatHistoryResponse(const TcpConnectionPtr& conn,
                                              const im::MessageContainer& msg)
{
    im::ChatHistoryReq req;
    im::ChatHistoryRsp rsp;

    if (!req.ParseFromString(msg.payload()))
    {
        rsp.set_code(101);
        rsp.set_msg("protobuf parse failed!");
    }
    else
    {
        int32_t targetid = req.targetid();
        std::list<im::ChatMsg> messages;
        UserManager& userMgr = Singleton<UserManager>::instance();

        if (userMgr.GetChatHistory(m_user->userid, targetid, messages, 50))
        {
            rsp.set_code(0);
            rsp.set_msg("ok");
            rsp.set_targetid(targetid);
            for (const auto& m : messages)
            {
                *rsp.add_messages() = m;
            }
        }
        else
        {
            rsp.set_code(100);
            rsp.set_msg("get chat history failed!");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_getchathistory);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());

    m_codec->send(conn, response);
    printf("%s(%d): %s, userid=%d\n", __FILE__, __LINE__, __FUNCTION__, m_user->userid);
}


// ============================================================
// 单聊消息处理 (cmd=1100)
// MessageContainer中包含 target_id 和 ChatMsg载荷
// ChatMsg { senderid, targetid, content, timestamp }
// ============================================================
void ClientSession::OnChatResponse(const TcpConnectionPtr& conn,
                                   const im::MessageContainer& msg)
{
    m_target = msg.target_id();

    // 构建要转发/缓存的完整MessageContainer
    im::MessageContainer forwardMsg;
    forwardMsg.set_cmd(msg_type_chat);
    forwardMsg.set_seq(m_seq);
    forwardMsg.set_target_id(m_target);
    forwardMsg.set_payload(msg.payload());  // 转发原始ChatMsg载荷

    // 解析ChatMsg用于记录
    im::ChatMsg chatMsg;
    chatMsg.ParseFromString(msg.payload());

    printf("%s(%d): %s target:%d cur:%d\r\n",
        __FILE__, __LINE__, __FUNCTION__, m_target, m_user->userid);
    std::cout << chatMsg.content() << std::endl;

    // --- Agent 智能助手路由 ---
    // target_id = -1 表示消息发往 AI Agent，由 IMServer 通过 gRPC 转发到 goagent
    constexpr int32_t AGENT_USERID = -1;
    if (m_target == AGENT_USERID) {
#ifdef HAVE_AGENT_GRPC
        IMSer& imserver = Singleton<IMSer>::instance();
        AgentGrpcClient* agent = imserver.GetAgentClient();
        if (agent && m_conn) {
            // 透传技能与上下文（客户端在 ChatMsg 里携带）
            std::string skill = chatMsg.skill();
            std::vector<std::string> context(chatMsg.context().begin(),
                                             chatMsg.context().end());
            agent->sendMessage(m_user->userid, m_target, chatMsg.content(), 1,
                [this](bool ok, const std::string& reply) {
                    // 构造 Agent 回复的 ChatMsg
                    im::ChatMsg agentReply;
                    agentReply.set_senderid(-1);
                    agentReply.set_targetid(m_user->userid);
                    agentReply.set_content(ok ? reply : ("[Agent不可用] " + reply));
                    agentReply.set_timestamp(time(nullptr));

                    // 封装到 MessageContainer 推送给客户端
                    im::MessageContainer container;
                    container.set_cmd(msg_type_chat);
                    container.set_target_id(m_user->userid);
                    container.set_seq(m_seq);
                    container.set_payload(agentReply.SerializeAsString());

                    if (m_codec) {
                        m_codec->send(m_conn, container);
                    }
                },
                skill, context);
        }
#else
        // gRPC 不可用时返回错误提示
        im::ChatMsg agentReply;
        agentReply.set_senderid(-1);
        agentReply.set_targetid(m_user->userid);
        agentReply.set_content("[Agent未配置]\ngRPC client not built. "
                               "Install libgrpc++-dev and rebuild IMServer.");
        agentReply.set_timestamp(time(nullptr));

        forwardMsg.set_payload(agentReply.SerializeAsString());
        m_codec->send(conn, forwardMsg);
#endif

        printf("%s(%d): %s Agent routed, reply pending\r\n",
            __FILE__, __LINE__, __FUNCTION__);
        return;
    }

    UserManager& userMgr = Singleton<UserManager>::instance();

    // 持久化:启用 RabbitMQ 时改为发 MQ(由 GoChatPersister 异步落库)，把 DB 写从 IO 线程剥离；
    // 发布失败或未启用则回退到同步落库，保证消息不丢。
    bool persisted = false;
#ifdef HAVE_RABBITMQ
    if (RabbitMqPublisher* pub = Singleton<IMSer>::instance().GetMqPublisher())
    {
        Json::Value ev;
        ev["senderid"]  = m_user->userid;
        ev["targetid"]  = m_target;
        ev["msgtype"]   = chatMsg.msg_type();
        ev["duration"]  = chatMsg.duration();
        ev["filesize"]  = static_cast<Json::Int64>(chatMsg.file_size());
        ev["timestamp"] = static_cast<Json::Int64>(chatMsg.timestamp());
        ev["content"]   = chatMsg.content();
        ev["media_url"] = chatMsg.media_url();
        ev["thumb_url"] = chatMsg.thumb_url();
        ev["filename"]  = chatMsg.file_name();
        Json::StreamWriterBuilder wb;
        wb["indentation"] = "";
        persisted = pub->Publish("chat", Json::writeString(wb, ev));
    }
#endif
    if (!persisted)
    {
        // 同步落库(含媒体元数据)
        if (!userMgr.SaveChatMsgToDb(m_user->userid, m_target, chatMsg))
        {
            LOG_ERROR << "Write chat msg to db error, senderid = " << m_user->userid
                      << ", targetid = " << m_target << ", chatmsg:" << chatMsg.content();
        }
    }

    IMSer& imserver = Singleton<IMSer>::instance();
    MsgCacheManager& msgCacheMgr = Singleton<MsgCacheManager>::instance();

    // 单聊消息
    if (m_target < GROUPID_BOUNDARY)
    {
        ClientSessionPtr targetSession = imserver.GetSessionByID(m_target);
        if (!targetSession)
        {
            // 目标用户不在线，缓存消息
            msgCacheMgr.AddChatMsgCache(m_target, forwardMsg.SerializeAsString());
            return;
        }

        targetSession->SendContainer(forwardMsg);
        return;
    }

    // 群聊消息：遍历群成员转发（跳过发送者本人）
    std::list<UserPtr> friends;
    userMgr.GetFriendInfoByUserID(m_target, friends);
    for (const auto& iter : friends)
    {
        if (iter->userid == m_user->userid)
            continue;  // 不转发给发送者，避免客户端重复显示
        ClientSessionPtr targetSession = imserver.GetSessionByID(iter->userid);
        if (!targetSession)
        {
            msgCacheMgr.AddChatMsgCache(iter->userid, forwardMsg.SerializeAsString());
            continue;
        }
        targetSession->SendContainer(forwardMsg);
    }

    printf("%s(%d):%s\r\n", __FILE__, __LINE__, __FUNCTION__);
}

// ============================================================
// 群发消息处理 (cmd=1101)
// MessageContainer中 targets 字段包含群发目标列表(JSON数组)
// payload为ChatMsg
// ============================================================
void ClientSession::OnMultiChatResponse(const TcpConnectionPtr& conn,
                                        const im::MessageContainer& msg)
{
    // targets字段在MessageContainer中，这里使用JSON数组解析（保持与旧协议兼容）
    m_targets = msg.targets();

    // 解析MultiChatTargets获取目标列表
    im::MultiChatTargets multiTargets;
    if (!multiTargets.ParseFromString(msg.payload()))
    {
        LOG_ERROR << "invalid protobuf: MultiChatTargets parse failed, userid="
                  << m_user->userid << ", client: " << conn->peerAddress().toIpPort();
        return;
    }

    for (int i = 0; i < multiTargets.targets_size(); ++i)
    {
        m_target = multiTargets.targets(i);

        // 构建转发消息
        im::MessageContainer forwardMsg;
        forwardMsg.set_cmd(msg_type_chat);
        forwardMsg.set_seq(m_seq);
        forwardMsg.set_target_id(m_target);
        forwardMsg.set_payload(multiTargets.content());

        OnChatResponse(conn, forwardMsg);
    }

    printf("%s(%d):%s\r\n", __FILE__, __LINE__, __FUNCTION__);
    LOG_INFO << "Send to client: cmd=msg_type_multichat, targets count: "
             << multiTargets.targets_size() << ", userid: "
             << m_user->userid << ", client: " << conn->peerAddress().toIpPort();
}

// ============================================================
// 删除好友通知
// ============================================================
void ClientSession::DeleteFriend(const TcpConnectionPtr& conn, int32_t friendid)
{
    IMSer& imserver = Singleton<IMSer>::instance();
    ClientSessionPtr targetSession = imserver.GetSessionByID(friendid);
    if (targetSession)
    {
        targetSession->SendUserStatusChangeMsg(m_user->userid, 3);  // 3=被删除好友
    }
    printf("%s(%d): %s, userid=%d, friendid=%d\r\n",
        __FILE__, __LINE__, __FUNCTION__, m_user->userid, friendid);
}

// ============================================================
// 加入群组
// ============================================================
void ClientSession::OnAddGroupResponse(const TcpConnectionPtr& conn, int32_t groupid)
{
    int32_t smallid = (m_user->userid < groupid) ? m_user->userid : groupid;
    int32_t greatid = (m_user->userid < groupid) ? groupid : m_user->userid;
    Singleton<UserManager>::instance().MakeFriendRelationship(smallid, greatid);
    printf("%s(%d): %s, userid=%d, groupid=%d\r\n",
        __FILE__, __LINE__, __FUNCTION__, m_user->userid, groupid);
}

// ============================================================
// 发送用户状态变更通知
// 通知: UserStatusChangeNotify { type, userid, customface }
// ============================================================
void ClientSession::SendUserStatusChangeMsg(int32_t userid, int type, const std::string& customface)
{
    im::UserStatusChangeNotify notify;
    notify.set_type(type);
    notify.set_userid(userid);
    if (!customface.empty()) {
        notify.set_customface(customface);
    }

    im::MessageContainer container;
    container.set_cmd(msg_type_userstatuschange);
    container.set_seq(0);
    container.set_payload(notify.SerializeAsString());

    m_codec->send(m_conn, container);
    printf("%s(%d): %s, userid=%d, type=%d\r\n",
        __FILE__, __LINE__, __FUNCTION__, userid, type);
}

// ============================================================
// 头像上传 (cmd=1012)
// ============================================================
void ClientSession::OnAvatarUploadResponse(const TcpConnectionPtr& conn,
                                            const im::MessageContainer& msg)
{
    im::AvatarUploadReq req;
    im::AvatarUploadRsp rsp;

    if (!req.ParseFromString(msg.payload())) {
        rsp.set_code(1);
        rsp.set_msg("解析请求失败");
    } else if (!m_user) {
        rsp.set_code(1);
        rsp.set_msg("未登录");
    } else {
#ifdef HAVE_AGENT_GRPC
        IMSer& imserver = Singleton<IMSer>::instance();
        AvatarGrpcClient* avatar = imserver.GetAvatarClient();
        UserManager& userMgr = Singleton<UserManager>::instance();
        if (!avatar) {
            rsp.set_code(3);
            rsp.set_msg("Avatar服务未配置");
        } else if (req.target_id() >= GROUPID_BOUNDARY) {
            // ── 群头像：复用同一图片服务(以群ID为key)，权限=群主∥管理员 ──
            int32_t groupId = req.target_id();
            UserPtr group = userMgr.GetUserByID(groupId);
            bool canEdit = group &&
                (group->ownerid == m_user->userid || userMgr.IsGroupAdmin(groupId, m_user->userid));
            if (!group) {
                rsp.set_code(4); rsp.set_msg("群不存在");
            } else if (!canEdit) {
                rsp.set_code(5); rsp.set_msg("只有群主或管理员可更换群头像");
            } else {
                auto result = avatar->Upload(groupId, req.image_data(), req.format());
                if (result.ok) {
                    rsp.set_code(0); rsp.set_msg("群头像已更新"); rsp.set_url(result.url);
                    group->customface = result.url;
                    userMgr.UpdateUserInfo(groupId, *group);
                    // 推送 type=5(头像更新) 给所有在线成员；上传者本人由响应自行刷新
                    std::list<UserPtr> members;
                    userMgr.GetFriendInfoByUserID(groupId, members);
                    for (const auto& m : members) {
                        if (m->userid == m_user->userid) continue;
                        ClientSessionPtr ms = imserver.GetSessionByID(m->userid);
                        if (ms) ms->SendUserStatusChangeMsg(groupId, 5, result.url);
                    }
                } else {
                    rsp.set_code(2); rsp.set_msg(result.errMsg);
                }
            }
        } else {
            // ── 本人头像 ──
            auto result = avatar->Upload(m_user->userid, req.image_data(), req.format());
            if (result.ok) {
                rsp.set_code(0);
                rsp.set_msg("上传成功");
                rsp.set_url(result.url);

                // 更新用户的 customface
                m_user->customface = result.url;
                userMgr.UpdateUserInfo(m_user->userid, *m_user);

                // 通知在线好友刷新头像
                std::list<UserPtr> lstFriend;
                if (userMgr.GetFriendInfoByUserID(m_user->userid, lstFriend)) {
                    for (const auto& friendUser : lstFriend) {
                        ClientSessionPtr targetSession = imserver.GetSessionByID(friendUser->userid);
                        if (targetSession) {
                            targetSession->SendUserStatusChangeMsg(m_user->userid, 5, result.url);
                        }
                    }
                }
            } else {
                rsp.set_code(2);
                rsp.set_msg(result.errMsg);
            }
        }
#else
        rsp.set_code(3);
        rsp.set_msg("Avatar服务未编译 (需gRPC)");
#endif
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_avatarupload);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}

// ============================================================
// 发送邮箱验证码 (cmd=1013)
// ============================================================
void ClientSession::OnSendEmailCodeResponse(const TcpConnectionPtr& conn,
                                             const im::MessageContainer& msg)
{
    im::SendEmailCodeReq req;
    im::SendEmailCodeRsp rsp;

    if (!req.ParseFromString(msg.payload())) {
        rsp.set_code(3);
        rsp.set_msg("解析请求失败");
    } else if (req.email().empty() ||
               req.email().find('@') == std::string::npos ||
               req.email().find('.') == std::string::npos) {
        rsp.set_code(2);
        rsp.set_msg("邮箱格式错误");
    } else {
#ifdef HAVE_AGENT_GRPC
        IMSer& imserver = Singleton<IMSer>::instance();
        MailGrpcClient* mail = imserver.GetMailClient();
        if (mail) {
            auto result = mail->SendCode(req.email());
            rsp.set_code(result.code);
            rsp.set_msg(result.msg);
            rsp.set_cooldown_seconds(result.cooldownSeconds);
        } else {
            rsp.set_code(3);
            rsp.set_msg("邮件服务未配置");
        }
#else
        rsp.set_code(3);
        rsp.set_msg("邮件服务未编译 (需gRPC)");
#endif
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_sendemailcode);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}

// ============================================================
// 升级 VIP (cmd=1014) —— 经 gRPC 转发至 goagent
// ============================================================
void ClientSession::OnUpgradeVipResponse(const TcpConnectionPtr& conn,
                                         const im::MessageContainer& msg)
{
    im::UpgradeVipReq req;
    im::UpgradeVipRsp rsp;

    if (!req.ParseFromString(msg.payload())) {
        rsp.set_code(1);
        rsp.set_msg("解析请求失败");
        rsp.set_is_vip(false);
    } else {
        // 以会话已认证用户ID为准，防止客户端伪造他人ID
        int32_t userId = m_user ? m_user->userid : req.userid();
#ifdef HAVE_AGENT_GRPC
        IMSer& imserver = Singleton<IMSer>::instance();
        AgentGrpcClient* agent = imserver.GetAgentClient();
        if (agent) {
            std::string outMsg;
            bool isVip = false;
            bool ok = agent->upgradeVip(userId, req.payment_token(),
                                        req.amount_cents(), outMsg, isVip);
            rsp.set_code(ok ? 0 : 1);
            rsp.set_msg(outMsg);
            rsp.set_is_vip(isVip);
        } else {
            rsp.set_code(2);
            rsp.set_msg("会员服务未配置");
            rsp.set_is_vip(false);
        }
#else
        rsp.set_code(2);
        rsp.set_msg("会员服务未编译 (需gRPC)");
        rsp.set_is_vip(false);
#endif
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_upgradevip);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}

// ============================================================
// 好友/入群 请求-审批
// ============================================================
void ClientSession::PushNewRequest(int64_t reqid, int32_t fromid, const std::string& fromname,
                                   const std::string& fromface, int32_t reqtype,
                                   int32_t groupid, const std::string& groupname,
                                   const std::string& message)
{
    im::FriendRequest fr;
    fr.set_reqid(reqid);
    fr.set_fromid(fromid);
    fr.set_fromname(fromname);
    fr.set_fromface(fromface);
    fr.set_toid(m_user ? m_user->userid : 0);
    fr.set_reqtype(reqtype);
    fr.set_groupid(groupid);
    fr.set_groupname(groupname);
    fr.set_timestamp(time(nullptr));
    fr.set_message(message);

    im::MessageContainer container;
    container.set_cmd(msg_type_newrequestpush);
    container.set_seq(0);
    container.set_payload(fr.SerializeAsString());
    m_codec->send(m_conn, container);
}

void ClientSession::OnSendRequestResponse(const TcpConnectionPtr& conn,
                                          const im::MessageContainer& msg)
{
    im::SendRequestReq req;
    im::SendRequestRsp rsp;

    if (!m_user) {
        rsp.set_code(1);
        rsp.set_msg("未登录");
    } else if (!req.ParseFromString(msg.payload())) {
        rsp.set_code(101);
        rsp.set_msg("解析请求失败");
    } else {
        UserManager& userMgr = Singleton<UserManager>::instance();
        IMSer& imserver = Singleton<IMSer>::instance();
        int32_t fromId = m_user->userid;
        int32_t reqType = req.reqtype();
        int64_t reqId = 0;

        if (reqType == 1) {  // 好友请求
            int32_t target = req.targetid();
            if (target == fromId) {
                rsp.set_code(102); rsp.set_msg("不能添加自己");
            } else if (target >= GROUPID_BOUNDARY) {
                rsp.set_code(103); rsp.set_msg("不能把群加为好友");
            } else if (!userMgr.GetUserByID(target)) {
                rsp.set_code(104); rsp.set_msg("用户不存在");
            } else if (userMgr.IsFriend(fromId, target)) {
                rsp.set_code(105); rsp.set_msg("对方已是你的好友");
            } else if (userMgr.AddFriendRequest(fromId, target, 1, 0, req.message(), reqId)) {
                rsp.set_code(0); rsp.set_msg("好友请求已发送，等待对方同意");
                ClientSessionPtr ts = imserver.GetSessionByID(target);
                if (ts) {
                    ts->PushNewRequest(reqId, fromId, m_user->nickname, m_user->customface,
                                       1, 0, "", req.message());
                }
            } else {
                rsp.set_code(100); rsp.set_msg("发送请求失败");
            }
        } else if (reqType == 2) {  // 入群邀请：邀请 target 加入 groupId
            int32_t target = req.targetid();
            int32_t groupId = req.groupid();
            UserPtr group = userMgr.GetUserByID(groupId);
            if (!group || groupId < GROUPID_BOUNDARY) {
                rsp.set_code(104); rsp.set_msg("群不存在");
            } else if (!userMgr.GetUserByID(target) || target >= GROUPID_BOUNDARY) {
                rsp.set_code(104); rsp.set_msg("用户不存在");
            } else if (userMgr.IsFriend(target, groupId)) {
                rsp.set_code(105); rsp.set_msg("对方已在群中");
            } else if (userMgr.AddFriendRequest(fromId, target, 2, groupId, req.message(), reqId)) {
                rsp.set_code(0); rsp.set_msg("入群邀请已发送，等待对方同意");
                ClientSessionPtr ts = imserver.GetSessionByID(target);
                if (ts) {
                    ts->PushNewRequest(reqId, fromId, m_user->nickname, m_user->customface,
                                       2, groupId, group->nickname, req.message());
                }
            } else {
                rsp.set_code(100); rsp.set_msg("发送邀请失败");
            }
        } else if (reqType == 3) {  // 申请入群：申请者(from)申请加入 groupId，由群主审批
            int32_t groupId = req.groupid();
            UserPtr group = userMgr.GetUserByID(groupId);
            if (!group || groupId < GROUPID_BOUNDARY) {
                rsp.set_code(104); rsp.set_msg("群不存在");
            } else if (userMgr.IsFriend(fromId, groupId)) {
                rsp.set_code(105); rsp.set_msg("你已在群中");
            } else if (group->ownerid == fromId) {
                rsp.set_code(106); rsp.set_msg("你是群主，无需申请");
            } else if (userMgr.AddFriendRequest(fromId, group->ownerid, 3, groupId, req.message(), reqId)) {
                rsp.set_code(0); rsp.set_msg("入群申请已发送，等待群主同意");
                // 审批人是群主(toid=ownerid)
                ClientSessionPtr ts = imserver.GetSessionByID(group->ownerid);
                if (ts) {
                    ts->PushNewRequest(reqId, fromId, m_user->nickname, m_user->customface,
                                       3, groupId, group->nickname, req.message());
                }
            } else {
                rsp.set_code(100); rsp.set_msg("发送申请失败");
            }
        } else {
            rsp.set_code(103); rsp.set_msg("未知请求类型");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_sendrequest);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}

void ClientSession::OnPendingRequestsResponse(const TcpConnectionPtr& conn,
                                              const im::MessageContainer& msg)
{
    (void)msg;
    im::PendingRequestsRsp rsp;
    if (!m_user) {
        rsp.set_code(1);
        rsp.set_msg("未登录");
    } else {
        UserManager& userMgr = Singleton<UserManager>::instance();
        std::list<FriendRequestInfo> reqs;
        userMgr.GetPendingRequests(m_user->userid, reqs);
        rsp.set_code(0);
        rsp.set_msg("ok");
        for (const auto& r : reqs) {
            im::FriendRequest* fr = rsp.add_requests();
            fr->set_reqid(r.reqid);
            fr->set_fromid(r.fromid);
            UserPtr from = userMgr.GetUserByID(r.fromid);
            if (from) { fr->set_fromname(from->nickname); fr->set_fromface(from->customface); }
            fr->set_toid(r.toid);
            fr->set_reqtype(r.reqtype);
            fr->set_groupid(r.groupid);
            if (r.reqtype == 2) {
                UserPtr g = userMgr.GetUserByID(r.groupid);
                if (g) fr->set_groupname(g->nickname);
            }
            fr->set_timestamp(r.createtime);
            fr->set_message(r.message);
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_pendingrequests);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}

void ClientSession::OnHandleRequestResponse(const TcpConnectionPtr& conn,
                                            const im::MessageContainer& msg)
{
    im::HandleRequestReq req;
    im::HandleRequestRsp rsp;

    if (!m_user) {
        rsp.set_code(1);
        rsp.set_msg("未登录");
    } else if (!req.ParseFromString(msg.payload())) {
        rsp.set_code(101);
        rsp.set_msg("解析请求失败");
    } else {
        UserManager& userMgr = Singleton<UserManager>::instance();
        IMSer& imserver = Singleton<IMSer>::instance();
        FriendRequestInfo info;
        if (!userMgr.GetFriendRequestById(req.reqid(), info)) {
            rsp.set_code(102); rsp.set_msg("请求不存在");
        } else if (info.toid != m_user->userid) {
            rsp.set_code(103); rsp.set_msg("无权处理该请求");
        } else if (req.action() == 2) {  // 拒绝
            userMgr.UpdateRequestStatus(info.reqid, 2);
            rsp.set_code(0); rsp.set_msg("已拒绝");
        } else {  // 接受
            bool ok = false;
            if (info.reqtype == 1) {  // 好友
                int32_t a = (info.fromid < info.toid) ? info.fromid : info.toid;
                int32_t b = (info.fromid < info.toid) ? info.toid : info.fromid;
                ok = userMgr.MakeFriendRelationship(a, b);
                if (ok) {
                    // 通知发起方：被同意=被添加好友(type=4)，触发其刷新好友列表
                    ClientSessionPtr fs = imserver.GetSessionByID(info.fromid);
                    if (fs) fs->SendUserStatusChangeMsg(info.toid, 4);
                }
            } else if (info.reqtype == 2) {  // 入群邀请：被邀请者(toid)加入群(groupid)
                int32_t g = info.groupid;
                int32_t u = info.toid;
                int32_t a = (u < g) ? u : g;
                int32_t b = (u < g) ? g : u;
                ok = userMgr.MakeFriendRelationship(a, b);
            } else if (info.reqtype == 3) {  // 申请入群：申请者(fromid)加入群(groupid)，群主审批
                int32_t g = info.groupid;
                int32_t u = info.fromid;
                int32_t a = (u < g) ? u : g;
                int32_t b = (u < g) ? g : u;
                ok = userMgr.MakeFriendRelationship(a, b);
                if (ok) {
                    // 通知申请者入群成功，刷新其群列表(type=6 群信息变更)
                    ClientSessionPtr fs = imserver.GetSessionByID(info.fromid);
                    if (fs) fs->SendUserStatusChangeMsg(info.groupid, 6);
                }
            }
            if (ok) {
                userMgr.UpdateRequestStatus(info.reqid, 1);
                rsp.set_code(0); rsp.set_msg("已接受");
            } else {
                rsp.set_code(100); rsp.set_msg("处理失败");
            }
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_handlerequest);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}

// ============================================================
// 群设置：群主踢出成员 (cmd=1019)
// ============================================================
void ClientSession::OnKickGroupMemberResponse(const TcpConnectionPtr& conn,
                                              const im::MessageContainer& msg)
{
    im::KickGroupMemberReq req;
    im::KickGroupMemberRsp rsp;

    if (!m_user) {
        rsp.set_code(1); rsp.set_msg("未登录");
    } else if (!req.ParseFromString(msg.payload())) {
        rsp.set_code(101); rsp.set_msg("解析请求失败");
    } else {
        UserManager& userMgr = Singleton<UserManager>::instance();
        IMSer& imserver = Singleton<IMSer>::instance();
        int32_t groupId  = req.groupid();
        int32_t memberId = req.memberid();
        rsp.set_groupid(groupId);
        rsp.set_memberid(memberId);

        UserPtr group = userMgr.GetUserByID(groupId);
        if (!group || groupId < GROUPID_BOUNDARY) {
            rsp.set_code(102); rsp.set_msg("群不存在");
        } else if (group->ownerid != m_user->userid) {
            rsp.set_code(103); rsp.set_msg("只有群主可以踢出成员");
        } else if (memberId == group->ownerid) {
            rsp.set_code(104); rsp.set_msg("群主不能踢自己");
        } else if (!userMgr.IsFriend(memberId, groupId)) {
            rsp.set_code(105); rsp.set_msg("该成员不在群中");
        } else if (userMgr.ReleaseFriendRelationship(memberId, groupId)) {
            rsp.set_code(0); rsp.set_msg("已移出该成员");
            // 通知被踢成员刷新列表（type=3：群关系被移除）
            ClientSessionPtr ms = imserver.GetSessionByID(memberId);
            if (ms) ms->SendUserStatusChangeMsg(groupId, 3);
        } else {
            rsp.set_code(100); rsp.set_msg("移出成员失败");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_kickgroupmember);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}

// ============================================================
// 群设置：群主重命名群 (cmd=1020)
// ============================================================
void ClientSession::OnRenameGroupResponse(const TcpConnectionPtr& conn,
                                          const im::MessageContainer& msg)
{
    im::RenameGroupReq req;
    im::RenameGroupRsp rsp;

    if (!m_user) {
        rsp.set_code(1); rsp.set_msg("未登录");
    } else if (!req.ParseFromString(msg.payload())) {
        rsp.set_code(101); rsp.set_msg("解析请求失败");
    } else {
        UserManager& userMgr = Singleton<UserManager>::instance();
        IMSer& imserver = Singleton<IMSer>::instance();
        int32_t groupId = req.groupid();
        std::string newname = req.newname();
        rsp.set_groupid(groupId);

        UserPtr group = userMgr.GetUserByID(groupId);
        // 改名权限：群主 或 管理员
        bool canRename = group &&
            (group->ownerid == m_user->userid || userMgr.IsGroupAdmin(groupId, m_user->userid));
        if (newname.empty() || newname.size() > 64) {
            rsp.set_code(102); rsp.set_msg("群名称长度不合法");
        } else if (!group || groupId < GROUPID_BOUNDARY) {
            rsp.set_code(103); rsp.set_msg("群不存在");
        } else if (!canRename) {
            rsp.set_code(104); rsp.set_msg("只有群主或管理员可以重命名群");
        } else if (userMgr.RenameGroup(groupId, newname)) {
            rsp.set_code(0); rsp.set_msg("群名称已更新");
            rsp.set_newname(newname);
            // 通知所有在线群成员刷新列表（type=6：群信息变更），改名同步到每个成员
            std::list<UserPtr> members;
            userMgr.GetFriendInfoByUserID(groupId, members);
            for (const auto& m : members) {
                if (m->userid == m_user->userid) continue;  // 操作者本人由响应自行刷新
                ClientSessionPtr ms = imserver.GetSessionByID(m->userid);
                if (ms) ms->SendUserStatusChangeMsg(groupId, 6);
            }
        } else {
            rsp.set_code(100); rsp.set_msg("重命名失败");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_renamegroup);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}

// ============================================================
// 群设置：群主设置/取消管理员 (cmd=1021)
// ============================================================
void ClientSession::OnSetGroupAdminResponse(const TcpConnectionPtr& conn,
                                            const im::MessageContainer& msg)
{
    im::SetGroupAdminReq req;
    im::SetGroupAdminRsp rsp;

    if (!m_user) {
        rsp.set_code(1); rsp.set_msg("未登录");
    } else if (!req.ParseFromString(msg.payload())) {
        rsp.set_code(101); rsp.set_msg("解析请求失败");
    } else {
        UserManager& userMgr = Singleton<UserManager>::instance();
        IMSer& imserver = Singleton<IMSer>::instance();
        int32_t groupId  = req.groupid();
        int32_t memberId = req.memberid();
        bool    isAdmin  = req.is_admin();
        rsp.set_groupid(groupId);
        rsp.set_memberid(memberId);
        rsp.set_is_admin(isAdmin);

        UserPtr group = userMgr.GetUserByID(groupId);
        if (!group || groupId < GROUPID_BOUNDARY) {
            rsp.set_code(102); rsp.set_msg("群不存在");
        } else if (group->ownerid != m_user->userid) {
            rsp.set_code(103); rsp.set_msg("只有群主可以设置管理员");
        } else if (memberId == group->ownerid) {
            rsp.set_code(104); rsp.set_msg("群主无需设置为管理员");
        } else if (!userMgr.IsFriend(memberId, groupId)) {
            rsp.set_code(105); rsp.set_msg("该用户不在群中");
        } else if (userMgr.SetGroupAdmin(groupId, memberId, isAdmin)) {
            rsp.set_code(0);
            rsp.set_msg(isAdmin ? "已设为管理员" : "已取消管理员");
            // 通知被设置者刷新（type=6：群信息变更，使其改名权限即时生效）
            ClientSessionPtr ms = imserver.GetSessionByID(memberId);
            if (ms) ms->SendUserStatusChangeMsg(groupId, 6);
        } else {
            rsp.set_code(100); rsp.set_msg("操作失败");
        }
    }

    im::MessageContainer response;
    response.set_cmd(msg_type_setgroupadmin);
    response.set_seq(m_seq);
    response.set_payload(rsp.SerializeAsString());
    m_codec->send(conn, response);
}
