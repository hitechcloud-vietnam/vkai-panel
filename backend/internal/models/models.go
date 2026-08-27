package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// JSONMap is a map[string]interface{} that implements database/sql Scanner and driver.Valuer
type JSONMap map[string]interface{}

// Value implements the driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("JSONMap.Scan: expected []byte, got %T", value)
	}
	return json.Unmarshal(bytes, j)
}

// ============================================================
// TENANT
// ============================================================

type Tenant struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Slug        string     `json:"slug" db:"slug"`
	Domain      string     `json:"domain" db:"domain"`
	Status      string     `json:"status" db:"status"`
	Plan        string     `json:"plan" db:"plan"`
	MaxServers  int        `json:"max_servers" db:"max_servers"`
	MaxWebsites int        `json:"max_websites" db:"max_websites"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

// ============================================================
// USER
// ============================================================

type User struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	TenantID     uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Username     string     `json:"username" db:"username"`
	Email        string     `json:"email" db:"email"`
	PasswordHash string     `json:"-" db:"password_hash"`
	FirstName    string     `json:"first_name" db:"first_name"`
	LastName     string     `json:"last_name" db:"last_name"`
	Status       string     `json:"status" db:"status"`
	LastLoginAt  *time.Time `json:"last_login_at" db:"last_login_at"`
	LastLoginIP  string     `json:"last_login_ip" db:"last_login_ip"`
	MFAEnabled   bool       `json:"mfa_enabled" db:"mfa_enabled"`
	MFASecret    string     `json:"-" db:"mfa_secret"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt    *time.Time `json:"-" db:"deleted_at"`
}

// ============================================================
// ROLE & PERMISSION
// ============================================================

type Role struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	IsSystem    bool      `json:"is_system" db:"is_system"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Permission struct {
	ID       uuid.UUID `json:"id" db:"id"`
	Resource string    `json:"resource" db:"resource"`
	Action   string    `json:"action" db:"action"`
}

type UserRole struct {
	UserID uuid.UUID `json:"user_id" db:"user_id"`
	RoleID uuid.UUID `json:"role_id" db:"role_id"`
}

type RolePermission struct {
	RoleID       uuid.UUID `json:"role_id" db:"role_id"`
	PermissionID uuid.UUID `json:"permission_id" db:"permission_id"`
}

// ============================================================
// SERVER
// ============================================================

type Server struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Hostname    string     `json:"hostname" db:"hostname"`
	IPAddress   string     `json:"ip_address" db:"ip_address"`
	IPv6Address string     `json:"ipv6_address" db:"ipv6_address"`
	SSHPort     int        `json:"ssh_port" db:"ssh_port"`
	AgentStatus string     `json:"agent_status" db:"agent_status"`
	AgentToken  string     `json:"-" db:"agent_token"`
	OS          string     `json:"os" db:"os"`
	Kernel      string     `json:"kernel" db:"kernel"`
	CPUCores    int        `json:"cpu_cores" db:"cpu_cores"`
	RAMTotal    int64      `json:"ram_total" db:"ram_total"`
	DiskTotal   int64      `json:"disk_total" db:"disk_total"`
	Location    string     `json:"location" db:"location"`
	Tags        []string   `json:"tags" db:"tags"`
	Role          string     `json:"role" db:"role"`
	WebServerType string     `json:"web_server_type" db:"web_server_type"`
	Status        string     `json:"status" db:"status"`
	LastSeenAt  *time.Time `json:"last_seen_at" db:"last_seen_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

type ServerMetric struct {
	ID         uuid.UUID `json:"id" db:"id"`
	ServerID   uuid.UUID `json:"server_id" db:"server_id"`
	CPUPercent float64   `json:"cpu_percent" db:"cpu_percent"`
	RAMUsed    int64     `json:"ram_used" db:"ram_used"`
	RAMTotal   int64     `json:"ram_total" db:"ram_total"`
	DiskUsed   int64     `json:"disk_used" db:"disk_used"`
	DiskTotal  int64     `json:"disk_total" db:"disk_total"`
	NetIn      int64     `json:"net_in" db:"net_in"`
	NetOut     int64     `json:"net_out" db:"net_out"`
	Load1      float64   `json:"load1" db:"load1"`
	Load5      float64   `json:"load5" db:"load5"`
	Load15     float64   `json:"load15" db:"load15"`
	Timestamp  time.Time `json:"timestamp" db:"timestamp"`
}

