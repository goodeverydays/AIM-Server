# CMakeLists.txt: jsoncpp 静态库构建
cmake_minimum_required(VERSION 3.8)

project(jsoncpp)

# jsoncpp 公共头文件
set(JSONCPP_PUBLIC_HEADERS
    include/json/config.h
    include/json/forwards.h
    include/json/json_features.h
    include/json/value.h
    include/json/reader.h
    include/json/version.h
    include/json/writer.h
    include/json/assertions.h
    include/json/allocator.h
    include/json/json.h
)

# jsoncpp 源文件
set(JSONCPP_SOURCES
    src/json_tool.h
    src/json_reader.cpp
    src/json_valueiterator.inl
    src/json_value.cpp
    src/json_writer.cpp
)

# 构建静态库
add_library(jsoncpp STATIC ${JSONCPP_PUBLIC_HEADERS} ${JSONCPP_SOURCES})

# 设置包含目录：使用者可以找到 json/*.h
target_include_directories(jsoncpp PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
)

# 要求 C++11 支持
target_compile_features(jsoncpp PUBLIC cxx_std_11)

# 禁用警告，避免第三方库警告污染项目输出
if(MSVC)
    target_compile_options(jsoncpp PRIVATE /W0)
else()
    target_compile_options(jsoncpp PRIVATE -w)
endif()
