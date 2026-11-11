package api

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// GetMultipleClients merges the main, ping, labels and hunts
// documents in hit order, so the main record may not come first.
func TestMergeClientRecordsKeepsFqdn(t *testing.T) {
	merged := &ClientRecord{ClientId: "C.1234", Ping: 10, Type: "ping"}

	mergeClientRecords(merged, &ClientRecord{
		ClientId: "C.1234",
		Hostname: "devbox",
		Fqdn:     "devbox.corp.example.com",
		Type:     "main",
	})
	mergeClientRecords(merged, &ClientRecord{
		ClientId: "C.1234",
		Labels:   []string{"Prod"},
	})

	assert.Equal(t, "devbox", merged.Hostname)
	assert.Equal(t, "devbox.corp.example.com", merged.Fqdn)
	assert.Equal(t, uint64(10), merged.Ping)
}
