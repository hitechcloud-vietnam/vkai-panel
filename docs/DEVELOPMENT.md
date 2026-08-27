# vKAI Panel Development Guide

## Table of Contents

1. [Getting Started](#getting-started)
2. [Development Environment](#development-environment)
3. [Project Structure](#project-structure)
4. [Backend Development](#backend-development)
5. [Frontend Development](#frontend-development)
6. [Agent Development](#agent-development)
7. [Database](#database)
8. [Testing](#testing)
9. [Debugging](#debugging)
10. [Code Style](#code-style)
11. [Git Workflow](#git-workflow)
12. [Common Tasks](#common-tasks)

---

## Getting Started

### Prerequisites

- **Go**: 1.22 or higher
- **Node.js**: 20 LTS or higher
- **PostgreSQL**: 16 or higher
- **Redis**: 7 or higher
- **Git**: Latest version
- **Docker**: For development databases (optional)

### Quick Setup

```bash
# Clone repository
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel

# Run setup script
chmod +x setup-dev.sh
./setup-dev.sh

# Start development
make dev
```

---

## Development Environment

### Option 1: Docker for Databases (Recommended)

```bash
# Start databases with Docker
docker-compose -f docker-compose.dev.yml up -d

# Start backend
cd backend
go run cmd/api/main.go

# Start frontend (in another terminal)
cd frontend
npm run dev
```

### Option 2: Local Databases

```bash
# Install PostgreSQL
sudo apt install postgresql postgresql-contrib

# Install Redis
sudo apt install redis-server

# Create database
sudo -u postgres createuser -P vkai
sudo -u postgres createdb -O vkai vkai_panel

# Start services
sudo systemctl start postgresql
sudo systemctl start redis-server

# Run migrations
cd backend
go run cmd/migrate/main.go

# Start backend
go run cmd/api/main.go

# Start frontend (in another terminal)
cd frontend
npm run dev
```

### IDE Setup

#### VS Code

Install recommended extensions:
- Go
- ESLint
- Prettier
- Tailwind CSS IntelliSense
- GitLens

#### GoLand/IntelliJ

1. Open project root
2. Configure Go SDK
3. Configure Node.js interpreter
4. Set up run configurations

---

## Project Structure

```
vkai-panel/
├── backend/                    # Go API server
│   ├── cmd/                   # Entry points
│   │   ├── api/              # API server
│   │   └── migrate/          # Database migrations
│   ├── internal/              # Internal packages
│   │   ├── auth/             # JWT authentication
│   │   ├── config/           # Configuration
│   │   ├── database/         # Database connections
│   │   ├── handler/          # HTTP handlers
│   │   ├── middleware/       # HTTP middleware
│   │   ├── models/           # Data models
│   │   ├── rbac/             # Role-based access control
│   │   ├── repository/       # Data access layer
│   │   ├── service/          # Business logic
│   │   ├── utils/            # Utilities
│   │   └── webserver/        # Web server adapters
│   ├── migrations/            # SQL migrations
│   └── config.yaml           # Configuration file
├── frontend/                   # Next.js frontend
│   ├── src/
│   │   ├── app/              # Next.js app router
│   │   ├── components/       # React components
│   │   ├── hooks/            # Custom hooks
│   │   ├── lib/              # Utility libraries
│   │   ├── services/         # API services
│   │   ├── store/            # Zustand stores
│   │   └── styles/           # CSS styles
│   └── package.json
├── agent/                      # vKAI Agent
│   └── cmd/main.go           # Agent entry point
├── deploy/                     # Deployment files
│   ├── systemd/              # Systemd service files
│   ├── install.sh            # Installation script
│   └── vkai.sh               # Management script
├── docs/                       # Documentation
├── scripts/                    # Utility scripts
├── docker-compose.dev.yml     # Development Docker
├── Makefile                   # Build commands
└── README.md                  # Project documentation
```

---

## Backend Development

### Architecture

```
Handler → Service → Repository → Database
   │         │          │
   │         │          └── SQL queries
   │         └── Business logic
   └── HTTP request/response
```

### Adding a New Feature

#### 1. Define Models

```go
// internal/models/models.go
type NewFeature struct {
    ID        int64     `json:"id" db:"id"`
    TenantID  int64     `json:"tenant_id" db:"tenant_id"`
    Name      string    `json:"name" db:"name"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
    UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
```

#### 2. Create Repository

```go
// internal/repository/new_feature.go
package repository

import (
    "context"
    "github.com/jmoiron/sqlx"
    "github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type NewFeatureRepository struct {
    db *sqlx.DB
}

func NewNewFeatureRepository(db *sqlx.DB) *NewFeatureRepository {
    return &NewFeatureRepository{db: db}
}

func (r *NewFeatureRepository) Create(ctx context.Context, feature *models.NewFeature) error {
    query := `
        INSERT INTO new_features (tenant_id, name)
        VALUES ($1, $2)
        RETURNING id, created_at, updated_at
    `
    return r.db.QueryRowContext(ctx, query,
        feature.TenantID,
        feature.Name,
    ).Scan(&feature.ID, &feature.CreatedAt, &feature.UpdatedAt)
}

func (r *NewFeatureRepository) GetByID(ctx context.Context, id int64) (*models.NewFeature, error) {
    var feature models.NewFeature
    query := `SELECT * FROM new_features WHERE id = $1`
    err := r.db.GetContext(ctx, &feature, query, id)
    return &feature, err
}
```

#### 3. Create Service

```go
// internal/service/new_feature.go
package service

import (
    "context"
    "github.com/hitechcloud-vietnam/vkai-panel/internal/models"
    "github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type NewFeatureService struct {
    repo *repository.NewFeatureRepository
}

func NewNewFeatureService(repo *repository.NewFeatureRepository) *NewFeatureService {
    return &NewFeatureService{repo: repo}
}

func (s *NewFeatureService) Create(ctx context.Context, tenantID int64, name string) (*models.NewFeature, error) {
    feature := &models.NewFeature{
        TenantID: tenantID,
        Name:     name,
    }
    
    if err := s.repo.Create(ctx, feature); err != nil {
        return nil, err
    }
    
    return feature, nil
}
```

#### 4. Create Handler

```go
// internal/handler/new_feature.go
package handler

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type NewFeatureHandler struct {
    service *service.NewFeatureService
}

func NewNewFeatureHandler(service *service.NewFeatureService) *NewFeatureHandler {
    return &NewFeatureHandler{service: service}
}

func (h *NewFeatureHandler) Create(c *gin.Context) {
    tenantID := c.GetInt64("tenant_id")
    
    var req struct {
        Name string `json:"name" binding:"required"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    feature, err := h.service.Create(c.Request.Context(), tenantID, req.Name)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusCreated, feature)
}
```

#### 5. Register Routes

```go
// internal/handler/router.go
func SetupRouter(...) *gin.Engine {
    // ... existing code ...
    
    // New feature routes
    newFeatureRepo := repository.NewNewFeatureRepository(db)
    newFeatureService := service.NewNewFeatureService(newFeatureRepo)
    newFeatureHandler := handler.NewNewFeatureHandler(newFeatureService)
    
    api := r.Group("/api/v1")
    {
        api.POST("/new-features", newFeatureHandler.Create)
        api.GET("/new-features/:id", newFeatureHandler.GetByID)
    }
    
    return r
}
```

#### 6. Add Database Migration

```sql
-- migrations/002_add_new_features.sql
CREATE TABLE IF NOT EXISTS new_features (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_new_features_tenant_id ON new_features(tenant_id);
```

### Configuration

```go
// internal/config/config.go
type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Database DatabaseConfig `mapstructure:"database"`
    Redis    RedisConfig    `mapstructure:"redis"`
    JWT      JWTConfig      `mapstructure:"jwt"`
    Log      LogConfig      `mapstructure:"log"`
}
```

### Middleware

```go
// internal/middleware/auth.go
func AuthMiddleware(jwtService *auth.JWTService) gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing token"})
            c.Abort()
            return
        }
        
        claims, err := jwtService.ValidateToken(token)
        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
            c.Abort()
            return
        }
        
        c.Set("user_id", claims.UserID)
        c.Set("tenant_id", claims.TenantID)
        c.Next()
    }
}
```

---

## Frontend Development

### Architecture

```
Pages → Components → Hooks → Services → API
  │         │          │         │
  │         │          │         └── HTTP requests
  │         │          └── State management
  │         └── UI components
  └── Route handlers
```

### Adding a New Feature

#### 1. Create API Service

```typescript
// src/services/new-feature.ts
import { apiClient } from '@/lib/api';

export interface NewFeature {
  id: number;
  name: string;
  created_at: string;
}

export const newFeatureService = {
  getAll: () => apiClient.get<NewFeature[]>('/new-features'),
  getById: (id: number) => apiClient.get<NewFeature>(`/new-features/${id}`),
  create: (data: { name: string }) => apiClient.post<NewFeature>('/new-features', data),
  update: (id: number, data: { name: string }) => apiClient.put<NewFeature>(`/new-features/${id}`, data),
  delete: (id: number) => apiClient.delete(`/new-features/${id}`),
};
```

#### 2. Create Zustand Store

```typescript
// src/store/new-feature.ts
import { create } from 'zustand';
import { newFeatureService, NewFeature } from '@/services/new-feature';

interface NewFeatureState {
  features: NewFeature[];
  loading: boolean;
  error: string | null;
  fetchFeatures: () => Promise<void>;
  createFeature: (name: string) => Promise<void>;
  deleteFeature: (id: number) => Promise<void>;
}

export const useNewFeatureStore = create<NewFeatureState>((set) => ({
  features: [],
  loading: false,
  error: null,
  
  fetchFeatures: async () => {
    set({ loading: true, error: null });
    try {
      const { data } = await newFeatureService.getAll();
      set({ features: data, loading: false });
    } catch (error) {
      set({ error: 'Failed to fetch features', loading: false });
    }
  },
  
  createFeature: async (name: string) => {
    try {
      await newFeatureService.create({ name });
      const { data } = await newFeatureService.getAll();
      set({ features: data });
    } catch (error) {
      set({ error: 'Failed to create feature' });
    }
  },
  
  deleteFeature: async (id: number) => {
    try {
      await newFeatureService.delete(id);
      const { data } = await newFeatureService.getAll();
      set({ features: data });
    } catch (error) {
      set({ error: 'Failed to delete feature' });
    }
  },
}));
```

#### 3. Create Custom Hook

```typescript
// src/hooks/use-new-features.ts
import { useEffect } from 'react';
import { useNewFeatureStore } from '@/store/new-feature';

export function useNewFeatures() {
  const { features, loading, error, fetchFeatures, createFeature, deleteFeature } = useNewFeatureStore();
  
  useEffect(() => {
    fetchFeatures();
  }, [fetchFeatures]);
  
  return {
    features,
    loading,
    error,
    createFeature,
    deleteFeature,
  };
}
```

#### 4. Create Component

```typescript
// src/components/NewFeatureList.tsx
'use client';

import { useNewFeatures } from '@/hooks/use-new-features';

export function NewFeatureList() {
  const { features, loading, error, deleteFeature } = useNewFeatures();
  
  if (loading) return <div>Loading...</div>;
  if (error) return <div>Error: {error}</div>;
  
  return (
    <div>
      <h2>New Features</h2>
      <ul>
        {features.map((feature) => (
          <li key={feature.id}>
            {feature.name}
            <button onClick={() => deleteFeature(feature.id)}>Delete</button>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

#### 5. Create Page

```typescript
// src/app/dashboard/new-features/page.tsx
import { NewFeatureList } from '@/components/NewFeatureList';

export default function NewFeaturesPage() {
  return (
    <div>
      <h1>New Features</h1>
      <NewFeatureList />
    </div>
  );
}
```

### Styling

```typescript
// Using Tailwind CSS
<div className="flex items-center justify-between p-4 bg-white rounded-lg shadow">
  <h2 className="text-lg font-semibold">Title</h2>
  <button className="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600">
    Action
  </button>
</div>
```

---

## Agent Development

### Architecture

```
Agent (vkaid) → System APIs → Server Resources
     │
     └── Heartbeat → API Server
```

### Adding a New Feature

#### 1. Define Command

```go
// agent/internal/command/command.go
type Command struct {
    Type    string      `json:"type"`
    Payload interface{} `json:"payload"`
}

type CommandHandler func(ctx context.Context, cmd Command) error
```

#### 2. Implement Handler

```go
// agent/internal/command/new_feature.go
func HandleNewFeature(ctx context.Context, cmd Command) error {
    // Implementation
    return nil
}
```

#### 3. Register Handler

```go
// agent/cmd/main.go
func main() {
    // ... existing code ...
    
    handlers := map[string]CommandHandler{
        "new_feature": HandleNewFeature,
    }
    
    // ... rest of code ...
}
```

---

## Database

### Migrations

```bash
# Run migrations
cd backend
go run cmd/migrate/main.go

# Create new migration
touch migrations/003_add_new_table.sql
```

### Migration Template

```sql
-- migrations/003_add_new_table.sql
-- Description: Add new table for feature X

-- Up
CREATE TABLE IF NOT EXISTS new_table (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_new_table_tenant_id ON new_table(tenant_id);

-- Down
-- DROP TABLE IF EXISTS new_table;
```

### Query Examples

```go
// Simple query
var count int
err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM websites WHERE tenant_id = $1", tenantID)

// Complex query
query := `
    SELECT w.*, s.name as server_name
    FROM websites w
    JOIN servers s ON w.server_id = s.id
    WHERE w.tenant_id = $1
    ORDER BY w.created_at DESC
    LIMIT $2 OFFSET $3
`
var websites []Website
err := db.SelectContext(ctx, &websites, query, tenantID, limit, offset)
```

---

## Testing

### Backend Tests

```bash
# Run all tests
cd backend
go test ./...

# Run specific package
go test ./internal/service/...

# Run with coverage
go test -cover ./...

# Run with verbose output
go test -v ./...
```

### Frontend Tests

```bash
# Run all tests
cd frontend
npm test

# Run with coverage
npm run test:coverage

# Run in watch mode
npm run test:watch
```

### Writing Tests

```go
// internal/service/website_test.go
func TestWebsiteService_Create(t *testing.T) {
    mockRepo := new(MockWebsiteRepository)
    service := NewWebsiteService(mockRepo)
    
    mockRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
    
    website, err := service.Create(context.Background(), 1, &CreateWebsiteRequest{
        Domain: "test.example.com",
    })
    
    assert.NoError(t, err)
    assert.NotNil(t, website)
}
```

---

## Debugging

### Backend Debugging

#### Using Delve

```bash
# Install Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug backend
cd backend
dlv debug cmd/api/main.go

# Set breakpoint
(dlv) break main.main
(dlv) continue
```

#### Using VS Code

1. Install Go extension
2. Create `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Backend",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/backend/cmd/api",
      "env": {},
      "args": []
    }
  ]
}
```

### Frontend Debugging

#### Using Browser DevTools

1. Open Chrome DevTools (F12)
2. Go to Sources tab
3. Set breakpoints in TypeScript files
4. Use Console for debugging

#### Using VS Code

1. Install JavaScript Debugger extension
2. Create `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Frontend",
      "type": "chrome",
      "request": "launch",
      "url": "http://localhost:3000",
      "webRoot": "${workspaceFolder}/frontend"
    }
  ]
}
```

---

## Code Style

### Go

```bash
# Format code
gofmt -w .

# Run linter
golangci-lint run

# Run vet
go vet ./...
```

### TypeScript/React

```bash
# Format code
npm run format

# Run linter
npm run lint

# Fix lint issues
npm run lint:fix
```

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Package | lowercase | `website` |
| Exported | CamelCase | `WebsiteService` |
| Unexported | camelCase | `getWebsite` |
| Interface | -er suffix | `Reader` |
| Constant | CamelCase | `MaxRetries` |
| Variable | camelCase | `defaultTimeout` |

---

## Git Workflow

### Branch Strategy

```
main
  └── develop
        ├── feature/your-feature
        ├── bugfix/your-bugfix
        └── hotfix/your-hotfix
```

### Commit Messages

```
<type>(<scope>): <subject>

<body>

<footer>
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `style`: Code style
- `refactor`: Refactoring
- `test`: Tests
- `chore`: Maintenance

### Pull Requests

1. Create feature branch
2. Make changes
3. Write tests
4. Update documentation
5. Create PR
6. Get review
7. Merge

---

## Common Tasks

### Add New API Endpoint

1. Define model in `models.go`
2. Create repository in `repository/`
3. Create service in `service/`
4. Create handler in `handler/`
5. Register route in `router.go`
6. Add migration if needed
7. Write tests

### Add New Frontend Page

1. Create API service in `services/`
2. Create Zustand store in `store/`
3. Create custom hook in `hooks/`
4. Create component in `components/`
5. Create page in `app/`
6. Add to navigation

### Add New Database Table

1. Create migration in `migrations/`
2. Add model in `models.go`
3. Create repository
4. Run migration

### Add New Middleware

1. Create middleware in `middleware/`
2. Register in `router.go`
3. Apply to routes

---

## Resources

- [Go Documentation](https://go.dev/doc/)
- [Next.js Documentation](https://nextjs.org/docs)
- [Tailwind CSS Documentation](https://tailwindcss.com/docs)
- [Zustand Documentation](https://docs.pmnd.rs/zustand/getting-started/introduction)
- [Gin Documentation](https://gin-gonic.com/docs/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)

---

## Support

- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Discussions**: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- **Email**: support@hitechcloud.vn
