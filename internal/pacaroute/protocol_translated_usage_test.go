package pacaroute

import (
	"testing"

	"github.com/LandoWang/pacaroute/internal/pacaroute/usage"
)

// A translated non-streaming Messages response uses the same usage mapping as the
// translated stream, so Messages observers (for example a downstream PacaRoute
// peer) count cached Responses input once. Absent usage stays absent.
func TestTranslatedMessagesJSONUsageCountsCacheOnce(t *testing.T) {
	tr := &translation{from: "messages", to: "responses", model: "coding", toolNames: map[string]toolIdentity{}}
	body, err := tr.responseJSON([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":80},"output_tokens":5,"total_tokens":105}}`))
	if err != nil {
		t.Fatal(err)
	}
	u := usage.Usage{}
	u.Observe(body)
	if v := u.TotalInput(true); v == nil || *v != 100 || u.Output == nil || *u.Output != 5 {
		t.Fatalf("Messages observer of translated response saw usage %s", obj(body)["usage"])
	}
	for _, upstream := range []string{`{"status":"completed","output":[]}`, `{"status":"completed","output":[],"usage":null}`} {
		body, err = tr.responseJSON([]byte(upstream))
		if err != nil {
			t.Fatal(err)
		}
		if usage := string(obj(body)["usage"]); usage != "null" {
			t.Fatalf("missing upstream usage became %s", usage)
		}
	}
}
