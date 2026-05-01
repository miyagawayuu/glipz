package httpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestValidTimelineSettingsPayload(t *testing.T) {
	timelines := func(n int) json.RawMessage {
		items := make([]string, 0, n)
		for i := 0; i < n; i++ {
			items = append(items, fmt.Sprintf(`{"id":"custom-%d"}`, i))
		}
		return json.RawMessage(`{"version":1,"defaultTimelineId":"all","timelines":[` + strings.Join(items, ",") + `]}`)
	}

	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "valid minimum", raw: json.RawMessage(`{"timelines":[{"id":"all"}]}`), want: true},
		{name: "valid upper bound", raw: timelines(64), want: true},
		{name: "empty", raw: nil, want: false},
		{name: "invalid json", raw: json.RawMessage(`{"timelines":[`), want: false},
		{name: "missing timelines", raw: json.RawMessage(`{"version":1}`), want: false},
		{name: "timelines not array", raw: json.RawMessage(`{"timelines":{}}`), want: false},
		{name: "no timelines", raw: json.RawMessage(`{"timelines":[]}`), want: false},
		{name: "too many timelines", raw: timelines(65), want: false},
		{name: "too large", raw: json.RawMessage(`{"timelines":[{"id":"all"}],"pad":"` + strings.Repeat("x", maxTimelineSettingsBytes) + `"}`), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validTimelineSettingsPayload(tt.raw); got != tt.want {
				t.Fatalf("validTimelineSettingsPayload() = %v, want %v", got, tt.want)
			}
		})
	}
}
