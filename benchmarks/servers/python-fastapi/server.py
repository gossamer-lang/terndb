# /// script
# requires-python = ">=3.11"
# dependencies = ["fastapi", "uvicorn[standard]"]
# ///
"""FastAPI + sqlite3, one connection per thread.

The lookup is by INTEGER PRIMARY KEY, the same keyed read the other servers do.
"""

import os
import sqlite3
import threading

from fastapi import FastAPI
from fastapi.responses import JSONResponse, PlainTextResponse

DB_PATH = os.environ.get("DB_PATH", "bench.sqlite")
app = FastAPI()
_local = threading.local()


def conn() -> sqlite3.Connection:
    # sqlite3 connections are not shareable across threads, and the server runs
    # the sync endpoint on a worker pool, so each thread keeps its own.
    if not hasattr(_local, "conn"):
        c = sqlite3.connect(DB_PATH, check_same_thread=False)
        c.execute("PRAGMA journal_mode=WAL")
        c.execute("PRAGMA synchronous=NORMAL")
        _local.conn = c
    return _local.conn


@app.get("/health", response_class=PlainTextResponse)
def health() -> str:
    return "ok"


@app.get("/user/{user_id}")
def user(user_id: int) -> JSONResponse:
    row = conn().execute(
        "SELECT id, name, email, score, active FROM users WHERE id = ?", (user_id,)
    ).fetchone()
    if row is None:
        return JSONResponse({"error": "not found"}, status_code=404)
    return JSONResponse(
        {"id": row[0], "name": row[1], "email": row[2], "score": row[3], "active": bool(row[4])}
    )
