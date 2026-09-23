/* CivetWeb + LMDB, an embedded memory-mapped B+tree store.
 *
 * The row is stored as its finished JSON under its integer id, as rust-redb
 * and cpp-rocksdb store it, and the store is opened read-only, as the terndb
 * servers open theirs. */
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <civetweb.h>
#include <lmdb.h>

#include "common.h"

static MDB_env* env;
static MDB_dbi dbi;

static const char NOT_FOUND[] = "{\"error\":\"not found\"}";

/* The status line, headers, and body leave in one write. */
static int send_json(struct mg_connection* conn, int status, const char* body, size_t len) {
    char out[512];
    const char* reason = status == 200 ? "OK" : status == 404 ? "Not Found" : "Internal Server Error";
    int head = snprintf(out, sizeof out,
                        "HTTP/1.1 %d %s\r\nContent-Type: application/json\r\n"
                        "Content-Length: %zu\r\n\r\n",
                        status, reason, len);
    if (head > 0 && (size_t)head + len <= sizeof out) {
        memcpy(out + head, body, len);
        mg_write(conn, out, (size_t)head + len);
    } else {
        mg_write(conn, out, (size_t)head);
        mg_write(conn, body, len);
    }
    return status;
}

static int health(struct mg_connection* conn, void* unused) {
    (void)unused;
    static const char ok[] = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nok";
    mg_write(conn, ok, sizeof ok - 1);
    return 200;
}

static int user(struct mg_connection* conn, void* unused) {
    (void)unused;
    const struct mg_request_info* info = mg_get_request_info(conn);
    const char* tail = info->local_uri + strlen("/user/");
    char* end;
    const uint64_t id = strtoull(tail, &end, 10);
    if (end == tail || *end != '\0') return send_json(conn, 404, NOT_FOUND, sizeof NOT_FOUND - 1);

    /* A read transaction per request, as redb takes a snapshot per request. */
    MDB_txn* txn;
    if (mdb_txn_begin(env, NULL, MDB_RDONLY, &txn)) {
        static const char failed[] = "{\"error\":\"read failed\"}";
        return send_json(conn, 500, failed, sizeof failed - 1);
    }
    unsigned char key_bytes[8];
    bench_key(id, key_bytes);
    MDB_val key = {8, key_bytes};
    MDB_val val;
    int status;
    if (mdb_get(txn, dbi, &key, &val) == 0) {
        status = send_json(conn, 200, val.mv_data, val.mv_size);
    } else {
        status = send_json(conn, 404, NOT_FOUND, sizeof NOT_FOUND - 1);
    }
    mdb_txn_abort(txn);
    return status;
}

int main(void) {
    const char* addr = getenv("ADDR") ? getenv("ADDR") : "127.0.0.1:8091";
    const char* path = getenv("DB_PATH") ? getenv("DB_PATH") : "bench.lmdb";

    int rc = mdb_env_create(&env);
    if (rc == 0) mdb_env_set_mapsize(env, BENCH_MAP_SIZE);
    /* Read transactions are not tied to the thread that opened them, so a
     * worker may serve any connection. */
    if (rc == 0) mdb_env_set_maxreaders(env, 1024);
    if (rc == 0) rc = mdb_env_open(env, path, MDB_RDONLY | MDB_NOTLS, 0644);
    MDB_txn* txn = NULL;
    if (rc == 0) rc = mdb_txn_begin(env, NULL, MDB_RDONLY, &txn);
    if (rc == 0) rc = mdb_dbi_open(txn, NULL, 0, &dbi);
    if (rc == 0) rc = mdb_txn_commit(txn);
    if (rc) {
        fprintf(stderr, "open %s: %s\n", path, mdb_strerror(rc));
        return 1;
    }

    /* One worker per hardware thread; a connection holds its worker while it
     * is kept alive. */
    long cpus = sysconf(_SC_NPROCESSORS_ONLN);
    char threads[24];
    snprintf(threads, sizeof threads, "%ld", cpus > 8 ? cpus : 8);
    /* A reply that spans two writes would otherwise wait on the peer's
     * delayed acknowledgement of the first. */
    const char* options[] = {"listening_ports", addr, "num_threads", threads,
                             "enable_keep_alive", "yes", "tcp_nodelay", "1", NULL};
    mg_init_library(0);
    struct mg_callbacks callbacks;
    memset(&callbacks, 0, sizeof callbacks);
    struct mg_context* ctx = mg_start(&callbacks, NULL, options);
    if (!ctx) {
        fprintf(stderr, "listen on %s failed\n", addr);
        return 1;
    }
    mg_set_request_handler(ctx, "/health$", health, NULL);
    mg_set_request_handler(ctx, "/user/", user, NULL);
    printf("c-lmdb on %s, serving %s\n", addr, path);
    fflush(stdout);
    for (;;) pause();
}
