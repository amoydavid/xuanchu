package app

import "time"

type Clock interface {
	Unix() int64
}

type realClock struct{}

func (realClock) Unix() int64 { return time.Now().Unix() }

type fixedClock struct {
	NowUnix int64
}

func (f fixedClock) Unix() int64 { return f.NowUnix }
