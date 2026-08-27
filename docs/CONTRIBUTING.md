# Contributing to VKAI Panel

Thank you for your interest in contributing to VKAI Panel! This document provides guidelines and instructions for contributing.

## Table of Contents

1. [Code of Conduct](#code-of-conduct)
2. [Getting Started](#getting-started)
3. [Development Workflow](#development-workflow)
4. [Coding Standards](#coding-standards)
5. [Commit Guidelines](#commit-guidelines)
6. [Pull Request Process](#pull-request-process)
7. [Testing](#testing)
8. [Documentation](#documentation)
9. [Issue Reporting](#issue-reporting)
10. [Community](#community)

---

## Code of Conduct

### Our Pledge

We are committed to making participation in this project a harassment-free experience for everyone, regardless of age, body size, disability, ethnicity, gender identity and expression, level of experience, nationality, personal appearance, race, religion, or sexual identity and orientation.

### Our Standards

**Positive behavior includes:**
- Using welcoming and inclusive language
- Being respectful of differing viewpoints and experiences
- Gracefully accepting constructive criticism
- Focusing on what is best for the community
- Showing empathy towards other community members

**Unacceptable behavior includes:**
- Trolling, insulting/derogatory comments, and personal attacks
- Public or private harassment
- Publishing others' private information without permission
- Other conduct which could reasonably be considered inappropriate

---

## Getting Started

### Prerequisites

- Go 1.22+
- Node.js 20 LTS
- PostgreSQL 16+
- Redis 7+
- Git

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork locally:

```bash
git clone https://github.com/YOUR_USERNAME/vkai-panel.git
cd vkai-panel
```

3. Add upstream remote:

```bash
git remote add upstream https://github.com/hitechcloud-vietnam/vkai-panel.git
```

### Setup Development Environment

```bash
# Run development setup script
chmod +x setup-dev.sh
./setup-dev.sh

# Or manually:
# Start databases with Docker
docker-compose -f docker-compose.dev.yml up -d

# Setup the API (core/)
cd core
go mod tidy
cd ..
make migrate DATABASE_URL=postgres://vkai:PASSWORD@localhost:5432/vkai_panel

# Setup the UI (panel/)
cd panel
npm install
```

---

## Development Workflow

### Branch Strategy

**Pushing directly to `main` is forbidden.** `main` is protected by a branch
ruleset, and the `Guard main` workflow fails any commit on `main` that did not
arrive through a Pull Request. Every change starts on a side branch and lands
via PR with green CI and at least one approving review.

```
main                      protected, PR only
  ├── feat/your-feature
  ├── fix/your-bugfix
  ├── docs/your-doc-change
  ├── refactor/your-cleanup
  └── chore/your-maintenance
```

`develop` may be used as an integration branch for larger efforts; CI runs on
both `main` and `develop`. It is not a way around the PR requirement.

| Prefix | Use for |
|--------|---------|
| `feat/` | New functionality |
| `fix/` | Bug fixes |
| `docs/` | Documentation only |
| `refactor/` | Restructuring with no behaviour change |
| `chore/` | Build, CI, dependencies, tooling |

### Creating a Branch

```bash
# Update your checkout
git fetch upstream
git checkout main
git merge --ff-only upstream/main

# Create a side branch
git checkout -b feat/your-feature-name
```

### Making Changes

1. Make your changes
2. Write tests for new functionality
3. Update documentation if needed
4. Run tests locally
5. Commit your changes

### Syncing with Upstream

```bash
git fetch upstream
git rebase upstream/main
```

---

## Coding Standards

### Go Code Style

#### Formatting

```bash
# Format code
gofmt -w .

# Run linter
golangci-lint run
```

#### Naming Conventions

```go
// Package names: lowercase, single word
package website

// Exported names: CamelCase
type WebsiteService struct {}

// Unexported names: camelCase
func (s *WebsiteService) getWebsite() {}

// Interfaces: -er suffix when possible
type Reader interface {
    Read(p []byte) (n int, err error)
}

// Constants: CamelCase
const MaxRetries = 3

// Variables: camelCase
var defaultTimeout = 30 * time.Second
```

#### Error Handling

```go
// Always handle errors
result, err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}

// Use custom error types
type NotFoundError struct {
    Resource string
    ID       int64
}

func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s with ID %d not found", e.Resource, e.ID)
}
```

#### Comments

```go
// Package website provides website management functionality.
package website

// WebsiteService handles website operations.
type WebsiteService struct {
    repo WebsiteRepository
}

// Create creates a new website for the given tenant.
// It validates the domain and checks for duplicates.
func (s *WebsiteService) Create(ctx context.Context, tenantID int64, req *CreateWebsiteRequest) (*Website, error) {
    // Implementation
}
```

### TypeScript/React Code Style

#### Formatting

```bash
# Format code
npm run format

# Run linter
npm run lint
```

#### Naming Conventions

```typescript
// Components: PascalCase
function WebsiteList() {}

// Functions/Variables: camelCase
function getWebsites() {}
const isLoading = false;

// Constants: UPPER_SNAKE_CASE
const API_BASE_URL = '/api/v1';

// Interfaces/Types: PascalCase with 'I' prefix for interfaces (optional)
interface Website {
  id: number;
  domain: string;
}

// Files: kebab-case
// website-list.tsx
// use-websites.ts
```

#### Component Structure

```typescript
// Imports
import React from 'react';
import { useWebsites } from '@/hooks/use-websites';

// Types
interface WebsiteListProps {
  serverId?: number;
}

// Component
export function WebsiteList({ serverId }: WebsiteListProps) {
  // Hooks
  const { websites, loading, error } = useWebsites();
  
  // Handlers
  const handleDelete = (id: number) => {
    // Implementation
  };
  
  // Render
  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage error={error} />;
  
  return (
    <div>
      {websites.map(website => (
        <WebsiteItem key={website.id} website={website} onDelete={handleDelete} />
      ))}
    </div>
  );
}
```

### SQL Style

```sql
-- Keywords: UPPERCASE
SELECT id, domain, status
FROM websites
WHERE tenant_id = $1
  AND status = 'active'
ORDER BY created_at DESC
LIMIT 10 OFFSET 0;

-- Table names: snake_case
-- Column names: snake_case
-- Index names: idx_table_column
-- Foreign keys: fk_table_referenced_table
```

---

## Commit Guidelines

### Commit Message Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

- **feat**: New feature
- **fix**: Bug fix
- **docs**: Documentation changes
- **style**: Code style changes (formatting, etc.)
- **refactor**: Code refactoring
- **perf**: Performance improvements
- **test**: Adding or updating tests
- **chore**: Maintenance tasks

### Examples

```bash
# Feature
git commit -m "feat(website): add domain validation"

# Bug fix
git commit -m "fix(auth): resolve token refresh issue"

# Documentation
git commit -m "docs(api): update endpoint documentation"

# With body
git commit -m "feat(ssl): add Let's Encrypt integration

- Implement ACME protocol
- Add certificate renewal
- Add DNS challenge support

Closes #123"
```

### Rules

1. Use imperative mood ("add feature" not "added feature")
2. First line should be 50 characters or less
3. Reference issues and pull requests
4. Do not use emoji, in commit messages or anywhere else: not in code, not in
   documentation, not in UI strings.

---

## Pull Request Process

### Before Submitting

1. **Update your branch**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run tests**:
   ```bash
   make test          # core, panel and agent
   # or individually
   cd core  && go test ./...
   cd panel && npm test
   ```

3. **Run linters**:
   ```bash
   make lint
   # or individually
   cd core  && golangci-lint run
   cd panel && npm run lint
   ```

4. **Update documentation** if needed

### PR Template

GitHub fills the description from
[`.github/PULL_REQUEST_TEMPLATE.md`](../.github/PULL_REQUEST_TEMPLATE.md).
Fill in every section rather than deleting it. The required checklist covers:

- green CI on all three jobs (`Core API`, `Panel UI`, `Agent`);
- confirmation that the change came through a side branch, not a direct push to
  `main`;
- what you actually tested, and how;
- before/after screenshots when the UI changed;
- the security impact (authentication, RBAC, panel port, entrance, IP allow
  list, command execution, new dependencies);
- the migration impact (files under `core/migrations/`, schema changes,
  rollback, runtime on large data).

### Review Process

1. **Automated Checks**: the CI pipeline runs on the PR.
2. **Code Review**: at least one maintainer reviews. `.github/CODEOWNERS`
   routes security-sensitive paths to the repository owner automatically.
3. **Approval**: a maintainer approves the PR.
4. **Merge**: squash merge into `main`. Never push to `main` directly.

### After Merge

1. Delete your side branch.
2. Update your local repository:
   ```bash
   git checkout main
   git pull upstream main
   ```

---

## Testing

### Writing Tests

#### Go Tests

```go
// internal/service/website_test.go
package service_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestWebsiteService_Create(t *testing.T) {
    // Arrange
    mockRepo := new(MockWebsiteRepository)
    service := NewWebsiteService(mockRepo)
    
    // Act
    website, err := service.Create(context.Background(), 1, &CreateWebsiteRequest{
        Domain: "test.example.com",
    })
    
    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, website)
}
```

#### React Tests

```typescript
// src/components/WebsiteList.test.tsx
import { render, screen } from '@testing-library/react';
import { WebsiteList } from './WebsiteList';

describe('WebsiteList', () => {
  it('renders website list', () => {
    render(<WebsiteList />);
    expect(screen.getByText('Websites')).toBeInTheDocument();
  });
});
```

### Running Tests

```bash
# API tests (core/)
cd core
go test ./...
go test -cover ./...

# UI tests (panel/)
cd panel
npm test
npm run test:coverage

# E2E tests
npx playwright test
```

---

## Documentation

### Code Documentation

#### Go

```go
// Package website provides website management functionality.
//
// It handles CRUD operations for websites, domain management,
// and web server configuration.
package website

// WebsiteService handles website operations.
type WebsiteService struct {
    repo   WebsiteRepository
    logger *zap.Logger
}

// Create creates a new website for the given tenant.
//
// It validates the domain, checks for duplicates, and creates
// the necessary web server configuration.
//
// Parameters:
//   - ctx: Context with tenant information
//   - tenantID: The tenant ID
//   - req: Website creation request
//
// Returns:
//   - *Website: The created website
//   - error: Error if creation fails
func (s *WebsiteService) Create(ctx context.Context, tenantID int64, req *CreateWebsiteRequest) (*Website, error) {
    // Implementation
}
```

#### TypeScript

```typescript
/**
 * Website list component.
 *
 * Displays a list of websites with actions for managing them.
 *
 * @example
 * ```tsx
 * <WebsiteList serverId={1} onDelete={handleDelete} />
 * ```
 */
export function WebsiteList({ serverId, onDelete }: WebsiteListProps) {
  // Implementation
}
```

### API Documentation

```go
// @Summary Create a new website
// @Description Create a new website for the authenticated user's tenant
// @Tags websites
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateWebsiteRequest true "Website creation request"
// @Success 201 {object} Website
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/websites [post]
func (h *WebsiteHandler) Create(c *gin.Context) {
    // Implementation
}
```

---

## Issue Reporting

### Bug Reports

```markdown
## Bug Description

Clear description of the bug.

## Steps to Reproduce

1. Go to '...'
2. Click on '...'
3. Scroll down to '...'
4. See error

## Expected Behavior

What you expected to happen.

## Actual Behavior

What actually happened.

## Environment

- OS: [e.g., Ubuntu 22.04]
- Browser: [e.g., Chrome 120]
- Version: [e.g., 1.0.0]

## Screenshots

If applicable, add screenshots.

## Additional Context

Any other context about the problem.
```

### Feature Requests

```markdown
## Feature Description

Clear description of the feature.

## Problem Statement

What problem does this feature solve?

## Proposed Solution

How should this feature work?

## Alternatives Considered

Other solutions you've considered.

## Additional Context

Any other context or screenshots.
```

---

## Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and discussions
- **Email**: support@hitechcloud.vn

### Getting Help

1. Check existing documentation
2. Search existing issues
3. Ask in GitHub Discussions
4. Create a new issue if needed

### Recognition

Contributors will be recognized in:
- README.md contributors section
- Release notes
- Project documentation

---

## License

By contributing, you agree that your contributions will be licensed under the project's MIT License.

---

## Questions?

If you have questions about contributing, please:
1. Check the documentation
2. Search existing issues
3. Ask in GitHub Discussions
4. Contact the maintainers

Thank you for contributing to VKAI Panel.
