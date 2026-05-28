package app

import "time"

type Clock interface {
	Unix() int64
	Location() *time.Location
}

type realClock struct{}

func (realClock) Unix() int64 { return time.Now().Unix() }
func (realClock) Location() *time.Location {
	return time.Local
}

type fixedClock struct {
	NowUnix int64
	Loc     *time.Location
}

func (f fixedClock) Unix() int64 { return f.NowUnix }
func (f fixedClock) Location() *time.Location {
	if f.Loc != nil {
		return f.Loc
	}
	return time.Local
}
