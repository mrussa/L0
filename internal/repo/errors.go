package repo

import "errors"

var (
	ErrNotFound     = errors.New("order not found")
	ErrBadUID       = errors.New("bad order_uid")
	ErrInconsistent = errors.New("inconsistent data")
)

const (
	maxUIDLen       = 100
	defaultItemsCap = 8
)
