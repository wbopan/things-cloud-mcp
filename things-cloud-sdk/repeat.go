package thingscloud

import "time"

// FrequencyUnit describes recurring frequencies
type FrequencyUnit int64

var (
	// FrequencyUnitDaily occurs every n days
	FrequencyUnitDaily FrequencyUnit = 16
	// FrequencyUnitWeekly occurs every n weeks
	FrequencyUnitWeekly FrequencyUnit = 256
	// FrequencyUnitMonthly occurs every n months
	FrequencyUnitMonthly FrequencyUnit = 8
	// FrequencyUnitYearly occurs every n years
	FrequencyUnitYearly FrequencyUnit = 4
)

// RepeaterDetailConfiguration configures specifics of a repeater configuration.
type RepeaterDetailConfiguration struct {
	Day     *int64        `json:"dy,omitempty"`
	Month   *int64        `json:"mo,omitempty"`
	Weekday *time.Weekday `json:"wd,omitempty"`
	MonthOf *int64        `json:"wdo,omitempty"`
}

// RepeaterConfiguration configures the recurring rules of a task/ project
type RepeaterConfiguration struct {
	FirstScheduledAt    *Timestamp                    `json:"ia,omitempty"`
	RepeatCount         *int64                        `json:"rc,omitempty"`
	FrequencyUnit       FrequencyUnit                 `json:"fu"`
	FrequencyAmplitude  int64                         `json:"fa"`
	DetailConfiguration []RepeaterDetailConfiguration `json:"of"`
	LastScheduledAt     *Timestamp                    `json:"ed,omitempty"`
	Version             int                           `json:"rrv,omitempty"`
	Type                int                           `json:"tp,omitempty"`
	TimeShift           int                           `json:"ts,omitempty"`
	StartReference      *Timestamp                    `json:"sr,omitempty"`
}

// IsNeverending determines if a recurring rule has a specific end
func (c RepeaterConfiguration) IsNeverending() bool {
	return c.LastScheduledAt != nil && c.LastScheduledAt.Time().Year() == 4001
}

func (c RepeaterConfiguration) nextScheduledAt(repeat int, dcF func(time.Time, RepeaterDetailConfiguration) time.Time, aF func(time.Time) time.Time) time.Time {
	ia := *c.FirstScheduledAt.Time()

	if !c.IsNeverending() && *c.RepeatCount > 0 {
		if repeat >= int(*c.RepeatCount) {
			return time.Time{}
		}
	}

	for i := 0; i < repeat; i += len(c.DetailConfiguration) {
		min := ia
		if len(c.DetailConfiguration) > 1 {
			for j, dc := range c.DetailConfiguration[1:] {
				nt := dcF(ia, dc)
				ia = nt
				if i+j+1 >= repeat {
					return ia
				}
			}
		}
		nt := aF(min)
		if !c.IsNeverending() && c.LastScheduledAt != nil {
			if nt.After(*c.LastScheduledAt.Time()) {
				return time.Time{}
			}
		}
		ia = nt
	}

	return ia
}

func (c RepeaterConfiguration) nextWeeklyScheduledAt(repeat int) time.Time {
	return c.nextScheduledAt(repeat, func(t time.Time, dc RepeaterDetailConfiguration) time.Time {
		return t.AddDate(0, 0, int(*dc.Weekday-t.Weekday()))
	}, func(t time.Time) time.Time {
		return t.AddDate(0, 0, int(c.FrequencyAmplitude)*7)
	})
}

func (c RepeaterConfiguration) computeFirstWeeklyScheduleAt(t time.Time) time.Time {
	d := c.DetailConfiguration[0]
	for _, dc := range c.DetailConfiguration {
		if *dc.Weekday < *d.Weekday {
			d = dc
		}
	}

	if t.Weekday() < *d.Weekday {
		return t.AddDate(0, 0, int(*d.Weekday-t.Weekday()))
	} else if t.Weekday() > *d.Weekday {
		return t.AddDate(0, 0, 7-int(t.Weekday())+int(*d.Weekday))
	}
	return t
}

func (c RepeaterConfiguration) computeFirstMonthlyScheduleAt(t time.Time) time.Time {
	min := t.AddDate(1, 0, 0)
	for _, dc := range c.DetailConfiguration {
		var d time.Time
		if dc.Day != nil {
			d = t.AddDate(0, 0, -t.Day()+int(*dc.Day)+1)
			if d.Before(t) {
				d = t.AddDate(0, 1, -t.Day()+int(*dc.Day)+1)
			}
		}
		if dc.Weekday != nil {
			if *dc.MonthOf == -1 {
				d = lastWeekdayOfMonth(t, *dc.Weekday)
			} else {
				d = nthWeekdayOfMonth(t, *dc.Weekday, int(*dc.MonthOf))
			}
		}
		if d.Before(min) {
			min = d
		}
	}
	return min
}

