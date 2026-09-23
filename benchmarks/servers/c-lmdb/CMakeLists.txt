# C + LMDB, an embedded memory-mapped B+tree store, behind CivetWeb.
#
# Both are fetched at pinned tags and compiled with the project, so the
# benchmark needs a C compiler and CMake and nothing installed system-wide.
cmake_minimum_required(VERSION 3.24)
project(c_lmdb C)

set(CMAKE_C_STANDARD 11)
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

include(FetchContent)
FetchContent_Declare(lmdb
  GIT_REPOSITORY https://github.com/LMDB/lmdb.git
  GIT_TAG LMDB_1.0.1
  GIT_SHALLOW TRUE)
FetchContent_Declare(civetweb
  GIT_REPOSITORY https://github.com/civetweb/civetweb.git
  GIT_TAG v1.16
  GIT_SHALLOW TRUE)
FetchContent_GetProperties(lmdb)
if(NOT lmdb_POPULATED)
  FetchContent_Populate(lmdb)
endif()
FetchContent_GetProperties(civetweb)
if(NOT civetweb_POPULATED)
  FetchContent_Populate(civetweb)
endif()

find_package(Threads REQUIRED)

set(LMDB_DIR ${lmdb_SOURCE_DIR}/libraries/liblmdb)
add_library(lmdb STATIC ${LMDB_DIR}/mdb.c ${LMDB_DIR}/midl.c)
target_include_directories(lmdb PUBLIC ${LMDB_DIR})
target_link_libraries(lmdb PUBLIC Threads::Threads)

# The server serves two routes and nothing else: no TLS, CGI, or file serving.
add_library(civetweb STATIC ${civetweb_SOURCE_DIR}/src/civetweb.c)
target_include_directories(civetweb PUBLIC ${civetweb_SOURCE_DIR}/include)
target_compile_definitions(civetweb PRIVATE NO_SSL NO_CGI NO_FILES NO_CACHING)
target_link_libraries(civetweb PUBLIC Threads::Threads ${CMAKE_DL_LIBS})

foreach(bin server seed embedded)
  add_executable(${bin} src/${bin}.c)
  target_link_libraries(${bin} PRIVATE lmdb)
endforeach()
target_link_libraries(server PRIVATE civetweb)
