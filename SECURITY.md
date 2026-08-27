# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.0.x   | ✅ Yes             |
| < 1.0   | ❌ No              |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability in vKAI Panel, please report it responsibly.

### How to Report

**DO NOT** create a public GitHub issue for security vulnerabilities.

Instead, please report security vulnerabilities by emailing:

📧 **security@hitechcloud.vn**

### What to Include

When reporting a vulnerability, please include:

1. **Description**: A clear description of the vulnerability
2. **Steps to Reproduce**: Detailed steps to reproduce the issue
3. **Impact**: Potential impact of the vulnerability
4. **Affected Versions**: Which versions are affected
5. **Suggested Fix**: If you have a suggested fix (optional)

### Response Timeline

- **Initial Response**: Within 24 hours
- **Triage**: Within 72 hours
- **Fix Development**: Within 7 days for critical vulnerabilities
- **Public Disclosure**: After fix is released

### What to Expect

1. **Acknowledgment**: We will acknowledge receipt of your report
2. **Assessment**: We will assess the vulnerability
3. **Fix**: We will develop and test a fix
4. **Release**: We will release a patched version
5. **Credit**: We will credit you in the release notes (unless you prefer anonymity)

## Security Best Practices

### For Users

1. **Keep Updated**: Always use the latest version
2. **Strong Passwords**: Use strong, unique passwords
3. **Two-Factor Authentication**: Enable 2FA when available
4. **Regular Backups**: Maintain regular backups
5. **Monitor Logs**: Regularly review system logs
6. **Limit Access**: Restrict access to trusted IPs
7. **Firewall**: Configure firewall rules properly
8. **SSL/TLS**: Always use HTTPS in production

### For Administrators

1. **System Updates**: Keep OS and dependencies updated
2. **User Management**: Regularly review user accounts
3. **Access Control**: Implement least privilege principle
4. **Audit Logging**: Enable and review audit logs
5. **Backup Strategy**: Implement comprehensive backup strategy
6. **Monitoring**: Set up monitoring and alerting
7. **Incident Response**: Have an incident response plan

### For Developers

1. **Input Validation**: Validate all user inputs
2. **Output Encoding**: Encode all outputs
3. **Parameterized Queries**: Use parameterized queries
4. **Authentication**: Implement proper authentication
5. **Authorization**: Implement proper authorization
6. **Session Management**: Secure session management
7. **Error Handling**: Secure error handling
8. **Logging**: Log security events
9. **Dependencies**: Keep dependencies updated
10. **Code Review**: Conduct security code reviews

## Security Features

### Authentication

- JWT-based authentication
- Refresh token rotation
- Password hashing with bcrypt
- Account lockout after failed attempts
- Session management

### Authorization

- Role-based access control (RBAC)
- 8 predefined roles
- Granular permissions
- Multi-tenant isolation

### Data Protection

- Encrypted passwords
- Secure session storage
- CSRF protection
- XSS prevention
- SQL injection prevention

### Network Security

- HTTPS enforcement
- CORS configuration
- Rate limiting
- IP whitelisting
- Firewall integration

### Audit & Compliance

- Audit logging
- User activity tracking
- Security event logging
- Compliance reporting

## Security Configuration

### Environment Variables

```bash
# JWT Configuration
JWT_SECRET=your-secure-secret-key
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=7d

# Database
DB_SSL_MODE=require

# Redis
REDIS_PASSWORD=your-redis-password

# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=30110
```

### Nginx Security Headers

```nginx
# Security headers
add_header X-Frame-Options "SAMEORIGIN" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-XSS-Protection "1; mode=block" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';" always;
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
```

### Firewall Rules

```bash
# Allow SSH
sudo ufw allow 22/tcp

# Allow HTTP/HTTPS
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Allow Panel API (restrict to trusted IPs)
sudo ufw allow from TRUSTED_IP to any port 30110

# Enable firewall
sudo ufw enable
```

## Vulnerability Disclosure Policy

### Scope

This policy applies to:

- vKAI Panel software
- Official documentation
- Official deployment scripts

### Out of Scope

- Third-party dependencies (report to their maintainers)
- Social engineering attacks
- Physical attacks
- Denial of service attacks

### Safe Harbor

We support responsible disclosure and will not take legal action against researchers who:

- Make a good faith effort to avoid privacy violations
- Avoid破坏 or data destruction
- Only interact with accounts you own or with explicit permission
- Do not exploit a vulnerability beyond what is necessary to confirm its existence

## Security Updates

### How We Handle Security Updates

1. **Assessment**: Assess the severity and impact
2. **Development**: Develop a fix
3. **Testing**: Test the fix thoroughly
4. **Release**: Release a patched version
5. **Notification**: Notify users of the update
6. **Disclosure**: Publish security advisory

### Severity Levels

| Level | Description | Response Time |
|-------|-------------|---------------|
| Critical | Remote code execution, authentication bypass | 24 hours |
| High | Data exposure, privilege escalation | 72 hours |
| Medium | Information disclosure, denial of service | 1 week |
| Low | Minor issues, best practices | 2 weeks |

## Security Checklist

### Before Deployment

- [ ] Change default passwords
- [ ] Enable HTTPS
- [ ] Configure firewall
- [ ] Set secure JWT secret
- [ ] Enable audit logging
- [ ] Configure backup strategy
- [ ] Review user permissions
- [ ] Test security features

### Regular Maintenance

- [ ] Update software regularly
- [ ] Review access logs
- [ ] Monitor for suspicious activity
- [ ] Test backup restoration
- [ ] Review user accounts
- [ ] Update SSL certificates
- [ ] Review firewall rules
- [ ] Conduct security audits

## Incident Response

### Steps to Take

1. **Identify**: Identify the security incident
2. **Contain**: Contain the incident
3. **Eradicate**: Remove the threat
4. **Recover**: Restore normal operations
5. **Learn**: Document lessons learned

### Contact Information

- **Security Team**: security@hitechcloud.vn
- **Emergency**: +84 xxx xxx xxx
- **GitHub Security**: https://github.com/hitechcloud-vietnam/vkai-panel/security

## Bug Bounty Program

We currently do not have a formal bug bounty program. However, we appreciate security researchers who responsibly disclose vulnerabilities and will acknowledge their contributions in our release notes.

## Security Resources

### Documentation

- [Security Guide](docs/SECURITY.md)
- [Deployment Guide](docs/DEPLOYMENT.md)
- [Configuration Guide](docs/CONFIGURATION.md)

### External Resources

- [OWASP Top Ten](https://owasp.org/www-project-top-ten/)
- [CWE/SANS Top 25](https://cwe.mitre.org/top25/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)

## Contact

- **Security Email**: security@hitechcloud.vn
- **General Email**: support@hitechcloud.vn
- **Website**: https://hitechcloud.vn
- **GitHub**: https://github.com/hitechcloud-vietnam/vkai-panel

---

**Last Updated**: January 2024
