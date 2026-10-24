package store

import (
	"database/sql"
	"errors"
)

var (
	ErrNotFound = errors.New("resource not found")
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *sql.DB {
	return s.db
}
