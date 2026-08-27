# vKAI Panel Testing Guide

## Table of Contents

1. [Testing Strategy](#testing-strategy)
2. [Unit Tests](#unit-tests)
3. [Integration Tests](#integration-tests)
4. [End-to-End Tests](#end-to-end-tests)
5. [Performance Tests](#performance-tests)
6. [Security Tests](#security-tests)
7. [Test Environment](#test-environment)
8. [Running Tests](#running-tests)
9. [Writing Tests](#writing-tests)
10. [Test Coverage](#test-coverage)
11. [CI/CD Integration](#cicd-integration)

---

## Testing Strategy

### Testing Pyramid

```
         /\
        /  \        E2E Tests (Few)
       /    \       - Critical user flows
      /------\      - Cross-browser testing
     /        \     
    / Integration\   Integration Tests (Some)
   /    Tests     \  - API endpoints
  /----------------\ - Database operations
 /                  \
/    Unit Tests      \  Unit Tests (Many)
/____________________\ - Business logic
                       - Data validation
```

### Test Types

1. **Unit Tests**: Test individual functions/methods
2. **Integration Tests**: Test component interactions
3. **End-to-End Tests**: Test complete user flows
4. **Performance Tests**: Test system performance
5. **Security Tests**: Test security vulnerabilities

---

## Unit Tests

### Backend Unit Tests

#### Repository Tests

```go
// internal/repository/website_test.go
package repository_test

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/suite"
)

type WebsiteRepositorySuite struct {
    suite.Suite
    repo *repository.WebsiteRepository
    db   *sqlx.DB
}

func (s *WebsiteRepositorySuite) SetupSuite() {
    // Setup test database
    var err error
    s.db, err = sqlx.Connect("postgres", "postgres://localhost:5432/vkai_test?sslmode=disable")
    s.Require().NoError(err)
    s.repo = repository.NewWebsiteRepository(s.db)
}

func (s *WebsiteRepositorySuite) TearDownSuite() {
    s.db.Close()
}

func (s *WebsiteRepositorySuite) TestCreate() {
    ctx := context.Background()
    website := &models.Website{
        TenantID: 1,
        Domain:   "test.example.com",
        ServerID: 1,
    }
    
    err := s.repo.Create(ctx, website)
    assert.NoError(s.T(), err)
    assert.NotZero(s.T(), website.ID)
}

func (s *WebsiteRepositorySuite) TestGetByID() {
    ctx := context.Background()
    
    // Create website first
    website := &models.Website{
        TenantID: 1,
        Domain:   "test2.example.com",
        ServerID: 1,
    }
    err := s.repo.Create(ctx, website)
    s.Require().NoError(err)
    
    // Get website
    result, err := s.repo.GetByID(ctx, website.ID)
    assert.NoError(s.T(), err)
    assert.Equal(s.T(), website.Domain, result.Domain)
}

func TestWebsiteRepository(t *testing.T) {
    suite.Run(t, new(WebsiteRepositorySuite))
}
```

#### Service Tests

```go
// internal/service/website_test.go
package service_test

import (
    "context"
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

func (m *MockWebsiteRepository) GetByID(ctx context.Context, id int64) (*models.Website, error) {
    args := m.Called(ctx, id)
    return args.Get(0).(*models.Website), args.Error(1)
}

func TestWebsiteService_Create(t *testing.T) {
    mockRepo := new(MockWebsiteRepository)
    service := service.NewWebsiteService(mockRepo)
    
    mockRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
    
    website, err := service.Create(context.Background(), 1, &service.CreateWebsiteRequest{
        Domain: "example.com",
    })
    
    assert.NoError(t, err)
    assert.NotNil(t, website)
    mockRepo.AssertExpectations(t)
}

func TestWebsiteService_Create_InvalidDomain(t *testing.T) {
    mockRepo := new(MockWebsiteRepository)
    service := service.NewWebsiteService(mockRepo)
    
    _, err := service.Create(context.Background(), 1, &service.CreateWebsiteRequest{
        Domain: "",
    })
    
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "domain is required")
}
```

#### Handler Tests

```go
// internal/handler/website_test.go
package handler_test

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
)

func TestWebsiteHandler_Create(t *testing.T) {
    gin.SetMode(gin.TestMode)
    
    mockService := new(MockWebsiteService)
    handler := handler.NewWebsiteHandler(mockService)
    
    router := gin.New()
    router.POST("/websites", handler.Create)
    
    body := map[string]string{
        "domain": "example.com",
    }
    jsonBody, _ := json.Marshal(body)
    
    req, _ := http.NewRequest("POST", "/websites", bytes.NewBuffer(jsonBody))
    req.Header.Set("Content-Type", "application/json")
    
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusCreated, w.Code)
}
```

### Frontend Unit Tests

#### Component Tests

```tsx
// src/components/WebsiteList.test.tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { WebsiteList } from './WebsiteList';
import { useWebsites } from '@/hooks/useWebsites';

// Mock the hook
jest.mock('@/hooks/useWebsites');

const mockWebsites = [
  { id: 1, domain: 'example.com', status: 'active' },
  { id: 2, domain: 'test.com', status: 'active' },
];

describe('WebsiteList', () => {
  beforeEach(() => {
    (useWebsites as jest.Mock).mockReturnValue({
      websites: mockWebsites,
      loading: false,
      error: null,
      fetchWebsites: jest.fn(),
    });
  });

  it('renders website list', () => {
    render(<WebsiteList />);
    
    expect(screen.getByText('example.com')).toBeInTheDocument();
    expect(screen.getByText('test.com')).toBeInTheDocument();
  });

  it('shows loading state', () => {
    (useWebsites as jest.Mock).mockReturnValue({
      websites: [],
      loading: true,
      error: null,
      fetchWebsites: jest.fn(),
    });

    render(<WebsiteList />);
    expect(screen.getByText('Loading...')).toBeInTheDocument();
  });

  it('shows error state', () => {
    (useWebsites as jest.Mock).mockReturnValue({
      websites: [],
      loading: false,
      error: 'Failed to fetch',
      fetchWebsites: jest.fn(),
    });

    render(<WebsiteList />);
    expect(screen.getByText('Error: Failed to fetch')).toBeInTheDocument();
  });

  it('handles delete click', async () => {
    const mockDelete = jest.fn();
    (useWebsites as jest.Mock).mockReturnValue({
      websites: mockWebsites,
      loading: false,
      error: null,
      fetchWebsites: jest.fn(),
      deleteWebsite: mockDelete,
    });

    render(<WebsiteList />);
    
    const deleteButton = screen.getAllByText('Delete')[0];
    fireEvent.click(deleteButton);
    
    await waitFor(() => {
      expect(mockDelete).toHaveBeenCalledWith(1);
    });
  });
});
```

#### Hook Tests

```tsx
// src/hooks/useWebsites.test.ts
import { renderHook, act } from '@testing-library/react';
import { useWebsites } from './useWebsites';
import { apiClient } from '@/lib/api';

jest.mock('@/lib/api');

describe('useWebsites', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('fetches websites', async () => {
    const mockWebsites = [
      { id: 1, domain: 'example.com' },
      { id: 2, domain: 'test.com' },
    ];

    (apiClient.get as jest.Mock).mockResolvedValue({ data: mockWebsites });

    const { result } = renderHook(() => useWebsites());

    await act(async () => {
      await result.current.fetchWebsites();
    });

    expect(result.current.websites).toEqual(mockWebsites);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('handles fetch error', async () => {
    (apiClient.get as jest.Mock).mockRejectedValue(new Error('Network error'));

    const { result } = renderHook(() => useWebsites());

    await act(async () => {
      await result.current.fetchWebsites();
    });

    expect(result.current.websites).toEqual([]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBe('Failed to fetch websites');
  });
});
```

---

## Integration Tests

### API Integration Tests

```go
// tests/integration/api_test.go
package integration_test

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/suite"
)

type APITestSuite struct {
    suite.Suite
    baseURL string
    token   string
}

func (s *APITestSuite) SetupSuite() {
    s.baseURL = "http://localhost:30110"
    
    // Login to get token
    body := map[string]string{
        "username": "admin",
        "password": "admin123",
    }
    jsonBody, _ := json.Marshal(body)
    
    resp, err := http.Post(s.baseURL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(jsonBody))
    s.Require().NoError(err)
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    s.token = result["access_token"].(string)
}

func (s *APITestSuite) TestCreateWebsite() {
    body := map[string]interface{}{
        "domain":    "integration-test.example.com",
        "server_id": 1,
        "web_server": "nginx",
    }
    jsonBody, _ := json.Marshal(body)
    
    req, _ := http.NewRequest("POST", s.baseURL+"/api/v1/websites", bytes.NewBuffer(jsonBody))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+s.token)
    
    client := &http.Client{}
    resp, err := client.Do(req)
    
    assert.NoError(s.T(), err)
    assert.Equal(s.T(), http.StatusCreated, resp.StatusCode)
}

func (s *APITestSuite) TestListWebsites() {
    req, _ := http.NewRequest("GET", s.baseURL+"/api/v1/websites", nil)
    req.Header.Set("Authorization", "Bearer "+s.token)
    
    client := &http.Client{}
    resp, err := client.Do(req)
    
    assert.NoError(s.T(), err)
    assert.Equal(s.T(), http.StatusOK, resp.StatusCode)
}

func TestAPI(t *testing.T) {
    suite.Run(t, new(APITestSuite))
}
```

### Database Integration Tests

```go
// tests/integration/database_test.go
package integration_test

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/suite"
)

type DatabaseTestSuite struct {
    suite.Suite
    db *sqlx.DB
}

func (s *DatabaseTestSuite) SetupSuite() {
    var err error
    s.db, err = sqlx.Connect("postgres", "postgres://localhost:5432/vkai_test?sslmode=disable")
    s.Require().NoError(err)
}

func (s *DatabaseTestSuite) TearDownSuite() {
    s.db.Close()
}

func (s *DatabaseTestSuite) TestWebsiteCRUD() {
    ctx := context.Background()
    repo := repository.NewWebsiteRepository(s.db)
    
    // Create
    website := &models.Website{
        TenantID: 1,
        Domain:   "db-test.example.com",
        ServerID: 1,
    }
    err := repo.Create(ctx, website)
    assert.NoError(s.T(), err)
    assert.NotZero(s.T(), website.ID)
    
    // Read
    result, err := repo.GetByID(ctx, website.ID)
    assert.NoError(s.T(), err)
    assert.Equal(s.T(), website.Domain, result.Domain)
    
    // Update
    website.Domain = "updated.example.com"
    err = repo.Update(ctx, website)
    assert.NoError(s.T(), err)
    
    // Delete
    err = repo.Delete(ctx, website.ID)
    assert.NoError(s.T(), err)
    
    // Verify deletion
    _, err = repo.GetByID(ctx, website.ID)
    assert.Error(s.T(), err)
}

func TestDatabase(t *testing.T) {
    suite.Run(t, new(DatabaseTestSuite))
}
```

---

## End-to-End Tests

### Playwright Tests

```typescript
// tests/e2e/website.spec.ts
import { test, expect } from '@playwright/test';

test.describe('Website Management', () => {
  test.beforeEach(async ({ page }) => {
    // Login
    await page.goto('/login');
    await page.fill('[data-testid="username"]', 'admin');
    await page.fill('[data-testid="password"]', 'admin123');
    await page.click('[data-testid="login-button"]');
    await page.waitForURL('/dashboard');
  });

  test('should create a new website', async ({ page }) => {
    await page.goto('/dashboard/websites');
    await page.click('[data-testid="add-website"]');
    
    await page.fill('[data-testid="domain"]', 'e2e-test.example.com');
    await page.selectOption('[data-testid="server"]', '1');
    await page.selectOption('[data-testid="web-server"]', 'nginx');
    
    await page.click('[data-testid="create-website"]');
    
    await expect(page.locator('[data-testid="success-message"]')).toBeVisible();
    await expect(page.locator('text=e2e-test.example.com')).toBeVisible();
  });

  test('should list websites', async ({ page }) => {
    await page.goto('/dashboard/websites');
    
    await expect(page.locator('[data-testid="website-list"]')).toBeVisible();
    await expect(page.locator('[data-testid="website-item"]')).toHaveCount(1);
  });

  test('should delete a website', async ({ page }) => {
    await page.goto('/dashboard/websites');
    
    await page.click('[data-testid="delete-website-1"]');
    await page.click('[data-testid="confirm-delete"]');
    
    await expect(page.locator('[data-testid="success-message"]')).toBeVisible();
  });
});
```

### Cypress Tests

```typescript
// cypress/e2e/website.cy.ts
describe('Website Management', () => {
  beforeEach(() => {
    cy.login('admin', 'admin123');
  });

  it('should create a new website', () => {
    cy.visit('/dashboard/websites');
    cy.get('[data-testid="add-website"]').click();
    
    cy.get('[data-testid="domain"]').type('cypress-test.example.com');
    cy.get('[data-testid="server"]').select('1');
    cy.get('[data-testid="web-server"]').select('nginx');
    
    cy.get('[data-testid="create-website"]').click();
    
    cy.get('[data-testid="success-message"]').should('be.visible');
    cy.contains('cypress-test.example.com').should('be.visible');
  });

  it('should list websites', () => {
    cy.visit('/dashboard/websites');
    
    cy.get('[data-testid="website-list"]').should('be.visible');
    cy.get('[data-testid="website-item"]').should('have.length.gte', 1);
  });
});
```

---

## Performance Tests

### Load Testing with k6

```javascript
// tests/performance/load-test.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '2m', target: 100 },  // Ramp up
    { duration: '5m', target: 100 },  // Stay at 100 users
    { duration: '2m', target: 200 },  // Ramp up to 200
    { duration: '5m', target: 200 },  // Stay at 200 users
    { duration: '2m', target: 0 },    // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],  // 95% of requests under 500ms
    http_req_failed: ['rate<0.01'],    // Less than 1% errors
  },
};

export default function () {
  const BASE_URL = 'http://localhost:30110';
  
  // Login
  const loginRes = http.post(`${BASE_URL}/api/v1/auth/login`, JSON.stringify({
    username: 'admin',
    password: 'admin123',
  }), {
    headers: { 'Content-Type': 'application/json' },
  });
  
  check(loginRes, {
    'login successful': (r) => r.status === 200,
  });
  
  const token = loginRes.json('access_token');
  
  // List websites
  const websitesRes = http.get(`${BASE_URL}/api/v1/websites`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  
  check(websitesRes, {
    'websites loaded': (r) => r.status === 200,
    'response time OK': (r) => r.timings.duration < 500,
  });
  
  sleep(1);
}
```

### Stress Testing

```javascript
// tests/performance/stress-test.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '1m', target: 500 },   // Ramp up to 500 users
    { duration: '3m', target: 500 },   // Stay at 500 users
    { duration: '1m', target: 1000 },  // Ramp up to 1000 users
    { duration: '3m', target: 1000 },  // Stay at 1000 users
    { duration: '1m', target: 0 },     // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(99)<1000'],  // 99% under 1s
    http_req_failed: ['rate<0.05'],     // Less than 5% errors
  },
};

export default function () {
  // ... test scenarios
}
```

---

## Security Tests

### OWASP ZAP

```bash
# Run OWASP ZAP scan
docker run -t owasp/zap2docker-stable zap-baseline.py \
  -t http://localhost:3000 \
  -r report.html
```

### SQL Injection Tests

```go
// tests/security/sql_injection_test.go
package security_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestSQLInjection(t *testing.T) {
    testCases := []struct {
        name     string
        input    string
        expected bool
    }{
        {
            name:     "Normal input",
            input:    "example.com",
            expected: true,
        },
        {
            name:     "SQL injection attempt",
            input:    "'; DROP TABLE websites; --",
            expected: false,
        },
        {
            name:     "SQL injection with UNION",
            input:    "' UNION SELECT * FROM users; --",
            expected: false,
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Test input validation
            result := validateDomain(tc.input)
            assert.Equal(t, tc.expected, result)
        })
    }
}
```

### XSS Tests

```typescript
// tests/security/xss.spec.ts
import { test, expect } from '@playwright/test';

test.describe('XSS Prevention', () => {
  test('should sanitize user input', async ({ page }) => {
    await page.goto('/dashboard/websites');
    
    // Try to inject script
    await page.fill('[data-testid="domain"]', '<script>alert("xss")</script>');
    await page.click('[data-testid="create-website"]');
    
    // Verify script is not executed
    const alertMessage = await page.evaluate(() => {
      return window.alert;
    });
    
    expect(alertMessage).toBeUndefined();
  });
});
```

---

## Test Environment

### Docker Compose for Testing

```yaml
# docker-compose.test.yml
version: '3.8'

services:
  postgres-test:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: vkai_test
      POSTGRES_USER: vkai_test
      POSTGRES_PASSWORD: test_password
    ports:
      - "5433:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U vkai_test -d vkai_test"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis-test:
    image: redis:7-alpine
    ports:
      - "6380:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5
```

### Test Configuration

```bash
# .env.test
DB_HOST=localhost
DB_PORT=5433
DB_NAME=vkai_test
DB_USER=vkai_test
DB_PASSWORD=test_password

REDIS_HOST=localhost
REDIS_PORT=6380

JWT_SECRET=test-secret-key
```

---

## Running Tests

### Backend Tests

```bash
cd backend

# Run all tests
go test ./...

# Run specific package
go test ./internal/service/...

# Run with coverage
go test -cover ./...

# Run with verbose output
go test -v ./...

# Run integration tests
go test -tags=integration ./tests/integration/...

# Run benchmarks
go test -bench=. ./...
```

### Frontend Tests

```bash
cd frontend

# Run all tests
npm test

# Run with coverage
npm run test:coverage

# Run in watch mode
npm run test:watch

# Run specific test file
npm test -- WebsiteList.test.tsx
```

### E2E Tests

```bash
# Playwright
npx playwright test

# Cypress
npx cypress run

# Specific test
npx playwright test tests/e2e/website.spec.ts
```

### Performance Tests

```bash
# k6 load test
k6 run tests/performance/load-test.js

# k6 stress test
k6 run tests/performance/stress-test.js
```

---

## Writing Tests

### Test Naming Convention

```go
// Good
func TestWebsiteService_Create_Success(t *testing.T) {}
func TestWebsiteService_Create_InvalidDomain(t *testing.T) {}
func TestWebsiteService_Create_DuplicateDomain(t *testing.T) {}

// Bad
func TestCreate(t *testing.T) {}
func TestWebsite(t *testing.T) {}
```

### Test Structure

```go
func TestSomething(t *testing.T) {
    // Arrange - Setup test data
    mockRepo := new(MockRepository)
    service := NewService(mockRepo)
    
    // Act - Execute the function
    result, err := service.DoSomething()
    
    // Assert - Verify results
    assert.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Table-Driven Tests

```go
func TestValidateDomain(t *testing.T) {
    tests := []struct {
        name    string
        domain  string
        wantErr bool
    }{
        {"valid domain", "example.com", false},
        {"empty domain", "", true},
        {"invalid domain", "invalid domain", true},
        {"subdomain", "sub.example.com", false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateDomain(tt.domain)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## Test Coverage

### Coverage Goals

- **Unit Tests**: 80%+ coverage
- **Integration Tests**: 60%+ coverage
- **E2E Tests**: Critical paths covered

### Coverage Reports

```bash
# Backend coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Frontend coverage
npm run test:coverage
open coverage/lcov-report/index.html
```

### Coverage Configuration

```json
// jest.config.js
module.exports = {
  collectCoverageFrom: [
    'src/**/*.{ts,tsx}',
    '!src/**/*.d.ts',
    '!src/**/index.ts',
  ],
  coverageThreshold: {
    global: {
      branches: 80,
      functions: 80,
      lines: 80,
      statements: 80,
    },
  },
};
```

---

## CI/CD Integration

### GitHub Actions

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  backend-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_DB: vkai_test
          POSTGRES_USER: vkai_test
          POSTGRES_PASSWORD: test_password
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432
      redis:
        image: redis:7
        options: >-
          --health-cmd redis-cli ping
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 6379:6379
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'
      - run: go test ./...
      - run: go test -cover ./...

  frontend-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: '20'
      - run: npm ci
      - run: npm test
      - run: npm run test:coverage

  e2e-tests:
    runs-on: ubuntu-latest
    needs: [backend-tests, frontend-tests]
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: '20'
      - run: npm ci
      - run: npx playwright install
      - run: npx playwright test
```

---

## Best Practices

### Test Isolation

- Each test should be independent
- Use fresh test data for each test
- Clean up after tests

### Test Speed

- Unit tests should be fast (< 100ms)
- Integration tests should be reasonable (< 1s)
- E2E tests can be slower (< 30s)

### Test Reliability

- Avoid flaky tests
- Use deterministic test data
- Mock external dependencies

### Test Maintenance

- Keep tests up to date
- Refactor tests when code changes
- Remove obsolete tests

---

## Resources

- [Go Testing](https://go.dev/doc/tutorial/add-a-test)
- [Jest Documentation](https://jestjs.io/)
- [Playwright Documentation](https://playwright.dev/)
- [Cypress Documentation](https://www.cypress.io/)
- [k6 Documentation](https://k6.io/docs/)

---

## Support

- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Discussions**: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
