package pacaroute

import (
	"strings"
	"testing"
	"time"
)

// Usage arrives at the top level or inside one response/message envelope.
// Deeper upstream nesting must not multiply parsing work for a single frame.
func TestUsageObserveNestingIsLinear(t *testing.T) {
	u := Usage{}
	u.observe([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":7,"output_tokens":3}}}`))
	u.observe([]byte(`{"type":"message_start","message":{"usage":{"cache_read_input_tokens":2}}}`))
	if u.Input == nil || *u.Input != 7 || u.Output == nil || *u.Output != 3 || u.CacheRead == nil || *u.CacheRead != 2 {
		t.Fatalf("envelope usage not observed: %+v", u)
	}
	pad := strings.Repeat("x", 1<<20)
	flat := []byte(`{"response":{"pad":"` + pad + `"}}`)
	nested := []byte(strings.Repeat(`{"response":`, 1600) + `{"pad":"` + pad + `"}` + strings.Repeat("}", 1600))
	start := time.Now()
	(&Usage{}).observe(flat)
	base := time.Since(start)
	start = time.Now()
	(&Usage{}).observe(nested)
	if elapsed := time.Since(start); elapsed > 20*base+500*time.Millisecond {
		t.Fatalf("observing one %d KiB nested frame took %v (flat frame %v)", len(nested)>>10, elapsed, base)
	}
}
