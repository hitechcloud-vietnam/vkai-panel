# VKAI Panel Security Guide

## Table of Contents

1. [Security Overview](#security-overview)
2. [Authentication](#authentication)
3. [Authorization](#authorization)
4. [Data Protection](#data-protection)
5. [Network Security](#network-security)
6. [Application Security](#application-security)
7. [Infrastructure Security](#infrastructure-security)
8. [Security Monitoring](#security-monitoring)
9. [Incident Response](#incident-response)
10. [Compliance](#compliance)
11. [Security Checklist](#security-checklist)

---

## Security Overview

### Security Principles

1. **Defense in Depth**: Multiple layers of security
2. **Least Privilege**: Minimum required permissions
3. **Zero Trust**: Verify everything, trust nothing
4. **Security by Design**: Security built into architecture
5. **Continuous Monitoring**: Real-time threat detection

### Security Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Security Layers                          │
├─────────────────────────────────────────────────────────────┤
│  Layer 0: Panel access gate                                 │
│           own port (VKAI_PANEL_PORT, default 8888),         │
│           host pin, IP allow list, security entrance;       │
│           anything else gets a neutral 404                  │
│  Layer 1: Network Security (Firewall, IDS/IPS)              │
│  Layer 2: Transport Security (TLS/SSL)                      │
│  Layer 3: Application Security (WAF, Rate Limiting)         │
│  Layer 4: Authentication (JWT, MFA)                         │
│  Layer 5: Authorization (RBAC, ABAC)                        │
│  Layer 6: Data Security (Encryption, Masking)               │
│  Layer 7: Monitoring (SIEM, Alerts)                         │
└─────────────────────────────────────────────────────────────┘
```

### The panel is not on 80/443

Ports 80 and 443 belong to the customer websites hosted on this server. Putting
an administration interface there means every scanner on the Internet probes it,
a misconfigured customer vhost can expose it, and the panel's log lines drown in
the sites' access logs. VKAI Panel therefore listens on its own port behind a
secret entrance path, and checks host, source IP and entrance in that order
before anything else runs. See [PANEL_ACCESS.md](PANEL_ACCESS.md).

---

## Authentication

### JWT Authentication

#### Token Structure

```json
{
  "header": {
    "alg": "RS256",
    "typ": "JWT"
  },
  "payload": {
    "sub": "user_id",
    "iss": "vkai-panel",
    "iat": 1699900000,
    "exp": 1699900900,
    "tenant_id": 1,
    "roles": ["admin"],
    "permissions": ["websites:create", "websites:read"]
  },
  "signature": "..."
}
```

#### Token Configuration

```go
// JWT configuration
type JWTConfig struct {
    Secret            string        // Secret key for signing
    AccessTokenTTL    time.Duration // Access token lifetime (15m)
    RefreshTokenTTL   time.Duration // Refresh token lifetime (7d)
    Issuer            string        // Token issuer
    SigningMethod     string        // Signing method (RS256/HS256)
}
```

#### Best Practices

1. **Short-lived Access Tokens**: 15 minutes
2. **Long-lived Refresh Tokens**: 7 days
3. **Token Rotation**: Rotate refresh tokens on use
4. **Token Revocation**: Blacklist revoked tokens
5. **Secure Storage**: Store tokens securely

### Password Security

#### Password Requirements

```go
type PasswordPolicy struct {
    MinLength        int  // Minimum 12 characters
    RequireUppercase bool // At least 1 uppercase
    RequireLowercase bool // At least 1 lowercase
    RequireNumbers   bool // At least 1 number
    RequireSpecial   bool // At least 1 special character
    MaxAge           int  // Maximum age in days (90)
    HistoryCount     int  // Remember last 5 passwords
}
```

#### Password Hashing

```go
// Use bcrypt for password hashing
func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
    return string(bytes), err
}

func CheckPassword(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

### Multi-Factor Authentication (MFA)

#### TOTP Implementation

```go
// Generate TOTP secret
func GenerateTOTPSecret() (string, error) {
    secret := make([]byte, 20)
    _, err := rand.Read(secret)
    if err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(secret), nil
}

// Validate TOTP code
func ValidateTOTP(secret, code string) bool {
    // Implementation using TOTP algorithm
    return totp.Validate(code, secret)
}
```

#### MFA Configuration

```yaml
mfa:
  enabled: true
  methods:
    - totp
    - sms
    - email
  backup_codes: 10
  grace_period: 7d
```

---

## Authorization

### Role-Based Access Control (RBAC)

#### Role Hierarchy

```
Super Admin
    └── Admin
        ├── Manager
        │   ├── Developer
        │   └── Support
        └── User
            └── Viewer
```

#### Permission Model

```go
type Permission struct {
    ID          int64  `json:"id"`
    Name        string `json:"name"`
    Resource    string `json:"resource"`
    Action      string `json:"action"`
    Description string `json:"description"`
}

// Example permissions
var permissions = []Permission{
    {Name: "websites:create", Resource: "websites", Action: "create"},
    {Name: "websites:read", Resource: "websites", Action: "read"},
    {Name: "websites:update", Resource: "websites", Action: "update"},
    {Name: "websites:delete", Resource: "websites", Action: "delete"},
    {Name: "databases:create", Resource: "databases", Action: "create"},
    {Name: "databases:read", Resource: "databases", Action: "read"},
    // ... more permissions
}
```

#### Role Definitions

```go
var roles = map[string][]string{
    "super_admin": {"*"}, // All permissions
    "admin": {
        "websites:*",
        "databases:*",
        "ssl:*",
        "dns:*",
        "users:read",
        "users:update",
    },
    "manager": {
        "websites:read",
        "websites:update",
        "databases:read",
        "ssl:read",
        "dns:read",
    },
    "developer": {
        "websites:read",
        "databases:read",
        "files:*",
    },
    "support": {
        "websites:read",
        "databases:read",
        "logs:read",
    },
    "user": {
        "websites:read",
        "files:read",
    },
    "viewer": {
        "websites:read",
    },
}
```

### Multi-Tenant Isolation

#### Tenant Isolation

```go
// All queries must include tenant_id
func (r *WebsiteRepository) GetByID(ctx context.Context, id int64) (*Website, error) {
    tenantID := ctx.Value("tenant_id").(int64)
    
    var website Website
    err := r.db.GetContext(ctx, &website,
        "SELECT * FROM websites WHERE id = $1 AND tenant_id = $2",
        id, tenantID,
    )
    return &website, err
}
```

#### Row-Level Security

```sql
-- Enable RLS
ALTER TABLE websites ENABLE ROW LEVEL SECURITY;

-- Create policy
CREATE POLICY tenant_isolation ON websites
    USING (tenant_id = current_setting('app.current_tenant')::bigint);
```

---

## Data Protection

### Encryption at Rest

#### Database Encryption

```sql
-- Enable encryption
ALTER SYSTEM SET ssl = on;
ALTER SYSTEM SET ssl_cert_file = '/path/to/server.crt';
ALTER SYSTEM SET ssl_key_file = '/path/to/server.key';
```

#### File Encryption

```go
// Encrypt sensitive files
func EncryptFile(key []byte, filename string) error {
    plaintext, err := os.ReadFile(filename)
    if err != nil {
        return err
    }
    
    block, err := aes.NewCipher(key)
    if err != nil {
        return err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
    return os.WriteFile(filename+".enc", ciphertext, 0644)
}
```

### Encryption in Transit

#### TLS Configuration

```nginx
# Strong TLS configuration
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;
ssl_prefer_server_ciphers off;
ssl_session_cache shared:SSL:10m;
ssl_session_timeout 10m;
ssl_stapling on;
ssl_stapling_verify on;
```

#### Certificate Management

Panel certificates live in `/vkai-panel/ssl/panel/` and are configured with
`VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY` (or generated on first start with
`VKAI_PANEL_TLS_SELF_SIGNED=true`). Customer site certificates live in
`/vkai-panel/ssl/<domain>/`, so a site renewal can never overwrite the panel key.

```bash
# Generate self-signed certificate (development)
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout server.key -out server.crt

# Let's Encrypt (production)
certbot certonly --webroot -w /vkai-panel/www/default -d your-domain.com
```

### Data Masking

```go
// Mask sensitive data
func MaskString(s string, visibleChars int) string {
    if len(s) <= visibleChars {
        return s
    }
    return s[:visibleChars] + strings.Repeat("*", len(s)-visibleChars)
}

// Example
// MaskString("1234567890", 4) => "1234******"
```

---

## Network Security

### Firewall Configuration

#### UFW Rules

```bash
# Allow SSH
sudo ufw allow 22/tcp

# Allow HTTP/HTTPS - customer websites only, never the panel
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Allow the panel port, restricted to the addresses you administer from
sudo ufw allow from 203.0.113.7 to any port 8888 proto tcp

# Allow the internal API port (localhost only)
sudo ufw allow from 127.0.0.1 to any port 30110

# Allow PostgreSQL (internal only)
sudo ufw allow from 127.0.0.1 to any port 5432

# Allow Redis (internal only)
sudo ufw allow from 127.0.0.1 to any port 6379

# Enable firewall
sudo ufw enable
```

#### iptables Rules

```bash
# Allow established connections
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

# Allow SSH
iptables -A INPUT -p tcp --dport 22 -j ACCEPT

# Allow HTTP/HTTPS - customer websites
iptables -A INPUT -p tcp --dport 80 -j ACCEPT
iptables -A INPUT -p tcp --dport 443 -j ACCEPT

# Allow the panel port from a trusted address only
iptables -A INPUT -p tcp -s 203.0.113.7 --dport 8888 -j ACCEPT

# Drop everything else
iptables -A INPUT -j DROP
```

### DDoS Protection

#### Rate Limiting

```go
// Rate limiting middleware
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
    limiter := rate.NewLimiter(rate.Every(window/time.Duration(limit)), limit)
    
    return func(c *gin.Context) {
        if !limiter.Allow() {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "error": "Rate limit exceeded",
            })
            c.Abort()
            return
        }
        c.Next()
    }
}
```

#### Nginx Rate Limiting

```nginx
# Rate limiting
limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
limit_req_zone $binary_remote_addr zone=login:10m rate=1r/s;

server {
    location /api/ {
        limit_req zone=api burst=20 nodelay;
    }
    
    location /api/v1/auth/login {
        limit_req zone=login burst=5 nodelay;
    }
}
```

### IP Whitelisting

```go
// IP whitelist middleware
func IPWhitelist(allowedIPs []string) gin.HandlerFunc {
    return func(c *gin.Context) {
        clientIP := c.ClientIP()
        
        allowed := false
        for _, ip := range allowedIPs {
            if clientIP == ip {
                allowed = true
                break
            }
        }
        
        if !allowed {
            c.JSON(http.StatusForbidden, gin.H{
                "error": "IP not allowed",
            })
            c.Abort()
            return
        }
        
        c.Next()
    }
}
```

---

## Application Security

### Input Validation

#### Request Validation

```go
// Validate request
type CreateWebsiteRequest struct {
    Domain    string `json:"domain" binding:"required,fqdn"`
    ServerID  int64  `json:"server_id" binding:"required,min=1"`
    WebServer string `json:"web_server" binding:"required,oneof=nginx apache"`
}

func (r *CreateWebsiteRequest) Validate() error {
    // Custom validation
    if strings.Contains(r.Domain, "..") {
        return errors.New("invalid domain")
    }
    return nil
}
```

#### SQL Injection Prevention

```go
// Use parameterized queries
func (r *WebsiteRepository) GetByDomain(ctx context.Context, domain string) (*Website, error) {
    var website Website
    err := r.db.GetContext(ctx, &website,
        "SELECT * FROM websites WHERE domain = $1", // Parameterized
        domain,
    )
    return &website, err
}

// Never concatenate strings
// BAD: "SELECT * FROM websites WHERE domain = '" + domain + "'"
```

#### XSS Prevention

```go
// HTML escaping
func EscapeHTML(s string) string {
    return html.EscapeString(s)
}

// Content Security Policy
func CSPMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Content-Security-Policy", 
            "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
        c.Next()
    }
}
```

### CSRF Protection

```go
// CSRF token generation
func GenerateCSRFToken() (string, error) {
    token := make([]byte, 32)
    _, err := rand.Read(token)
    if err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(token), nil
}

// CSRF middleware
func CSRFProtection() gin.HandlerFunc {
    return func(c *gin.Context) {
        if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "DELETE" {
            token := c.GetHeader("X-CSRF-Token")
            sessionToken := c.MustGet("csrf_token").(string)
            
            if token != sessionToken {
                c.JSON(http.StatusForbidden, gin.H{
                    "error": "Invalid CSRF token",
                })
                c.Abort()
                return
            }
        }
        c.Next()
    }
}
```

### Security Headers

```go
// Security headers middleware
func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Frame-Options", "SAMEORIGIN")
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-XSS-Protection", "1; mode=block")
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        c.Next()
    }
}
```

---

## Infrastructure Security

### System Hardening

#### SSH Hardening

```bash
# /etc/ssh/sshd_config
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
Port 2222
MaxAuthTries 3
ClientAliveInterval 300
ClientAliveCountMax 2
```

#### File Permissions

```bash
# Set secure permissions
chmod 700 /home/vkai
chmod 600 /vkai-panel/etc/.env
chmod 644 /vkai-panel/config/*
chmod 755 /vkai-panel/scripts/*
```

#### User Management

```bash
# Create service user
sudo useradd -r -m -d /home/vkai -s /bin/bash vkai

# Add to necessary groups
sudo usermod -aG www-data vkai

# Set password policy
sudo chage -M 90 -m 7 -W 14 vkai
```

### Service Sandboxing (systemd)

The panel runs as native systemd services, not in containers, so process
isolation is enforced by systemd directives rather than by a container runtime.
The shipped units in `deploy/systemd/` already carry these settings; keep them if
you edit the units locally.

```ini
# Excerpt from deploy/systemd/vkai-api.service
[Service]
User=vkai
Group=vkai

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true

# The whole filesystem is read-only except these paths.
ReadWritePaths=/vkai-panel/etc
ReadWritePaths=/vkai-panel/www
ReadWritePaths=/vkai-panel/logs
ReadWritePaths=/vkai-panel/ssl
ReadWritePaths=/vkai-panel/tmp
# The panel manages the customer web server and certificates.
ReadWritePaths=-/etc/nginx
ReadWritePaths=-/etc/letsencrypt

LimitNOFILE=65536
LimitNPROC=4096
```

Rules to keep:

- `vkai-api` and `vkai-ui` run as the unprivileged `vkai` user, never as root.
- `vkai-agent` needs root for host-level operations. It is **optional** -- do not
  enable it on a single-server install that does not manage remote nodes.
- Never widen `ReadWritePaths` beyond the directory a change actually needs.
- The panel binds a dedicated port, so no service needs `NET_BIND_SERVICE` or any
  other capability to bind a privileged port.

Audit the effective sandbox of a running unit:

```bash
systemd-analyze security vkai-api
systemd-analyze security vkai-ui
```

### Customer Container Security

Separately from the above: the panel offers customers a Docker management
feature (the Docker screen, `/api/v1/docker/*`). That feature drives the Docker
Engine on the host on the customer's behalf. Access to it is gated by the
`docker:*` RBAC permissions -- grant them only to roles that are meant to control
containers, because the Docker socket is effectively root-equivalent on the host.

---

## Security Monitoring

### Logging

#### Security Events

```go
// Log security events
func LogSecurityEvent(eventType, userID, ip, details string) {
    log.Info("Security event",
        zap.String("type", eventType),
        zap.String("user_id", userID),
        zap.String("ip", ip),
        zap.String("details", details),
        zap.Time("timestamp", time.Now()),
    )
}

// Example events
LogSecurityEvent("login_success", userID, clientIP, "")
LogSecurityEvent("login_failed", "", clientIP, "invalid password")
LogSecurityEvent("permission_denied", userID, clientIP, "websites:delete")
LogSecurityEvent("suspicious_activity", userID, clientIP, "multiple failed logins")
```

#### Audit Logging

```go
// Audit log structure
type AuditLog struct {
    ID        int64     `json:"id"`
    UserID    int64     `json:"user_id"`
    TenantID  int64     `json:"tenant_id"`
    Action    string    `json:"action"`
    Resource  string    `json:"resource"`
    ResourceID int64    `json:"resource_id"`
    Details   string    `json:"details"`
    IP        string    `json:"ip"`
    UserAgent string    `json:"user_agent"`
    CreatedAt time.Time `json:"created_at"`
}
```

### Intrusion Detection

#### Fail2Ban Configuration

```ini
# /etc/fail2ban/jail.local
[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 3
bantime = 3600

[vkai-api]
enabled = true
port = 30110
filter = vkai-api
logpath = /vkai-panel/logs/api.log
maxretry = 5
bantime = 1800
```

#### Custom Filter

```ini
# /etc/fail2ban/filter.d/vkai-api.conf
[Definition]
failregex = ^.*Login failed.*IP: <HOST>.*$
            ^.*Invalid token.*IP: <HOST>.*$
            ^.*Rate limit exceeded.*IP: <HOST>.*$
ignoreregex =
```

### Alerting

#### Alert Rules

```yaml
# Prometheus alerting rules
groups:
  - name: vkai-alerts
    rules:
      - alert: HighFailedLogins
        expr: rate(vkai_login_failed_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High failed login rate"
          
      - alert: APIHighLatency
        expr: histogram_quantile(0.95, rate(vkai_api_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "API high latency"
          
      - alert: DiskSpaceLow
        expr: node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"} < 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Disk space low"
```

---

## Incident Response

### Incident Response Plan

#### 1. Detection

```bash
# Monitor logs
tail -f /vkai-panel/logs/api.log | grep -i "error\|warning\|security"

# Check for suspicious activity
grep "login_failed" /vkai-panel/logs/api.log | awk '{print $1}' | sort | uniq -c | sort -rn
```

#### 2. Containment

```bash
# Block suspicious IP
sudo ufw deny from <suspicious_ip>

# Disable compromised user
sudo -u postgres psql -c "UPDATE users SET status = 'disabled' WHERE id = <user_id>;"

# Revoke all tokens
redis-cli FLUSHDB
```

#### 3. Eradication

```bash
# Change passwords
sudo -u postgres psql -c "UPDATE users SET password_hash = '<new_hash>' WHERE id = <user_id>;"

# Rotate secrets
openssl rand -base64 64 > /vkai-panel/etc/.env.new
```

#### 4. Recovery

```bash
# Restore from backup
pg_dump -U vkai -d vkai_panel < backup.sql

# Restart services
systemctl restart vkai-api
systemctl restart vkai-ui
```

#### 5. Lessons Learned

- Document incident
- Update security measures
- Train team
- Review procedures

### Incident Response Team

```yaml
incident_response_team:
  lead:
    name: "Security Lead"
    contact: "security@hitechcloud.vn"
    phone: "+84 xxx xxx xxx"
  
  members:
    - name: "DevOps Engineer"
      role: "Infrastructure"
    - name: "Backend Developer"
      role: "Application"
    - name: "Database Admin"
      role: "Database"
```

---

## Compliance

### GDPR Compliance

#### Data Protection

```go
// Data retention policy
type DataRetentionPolicy struct {
    UserData     time.Duration // 30 days after account deletion
    AuditLogs    time.Duration // 1 year
    Backups      time.Duration // 90 days
    SessionData  time.Duration // 7 days
}

// Right to be forgotten
func DeleteUserData(userID int64) error {
    // Delete user data
    // Anonymize logs
    // Remove from backups
    return nil
}
```

#### Privacy Policy

```markdown
## Data Collection

We collect:
- Account information (name, email)
- Usage data (pages visited, actions taken)
- Technical data (IP, browser, device)

## Data Usage

We use data to:
- Provide and improve services
- Ensure security
- Comply with legal obligations

## Data Sharing

We do not sell data. We share data with:
- Service providers (hosting, analytics)
- Legal authorities (when required)
```

### SOC 2 Compliance

#### Controls

```yaml
security_controls:
  access_control:
    - Multi-factor authentication
    - Role-based access control
    - Regular access reviews
  
  change_management:
    - Code review requirements
    - Automated testing
    - Deployment approvals
  
  incident_response:
    - Incident response plan
    - Regular drills
    - Post-incident reviews
  
  monitoring:
    - Real-time monitoring
    - Alerting
    - Log retention
```

---

## Security Checklist

### Pre-Deployment Checklist

- [ ] **Panel access**
  - [ ] Panel port is not 80/443 and is open in the firewall
  - [ ] Security entrance enabled and the generated path recorded safely
  - [ ] `VKAI_PANEL_ALLOWED_IPS` set to the administrators' addresses
  - [ ] `VKAI_PANEL_TRUSTED_PROXIES` set if and only if a proxy fronts the panel
  - [ ] `VKAI_PANEL_DOMAIN` set when the panel has a host name
  - [ ] Panel TLS configured (`VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY`)
  - [ ] Ports 30110 and 3000 are not reachable from the Internet
  - [ ] `/vkai-panel/etc/.env` and `panel_access.json` are mode `0600`, owner `vkai`

- [ ] **Secrets**
  - [ ] `VKAI_JWT_SECRET` is at least 32 random characters and unique to this install
  - [ ] `VKAI_SECRET_KEY` is set (32 bytes hex/base64)
  - [ ] `VKAI_AGENT_TOKEN` is ABSENT from `.env`: it is obsolete and ignored (docs/AGENT_CHANNEL.md)
  - [ ] `VKAI_DB_PASSWORD` is not a value published in this repository
  - [ ] Default administrator password changed at first login

- [ ] **Authentication**
  - [ ] JWT tokens configured correctly
  - [ ] Password policy enforced
  - [ ] MFA enabled for admin users
  - [ ] Session management configured

- [ ] **Authorization**
  - [ ] RBAC implemented
  - [ ] Tenant isolation verified
  - [ ] Permissions tested
  - [ ] API endpoints protected

- [ ] **Data Protection**
  - [ ] Encryption at rest enabled
  - [ ] Encryption in transit (TLS) configured
  - [ ] Sensitive data masked
  - [ ] Backup encryption enabled

- [ ] **Network Security**
  - [ ] Firewall configured
  - [ ] Rate limiting enabled
  - [ ] DDoS protection configured
  - [ ] IP whitelisting (if needed)

- [ ] **Application Security**
  - [ ] Input validation implemented
  - [ ] SQL injection prevention
  - [ ] XSS prevention
  - [ ] CSRF protection
  - [ ] Security headers configured

- [ ] **Infrastructure Security**
  - [ ] SSH hardened
  - [ ] File permissions set
  - [ ] System updated
  - [ ] Unnecessary services disabled

- [ ] **Monitoring**
  - [ ] Logging configured
  - [ ] Alerting configured
  - [ ] Intrusion detection enabled
  - [ ] Audit logging enabled

- [ ] **Compliance**
  - [ ] GDPR compliance verified
  - [ ] Privacy policy updated
  - [ ] Data retention policy configured
  - [ ] Security documentation updated

### Regular Security Tasks

#### Daily

- [ ] Review security logs
- [ ] Check for failed login attempts
- [ ] Monitor system resources
- [ ] Verify backup completion

#### Weekly

- [ ] Review user access
- [ ] Check for security updates
- [ ] Analyze security metrics
- [ ] Test backup restoration

#### Monthly

- [ ] Security audit
- [ ] Penetration testing
- [ ] Access review
- [ ] Policy review

#### Quarterly

- [ ] Security training
- [ ] Incident response drill
- [ ] Compliance review
- [ ] Vendor security assessment

---

## Security Tools

### Recommended Tools

```bash
# Vulnerability scanning
sudo apt install nikto
nikto -h http://127.0.0.1:3000

# Port scanning
sudo apt install nmap
nmap -sV localhost

# SSL testing
openssl s_client -connect your-domain.com:443

# Log analysis
sudo apt install logwatch
logwatch --detail high --mailto admin@example.com
```

### Security Resources

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [CWE/SANS Top 25](https://cwe.mitre.org/top25/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)

---

## Support

- **Security Issues**: security@hitechcloud.vn
- **Bug Bounty**: https://hitechcloud.vn/security
- **Documentation**: https://docs.vkai.vn/security
