package identity

import (
	"encoding/json"
	"testing"
	"time"
)

// Signing in as an administrator hung until the server's write timeout fired
// and the proxy answered 502, while pharmacy and vendor accounts signed in
// normally. The cause was a token set that grew forever and was walked one
// Redis round trip at a time. These tests pin the two properties that fix it.

func blobFor(t *testing.T, userID int64, created time.Time) []byte {
	t.Helper()
	raw, err := json.Marshal(Session{UserID: userID, CreatedAt: created})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	return raw
}

// A token whose session is gone must come back as DEAD, so the caller can
// remove it from the set.
//
// This is the leak. The old code loaded each token and skipped the ones that
// failed, and because only tokens that loaded were ever removed, an expired one
// stayed a member of the set permanently — costing every later sign-in two more
// round trips to rediscover, forever.
func TestExpiredTokensAreReportedDeadSoTheSetCanBePruned(t *testing.T) {
	now := time.Now()
	tokens := []string{"live-a", "expired-b", "live-c", "expired-d", "corrupt-e"}
	blobs := [][]byte{
		blobFor(t, 1, now.Add(-time.Hour)),
		nil, // key expired
		blobFor(t, 1, now.Add(-time.Minute)),
		nil, // key expired
		[]byte("{not json"),
	}

	live, dead := classifySessions(tokens, blobs)

	if len(live) != 2 {
		t.Fatalf("live = %d, want 2: %#v", len(live), live)
	}
	if len(dead) != 3 {
		t.Fatalf("dead = %d, want 3 (two expired, one corrupt): %#v", len(dead), dead)
	}
	for _, want := range []string{"expired-b", "expired-d", "corrupt-e"} {
		found := false
		for _, d := range dead {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q was not reported dead; it would stay in the set forever", want)
		}
	}
}

// A set that is entirely dead — the state the administrator's account had
// reached — must report every token for pruning and evict nobody.
func TestAnEntirelyDeadSetIsFullyPruned(t *testing.T) {
	tokens := make([]string, 5000)
	blobs := make([][]byte, 5000)
	for i := range tokens {
		tokens[i] = string(rune('a'+i%26)) + "-dead"
	}

	live, dead := classifySessions(tokens, blobs)

	if len(live) != 0 {
		t.Fatalf("live = %d, want 0", len(live))
	}
	if len(dead) != len(tokens) {
		t.Fatalf("dead = %d, want %d — anything left behind keeps the ratchet turning", len(dead), len(tokens))
	}
	if got := evictCount(len(live), 3); got != 0 {
		t.Fatalf("evictCount = %d; a dead set must not evict live sessions", got)
	}
}

// Eviction keeps the newest `max` sessions and never evicts when at or under
// the limit. Getting this wrong signs people out for no reason.
func TestEvictCountBoundaries(t *testing.T) {
	for _, tc := range []struct {
		live, max, want int
	}{
		{live: 0, max: 3, want: 0},
		{live: 3, max: 3, want: 0},
		{live: 4, max: 3, want: 1},
		{live: 10, max: 3, want: 7},
		{live: 5, max: 0, want: 0}, // no limit configured: evict nothing
		{live: 5, max: -1, want: 0},
	} {
		if got := evictCount(tc.live, tc.max); got != tc.want {
			t.Errorf("evictCount(%d, %d) = %d, want %d", tc.live, tc.max, got, tc.want)
		}
	}
}

// Batching must cover every token exactly once. A token dropped here is a
// session that never gets cleaned up or never gets evicted.
func TestChunkCoversEveryTokenExactlyOnce(t *testing.T) {
	for _, n := range []int{0, 1, 499, 500, 501, 1000, 1001, 5003} {
		tokens := make([]string, n)
		for i := range tokens {
			tokens[i] = string(rune(i))
		}

		var seen int
		for _, batch := range chunk(tokens, sessionPipelineBatch) {
			if len(batch) == 0 {
				t.Fatalf("n=%d produced an empty batch", n)
			}
			if len(batch) > sessionPipelineBatch {
				t.Fatalf("n=%d produced a batch of %d, over the cap of %d",
					n, len(batch), sessionPipelineBatch)
			}
			seen += len(batch)
		}
		if seen != n {
			t.Fatalf("n=%d: batches covered %d tokens", n, seen)
		}
	}
}

// The oldest sessions are the ones evicted, so the sort the callers rely on
// must order by creation time and a missing CreatedAt must not sort randomly.
func TestClassifyFallsBackToLastActiveWhenCreatedAtIsMissing(t *testing.T) {
	last := time.Now().Add(-2 * time.Hour)
	raw, err := json.Marshal(Session{UserID: 7, LastActiveAt: last})
	if err != nil {
		t.Fatal(err)
	}

	live, dead := classifySessions([]string{"t"}, [][]byte{raw})
	if len(dead) != 0 || len(live) != 1 {
		t.Fatalf("live=%d dead=%d, want 1/0", len(live), len(dead))
	}
	if live[0].created.IsZero() {
		t.Fatal("created is zero; the eviction order would be arbitrary")
	}
	if !live[0].created.Equal(last) {
		t.Fatalf("created = %v, want the last-active fallback %v", live[0].created, last)
	}
	if live[0].userID != 7 {
		t.Fatalf("userID = %d, want 7 — the org path needs it to clean the user set", live[0].userID)
	}
}
