package service

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// Firewall rules are turned into iptables arguments. They do not go through a
// shell, but an unvalidated value can still add options to the iptables
// invocation, so each field is checked against a fixed allowlist.
var (
	firewallPortRe   = regexp.MustCompile(`^[0-9]{1,5}(:[0-9]{1,5})?$`)
	allowedProtocols = map[string]bool{"tcp": true, "udp": true, "icmp": true, "all": true, "": true}
	allowedActions   = map[string]bool{"ACCEPT": true, "DROP": true, "REJECT": true, "LOG": true}
	allowedDirection = map[string]bool{"in": true, "out": true, "": true}
)

func validateFirewallRule(rule *models.FirewallRule) error {
	if !allowedProtocols[strings.ToLower(rule.Protocol)] {
		return fmt.Errorf("protocol %q is not allowed", rule.Protocol)
	}
	if !allowedDirection[strings.ToLower(rule.Direction)] {
		return fmt.Errorf("direction %q is not allowed", rule.Direction)
	}
	if !allowedActions[strings.ToUpper(rule.Action)] {
		return fmt.Errorf("action %q is not allowed", rule.Action)
	}
	if rule.Port != "" && !firewallPortRe.MatchString(rule.Port) {
		return fmt.Errorf("port %q is not a valid port or port range", rule.Port)
	}
	if rule.Source != "" && strings.ToLower(rule.Source) != "any" {
		if _, _, err := net.ParseCIDR(rule.Source); err != nil {
			if net.ParseIP(rule.Source) == nil {
				return fmt.Errorf("source %q is not a valid IP address or CIDR", rule.Source)
			}
		}
	}
	return nil
}

type FirewallService struct {
	firewallRepo *repository.FirewallRepository
	serverRepo   *repository.ServerRepository
}

func NewFirewallService(firewallRepo *repository.FirewallRepository, serverRepo *repository.ServerRepository) *FirewallService {
	return &FirewallService{
		firewallRepo: firewallRepo,
		serverRepo:   serverRepo,
	}
}

func (s *FirewallService) Create(ctx context.Context, req *models.CreateFirewallRuleRequest, tenantID uuid.UUID) (*models.FirewallRule, error) {
	rule := &models.FirewallRule{
		TenantID:  tenantID,
		ServerID:  req.ServerID,
		Protocol:  req.Protocol,
		Port:      req.Port,
		Source:    req.Source,
		Action:    req.Action,
		Direction: req.Direction,
		Status:    "active",
	}

	if rule.Direction == "" {
		rule.Direction = "in"
	}

	if err := validateFirewallRule(rule); err != nil {
		return nil, err
	}

	if err := s.firewallRepo.Create(ctx, rule); err != nil {
		return nil, fmt.Errorf("failed to create firewall rule: %w", err)
	}

	// Apply rule to iptables
	if err := s.applyRule(ctx, rule); err != nil {
		fmt.Printf("Warning: failed to apply firewall rule: %v\n", err)
	}

	return rule, nil
}

func (s *FirewallService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.FirewallRule, error) {
	return s.firewallRepo.GetByID(ctx, tenantID, id)
}

func (s *FirewallService) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.FirewallRule, error) {
	return s.firewallRepo.ListByTenant(ctx, tenantID)
}

func (s *FirewallService) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.FirewallRule, error) {
	return s.firewallRepo.ListByServer(ctx, serverID)
}

func (s *FirewallService) Update(ctx context.Context, rule *models.FirewallRule) error {
	if err := validateFirewallRule(rule); err != nil {
		return err
	}

	// Remove old rule
	_ = s.removeRule(ctx, rule)

	if err := s.firewallRepo.Update(ctx, rule); err != nil {
		return err
	}

	// Apply updated rule
	return s.applyRule(ctx, rule)
}

func (s *FirewallService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	rule, err := s.firewallRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	_ = s.removeRule(ctx, rule)
	return s.firewallRepo.Delete(ctx, tenantID, id)
}

func (s *FirewallService) applyRule(ctx context.Context, rule *models.FirewallRule) error {
	// A rule stored before this validation existed is re-checked here.
	if err := validateFirewallRule(rule); err != nil {
		return err
	}

	// Build iptables command
	args := []string{"-A", fmt.Sprintf("INPUT%s", "")}

	if rule.Direction == "out" {
		args = []string{"-A", "OUTPUT"}
	}

	if rule.Protocol != "" && rule.Protocol != "all" {
		args = append(args, "-p", rule.Protocol)
	}

	if rule.Port != "" {
		if rule.Protocol == "tcp" || rule.Protocol == "udp" {
			args = append(args, "--dport", rule.Port)
		}
	}

	if rule.Source != "" && rule.Source != "any" {
		args = append(args, "-s", rule.Source)
	}

	args = append(args, "-j", strings.ToUpper(rule.Action))

	cmd := exec.CommandContext(ctx, "iptables", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables failed: %s: %w", string(output), err)
	}

	return nil
}

func (s *FirewallService) removeRule(ctx context.Context, rule *models.FirewallRule) error {
	if err := validateFirewallRule(rule); err != nil {
		return err
	}

	args := []string{"-D", "INPUT"}

	if rule.Direction == "out" {
		args = []string{"-D", "OUTPUT"}
	}

	if rule.Protocol != "" && rule.Protocol != "all" {
		args = append(args, "-p", rule.Protocol)
	}

	if rule.Port != "" {
		if rule.Protocol == "tcp" || rule.Protocol == "udp" {
			args = append(args, "--dport", rule.Port)
		}
	}

	if rule.Source != "" && rule.Source != "any" {
		args = append(args, "-s", rule.Source)
	}

	args = append(args, "-j", strings.ToUpper(rule.Action))

	cmd := exec.CommandContext(ctx, "iptables", args...)
	return cmd.Run()
}

func (s *FirewallService) GetActiveRules(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "iptables", "-L", "-n", "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get iptables rules: %w", err)
	}
	return string(output), nil
}

func (s *FirewallService) SaveRules(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", "iptables-save > /etc/iptables/rules.v4")
	return cmd.Run()
}
