package library

import (
	"testing"
	"time"
)

func TestScanTierUsesConfiguredIntervals(t *testing.T) {
	now := time.Now()
	base := 30 * time.Second
	warm := 10 * time.Minute
	warmSlow := 20 * time.Minute
	cold := 40 * time.Minute
	dormant := 24 * time.Hour
	cases := []struct {
		age      time.Duration
		tier     string
		interval time.Duration
	}{
		{time.Minute, "hot", base},
		{2 * time.Hour, "warm", warm},
		{7 * time.Hour, "warm", warmSlow},
		{24 * time.Hour, "cold", cold},
		{2*24*time.Hour + time.Hour, "cold", cold * 3},
		{4 * 24 * time.Hour, "cold", cold * 6},
		{6 * 24 * time.Hour, "cold", cold * 12},
		{8 * 24 * time.Hour, "dormant", dormant},
	}
	for _, item := range cases {
		tier, interval := scanTier(now, now.Add(-item.age), base, warm, warmSlow, cold, dormant)
		if tier != item.tier || interval != item.interval {
			t.Fatalf("age %s: got %s/%s, want %s/%s", item.age, tier, interval, item.tier, item.interval)
		}
	}
}

func TestPromotedTierMovesOneStep(t *testing.T) {
	scanner := &Scanner{interval: 30 * time.Second, warm: 5 * time.Minute, warmSlow: 15 * time.Minute, cold: time.Hour, idleMax: 24 * time.Hour}
	cases := []struct {
		tier     string
		interval time.Duration
		wantTier string
		want     time.Duration
	}{
		{"cold", time.Hour, "warm", 15 * time.Minute},
		{"cold", 3 * time.Hour, "cold", time.Hour},
		{"cold", 6 * time.Hour, "cold", 3 * time.Hour},
		{"cold", 12 * time.Hour, "cold", 6 * time.Hour},
		{"warm", 15 * time.Minute, "warm", 5 * time.Minute},
		{"warm", 5 * time.Minute, "hot", 30 * time.Second},
		{"dormant", 24 * time.Hour, "cold", 12 * time.Hour},
	}
	for _, tc := range cases {
		tier, interval := scanner.promotedTier(tc.tier, tc.interval)
		if tier != tc.wantTier || interval != tc.want {
			t.Fatalf("promote %s/%s: got %s/%s want %s/%s", tc.tier, tc.interval, tier, interval, tc.wantTier, tc.want)
		}
	}
}
