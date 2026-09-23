# C++ + RocksDB, an embedded LSM key-value store, behind cpp-httplib.
#
# Both dependencies are fetched at pinned tags and built with the project, so
# the benchmark needs a C++ compiler and CMake and nothing installed system-wide.
cmake_minimum_required(VERSION 3.24)
project(cpp_rocksdb CXX C)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

include(FetchContent)

set(WITH_GFLAGS OFF CACHE BOOL "" FORCE)
set(WITH_TESTS OFF CACHE BOOL "" FORCE)
set(WITH_BENCHMARK_TOOLS OFF CACHE BOOL "" FORCE)
set(WITH_TOOLS OFF CACHE BOOL "" FORCE)
set(WITH_CORE_TOOLS OFF CACHE BOOL "" FORCE)
set(WITH_TRACE_TOOLS OFF CACHE BOOL "" FORCE)
set(WITH_EXAMPLES OFF CACHE BOOL "" FORCE)
set(ROCKSDB_BUILD_SHARED OFF CACHE BOOL "" FORCE)
set(FAIL_ON_WARNINGS OFF CACHE BOOL "" FORCE)
set(USE_RTTI ON CACHE BOOL "" FORCE)
FetchContent_Declare(rocksdb
  GIT_REPOSITORY https://github.com/facebook/rocksdb.git
  GIT_TAG v11.8.1
  GIT_SHALLOW TRUE)

set(HTTPLIB_COMPILE OFF CACHE BOOL "" FORCE)
set(HTTPLIB_USE_OPENSSL_IF_AVAILABLE OFF CACHE BOOL "" FORCE)
set(HTTPLIB_USE_ZLIB_IF_AVAILABLE OFF CACHE BOOL "" FORCE)
set(HTTPLIB_USE_BROTLI_IF_AVAILABLE OFF CACHE BOOL "" FORCE)
set(HTTPLIB_USE_ZSTD_IF_AVAILABLE OFF CACHE BOOL "" FORCE)
FetchContent_Declare(httplib
  GIT_REPOSITORY https://github.com/yhirose/cpp-httplib.git
  GIT_TAG v0.58.0
  GIT_SHALLOW TRUE)

FetchContent_MakeAvailable(rocksdb httplib)

foreach(bin server seed embedded)
  add_executable(${bin} src/${bin}.cpp)
  target_include_directories(${bin} PRIVATE ${rocksdb_SOURCE_DIR}/include)
  target_link_libraries(${bin} PRIVATE rocksdb)
endforeach()
target_link_libraries(server PRIVATE httplib::httplib)
