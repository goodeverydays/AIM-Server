#include "mysqltool.h"

MySQLTool::MySQLTool()
{
	m_mysql = NULL;
	m_port = 3306;
}

MySQLTool::~MySQLTool()
{
	if (m_mysql != NULL)
	{
		MYSQL* tmp = m_mysql;
		m_mysql = NULL;
		mysql_close(tmp);
	}
}

bool MySQLTool::connect(const string& host, const string& user, const string& passwd, const string& db, unsigned port)
{
	if (m_mysql != NULL)
	{
		MYSQL* tmp = m_mysql;
		m_mysql = NULL;
		mysql_close(tmp);
	}

	m_mysql = mysql_init(m_mysql);
	//mysql 默认端口是3306
	m_mysql = mysql_real_connect(m_mysql, host.c_str(), user.c_str(), passwd.c_str(), db.c_str(), port, NULL, 0);
	cout << host << endl << user << endl << passwd << endl << db << endl;
	if (m_mysql != NULL)
	{
		m_host = host;
		m_user = user;
		m_password = passwd;
		m_dbname = db;
		mysql_query(m_mysql, "set names utf8;");
		cout << "connect mysql success!\r\n";
		return true;
	}
	cout << "connect mysql failed!\r\n";
	return false;
}

QueryResultPtr MySQLTool::Query(const string& sql)
{
	if (m_mysql == NULL)
	{
		if (connect(m_host, m_user, m_password, m_dbname) == false)
		{
			return QueryResultPtr();
		}
	}
	int ret = mysql_real_query(m_mysql, sql.c_str(), sql.size());
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	if (ret)//如果执行SQL查询语句失败，返回一个空的智能指针，表示查询失败
	{
		uint32_t nErrno = mysql_errno(m_mysql);
		cout << "mysql_real_query call failed! code is " << nErrno << endl;
		if (CR_SERVER_GONE_ERROR == nErrno)
		{
			cout << __FILE__ << "(" << __LINE__ << ")\r\n";
			if (connect(m_host, m_user, m_password, m_dbname) == false)
			{
				cout << __FILE__ << "(" << __LINE__ << ")\r\n";
				return QueryResultPtr();
			}
			ret = mysql_real_query(m_mysql, sql.c_str(), sql.size());
			if (ret)
			{
				cout << __FILE__ << "(" << __LINE__ << ")\r\n";
				nErrno = mysql_errno(m_mysql);
				cout << "mysql_real_query call failed again code is : " << nErrno << endl;
				return QueryResultPtr();
			}
		}
		else {
			cout << __FILE__ << "(" << __LINE__ << ")\r\n";
			return QueryResultPtr();
		}
	}
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	MYSQL_RES* result = mysql_store_result(m_mysql);//获取查询结果对象，包含查询结果的元数据和数据，如果查询失败或没有结果集，则返回NULL
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	uint32_t rowcount = mysql_affected_rows(m_mysql);//获取查询结果的行数，表示查询结果中包含多少行数据，如果查询失败或没有结果集，则返回0
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	uint32_t cloumncount = mysql_field_count(m_mysql);//获取查询结果的列数，表示查询结果中包含多少列数据，如果查询失败或没有结果集，则返回0
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	return QueryResultPtr (new QueryResult(result, rowcount, cloumncount));
	//创建一个QueryResult对象，并将其封装在智能指针QueryResultPtr中返回，方便管理查询结果的生命周期，确保在程序结束时自动释放资源，避免内存泄漏
}

bool MySQLTool::Execute(const string& sql)
{
	uint32_t nAffectedCount;
	int nErrno;
	return Execute(sql, nAffectedCount, nErrno);
}

bool MySQLTool::Execute(const string& sql, uint32_t& nAffectedCount, int& nErrno)
{
	if (m_mysql == NULL)
	{
		if (connect(m_host, m_user, m_password, m_dbname) == false)
		{
			return false;
		}
	}
	//blob real_query 遇到\0 不会认为是字符串结束
	int ret = mysql_query(m_mysql, sql.c_str());
	if (ret)
	{
		nErrno = mysql_errno(m_mysql);
		cout << "mysql_query call failed! code is " << nErrno << endl;
		cout << "mysql_query call failed! msg : " << mysql_error(m_mysql) << endl;
		if (CR_SERVER_GONE_ERROR == nErrno)
		{
			if (connect(m_host, m_user, m_password, m_dbname) == false)
			{
				return false;
			}
			ret = mysql_query(m_mysql, sql.c_str());
			if (ret)
			{
				nErrno = mysql_errno(m_mysql);
				cout << "mysql_query call failed again code is : " << nErrno << endl;
				return false;
			}
		}
		else {
			return false;
		}
	}
	
	nErrno = 0;
	nAffectedCount = mysql_affected_rows(m_mysql);
	return true;
}

