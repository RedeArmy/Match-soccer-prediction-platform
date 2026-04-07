---
name: User working preferences
description: How the user likes to collaborate and what to avoid
type: user
---

The user is building a FIFA World Cup 2026 quiniela (prediction pool) platform in Go.
They speak Spanish natively and sometimes write requests in Spanish.

They take a clean architecture approach seriously: domain → service → repository → handler layers with strict separation.

**Why:** Strong SDE III MAANG sensibilities; quality gate checks (SonarCloud ≥80% coverage, golangci-lint v2, gofmt).

**How to apply:** Always maintain layer boundaries. No business logic in handlers. No infrastructure imports in domain. Write tests for new code paths.
