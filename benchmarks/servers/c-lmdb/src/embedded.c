/* LMDB queried in process, with nothing above it.
 *
 * One thread takes a read transaction per query and copies the row's stored
 * JSON into a buffer of its own, which is what the HTTP server does per
 * request - so the two numbers describe the same read path with and without a
 * web tier above it.
 *
 *   embedded <path> <rows> <seconds> */
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include <lmdb.h>

#include "common.h"

static MDB_env* env;
static MDB_dbi dbi;

/* A read transaction per query, as redb takes a snapshot per query: holding
 * one open across the run would answer from a fixed view. The caller receives
 * an owned copy of the row, as it does from every other target. */
static size_t read_row(uint64_t id) {
    MDB_txn* txn;
    if (mdb_txn_begin(env, NULL, MDB_RDONLY, &txn)) return 0;
    unsigned char key_bytes[8];
    bench_key(id, key_bytes);
    MDB_val key = {8, key_bytes};
    MDB_val val;
    size_t n = 0;
    if (mdb_get(txn, dbi, &key, &val) == 0) {
        char* owned = malloc(val.mv_size);
        if (owned) {
            memcpy(owned, val.mv_data, val.mv_size);
            n = val.mv_size;
            free(owned);
        }
    }
    mdb_txn_abort(txn);
    return n;
}

static double now_seconds(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (double)ts.tv_sec + (double)ts.tv_nsec * 1e-9;
}

int main(int argc, char** argv) {
    if (argc < 4) {
        fprintf(stderr, "usage: embedded <path> <rows> <seconds>\n");
        return 2;
    }
    const char* path = argv[1];
    const int64_t rows = strtoll(argv[2], NULL, 10);
    const double seconds = strtod(argv[3], NULL);

    int rc = mdb_env_create(&env);
    if (rc == 0) mdb_env_set_mapsize(env, BENCH_MAP_SIZE);
    if (rc == 0) rc = mdb_env_open(env, path, MDB_RDONLY, 0644);
    MDB_txn* txn = NULL;
    if (rc == 0) rc = mdb_txn_begin(env, NULL, MDB_RDONLY, &txn);
    if (rc == 0) rc = mdb_dbi_open(txn, NULL, 0, &dbi);
    if (rc == 0) rc = mdb_txn_commit(txn);
    if (rc) {
        fprintf(stderr, "open %s: %s\n", path, mdb_strerror(rc));
        return 1;
    }

    if (read_row(1) == 0) {
        fprintf(stderr, "row 1 is missing from %s\n", path);
        return 1;
    }
    for (int64_t i = 0; i < BENCH_BATCH * 8; ++i) read_row(bench_id_at(i, rows));

    const double start = now_seconds();
    int64_t queries = 0;
    unsigned long long bytes = 0;
    for (;;) {
        for (int64_t k = 0; k < BENCH_BATCH; ++k) bytes += read_row(bench_id_at(queries + k, rows));
        queries += BENCH_BATCH;
        if (now_seconds() - start >= seconds) break;
    }
    const double secs = now_seconds() - start;
    printf("{\"target\":\"c-lmdb\",\"queries\":%lld,\"seconds\":%.3f,\"qps\":%.1f,"
           "\"ns_per_query\":%.1f,\"bytes\":%llu}\n",
           (long long)queries, secs, (double)queries / secs, secs * 1e9 / (double)queries, bytes);
    mdb_env_close(env);
    return 0;
}
