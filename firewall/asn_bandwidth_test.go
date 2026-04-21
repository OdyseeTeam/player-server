package firewall

import (
	"testing"
	"time"

	"github.com/bluele/gcache"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetASNBandwidthState(t *testing.T) {
	t.Helper()
	asnBandwidthMultiplier.Store(10)
	asnBandwidthMinThreshold.Store(defaultMinASNThreshold)
	asnBandwidthWarmup.Store(0)
	completedBandwidthSnapshot.Store(nil)
	bandwidthPerIP.Store(xsync.NewMapOf[string, *int64]())
	bandwidthPerASN.Store(xsync.NewMapOf[string, *asnBandwidth]())
	ipToASNCache = gcache.New(10000).LRU().Build()
	nowFunc = time.Now
	blockCooldown = 10 * time.Minute
}

func withFakeClock(t *testing.T, start time.Time) *fakeClock {
	t.Helper()
	c := &fakeClock{now: start}
	nowFunc = c.Now
	t.Cleanup(func() { nowFunc = time.Now })
	return c
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func fiveBaselineEntries() []asnEntry {
	out := make([]asnEntry, minASNCount)
	for i := range out {
		out[i] = asnEntry{
			ASN:   "AS" + string(rune('A'+i)),
			Org:   "Baseline",
			Bytes: 1 << 30,
		}
	}
	return out
}

func TestComputeThreshold_WarmupReturnsZeroAndDecrements(t *testing.T) {
	resetASNBandwidthState(t)
	asnBandwidthWarmup.Store(3)

	assert.Equal(t, int64(0), computeASNBandwidthThreshold(fiveBaselineEntries()))
	assert.Equal(t, int32(2), asnBandwidthWarmup.Load())

	computeASNBandwidthThreshold(fiveBaselineEntries())
	computeASNBandwidthThreshold(fiveBaselineEntries())
	assert.Equal(t, int32(0), asnBandwidthWarmup.Load())

	threshold := computeASNBandwidthThreshold(fiveBaselineEntries())
	assert.Greater(t, threshold, int64(0), "threshold should be positive after warmup")
}

func TestComputeThreshold_BelowMinASNCount(t *testing.T) {
	resetASNBandwidthState(t)
	entries := fiveBaselineEntries()[:minASNCount-1]
	assert.Equal(t, int64(0), computeASNBandwidthThreshold(entries))
}

func TestComputeThreshold_MultiplierZero(t *testing.T) {
	resetASNBandwidthState(t)
	asnBandwidthMultiplier.Store(0)
	assert.Equal(t, int64(0), computeASNBandwidthThreshold(fiveBaselineEntries()))
}

func TestComputeThreshold_BelowMinThreshold(t *testing.T) {
	resetASNBandwidthState(t)
	asnBandwidthMinThreshold.Store(1 << 40) // 1 TiB floor, well above median*multiplier
	assert.Equal(t, int64(0), computeASNBandwidthThreshold(fiveBaselineEntries()))
}

func TestComputeThreshold_Normal(t *testing.T) {
	resetASNBandwidthState(t)
	asnBandwidthMinThreshold.Store(0)
	asnBandwidthMultiplier.Store(10)
	threshold := computeASNBandwidthThreshold(fiveBaselineEntries())
	assert.Equal(t, int64(10)<<30, threshold)
}

func TestHysteresis_SingleOverThresholdDoesNotBlock(t *testing.T) {
	resetASNBandwidthState(t)
	now := time.Unix(1_700_000_000, 0)
	observed := map[string]asnSnapshotEntry{
		"AS714": {Org: "Apple Inc.", Bytes: 100 << 30},
	}
	over, blocks := buildHysteresisState(nil, observed, 10<<30, now)

	assert.Equal(t, 1, over["AS714"])
	assert.Empty(t, blocks, "single over-threshold window must not create a block")
}

func TestHysteresis_TwoConsecutiveOverSetsBlock(t *testing.T) {
	resetASNBandwidthState(t)
	now1 := time.Unix(1_700_000_000, 0)
	observed := map[string]asnSnapshotEntry{
		"AS714": {Org: "Apple Inc.", Bytes: 100 << 30},
	}

	over1, blocks1 := buildHysteresisState(nil, observed, 10<<30, now1)
	prev := &bandwidthSnapshot{
		consecutiveOver: over1,
		blockedUntil:    blocks1,
	}

	now2 := now1.Add(2 * time.Minute)
	over2, blocks2 := buildHysteresisState(prev, observed, 10<<30, now2)

	assert.Equal(t, 2, over2["AS714"])
	require.Contains(t, blocks2, "AS714", "second consecutive over-threshold window must create a block")
	assert.Equal(t, now2.Add(blockCooldown), blocks2["AS714"].Until)
	assert.Equal(t, "Apple Inc.", blocks2["AS714"].Org)
}

func TestHysteresis_UnderThresholdResetsCounter(t *testing.T) {
	resetASNBandwidthState(t)
	now := time.Unix(1_700_000_000, 0)
	prev := &bandwidthSnapshot{
		consecutiveOver: map[string]int{"AS714": 1},
		blockedUntil:    map[string]asnBlockEntry{},
	}
	observed := map[string]asnSnapshotEntry{
		"AS714": {Org: "Apple Inc.", Bytes: 1 << 30}, // under the 10-GiB threshold
	}
	over, blocks := buildHysteresisState(prev, observed, 10<<30, now)

	assert.Equal(t, 0, over["AS714"])
	assert.Empty(t, blocks)
}

func TestHysteresis_ThresholdZeroDoesNotAccumulate(t *testing.T) {
	resetASNBandwidthState(t)
	now := time.Unix(1_700_000_000, 0)
	observed := map[string]asnSnapshotEntry{
		"AS714": {Org: "Apple Inc.", Bytes: 100 << 30},
		"AS1":   {Org: "Another", Bytes: 50 << 30},
	}
	over, blocks := buildHysteresisState(nil, observed, 0, now)

	assert.Empty(t, over, "threshold<=0 must not increment consecutiveOver")
	assert.Empty(t, blocks, "threshold<=0 must not create blocks")
}

func TestHysteresis_ThresholdZeroResetsObservedCounter(t *testing.T) {
	resetASNBandwidthState(t)
	now := time.Unix(1_700_000_000, 0)
	prev := &bandwidthSnapshot{
		consecutiveOver: map[string]int{"AS714": 1},
		blockedUntil:    map[string]asnBlockEntry{},
	}
	observed := map[string]asnSnapshotEntry{
		"AS714": {Org: "Apple Inc.", Bytes: 100 << 30},
	}
	over, _ := buildHysteresisState(prev, observed, 0, now)

	assert.Empty(t, over, "threshold<=0 must reset consecutiveOver even for observed ASNs")
}

func TestHysteresis_NonEnforcingWindowPreservesExistingBlock(t *testing.T) {
	resetASNBandwidthState(t)
	now := time.Unix(1_700_000_000, 0)
	until := now.Add(5 * time.Minute)
	prev := &bandwidthSnapshot{
		consecutiveOver: map[string]int{"AS714": 2},
		blockedUntil: map[string]asnBlockEntry{
			"AS714": {Until: until, Org: "Apple Inc."},
		},
	}
	observed := map[string]asnSnapshotEntry{"AS1": {Org: "Other", Bytes: 1 << 30}}

	_, blocks := buildHysteresisState(prev, observed, 0, now)

	require.Contains(t, blocks, "AS714", "unexpired block must carry across a threshold<=0 window")
	assert.Equal(t, until, blocks["AS714"].Until)
}

func TestHysteresis_BlockSurvivesZeroBandwidthWindows(t *testing.T) {
	resetASNBandwidthState(t)
	now1 := time.Unix(1_700_000_000, 0)

	observed := map[string]asnSnapshotEntry{
		"AS714": {Org: "Apple Inc.", Bytes: 100 << 30},
	}
	over1, blocks1 := buildHysteresisState(nil, observed, 10<<30, now1)
	snap1 := &bandwidthSnapshot{consecutiveOver: over1, blockedUntil: blocks1}

	now2 := now1.Add(2 * time.Minute)
	over2, blocks2 := buildHysteresisState(snap1, observed, 10<<30, now2)
	snap2 := &bandwidthSnapshot{consecutiveOver: over2, blockedUntil: blocks2}
	require.Contains(t, blocks2, "AS714")

	current := snap2
	for i := 0; i < 3; i++ {
		current.consecutiveOver, current.blockedUntil = buildHysteresisState(
			current,
			map[string]asnSnapshotEntry{"AS1": {Org: "Other", Bytes: 1 << 30}},
			10<<30,
			now2.Add(time.Duration(i+1)*2*time.Minute),
		)
		require.Contains(t, current.blockedUntil, "AS714",
			"block must persist across zero-bandwidth window %d", i+1)
	}

	past := now2.Add(blockCooldown + time.Minute)
	_, blocksAfter := buildHysteresisState(current, map[string]asnSnapshotEntry{}, 10<<30, past)
	assert.NotContains(t, blocksAfter, "AS714", "expired block must be pruned")
}

func TestHysteresis_BlockRenewsOnRepeatedObservation(t *testing.T) {
	resetASNBandwidthState(t)
	now := time.Unix(1_700_000_000, 0)
	until := now.Add(blockCooldown - time.Minute) // already partway through
	prev := &bandwidthSnapshot{
		consecutiveOver: map[string]int{"AS714": 2},
		blockedUntil: map[string]asnBlockEntry{
			"AS714": {Until: until, Org: "Apple Inc."},
		},
	}
	observed := map[string]asnSnapshotEntry{
		"AS714": {Org: "Apple Inc.", Bytes: 100 << 30},
	}
	now2 := now.Add(2 * time.Minute)
	_, blocks := buildHysteresisState(prev, observed, 10<<30, now2)

	require.Contains(t, blocks, "AS714")
	assert.Equal(t, now2.Add(blockCooldown), blocks["AS714"].Until,
		"block should be renewed to now+blockCooldown, not kept at the stale timestamp")
}

func TestCheckLimit_NoSnapshotReturnsNotBlocked(t *testing.T) {
	resetASNBandwidthState(t)
	blocked, _, _ := CheckASNBandwidthLimit("1.2.3.4")
	assert.False(t, blocked)
}

func TestCheckLimit_BlockedSnapshotReturnsBlocked(t *testing.T) {
	resetASNBandwidthState(t)
	clk := withFakeClock(t, time.Unix(1_700_000_000, 0))

	_ = ipToASNCache.Set("1.2.3.4", asnCacheEntry{asn: 714, org: "Apple Inc."})
	completedBandwidthSnapshot.Store(&bandwidthSnapshot{
		blockedUntil: map[string]asnBlockEntry{
			"AS714": {Until: clk.Now().Add(5 * time.Minute), Org: "Apple Inc."},
		},
	})

	blocked, asn, org := CheckASNBandwidthLimit("1.2.3.4")
	assert.True(t, blocked)
	assert.Equal(t, 714, asn)
	assert.Equal(t, "Apple Inc.", org)
}

func TestCheckLimit_ExpiredBlockReturnsNotBlocked(t *testing.T) {
	resetASNBandwidthState(t)
	clk := withFakeClock(t, time.Unix(1_700_000_000, 0))

	_ = ipToASNCache.Set("1.2.3.4", asnCacheEntry{asn: 714, org: "Apple Inc."})
	completedBandwidthSnapshot.Store(&bandwidthSnapshot{
		blockedUntil: map[string]asnBlockEntry{
			"AS714": {Until: clk.Now().Add(-time.Minute), Org: "Apple Inc."},
		},
	})

	blocked, _, _ := CheckASNBandwidthLimit("1.2.3.4")
	assert.False(t, blocked)
}

func TestCheckLimit_LiveKillSwitchOverridesStoredBlock(t *testing.T) {
	resetASNBandwidthState(t)
	clk := withFakeClock(t, time.Unix(1_700_000_000, 0))

	_ = ipToASNCache.Set("1.2.3.4", asnCacheEntry{asn: 714, org: "Apple Inc."})
	completedBandwidthSnapshot.Store(&bandwidthSnapshot{
		blockedUntil: map[string]asnBlockEntry{
			"AS714": {Until: clk.Now().Add(5 * time.Minute), Org: "Apple Inc."},
		},
	})

	SetASNBandwidthMultiplier(0)
	blocked, _, _ := CheckASNBandwidthLimit("1.2.3.4")
	assert.False(t, blocked, "multiplier=0 must short-circuit enforcement immediately")

	blockedASNs := GetBlockedASNs()
	require.Len(t, blockedASNs, 1, "stored block must still be visible via GetBlockedASNs")
	assert.Equal(t, "AS714", blockedASNs[0].ASN)

	SetASNBandwidthMultiplier(10)
	blocked, _, _ = CheckASNBandwidthLimit("1.2.3.4")
	assert.True(t, blocked, "restoring multiplier must re-enforce stored block")
}

func TestCheckLimit_WhitelistedIP(t *testing.T) {
	resetASNBandwidthState(t)
	completedBandwidthSnapshot.Store(&bandwidthSnapshot{
		blockedUntil: map[string]asnBlockEntry{
			"AS714": {Until: time.Now().Add(5 * time.Minute), Org: "Apple Inc."},
		},
	})

	blocked, _, _ := CheckASNBandwidthLimit("51.210.0.171")
	assert.False(t, blocked, "whitelisted IPs bypass the limiter")
}

func trackBytes(asnKey, org string, bytes int64) {
	asnMap := bandwidthPerASN.Load()
	asnMap.Compute(asnKey, func(oldVal *asnBandwidth, exists bool) (*asnBandwidth, bool) {
		if exists {
			oldVal.bytes += bytes
			return oldVal, false
		}
		return &asnBandwidth{org: org, bytes: bytes}, false
	})
}

func seedOtherASNs() {
	for i := 0; i < minASNCount; i++ {
		trackBytes("ASother"+string(rune('A'+i)), "Other", 1<<30)
	}
}

func TestReporter_SteadyOverThresholdIsConsistentlyBlocked(t *testing.T) {
	resetASNBandwidthState(t)
	clk := withFakeClock(t, time.Unix(1_700_000_000, 0))
	_ = ipToASNCache.Set("17.0.0.1", asnCacheEntry{asn: 714, org: "Apple Inc."})

	seedOtherASNs()
	trackBytes("AS714", "Apple Inc.", 100<<30)
	logBandwidthReport()
	clk.Advance(asnBandwidthReportInterval)

	blocked, _, _ := CheckASNBandwidthLimit("17.0.0.1")
	assert.False(t, blocked, "one over-threshold window must not block (hysteresis)")

	seedOtherASNs()
	trackBytes("AS714", "Apple Inc.", 100<<30)
	logBandwidthReport()

	blocked, _, _ = CheckASNBandwidthLimit("17.0.0.1")
	assert.True(t, blocked, "second over-threshold window must trigger block")

	for i := 0; i < 3; i++ {
		clk.Advance(asnBandwidthReportInterval)
		seedOtherASNs()
		logBandwidthReport()

		blocked, _, _ = CheckASNBandwidthLimit("17.0.0.1")
		require.True(t, blocked, "block must persist through zero-bandwidth window %d (oscillation regression)", i+1)
	}

	clk.Advance(blockCooldown + time.Minute)
	seedOtherASNs()
	logBandwidthReport()

	blocked, _, _ = CheckASNBandwidthLimit("17.0.0.1")
	assert.False(t, blocked, "block must expire after blockCooldown elapses")
}

func TestReporter_WarmupDoesNotPreBlock(t *testing.T) {
	resetASNBandwidthState(t)
	ResetASNBandwidthWarmup()
	withFakeClock(t, time.Unix(1_700_000_000, 0))
	_ = ipToASNCache.Set("17.0.0.1", asnCacheEntry{asn: 714, org: "Apple Inc."})

	for i := 0; i < defaultWarmupCycles; i++ {
		seedOtherASNs()
		trackBytes("AS714", "Apple Inc.", 1000<<30)
		logBandwidthReport()
		assert.Empty(t, GetBlockedASNs(), "no blocks must accumulate during warmup cycle %d", i)
	}

	seedOtherASNs()
	trackBytes("AS714", "Apple Inc.", 1000<<30)
	logBandwidthReport()
	blocked, _, _ := CheckASNBandwidthLimit("17.0.0.1")
	assert.False(t, blocked, "first post-warmup over-threshold window must still respect hysteresis")
}

func TestReporter_BelowMinASNCountDoesNotBlock(t *testing.T) {
	resetASNBandwidthState(t)
	clk := withFakeClock(t, time.Unix(1_700_000_000, 0))
	_ = ipToASNCache.Set("17.0.0.1", asnCacheEntry{asn: 714, org: "Apple Inc."})

	for i := 0; i < 5; i++ {
		trackBytes("AS714", "Apple Inc.", 1000<<30)
		logBandwidthReport()
		clk.Advance(asnBandwidthReportInterval)
	}

	assert.Empty(t, GetBlockedASNs(), "threshold<=0 (minASNCount) must never create blocks")
	blocked, _, _ := CheckASNBandwidthLimit("17.0.0.1")
	assert.False(t, blocked)
}

func TestDebugSurface_ThresholdAndOverReadFromSnapshot(t *testing.T) {
	resetASNBandwidthState(t)
	withFakeClock(t, time.Unix(1_700_000_000, 0))

	completedBandwidthSnapshot.Store(&bandwidthSnapshot{
		entries: map[string]asnSnapshotEntry{
			"AS714": {Org: "Apple Inc.", Bytes: 100 << 30},
			"AS1":   {Org: "Other", Bytes: 1 << 30},
		},
		threshold:    10 << 30,
		blockedUntil: map[string]asnBlockEntry{},
	})

	assert.Equal(t, int64(10)<<30, GetASNBandwidthThreshold())
	assert.Equal(t, []string{"AS714"}, GetASNsOverThreshold())
}

func TestDebugSurface_SnapshotViewReportsFullState(t *testing.T) {
	resetASNBandwidthState(t)
	clk := withFakeClock(t, time.Unix(1_700_000_000, 0))

	completedBandwidthSnapshot.Store(&bandwidthSnapshot{
		entries: map[string]asnSnapshotEntry{
			"AS714": {Org: "Apple Inc.", Bytes: 100 << 30},
			"AS1":   {Org: "Other", Bytes: 1 << 30},
		},
		consecutiveOver: map[string]int{"AS714": 2},
		blockedUntil: map[string]asnBlockEntry{
			"AS714": {Until: clk.Now().Add(5 * time.Minute), Org: "Apple Inc."},
		},
		threshold:   10 << 30,
		windowStart: clk.Now().Add(-2 * time.Minute),
		windowEnd:   clk.Now(),
	})

	view := GetBandwidthSnapshotView()
	require.NotNil(t, view)
	assert.Equal(t, int64(10)<<30, view.Threshold)
	assert.Equal(t, clk.Now(), view.WindowEnd)
	require.Len(t, view.Entries, 2)
	assert.Equal(t, "AS714", view.Entries[0].ASN, "entries must be sorted by bytes desc")
	assert.Equal(t, []string{"AS714"}, view.OverThreshold)
	assert.Equal(t, 2, view.ConsecutiveOver["AS714"])
	require.Len(t, view.BlockedASNs, 1)
	assert.Equal(t, "AS714", view.BlockedASNs[0].ASN)
}

func TestDebugSurface_SnapshotViewNilBeforeFirstTick(t *testing.T) {
	resetASNBandwidthState(t)
	assert.Nil(t, GetBandwidthSnapshotView())
}

func TestDebugSurface_BlockedASNsIgnoresKillSwitch(t *testing.T) {
	resetASNBandwidthState(t)
	clk := withFakeClock(t, time.Unix(1_700_000_000, 0))

	completedBandwidthSnapshot.Store(&bandwidthSnapshot{
		blockedUntil: map[string]asnBlockEntry{
			"AS714": {Until: clk.Now().Add(5 * time.Minute), Org: "Apple Inc."},
		},
		threshold: 10 << 30,
	})

	SetASNBandwidthMultiplier(0)

	blocked := GetBlockedASNs()
	require.Len(t, blocked, 1, "GetBlockedASNs must report stored state regardless of kill-switch")
	assert.Equal(t, "AS714", blocked[0].ASN)
	assert.Equal(t, "Apple Inc.", blocked[0].Org)
}
