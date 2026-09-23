// cpp-httplib + RocksDB, an embedded LSM key-value store.
//
// The row is stored as its finished JSON under its integer id, as rust-redb
// stores it, and the store is opened read-only, as the terndb servers open
// theirs.
#include <cstdio>
#include <cstdlib>
#include <memory>
#include <string>
#include <thread>

#include <httplib.h>
#include <rocksdb/db.h>

#include "common.hpp"

int main() {
    const char* addr_env = std::getenv("ADDR");
    const std::string addr = addr_env ? addr_env : "127.0.0.1:8085";
    const char* path_env = std::getenv("DB_PATH");
    const std::string path = path_env ? path_env : "bench.rocksdb";

    rocksdb::Options options;
    std::unique_ptr<rocksdb::DB> db;
    auto status = rocksdb::DB::OpenForReadOnly(options, path, &db);
    if (!status.ok()) {
        std::fprintf(stderr, "open %s: %s\n", path.c_str(), status.ToString().c_str());
        return 1;
    }

    httplib::Server server;
    // One worker per hardware thread; a connection holds its worker while it
    // is kept alive.
    const unsigned threads = std::max(8u, std::thread::hardware_concurrency());
    server.new_task_queue = [threads] { return new httplib::ThreadPool(threads); };
    server.Get("/health", [](const httplib::Request&, httplib::Response& res) {
        res.set_content("ok", "text/plain");
    });
    server.Get(R"(/user/(\d+))", [&db](const httplib::Request& req, httplib::Response& res) {
        const std::uint64_t id = std::strtoull(req.matches[1].str().c_str(), nullptr, 10);
        std::string value;
        auto got = db->Get(rocksdb::ReadOptions(), bench::key(id), &value);
        if (got.ok()) {
            res.set_content(std::move(value), "application/json");
            return;
        }
        res.status = got.IsNotFound() ? 404 : 500;
        res.set_content(got.IsNotFound() ? R"({"error":"not found"})" : R"({"error":"read failed"})",
                        "application/json");
    });

    const auto colon = addr.rfind(':');
    const std::string host = addr.substr(0, colon);
    const int port = std::atoi(addr.c_str() + colon + 1);
    std::printf("cpp-rocksdb on %s, serving %s\n", addr.c_str(), path.c_str());
    std::fflush(stdout);
    return server.listen(host, port) ? 0 : 1;
}
