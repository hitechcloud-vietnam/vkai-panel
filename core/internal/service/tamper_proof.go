package service

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"go.uber.org/zap"
)

type TamperProofService struct {
	repo   *repository.TamperProofRepository
	logger *zap.Logger
}

func NewTamperProofService(repo *repository.TamperProofRepository, logger *zap.Logger) *TamperProofService {
	return &TamperProofService{repo: repo, logger: logger}
}

// Protected Paths
func (s *TamperProofService) CreateProtectedPath(ctx context.Context, tenantID uuid.UUID, req models.CreateProtectedPathRequest) (*models.ProtectedPath, error) {
	return s.repo.CreateProtectedPath(ctx, tenantID, req)
}

func (s *TamperProofService) ListProtectedPaths(ctx context.Context, tenantID uuid.UUID) ([]models.ProtectedPath, error) {
	return s.repo.ListProtectedPaths(ctx, tenantID)
}

func (s *TamperProofService) GetProtectedPath(ctx context.Context, tenantID, id uuid.UUID) (*models.ProtectedPath, error) {
	return s.repo.GetProtectedPath(ctx, tenantID, id)
}

func (s *TamperProofService) UpdateProtectedPath(ctx context.Context, tenantID, id uuid.UUID, req models.UpdateProtectedPathRequest) (*models.ProtectedPath, error) {
	return s.repo.UpdateProtectedPath(ctx, tenantID, id, req)
}

func (s *TamperProofService) DeleteProtectedPath(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteProtectedPath(ctx, tenantID, id)
}

