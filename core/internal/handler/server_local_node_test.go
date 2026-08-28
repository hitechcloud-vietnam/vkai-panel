package handler

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

// The local node is added to the servers API rather than given an API of its
// own, so what has to be proved here is that nothing already there moved.

func TestAServerIsSerialisedExactlyAsBeforeWhenItIsNotThePanelHost(t *testing.T) {
	id := uuid.New()
	server := models.Server{ID: id, Hostname: "web-01", IPAddress: "203.0.113.4", CPUCores: 2}

	plain, err := json.Marshal(server)
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := json.Marshal(serverView{Server: &server})
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != string(wrapped) {
		t.Fatalf("wrapping a server changed its JSON:\n before: %s\n  after: %s", plain, wrapped)
	}
}

func TestThePanelHostCarriesItsLocalNodeAlongsideTheUnchangedServerFields(t *testing.T) {
	id := uuid.New()
	server := models.Server{ID: id, Hostname: "panel-host", IPAddress: "203.0.113.4"}
	view := serverView{
		Server:    &server,
		LocalNode: &service.LocalNodeStatus{Registered: true, NodeID: id, Local: true, Verified: true, Healthy: true},
	}

	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	// The server's own fields stay where a client already looks for them.
	for _, key := range []string{"id", "hostname", "ip_address", "agent_status", "created_at"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("the server field %q disappeared from the response", key)
		}
	}
	local, ok := decoded["local_node"]
	if !ok {
		t.Fatal("the panel host carried no local_node object")
	}
	var status service.LocalNodeStatus
	if err := json.Unmarshal(local, &status); err != nil {
		t.Fatal(err)
	}
	if !status.Local || !status.Healthy || status.NodeID != id {
		t.Fatalf("local_node=%+v", status)
	}
}

func TestAPanelWithNoLocalNodeStillDescribesItsServers(t *testing.T) {
	// A zero-value service is one with no repository and no probe: the wiring
	// a unit test has, and the wiring a panel has before its database is up.
	handler := NewServerHandler(&service.ServerService{}, nil)
	if handler.logger == nil {
		t.Fatal("the handler was left without a logger to report with")
	}
}
