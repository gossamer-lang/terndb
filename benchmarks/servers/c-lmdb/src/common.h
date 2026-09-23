/* What the three programs share: the key encoding, the stored row, and the id
 * sequence every benchmark target walks. */
#ifndef BENCH_COMMON_H
#define BENCH_COMMON_H

#include <stdint.h>
#include <stdio.h>

/* Big-endian, so the store's byte order is the ids' numeric order. */
static inline void bench_key(uint64_t id, unsigned char out[8]) {
    for (int i = 7; i >= 0; --i) {
        out[i] = (unsigned char)(id & 0xff);
        id >>= 8;
    }
}

/* The row as its finished JSON, the same shape rust-redb and cpp-rocksdb
 * store. Answers the length written. */
static inline int bench_row(uint64_t id, char* out, size_t cap) {
    unsigned long long n = (unsigned long long)id;
    return snprintf(out, cap,
                    "{\"id\":%llu,\"name\":\"user-%llu\",\"email\":\"user-%llu@example.com\","
                    "\"score\":1.5,\"active\":true}",
                    n, n, n);
}

/* Every target walks the same id sequence, so each reads the same rows in the
 * same order. */
#define BENCH_STRIDE 2654435761LL
#define BENCH_BATCH 1024LL

static inline uint64_t bench_id_at(int64_t i, int64_t rows) {
    return (uint64_t)((int64_t)((uint64_t)i * (uint64_t)BENCH_STRIDE) % rows + 1);
}

/* Room for the store: far beyond the rows the benchmarks seed, and only
 * address space until pages are written. */
#define BENCH_MAP_SIZE (1ULL << 32)

#endif
