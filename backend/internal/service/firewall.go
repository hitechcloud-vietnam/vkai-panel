package service

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

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

	if err := s.firewallRepo.Create(ctx, rule); err != nil {
		return nil, fmt.Errorf("failed to create firewall rule: %w", err)
	}

	// Apply rule to iptables
	if err := s.applyRule(ctx, rule); err != nil {
		fmt.Printf("Warning: failed to apply firewall rule: %v\n", err)
	}

	return rule, nil
}

func (s *FirewallService) GetByID(ctx context.Context, id uuid.UUID) (*models.FirewallRule, error) {
	return s.firewallRepo.GetByID(ctx, id)
}

func (s *FirewallService) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.FirewallRule, error) {
	return s.firewallRepo.ListByTenant(ctx, tenantID)
}

func (s *FirewallService) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.FirewallRule, error) {
	return s.firewallRepo.ListByServer(ctx, serverID)
}

func (s *FirewallService) Update(ctx context.Context, rule *models.FirewallRule) error {
	// Remove old rule
	_ = s.removeRule(ctx, rule)

	if err := s.firewallRepo.Update(ctx, rule); err != nil {
		return err
	}

	// Apply updated rule
	return s.applyRule(ctx, rule)
}

func (s *FirewallService) Delete(ctx context.Context, id uuid.UUID) error {
	rule, err := s.firewallRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	_ = s.removeRule(ctx, rule)
	return s.firewallRepo.Delete(ctx, id)
}

func (s *FirewallService) applyRule(ctx context.Context, rule *models.FirewallRule) error {
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

	args = append(args, "-j", rule.Action)

	cmd := exec.CommandContext(ctx, "iptables", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables failed: %s: %w", string(output), err)
	}

	return nil
}

func (s *FirewallService) removeRule(ctx context.Context, rule *models.FirewallRule) error {
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

	args = append(args, "-j", rule.Action)

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
