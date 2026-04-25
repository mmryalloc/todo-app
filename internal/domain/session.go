package domain

import (
	"errors"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	UserAgent string
	IPAddress string
	CreatedAt time.Time
}