// ============================================================
// WEBSITE
// ============================================================

type Website struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	TenantID      uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ServerID      uuid.UUID  `json:"server_id" db:"server_id"`
	Domain        string     `json:"domain" db:"domain"`
	RootDir       string     `json:"root_dir" db:"root_dir"`
	WebServerType string     `json:"web_server_type" db:"web_server_type"`
	PHPVersion    string     `json:"php_version" db:"php_version"`
	SiteType      string     `json:"site_type" db:"site_type"`
	Status        string     `json:"status" db:"status"`
	SSLEnabled    bool       `json:"ssl_enabled" db:"ssl_enabled"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt     *time.Time `json:"-" db:"deleted_at"`
}

type Domain struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	WebsiteID *uuid.UUID `json:"website_id" db:"website_id"`
	Name      string     `json:"name" db:"name"`
	Type      string     `json:"type" db:"type"` // primary, alias, subdomain, wildcard
	Status    string     `json:"status" db:"status"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}

// ============================================================
// SSL CERTIFICATE
// ============================================================

type SSLCertificate struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	TenantID      uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	WebsiteID     *uuid.UUID `json:"website_id" db:"website_id"`
	Domain        string     `json:"domain" db:"domain"`
	Issuer        string     `json:"issuer" db:"issuer"`
	Certificate   string     `json:"-" db:"certificate"`
	PrivateKey    string     `json:"-" db:"private_key"`
	ChainCert     string     `json:"-" db:"chain_cert"`
	NotBefore     time.Time  `json:"not_before" db:"not_before"`
	NotAfter      time.Time  `json:"not_after" db:"not_after"`
	Status        string     `json:"status" db:"status"`
	AutoRenew     bool       `json:"auto_renew" db:"auto_renew"`
	Source        string     `json:"source" db:"source"` // letsencrypt, custom
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

// ============================================================
// DATABASE
// ============================================================

type DatabaseServer struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID  uuid.UUID `json:"server_id" db:"server_id"`
	Type      string    `json:"type" db:"type"` // mariadb, mysql, postgresql
	Version   string    `json:"version" db:"version"`
	Port      int       `json:"port" db:"port"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type DatabaseEntry struct {
	ID               uuid.UUID `json:"id" db:"id"`
	TenantID         uuid.UUID `json:"tenant_id" db:"tenant_id"`
	DatabaseServerID uuid.UUID `json:"database_server_id" db:"database_server_id"`
	Name             string    `json:"name" db:"name"`
	Username         string    `json:"username" db:"username"`
	Password         string    `json:"-" db:"password"`
	Charset          string    `json:"charset" db:"charset"`
	Collation        string    `json:"collation" db:"collation"`
	Size             int64     `json:"size" db:"size"`
	Status           string    `json:"status" db:"status"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// ============================================================
// DNS
// ============================================================

type DNSZone struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name      string    `json:"name" db:"name"`
	Provider  string    `json:"provider" db:"provider"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type DNSRecord struct {
	ID       uuid.UUID `json:"id" db:"id"`
	ZoneID   uuid.UUID `json:"zone_id" db:"zone_id"`
	TenantID uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Type     string    `json:"type" db:"type"`
	Name     string    `json:"name" db:"name"`
	Value    string    `json:"value" db:"value"`
	TTL      int       `json:"ttl" db:"ttl"`
	Priority *int      `json:"priority" db:"priority"`
	Status   string    `json:"status" db:"status"`
}

// ============================================================
// DOCKER
// ============================================================

type DockerHost struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ServerID  uuid.UUID `json:"server_id" db:"server_id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Version   string    `json:"version" db:"version"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type DockerContainer struct {
	ID          uuid.UUID `json:"id" db:"id"`
	HostID      uuid.UUID `json:"host_id" db:"host_id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Image       string    `json:"image" db:"image"`
	Status      string    `json:"status" db:"status"`
	Ports       string    `json:"ports" db:"ports"`
	State       string    `json:"state" db:"state"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// ============================================================
