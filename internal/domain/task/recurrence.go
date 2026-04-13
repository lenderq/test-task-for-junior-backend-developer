package task

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const recurrenceDateLayout = "2006-01-02"

type RecurrenceType string

const (
	RecurrenceEveryNDays   RecurrenceType = "every_n_days"
	RecurrenceMonthlyByDay RecurrenceType = "monthly_by_day"
	RecurrenceSpecificDates RecurrenceType = "specific_dates"
	RecurrenceDayParity    RecurrenceType = "day_parity"
)

type DayParity string

const (
	DayParityOdd  DayParity = "odd"
	DayParityEven DayParity = "even"
)

type Date struct {
	time.Time
}

type Recurrence struct {
	Type          RecurrenceType `json:"type"`
	EveryNDays    int            `json:"every_n_days,omitempty"`
	DayOfMonth    int            `json:"day_of_month,omitempty"`
	SpecificDates []Date         `json:"specific_dates,omitempty"`
	DayParity     DayParity      `json:"day_parity,omitempty"`
}

func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse(recurrenceDateLayout, value)
	if err != nil {
		return Date{}, fmt.Errorf("parse recurrence date: %w", err)
	}

	return Date{Time: parsed.UTC()}, nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Date) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	parsed, err := ParseDate(raw)
	if err != nil {
		return err
	}

	*d = parsed

	return nil
}

func (d Date) String() string {
	return d.Time.UTC().Format(recurrenceDateLayout)
}

func (p DayParity) Valid() bool {
	switch p {
	case DayParityOdd, DayParityEven:
		return true
	default:
		return false
	}
}

func (r *Recurrence) Clone() *Recurrence {
	if r == nil {
		return nil
	}

	cloned := *r
	if len(r.SpecificDates) > 0 {
		cloned.SpecificDates = append([]Date(nil), r.SpecificDates...)
	}

	return &cloned
}

func (r Recurrence) Normalize(anchor time.Time) (*Recurrence, error) {
	anchor = anchor.UTC()

	normalized := Recurrence{Type: r.Type}

	switch r.Type {
	case RecurrenceEveryNDays:
		if r.EveryNDays < 1 {
			return nil, fmt.Errorf("every_n_days must be greater than zero")
		}

		normalized.EveryNDays = r.EveryNDays
	case RecurrenceMonthlyByDay:
		if r.DayOfMonth < 1 || r.DayOfMonth > 30 {
			return nil, fmt.Errorf("day_of_month must be between 1 and 30")
		}

		normalized.DayOfMonth = r.DayOfMonth
	case RecurrenceSpecificDates:
		if len(r.SpecificDates) == 0 {
			return nil, fmt.Errorf("specific_dates must not be empty")
		}

		seen := make(map[string]struct{}, len(r.SpecificDates))
		dates := make([]Date, 0, len(r.SpecificDates))
		hasFutureDate := false
		anchorDate := startOfDay(anchor)

		for _, date := range r.SpecificDates {
			value := Date{Time: startOfDay(date.Time.UTC())}
			key := value.String()
			if _, ok := seen[key]; ok {
				continue
			}

			seen[key] = struct{}{}
			if !value.Time.Before(anchorDate) {
				hasFutureDate = true
			}

			dates = append(dates, value)
		}

		if !hasFutureDate {
			return nil, fmt.Errorf("specific_dates must contain at least one date on or after scheduled_at")
		}

		sort.Slice(dates, func(i, j int) bool {
			return dates[i].Time.Before(dates[j].Time)
		})

		normalized.SpecificDates = dates
	case RecurrenceDayParity:
		if !r.DayParity.Valid() {
			return nil, fmt.Errorf("day_parity must be odd or even")
		}

		normalized.DayParity = r.DayParity
	default:
		return nil, fmt.Errorf("unknown recurrence type")
	}

	return &normalized, nil
}

