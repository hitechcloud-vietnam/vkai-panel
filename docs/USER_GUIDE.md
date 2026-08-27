# vKAI Panel User Guide

## Table of Contents

1. [Getting Started](#getting-started)
2. [Dashboard](#dashboard)
3. [Server Management](#server-management)
4. [Website Management](#website-management)
5. [Database Management](#database-management)
6. [SSL Certificates](#ssl-certificates)
7. [File Manager](#file-manager)
8. [Cron Jobs](#cron-jobs)
9. [Firewall](#firewall)
10. [Backups](#backups)
11. [Service Management](#service-management)
12. [DNS Management](#dns-management)
13. [User Management](#user-management)
14. [Settings](#settings)
15. [Troubleshooting](#troubleshooting)

---

## Getting Started

### First Login

1. Open your browser and navigate to `http://your-server-ip`
2. Enter the default credentials:
   - **Username**: `admin`
   - **Password**: `admin123`
3. You will be prompted to change your password immediately
4. Complete the initial setup wizard

### Initial Setup Wizard

The setup wizard will guide you through:

1. **Change Password**: Set a strong password
2. **Profile Setup**: Enter your name and email
3. **Server Registration**: Add your first server
4. **Basic Configuration**: Set timezone, language, etc.

---

## Dashboard

The dashboard provides an overview of your infrastructure:

### Server Status

- **Online Servers**: Number of servers currently online
- **Total Servers**: Total number of registered servers
- **CPU Usage**: Average CPU usage across all servers
- **RAM Usage**: Average RAM usage across all servers
- **Disk Usage**: Average disk usage across all servers

### Quick Actions

- **Add Server**: Register a new server
- **Create Website**: Create a new website
- **Create Database**: Create a new database
- **Issue SSL**: Issue an SSL certificate

### Recent Activity

- Latest actions performed
- System notifications
- Error alerts

---

## Server Management

### Adding a Server

1. Navigate to **Servers** → **Add Server**
2. Fill in the server details:
   - **Name**: A friendly name for the server
   - **Hostname**: The server's hostname
   - **IP Address**: The server's IP address
   - **SSH Port**: SSH port (default: 22)
   - **OS**: Operating system
   - **Role**: Server role (Web, Database, etc.)
3. Click **Add Server**
4. Install the vKAI Agent on the server:
   ```bash
   curl -sSL https://install.vkai.vn/agent.sh | bash
   ```

### Server Details

Click on a server to view:

- **Overview**: CPU, RAM, Disk, Network usage
- **Websites**: Websites hosted on this server
- **Services**: Running services
- **Databases**: Databases on this server
- **Firewall**: Firewall rules
- **Logs**: System logs
- **Terminal**: Web terminal access

### Server Actions

- **Start/Stop/Restart**: Control server services
- **Reboot**: Reboot the server
- **Delete**: Remove the server from the panel

---

## Website Management

### Creating a Website

1. Navigate to **Websites** → **Add Website**
2. Fill in the website details:
   - **Domain**: Primary domain (e.g., `example.com`)
   - **Server**: Select the server
   - **Web Server**: Select web server (Nginx, Apache, etc.)
   - **PHP Version**: Select PHP version (if applicable)
   - **Root Directory**: Document root (default: `/var/www/domain`)
3. Optional settings:
   - **Enable SSL**: Issue Let's Encrypt certificate
   - **Create Database**: Create a database for the website
   - **Database Name**: Database name
   - **Database User**: Database username
   - **Database Password**: Database password
4. Click **Create Website**

### Website Details

Click on a website to view:

- **Overview**: Website status, domain, web server, PHP version
- **Files**: File manager for the website
- **Database**: Database management
- **SSL**: SSL certificate management
- **DNS**: DNS records
- **PHP**: PHP configuration
- **Logs**: Access and error logs
- **Cron**: Cron jobs for this website
- **Backups**: Backup history

### Website Actions

- **Enable/Disable**: Enable or disable the website
- **Delete**: Delete the website and its files
- **Clone**: Clone the website
- **Backup**: Create a backup
- **Restore**: Restore from backup

### Adding Domains

1. Go to website details → **Domains**
2. Click **Add Domain**
3. Enter the domain name
4. Click **Add**

### Enabling SSL

1. Go to website details → **SSL**
2. Click **Issue Certificate**
3. Select certificate type:
   - **Let's Encrypt**: Free, automatic certificate
   - **Custom**: Upload your own certificate
4. Click **Issue**

---

## Database Management

### Database Servers

1. Navigate to **Databases** → **Servers**
2. Click **Add Server**
3. Fill in the details:
   - **Name**: Server name
   - **Type**: MySQL, PostgreSQL, or Redis
   - **Host**: Server hostname
   - **Port**: Server port
   - **Admin User**: Admin username
   - **Admin Password**: Admin password
4. Click **Add Server**

### Creating a Database

1. Navigate to **Databases** → **Databases**
2. Click **Create Database**
3. Fill in the details:
   - **Server**: Select database server
   - **Name**: Database name
   - **User**: Database username
   - **Password**: Database password
   - **Charset**: Character set (default: utf8mb4)
4. Click **Create Database**

### Database Actions

- **Change Password**: Change database user password
- **Delete**: Delete the database
- **Backup**: Create a database backup
- **Restore**: Restore from backup
- **Import**: Import SQL file
- **Export**: Export database

---

## SSL Certificates

### Viewing Certificates

1. Navigate to **SSL** → **Certificates**
2. View all certificates with status:
   - **Valid**: Certificate is valid
   - **Expiring**: Certificate expires within 30 days
   - **Expired**: Certificate has expired
   - **Error**: Certificate has errors

### Issuing a Certificate

1. Click **Issue Certificate**
2. Select domain
3. Enter email for Let's Encrypt notifications
4. Click **Issue**

### Uploading Custom Certificate

1. Click **Upload Certificate**
2. Upload certificate file
3. Upload private key file
4. Click **Upload**

### Renewing Certificates

- **Auto-renewal**: Certificates are automatically renewed 30 days before expiration
- **Manual renewal**: Click **Renew All** to renew all certificates

---

## File Manager

### Navigating Files

1. Navigate to **Files**
2. Browse the file system
3. Click on folders to navigate
4. Use breadcrumb navigation to go back

### File Operations

- **Upload**: Click **Upload** to upload files
- **Download**: Click on a file to download
- **Edit**: Click **Edit** to edit text files
- **Rename**: Right-click → **Rename**
- **Delete**: Right-click → **Delete**
- **Copy**: Right-click → **Copy**
- **Move**: Right-click → **Move**
- **Permissions**: Right-click → **Permissions**

### Creating Files/Folders

1. Click **New File** or **New Folder**
2. Enter the name
3. Click **Create**

### Searching Files

1. Click **Search**
2. Enter search term
3. Select search directory
4. Click **Search**

---

## Cron Jobs

### Creating a Cron Job

1. Navigate to **Cron** → **Jobs**
2. Click **Add Job**
3. Fill in the details:
   - **Name**: Job name
   - **Command**: Command to execute
   - **Schedule**: Cron schedule (e.g., `0 2 * * *` for daily at 2 AM)
   - **Type**: Shell, PHP, URL, or Node.js
   - **Enabled**: Enable/disable the job
4. Click **Add Job**

### Cron Schedule Examples

- `0 2 * * *`: Daily at 2 AM
- `0 */6 * * *`: Every 6 hours
- `0 0 * * 0`: Weekly on Sunday
- `0 0 1 * *`: Monthly on 1st
- `*/5 * * * *`: Every 5 minutes

### Job Actions

- **Run Now**: Execute the job immediately
- **Enable/Disable**: Enable or disable the job
- **Edit**: Edit job details
- **Delete**: Delete the job
- **View Logs**: View job execution logs

---

## Firewall

### Viewing Rules

1. Navigate to **Firewall** → **Rules**
2. View all firewall rules

### Creating a Rule

1. Click **Add Rule**
2. Fill in the details:
   - **Name**: Rule name
   - **Protocol**: TCP, UDP, or Both
   - **Port**: Port number or range
   - **Action**: Allow or Deny
   - **Source**: IP address or range (e.g., `0.0.0.0/0` for all)
   - **Direction**: Inbound or Outbound
3. Click **Add Rule**

### Preset Rules

- **HTTP (80)**: Allow HTTP traffic
- **HTTPS (443)**: Allow HTTPS traffic
- **SSH (22)**: Allow SSH traffic
- **DNS (53)**: Allow DNS traffic

### Rule Actions

- **Edit**: Edit rule details
- **Delete**: Delete the rule
- **Enable/Disable**: Enable or disable the rule

---

## Backups

### Creating a Backup Job

1. Navigate to **Backups** → **Jobs**
2. Click **Add Job**
3. Fill in the details:
   - **Name**: Job name
   - **Type**: Website, Database, or Files
   - **Source**: Select source to backup
   - **Schedule**: Cron schedule
   - **Destination**: Local, S3, SFTP, etc.
   - **Retention**: Days to keep backups
   - **Enabled**: Enable/disable the job
4. Click **Add Job**

### Running a Backup

1. Go to backup jobs
2. Click **Run Now** on a job
3. Monitor progress in the jobs list

### Restoring a Backup

1. Navigate to **Backups** → **Records**
2. Find the backup to restore
3. Click **Restore**
4. Confirm the restoration

### Backup Destinations

- **Local**: Store on the server
- **S3**: Amazon S3 or compatible
- **SFTP**: SFTP server
- **FTP**: FTP server

---

## Service Management

### Viewing Services

1. Navigate to **Services**
2. View all systemd services

### Service Actions

- **Start**: Start the service
- **Stop**: Stop the service
- **Restart**: Restart the service
- **Enable**: Enable service to start on boot
- **Disable**: Disable service from starting on boot
- **View Logs**: View service logs

### Creating a Custom Service

1. Click **Add Service**
2. Fill in the details:
   - **Name**: Service name
   - **Description**: Service description
   - **Exec Start**: Command to start the service
   - **Working Directory**: Working directory
   - **User**: User to run as
   - **Environment**: Environment variables
   - **Restart**: Restart policy (always, on-failure, etc.)
   - **Enabled**: Enable on boot
3. Click **Add Service**

---

## DNS Management

### DNS Zones

1. Navigate to **DNS** → **Zones**
2. Click **Add Zone**
3. Fill in the details:
   - **Name**: Domain name
   - **Type**: Master or Slave
   - **Nameservers**: Nameserver list
4. Click **Add Zone**

### DNS Records

1. Go to a zone
2. Click **Add Record**
3. Fill in the details:
   - **Name**: Record name (e.g., `www`)
   - **Type**: A, AAAA, CNAME, MX, TXT, NS, SRV
   - **Value**: Record value
   - **TTL**: Time to live
4. Click **Add Record**

### Record Types

- **A**: IPv4 address
- **AAAA**: IPv6 address
- **CNAME**: Canonical name
- **MX**: Mail exchange
- **TXT**: Text record
- **NS**: Nameserver
- **SRV**: Service record

---

## User Management

### Adding Users

1. Navigate to **Users** → **All Users**
2. Click **Add User**
3. Fill in the details:
   - **Username**: Username
   - **Email**: Email address
   - **Password**: Password
   - **First Name**: First name
   - **Last Name**: Last name
   - **Role**: User role
4. Click **Add User**

### User Roles

- **Super Admin**: Full access to everything
- **Admin**: Full access to tenant resources
- **Server Admin**: Manage servers
- **Web Admin**: Manage websites
- **Database Admin**: Manage databases
- **Developer**: Limited access
- **Operator**: Operational access
- **Viewer**: Read-only access

### User Actions

- **Edit**: Edit user details
- **Delete**: Delete the user
- **Reset Password**: Reset user password
- **Change Role**: Change user role

---

## Settings

### General Settings

- **Panel Name**: Custom panel name
- **Timezone**: Server timezone
- **Language**: Interface language
- **Theme**: Light or dark theme

### Security Settings

- **Two-Factor Authentication**: Enable 2FA
- **Session Timeout**: Session timeout duration
- **IP Whitelist**: Allowed IP addresses
- **API Keys**: Manage API keys

### Notification Settings

- **Email**: Email notifications
- **Webhook**: Webhook notifications
- **Telegram**: Telegram notifications
- **Slack**: Slack notifications

### Backup Settings

- **Default Destination**: Default backup destination
- **Retention Policy**: Default retention policy
- **Encryption**: Enable backup encryption

---

## Troubleshooting

### Common Issues

#### Website Not Loading

1. Check if the web server is running:
   ```bash
   systemctl status nginx
   ```
2. Check website configuration:
   ```bash
   nginx -t
   ```
3. Check error logs:
   ```bash
   tail -f /var/log/nginx/error.log
   ```

#### SSL Certificate Issues

1. Check certificate status:
   ```bash
   certbot certificates
   ```
2. Renew certificate:
   ```bash
   certbot renew
   ```
3. Check certificate expiration:
   ```bash
   openssl x509 -enddate -noout -in /etc/letsencrypt/live/domain/fullchain.pem
   ```

#### Database Connection Issues

1. Check database status:
   ```bash
   systemctl status postgresql
   ```
2. Test connection:
   ```bash
   psql -h localhost -U username -d database
   ```
3. Check database logs:
   ```bash
   tail -f /var/log/postgresql/postgresql-*.log
   ```

#### Service Won't Start

1. Check service status:
   ```bash
   systemctl status service-name
   ```
2. View service logs:
   ```bash
   journalctl -u service-name -n 100
   ```
3. Check configuration:
   ```bash
   service-name -t  # For nginx, apache, etc.
   ```

### Getting Help

- **Documentation**: https://docs.vkai.vn
- **Community**: https://community.vkai.vn
- **Support**: support@hitechcloud.vn
- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues

---

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl + K` | Quick search |
| `Ctrl + N` | Create new |
| `Ctrl + S` | Save |
| `Ctrl + Z` | Undo |
| `Ctrl + Shift + Z` | Redo |
| `Esc` | Close modal |
| `?` | Show shortcuts |

---

## Best Practices

### Security

1. **Change default password** immediately
2. **Enable two-factor authentication**
3. **Use strong passwords**
4. **Limit SSH access**
5. **Keep software updated**
6. **Regular backups**
7. **Monitor logs**

### Performance

1. **Optimize database queries**
2. **Use caching** (Redis, Nginx cache)
3. **Enable compression**
4. **Optimize images**
5. **Use CDN** for static assets
6. **Monitor resource usage**

### Backups

1. **Regular backups** (daily recommended)
2. **Test restores** regularly
3. **Off-site backups** (S3, SFTP)
4. **Encrypt sensitive backups**
5. **Document backup procedures**

---

## Glossary

- **Tenant**: A customer or organization
- **Server**: A physical or virtual machine
- **Website**: A web application
- **SSL**: Secure Sockets Layer
- **DNS**: Domain Name System
- **Cron**: Scheduled task
- **Firewall**: Network security system
- **Backup**: Data copy for recovery
- **Service**: Systemd service
- **Agent**: vKAI Agent (vkaid)

---

## Changelog

### Version 1.0.0

- Initial release
- Core functionality
- Website management
- Database management
- SSL management
- File manager
- Cron jobs
- Firewall
- Backups
- Service management