// CRON JOB
// ============================================================

type CronJob struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ServerID  uuid.UUID  `json:"server_id" db:"server_id"`
	Name      string     `json:"name" db:"name"`
	Command   string     `json:"command" db:"command"`
	Schedule  string     `json:"schedule" db:"schedule"`
	Type      string     `json:"type" db:"type"` // shell, php, url, nodejs
	Status    string     `json:"status" db:"status"`
	LastRunAt *time.Time `json:"last_run_at" db:"last_run_at"`
	NextRunAt *time.Time `json:"next_run_at" db:"next_run_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}

// ============================================================
// FIREWALL RULE
// ============================================================

type FirewallRule struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID  uuid.UUID `json:"server_id" db:"server_id"`
	Protocol  string    `json:"protocol" db:"protocol"`
	Port      string    `json:"port" db:"port"`
	Source    string    `json:"source" db:"source"`
	Action    string    `json:"action" db:"action"` // allow, deny
	Direction string    `json:"direction" db:"direction"` // in, out
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// ============================================================
// BACKUP
// ============================================================

type BackupJob struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Type        string    `json:"type" db:"type"` // website, database, config
	ResourceID  uuid.UUID `json:"resource_id" db:"resource_id"`
	Destination string    `json:"destination" db:"destination"` // local, s3, sftp
	Schedule    string    `json:"schedule" db:"schedule"`
	Retention   int       `json:"retention" db:"retention"`
	Encrypted   bool      `json:"encrypted" db:"encrypted"`
	Status      string    `json:"status" db:"status"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type BackupRecord struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	JobID       uuid.UUID  `json:"job_id" db:"job_id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Size        int64      `json:"size" db:"size"`
	Path        string     `json:"path" db:"path"`
	Status      string     `json:"status" db:"status"`
	StartedAt   time.Time  `json:"started_at" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
	ErrorMsg    string     `json:"error_msg" db:"error_msg"`
}

// ============================================================
// JOB
// ============================================================

type Job struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Type        string     `json:"type" db:"type"`
	Status      string     `json:"status" db:"status"` // queued, running, success, failed, cancelled
	Payload     string     `json:"payload" db:"payload"`
	Result      string     `json:"result" db:"result"`
	ErrorMsg    string     `json:"error_msg" db:"error_msg"`
	Progress    int        `json:"progress" db:"progress"`
	StartedAt   *time.Time `json:"started_at" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// ============================================================
// API KEY
// ============================================================

type APIKey struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	Name      string     `json:"name" db:"name"`
	KeyHash   string     `json:"-" db:"key_hash"`
	KeyPrefix string     `json:"key_prefix" db:"key_prefix"`
	Scopes    []string   `json:"scopes" db:"scopes"`
	LastUsed  *time.Time `json:"last_used" db:"last_used"`
	ExpiresAt *time.Time `json:"expires_at" db:"expires_at"`
	Status    string     `json:"status" db:"status"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// ============================================================
// DEPLOYMENT
// ============================================================

type Deployment struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	WebsiteID   uuid.UUID  `json:"website_id" db:"website_id"`
	Source      string     `json:"source" db:"source"` // git, manual
	Branch      string     `json:"branch" db:"branch"`
	CommitHash  string     `json:"commit_hash" db:"commit_hash"`
	Status      string     `json:"status" db:"status"`
	Logs        string     `json:"logs" db:"logs"`
	StartedAt   *time.Time `json:"started_at" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// ============================================================
// REQUEST/RESPONSE DTOs
// ============================================================

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	User         User   `json:"user"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type CreateTenantRequest struct {
	Name        string `json:"name" binding:"required"`
	Slug        string `json:"slug" binding:"required"`
	Domain      string `json:"domain"`
	Plan        string `json:"plan"`
	MaxServers  int    `json:"max_servers"`
	MaxWebsites int    `json:"max_websites"`
}

