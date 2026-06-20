#include <unistd.h>
#include <mysql/mysql.h>
#include <stdio.h>
#include <string>
#include <sys/types.h>
#include <errno.h>
#include <memory>
#include <vector>
#include "base/Logging.h"
#include "Field.h"

using namespace std;

class QueryResult
{
public:
	QueryResult(MYSQL_RES* result, uint32_t rowcount, uint32_t cloumncount);
	~QueryResult();
	bool NextRow();//获取下一行数据，返回true表示还有下一行，并且切换成功，false表示没有更多数据可供获取
	Field* Fetch()
	{
		// 关键修复：结果集为 0 行（或已遍历完）时 m_CurrentRow 被 EndQuery() 清空，
		// 此时 data() 在 libstdc++ 下仍返回非空指针（clear 不释放缓冲区），
		// 会让调用方的 "if(row==NULL) break" 失效，从而读到一条全 0 的"幻影行"。
		// 必须在无行时显式返回 nullptr。
		return m_CurrentRow.empty() ? nullptr : m_CurrentRow.data();
	}
	const Field& operator[](int index) const
	{
		return m_CurrentRow[index];
	}
	const Field& operator[](const string& name) const
	{
		return m_CurrentRow[GetFieldIndexByname(name)];//通过字段名称获取字段值，首先调用GetFieldIndexByname函数获取字段名称对应的索引，然后返回对应索引位置的字段值
	}
	uint32_t GetFieldCount() const { return m_cloumncount; }
	uint32_t GetRowCount() const { return m_rowcount; }

	vector<string> const& GetFieldNames() const { return m_vecFieldName; }
	void EndQuery();
	Field::DataTypes toType(enum_field_types mysqlType) const;

protected:
	int GetFieldIndexByname(const string& name) const
	{
		for (uint32_t i = 0; i < m_vecFieldName.size(); i++)
		{
			if (m_vecFieldName[i] == name) return i;
		}		
		return -1;
	}

private:
	vector<Field> m_CurrentRow;//查询结果的字段列表，包含每个字段的名称、类型等信息
	vector<string> m_vecFieldName;//查询结果的字段名称列表，方便通过字段名称获取字段值
	MYSQL_RES* m_result;//MySQL查询结果对象，包含查询结果的元数据和数据
	uint32_t m_rowcount;//查询结果的行数，表示查询结果中包含多少行数据
	uint32_t m_cloumncount;//查询结果的列数，表示查询结果中包含多少列数据
};

typedef shared_ptr<QueryResult> QueryResultPtr;//定义一个智能指针类型，方便管理QueryResult对象的生命周期