func (c RepeaterConfiguration) computeFirstYearlyScheduleAt(t time.Time) time.Time {
	min := t.AddDate(1, 0, 0)
	for _, dc := range c.DetailConfiguration {
		var d time.Time
		if dc.Day != nil {
			d = nthDayOfMonthOfYear(t, int(*dc.Month), int(*dc.Day))
			if d.Before(t) {
				d = nthDayOfMonthOfYear(t.AddDate(1, 0, 0), int(*dc.Month), int(*dc.Day))
			}
		}
		if dc.Weekday != nil {
			nt := nthDayOfMonthOfYear(t, int(*dc.Month), 1)
			if dc.MonthOf != nil && *dc.MonthOf == -1 {
				d = lastWeekdayOfMonth(nt, *dc.Weekday)
				if d.Before(t) {
					d = lastWeekdayOfMonth(nt.AddDate(1, 0, 0), *dc.Weekday)
				}
			} else {
				d = nthWeekdayOfMonth(nt, *dc.Weekday, int(*dc.MonthOf))
				if d.Before(t) {
					d = nthWeekdayOfMonth(nt.AddDate(1, 0, 0), *dc.Weekday, int(*dc.MonthOf))
				}
			}
		}
		if d.Before(min) {
			min = d
		}
	}
	return min
}

// ComputeFirstScheduledAt calculates the first occurrence of a recurring rule based on the pattern
// This value has to be stored as FirstScheduledAt per thingscloud convention
func (c RepeaterConfiguration) ComputeFirstScheduledAt(t time.Time) time.Time {

	if c.FrequencyUnit == FrequencyUnitDaily {
		return t
	}

	if c.FrequencyUnit == FrequencyUnitWeekly {
		return c.computeFirstWeeklyScheduleAt(t)
	}

	if c.FrequencyUnit == FrequencyUnitMonthly {
		return c.computeFirstMonthlyScheduleAt(t)
	}

	if c.FrequencyUnit == FrequencyUnitYearly {
		return c.computeFirstYearlyScheduleAt(t)
	}

	return time.Time{}
}

func lastWeekdayOfMonth(t time.Time, wdo time.Weekday) time.Time {
	lastDayOfMonth := t.AddDate(0, 1, -t.Day())
	if lastDayOfMonth.Weekday() == wdo {
		return lastDayOfMonth
	}
	if lastDayOfMonth.Weekday() > wdo {
		return lastDayOfMonth.AddDate(0, 0, -int(lastDayOfMonth.Weekday()-wdo))
	}
	return lastDayOfMonth.AddDate(0, 0, -7-int(lastDayOfMonth.Weekday())+int(wdo))
}

func nthWeekdayOfMonth(t time.Time, wdo time.Weekday, n int) time.Time {
	firstDayOfMonth := t.AddDate(0, 0, -t.Day()+1)
	nthWeekdayOfMonth := firstDayOfMonth
	if firstDayOfMonth.Weekday() < wdo {
		nthWeekdayOfMonth = firstDayOfMonth.AddDate(0, 0, int(wdo-firstDayOfMonth.Weekday()))
	} else if firstDayOfMonth.Weekday() > wdo {
		nthWeekdayOfMonth = firstDayOfMonth.AddDate(0, 0, 7-int(firstDayOfMonth.Weekday())+int(wdo))
	}
	return nthWeekdayOfMonth.AddDate(0, 0, (n-1)*7)
}

func firstDayOfMonth(t time.Time) time.Time {
	return t.AddDate(0, 0, -t.Day()+1)
}

func lastDayOfMonth(t time.Time) time.Time {
	return firstDayOfMonth(t).AddDate(0, 1, 0).Add(-time.Hour)
}

func (c RepeaterConfiguration) nextMonthlyScheduledAt(repeat int) time.Time {
	return c.nextScheduledAt(repeat, func(t time.Time, dc RepeaterDetailConfiguration) time.Time {
		if dc.MonthOf != nil && dc.Weekday != nil {
			nt := t.AddDate(0, 0, -t.Day()+1)

			if *dc.MonthOf == -1 {
				return lastWeekdayOfMonth(nt, *dc.Weekday)
			}
			return nthWeekdayOfMonth(nt, *dc.Weekday, int(*dc.MonthOf))
		}
		if *dc.Day == -1 {
			return t.AddDate(0, 1, -t.Day())
		}
		return t.AddDate(0, 0, int(*dc.Day)-t.Day()+1)
	}, func(t time.Time) time.Time {
		nt := t.AddDate(0, int(c.FrequencyAmplitude), 0)
		if len(c.DetailConfiguration) == 1 {
			// correct last day of month
			if c.DetailConfiguration[0].Day != nil && *c.DetailConfiguration[0].Day == -1 {
				nt = nt.AddDate(0, 1, -nt.Day())
			} else if c.DetailConfiguration[0].MonthOf != nil {
				if *c.DetailConfiguration[0].MonthOf == -1 {
					nt = lastWeekdayOfMonth(nt, *c.DetailConfiguration[0].Weekday)
				} else {
					nt = nthWeekdayOfMonth(nt, *c.DetailConfiguration[0].Weekday, int(*c.DetailConfiguration[0].MonthOf))
				}
			}
		}

		return nt
	})
}

func nthDayOfMonthOfYear(t time.Time, month, day int) time.Time {
	return t.AddDate(0, -int(t.Month())+month+1, -t.Day()+day+1)
}