// Scan performs an integrity scan on a protected path
func (s *TamperProofService) Scan(ctx context.Context, tenantID, protectedID uuid.UUID) (*models.TamperScanResult, error) {
	startTime := time.Now()

	protected, err := s.repo.GetProtectedPath(ctx, tenantID, protectedID)
	if err != nil {
		return nil, fmt.Errorf("get protected path: %w", err)
	}

	result := &models.TamperScanResult{
		ID:          uuid.New(),
		TenantID:    tenantID,
		ProtectedID: protectedID,
		CreatedAt:   startTime,
	}

	// Collect files to scan
	files, err := s.collectFiles(protected)
	if err != nil {
		result.Status = "error"
		result.ScanLog = fmt.Sprintf("Error collecting files: %v", err)
		result.Duration = int(time.Since(startTime).Milliseconds())
		_ = s.repo.CreateScanResult(ctx, result)
		return result, nil
	}

	result.TotalFiles = len(files)

	// Get existing baselines
	existingBaselines, err := s.repo.GetBaselines(ctx, tenantID, protectedID)
	if err != nil {
		existingBaselines = []models.FileBaseline{}
	}
	baselineMap := make(map[string]models.FileBaseline)
	for _, b := range existingBaselines {
		baselineMap[b.FilePath] = b
	}

	var violations []string
	newFiles := 0
	modifiedFiles := 0
	deletedFiles := 0

	// Scan each file
	for _, filePath := range files {
		checksum, err := s.computeChecksum(filePath, protected.Algorithm)
		if err != nil {
			violations = append(violations, fmt.Sprintf("ERROR reading %s: %v", filePath, err))
			continue
		}

		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		result.ScannedFiles++

		baseline, exists := baselineMap[filePath]
		if !exists {
			// New file detected
			newFiles++
			if protected.AlertOnCreate {
				alert := &models.TamperAlert{
					ID:          uuid.New(),
					TenantID:    tenantID,
					ProtectedID: protectedID,
					FilePath:    filePath,
					AlertType:   "created",
					Severity:    "medium",
					NewChecksum: checksum,
					NewSize:     info.Size(),
					NewMode:     info.Mode().String(),
				}
				_ = s.repo.CreateAlert(ctx, alert)
				violations = append(violations, fmt.Sprintf("NEW: %s", filePath))
			}
			// Save baseline
			_ = s.repo.UpsertBaseline(ctx, &models.FileBaseline{
				ID:          uuid.New(),
				TenantID:    tenantID,
				ProtectedID: protectedID,
				FilePath:    filePath,
				Checksum:    checksum,
				FileSize:    info.Size(),
				FileMode:    info.Mode().String(),
				ModTime:     info.ModTime(),
				ScannedAt:   time.Now(),
			})
		} else {
			// Check for modifications
			if baseline.Checksum != checksum {
				modifiedFiles++
				if protected.AlertOnChange {
					severity := "high"
					if isCriticalFile(filePath) {
						severity = "critical"
					}
					alert := &models.TamperAlert{
						ID:          uuid.New(),
						TenantID:    tenantID,
						ProtectedID: protectedID,
						FilePath:    filePath,
						AlertType:   "modified",
						Severity:    severity,
						OldChecksum: baseline.Checksum,
						NewChecksum: checksum,
						OldSize:     baseline.FileSize,
						NewSize:     info.Size(),
						OldMode:     baseline.FileMode,
						NewMode:     info.Mode().String(),
					}
					_ = s.repo.CreateAlert(ctx, alert)
					violations = append(violations, fmt.Sprintf("MODIFIED: %s (was %s, now %s)", filePath, baseline.Checksum[:12], checksum[:12]))
				}
				// Update baseline
				_ = s.repo.UpsertBaseline(ctx, &models.FileBaseline{
					ID:          baseline.ID,
					TenantID:    tenantID,
					ProtectedID: protectedID,
					FilePath:    filePath,
					Checksum:    checksum,
					FileSize:    info.Size(),
					FileMode:    info.Mode().String(),
					ModTime:     info.ModTime(),
					ScannedAt:   time.Now(),
				})
			}
			delete(baselineMap, filePath)
		}
	}

	// Check for deleted files
	for _, baseline := range baselineMap {
		deletedFiles++
		if protected.AlertOnDelete {
			alert := &models.TamperAlert{
				ID:          uuid.New(),
				TenantID:    tenantID,
				ProtectedID: protectedID,
				FilePath:    baseline.FilePath,
				AlertType:   "deleted",
				Severity:    "critical",
				OldChecksum: baseline.Checksum,
				OldSize:     baseline.FileSize,
				OldMode:     baseline.FileMode,
			}
			_ = s.repo.CreateAlert(ctx, alert)
			violations = append(violations, fmt.Sprintf("DELETED: %s", baseline.FilePath))
		}
	}

	// Update result
	result.Violations = len(violations)
	result.NewFiles = newFiles
	result.DeletedFiles = deletedFiles
	result.ModifiedFiles = modifiedFiles
	result.Duration = int(time.Since(startTime).Milliseconds())

	if len(violations) > 0 {
		result.Status = "violations_found"
		result.ScanLog = strings.Join(violations, "\n")
		_ = s.repo.UpdatePathAlertInfo(ctx, tenantID, protectedID)
	} else {
		result.Status = "clean"
		result.ScanLog = fmt.Sprintf("Scan complete. %d files checked, all clean.", result.ScannedFiles)
	}

	_ = s.repo.UpdatePathScanInfo(ctx, tenantID, protectedID, result.TotalFiles)
	_ = s.repo.CreateScanResult(ctx, result)

	return result, nil
}

