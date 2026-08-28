# VKAI Panel API Documentation

## Overview

The VKAI Panel API is a RESTful API that provides programmatic access to all panel functionality.

**Base URL**: `https://<your-server>:8888/<entrance>/api/v1`

The API is served on the panel port (`VKAI_PANEL_PORT`, default `8888`) behind
the security entrance, never on 80/443. Port `30110` is the internal listener
and is bound to localhost. Run `vkai panel info` on the server to print the
current port and entrance.

From the server itself the internal listener can be called directly:
`http://127.0.0.1:30110/api/v1`.

**Authentication**: JWT Bearer Token

**Health endpoints**: `/health` is the canonical liveness probe, with
`/api/v1/health` as an alias of it; `/ready` and `/live` complete the set. All of
them answer without the security entrance, so a load balancer or a container
health check never needs the secret. The probe path deliberately sits outside
`/api/v1` so it does not move when the API version does.

```json
GET /health
{"success":true,"data":{"status":"healthy","service":"vkai-panel","version":"0.5.0","time":"..."},"request_id":"..."}
```

`version` is the release of the running binary, stamped at link time from the
repository `VERSION` file.

---

## Authentication

### Login

```http
POST /api/v1/auth/login
```

**Request Body**:
```json
{
  "username": "admin",
  "password": "your-admin-password"
}
```

