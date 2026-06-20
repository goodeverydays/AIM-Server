#include "MsgCacheManager.h"
#include "MySqlManager.h"
#include "base/Singleton.h"
#include "base/Logging.h"
#include <sstream>

// QueryResult.h 无 include guard，已由 MySqlManager.h 间接引入，此处不可重复 include。
// Singleton 位于 muduo 命名空间，需 using（与 UserManager.cpp 保持一致）。
using namespace std;
using namespace muduo;

namespace {
	// 离线消息类型
	const int kMsgTypeNotify = 0;
	const int kMsgTypeChat   = 1;

	// 二进制 -> 大写十六进制（用于 MySQL X'..' 字面量）。
	// 离线消息是序列化后的 protobuf，含 NUL、引号、反斜杠等，EscapeSqlString 只转义单引号、
	// 对二进制不安全，故统一用十六进制编码入库、用 HEX() 取出。
	std::string ToHex(const std::string& data)
	{
		static const char* k = "0123456789ABCDEF";
		std::string out;
		out.reserve(data.size() * 2);
		for (unsigned char c : data)
		{
			out.push_back(k[c >> 4]);
			out.push_back(k[c & 0x0F]);
		}
		return out;
	}

	int HexVal(char c)
	{
		if (c >= '0' && c <= '9') return c - '0';
		if (c >= 'A' && c <= 'F') return c - 'A' + 10;
		if (c >= 'a' && c <= 'f') return c - 'a' + 10;
		return 0;
	}

	// 十六进制 -> 二进制
	std::string FromHex(const std::string& hex)
	{
		std::string out;
		out.reserve(hex.size() / 2);
		for (size_t i = 0; i + 1 < hex.size(); i += 2)
		{
			out.push_back(static_cast<char>((HexVal(hex[i]) << 4) | HexVal(hex[i + 1])));
		}
		return out;
	}

	// 写入一条离线消息
	bool InsertOfflineMsg(int32_t userid, int msgtype, const std::string& blob)
	{
		std::stringstream sql;
		sql << "INSERT INTO t_offline_msg (f_userid, f_msgtype, f_content) VALUES ("
			<< userid << ", " << msgtype << ", X'" << ToHex(blob) << "')";
		return Singleton<MySqlManager>::instance().Execute(sql.str());
	}

	// 取出某用户某类型的全部离线消息（按入队顺序），并从库中删除避免重复推送
	void FetchAndDeleteOfflineMsg(int32_t userid, int msgtype, std::list<std::string>& out)
	{
		std::stringstream sql;
		sql << "SELECT HEX(f_content) FROM t_offline_msg WHERE f_userid = " << userid
			<< " AND f_msgtype = " << msgtype << " ORDER BY f_id ASC";
		QueryResultPtr result = Singleton<MySqlManager>::instance().Query(sql.str());
		if (result == NULL) return;
		while (result != NULL)
		{
			Field* pRow = result->Fetch();
			if (pRow == NULL) break;
			out.push_back(FromHex(pRow[0].GetString()));
			if (result->NextRow() == false) break;
		}
		result->EndQuery();

		// 已取出，删除对应行，避免下次登录重复推送
		if (!out.empty())
		{
			std::stringstream del;
			del << "DELETE FROM t_offline_msg WHERE f_userid = " << userid
				<< " AND f_msgtype = " << msgtype;
			Singleton<MySqlManager>::instance().Execute(del.str());
		}
	}
}

MsgCacheManager::MsgCacheManager()
{
}

MsgCacheManager::~MsgCacheManager()
{
}

bool MsgCacheManager::AddNotifyMsgCache(int32_t userid, const std::string& cache)
{
	std::lock_guard<std::mutex> guard(m_mutex);
	bool ok = InsertOfflineMsg(userid, kMsgTypeNotify, cache);
	LOG_INFO << "persist offline notify msg, userid: " << userid
		<< ", length: " << cache.length() << ", ok: " << ok;
	return ok;
}

void MsgCacheManager::GetNotifyMsgCache(int32_t userid, std::list<NotifyMsgCache>& cached)
{
	std::lock_guard<std::mutex> guard(m_mutex);
	std::list<std::string> blobs;
	FetchAndDeleteOfflineMsg(userid, kMsgTypeNotify, blobs);
	for (auto& b : blobs)
	{
		NotifyMsgCache nc;
		nc.userid = userid;
		nc.notifymsg = std::move(b);
		cached.push_back(std::move(nc));
	}
	LOG_INFO << "load offline notify msg, userid: " << userid << ", count: " << cached.size();
}

bool MsgCacheManager::AddChatMsgCache(int32_t userid, const std::string& cache)
{
	std::lock_guard<std::mutex> guard(m_mutex);
	bool ok = InsertOfflineMsg(userid, kMsgTypeChat, cache);
	LOG_INFO << "persist offline chat msg, userid: " << userid
		<< ", length: " << cache.length() << ", ok: " << ok;
	return ok;
}

void MsgCacheManager::GetChatMsgCache(int32_t userid, std::list<ChatMsgCache>& cached)
{
	std::lock_guard<std::mutex> guard(m_mutex);
	std::list<std::string> blobs;
	FetchAndDeleteOfflineMsg(userid, kMsgTypeChat, blobs);
	for (auto& b : blobs)
	{
		ChatMsgCache c;
		c.userid = userid;
		c.chatmsg = std::move(b);
		cached.push_back(std::move(c));
	}
	LOG_INFO << "load offline chat msg, userid: " << userid << ", count: " << cached.size();
}
