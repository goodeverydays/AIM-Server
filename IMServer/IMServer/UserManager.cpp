#include "UserManager.h"
#include "MySqlManager.h"
#include "base/Singleton.h"
#include "im.pb.h"
#include "PasswordUtil.h"
#include <sstream>

using namespace std;
using namespace muduo;

namespace {
    // SQL字符串转义：转义反斜杠与单引号，降低 SQL 注入风险。
    // 注：完整方案应使用预处理语句（prepared statement）；此处为最小加固。
    string EscapeSqlString(const string& str) {
        string result;
        result.reserve(str.size());
        for (char c : str) {
            if (c == '\\') {
                result += "\\\\";   // 反斜杠转义，防止吃掉后续引号
            } else if (c == '\'') {
                result += "''";
            } else {
                result += c;
            }
        }
        return result;
    }
}

bool UserManager::init()
{
	//加载用户信息（所有的用户）
	if(LoadUserFromDB() != true)
	{
		cout << "LoadUserFromDB failed!";
		return false;
	}
	//加载用户关系（好友关系）
	for (auto& iter : m_cachedUsers)
	{
		if (!LoadRelationshipFromDB(iter->userid, iter->friends))
		{
			cout << "load friend failed!\r\n";
		}
		//群账号(userid >= 0xFFFFFFF)额外加载管理员列表
		if (iter->userid >= 0xFFFFFFF)
		{
			LoadGroupAdminsFromDB(iter->userid, iter->admins);
		}
	}
	return true;
}

int UserManager::AddUser(User& user)
//添加用户。返回值：0=成功，1=用户名已占用，2=邮箱已注册，3=数据库错误
{
	// 整段加锁：查重→取号→入库→更新缓存 作为原子操作，
	// 杜绝并发注册导致的用户名重复、ID 竞态等问题。
	lock_guard<mutex> guard(m_mutex);

	// 用户名 / 邮箱查重（启动时全量用户已载入内存，此处即权威）
	for (const auto& iter : m_cachedUsers)
	{
		if (iter->username == user.username) return 1;
		if (!user.mail.empty() && iter->mail == user.mail) return 2;
	}

	// 密码加盐哈希后存储（不再保存明文）
	std::string hashed = pwd::HashPassword(user.password);

	int32_t newid = m_baseUserID + 1;
	stringstream sql;
	sql << "INSERT INTO t_user (f_user_id, f_username, f_nickname, f_password, f_mail, f_register_time)" <<
		"VALUES (" << newid << ", '" << EscapeSqlString(user.username) << "', '"
		<< EscapeSqlString(user.nickname) << "', '" << EscapeSqlString(hashed) << "', '"
		<< EscapeSqlString(user.mail) << "', NOW())";
	if (Singleton<MySqlManager>::instance().Execute(sql.str()) == false)
	{
		cout << "insert user failed!\r\n";
		return 3;
	}
	m_baseUserID = newid;//仅在 INSERT 成功后推进基数，避免失败留下 ID 空洞

	user.userid = m_baseUserID;
	user.password = hashed;//缓存与数据库保持一致（存哈希）
	user.facetype = 0;
	user.birthday = 20000101;
	user.gender = 0;
	user.ownerid = 0;

	// 两个索引（按名遍历的列表 / 按ID查找的映射）必须共享同一个 User 对象，
	// 否则好友关系、头像等字段在一处更新后，另一处会读到过期数据
	UserPtr sp = std::make_shared<User>(user);
	m_cachedUsers.push_back(sp);
	m_mapUsers[user.userid] = sp;
	return 0;
}

