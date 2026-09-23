// Seeds the RocksDB store with the shared dataset.
//
//   seed <path> <rows>
#include <cstdio>
#include <cstdlib>
#include <filesystem>

#include <rocksdb/db.h>
#include <rocksdb/write_batch.h>

#include "common.hpp"

int main(int argc, char** argv) {
    if (argc < 3) {
        std::fprintf(stderr, "usage: seed <path> <rows>\n");
        return 2;
    }
    const std::string path = argv[1];
    const std::uint64_t rows = std::strtoull(argv[2], nullptr, 10);
    std::filesystem::remove_all(path);

    rocksdb::Options options;
    options.create_if_missing = true;
    std::unique_ptr<rocksdb::DB> db;
    auto status = rocksdb::DB::Open(options, path, &db);
    if (!status.ok()) {
        std::fprintf(stderr, "open %s: %s\n", path.c_str(), status.ToString().c_str());
        return 1;
    }
    rocksdb::WriteBatch batch;
    for (std::uint64_t id = 1; id <= rows; ++id) {
        batch.Put(bench::key(id), bench::row(id));
    }
    status = db->Write(rocksdb::WriteOptions(), &batch);
    if (!status.ok()) {
        std::fprintf(stderr, "write: %s\n", status.ToString().c_str());
        return 1;
    }
    // Readers open the store read-only, so the rows are flushed and compacted
    // into table files rather than left in the write-ahead log.
    db->Flush(rocksdb::FlushOptions());
    db->CompactRange(rocksdb::CompactRangeOptions(), nullptr, nullptr);
    std::printf("seeded %llu rows into %s\n", static_cast<unsigned long long>(rows), path.c_str());
    return 0;
}
