package model_test

import (
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"wherewhat/internal/storage/model"
)

// TestTimeScan covers every shape the three supported drivers hand back for a TIMESTAMP column: a
// natively-decoded time.Time, and text in each of the layouts model.Time knows how to parse. Every
// input is derived from one random instant, so a case can't accidentally pass by matching a stale
// literal instead of what Scan actually did with it.
func TestTimeScan(t *testing.T) {
	want := gofakeit.DateRange(
		time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
	).UTC().Truncate(time.Second)

	nanos := time.Duration(gofakeit.Number(1, 999_999_999))
	withNanos := want.Add(nanos)

	offsetHours := gofakeit.Number(1, 12)
	if gofakeit.Bool() {
		offsetHours = -offsetHours
	}

	offsetSign, absOffset := "+", offsetHours
	if offsetHours < 0 {
		offsetSign, absOffset = "-", -offsetHours
	}

	offsetLocal := want.Add(time.Duration(offsetHours) * time.Hour)
	offsetInput := offsetLocal.Format("2006-01-02 15:04:05") + offsetSign + itoa02(absOffset) + ":00"

	unparseable := gofakeit.Sentence()
	unsupported := gofakeit.Number(1, 1_000_000)

	tests := []struct {
		name    string
		in      any
		want    time.Time
		wantErr bool
	}{
		{name: "time.Time value", in: want, want: want},
		{name: "RFC3339Nano string", in: withNanos.Format(time.RFC3339Nano), want: withNanos},
		{name: "RFC3339 string", in: want.Format(time.RFC3339), want: want},
		{name: "space-separated string", in: want.Format("2006-01-02 15:04:05"), want: want},
		{name: "offset string", in: offsetInput, want: want},
		{name: "[]byte", in: []byte(want.Format(time.RFC3339)), want: want},
		{name: "nil column", in: nil, want: time.Time{}},
		{name: "unparseable string", in: unparseable, wantErr: true},
		{name: "unsupported type", in: unsupported, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got model.Time

			err := got.Scan(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Scan(%v) error = nil, want error", tt.in)
				}

				return
			}

			if err != nil {
				t.Fatalf("Scan(%v) unexpected error: %v", tt.in, err)
			}

			if !got.Equal(tt.want) {
				t.Fatalf("Scan(%v) = %v, want %v", tt.in, got.Time, tt.want)
			}
		})
	}
}

// TestTimeValue checks that a Time binds as the plain time.Time every driver already understands.
func TestTimeValue(t *testing.T) {
	want := gofakeit.Date().UTC()

	got, err := model.NewTime(want).Value()
	if err != nil {
		t.Fatalf("Value() unexpected error: %v", err)
	}

	if got != any(want) {
		t.Fatalf("Value() = %v, want %v", got, want)
	}
}

// itoa02 renders n (0-99) as a zero-padded two-digit string, for the "+03:00"-style offset that
// TestTimeScan's offset case builds by hand rather than trusting a hardcoded example.
func itoa02(n int) string {
	digits := "0123456789"

	return string([]byte{digits[n/10], digits[n%10]})
}