bool UserManager::LoadUserFromDB()
//从数据库加载用户信息
{
	stringstream sql;
	sql << "SELECT f_user_id, f_username, f_nickname, f_password, f_facetype, f_customface,"
		<< " f_gender, f_birthday, f_signature, f_address, f_phonenumber, f_mail, f_owner_id FROM t_user ORDER BY f_user_id DESC";
	//为什么用DESC：先注册的用户，id小， 后注册的用户id大，后注册的用户上线的概论大， 所以降序排列
	QueryResultPtr result = Singleton<MySqlManager>::instance().Query(sql.str());
	if (result == NULL)
	{
		return false;
	}
	while (result != NULL)
	{
		Field* pRow = result->Fetch();
		if (pRow == NULL) break;
		User u;
		u.userid = pRow[0].toInt32();
		u.username = pRow[1].GetString();
		u.nickname = pRow[2].GetString();
		u.password = pRow[3].GetString();
		u.facetype = pRow[4].toInt32();
		u.customface = pRow[5].GetString();
		u.gender = pRow[6].toInt32();
		u.birthday = pRow[7].toInt32();
		u.signature = pRow[8].GetString();
		u.address = pRow[9].GetString();
		u.phonenumber = pRow[10].GetString();
		u.mail = pRow[11].GetString();
		u.ownerid = pRow[12].toInt32();   // 群账号的群主ID(普通用户为0)；缺此字段会导致群主校验/识别失效
		{
			lock_guard<mutex> guard(m_mutex);//使用lock_guard对象自动管理互斥锁的锁定和释放，确保在访问和修改用户信息时线程安全，避免数据竞争和不一致的问题
			// 两个索引共享同一个 User 对象（关键：init() 加载好友关系时只填充
			// m_cachedUsers 中的对象，若此处是两份拷贝，m_mapUsers 里的 friends 将永远为空，
			// 导致 GetFriendInfoByUserID 查不到好友、头像/上线通知无法推送）
			UserPtr sp = std::make_shared<User>(u);
			m_cachedUsers.push_back(sp);
			m_mapUsers[u.userid] = sp;
		}
		if(u.userid < 0xFFFFFFF && u.userid > m_baseUserID)
		{
			m_baseUserID = u.userid;//更新基数，排除群ID，确保用户ID不落入群ID范围
		}
		if(u.userid >= 0xFFFFFFF && u.userid > m_baseGroupID)
		{
			m_baseGroupID = u.userid;//更新群ID基数，确保重启后群ID不冲突
		}
		if (result->NextRow() == false) break;//如果没有更多数据可供获取，跳出循环
	}
	result->EndQuery();//结束查询，释放资源
	return true;
}

bool UserManager::LoadRelationshipFromDB(int32_t userid, set<int32_t>& friends)
//从数据库加载用户关系信息，例如好友关系等
{
	stringstream sql;
	sql << "SELECT f_user_id1, f_user_id2 FROM t_user_relationship WHERE f_user_id1 = " << userid << " OR f_user_id2 = " << userid << " ;";
	QueryResultPtr result = Singleton<MySqlManager>::instance().Query(sql.str());
	if (result == NULL)
	{
		return false;
	}
	while (result != NULL)
	{
		Field* pRow = result->Fetch();
		if (pRow == NULL) break;
		int friendid1 = pRow[0].toInt32();
		int friendid2 = pRow[1].toInt32();
		if (friendid1 == userid)//如果查询结果中的用户ID1与当前用户ID匹配，说明用户ID2是当前用户的好友，将其插入到好友列表中
		{
			friends.insert(friendid2);
		}
		else if (friendid2 == userid)
		{
			friends.insert(friendid1);
		}
		if (result->NextRow() == false) break;//如果没有更多数据可供获取，跳出循环
	}
	result->EndQuery();//结束查询，释放资源
	return true;
}

bool UserManager::GetUserInfoUsername(const string& name, UserPtr& user)
{
	lock_guard<mutex> guard(m_mutex);//使用lock_guard对象自动管理互斥锁的锁定和释放，确保在访问和修改用户信息时线程安全，避免数据竞争和不一致的问题
	for (const auto& iter : m_cachedUsers)
	{
		if (iter->username == name)//遍历缓存用户信息的列表，查找与给定用户名匹配的用户对象，如果找到，则将其赋值给参数user，并返回true
		{
			user = make_shared<User>(*iter);  // 返回深拷贝快照：多 IO 线程下避免共享可变对象引发数据竞争
			return true;
		}
	}
	return false;//如果没有找到匹配的用户对象，则返回false
}