type CreateUserRequest struct {
	TenantID  uuid.UUID `json:"tenant_id" binding:"required"`
	Username  string    `json:"username" binding:"required"`
	Email     string    `json:"email" binding:"required,email"`
	Password  string    `json:"password" binding:"required,min=8"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	RoleIDs   []uuid.UUID `json:"role_ids"`
}

type CreateServerRequest struct {
	Hostname  string   `json:"hostname" binding:"required"`
	IPAddress string   `json:"ip_address" binding:"required"`
	SSHPort   int      `json:"ssh_port"`
	Location  string   `json:"location"`
	Tags      []string `json:"tags"`
	Role      string   `json:"role"`
}

type CreateWebsiteRequest struct {
	Domain        string `json:"domain" binding:"required"`
	ServerID      uuid.UUID `json:"server_id" binding:"required"`
	RootDir       string `json:"root_dir"`
	WebServerType string `json:"web_server_type" binding:"required"`
	PHPVersion    string `json:"php_version"`
	SiteType      string `json:"site_type"`
}

type PaginationParams struct {
	Page    int `form:"page"`
	PerPage int `form:"per_page"`
}

func (p *PaginationParams) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PerPage < 1 {
		p.PerPage = 20
	}
	if p.PerPage > 100 {
		p.PerPage = 100
	}
}

func (p *PaginationParams) Offset() int {
	return (p.Page - 1) * p.PerPage
}

func NewPaginationParams(c interface{ Query(string) string }) *PaginationParams {
	params := &PaginationParams{}
	page := c.Query("page")
	perPage := c.Query("per_page")
	if page != "" {
		fmt.Sscanf(page, "%d", &params.Page)
	}
	if perPage != "" {
		fmt.Sscanf(perPage, "%d", &params.PerPage)
	}
	params.Normalize()
	return params
}

// ============================================================
// ADDITIONAL REQUEST DTOs
// ============================================================

type CreateDBServerRequest struct {
	ServerID uuid.UUID `json:"server_id" binding:"required"`
	Type     string    `json:"type" binding:"required"` // mysql, postgresql, redis
	Version  string    `json:"version"`
	Port     int       `json:"port"`
}

type CreateDBEntryRequest struct {
	DatabaseServerID uuid.UUID `json:"database_server_id" binding:"required"`
	Name             string    `json:"name" binding:"required"`
	Username         string    `json:"username" binding:"required"`
	Password         string    `json:"password" binding:"required"`
	Charset          string    `json:"charset"`
	Collation        string    `json:"collation"`
}

type CreateCronJobRequest struct {
	ServerID uuid.UUID `json:"server_id" binding:"required"`
	Name     string    `json:"name" binding:"required"`
	Command  string    `json:"command" binding:"required"`
	Schedule string    `json:"schedule" binding:"required"`
	Type     string    `json:"type"`
}

type CreateFirewallRuleRequest struct {
	ServerID  uuid.UUID `json:"server_id" binding:"required"`
	Protocol  string    `json:"protocol" binding:"required"`
	Port      string    `json:"port"`
	Source    string    `json:"source"`
	Action    string    `json:"action" binding:"required"`
	Direction string    `json:"direction"`
}

type CreateBackupJobRequest struct {
	Name        string    `json:"name" binding:"required"`
	Type        string    `json:"type" binding:"required"`
	ResourceID  uuid.UUID `json:"resource_id" binding:"required"`
	Destination string    `json:"destination"`
	Schedule    string    `json:"schedule"`
	Retention   int       `json:"retention"`
	Encrypted   bool      `json:"encrypted"`
}
