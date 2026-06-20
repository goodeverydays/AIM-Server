#include <unistd.h>
#include <mysql/mysql.h>
#include <stdio.h>
#include <string>
#include <sys/types.h>
#include <errno.h>
#include <memory>
#include "base/Logging.h"

using namespace std;

class Field
{
public:
	typedef enum
	{
		TYPE_NONE = 0,
		TYPE_STRING,
		TYPE_INTEGER,
		TYPE_FLOAT,
		TYPE_BOOL,
		TYPE_NULL
	}DataTypes;
public:
	Field();
	~Field() = default;
	void SetType(DataTypes tp) { m_type = tp; }
	DataTypes GetType() const { return m_type; }//获取字段类型，返回一个DataTypes枚举值，表示字段的类型，例如字符串、整数、浮点数等
	void SetName(const string& name) { m_name = name; }
	const string& GetName() const { return m_name; }//获取字段名称，返回一个字符串，表示字段的名称，例如列名、别名等 
	void SetValue(const char* value, size_t nLen);
	const string& GetValue() const {return m_value; }//获取字段值，返回一个字符串，表示字段的值，例如查询结果中的数据值等
	bool isNull()const { return m_isnull; }//判断字段是否为NULL，返回一个布尔值，表示字段的值是否为NULL，例如查询结果中的数据值是否为NULL等
	bool toBool() const{return atoi(m_value.c_str()) != 0;}//将字段值转换为布尔类型，返回一个布尔值，表示字段的值是否为真，例如查询结果中的数据值是否为真等
	int8_t toInt8() const {return static_cast<int8_t>(atoi(m_value.c_str())); }
	uint8_t toUint8() const {return static_cast<uint8_t>(atoi(m_value.c_str())); }
	int32_t toInt32() const {return atoi(m_value.c_str()); }
	uint32_t toUint32() const {return static_cast<uint32_t>(atoi(m_value.c_str())); }
	int64_t toInt64() const {return atoll(m_value.c_str()); }
	uint64_t toUint64() const {return static_cast<uint64_t>(atoll(m_value.c_str())); }
	double toFloat() const {return atof(m_value.c_str()); }
	const string& GetString() const { return m_value; }//获取字段值的字符串表示，返回一个字符串，表示字段的值的字符串形式，例如查询结果中的数据值的字符串形式等

private:
	string m_value;//字段值,保存查询结果中每个字段的值信息
	string m_name;
	DataTypes m_type;//字段类型，保存查询结果中每个字段的类型信息
	
	bool m_isnull;//字段是否为NULL，表示查询结果中每个字段的值是否为NULL
};