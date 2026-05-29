package launch

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ProbeResult records the outcome of a /json/version probe.
type ProbeResult struct {
	OK      bool
	Version string
	Reason  string
}

// ProbeCDPEndpoint issues GET {url}/json/version with a bounded timeout. Never
// returns an error — always populates a ProbeResult so callers can poll in a
// loop without needing to discriminate transient vs. permanent failures.
func ProbeCDPEndpoint(ctx context.Context, url string, timeout time.Duration) ProbeResult {
	endpoint := strings.TrimRight(url, "/") + "/json/version"
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, fail := sendVersionRequest(probeCtx, endpoint)
	if fail != nil {
		return *fail
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProbeResult{Reason: fmt.Sprintf("http-error:%d", resp.StatusCode)}
	}
	return decodeVersionBody(resp)
}

// sendVersionRequest issues GET endpoint within probeCtx. On success it returns
// the response and a nil fail. On any transport-level failure it returns a
// non-nil fail ProbeResult (and a nil response) describing the reason.
func sendVersionRequest(probeCtx context.Context, endpoint string) (*http.Response, *ProbeResult) {
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &ProbeResult{Reason: fmt.Sprintf("invalid-request:%s", err)}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if probeCtx.Err() == context.DeadlineExceeded {
			return nil, &ProbeResult{Reason: "timeout"}
		}
		return nil, &ProbeResult{Reason: "unreachable"}
	}
	return resp, nil
}

// decodeVersionBody parses a /json/version response, requiring a non-empty
// Browser field. A malformed or empty body yields an invalid-response result.
func decodeVersionBody(resp *http.Response) ProbeResult {
	var body struct {
		Browser string `json:"Browser"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ProbeResult{Reason: "invalid-response"}
	}
	if body.Browser == "" {
		return ProbeResult{Reason: "invalid-response"}
	}
	return ProbeResult{OK: true, Version: body.Browser}
}

// IsPortListening returns true if a TCP connection to 127.0.0.1:port can be
// established within timeout. Used to detect a running dev server before
// spawning one.
func IsPortListening(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
