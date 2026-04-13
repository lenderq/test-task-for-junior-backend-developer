package task

import (
	"reflect"
	"testing"
	"time"
)

func TestRecurrenceOccurrencesBetween(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		recurrence Recurrence
		anchor     time.Time
		from       time.Time
		to         time.Time
		want       []string
	}{
		{
			name: "every_n_days",
			recurrence: Recurrence{
				Type:       RecurrenceEveryNDays,
				EveryNDays: 2,
			},
			anchor: mustTime(t, "2026-04-13T09:00:00Z"),
			from:   mustTime(t, "2026-04-13T00:00:00Z"),
			to:     mustTime(t, "2026-04-18T23:59:59Z"),
			want: []string{
				"2026-04-13T09:00:00Z",
				"2026-04-15T09:00:00Z",
				"2026-04-17T09:00:00Z",
			},
		},
		{
			name: "monthly_by_day_skips_invalid_months",
			recurrence: Recurrence{
				Type:       RecurrenceMonthlyByDay,
				DayOfMonth: 30,
			},
			anchor: mustTime(t, "2026-01-10T11:30:00Z"),
			from:   mustTime(t, "2026-01-01T00:00:00Z"),
			to:     mustTime(t, "2026-03-31T23:59:59Z"),
			want: []string{
				"2026-01-30T11:30:00Z",
				"2026-03-30T11:30:00Z",
			},
		},
		{
			name: "specific_dates",
			recurrence: Recurrence{
				Type: RecurrenceSpecificDates,
				SpecificDates: []Date{
					mustDate(t, "2026-04-15"),
					mustDate(t, "2026-04-20"),
					mustDate(t, "2026-05-01"),
				},
			},
			anchor: mustTime(t, "2026-04-13T08:00:00Z"),
			from:   mustTime(t, "2026-04-14T00:00:00Z"),
			to:     mustTime(t, "2026-04-30T23:59:59Z"),
			want: []string{
				"2026-04-15T08:00:00Z",
				"2026-04-20T08:00:00Z",
			},
		},
		{
			name: "day_parity",
			recurrence: Recurrence{
				Type:      RecurrenceDayParity,
				DayParity: DayParityEven,
			},
			anchor: mustTime(t, "2026-04-13T07:45:00Z"),
			from:   mustTime(t, "2026-04-13T00:00:00Z"),
			to:     mustTime(t, "2026-04-18T23:59:59Z"),
			want: []string{
				"2026-04-14T07:45:00Z",
				"2026-04-16T07:45:00Z",
				"2026-04-18T07:45:00Z",
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			occurrences, err := testCase.recurrence.OccurrencesBetween(testCase.anchor, testCase.from, testCase.to)
			if err != nil {
				t.Fatalf("OccurrencesBetween() error = %v", err)
			}

			got := make([]string, 0, len(occurrences))
			for _, occurrence := range occurrences {
				got = append(got, occurrence.UTC().Format(time.RFC3339))
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("OccurrencesBetween() mismatch\nwant: %#v\ngot:  %#v", testCase.want, got)
			}
		})
	}
}

func TestRecurrenceNormalizeSpecificDates(t *testing.T) {
	t.Parallel()

	recurrence := Recurrence{
		Type: RecurrenceSpecificDates,
		SpecificDates: []Date{
			mustDate(t, "2026-04-20"),
			mustDate(t, "2026-04-15"),
			mustDate(t, "2026-04-20"),
		},
	}

	normalized, err := recurrence.Normalize(mustTime(t, "2026-04-13T09:00:00Z"))
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	got := make([]string, 0, len(normalized.SpecificDates))
	for _, date := range normalized.SpecificDates {
		got = append(got, date.String())
	}

	want := []string{"2026-04-15", "2026-04-20"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func mustDate(t *testing.T, value string) Date {
	t.Helper()

	parsed, err := ParseDate(value)
	if err != nil {
		t.Fatalf("ParseDate(%q) error = %v", value, err)
	}

	return parsed
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", value, err)
	}

	return parsed.UTC()
}
