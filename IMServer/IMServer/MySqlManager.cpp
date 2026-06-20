#include "MySqlManager.h"
#include <sstream>

MySqlManager::MySqlManager()
{
	//用户
	sTableInfo info;
	info.sName = "t_user";
	info.mapField["f_id"] = { "f_id", "bigint(20) NOT NULL AUTO_INCREMENT COMMENT '自增ID'", "bigint(20)" };
	info.mapField["f_user_id"] = { "f_user_id", "bigint(20) NOT NULL COMMENT '用户ID'", "bigint(20)" };
	info.mapField["f_username"] = { "f_username", "varchar(64) NOT NULL COMMENT '用户名'", "varchar(64)" };
	info.mapField["f_nickname"] = { "f_nickname", "varchar(64) NOT NULL COMMENT '用户昵称'", "varchar(64)" };
	info.mapField["f_password"] = { "f_password", "varchar(128) NOT NULL COMMENT '用户密码'", "varchar(128)" };
	info.mapField["f_facetype"] = { "f_facetype", "int(10) DEFAULT 0 COMMENT '用户头像类型'", "int(10)" };
	info.mapField["f_customface"] = { "f_customface", "varchar(255) DEFAULT NULL COMMENT '自定义头像URL'", "varchar(255)" };
	info.mapField["f_customfacefmt"] = { "f_customfacefmt", "varchar(6) DEFAULT NULL COMMENT '自定义头像格式'", "varchar(6)" };
	info.mapField["f_gender"] = { "f_gender", "int(2)  DEFAULT 0 COMMENT '性别'", "int(2)" };
	info.mapField["f_birthday"] = { "f_birthday", "bigint(20)  DEFAULT 19900101 COMMENT '生日'", "bigint(20)" };
	info.mapField["f_signature"] = { "f_signature", "varchar(256) DEFAULT NULL COMMENT '地址'", "varchar(256)" };
	info.mapField["f_address"] = { "f_address", "varchar(256) DEFAULT NULL COMMENT '地址'", "varchar(256)" };
	info.mapField["f_phonenumber"] = { "f_phonenumber", "varchar(64) DEFAULT NULL COMMENT '电话'", "varchar(64)" };
	info.mapField["f_mail"] = { "f_mail", "varchar(256) DEFAULT NULL COMMENT '邮箱'", "varchar(256)" };
	info.mapField["f_owner_id"] = { "f_owner_id", "bigint(20) DEFAULT 0 COMMENT '群账号群主userid'", "bigint(20)" };
	info.mapField["f_register_time"] = { "f_register_time", "datetime NOT NULL COMMENT '注册时间'", "datetime" };
	info.mapField["f_remark"] = { "f_remark", "varchar(64) NULL COMMENT '备注'", "varchar(64)" };
	info.mapField["f_update_time"] = { "f_update_time", "timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'", "timestamp" };
	info.sKey = "PRIMARY KEY (f_user_id), INDEX f_user_id (f_user_id), KEY  f_id  ( f_id )";
	m_mapTable.insert(TablePair(info.sName, info));

	//聊天内容
	info.mapField.clear();
	info.sName = "t_chatmsg";
	info.mapField["f_id"] = { "f_id", "bigint(20) NOT NULL AUTO_INCREMENT COMMENT '自增ID'", "bigint(20)" };
	info.mapField["f_senderid"] = { "f_senderid", "bigint(20) NOT NULL COMMENT '发送者id'", "bigint(20)" };
	info.mapField["f_targetid"] = { "f_targetid", "bigint(20) NOT NULL COMMENT '接收者id'", "bigint(20)" };
	info.mapField["f_msgcontent"] = { "f_msgcontent", "blob NOT NULL COMMENT '聊天内容/媒体附带文字'", "blob" };
	info.mapField["f_msgtype"] = { "f_msgtype", "int(2) NOT NULL DEFAULT 0 COMMENT '0文本 1语音 2视频 3图片 4文件'", "int(2)" };
	info.mapField["f_media_url"] = { "f_media_url", "varchar(512) DEFAULT NULL COMMENT '媒体文件URL'", "varchar(512)" };
	info.mapField["f_duration"] = { "f_duration", "int(11) NOT NULL DEFAULT 0 COMMENT '语音/视频时长(秒)'", "int(11)" };
	info.mapField["f_thumb_url"] = { "f_thumb_url", "varchar(512) DEFAULT NULL COMMENT '视频缩略图URL'", "varchar(512)" };
	info.mapField["f_filesize"] = { "f_filesize", "bigint(20) NOT NULL DEFAULT 0 COMMENT '文件字节数'", "bigint(20)" };
	info.mapField["f_filename"] = { "f_filename", "varchar(256) DEFAULT NULL COMMENT '原始文件名'", "varchar(256)" };
	info.mapField["f_create_time"] = { "f_create_time", "timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'", "timestamp" };
	info.mapField["f_remark"] = { "f_remark", "varchar(64) NULL COMMENT '备注'", "varchar(64)" };
	info.sKey = "PRIMARY KEY (f_id), INDEX f_id (f_id)";
	m_mapTable.insert(TablePair(info.sName, info));

	//用户关系
	info.mapField.clear();
	info.sName = "t_user_relationship";
	info.mapField["f_id"] = { "f_id", "bigint(20) NOT NULL AUTO_INCREMENT COMMENT '自增ID'", "bigint(20)" };
	info.mapField["f_user_id1"] = { "f_user_id1", "bigint(20) NOT NULL COMMENT '用户ID'", "bigint(20)" };
	info.mapField["f_user_id2"] = { "f_user_id2", "bigint(20) NOT NULL COMMENT '用户ID'", "bigint(20)" };
	info.mapField["f_remark"] = { "f_remark", "varchar(64) NULL COMMENT '备注'", "varchar(64)" };
	info.mapField["f_create_time"] = { "f_create_time", "timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'", "timestamp" };
	info.sKey = "PRIMARY KEY (f_id), INDEX f_id (f_id)";
	m_mapTable.insert(TablePair(info.sName, info));

	//离线消息（接收方不在线时暂存，登录后补推；持久化防止进程重启丢失）
	info.mapField.clear();
	info.sName = "t_offline_msg";
	info.mapField["f_id"] = { "f_id", "bigint(20) NOT NULL AUTO_INCREMENT COMMENT '自增ID'", "bigint(20)" };
	info.mapField["f_userid"] = { "f_userid", "bigint(20) NOT NULL COMMENT '接收者id'", "bigint(20)" };
	info.mapField["f_msgtype"] = { "f_msgtype", "int(2) NOT NULL DEFAULT 1 COMMENT '0=通知 1=聊天'", "int(2)" };
	info.mapField["f_content"] = { "f_content", "blob NOT NULL COMMENT '序列化的MessageContainer'", "blob" };
	info.mapField["f_create_time"] = { "f_create_time", "timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间'", "timestamp" };
	info.sKey = "PRIMARY KEY (f_id), INDEX f_userid (f_userid)";
	m_mapTable.insert(TablePair(info.sName, info));

	//好友/入群 请求-审批（待处理请求持久化）
	info.mapField.clear();
	info.sName = "t_friend_request";
	info.mapField["f_id"] = { "f_id", "bigint(20) NOT NULL AUTO_INCREMENT COMMENT '请求ID'", "bigint(20)" };
	info.mapField["f_from_id"] = { "f_from_id", "bigint(20) NOT NULL COMMENT '发起者id'", "bigint(20)" };
	info.mapField["f_to_id"] = { "f_to_id", "bigint(20) NOT NULL COMMENT '接收者id'", "bigint(20)" };
	info.mapField["f_reqtype"] = { "f_reqtype", "int(2) NOT NULL DEFAULT 1 COMMENT '1=好友 2=入群'", "int(2)" };
	info.mapField["f_group_id"] = { "f_group_id", "bigint(20) DEFAULT 0 COMMENT '入群请求的群ID'", "bigint(20)" };
	info.mapField["f_message"] = { "f_message", "varchar(256) DEFAULT NULL COMMENT '验证留言'", "varchar(256)" };
	info.mapField["f_status"] = { "f_status", "int(2) NOT NULL DEFAULT 0 COMMENT '0=待处理 1=接受 2=拒绝'", "int(2)" };
	info.mapField["f_create_time"] = { "f_create_time", "timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间'", "timestamp" };
	info.sKey = "PRIMARY KEY (f_id), INDEX f_to_id (f_to_id)";
	m_mapTable.insert(TablePair(info.sName, info));

	//群管理员（群主可设置成员为管理员）
	info.mapField.clear();
	info.sName = "t_group_admin";
	info.mapField["f_id"] = { "f_id", "bigint(20) NOT NULL AUTO_INCREMENT COMMENT '自增ID'", "bigint(20)" };
	info.mapField["f_group_id"] = { "f_group_id", "bigint(20) NOT NULL COMMENT '群ID'", "bigint(20)" };
	info.mapField["f_user_id"] = { "f_user_id", "bigint(20) NOT NULL COMMENT '管理员用户ID'", "bigint(20)" };
	info.mapField["f_create_time"] = { "f_create_time", "timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间'", "timestamp" };
	info.sKey = "PRIMARY KEY (f_id), INDEX f_group_id (f_group_id)";
	m_mapTable.insert(TablePair(info.sName, info));
}

