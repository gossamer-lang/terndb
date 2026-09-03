//! axum + redb, an embedded key-value store.
//!
//! The row is stored as its finished JSON under its integer id, which is the
//! closest redb equivalent of the single keyed lookup the other servers do.

use axum::{extract::Path, http::StatusCode, response::IntoResponse, routing::get, Router};
use redb::{Database, ReadableTable, TableDefinition};
use std::sync::Arc;

const USERS: TableDefinition<u64, &str> = TableDefinition::new("users");

struct State {
    db: Database,
}

fn db_path() -> String {
    std::env::var("DB_PATH").unwrap_or_else(|_| "bench.redb".to_string())
}

async fn user(
    axum::extract::State(state): axum::extract::State<Arc<State>>,
    Path(id): Path<u64>,
) -> impl IntoResponse {
    // redb reads take a snapshot and do not block writers or each other.
    let txn = match state.db.begin_read() {
        Ok(t) => t,
        Err(_) => return StatusCode::INTERNAL_SERVER_ERROR.into_response(),
    };
    let table = match txn.open_table(USERS) {
        Ok(t) => t,
        Err(_) => return StatusCode::INTERNAL_SERVER_ERROR.into_response(),
    };
    match table.get(id) {
        Ok(Some(v)) => (
            [("content-type", "application/json")],
            v.value().to_string(),
        )
            .into_response(),
        _ => (
            StatusCode::NOT_FOUND,
            [("content-type", "application/json")],
            r#"{"error":"not found"}"#,
        )
            .into_response(),
    }
}

#[tokio::main]
async fn main() {
    let addr = std::env::var("ADDR").unwrap_or_else(|_| "127.0.0.1:8082".to_string());
    let db = Database::open(db_path()).expect("open redb");
    let state = Arc::new(State { db });
    let app = Router::new()
        .route("/user/:id", get(user))
        .route("/health", get(|| async { "ok" }))
        .with_state(state);
    let listener = tokio::net::TcpListener::bind(&addr).await.expect("bind");
    axum::serve(listener, app).await.expect("serve");
}
