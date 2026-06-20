#pragma once
#include <iostream>
#include <stdio.h>
#include <unistd.h>
#include <sys/types.h>
#include <errno.h>
#include "base/Logging.h"
#include "base/Singleton.h"
#include <set>
#include <list>
#include <mutex>
#include<memory>
#include<map>

// 前向声明 proto 类型
namespace im {
class ChatMsg;
}

using namespace std;

class User
{
public:
	int32_t		 userid;//用户ID
	string		 username;//用户名
	string		 nickname;//昵称
	string		 password;//密码
	int32_t		 facetype;//默认头像
	string		 customface;//自定义头像
	string		 customfacefmt;//自定义头像格式
	int32_t		 gender;//性别
	int32_t		 birthday;//生日
	string		 signature;//个性签名
	string		 address;//地址
	string       phonenumber;//电话号码
	string		 mail;//邮箱
	int32_t      status;//用户在线状态 0离线 1在线
	int32_t		 ownerid;//用于标记群主信息的
	set<int32_t> friends;//好友列表，存储用户的好友ID，可以使用set容器来管理好友关系，方便进行添加、删除和查询操作
	set<int32_t> admins;//群管理员列表（仅群账号有效），由群主设置
	User() : userid(-1), facetype(-1), gender(-1),
		birthday(-1), status(0), ownerid(-1) {}
	~User() = default;
};

typedef std::shared_ptr<User> UserPtr;

// 好友/入群 待处理请求
struct FriendRequestInfo {
	int64_t     reqid    = 0;
	int32_t     fromid   = 0;
	int32_t     toid     = 0;
	int32_t     reqtype  = 1;   // 1=好友, 2=入群
	int32_t     groupid  = 0;
	std::string message;
	int64_t     createtime = 0;
};

class UserManager final
{
public:
	UserManager() = default;
	~UserManager() = default;
	bool init();
	UserManager(const UserManager&) = delete;
	UserManager& operator=(const UserManager&) = delete;

	//添加用户。返回：0=成功，1=用户名已占用，2=邮箱已注册，3=数据库错误
	int AddUser(User& user);
	//从数据库加载用户信息
	bool LoadUserFromDB();
	bool LoadRelationshipFromDB(int32_t userid, set<int32_t>& friends);//从数据库加载用户关系信息，例如好友关系等
	bool GetUserInfoUsername(const string& name, UserPtr& user);
	bool GetFriendInfoByUserID(int32_t userid, list<UserPtr>& friends);
	UserPtr GetUserByID(int32_t userid);
	bool SetUserStatus(int32_t userid, int32_t status);  // 线程安全更新缓存中的在线状态(0离线/1在线)
	bool MakeFriendRelationship(int32_t smallid, int32_t greatid);
	bool ReleaseFriendRelationship(int32_t smallid, int32_t greatid);
	bool UpdateUserInfo(int32_t userid, const User& newuserinfo);
	bool ModifyUserPassword(int32_t userid, const string& newpassword);
	bool AddGroup(const char* groupname, int32_t ownerid, int32_t& groupid);
	// 重命名群（更新 t_user 中群账号的昵称/用户名，并同步缓存）
	bool RenameGroup(int32_t groupid, const std::string& newname);
	// 群管理员：加载/判断/设置
	bool LoadGroupAdminsFromDB(int32_t groupid, set<int32_t>& admins);
	bool IsGroupAdmin(int32_t groupid, int32_t userid);   // 是否为该群管理员(不含群主)
	bool SetGroupAdmin(int32_t groupid, int32_t userid, bool isAdmin);  // 设置/取消管理员(DB+缓存)
	bool SaveChatMsgToDb(int32_t senderid, int32_t targetid, const im::ChatMsg& msg);
	bool GetChatHistory(int32_t userid, int32_t targetid, std::list<im::ChatMsg>& messages, int limit = 50);
	bool DeleteFriendToUser(int32_t userid, int32_t friendid);

	// ── 好友/入群 请求-审批 ──
	// 新增待处理请求；若已存在同样的待处理请求则复用其ID。返回是否成功，reqIdOut 输出请求ID
	bool AddFriendRequest(int32_t fromId, int32_t toId, int32_t reqType,
	                      int32_t groupId, const std::string& msg, int64_t& reqIdOut);
	// 拉取某用户的待处理(status=0)请求列表
	bool GetPendingRequests(int32_t toId, std::list<FriendRequestInfo>& out);
	// 按ID取请求
	bool GetFriendRequestById(int64_t reqId, FriendRequestInfo& out);
	// 更新请求状态：1=接受 2=拒绝
	bool UpdateRequestStatus(int64_t reqId, int32_t status);
	// 判断两用户是否已是好友
	bool IsFriend(int32_t userid, int32_t friendid);

private:
	mutex m_mutex;//互斥锁对象，用于保护对用户信息的访问和修改，确保线程安全，避免数据竞争和不一致的问题
	list<UserPtr> m_cachedUsers;//缓存用户信息的列表，可以使用list容器来存储用户对象，方便进行遍历和管理
	std::map<int32_t, UserPtr> m_mapUsers;//用户ID与用户对象的映射关系，可以使用map容器来存储用户ID和用户对象的对应关系，方便根据用户ID快速查找用户信息
	int32_t m_baseUserID{ 0 };//用于生成新的用户ID的基数，可以根据需要进行自增或者其他操作来确保每个用户都有一个唯一的ID
	int32_t m_baseGroupID{ 0xFFFFFFF };//用于生成新的群ID的基数，可以根据需要进行自增或者其他操作来确保每个群都有一个唯一的ID
	
};

typedef std::map<int32_t, UserPtr>::iterator iterMapUser;
typedef set<int32_t>::iterator iterSetUserID;