MySqlManager::~MySqlManager()
{

}

bool MySqlManager::Init(
	const string& host,
	const string user,
	const string passwd,
	const string dbname,
	unsigned port,
	int poolSize
)
{
	if (poolSize < 1) poolSize = 1;
	// 建立连接池：每条连接独立(MYSQL* 非线程安全)，多 IO 线程各借一条。
	for (int i = 0; i < poolSize; ++i)
	{
		auto conn = make_shared<MySQLTool>();
		if (conn->connect(host, user, passwd, dbname, port) == false)
		{
			cout << "connect mysql failed! (pool index " << i << ")\r\n";
			return false;
		}
		m_pool.push_back(conn);
		m_idle.push_back(conn);
	}
	m_mysql = m_pool[0];  // 启动期建库/建表用第一条(单线程，无竞争)
	cout << "mysql connection pool ready, size = " << poolSize << "\r\n";

	if(CheckDatabase() == false)
	{
		if (CreateDatabase() == false)
		{
			cout << "create database failed!\r\n";
			return false;
		}
	}
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	TableIter it = m_mapTable.begin();
	for (; it != m_mapTable.end(); it++)
	{
		cout << __FILE__ << "(" << __LINE__ << ")\r\n";
		if (CheckTable(it->second) == false)
		{
			cout << __FILE__ << "(" << __LINE__ << ")\r\n";
			if(CreateTable(it->second) == false)
			{
					cout << "create table " << it->second.sName << " failed!\r\n";
					return false;
			}
		}
		cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	}
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	return true;
}

