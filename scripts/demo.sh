#!/bin/bash
# Runs the demo script through one runner and prints the transcript.
RUN=("$@")
D=$(mktemp -d)
"${RUN[@]}" exec "$D" \
  "CREATE TABLE users (name TEXT, age INT32, active BOOL, at DATETIME, score FLOAT)" \
  "INSERT INTO users VALUES ('alice', 30, true, '2023-11-14T22:13:20Z', 1.5), ('bob', 40, false, '2023-12-01T10:00:00Z', 2.5), ('carol', 25, true, '2024-01-01T00:00:00Z', 3.5)" \
  "CREATE INDEX ON users (name)" \
  "SELECT name, age FROM users WHERE name = 'bob'" \
  "SELECT name, age FROM users WHERE age > 26 ORDER BY age DESC" \
  "SELECT COUNT(*) FROM users" \
  "UPDATE users SET score = 9.5 WHERE name = 'bob'" \
  "SELECT * FROM users WHERE name = 'bob'" \
  "DELETE FROM users WHERE age < 28" "COMPACT" \
  "SELECT name, score FROM users WHERE name = 'bob'" \
  "SELECT name, score FROM users ORDER BY name" 2>&1
# a genuine restart, in a second process
"${RUN[@]}" exec "$D" "SELECT COUNT(*) FROM users" \
  "SELECT name, score FROM users WHERE name = 'bob'" \
  "SELECT * FROM users ORDER BY name" 2>&1
rm -rf "$D"
