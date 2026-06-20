#include <unistd.h>
#include <mysql/mysql.h>
#include <mysql/errmsg.h>
#include <stdio.h>
#include <string>
#include <sys/types.h>
#include <errno.h>
#include "base/Logging.h"
#include "QueryResult.h"
#include <iostream>

using namespace std;

class MySQLTool
{
public:
	MySQLTool();
	~MySQLTool();
	//初始化MySQL连接，连接成功返回true，失败返回false
	bool connect(const string& host, const string& user, const string& password, const string& db, unsigned port = 3306);
	QueryResultPtr Query(const string& sql);//执行SQL查询语句，返回查询结果
	bool Execute(const string& sql);//执行SQL语句，返回执行结果
	bool Execute(const string& sql, uint32_t& nAffectedCount, int& nErrno);
	const string& GetDBName() const { return m_dbname; }
	//执行SQL查询语句，返回查询结果，并获取受影响的行数和错误码
private:
	MYSQL* m_mysql;//MySQL连接对象
	string m_host;//MySQL服务器地址
	string m_user;//MySQL用户名
	string m_password;//MySQL密码
	string m_dbname;//MySQL数据库名称
	unsigned m_port;
};