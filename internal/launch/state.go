package launch

import (
	"sync"
)

// TrackedLaunch holds a live office-addin-debugging child process plus the
// optional dev-server child it spawned and the metadata needed to stop them.
type TrackedLaunch struct {
	Project   *Project
	CDPURL    string
	PID       int
	Launcher  string // resolved path or shell command for office-addin-debugging
	StopFn    func() error
	stopOnce  sync.Once
	stopErr   error
	devServer *devServerHandle
}

// Stop runs the launcher's stop sequence at most once. Subsequent calls
// return the same error.
func (t *TrackedLaunch) Stop() error {
	t.stopOnce.Do(func() {
		if t.StopFn != nil {
			t.stopErr = t.StopFn()
		}
	})
	return t.stopErr
}

// stateRegistry tracks live launches for the lifetime of the process. It is
// keyed by manifest path so a repeated addin.launch call returns the existing
// launch rather than spawning a duplicate Excel window.
type stateRegistry struct {
	mu       sync.Mutex
	launches map[string]*TrackedLaunch
}

var defaultRegistry = &stateRegistry{launches: map[string]*TrackedLaunch{}}

// LookupLaunch returns the active launch for a manifest path, if any.
func LookupLaunch(manifestPath string) (*TrackedLaunch, bool) {
	return defaultRegistry.lookup(manifestPath)
}

// ListLaunches returns a snapshot of every tracked launch.
func ListLaunches() []*TrackedLaunch {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()
	out := make([]*TrackedLaunch, 0, len(defaultRegistry.launches))
	for _, l := range defaultRegistry.launches {
		out = append(out, l)
	}
	return out
}

// StopAll stops every tracked launch. Errors are collected per-launch but the
// caller only sees that some failed; the launches are removed from the
// registry regardless so the next addin.launch can succeed.
func StopAll() {
	defaultRegistry.mu.Lock()
	victims := make([]*TrackedLaunch, 0, len(defaultRegistry.launches))
	for _, l := range defaultRegistry.launches {
		victims = append(victims, l)
	}
	defaultRegistry.launches = map[string]*TrackedLaunch{}
	defaultRegistry.mu.Unlock()

	for _, l := range victims {
		_ = l.Stop()
	}
}

func (r *stateRegistry) lookup(manifestPath string) (*TrackedLaunch, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.launches[manifestPath]
	return l, ok
}

func (r *stateRegistry) put(manifestPath string, t *TrackedLaunch) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.launches[manifestPath] = t
}

func (r *stateRegistry) delete(manifestPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.launches, manifestPath)
}
