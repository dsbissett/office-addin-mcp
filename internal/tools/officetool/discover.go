package officetool

import (
	"context"
	"encoding/json"
	"errors"
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
		return tools.Result{
			Err:     &tools.EnvelopeError{Code: "attach_failed", Message: err.Error(), Category: tools.CategoryNotFound},
			Summary: hostLabel + " attach failed: " + err.Error(),
		}
	}
	exec := officejs.New(att.Conn, att.SessionID)
	rawResult, err := exec.Run(ctx, payload, map[string]any{})
	if err != nil {
		return classifyDiscoverErr(err, hostLabel)
	}

	var head struct {
		FilePath    string `json:"filePath"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(rawResult, &head); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_discover", err.Error(), false)
	}

	cache := env.DocCache
	cached, hit := cache.Get(host, head.FilePath)
	if !force && hit && cached.Fingerprint == head.Fingerprint {
		var data any
		if err := json.Unmarshal(cached.Data, &data); err != nil {
			return tools.Fail(tools.CategoryInternal, "decode_cached", err.Error(), false)
		}
		return tools.OKWithSummary(
			fmt.Sprintf("%s discovery cache hit (%s).", hostLabel, head.FilePath),
			withCacheMeta(data, head.FilePath, head.Fingerprint, true),
		)
	}

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

func classifyDiscoverErr(err error, hostLabel string) tools.Result {
	var oerr *officejs.OfficeError
	if errors.As(err, &oerr) {
		details := map[string]any{}
		if len(oerr.DebugInfo) > 0 {
			var di any
			if json.Unmarshal(oerr.DebugInfo, &di) == nil {
				details["debugInfo"] = di
			}
		}
		code := oerr.Code
		if code == "" {
			code = "office_js_error"
		}
		res := tools.FailWithDetails(tools.CategoryOfficeJS, code, oerr.Message, false, details)
		res.Summary = "Office.js error: " + oerr.Message
		return res
	}
	var pe *officejs.ProtocolException
	if errors.As(err, &pe) {
		return tools.Result{
			Err:     &tools.EnvelopeError{Code: "payload_protocol_exception", Message: pe.Text, Category: tools.CategoryProtocol},
			Summary: "Payload protocol exception: " + pe.Text,
		}
	}
	res := tools.ClassifyCDPErr("payload_failed", err)
	res.Summary = hostLabel + " payload failed: " + err.Error()
	return res
}
