package model

import "time"

// EpochDays returns the number of civil days since 1970-01-01 for the local
// date containing t. The original computes elapsedDays(local time_t)/86400,
// which is equivalent to counting civil days of the local calendar date.
func EpochDays(t time.Time) int {
	y, m, d := t.Date()
	return daysFromCivil(y, int(m), d)
}

// daysFromCivil implements Howard Hinnant's days_from_civil algorithm.
func daysFromCivil(y, m, d int) int {
	if m <= 2 {
		y--
	}
	era := y / 400
	if y < 0 && y%400 != 0 {
		era--
	}
	yoe := y - era*400 // [0, 399]
	var doy int
	if m > 2 {
		doy = (153*(m-3)+2)/5 + d - 1
	} else {
		doy = (153*(m+9)+2)/5 + d - 1
	}
	doe := yoe*365 + yoe/4 - yoe/100 + doy // [0, 146096]
	return era*146097 + doe - 719468
}

// MinutesOfDay returns the minutes elapsed since local midnight.
func MinutesOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}

// SecondsOfDay returns the seconds elapsed since local midnight.
func SecondsOfDay(t time.Time) int {
	return t.Hour()*3600 + t.Minute()*60 + t.Second()
}
