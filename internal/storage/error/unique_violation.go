// Package error holds the errors the storage layer raises on its own behalf — the ones with no
// portable representation in database/sql, which every driver adapter therefore has to normalize
// before a repository can recognize them. It deliberately names itself after the predeclared type,
// like internal/domain/error does, so importers alias it (storageError) and the two never blur
// together at a call site.
package error

import "errors"

// UniqueViolationError reports that a write collided with a UNIQUE constraint. Each driver adapter
// detects its own flavor of the underlying error (SQLite result code, MySQL 1062, Postgres 23505)
// and wraps it in this sentinel; the repository that recognizes it with errors.Is is what turns it
// into a domain *ConflictError, with wording that fits the record in question. Nothing maps this one
// to an HTTP status — it isn't meant to travel past the repository.
//
// Its counterpart needs no declaration: a lookup that matched nothing crosses the same boundary as
// the standard library's sql.ErrNoRows, which repositories translate into a domain *NotFoundError.
var UniqueViolationError = errors.New("unique constraint violation")
