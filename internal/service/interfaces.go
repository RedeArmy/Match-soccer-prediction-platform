// Package service contains the application's business logic.
//
// Each service orchestrates one domain concern: it reads from and writes to
// repositories, enforces business rules, and emits domain events. Services
// must not be aware of HTTP or database implementation details - they operate
// exclusively on domain entities and depend on repository interfaces defined
// in internal/repository, not on concrete PostgreSQL implementations.
//
// Interface and implementation co-location: each service interface is defined
// in the same file as its concrete implementation. This means opening
// match_service.go shows both the MatchService contract and its implementation,
// making the navigation path consistent and predictable.
//
// Service interfaces are the contracts consumed by the handler layer. Concrete
// implementations are wired at the composition root (cmd/api/main.go).
// This separation allows handlers to be tested with lightweight mock services
// without touching a real database.
package service
