package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// The census is what makes the deprecation visible. These tests fix what it
// counts, because the number it prints at every boot is what decides when
// servers.agent_token can be dropped.

func TestTheCensusCountsAServerAsMigratedOnlyWhenItsTokenCannotAuthenticate(t *testing.T) {
	summary := summariseAgentChannels([]repository.AgentChannelRow{
		{ServerID: "1", Hostname: "old-node"},
		{ServerID: "2", Hostname: "enrolled-but-token-live", Channel: agentpki.ChannelMutualTLS},
		{ServerID: "3", Hostname: "finished", Channel: agentpki.ChannelMutualTLS, Retired: true},
		{ServerID: "4", Hostname: "created-after-the-upgrade", Retired: true},
		{ServerID: "5"},
	})
	if summary.Total != 5 {
		t.Fatalf("total=%d, want 5", summary.Total)
	}
	if summary.StaticToken != 2 {
		t.Fatalf("static_token=%d, want 2", summary.StaticToken)
	}
	if summary.MutualTLS != 3 {
		t.Fatalf("mutual_tls=%d, want 3", summary.MutualTLS)
	}
	if summary.Retired != 2 {
		t.Fatalf("token_retired=%d, want 2", summary.Retired)
	}
	// A server with no hostname is still named, by identifier, so the operator
	// can find it.
	if len(summary.Pending) != 2 || summary.Pending[0] != "old-node" || summary.Pending[1] != "5" {
		t.Fatalf("pending=%v, want [old-node 5]", summary.Pending)
	}
}

func TestANewServerIsCreatedWithATokenThatCannotAuthenticate(t *testing.T) {
	token := repository.NewAgentToken()
	if !strings.HasPrefix(token, repository.RetiredAgentTokenPrefix) {
		t.Fatalf("a new server was given the token %q, which is not retired on arrival", token)
	}
	// The panel refuses it before any lookup, which is where it matters: the
	// two constants have to agree.
	if !agentpki.IsRetiredToken(token) {
		t.Fatal("agentpki does not recognise the value the repository writes as retired")
	}
}

func TestTheLegacyDirectoryIsUnavailableRatherThanPanickingWithoutARepository(t *testing.T) {
	s := &ServerService{logger: zap.NewNop()}
	dir := s.LegacyAgentDirectory()
	if _, err := dir.LookupByStaticToken(context.Background(), "anything"); !errors.Is(err, agentpki.ErrLegacyUnavailable) {
		t.Fatalf("LookupByStaticToken returned %v, want ErrLegacyUnavailable", err)
	}
	if _, err := dir.ListStaticTokenServers(context.Background()); !errors.Is(err, agentpki.ErrLegacyUnavailable) {
		t.Fatalf("ListStaticTokenServers returned %v, want ErrLegacyUnavailable", err)
	}
}
