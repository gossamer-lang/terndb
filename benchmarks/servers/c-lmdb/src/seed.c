/* Seeds the LMDB store with the shared dataset.
 *
 *   seed <path> <rows> */
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

#include <lmdb.h>

#include "common.h"

static int fail(const char* what, int rc) {
    fprintf(stderr, "%s: %s\n", what, mdb_strerror(rc));
    return 1;
}

int main(int argc, char** argv) {
    if (argc < 3) {
        fprintf(stderr, "usage: seed <path> <rows>\n");
        return 2;
    }
    const char* path = argv[1];
    const uint64_t rows = strtoull(argv[2], NULL, 10);

    /* The environment is a directory holding data.mdb and lock.mdb; a reseed
     * starts from empty files. */
    mkdir(path, 0755);
    char file[4096];
    snprintf(file, sizeof file, "%s/data.mdb", path);
    unlink(file);
    snprintf(file, sizeof file, "%s/lock.mdb", path);
    unlink(file);

    MDB_env* env;
    int rc = mdb_env_create(&env);
    if (rc) return fail("env create", rc);
    mdb_env_set_mapsize(env, BENCH_MAP_SIZE);
    if ((rc = mdb_env_open(env, path, 0, 0644))) return fail("env open", rc);
    MDB_txn* txn;
    if ((rc = mdb_txn_begin(env, NULL, 0, &txn))) return fail("begin", rc);
    MDB_dbi dbi;
    if ((rc = mdb_dbi_open(txn, NULL, 0, &dbi))) return fail("dbi open", rc);
    unsigned char key_bytes[8];
    char row[256];
    for (uint64_t id = 1; id <= rows; ++id) {
        bench_key(id, key_bytes);
        int n = bench_row(id, row, sizeof row);
        MDB_val key = {8, key_bytes};
        MDB_val val = {(size_t)n, row};
        /* Ids arrive in key order, so each row is appended to the last page. */
        if ((rc = mdb_put(txn, dbi, &key, &val, MDB_APPEND))) return fail("put", rc);
    }
    if ((rc = mdb_txn_commit(txn))) return fail("commit", rc);
    mdb_env_close(env);
    printf("seeded %llu rows into %s\n", (unsigned long long)rows, path);
    return 0;
}