bool UserManager::GetFriendInfoByUserID(int32_t userid, list<UserPtr>& friends)
{
	lock_guard<mutex> guard(m_mutex);
	iterMapUser iter = m_mapUsers.find(userid);
	if (iter == m_mapUsers.end()) return false;
	//遍历该用户的好友ID集合，查找每个好友的详细信息并加入结果列表
	for (int32_t friendid : iter->second->friends)
	{
		iterMapUser itFriend = m_mapUsers.find(friendid);
		if (itFriend != m_mapUsers.end())
		{
			friends.push_back(make_shared<User>(*(itFriend->second)));  // 深拷贝快照，避免调用方在锁外读共享对象
		}
	}
	return true;
}

UserPtr UserManager::GetUserByID(int32_t userid)
{
	lock_guard<mutex> guard(m_mutex);
	iterMapUser iter = m_mapUsers.find(userid);//在用户ID到用户对象的映射中查找与给定用户ID匹配的用户对象，如果找到，则返回其对应的智能指针，否则返回空指针
	if (iter == m_mapUsers.end()) return UserPtr();
	return make_shared<User>(*(iter->second));  // 深拷贝快照，调用方在锁外读取/修改不影响缓存、不与他线程竞争
}

// 线程安全地更新缓存中用户的在线状态(登录/登出用)。
bool UserManager::SetUserStatus(int32_t userid, int32_t status)
{
	lock_guard<mutex> guard(m_mutex);
	iterMapUser iter = m_mapUsers.find(userid);
	if (iter == m_mapUsers.end()) return false;
	iter->second->status = status;
	return true;
}

bool UserManager::MakeFriendRelationship(int32_t smallid, int32_t greatid)
{
	if (smallid >= greatid) return false;//判断小ID是否大于等于大ID，如果是，则返回false，表示无法建立好友关系，因为小ID应该小于大ID
	stringstream sql;
	sql << "INSERT INTO t_user_relationship(f_user_id1, f_user_id2) VALUES(" <<
		smallid << ", " << greatid << ")";//构造SQL插入语句，将好友关系信息插入到数据库中，假设t_user_relationship表已经存在，并且包含相应的字段
	if (!Singleton<MySqlManager>::instance().Execute(sql.str()))
	{
		return false;
	}
	//修改缓存中的好友关系
	lock_guard<mutex> guard(m_mutex);
	//使用lock_guard对象自动管理互斥锁的锁定和释放，确保在访问和修改用户信息时线程安全，避免数据竞争和不一致的问题
	iterMapUser it = m_mapUsers.find(smallid);//在用户ID到用户对象的映射中查找与小ID匹配的用户对象，如果找到，则将大ID插入到其好友列表中
	if (it == m_mapUsers.end()) return false;
	it->second->friends.insert(greatid);
	it = m_mapUsers.find(greatid);//在用户ID到用户对象的映射
	if (it == m_mapUsers.end()) return false;
	it->second->friends.insert(smallid);
	return true;
}

bool UserManager::ReleaseFriendRelationship(int32_t smallid, int32_t greatid)
{
	if (smallid >= greatid) return false;
	stringstream sql;
	sql << "DELETE FROM t_user_relationship WHERE f_user_id1 = "
		<< smallid << " AND f_user_id2 = " << greatid;
	if (!Singleton<MySqlManager>::instance().Execute(sql.str()))
	{
		return false;
	}
	//更新缓存中双方的好友列表
	lock_guard<mutex> guard(m_mutex);
	iterMapUser itSmall = m_mapUsers.find(smallid);
	if (itSmall != m_mapUsers.end())
	{
		itSmall->second->friends.erase(greatid);
	}
	iterMapUser itGreat = m_mapUsers.find(greatid);
	if (itGreat != m_mapUsers.end())
	{
		itGreat->second->friends.erase(smallid);
	}
	return true;
}

