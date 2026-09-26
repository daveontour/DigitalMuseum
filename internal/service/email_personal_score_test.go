package service

import "testing"

func TestEmailClassFlag(t *testing.T) {
	if emailClassFlag(0.5) {
		t.Error("probability of 0.5 should not set the flag")
	}
	if emailClassFlag(0.49) {
		t.Error("probability under 0.5 should not set the flag")
	}
	if !emailClassFlag(0.51) {
		t.Error("probability over 0.5 should set the flag")
	}
}

func TestPersonalScoreFromNoul(t *testing.T) {
	cases := []struct {
		p    float64
		want int
	}{
		{0, 0},
		{1, 100},
		{0.5, 50},
		{0.726, 73},
		{0.004, 0},
		{0.995, 100},
		{-0.2, 0},
		{1.4, 100},
	}
	for _, tc := range cases {
		if got := personalScoreFromNoul(tc.p); got != tc.want {
			t.Errorf("personalScoreFromNoul(%v) = %d, want %d", tc.p, got, tc.want)
		}
	}
}