**Response**:
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 900
}
```

### Register

```http
POST /api/v1/auth/register
```

**Request Body**:
```json
{
  "username": "newuser",
  "email": "user@example.com",
  "password": "securepassword",
  "first_name": "John",
  "last_name": "Doe"
}
```

### Refresh Token

```http
POST /api/v1/auth/refresh
```

**Request Body**:
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

---

## Headers

All authenticated requests must include:

```http
Authorization: Bearer <access_token>
Content-Type: application/json
X-Tenant-ID: <tenant_id>  # Optional, for multi-tenant
```

---

## Users

### List Users

```http
GET /api/v1/users
```

**Query Parameters**:
- `page` (int): Page number (default: 1)
- `per_page` (int): Items per page (default: 20)
- `search` (string): Search term

### Get User

```http
GET /api/v1/users/:id
```

### Create User

```http
POST /api/v1/users
```

**Request Body**:
```json
{
  "username": "newuser",
  "email": "user@example.com",
  "password": "securepassword",
  "first_name": "John",
  "last_name": "Doe",
  "role_id": 1
}
```

### Update User

```http
PUT /api/v1/users/:id
```

### Delete User

```http
DELETE /api/v1/users/:id
```

---

## Tenants

### List Tenants

```http
GET /api/v1/tenants
```

### Get Tenant

```http
GET /api/v1/tenants/:id
```

### Create Tenant

```http
POST /api/v1/tenants
```

**Request Body**:
```json
{
  "name": "My Company",
  "slug": "my-company",
  "plan": "professional",
  "max_servers": 10,
  "max_websites": 100
}
```

### Update Tenant

```http
PUT /api/v1/tenants/:id
```

### Delete Tenant

```http
DELETE /api/v1/tenants/:id
```

---

## Servers

### List Servers

```http
GET /api/v1/servers
```

### Get Server

```http
GET /api/v1/servers/:id
```

### Create Server

```http
POST /api/v1/servers
```

**Request Body**:
```json
{
  "name": "Web Server 1",
  "hostname": "web1.example.com",
  "ip_address": "192.168.1.100",
  "ssh_port": 22,
  "os": "Ubuntu 22.04",
  "role": "web_server"
}
```

### Update Server

```http
PUT /api/v1/servers/:id
```

### Delete Server

```http
DELETE /api/v1/servers/:id
```

### Get Server Status

```http
GET /api/v1/servers/:id/status
```

**Response**:
```json
{
  "cpu_usage": 45.2,
  "ram_usage": 68.5,
  "disk_usage": 42.1,
  "network_in": 1024000,
  "network_out": 512000,
  "uptime": 864000,
  "load_average": [1.2, 1.5, 1.8]
}
```

---

## Websites

### List Websites

```http
GET /api/v1/websites
```

**Query Parameters**:
- `server_id` (int): Filter by server
- `status` (string): Filter by status (active, suspended, deleted)
- `web_server` (string): Filter by web server (nginx, apache, etc.)

### Get Website

```http
GET /api/v1/websites/:id
```

### Create Website

```http
POST /api/v1/websites
```

**Request Body**:
```json
{
  "domain": "example.com",
  "server_id": 1,
  "web_server": "nginx",
  "php_version": "8.2",
  "root_directory": "/vkai-panel/www/domains/example.com",
  "enable_ssl": true,
  "create_database": true,
  "database_name": "example_db",
  "database_user": "example_user",
  "database_password": "securepassword"
}
```

### Update Website

```http
PUT /api/v1/websites/:id
```

### Delete Website

```http
DELETE /api/v1/websites/:id
```

### Enable SSL

```http
POST /api/v1/websites/:id/ssl
```

**Request Body**:
```json
{
  "type": "letsencrypt",
  "email": "admin@example.com"
}
```

### Add Domain

```http
POST /api/v1/websites/:id/domains
```

**Request Body**:
```json
{
  "domain": "www.example.com"
}
```

### List Domains

```http
GET /api/v1/websites/:id/domains
```

### Delete Domain

```http
DELETE /api/v1/websites/:id/domains/:domain_id
```

---

## SSL Certificates

### List Certificates

```http
GET /api/v1/ssl
```

### Get Certificate

```http
GET /api/v1/ssl/:id
```

### Issue Let's Encrypt Certificate

```http
POST /api/v1/ssl/issue
```

**Request Body**:
```json
{
  "domain": "example.com",
  "email": "admin@example.com",
  "webroot": "/vkai-panel/www/domains/example.com"
}
```

### Upload Custom Certificate

```http
POST /api/v1/ssl/upload
```

**Request Body** (multipart/form-data):
- `certificate`: Certificate file
- `private_key`: Private key file
- `domain`: Domain name

### Renew All Certificates

```http
POST /api/v1/ssl/renew
```

### Get Expiring Soon

```http
GET /api/v1/ssl/expiring
```

**Query Parameters**:
- `days` (int): Days until expiration (default: 30)

---

## Databases

### List Database Servers

```http
GET /api/v1/databases/servers
```

### Create Database Server

```http
POST /api/v1/databases/servers
```

**Request Body**:
```json
{
  "name": "MySQL Server",
  "type": "mysql",
  "host": "localhost",
  "port": 3306,
  "admin_user": "root",
  "admin_password": "rootpassword"
}
```

### List Databases

```http
GET /api/v1/databases
```

**Query Parameters**:
- `server_id` (int): Filter by server

### Create Database

```http
POST /api/v1/databases
```

**Request Body**:
```json
{
  "server_id": 1,
  "name": "my_database",
  "user": "db_user",
  "password": "db_password",
  "charset": "utf8mb4"
}
```

### Delete Database

```http
DELETE /api/v1/databases/:id
```

### Change Password

```http
POST /api/v1/databases/:id/password
```

**Request Body**:
```json
{
  "new_password": "newsecurepassword"
}
```

---

## Cron Jobs

### List Cron Jobs

```http
GET /api/v1/cron
```

### Get Cron Job

```http
GET /api/v1/cron/:id
```

### Create Cron Job

```http
POST /api/v1/cron
```

**Request Body**:
```json
{
  "name": "Backup Database",
  "command": "/usr/local/bin/backup-db.sh",
  "schedule": "0 2 * * *",
  "type": "shell",
  "enabled": true
}
```

### Update Cron Job

```http
PUT /api/v1/cron/:id
```

### Delete Cron Job

```http
DELETE /api/v1/cron/:id
```

### Toggle Status

```http
POST /api/v1/cron/:id/toggle
```

### Run Now

```http
POST /api/v1/cron/:id/run
```

---

## Firewall

### List Rules

```http
GET /api/v1/firewall
```

### Get Rule

```http
GET /api/v1/firewall/:id
```

### Create Rule

```http
POST /api/v1/firewall
```

**Request Body**:
```json
{
  "name": "Allow HTTP",
  "protocol": "tcp",
  "port": 80,
  "action": "allow",
  "source": "0.0.0.0/0",
  "direction": "in"
}
```

### Update Rule

```http
PUT /api/v1/firewall/:id
```

### Delete Rule

```http
DELETE /api/v1/firewall/:id
```

### Get Active Rules

```http
GET /api/v1/firewall/active
```

### Save Rules

```http
POST /api/v1/firewall/save
```

---

## Backups

### List Backup Jobs

```http
GET /api/v1/backups/jobs
```

### Create Backup Job

```http
POST /api/v1/backups/jobs
```

**Request Body**:
```json
{
  "name": "Daily Website Backup",
  "type": "website",
  "source_id": 1,
  "schedule": "0 3 * * *",
  "destination": "local",
  "retention_days": 30,
  "enabled": true
}
```

### Run Backup

```http
POST /api/v1/backups/jobs/:id/run
```

### List Backup Records

```http
GET /api/v1/backups/records
```

**Query Parameters**:
- `job_id` (int): Filter by job

### Restore Backup

```http
POST /api/v1/backups/records/:id/restore
```

### Delete Backup Record

```http
DELETE /api/v1/backups/records/:id
```

---

## Services

### List Services

```http
GET /api/v1/services
```

### Get Service Status

```http
GET /api/v1/services/:name
```

**Response**:
```json
{
  "name": "nginx",
  "status": "active",
  "enabled": true,
  "pid": 1234,
  "memory": 52428800,
  "uptime": 86400
}
```

### Start Service

```http
POST /api/v1/services/:name/start
```

### Stop Service

```http
POST /api/v1/services/:name/stop
```

### Restart Service

```http
POST /api/v1/services/:name/restart
```

### Enable Service

```http
POST /api/v1/services/:name/enable
```

### Disable Service

```http
POST /api/v1/services/:name/disable
```

### Get Service Logs

```http
GET /api/v1/services/:name/logs
```

**Query Parameters**:
- `lines` (int): Number of lines (default: 100)
- `since` (string): Time duration (e.g., "1h", "24h")

### Create Service

```http
POST /api/v1/services
```

**Request Body**:
```json
{
  "name": "my-app",
  "description": "My Application",
  "exec_start": "/usr/bin/node /opt/my-app/server.js",
  "working_directory": "/opt/my-app",
  "user": "myapp",
  "environment": {
    "NODE_ENV": "production",
    "PORT": "3000"
  },
  "restart": "always",
  "enabled": true
}
```

---

## File Manager

### List Files

```http
GET /api/v1/files/list
```

**Query Parameters**:
- `path` (string): Directory path (default: "/")

### Read File

```http
GET /api/v1/files/read
```

**Query Parameters**:
- `path` (string): File path

### Write File

```http
POST /api/v1/files/write
```

**Request Body**:
```json
{
  "path": "/vkai-panel/www/domains/example.com/index.html",
  "content": "<html>...</html>"
}
```

### Create Directory

```http
POST /api/v1/files/mkdir
```

**Request Body**:
```json
{
  "path": "/vkai-panel/www/domains/example.com/new-site"
}
```

### Delete

```http
POST /api/v1/files/delete
```

**Request Body**:
```json
{
  "path": "/vkai-panel/www/domains/example.com/old-file.txt"
}
```

### Rename

```http
POST /api/v1/files/rename
```

**Request Body**:
```json
{
  "old_path": "/vkai-panel/www/domains/example.com/old-name.txt",
  "new_path": "/vkai-panel/www/domains/example.com/new-name.txt"
}
```

### Copy

```http
POST /api/v1/files/copy
```

**Request Body**:
```json
{
  "source": "/vkai-panel/www/domains/example.com/file.txt",
  "destination": "/vkai-panel/www/backup/file.txt"
}
```

### Change Permissions

```http
POST /api/v1/files/chmod
```

**Request Body**:
```json
{
  "path": "/vkai-panel/www/domains/example.com/file.txt",
  "mode": "0755"
}
```

### Upload File

```http
POST /api/v1/files/upload
```

**Request Body** (multipart/form-data):
- `file`: File to upload
- `path`: Destination directory

### Download File

```http
GET /api/v1/files/download
```

**Query Parameters**:
- `path` (string): File path

### Search Files

```http
GET /api/v1/files/search
```

**Query Parameters**:
- `path` (string): Search directory
- `query` (string): Search term

### Get Disk Usage

```http
GET /api/v1/files/disk-usage
```

**Query Parameters**:
- `path` (string): Directory path

---

## DNS

### List Zones

```http
GET /api/v1/dns/zones
```

### Create Zone

```http
POST /api/v1/dns/zones
```

**Request Body**:
```json
{
  "name": "example.com",
  "type": "master",
  "nameservers": ["ns1.example.com", "ns2.example.com"]
}
```

### List Records

```http
GET /api/v1/dns/zones/:zone_id/records
```

### Create Record

```http
POST /api/v1/dns/zones/:zone_id/records
```

**Request Body**:
```json
{
  "name": "www",
  "type": "A",
  "value": "192.168.1.100",
  "ttl": 3600
}
```

### Update Record

```http
PUT /api/v1/dns/zones/:zone_id/records/:id
```

### Delete Record

```http
DELETE /api/v1/dns/zones/:zone_id/records/:id
```

---

## Error Responses

All error responses follow this format:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid request parameters",
    "details": [
      {
        "field": "email",
        "message": "Invalid email format"
      }
    ]
  }
}
```

### Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `VALIDATION_ERROR` | 400 | Invalid request parameters |
| `UNAUTHORIZED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `CONFLICT` | 409 | Resource already exists |
| `INTERNAL_ERROR` | 500 | Internal server error |

---

## Pagination

List endpoints support pagination:

**Request**:
```http
GET /api/v1/websites?page=2&per_page=20
```

**Response**:
```json
{
  "data": [...],
  "pagination": {
    "page": 2,
    "per_page": 20,
    "total": 100,
    "total_pages": 5
  }
}
```

---

## Rate Limiting

API requests are rate-limited:

- **Authenticated**: 100 requests per minute
- **Unauthenticated**: 20 requests per minute

Rate limit headers:
```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640000000
```

---

## WebSocket

Real-time features use WebSocket:

```javascript
// Same origin as the API: the panel port plus the entrance.
const ws = new WebSocket('wss://your-server:8888/vkai_a1b2c3d4/ws');

ws.onopen = () => {
  ws.send(JSON.stringify({
    type: 'subscribe',
    channel: 'server:1:metrics'
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log(data);
};
```

---

## SDKs

### JavaScript/TypeScript

```bash
npm install @vkai/sdk
```

```typescript
import { VkaiClient } from '@vkai/sdk';

const client = new VkaiClient({
  baseUrl: 'https://your-server:8888/vkai_a1b2c3d4',
  apiKey: 'your-api-key'
});

const websites = await client.websites.list();
```

### Go

```go
import "github.com/hitechcloud-vietnam/vkai-panel/sdk-go"

client := vkai.NewClient("https://your-server:8888/vkai_a1b2c3d4", "your-api-key")
websites, err := client.Websites.List(ctx, nil)
```

---

## Examples

These run on the server itself against the internal listener. From outside the
server, replace `http://127.0.0.1:30110` with
`https://<your-server>:8888/<entrance>`.

### Create a Complete Website

```bash
# 1. Create website
curl -X POST http://127.0.0.1:30110/api/v1/websites \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "example.com",
    "server_id": 1,
    "web_server": "nginx",
    "php_version": "8.2",
    "enable_ssl": true,
    "create_database": true,
    "database_name": "example_db",
    "database_user": "example_user",
    "database_password": "securepassword"
  }'

# 2. Upload files
curl -X POST http://127.0.0.1:30110/api/v1/files/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@website.zip" \
  -F "path=/vkai-panel/www/domains/example.com"

# 3. Issue SSL certificate
curl -X POST http://127.0.0.1:30110/api/v1/ssl/issue \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "example.com",
    "email": "admin@example.com"
  }'
```

---

## Support

- **Documentation**: https://docs.vkai.vn
- **API Reference**: https://api.vkai.vn
- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
