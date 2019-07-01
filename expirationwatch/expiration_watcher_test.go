package expirationwatch

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrunesExpiredItems(t *testing.T) {
	watcher := New(0)

	current := time.Now().Unix()
	expiryEntryOne := ExpiredItem{
		ExpirationTimeSeconds: current - 3,
		ID:                    "0x8e209dda7e515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d97385e",
	}
	watcher.Add(expiryEntryOne.ExpirationTimeSeconds, expiryEntryOne.ID)

	expiryEntryTwo := ExpiredItem{
		ExpirationTimeSeconds: current - 1,
		ID:                    "0x12ab7edd34515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d3bee521",
	}
	watcher.Add(expiryEntryTwo.ExpirationTimeSeconds, expiryEntryTwo.ID)

	pruned := watcher.prune()
	assert.Len(t, pruned, 2, "Pruned the expired item")
	assert.Equal(t, expiryEntryOne, pruned[0])
	assert.Equal(t, expiryEntryTwo, pruned[1])
}

func TestPrunesTwoExpiredItemsWithSameExpiration(t *testing.T) {
	watcher := New(0)

	current := time.Now().Unix()
	expiration := current - 3
	expiryEntryOne := ExpiredItem{
		ExpirationTimeSeconds: expiration,
		ID:                    "0x8e209dda7e515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d97385e",
	}
	watcher.Add(expiryEntryOne.ExpirationTimeSeconds, expiryEntryOne.ID)

	expiryEntryTwo := ExpiredItem{
		ExpirationTimeSeconds: expiration,
		ID:                    "0x12ab7edd34515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d3bee521",
	}
	watcher.Add(expiryEntryTwo.ExpirationTimeSeconds, expiryEntryTwo.ID)

	pruned := watcher.prune()
	assert.Len(t, pruned, 2, "Pruned the expired item")
	hashes := map[string]bool{
		expiryEntryOne.ID: true,
		expiryEntryTwo.ID: true,
	}
	for _, expiredItem := range pruned {
		assert.True(t, hashes[expiredItem.ID])
	}
}

func TestKeepsUnexpiredItem(t *testing.T) {
	watcher := New(0)

	id := "0x8e209dda7e515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d97385e"
	current := time.Now().Unix()
	watcher.Add(current+10, id)

	pruned := watcher.prune()
	assert.Equal(t, 0, len(pruned), "Doesn't prune unexpired item")
}

func TestReturnsEmptyIfNoItems(t *testing.T) {
	watcher := New(0)

	pruned := watcher.prune()
	assert.Len(t, pruned, 0, "Returns empty array when no items tracked")
}

func TestRemoveOnlyItemWithSpecificExpirationTime(t *testing.T) {
	watcher := New(0)

	current := time.Now().Unix()
	expiryEntryOne := ExpiredItem{
		ExpirationTimeSeconds: current - 3,
		ID:                    "0x8e209dda7e515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d97385e",
	}
	watcher.Add(expiryEntryOne.ExpirationTimeSeconds, expiryEntryOne.ID)

	expiryEntryTwo := ExpiredItem{
		ExpirationTimeSeconds: current - 1,
		ID:                    "0x12ab7edd34515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d3bee521",
	}
	watcher.Add(expiryEntryTwo.ExpirationTimeSeconds, expiryEntryTwo.ID)

	watcher.Remove(expiryEntryTwo.ExpirationTimeSeconds, expiryEntryTwo.ID)

	pruned := watcher.prune()
	assert.Len(t, pruned, 1, "Pruned the expired item")
	assert.Equal(t, expiryEntryOne, pruned[0])
}
func TestRemoveItemWhichSharesExpirationTimeWithOtherItems(t *testing.T) {
	watcher := New(0)

	current := time.Now().Unix()
	singleExpirationTimeSeconds := current - 3
	expiryEntryOne := ExpiredItem{
		ExpirationTimeSeconds: singleExpirationTimeSeconds,
		ID:                    "0x8e209dda7e515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d97385e",
	}
	watcher.Add(expiryEntryOne.ExpirationTimeSeconds, expiryEntryOne.ID)

	expiryEntryTwo := ExpiredItem{
		ExpirationTimeSeconds: singleExpirationTimeSeconds,
		ID:                    "0x12ab7edd34515025d0c34aa61a0d1156a631248a4318576a2ce0fb408d3bee521",
	}
	watcher.Add(expiryEntryTwo.ExpirationTimeSeconds, expiryEntryTwo.ID)

	watcher.Remove(expiryEntryTwo.ExpirationTimeSeconds, expiryEntryTwo.ID)

	pruned := watcher.prune()
	assert.Len(t, pruned, 1, "Pruned the expired item")
	assert.Equal(t, expiryEntryOne, pruned[0])
}

func TestStartsAndStopsPoller(t *testing.T) {
	watcher := New(0)

	pollingInterval := 50 * time.Millisecond
	require.NoError(t, watcher.Start(pollingInterval))

	var countMux sync.Mutex
	channelCount := 0
	go func() {
		for {
			select {
			case _, isOpen := <-watcher.Receive():
				if !isOpen {
					return
				}
				countMux.Lock()
				channelCount++
				countMux.Unlock()
			}
		}
	}()

	expectedIsWatching := true
	assert.Equal(t, expectedIsWatching, watcher.isWatching)

	time.Sleep(60 * time.Millisecond)
	watcher.Stop()
	expectedIsWatching = false
	assert.Equal(t, expectedIsWatching, watcher.isWatching)

	countMux.Lock()
	expectedChannelCount := 1
	assert.Equal(t, expectedChannelCount, channelCount)
	countMux.Unlock()
}
