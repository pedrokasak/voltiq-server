package domain

import (
	"time"
	_ "time/tzdata" // embute o banco de fusos: a imagem de producao pode nao ter zoneinfo
)

// BusinessLocation is the timezone of billing periods, balances and
// schedules (horario de Brasilia).
var BusinessLocation = func() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return loc
}()

// MonthStart returns the first instant of t's month in BusinessLocation.
func MonthStart(t time.Time) time.Time {
	l := t.In(BusinessLocation)
	return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, BusinessLocation)
}
