package expirationwatch

import (
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/ocdogan/rbt"
	log "github.com/sirupsen/logrus"
)

// ExpiredItem represents an expired item returned from the Watcher
type ExpiredItem struct {
	ExpirationTimeSeconds int64
	ID                    string
}

// Watcher watches the expiration of items
type Watcher struct {
	expiredItems     chan []ExpiredItem
	rbTreeMu         sync.RWMutex
	rbTree           *rbt.RbTree
	expirationBuffer time.Duration
	ticker           *time.Ticker
	isWatching       bool
	wasStartedOnce   bool
	mu               sync.Mutex
}

// New instantiates a new expiration watcher. An expiration buffer (positive or negative) can
// be specified, causing items to be deemed expired some time before or after their expiration reaches current
// UTC time. A positive expirationBuffer will make the item expire sooner then UTC, and a negative buffer after.
func New(expirationBuffer time.Duration) *Watcher {
	rbTree := rbt.NewRbTree()
	return &Watcher{
		expiredItems:     make(chan []ExpiredItem, 10),
		rbTree:           rbTree,
		expirationBuffer: expirationBuffer,
	}
}

// Add adds a new item identified by an ID to the expiration watcher
func (w *Watcher) Add(expirationTimeSeconds int64, id string) {
	key := rbt.Int64Key(expirationTimeSeconds)
	w.rbTreeMu.Lock()
	defer w.rbTreeMu.Unlock()
	value, ok := w.rbTree.Get(&key)
	if !ok {
		w.rbTree.Insert(&key, map[string]struct{}{id: struct{}{}})
	} else {
		ids := value.(map[string]struct{})
		ids[id] = struct{}{}
		w.rbTree.Insert(&key, ids)
	}
}

// Remove removes the item with a specified id from the expiration watcher
func (w *Watcher) Remove(expirationTimeSeconds int64, id string) {
	key := rbt.Int64Key(expirationTimeSeconds)
	w.rbTreeMu.Lock()
	defer w.rbTreeMu.Unlock()
	value, ok := w.rbTree.Get(&key)
	if !ok {
		// Due to the asynchronous nature of the Watcher and OrderWatcher, there are
		// race-conditions where we try to remove an item from the Watcher after it
		// has already been removed.
		log.WithFields(log.Fields{
			"id": id,
		}).Trace("Attempted to remove item from Watcher that no longer exists")
		return // Noop
	} else {
		ids := value.(map[string]struct{})
		delete(ids, id)
		if len(ids) == 0 {
			w.rbTree.Delete(&key)
		} else {
			w.rbTree.Insert(&key, ids)
		}
	}
}

// Start starts the expiration watchers poller
func (w *Watcher) Start(pollingInterval time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.isWatching {
		return errors.New("Expiration watcher already started")
	}
	if w.wasStartedOnce {
		return errors.New("Can only start Watcher once per instance")
	}
	w.isWatching = true
	w.wasStartedOnce = true

	go func() {
		// TODO(fabio): Optimize this poller. We could keep track of soonestExpirationTime as a property of
		// Watcher. Whenever a new item is added via Add, we check if the expiration time is sooner
		// than soonestExpirationTime and if so, we update soonestExpirationTime. Then instead of running the
		// inner for loop at a constant frequency, we adjust the frequency based on the value of
		// soonestExpirationTime (probably by using time.After or time.Sleep).
		ticker := time.NewTicker(pollingInterval)
		for {
			<-ticker.C

			w.mu.Lock()
			if !w.isWatching {
				ticker.Stop()
				close(w.expiredItems)
				w.mu.Unlock()
				return
			}
			w.mu.Unlock()

			w.expiredItems <- w.prune()
		}
	}()
	return nil
}

// Stop stops the expiration watchers poller
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.isWatching = false
}

// Receive returns a read-only channel that can be used to listen for expired items
func (w *Watcher) Receive() <-chan []ExpiredItem {
	return w.expiredItems
}

// prune checks for any expired items, removes them from the expiration watcher and returns them
// to the caller
func (w *Watcher) prune() []ExpiredItem {
	pruned := []ExpiredItem{}
	for {
		w.rbTreeMu.RLock()
		key, value := w.rbTree.Min()
		w.rbTreeMu.RUnlock()
		if key == nil {
			break
		}
		expirationTimeSeconds := reflect.ValueOf(key).Elem().Int()
		currentTimePlusBuffer := time.Now().Add(w.expirationBuffer)
		if expirationTimeSeconds > currentTimePlusBuffer.Unix() {
			break
		}
		ids := value.(map[string]struct{})
		for id := range ids {
			pruned = append(pruned, ExpiredItem{
				ExpirationTimeSeconds: expirationTimeSeconds,
				ID:                    id,
			})
		}
		w.rbTreeMu.Lock()
		w.rbTree.Delete(key)
		w.rbTreeMu.Unlock()
	}
	return pruned
}