bool UserManager::UpdateUserInfo(int32_t userid, const User& newuserinfo)
{
	stringstream sql;
	sql << "UPDATE t_user SET "
		<< "f_nickname = '" << EscapeSqlString(newuserinfo.nickname) << "', "
		<< "f_facetype = " << newuserinfo.facetype << ", "
		<< "f_customface = '" << EscapeSqlString(newuserinfo.customface) << "', "
		<< "f_gender = " << newuserinfo.gender << ", "
		<< "f_birthday = " << newuserinfo.birthday << ", "
		<< "f_signature = '" << EscapeSqlString(newuserinfo.signature) << "', "
		<< "f_address = '" << EscapeSqlString(newuserinfo.address) << "', "
		<< "f_phonenumber = '" << EscapeSqlString(newuserinfo.phonenumber) << "', "
		<< "f_mail = '" << EscapeSqlString(newuserinfo.mail) << "' "
		<< "WHERE f_user_id = " << userid;
	if (!Singleton<MySqlManager>::instance().Execute(sql.str()))
	{
		return false;
	}
	//更新缓存中的用户信息
	lock_guard<mutex> guard(m_mutex);
	iterMapUser iter = m_mapUsers.find(userid);
	if (iter != m_mapUsers.end())
	{
		iter->second->nickname = newuserinfo.nickname;
		iter->second->facetype = newuserinfo.facetype;
		iter->second->customface = newuserinfo.customface;
		iter->second->gender = newuserinfo.gender;
		iter->second->birthday = newuserinfo.birthday;
		iter->second->signature = newuserinfo.signature;
		iter->second->address = newuserinfo.address;
		iter->second->phonenumber = newuserinfo.phonenumber;
		iter->second->mail = newuserinfo.mail;
	}
	return true;
}

bool UserManager::ModifyUserPassword(int32_t userid, const string& newpassword)
{
	// 新密码同样加盐哈希后存储
	std::string hashed = pwd::HashPassword(newpassword);
	stringstream sql;
	sql << "UPDATE t_user SET f_password = '"
		<< EscapeSqlString(hashed) << "' WHERE f_user_id = " << userid;
	if (!Singleton<MySqlManager>::instance().Execute(sql.str()))
	{
		return false;
	}
	//更新缓存中的密码（存哈希，与数据库一致）
	lock_guard<mutex> guard(m_mutex);
	iterMapUser iter = m_mapUsers.find(userid);
	if (iter != m_mapUsers.end())
	{
		iter->second->password = hashed;
	}
	return true;
}

bool UserManager::AddGroup(const char* groupname, int32_t ownerid, int32_t& groupid)
{
	m_baseGroupID++;
	groupid = m_baseGroupID;
	stringstream sql;
	sql << "INSERT INTO t_user (f_user_id, f_username, f_nickname, f_password, f_owner_id, f_register_time) VALUES ("
		<< groupid << ", '" << EscapeSqlString(groupname) << "', '"
		<< EscapeSqlString(groupname) << "', '', " << ownerid << ", NOW())";
	if (!Singleton<MySqlManager>::instance().Execute(sql.str()))
	{
		return false;
	}
	//将群组作为特殊用户加入缓存
	User group;
	group.userid = groupid;
	group.username = groupname;
	group.nickname = groupname;
	group.ownerid = ownerid;
	group.facetype = 0;
	group.gender = 0;
	group.birthday = 20000101;
	{
		lock_guard<mutex> guard(m_mutex);
		// 两个索引共享同一个 User 对象，保持数据一致
		UserPtr sp = std::make_shared<User>(group);
		m_cachedUsers.push_back(sp);
		m_mapUsers[groupid] = sp;
	}
	return true;
}

