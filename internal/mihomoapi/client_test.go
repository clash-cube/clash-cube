package mihomoapi

import (
	"encoding/json"
	"testing"
)

func TestConnectionPreservesCoreRecord(t *testing.T) {
	data := `{"id":"one","metadata":{"sourcePort":"1234","inboundName":"mixed","dnsMode":"fake-ip","futureField":[1,2]},"providerChains":["provider"],"futureField":{"enabled":true}}`
	var c Connection
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		t.Fatal(err)
	}
	if c.Metadata.InboundName != "mixed" || c.Metadata.SourcePort != "1234" {
		t.Fatalf("metadata not decoded: %+v", c.Metadata)
	}
	// The GUI transport must retain unknown fields without adding UI counters
	// or recursively embedding the transport's own rawJSON field.
	wire, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var received struct {
		RawJSON string `json:"rawJSON"`
	}
	if err := json.Unmarshal(wire, &received); err != nil {
		t.Fatal(err)
	}
	if received.RawJSON != data {
		t.Fatalf("lost original record: %s", received.RawJSON)
	}
}
