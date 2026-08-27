# vKAI Panel Developer Guide

## Table of Contents

1. [Development Setup](#development-setup)
2. [Project Structure](#project-structure)
3. [Architecture](#architecture)
4. [Backend Development](#backend-development)
5. [Frontend Development](#frontend-development)
6. [Agent Development](#agent-development)
7. [Database](#database)
8. [Testing](#testing)
9. [Code Style](#code-style)
10. [Contributing](#contributing)
11. [API Design](#api-design)
12. [Security](#security)

---

## Development Setup

### Prerequisites

- Go 1.22+
- Node.js 20+
- PostgreSQL 16+
- Redis 7+
- Docker (optional, for databases only)
- Git

### Quick Setup

```bash
# Clone repository
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel

# Run development setup
chmod +x setup-dev.sh
./setup-dev.sh
```

### Manual Setup

#### 1. Start Databases

```bash
# Using Docker (recommended for development)
docker-compose -f docker-compose.dev.yml up -d

# Or install locally
# PostgreSQL
sudo apt install postgresql postgresql-contrib
sudo systemctl start postgresql

# Redis
sudo apt install redis-server
sudo systemctl start redis-server
```

#### 2. Setup Backend

```bash
cd backend

# Install dependencies
go mod tidy

# Create .env file
cp .env.example .env
# Edit .env with your database credentials

# Run migrations
go run cmd/migrate/main.go

# Start server
go run cmd/api/main.go
```

#### 3. Setup Frontend

```bash
cd frontend

# Install dependencies
npm install

# Create .env.local
cp .env.example .env.local
# Edit .env.local

# Start development server
npm run dev
```

#### 4. Setup Agent (Optional)

```bash
cd agent

# Install dependencies
go mod tidy

# Build
go build -o vkaid cmd/main.go

# Run
./vkaid
```

---

## Project Structure

```
vkai-panel/
├── backend/                    # Go backend
│   ├── cmd/                   # Entry points
│   │   ├── api/              # API server
│   │   └── migrate/          # Database migrations
│   ├── internal/             # Internal packages
│   │   ├── config/          # Configuration
│   │   ├── handler/         # HTTP handlers
│   │   ├── middleware/      # Middleware
│   │   ├── models/         # Data models
│   │   ├── repository/     # Database access
│   │   ├── service/        # Business logic
│   │   └── webserver/      # Web server adapters
│   ├── migrations/          # SQL migrations
│   ├── go.mod
│   └── go.sum
├── frontend/                   # Next.js frontend
│   ├── src/
│   │   ├── app/             # App router
│   │   ├── components/      # React components
│   │   ├── hooks/           # Custom hooks
│   │   ├── lib/             # Utilities
│   │   └── stores/          # Zustand stores
│   ├── public/              # Static assets
│   ├── package.json
│   └── tsconfig.json
├── agent/                      # System agent
│   ├── cmd/                  # Entry point
│   └── internal/             # Agent logic
├── deploy/                     # Deployment files
│   ├── systemd/             # Systemd services
│   ├── install.sh           # Installation script
│   └── vkai.sh              # Management script
├── docs/                      # Documentation
├── docker-compose.dev.yml     # Development databases
├── setup-dev.sh              # Development setup
└── README.md
```

---

## Architecture

### Backend Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        HTTP Request                         │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         Middleware                          │
│  (CORS, Auth, Tenant, Rate Limit, Request ID, Logging)     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                          Handler                            │
│              (Parse request, validate, respond)             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                          Service                            │
│               (Business logic, validation)                  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        Repository                           │
│              (Database operations, queries)                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         Database                            │
│                    (PostgreSQL, Redis)                      │
└─────────────────────────────────────────────────────────────┘
```

### Frontend Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Page                                │
│                    (Next.js App Router)                     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       Component                             │
│              (React components, UI logic)                   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         Hook                                │
│              (Custom hooks, state management)               │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         Store                               │
│              (Zustand stores, global state)                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       API Client                            │
│              (Axios, fetch, API calls)                      │
└─────────────────────────────────────────────────────────────┘
```

---

## Backend Development

### Adding a New Feature

1. **Create Model** (`internal/models/models.go`)
   ```go
   type MyFeature struct {
       ID        int64     `db:"id" json:"id"`
       TenantID  int64     `db:"tenant_id" json:"tenant_id"`
       Name      string    `db:"name" json:"name"`
       CreatedAt time.Time `db:"created_at" json:"created_at"`
       UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
   }
   ```

2. **Create Repository** (`internal/repository/my_feature.go`)
   ```go
   type MyFeatureRepository struct {
       db *sqlx.DB
   }
   
   func NewMyFeatureRepository(db *sqlx.DB) *MyFeatureRepository {
       return &MyFeatureRepository{db: db}
   }
   
   func (r *MyFeatureRepository) Create(ctx context.Context, feature *models.MyFeature) error {
       query := `INSERT INTO my_features (tenant_id, name) VALUES ($1, $2) RETURNING id, created_at, updated_at`
       return r.db.QueryRowContext(ctx, query, feature.TenantID, feature.Name).Scan(&feature.ID, &feature.CreatedAt, &feature.UpdatedAt)
   }
   ```

3. **Create Service** (`internal/service/my_feature.go`)
   ```go
   type MyFeatureService struct {
       repo *repository.MyFeatureRepository
   }
   
   func NewMyFeatureService(repo *repository.MyFeatureRepository) *MyFeatureService {
       return &MyFeatureService{repo: repo}
   }
   
   func (s *MyFeatureService) Create(ctx context.Context, tenantID int64, req *CreateMyFeatureRequest) (*models.MyFeature, error) {
       feature := &models.MyFeature{
           TenantID: tenantID,
           Name:     req.Name,
       }
       if err := s.repo.Create(ctx, feature); err != nil {
           return nil, err
       }
       return feature, nil
   }
   ```

4. **Create Handler** (`internal/handler/my_feature.go`)
   ```go
   type MyFeatureHandler struct {
       service *service.MyFeatureService
   }
   
   func NewMyFeatureHandler(service *service.MyFeatureService) *MyFeatureHandler {
       return &MyFeatureHandler{service: service}
   }
   
   func (h *MyFeatureHandler) Create(c *gin.Context) {
       tenantID := c.GetInt64("tenant_id")
       
       var req service.CreateMyFeatureRequest
       if err := c.ShouldBindJSON(&req); err != nil {
           c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
           return
       }
       
       feature, err := h.service.Create(c.Request.Context(), tenantID, &req)
       if err != nil {
           c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
           return
       }
       
       c.JSON(http.StatusCreated, feature)
   }
   ```

5. **Register Routes** (`internal/handler/router.go`)
   ```go
   func (r *Router) setupMyFeatureRoutes() {
       myFeature := r.api.Group("/my-features")
       myFeature.Use(r.middleware.Auth())
       myFeature.Use(r.middleware.Tenant())
       
       myFeature.POST("", r.handlers.MyFeature.Create)
       myFeature.GET("", r.handlers.MyFeature.List)
       myFeature.GET("/:id", r.handlers.MyFeature.Get)
       myFeature.PUT("/:id", r.handlers.MyFeature.Update)
       myFeature.DELETE("/:id", r.handlers.MyFeature.Delete)
   }
   ```

6. **Initialize in Main** (`cmd/api/main.go`)
   ```go
   // Initialize repository
   myFeatureRepo := repository.NewMyFeatureRepository(db)
   
   // Initialize service
   myFeatureService := service.NewMyFeatureService(myFeatureRepo)
   
   // Initialize handler
   myFeatureHandler := handler.NewMyFeatureHandler(myFeatureService)
   
   // Pass to router
   router := handler.NewRouter(handler.Handlers{
       MyFeature: myFeatureHandler,
       // ... other handlers
   })
   ```

### Database Migrations

Create migration files in `migrations/`:

```sql
-- migrations/002_add_my_features.sql
CREATE TABLE my_features (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_my_features_tenant_id ON my_features(tenant_id);
```

Run migrations:
```bash
go run cmd/migrate/main.go
```

---

## Frontend Development

### Adding a New Page

1. **Create Page** (`src/app/dashboard/my-features/page.tsx`)
   ```tsx
   'use client';
   
   import { useEffect, useState } from 'react';
   import { useMyFeatures } from '@/hooks/useMyFeatures';
   
   export default function MyFeaturesPage() {
     const { features, loading, error, fetchFeatures } = useMyFeatures();
     
     useEffect(() => {
       fetchFeatures();
     }, [fetchFeatures]);
     
     if (loading) return <div>Loading...</div>;
     if (error) return <div>Error: {error}</div>;
     
     return (
       <div>
         <h1>My Features</h1>
         {/* Feature list */}
       </div>
     );
   }
   ```

2. **Create Hook** (`src/hooks/useMyFeatures.ts`)
   ```tsx
   import { create } from 'zustand';
   import { apiClient } from '@/lib/api';
   
   interface MyFeature {
     id: number;
     name: string;
     created_at: string;
   }
   
   interface MyFeaturesStore {
     features: MyFeature[];
     loading: boolean;
     error: string | null;
     fetchFeatures: () => Promise<void>;
     createFeature: (name: string) => Promise<void>;
   }
   
   export const useMyFeatures = create<MyFeaturesStore>((set) => ({
     features: [],
     loading: false,
     error: null,
     
     fetchFeatures: async () => {
       set({ loading: true, error: null });
       try {
         const response = await apiClient.get('/my-features');
         set({ features: response.data, loading: false });
       } catch (error) {
         set({ error: 'Failed to fetch features', loading: false });
       }
     },
     
     createFeature: async (name: string) => {
       try {
         await apiClient.post('/my-features', { name });
         // Refresh list
         const response = await apiClient.get('/my-features');
         set({ features: response.data });
       } catch (error) {
         set({ error: 'Failed to create feature' });
       }
     },
   }));
   ```

3. **Create Component** (`src/components/MyFeatureList.tsx`)
   ```tsx
   'use client';
   
   import { useMyFeatures } from '@/hooks/useMyFeatures';
   
   export function MyFeatureList() {
     const { features, loading, error } = useMyFeatures();
     
     if (loading) return <div>Loading...</div>;
     if (error) return <div>Error: {error}</div>;
     
     return (
       <div className="space-y-4">
         {features.map((feature) => (
           <div key={feature.id} className="p-4 border rounded">
             <h3>{feature.name}</h3>
             <p>Created: {new Date(feature.created_at).toLocaleDateString()}</p>
           </div>
         ))}
       </div>
     );
   }
   ```

### API Client

The API client is in `src/lib/api.ts`:

```typescript
import axios from 'axios';

export const apiClient = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL || 'http://localhost:30110',
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add auth token to requests
apiClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Handle auth errors
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('access_token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);
```

---

## Agent Development

### Agent Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        vKAI Agent                           │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
              ▼               ▼               ▼
        ┌──────────┐   ┌──────────┐   ┌──────────┐
        │ System   │   │ Service  │   │ Command  │
        │ Info     │   │ Manager  │   │ Executor │
        └──────────┘   └──────────┘   └──────────┘
              │               │               │
              ▼               ▼               ▼
        ┌──────────┐   ┌──────────┐   ┌──────────┐
        │ Heartbeat│   │ systemd  │   │ Shell    │
        │ Reporter │   │ Control  │   │ Commands │
        └──────────┘   └──────────┘   └──────────┘
```

### Adding a New Agent Feature

1. **Create Feature** (`internal/features/my_feature.go`)
   ```go
   package features
   
   type MyFeature struct {
       // Feature configuration
   }
   
   func NewMyFeature() *MyFeature {
       return &MyFeature{}
   }
   
   func (f *MyFeature) Execute(params map[string]interface{}) (interface{}, error) {
       // Implementation
       return nil, nil
   }
   ```

2. **Register Feature** (`cmd/main.go`)
   ```go
   func main() {
       // ... initialization
       
       // Register features
       agent.RegisterFeature("my_feature", features.NewMyFeature())
       
       // ... start agent
   }
   ```

---

## Database

### Schema Design

Follow these conventions:

1. **Table Names**: Plural, snake_case (e.g., `websites`, `database_entries`)
2. **Column Names**: snake_case (e.g., `created_at`, `tenant_id`)
3. **Primary Keys**: `id` (BIGSERIAL)
4. **Foreign Keys**: `{table}_id` (e.g., `tenant_id`, `server_id`)
5. **Timestamps**: `created_at`, `updated_at` (TIMESTAMP WITH TIME ZONE)
6. **Soft Deletes**: `deleted_at` (nullable TIMESTAMP)

### Example Table

```sql
CREATE TABLE websites (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    server_id BIGINT NOT NULL REFERENCES servers(id),
    domain VARCHAR(255) NOT NULL,
    root_directory VARCHAR(500),
    web_server VARCHAR(50) NOT NULL DEFAULT 'nginx',
    php_version VARCHAR(20),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX idx_websites_tenant_id ON websites(tenant_id);
CREATE INDEX idx_websites_server_id ON websites(server_id);
CREATE INDEX idx_websites_domain ON websites(domain);
CREATE INDEX idx_websites_status ON websites(status);
```

### Migrations

Migration files are in `backend/migrations/`:

```
migrations/
├── 001_initial_schema.sql
├── 002_add_websites.sql
├── 003_add_databases.sql
└── ...
```

Run migrations:
```bash
cd backend
go run cmd/migrate/main.go
```

---

## Testing

### Backend Tests

```bash
cd backend

# Run all tests
go test ./...

# Run specific package tests
go test ./internal/service/...

# Run with coverage
go test -cover ./...

# Run with verbose output
go test -v ./...
```

### Test Structure

```go
// internal/service/website_test.go
package service_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

type MockWebsiteRepository struct {
    mock.Mock
}

func (m *MockWebsiteRepository) Create(ctx context.Context, website *models.Website) error {
    args := m.Called(ctx, website)
    return args.Error(0)
}

func TestWebsiteService_Create(t *testing.T) {
    mockRepo := new(MockWebsiteRepository)
    service := NewWebsiteService(mockRepo)
    
    mockRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
    
    website, err := service.Create(context.Background(), 1, &CreateWebsiteRequest{
        Domain: "example.com",
    })
    
    assert.NoError(t, err)
    assert.NotNil(t, website)
    mockRepo.AssertExpectations(t)
}
```

### Frontend Tests

```bash
cd frontend

# Run tests
npm test

# Run with coverage
npm run test:coverage

# Run in watch mode
npm run test:watch
```

### Test Structure

```tsx
// src/components/MyComponent.test.tsx
import { render, screen, fireEvent } from '@testing-library/react';
import { MyComponent } from './MyComponent';

describe('MyComponent', () => {
  it('renders correctly', () => {
    render(<MyComponent />);
    expect(screen.getByText('My Component')).toBeInTheDocument();
  });
  
  it('handles click events', () => {
    const handleClick = jest.fn();
    render(<MyComponent onClick={handleClick} />);
    fireEvent.click(screen.getByRole('button'));
    expect(handleClick).toHaveBeenCalled();
  });
});
```

---

## Code Style

### Go

Follow [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments):

```go
// Good
func (s *UserService) GetUser(ctx context.Context, id int64) (*models.User, error) {
    user, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    return user, nil
}

// Bad
func (s *UserService) GetUser(ctx context.Context, id int64) (*models.User, error) {
    user, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, err
    }
    return user, nil
}
```

### TypeScript/React

Follow [TypeScript Style Guide](https://typescript-eslint.io/):

```tsx
// Good
interface UserProps {
  name: string;
  email: string;
  onUpdate: (user: User) => void;
}

export function UserCard({ name, email, onUpdate }: UserProps) {
  return (
    <div className="p-4 border rounded">
      <h2>{name}</h2>
      <p>{email}</p>
    </div>
  );
}

// Bad
export function UserCard(props: any) {
  return (
    <div>
      <h2>{props.name}</h2>
      <p>{props.email}</p>
    </div>
  );
}
```

### Commits

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add user authentication
fix: resolve database connection issue
docs: update API documentation
style: format code with prettier
refactor: extract user service
test: add unit tests for user service
chore: update dependencies
```

---

## Contributing

### Workflow

1. **Fork** the repository
2. **Create** a feature branch
   ```bash
   git checkout -b feature/my-feature
   ```
3. **Make** your changes
4. **Write** tests
5. **Commit** your changes
   ```bash
   git commit -m "feat: add my feature"
   ```
6. **Push** to your fork
   ```bash
   git push origin feature/my-feature
   ```
7. **Create** a pull request

### Pull Request Guidelines

1. **Title**: Clear, descriptive title
2. **Description**: Explain what and why
3. **Tests**: Include tests for new features
4. **Documentation**: Update docs if needed
5. **Code Style**: Follow project conventions
6. **No Breaking Changes**: Unless discussed

### Code Review

All pull requests require:

1. **Passing tests**
2. **Code review approval**
3. **No merge conflicts**
4. **Up-to-date with main**

---

## API Design

### RESTful Conventions

```
GET    /api/v1/resources          # List resources
POST   /api/v1/resources          # Create resource
GET    /api/v1/resources/:id      # Get resource
PUT    /api/v1/resources/:id      # Update resource
DELETE /api/v1/resources/:id      # Delete resource
```

### Response Format

```json
{
  "data": { ... },
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 100
  }
}
```

### Error Format

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

---

## Security

### Authentication

- JWT tokens with short expiration
- Refresh token rotation
- Secure token storage

### Authorization

- Role-based access control (RBAC)
- Tenant isolation
- Permission checks at service layer

### Input Validation

- Validate all input
- Sanitize user input
- Use parameterized queries

### Secrets

- Never commit secrets
- Use environment variables
- Encrypt sensitive data

---

## Resources

- [Go Documentation](https://go.dev/doc/)
- [Next.js Documentation](https://nextjs.org/docs)
- [TypeScript Documentation](https://www.typescriptlang.org/docs/)
- [Tailwind CSS Documentation](https://tailwindcss.com/docs)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Redis Documentation](https://redis.io/documentation)

---

## Support

- **Documentation**: https://docs.vkai.vn
- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Discussions**: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- **Email**: dev@hitechcloud.vn