bool UserManager::RenameGroup(int32_t groupid, const std::string& newname)
{
	// 群账号在 t_user 中以特殊用户存在，群名即其 f_nickname/f_username
	stringstream sql;
	sql << "UPDATE t_user SET f_nickname = '" << EscapeSqlString(newname)
		<< "', f_username = '" << EscapeSqlString(newname)
		<< "' WHERE f_user_id = " << groupid;
	if (!Singleton<MySqlManager>::instance().Execute(sql.str()))
	{
		return false;
	}
	// 同步缓存
	lock_guard<mutex> guard(m_mutex);
	iterMapUser iter = m_mapUsers.find(groupid);
	if (iter != m_mapUsers.end())
	{
		iter->second->nickname = newname;
		iter->second->username = newname;
	}
	return true;
}

bool UserManager::LoadGroupAdminsFromDB(int32_t groupid, set<int32_t>& admins)
{
	stringstream sql;
	sql << "SELECT f_user_id FROM t_group_admin WHERE f_group_id = " << groupid;
	QueryResultPtr result = Singleton<MySqlManager>::instance().Query(sql.str());
	if (result == NULL) return false;
	while (result != NULL)
	{
		Field* pRow = result->Fetch();
		if (pRow == NULL) break;
		admins.insert(pRow[0].toInt32());
		if (result->NextRow() == false) break;
	}
	result->EndQuery();
	return true;
}

bool UserManager::IsGroupAdmin(int32_t groupid, int32_t userid)
{
	lock_guard<mutex> guard(m_mutex);
	iterMapUser it = m_mapUsers.find(groupid);
	if (it == m_mapUsers.end()) return false;
	return it->second->admins.count(userid) > 0;
}

bool UserManager::SetGroupAdmin(int32_t groupid, int32_t userid, bool isAdmin)
{
	stringstream sql;
	if (isAdmin)
	{
		// 先查重，避免重复插入
		stringstream q;
		q << "SELECT f_id FROM t_group_admin WHERE f_group_id = " << groupid
		  << " AND f_user_id = " << userid << " LIMIT 1";
		QueryResultPtr r = Singleton<MySqlManager>::instance().Query(q.str());
		bool exists = (r != NULL && r->Fetch() != NULL);
		if (r != NULL) r->EndQuery();
		if (!exists)
		{
			sql << "INSERT INTO t_group_admin (f_group_id, f_user_id, f_create_time) VALUES ("
				<< groupid << ", " << userid << ", NOW())";
			if (!Singleton<MySqlManager>::instance().Execute(sql.str())) return false;
		}
	}
	else
	{
		sql << "DELETE FROM t_group_admin WHERE f_group_id = " << groupid
			<< " AND f_user_id = " << userid;
		if (!Singleton<MySqlManager>::instance().Execute(sql.str())) return false;
	}
	// 同步缓存
	lock_guard<mutex> guard(m_mutex);
	iterMapUser it = m_mapUsers.find(groupid);
	if (it != m_mapUsers.end())
	{
		if (isAdmin) it->second->admins.insert(userid);
		else         it->second->admins.erase(userid);
	}
	return true;
}

bool UserManager::SaveChatMsgToDb(int32_t senderid, int32_t targetid, const im::ChatMsg& msg)
{
	// 文本 + 媒体元数据一并持久化；列名与建表(f_create_time)一致
	stringstream sql;
	sql << "INSERT INTO t_chatmsg (f_senderid, f_targetid, f_msgcontent, f_msgtype, "
		<< "f_media_url, f_duration, f_thumb_url, f_filesize, f_filename, f_create_time) VALUES ("
		<< senderid << ", " << targetid << ", '"
		<< EscapeSqlString(msg.content()) << "', "
		<< msg.msg_type() << ", '"
		<< EscapeSqlString(msg.media_url()) << "', "
		<< msg.duration() << ", '"
		<< EscapeSqlString(msg.thumb_url()) << "', "
		<< msg.file_size() << ", '"
		<< EscapeSqlString(msg.file_name()) << "', ";
	// 用发送端时间戳作为 f_create_time，保证服务端历史与客户端实时/本地缓存的
	// timestamp 完全一致，客户端据此精确去重(避免同一条消息显示多次)。
	if (msg.timestamp() > 0)
		sql << "FROM_UNIXTIME(" << msg.timestamp() << "))";
	else
		sql << "NOW())";
	return Singleton<MySqlManager>::instance().Execute(sql.str());
}

