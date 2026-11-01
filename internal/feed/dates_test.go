package feed

import (
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		in   string
		want time.Time
	}{
		{"2024-01-02T03:04:05Z", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"2024-01-02T03:04:05+02:00", time.Date(2024, 1, 2, 1, 4, 5, 0, time.UTC)},
		{"2024-01-02T03:04:05.123456789Z", time.Date(2024, 1, 2, 3, 4, 5, 123456789, time.UTC)},
		{"Mon, 02 Jan 2006 15:04:05 -0700", time.Date(2006, 1, 2, 22, 4, 5, 0, time.UTC)},
		{"Mon, 02 Jan 2006 15:04:05 GMT", time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)},
		{"02 Jan 06 15:04 -0700", time.Date(2006, 1, 2, 22, 4, 0, 0, time.UTC)},
		{"02 Jan 06 15:04 MST", time.Date(2006, 1, 2, 15, 4, 0, 0, time.UTC)},
		{"Mon, 2 Jan 2006 15:04:05 -0700", time.Date(2006, 1, 2, 22, 4, 5, 0, time.UTC)},
		{"Mon, 2 Jan 2006 15:04:05 MST", time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)},
		{"2 Jan 2006 15:04:05 -0700", time.Date(2006, 1, 2, 22, 4, 5, 0, time.UTC)},
		{"2006-01-02 15:04:05", time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)},
		{"2006-01-02", time.Date(2006, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"January 2, 2006", time.Date(2006, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"2 January 2006", time.Date(2006, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"  2024-01-02  ", time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"Mon,  02 Jan 2006 15:04:05 GMT", time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)},
		{"1700000000", time.Unix(1700000000, 0).UTC()},
		{"1700000000123", time.UnixMilli(1700000000123).UTC()},
	}
	for _, tt := range tests {
		got, ok := ParseDate(tt.in)
		if !ok {
			t.Errorf("ParseDate(%q): not ok", tt.in)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("ParseDate(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestParseDateInvalid(t *testing.T) {
	for _, in := range []string{"", "yesterday", "12345", "2024-13-40", "Mon, 32 Jan 2006"} {
		if got, ok := ParseDate(in); ok {
			t.Errorf("ParseDate(%q) = %s, true; want false", in, got)
		}
	}
}
