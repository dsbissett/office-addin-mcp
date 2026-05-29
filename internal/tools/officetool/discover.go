package officetool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// RunDiscover is the shared discover-tool entry point used by every host
// package. It attaches to the target, runs the host's discover payload (which
// must return at least filePath + fingerprint at the top level), then consults
// env.DocCache to decide whether to return the cached snapshot or persist a
// fresh one.
//
// host is the doccache key prefix ("excel", "word", …). payload is the
// embedded JS payload name ("excel.discover" etc.). hostLabel ("Excel" / "Word"
// / …) is embedded in result summaries.
//
// On a fingerprint match (and force=false) the on-disk snapshot is returned
// in place of the live one — meaning a discover-after-discover within the
// session pays one CDP round-trip but costs zero in the agent's context budget
// since the answer is already known.
func RunDiscover(
	ctx context.Context,
	env *tools.RunEnv,
	sel tools.TargetSelector,
	host string,
	payload string,
	force bool,
	hostLabel string,
) tools.Result {
	att, err := env.Attach(ctx, sel)
	if err != nil {
		return discoverAttachErr(err, hostLabel)
	}
	exec := officejs.New(att.Conn, att.SessionID)
	rawResult, err := exec.Run(ctx, payload, map[string]any{})
	if err != nil {
		return classifyDiscoverErr(err, hostLabel)
	}

	var head discoverHead
	if err := json.Unmarshal(rawResult, &head); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_discover", err.Error(), false)
	}

	cache := env.DocCache
	if cached, hit := cache.Get(host, head.FilePath); cacheFresh(cached.Fingerprint, head.Fingerprint, hit, force) {
		return discoverCacheHit(cached, head, hostLabel)
	}
	return discoverRefresh(cache, host, rawResult, head, hostLabel)
}

// discoverHead is the minimal top-level shape every host discover payload must
// return: a stable file identity plus a content fingerprint used for caching.
type discoverHead struct {
	FilePath    string `json:"filePath"`
	Fingerprint string `json:"fingerprint"`
}

// discoverAttachErr maps an env.Attach failure for RunDiscover. RunDiscover has
// always surfaced a plain not_found attach_failed (no deadline/cancellation
// special-casing), so this intentionally does not reuse RunPayload's
// deadline-aware attachErrToResult.
func discoverAttachErr(err error, hostLabel string) tools.Result {
	return tools.Result{
		Err:     &tools.EnvelopeError{Code: "attach_failed", Message: err.Error(), Category: tools.CategoryNotFound},
		Summary: hostLabel + " attach failed: " + err.Error(),
	}
}

// cacheFresh reports whether the cached snapshot may be returned in place of a
// live refresh: caching not forced off, an entry exists, and its fingerprint
// matches the live one.
func cacheFresh(cachedFingerprint, liveFingerprint string, hit, force bool) bool {
	return !force && hit && cachedFingerprint == liveFingerprint
}

// discoverCacheHit returns the on-disk snapshot decorated with cache metadata.
func discoverCacheHit(cached doccache.Entry, head discoverHead, hostLabel string) tools.Result {
	var data any
	if err := json.Unmarshal(cached.Data, &data); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_cached", err.Error(), false)
	}
	return tools.OKWithSummary(
		fmt.Sprintf("%s discovery cache hit (%s).", hostLabel, head.FilePath),
		withCacheMeta(data, head.FilePath, head.Fingerprint, true),
	)
}

// discoverRefresh persists the live snapshot and returns it. A cache-write
// failure is non-fatal: the live data is still returned with a note in the
// summary, matching the pre-refactor behavior.
func discoverRefresh(cache *doccache.Store, host string, rawResult json.RawMessage, head discoverHead, hostLabel string) tools.Result {
	if err := cache.Put(doccache.Entry{
		Host:        host,
		FilePath:    head.FilePath,
		Fingerprint: head.Fingerprint,
		Data:        rawResult,
	}); err != nil {
		var data any
		_ = json.Unmarshal(rawResult, &data)
		return tools.OKWithSummary(
			fmt.Sprintf("%s discovery refreshed (%s); cache write failed: %v.", hostLabel, head.FilePath, err),
			withCacheMeta(data, head.FilePath, head.Fingerprint, false),
		)
	}
	var data any
	if err := json.Unmarshal(rawResult, &data); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_discover", err.Error(), false)
	}
	return tools.OKWithSummary(
		fmt.Sprintf("%s discovery refreshed (%s).", hostLabel, head.FilePath),
		withCacheMeta(data, head.FilePath, head.Fingerprint, false),
	)
}

func withCacheMeta(data any, filePath, fingerprint string, cached bool) map[string]any {
	out := map[string]any{}
	if m, ok := data.(map[string]any); ok {
		for k, v := range m {
			out[k] = v
		}
	} else {
		out["data"] = data
	}
	out["cached"] = cached
	out["filePath"] = filePath
	out["fingerprint"] = fingerprint
	return out
}

// classifyDiscoverErr maps a discover-payload error to a tools.Result. The
// classification is identical to RunPayload's, so it delegates to the shared
// payloadErrToResult helper to keep the two envelopes byte-for-byte aligned.
func classifyDiscoverErr(err error, hostLabel string) tools.Result {
	return payloadErrToResult(err, hostLabel)
}