// ScanAll scans all enabled protected paths
func (s *TamperProofService) ScanAll(ctx context.Context, tenantID uuid.UUID) ([]*models.TamperScanResult, error) {
	paths, err := s.repo.GetEnabledPaths(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	var results []*models.TamperScanResult
	for _, p := range paths {
		result, err := s.Scan(ctx, tenantID, p.ID)
		if err != nil {
			s.logger.Warn("scan failed", zap.String("path", p.Path), zap.Error(err))
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// Baselines
func (s *TamperProofService) GetBaselines(ctx context.Context, tenantID, protectedID uuid.UUID) ([]models.FileBaseline, error) {
	return s.repo.GetBaselines(ctx, tenantID, protectedID)
}

// RefreshBaseline re-scans and updates all baselines for a protected path
func (s *TamperProofService) RefreshBaseline(ctx context.Context, tenantID, protectedID uuid.UUID) (int, error) {
	protected, err := s.repo.GetProtectedPath(ctx, tenantID, protectedID)
	if err != nil {
		return 0, err
	}

	// Clear existing baselines
	_ = s.repo.DeleteBaselinesForPath(ctx, tenantID, protectedID)

	files, err := s.collectFiles(protected)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, filePath := range files {
		checksum, err := s.computeChecksum(filePath, protected.Algorithm)
		if err != nil {
			continue
		}
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}
		_ = s.repo.UpsertBaseline(ctx, &models.FileBaseline{
			ID:          uuid.New(),
			TenantID:    tenantID,
			ProtectedID: protectedID,
			FilePath:    filePath,
			Checksum:    checksum,
			FileSize:    info.Size(),
			FileMode:    info.Mode().String(),
			ModTime:     info.ModTime(),
			ScannedAt:   time.Now(),
		})
		count++
	}

	_ = s.repo.UpdatePathScanInfo(ctx, tenantID, protectedID, count)
	return count, nil
}

// Alerts
func (s *TamperProofService) ListAlerts(ctx context.Context, tenantID uuid.UUID, resolved *bool) ([]models.TamperAlert, error) {
	return s.repo.ListAlerts(ctx, tenantID, resolved)
}

func (s *TamperProofService) GetAlert(ctx context.Context, tenantID, id uuid.UUID) (*models.TamperAlert, error) {
	return s.repo.GetAlert(ctx, tenantID, id)
}

func (s *TamperProofService) ResolveAlert(ctx context.Context, tenantID, id uuid.UUID, resolvedBy, notes string) error {
	return s.repo.ResolveAlert(ctx, tenantID, id, resolvedBy, notes)
}

// Scan Results
func (s *TamperProofService) ListScanResults(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.TamperScanResult, error) {
	return s.repo.ListScanResults(ctx, tenantID, limit)
}

// Audit Log
func (s *TamperProofService) ListAuditLogs(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.TamperAuditLog, error) {
	return s.repo.ListAuditLogs(ctx, tenantID, limit)
}

func (s *TamperProofService) LogAudit(ctx context.Context, tenantID, userID uuid.UUID, username, action, target, details, ip string) {
	_ = s.repo.CreateAuditLog(ctx, &models.TamperAuditLog{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Action:    action,
		Target:    target,
		Details:   details,
		IPAddress: ip,
		UserID:    userID,
		Username:  username,
		CreatedAt: time.Now(),
	})
}

// Stats
func (s *TamperProofService) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.TamperStats, error) {
	return s.repo.GetStats(ctx, tenantID)
}

// Cleanup
func (s *TamperProofService) Cleanup(ctx context.Context, tenantID uuid.UUID, days int) (map[string]int64, error) {
	alerts, _ := s.repo.CleanupOldAlerts(ctx, tenantID, days)
	scans, _ := s.repo.CleanupOldScanResults(ctx, tenantID, days)
	logs, _ := s.repo.CleanupOldAuditLogs(ctx, tenantID, days)
	return map[string]int64{
		"alerts_deleted":      alerts,
		"scan_results_deleted": scans,
		"audit_logs_deleted":  logs,
	}, nil
}

// Helper functions
func (s *TamperProofService) collectFiles(p *models.ProtectedPath) ([]string, error) {
	var files []string

	if p.PathType == "file" {
		if _, err := os.Stat(p.Path); err == nil {
			files = append(files, p.Path)
		}
		return files, nil
	}

	if !p.Recursive {
		entries, err := os.ReadDir(p.Path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				fullPath := filepath.Join(p.Path, entry.Name())
				if !s.shouldIgnore(fullPath, p.IgnorePatterns) {
					files = append(files, fullPath)
				}
			}
		}
		return files, nil
	}

	err := filepath.Walk(p.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			if !s.shouldIgnore(path, p.IgnorePatterns) {
				files = append(files, path)
			}
		}
		return nil
	})
	return files, err
}

func (s *TamperProofService) shouldIgnore(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

func (s *TamperProofService) computeChecksum(filePath, algorithm string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var h hash.Hash
	switch algorithm {
	case "sha512":
		h = sha512.New()
	case "md5":
		h = md5.New()
	default: // sha256
		h = sha256.New()
	}

	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func isCriticalFile(path string) bool {
	critical := []string{"/etc/passwd", "/etc/shadow", "/etc/sudoers", "/etc/ssh/sshd_config", "/etc/nginx/nginx.conf"}
	for _, c := range critical {
		if path == c {
			return true
		}
	}
	return false
}
