#include "Field.h"

Field::Field() : m_type(TYPE_NONE), m_isnull(true) {}

//Field::~Field(){}

void Field::SetValue(const char* value, size_t nLen)
{
	if (value == NULL || (nLen == 0))
	{
		m_isnull = true;
		m_value.clear();
		return;
	}
	/*_value.resize(nLen);
	memcpy((char*)m_value.c_str(), value, nLen);*/
	m_value.assign(value, nLen);//将字段值保存到m_value字符串中，使用assign函数将value指针指向的字符串内容复制到m_value中，长度为nLen
	m_isnull = false;
}