func (r Recurrence) OccurrencesBetween(anchor, from, to time.Time) ([]time.Time, error) {
	normalized, err := r.Normalize(anchor)
	if err != nil {
		return nil, err
	}

	anchor = anchor.UTC()
	from = from.UTC()
	to = to.UTC()

	if to.Before(from) {
		return []time.Time{}, nil
	}

	switch normalized.Type {
	case RecurrenceEveryNDays:
		return everyNDaysOccurrences(anchor, from, to, normalized.EveryNDays), nil
	case RecurrenceMonthlyByDay:
		return monthlyOccurrences(anchor, from, to, normalized.DayOfMonth), nil
	case RecurrenceSpecificDates:
		return specificDateOccurrences(anchor, from, to, normalized.SpecificDates), nil
	case RecurrenceDayParity:
		return parityOccurrences(anchor, from, to, normalized.DayParity), nil
	default:
		return nil, fmt.Errorf("unknown recurrence type")
	}
}

func everyNDaysOccurrences(anchor, from, to time.Time, step int) []time.Time {
	candidate := anchor
	if from.After(anchor) {
		diffDays := int(startOfDay(from).Sub(startOfDay(anchor)).Hours() / 24)
		if diffDays > 0 {
			candidate = anchor.AddDate(0, 0, (diffDays/step)*step)
		}
		for candidate.Before(from) {
			candidate = candidate.AddDate(0, 0, step)
		}
	}

	occurrences := make([]time.Time, 0)
	for !candidate.After(to) {
		if !candidate.Before(from) {
			occurrences = append(occurrences, candidate)
		}

		candidate = candidate.AddDate(0, 0, step)
	}

	return occurrences
}

func monthlyOccurrences(anchor, from, to time.Time, dayOfMonth int) []time.Time {
	startMonth := time.Date(from.Year(), from.Month(), 1, anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), time.UTC)
	anchorMonth := time.Date(anchor.Year(), anchor.Month(), 1, anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), time.UTC)
	if startMonth.Before(anchorMonth) {
		startMonth = anchorMonth
	}

	occurrences := make([]time.Time, 0)
	for month := startMonth; !month.After(to); month = month.AddDate(0, 1, 0) {
		candidate := time.Date(month.Year(), month.Month(), dayOfMonth, anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), time.UTC)
		if candidate.Month() != month.Month() {
			continue
		}

		if candidate.Before(anchor) || candidate.Before(from) || candidate.After(to) {
			continue
		}

		occurrences = append(occurrences, candidate)
	}

	return occurrences
}

func specificDateOccurrences(anchor, from, to time.Time, dates []Date) []time.Time {
	occurrences := make([]time.Time, 0, len(dates))
	for _, date := range dates {
		candidate := time.Date(
			date.Time.Year(),
			date.Time.Month(),
			date.Time.Day(),
			anchor.Hour(),
			anchor.Minute(),
			anchor.Second(),
			anchor.Nanosecond(),
			time.UTC,
		)

		if candidate.Before(anchor) || candidate.Before(from) || candidate.After(to) {
			continue
		}

		occurrences = append(occurrences, candidate)
	}

	return occurrences
}

func parityOccurrences(anchor, from, to time.Time, parity DayParity) []time.Time {
	start := startOfDay(from)
	anchorDay := startOfDay(anchor)
	if start.Before(anchorDay) {
		start = anchorDay
	}

	occurrences := make([]time.Time, 0)
	for day := start; !day.After(to); day = day.AddDate(0, 0, 1) {
		if !matchesParity(day.Day(), parity) {
			continue
		}

		candidate := time.Date(day.Year(), day.Month(), day.Day(), anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), time.UTC)
		if candidate.Before(anchor) || candidate.Before(from) || candidate.After(to) {
			continue
		}

		occurrences = append(occurrences, candidate)
	}

	return occurrences
}

func matchesParity(day int, parity DayParity) bool {
	switch parity {
	case DayParityOdd:
		return day%2 == 1
	case DayParityEven:
		return day%2 == 0
	default:
		return false
	}
}

func startOfDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