bool UserManager::GetChatHistory(int32_t userid, int32_t targetid, std::list<im::ChatMsg>& messages, int limit)
{
	stringstream sql;
	const char* cols = "SELECT f_senderid, f_targetid, f_msgcontent, UNIX_TIMESTAMP(f_create_time), "
	                   "f_msgtype, f_media_url, f_duration, f_thumb_url, f_filesize, f_filename FROM t_chatmsg ";
	if (targetid >= 0xFFFFFFF)
	{
		// 群聊：查所有发给该群的消息
		sql << cols
			<< "WHERE f_targetid = " << targetid << " ORDER BY f_create_time DESC LIMIT " << limit;
	}
	else
	{
		// 单聊：查双向消息
		sql << cols
			<< "WHERE (f_senderid = " << userid << " AND f_targetid = " << targetid << ") "
			<< "OR (f_senderid = " << targetid << " AND f_targetid = " << userid << ") "
			<< "ORDER BY f_create_time DESC LIMIT " << limit;
	}
	QueryResultPtr result = Singleton<MySqlManager>::instance().Query(sql.str());
	if (result == NULL) return false;
	while (result != NULL)
	{
		Field* pRow = result->Fetch();
		if (pRow == NULL) break;
		im::ChatMsg msg;
		msg.set_senderid(pRow[0].toInt32());
		msg.set_targetid(pRow[1].toInt32());
		msg.set_content(pRow[2].GetString());
		msg.set_timestamp(pRow[3].toInt64());
		msg.set_msg_type(pRow[4].toInt32());
		msg.set_media_url(pRow[5].GetString());
		msg.set_duration(pRow[6].toInt32());
		msg.set_thumb_url(pRow[7].GetString());
		msg.set_file_size(pRow[8].toInt64());
		msg.set_file_name(pRow[9].GetString());
		messages.push_back(msg);
		if (result->NextRow() == false) break;
	}
	result->EndQuery();
	return true;
}

bool UserManager::DeleteFriendToUser(int32_t userid, int32_t friendid)
{
	int32_t smallid = (userid < friendid) ? userid : friendid;
	int32_t greatid = (userid < friendid) ? friendid : userid;
	stringstream sql;
	sql << "DELETE FROM t_user_relationship WHERE f_user_id1 = "
		<< smallid << " AND f_user_id2 = " << greatid;
	if (!Singleton<MySqlManager>::instance().Execute(sql.str()))
	{
		return false;
	}
	//更新缓存中双方的好友列表
	lock_guard<mutex> guard(m_mutex);
	iterMapUser iterUser = m_mapUsers.find(userid);
	if (iterUser != m_mapUsers.end())
	{
		iterUser->second->friends.erase(friendid);
	}
	iterMapUser iterFriend = m_mapUsers.find(friendid);
	if (iterFriend != m_mapUsers.end())
	{
		iterFriend->second->friends.erase(userid);
	}
	return true;
}

// ── 好友/入群 请求-审批 ──

bool UserManager::IsFriend(int32_t userid, int32_t friendid)
{
	lock_guard<mutex> guard(m_mutex);
	iterMapUser it = m_mapUsers.find(userid);
	if (it == m_mapUsers.end()) return false;
	return it->second->friends.count(friendid) > 0;
}

