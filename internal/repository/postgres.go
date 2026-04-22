package repository

import (
	"errors"

	"github.com/lib/pq"
)

const pgUniqueViolationCode = "23505"

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == pgUniqueViolationCode
}
