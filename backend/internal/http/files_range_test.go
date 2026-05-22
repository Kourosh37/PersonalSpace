package httpapi

import "testing"

func TestParseSingleRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		header    string
		size      int64
		wantStart int64
		wantEnd   int64
		wantOK    bool
	}{
		{name: "full explicit range", header: "bytes=0-99", size: 1000, wantStart: 0, wantEnd: 99, wantOK: true},
		{name: "open ended range", header: "bytes=100-", size: 1000, wantStart: 100, wantEnd: 999, wantOK: true},
		{name: "suffix range", header: "bytes=-200", size: 1000, wantStart: 800, wantEnd: 999, wantOK: true},
		{name: "suffix larger than size", header: "bytes=-5000", size: 1000, wantStart: 0, wantEnd: 999, wantOK: true},
		{name: "multiple ranges not supported", header: "bytes=0-10,20-30", size: 1000, wantOK: false},
		{name: "invalid prefix", header: "items=0-10", size: 1000, wantOK: false},
		{name: "start out of bounds", header: "bytes=1000-1001", size: 1000, wantOK: false},
		{name: "end before start", header: "bytes=100-50", size: 1000, wantOK: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start, end, ok := parseSingleRange(tc.header, tc.size)
			if ok != tc.wantOK {
				t.Fatalf("ok mismatch: got %v want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if start != tc.wantStart || end != tc.wantEnd {
				t.Fatalf("range mismatch: got %d-%d want %d-%d", start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

