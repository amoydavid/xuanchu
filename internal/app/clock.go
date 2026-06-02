package app

import "time"

type Clock interface {
	Unix() int64
	Location() *time.Location
}

type RealClock struct{}

func (RealClock) Unix() int64 { return time.Now().Unix() }
func (RealClock) Location() *time.Location {
	return time.Local
}

type FixedClock struct {
	NowUnix int64
	Loc     *time.Location
}

func (f FixedClock) Unix() int64 { return f.NowUnix }
func (f FixedClock) Location() *time.Location {
	if f.Loc != nil {
		return f.Loc
	}
	return time.Local
}
