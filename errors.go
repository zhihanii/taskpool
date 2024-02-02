package gopool

import "errors"

var (
	ErrPoolIsReleased = errors.New("pool is released")
	ErrPoolIsFull     = errors.New("pool is full")
	ErrPoolIsOverload = errors.New("pool is overload")
)