bool UserManager::AddFriendRequest(int32_t fromId, int32_t toId, int32_t reqType,
                                   int32_t groupId, const string& msg, int64_t& reqIdOut)
{
	MySqlManager& db = Singleton<MySqlManager>::instance();
	// 复用已存在的待处理请求，避免重复堆积
	{
		stringstream q;
		q << "SELECT f_id FROM t_friend_request WHERE f_from_id = " << fromId
		  << " AND f_to_id = " << toId << " AND f_reqtype = " << reqType
		  << " AND f_group_id = " << groupId << " AND f_status = 0 LIMIT 1";
		QueryResultPtr r = db.Query(q.str());
		if (r != NULL)
		{
			Field* row = r->Fetch();
			if (row != NULL)
			{
				reqIdOut = row[0].toInt64();
				r->EndQuery();
				return true;
			}
			r->EndQuery();
		}
	}
	stringstream sql;
	sql << "INSERT INTO t_friend_request (f_from_id, f_to_id, f_reqtype, f_group_id, f_message, f_status, f_create_time) VALUES ("
		<< fromId << ", " << toId << ", " << reqType << ", " << groupId << ", '"
		<< EscapeSqlString(msg) << "', 0, NOW())";
	if (!db.Execute(sql.str())) return false;

	reqIdOut = 0;
	QueryResultPtr r = db.Query("SELECT LAST_INSERT_ID()");
	if (r != NULL)
	{
		Field* row = r->Fetch();
		if (row != NULL) reqIdOut = row[0].toInt64();
		r->EndQuery();
	}
	return true;
}

bool UserManager::GetPendingRequests(int32_t toId, std::list<FriendRequestInfo>& out)
{
	stringstream sql;
	sql << "SELECT f_id, f_from_id, f_to_id, f_reqtype, f_group_id, f_message, UNIX_TIMESTAMP(f_create_time) "
		<< "FROM t_friend_request WHERE f_to_id = " << toId
		<< " AND f_status = 0 AND f_from_id > 0 ORDER BY f_id DESC";   // 过滤脏行(发起方ID为0)
	QueryResultPtr result = Singleton<MySqlManager>::instance().Query(sql.str());
	if (result == NULL) return false;
	while (result != NULL)
	{
		Field* row = result->Fetch();
		if (row == NULL) break;
		FriendRequestInfo info;
		info.reqid      = row[0].toInt64();
		info.fromid     = row[1].toInt32();
		info.toid       = row[2].toInt32();
		info.reqtype    = row[3].toInt32();
		info.groupid    = row[4].toInt32();
		info.message    = row[5].GetString();
		info.createtime = row[6].toInt64();
		out.push_back(info);
		if (result->NextRow() == false) break;
	}
	result->EndQuery();
	return true;
}

bool UserManager::GetFriendRequestById(int64_t reqId, FriendRequestInfo& out)
{
	stringstream sql;
	sql << "SELECT f_id, f_from_id, f_to_id, f_reqtype, f_group_id, f_message, UNIX_TIMESTAMP(f_create_time) "
		<< "FROM t_friend_request WHERE f_id = " << reqId << " LIMIT 1";
	QueryResultPtr result = Singleton<MySqlManager>::instance().Query(sql.str());
	if (result == NULL) return false;
	Field* row = result->Fetch();
	bool ok = false;
	if (row != NULL)
	{
		out.reqid      = row[0].toInt64();
		out.fromid     = row[1].toInt32();
		out.toid       = row[2].toInt32();
		out.reqtype    = row[3].toInt32();
		out.groupid    = row[4].toInt32();
		out.message    = row[5].GetString();
		out.createtime = row[6].toInt64();
		ok = true;
	}
	result->EndQuery();
	return ok;
}

bool UserManager::UpdateRequestStatus(int64_t reqId, int32_t status)
{
	stringstream sql;
	sql << "UPDATE t_friend_request SET f_status = " << status << " WHERE f_id = " << reqId;
	return Singleton<MySqlManager>::instance().Execute(sql.str());
}
