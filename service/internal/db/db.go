package db

import (
    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq"
)

type DB = sqlx.DB

func Connect(dsn string) (*DB, error) {
    return sqlx.Connect("postgres", dsn)
}

