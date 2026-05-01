package launch

import "testing"

func TestState_PutLookupDelete(t *testing.T) {
	t.Cleanup(func() { defaultRegistry.launches = map[string]*TrackedLaunch{} })

	called := 0
	tl := &TrackedLaunch{
		PID:     1234,
		CDPURL:  "http://127.0.0.1:9222",
		Project: &Project{ManifestPath: "C:/x/manifest.xml"},
	}
	tl.StopFn = func() error { called++; return nil }
	defaultRegistry.put(tl.Project.ManifestPath, tl)

	got, ok := LookupLaunch(tl.Project.ManifestPath)
	if !ok || got != tl {
		t.Fatal("LookupLaunch did not return the tracked launch")
	}

	if list := ListLaunches(); len(list) != 1 || list[0] != tl {
		t.Errorf("ListLaunches = %v, want single entry", list)
	}

	if err := tl.Stop(); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if err := tl.Stop(); err != nil {
		t.Errorf("Stop again: %v", err)
	}
	if called != 1 {
		t.Errorf("StopFn called %d times, want 1 (idempotent)", called)
	}

	defaultRegistry.delete(tl.Project.ManifestPath)
	if _, ok := LookupLaunch(tl.Project.ManifestPath); ok {
		t.Error("LookupLaunch returned a deleted launch")
	}
}

func TestState_StopAll(t *testing.T) {
	t.Cleanup(func() { defaultRegistry.launches = map[string]*TrackedLaunch{} })

	stopped := 0
	for i, mp := range []string{"a", "b", "c"} {
		tl := &TrackedLaunch{PID: 100 + i, Project: &Project{ManifestPath: mp}}
		tl.StopFn = func() error { stopped++; return nil }
		defaultRegistry.put(mp, tl)
	}
	StopAll()
	if stopped != 3 {
		t.Errorf("stopped = %d, want 3", stopped)
	}
	if len(defaultRegistry.launches) != 0 {
		t.Errorf("registry not cleared: %v", defaultRegistry.launches)
	}
}
