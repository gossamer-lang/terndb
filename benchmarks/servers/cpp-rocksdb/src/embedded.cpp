// RocksDB queried in process, with nothing above it.
//
// One thread looks a row up per query and receives its stored JSON as an owned
// string, which is what the HTTP server does per request - so the two numbers
// describe the same read path with and without a web tier above it.
//
//   embedded <path> <rows> <seconds>
#include <chrono>
#include <cstdio>
#include <cstdlib>
#include <memory>
#include <string>

#include <rocksdb/db.h>

#include "common.hpp"

int main(int argc, char** argv) {
    if (argc < 4) {
        std::fprintf(stderr, "usage: embedded <path> <rows> <seconds>\n");
        return 2;
    }
    const std::string path = argv[1];
    const std::int64_t rows = std::strtoll(argv[2], nullptr, 10);
    const double seconds = std::strtod(argv[3], nullptr);

    rocksdb::Options options;
    std::unique_ptr<rocksdb::DB> db;
    auto status = rocksdb::DB::OpenForReadOnly(options, path, &db);
    if (!status.ok()) {
        std::fprintf(stderr, "open %s: %s\n", path.c_str(), status.ToString().c_str());
        return 1;
    }
    const rocksdb::ReadOptions read_options;
    // The answer is an owned string, as every other target hands back.
    auto read = [&](std::uint64_t id) -> std::size_t {
        std::string value;
        return db->Get(read_options, bench::key(id), &value).ok() ? value.size() : 0;
    };

    if (read(1) == 0) {
        std::fprintf(stderr, "row 1 is missing from %s\n", path.c_str());
        return 1;
    }
    for (std::int64_t i = 0; i < bench::BATCH * 8; ++i) {
        read(bench::id_at(i, rows));
    }

    using clock = std::chrono::steady_clock;
    const auto start = clock::now();
    const auto window = std::chrono::duration<double>(seconds);
    std::int64_t queries = 0;
    std::uint64_t bytes = 0;
    for (;;) {
        for (std::int64_t k = 0; k < bench::BATCH; ++k) {
            bytes += read(bench::id_at(queries + k, rows));
        }
        queries += bench::BATCH;
        if (clock::now() - start >= window) {
            break;
        }
    }
    const double secs = std::chrono::duration<double>(clock::now() - start).count();
    std::printf(
        R"({"target":"cpp-rocksdb","queries":%lld,"seconds":%.3f,"qps":%.1f,"ns_per_query":%.1f,"bytes":%llu})"
        "\n",
        static_cast<long long>(queries), secs, queries / secs, secs * 1e9 / queries,
        static_cast<unsigned long long>(bytes));
    return 0;
}