shared_ptr<MySQLTool> MySqlManager::acquire()
{
	unique_lock<mutex> lk(m_poolMutex);
	m_poolCv.wait(lk, [this] { return !m_idle.empty(); });
	auto conn = m_idle.back();
	m_idle.pop_back();
	return conn;
}

void MySqlManager::release(const shared_ptr<MySQLTool>& conn)
{
	{
		lock_guard<mutex> lk(m_poolMutex);
		m_idle.push_back(conn);
	}
	m_poolCv.notify_one();
}

QueryResultPtr MySqlManager::Query(const string& sql)
{
	if (m_pool.empty()) return QueryResultPtr();
	// 借连接 → 查询(结果已被 store_result 缓存到客户端) → 立即归还。
	auto conn = acquire();
	QueryResultPtr r = conn->Query(sql);
	release(conn);
	return r;
}

bool MySqlManager::Execute(const string& sql)
{
	if (m_pool.empty()) return false;
	auto conn = acquire();
	bool ok = conn->Execute(sql);
	release(conn);
	return ok;
}

bool MySqlManager::CheckDatabase()
{
	if(m_mysql == NULL) return false;
	QueryResultPtr result = m_mysql->Query("show databases;");
	if (result == NULL)
	{
		cout << "no database found in mysql!\r\n";
		return false;
	}
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	Field* pRow = result->Fetch();
	string dbname = m_mysql->GetDBName();
	while (pRow)
	{
		string name = pRow[0].GetString();
		if (name == dbname)//如果查询结果中的数据库名称与当前连接的数据库名称匹配，说明数据库存在，返回true
		{
			cout << __FILE__ << "(" << __LINE__ << ")\r\n";
			result->EndQuery();//结束查询，释放资源
			return true;
		}
		if (result->NextRow() == false) break;//如果没有更多数据可供获取，跳出循环
		pRow = result->Fetch();//继续获取下一行数据，直到没有更多数据可供获取
	}
	cout << "database not found!\r\n";
	result->EndQuery();//结束查询，释放资源
	return false;//如果查询结果中没有找到匹配的数据库名称，说明数据库不存在，返回false
}

