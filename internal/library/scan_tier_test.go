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
	dormant := 3 * time.Hour
	cases := []struct {
		age      time.Duration
		tier     string
		interval time.Duration
	}{
		{time.Minute, "hot", base},
		{2 * time.Hour, "warm", warm},
		{7 * time.Hour, "warm", warmSlow},
		{2 * 24 * time.Hour, "cold", cold},
		{8 * 24 * time.Hour, "dormant", dormant},
	}
	for _, item := range cases {
		tier, interval := scanTier(now, now.Add(-item.age), base, warm, warmSlow, cold, dormant)
		if tier != item.tier || interval != item.interval {
			t.Fatalf("age %s: got %s/%s, want %s/%s", item.age, tier, interval, item.tier, item.interval)
		}
	}
}
