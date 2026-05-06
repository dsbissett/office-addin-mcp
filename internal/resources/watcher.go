package resources

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Watcher tracks subscriptions to resources and polls for changes.
// When a resource's fingerprint changes, it notifies via the notify callback.
type Watcher struct {
	mu           sync.RWMutex
	subs         map[string]*subscription
	provider     *Provider
	notify       func(context.Context, string)
	PollInterval time.Duration
}

type subscription struct {
	uri         string
	fingerprint string
	cancel      context.CancelFunc
	done        chan struct{}
}

// NewWatcher creates a new subscription watcher.
func NewWatcher(provider *Provider, notify func(context.Context, string)) *Watcher {
	return &Watcher{
		subs:         make(map[string]*subscription),
		provider:     provider,
		notify:       notify,
		PollInterval: 30 * time.Second,
	}
}

// Subscribe starts polling for changes to a resource URI.
// The first fingerprint is fetched synchronously; subsequent checks run in a background goroutine.
// Returns error if Fingerprint fails.
func (w *Watcher) Subscribe(ctx context.Context, uri string) error {
	// Get the initial fingerprint.
	fp, err := w.provider.Fingerprint(ctx, uri)
	if err != nil {
		return err
	}

	w.mu.Lock()
	// If already subscribed, update fingerprint and return.
	if sub, ok := w.subs[uri]; ok {
		sub.fingerprint = fp
		w.mu.Unlock()
		return nil
	}

	// Create a cancellable context for this subscription's polling goroutine.
	pollCtx, cancel := context.WithCancel(context.Background())

	sub := &subscription{
		uri:         uri,
		fingerprint: fp,
		cancel:      cancel,
		done:        make(chan struct{}),
	}
	w.subs[uri] = sub
	w.mu.Unlock()

	// Start the polling goroutine.
	go w.poll(pollCtx, sub)

	return nil
}

// Unsubscribe stops polling for a resource URI.
func (w *Watcher) Unsubscribe(uri string) {
	w.mu.Lock()
	sub, ok := w.subs[uri]
	if !ok {
		w.mu.Unlock()
		return
	}
	delete(w.subs, uri)
	w.mu.Unlock()

	// Cancel the polling goroutine and wait for it to finish.
	sub.cancel()
	<-sub.done
}

// Close stops all polling goroutines.
func (w *Watcher) Close() {
	w.mu.Lock()
	subs := make([]*subscription, 0, len(w.subs))
	for _, sub := range w.subs {
		subs = append(subs, sub)
	}
	w.subs = make(map[string]*subscription)
	w.mu.Unlock()

	for _, sub := range subs {
		sub.cancel()
		<-sub.done
	}
}

// poll runs in a background goroutine and checks for changes at PollInterval.
func (w *Watcher) poll(ctx context.Context, sub *subscription) {
	defer close(sub.done)

	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fp, err := w.provider.Fingerprint(ctx, sub.uri)
			if err != nil {
				slog.Warn("fingerprint check failed", "uri", sub.uri, "error", err)
				continue
			}

			w.mu.Lock()
			oldFP := sub.fingerprint
			w.mu.Unlock()

			if fp != oldFP {
				slog.Debug("resource changed", "uri", sub.uri)
				w.mu.Lock()
				sub.fingerprint = fp
				w.mu.Unlock()

				// Notify outside the lock.
				if w.notify != nil {
					w.notify(context.Background(), sub.uri)
				}
			}
		}
	}
}