bool MySqlManager::CreateDatabase()
{
	if(m_mysql == NULL) return false;
	stringstream sql;
	sql << "create database " << m_mysql->GetDBName() << ";";
	uint32_t naffect = 0;
	int nErrno = 0;
	if(m_mysql->Execute(sql.str(), naffect, nErrno) == false)
	{
		cout << "create database failed!\r\n";
		return false;
	}
	if (naffect == 1) return true;
	return false;
}

bool MySqlManager::CheckTable(const sTableInfo& info)
{
	if (m_mysql == NULL) return false;
	stringstream sql;//构造SQL查询语句，检查表是否存在
	sql << "desc " << info.sName << ";";
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	QueryResultPtr result = m_mysql->Query(sql.str());
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	if (result == NULL)
	{
		if(CreateTable(info) == false)
		{
			cout << "create table " << info.sName << " failed!\r\n";
			return false;
		}
		return true;
	}
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	map<string, sFieldInfo> rest;
	rest.insert(info.mapField.begin(), info.mapField.end());//将表结构信息中的字段信息插入到rest映射表中，方便后续检查字段信息的匹配情况
	map<string, sFieldInfo> mapChange;
	
	Field* pRow = result->Fetch();//获取查询结果的第一行数据，返回一个指向Field对象的指针，表示表结构信息的第一行数据
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	while(pRow != NULL)
	{
		string name = pRow[0].GetString();
		string type = pRow[1].GetString();//获取字段名称和类型信息，分别保存在name和type字符串中
		FieldConstIter iter = info.mapField.find(name);
		cout << __FILE__ << "(" << __LINE__ << "): " << name << " type "  << type << "\r\n";
		if (iter == info.mapField.end())
		{
			if (result->NextRow() == false) break;
			pRow = result->Fetch();
			continue;
		}
		cout << __FILE__ << "(" << __LINE__ << "): " << iter->second.sType<< "\r\n";
		rest.erase(name);//如果表结构信息中找到了匹配的字段名称，说明该字段存在，从rest映射表中删除该字段信息，表示该字段已经匹配成功
		if (iter->second.sType != type)
		{
			mapChange.insert(FieldPair(name, iter->second));//如果表结构信息中的字段类型与查询结果中的字段类型不匹配，说明该字段需要更新，将字段名称和字段信息插入到mapChange映射表中，方便后续更新字段
		}
		if (result->NextRow() == false) break;//如果没有更多数据可供获取，跳出循环
		pRow = result->Fetch();//继续获取下一行数据，直到没有更多数据可供获取
		cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	}
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	result->EndQuery();
	if (rest.size() > 0)
	{//补全缺掉的列
		FieldIter it = rest.begin();
		for (; it != rest.end(); it++)
		{
			stringstream ss;
			ss << "alter table " << info.sName << 
				" add column " << it->second.sName << 
				" " << it->second.sType << ";";//构造SQL语句，添加缺失的字段信息
			if (m_mysql->Execute(ss.str()) == false)
			{
				return false;
			}

		}
	}
	if (mapChange.size() > 0)
	{//修改不匹配的列
		FieldIter iter = mapChange.begin();
		for (; iter != mapChange.end(); iter++)
		{
			stringstream ss;
			ss << "alter table " << info.sName <<
				" modify column " << iter->second.sName <<
				" " << iter->second.sType << ";";
			if(m_mysql->Execute(ss.str()) == false)
			{
				return false;
			}
		}
	}
	return true;
}



bool MySqlManager::CreateTable(const sTableInfo& info)
{
	if (m_mysql == NULL) return false;
	if (info.mapField.size() == 0) return false;
	stringstream sql;
	sql << "create table if not exists " << info.sName << "(";
	FieldConstIter it = info.mapField.begin();
	for (; it != info.mapField.end(); it++)
	{
		if (it != info.mapField.begin())
		{
			sql << ",";
		}
		sFieldInfo field = it->second;
		sql << field.sName << " " << field.sType;
	}
	if (info.sKey.size() > 0)
	{
		sql << "," << info.sKey;
	}
	sql << ") default charset=utf8mb4,ENGINE=InnoDB;";
	return m_mysql->Execute(sql.str());
}

bool MySqlManager::UpdateTable(const sTableInfo& info)
{
	if (CheckTable(info) == false)
	{
		return CreateTable(info);
	}
	return true;
}
