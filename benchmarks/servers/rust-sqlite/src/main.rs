//! axum + SQLite, one connection per blocking thread.
//!
//! The lookup is by INTEGER PRIMARY KEY, which is SQLite's rowid, so it is the
//! same shape of read the other servers do.

use axum::{extract::Path, http::StatusCode, response::IntoResponse, routing::get, Router};
use rusqlite::Connection;
use std::cell::RefCell;

thread_local! {
    static CONN: RefCell<Option<Connection>> = RefCell::new(None);
}

fn db_path() -> String {
    std::env::var("DB_PATH").unwrap_or_else(|_| "bench.sqlite".to_string())
}

fn with_conn<T>(f: impl FnOnce(&Connection) -> T) -> T {
    CONN.with(|cell| {
        let mut slot = cell.borrow_mut();
        if slot.is_none() {
            let conn = Connection::open(db_path()).expect("open sqlite");
            conn.pragma_update(None, "journal_mode", "WAL").ok();
            conn.pragma_update(None, "synchronous", "NORMAL").ok();
            *slot = Some(conn);
        }
        f(slot.as_ref().unwrap())
    })
}

async fn user(Path(id): Path<i64>) -> impl IntoResponse {
    let found = tokio::task::spawn_blocking(move || {
        with_conn(|conn| {
            let mut stmt = conn
                .prepare_cached("SELECT id, name, email, score, active FROM users WHERE id = ?1")
                .expect("prepare");
            stmt.query_row([id], |row| {
                Ok(serde_json::json!({
                    "id": row.get::<_, i64>(0)?,
                    "name": row.get::<_, String>(1)?,
                    "email": row.get::<_, String>(2)?,
                    "score": row.get::<_, f64>(3)?,
                    "active": row.get::<_, i64>(4)? != 0,
                })
                .to_string())
            })
            .ok()
        })
    })
    .await
    .expect("join");

    match found {
        Some(body) => ([("content-type", "application/json")], body).into_response(),
        None => (
            StatusCode::NOT_FOUND,
            [("content-type", "application/json")],
            r#"{"error":"not found"}"#,
        )
            .into_response(),
    }
}

#[tokio::main]
async fn main() {
    let addr = std::env::var("ADDR").unwrap_or_else(|_| "127.0.0.1:8081".to_string());
    let app = Router::new()
        .route("/user/:id", get(user))
        .route("/health", get(|| async { "ok" }));
    let listener = tokio::net::TcpListener::bind(&addr).await.expect("bind");
    axum::serve(listener, app).await.expect("serve");
}
