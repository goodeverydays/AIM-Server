#include "QueryResult.h"
#include <iostream>

QueryResult::QueryResult(MYSQL_RES* result, uint32_t rowcount, uint32_t cloumncount)
	:m_result(result), m_rowcount(rowcount), m_cloumncount(cloumncount)
{
	cout << __FILE__ << "(" << __LINE__ << "):" << rowcount << ", " << cloumncount << "\r\n";
	m_CurrentRow.resize(m_cloumncount);
	m_vecFieldName.resize(m_cloumncount);
	MYSQL_FIELD* fields = mysql_fetch_fields(m_result);//获取查询结果的字段信息，返回一个指向MYSQL_FIELD结构体数组的指针，数组中的每个元素包含一个字段的元数据信息，例如字段名称、类型等
	cout << __FILE__ << "(" << __LINE__ << "):" << fields << "\r\n";
	MYSQL_ROW row = mysql_fetch_row(m_result);//从查询结果中获取下一行数据，返回一个MYSQL_ROW类型的指针，如果没有更多数据可供获取，则返回NULL
	if (row == NULL)
	{
		EndQuery();//结束查询，释放资源
		return;
	}
	unsigned long* pFieldLength = mysql_fetch_lengths(m_result);//获取当前行中每个字段的长度信息，返回一个指向长度数组的指针，数组中的每个元素对应一个字段的长度

	for (uint32_t i = 0; i < m_cloumncount; i++)
	{
		//cout << __FILE__ << "(" << __LINE__ << "):" << i << "\r\n";
		m_vecFieldName[i] = fields[i].name;//将字段名称保存到字段名称列表中，方便通过字段名称获取字段值
		//cout << __FILE__ << "(" << __LINE__ << "):" << i << "\r\n";
		m_CurrentRow[i].SetType(toType(fields[i].type));//设置字段的类型，方便后续根据字段类型进行数据转换和处理
		//cout << __FILE__ << "(" << __LINE__ << ")\r\n";
		m_CurrentRow[i].SetName(m_vecFieldName[i]);//设置字段的名称，方便通过字段名称获取字段值
		//cout << __LINE__ << ")QueryResult: " << row[i] << "length: " << pFieldLength[i] <<  "\r\n";
		if (row[i] == NULL)
		{
			m_CurrentRow[i].SetValue(NULL, 0);
		}
		else {
			m_CurrentRow[i].SetValue(row[i], pFieldLength[i]);//将字段值保存到字段列表中，方便通过字段名称获取字段值
			//row[i]是一个字符串，表示查询结果中当前行的第i个字段的值，pFiledLength[i]是一个整数，表示该字段值的长度
			//SetValue函数将字段值和长度作为参数，保存到Field对象中，方便后续根据字段类型进行数据转换和处理
		}
	}
	//cout << __FILE__ << "(" << __LINE__ << ")"  << "\r\n";
}

QueryResult::~QueryResult()
{

}

bool QueryResult::NextRow()
{
	if(m_result == NULL) return false;
	MYSQL_ROW row = mysql_fetch_row(m_result);//从查询结果中获取下一行数据，返回一个MYSQL_ROW类型的指针，如果没有更多数据可供获取，则返回NULL
	if (row == NULL)
	{
		EndQuery();//结束查询，释放资源
		return false;
	}
	unsigned long* pFiledLength = mysql_fetch_lengths(m_result);//获取当前行中每个字段的长度信息，返回一个指向长度数组的指针，数组中的每个元素对应一个字段的长度
	for (uint32_t i = 0; i < m_cloumncount; i++)
	{
		if (row[i] == NULL)
		{
			m_CurrentRow[i].SetValue(NULL, 0);
		}
		else {
			m_CurrentRow[i].SetValue(row[i], pFiledLength[i]);//将字段值保存到字段列表中，方便通过字段名称获取字段值
			 //row[i]是一个字符串，表示查询结果中当前行的第i个字段的值，pFiledLength[i]是一个整数，表示该字段值的长度
			 //SetValue函数将字段值和长度作为参数，保存到Field对象中，方便后续根据字段类型进行数据转换和处理
		}
		
	}
	return true;
}

void QueryResult::EndQuery()
{
	m_CurrentRow.clear();
	m_vecFieldName.clear();
	if (m_result)
	{
		mysql_free_result(m_result);//释放查询结果对象，释放与查询结果相关的内存资源，避免内存泄漏
		m_result = NULL;
	}
	m_rowcount = 0;
	m_cloumncount = 0;
}

Field::DataTypes QueryResult::toType(enum_field_types mysqlType) const
{
	//TODO:根据MySQL字段类型，转换为Field的DataTypes枚举类型，方便后续根据字段类型进行数据转换和处理
	switch (mysqlType)
	{
	case FIELD_TYPE_TIMESTAMP:
	case FIELD_TYPE_DATE:
	case FIELD_TYPE_TIME:
	case FIELD_TYPE_DATETIME:
	case FIELD_TYPE_YEAR:
	case FIELD_TYPE_STRING:
	case FIELD_TYPE_VAR_STRING:
	case FIELD_TYPE_BLOB:
	case FIELD_TYPE_SET:
		return Field::TYPE_STRING;
	case FIELD_TYPE_NULL:
		return Field::TYPE_NULL;
	case FIELD_TYPE_TINY:
	case FIELD_TYPE_SHORT:
	case FIELD_TYPE_LONG:
	case FIELD_TYPE_LONGLONG:
	case FIELD_TYPE_INT24:
	case FIELD_TYPE_ENUM:
		return Field::TYPE_INTEGER;
	case FIELD_TYPE_FLOAT:
	case FIELD_TYPE_DOUBLE:
		return Field::TYPE_FLOAT;
	default:
		return Field::TYPE_NONE;
	}
	return Field::TYPE_NONE;
}