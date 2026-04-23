package repository

import (
	"errors"

	"github.com/lib/pq"
)

const (
	pgUniqueViolationCode     = "23505"
	pgForeignKeyViolationCode = "23503"
)

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == pgUniqueViolationCode
}

func isForeignKeyViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == pgForeignKeyViolationCode
}