func (c RepeaterConfiguration) nextYearlyScheduledAt(repeat int) time.Time {
	return c.nextScheduledAt(repeat, func(t time.Time, dc RepeaterDetailConfiguration) time.Time {
		if dc.MonthOf != nil && dc.Weekday != nil {
			nt := nthDayOfMonthOfYear(t, int(*dc.Month), 1)

			if *dc.MonthOf == -1 {
				return lastWeekdayOfMonth(nt, *dc.Weekday)
			}
			return nthWeekdayOfMonth(nt, *dc.Weekday, int(*dc.MonthOf))
		}
		if *dc.Day == -1 {
			return lastDayOfMonth(nthDayOfMonthOfYear(t, int(*dc.Month), 1))
		}
		return nthDayOfMonthOfYear(t, int(*dc.Month), int(*dc.Day))

	}, func(t time.Time) time.Time {
		nt := t.AddDate(int(c.FrequencyAmplitude), 0, 0)
		if len(c.DetailConfiguration) == 1 {
			// correct last day of month
			if c.DetailConfiguration[0].MonthOf != nil {
				nt = nthDayOfMonthOfYear(nt, int(*c.DetailConfiguration[0].Month), 1)
				if *c.DetailConfiguration[0].MonthOf == -1 {
					return lastWeekdayOfMonth(nt, *c.DetailConfiguration[0].Weekday)
				}
				return nthWeekdayOfMonth(nt, *c.DetailConfiguration[0].Weekday, int(*c.DetailConfiguration[0].MonthOf))
			}
			if *c.DetailConfiguration[0].Day == -1 {
				return firstDayOfMonth(t).AddDate(int(c.FrequencyAmplitude), 1, -1)
			}
		}
		return nt
	})
}

func (c RepeaterConfiguration) nextDailyScheduledAt(repeat int) time.Time {
	ia := *c.FirstScheduledAt.Time()

	nt := ia.AddDate(0, 0, int(c.FrequencyAmplitude)*repeat)

	if c.IsNeverending() {
		return nt
	}

	if c.LastScheduledAt != nil {
		if nt.After(*c.LastScheduledAt.Time()) {
			return time.Time{}
		}
	} else {
		if repeat >= int(*c.RepeatCount) {
			return time.Time{}
		}
	}

	return nt
}

// NextOccurrenceAfter returns the next occurrence date strictly after current.
// This is used when completing a recurring task to advance tir to the next date.
func (c RepeaterConfiguration) NextOccurrenceAfter(current time.Time) time.Time {
	current = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC)

	switch c.FrequencyUnit {
	case FrequencyUnitDaily:
		return current.AddDate(0, 0, int(c.FrequencyAmplitude))

	case FrequencyUnitWeekly:
		if len(c.DetailConfiguration) <= 1 {
			return current.AddDate(0, 0, int(c.FrequencyAmplitude)*7)
		}
		// Multiple weekdays: find the next weekday in the cycle
		currentWD := current.Weekday()
		// Collect and sort weekdays
		type wdEntry struct {
			wd   time.Weekday
			orig int
		}
		var wds []wdEntry
		for i, dc := range c.DetailConfiguration {
			if dc.Weekday != nil {
				wds = append(wds, wdEntry{*dc.Weekday, i})
			}
		}
		// Sort by weekday value
		for i := 1; i < len(wds); i++ {
			for j := i; j > 0 && wds[j].wd < wds[j-1].wd; j-- {
				wds[j], wds[j-1] = wds[j-1], wds[j]
			}
		}
		// Find next weekday after current
		for _, w := range wds {
			if w.wd > currentWD {
				return current.AddDate(0, 0, int(w.wd-currentWD))
			}
		}
		// Wrap to first weekday of next cycle
		daysToFirst := 7*int(c.FrequencyAmplitude) - int(currentWD) + int(wds[0].wd)
		return current.AddDate(0, 0, daysToFirst)

	case FrequencyUnitMonthly:
		return current.AddDate(0, int(c.FrequencyAmplitude), 0)

	case FrequencyUnitYearly:
		return current.AddDate(int(c.FrequencyAmplitude), 0, 0)
	}
	return time.Time{}
}

// NextScheduledAt returns the next Nth date a rule should occur.
// Note that things generates these ToDos as necessary.
func (c RepeaterConfiguration) NextScheduledAt(repeat int) time.Time {
	if c.FrequencyUnit == FrequencyUnitDaily {
		return c.nextDailyScheduledAt(repeat)
	}

	// FirstScheduledAt is ALWAYS the first date matching pattern, invariant from thingscloud
	// TODO ensure the same invariant within this codebase!
	if c.FrequencyUnit == FrequencyUnitWeekly {
		return c.nextWeeklyScheduledAt(repeat)
	}
	if c.FrequencyUnit == FrequencyUnitMonthly {
		return c.nextMonthlyScheduledAt(repeat)
	}
	if c.FrequencyUnit == FrequencyUnitYearly {
		return c.nextYearlyScheduledAt(repeat)
	}
	return time.Time{}
}
