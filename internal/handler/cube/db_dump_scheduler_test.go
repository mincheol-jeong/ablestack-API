package cube

import (
	"testing"
	"time"
)

func TestNextAutoCCVMDBDumpTime(t *testing.T) {
	location := time.FixedZone("KST", 9*60*60)
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before schedule",
			now:  time.Date(2026, 8, 20, 0, 30, 0, 0, location),
			want: time.Date(2026, 8, 20, 1, 0, 0, 0, location),
		},
		{
			name: "at schedule",
			now:  time.Date(2026, 8, 20, 1, 0, 0, 0, location),
			want: time.Date(2026, 8, 21, 1, 0, 0, 0, location),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextAutoCCVMDBDumpTime(tt.now); !got.Equal(tt.want) {
				t.Fatalf("next schedule = %s, want %s", got, tt.want)
			}
		})
	}
}
