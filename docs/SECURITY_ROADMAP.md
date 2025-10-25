# Security & Architecture Roadmap

**Document Version:** 1.0
**Last Updated:** 2025-10-25
**Prepared By:** Solution Architecture & Security Review

## Executive Summary

This roadmap provides a comprehensive plan for securing and hardening the Event Sourcing Framework for production deployment. Based on a thorough security review, items are categorized by priority and organized into phased releases.

**Project Status:** Alpha - Not production-ready
**Current Test Coverage:** ~18% (12 test files / 67 production files)
**Security Posture:** Foundational (basic security primitives exist, significant gaps remain)

---

## 🚨 Phase 0: Critical Security Issues (IMMEDIATE)

**Timeline:** 1-2 weeks
**Priority:** P0 - Must fix before any production use

### SEC-001: Authentication & Credentials Management

**Severity:** CRITICAL
**Current State:** No encrypted credential storage, plaintext credentials in NATS transport
**Risk:** Credential exposure, unauthorized access

**Issues:**
- NATS transport accepts plaintext `User` and `Pass` fields (pkg/cqrs/nats/transport.go:35-36)
- No credential encryption at rest
- No secrets management integration
- Token authentication stored as plain strings
- No rotation mechanism for credentials

**Required Actions:**
1. ✅ **Remove plaintext password fields from public APIs**
   - Replace with credential provider pattern
   - Support external secret managers (HashiCorp Vault, AWS Secrets Manager, etc.)

2. ✅ **Implement secure credential storage**
   ```go
   type CredentialProvider interface {
       GetCredentials(ctx context.Context) (*Credentials, error)
       Rotate(ctx context.Context) error
   }
   ```

3. ✅ **Add credential encryption for local storage**
   - Use OS keychain on macOS/Windows
   - Use secret-tool on Linux
   - Fall back to encrypted file with key derivation (PBKDF2/Argon2)

**Files to Update:**
- `pkg/cqrs/nats/transport.go`
- `pkg/infrastructure/nats/embedded.go`
- New: `pkg/security/credentials.go`

---

### SEC-002: TLS/Encryption for NATS

**Severity:** CRITICAL
**Current State:** No TLS configuration for NATS connections
**Risk:** Man-in-the-middle attacks, data interception

**Issues:**
- NATS connections default to unencrypted
- No certificate validation
- No mTLS support for client authentication
- Embedded NATS has no TLS configuration options

**Required Actions:**
1. ✅ **Add TLS configuration to NATS transport**
   ```go
   type TLSConfig struct {
       Enabled            bool
       CertFile          string
       KeyFile           string
       CAFile            string
       InsecureSkipVerify bool  // Only for development
       ClientAuth         bool  // mTLS
   }
   ```

2. ✅ **Add TLS options to embedded NATS**
   - Certificate management
   - Auto-renewal hooks
   - Let's Encrypt integration for development

3. ✅ **Enforce TLS by default in production**
   - Fail fast if TLS not configured in production mode
   - Warning logs for development mode

**Files to Create/Update:**
- `pkg/infrastructure/nats/tls.go`
- `pkg/infrastructure/nats/embedded.go`
- `pkg/cqrs/nats/transport.go`

---

### SEC-003: SQL Injection Prevention

**Severity:** HIGH
**Current State:** Using sqlc with parameterized queries ✅, but manual SQL in migrations
**Risk:** SQL injection in custom queries or migrations

**Issues:**
- Migration files use string concatenation (review needed)
- Custom query extension points may bypass sqlc
- No input validation on SQL identifiers (table names, etc.)

**Required Actions:**
1. ✅ **Audit all migration files for SQL injection risks**
   - Review: `pkg/store/sqlite/migrations/*.sql`
   - Ensure no dynamic SQL construction

2. ✅ **Add SQL identifier validation**
   ```go
   func ValidateSQLIdentifier(name string) error {
       if !sqlIdentifierRegex.MatchString(name) {
           return ErrInvalidIdentifier
       }
       return nil
   }
   ```

3. ✅ **Document safe query patterns**
   - Add examples to CONTRIBUTING.md
   - Create security guidelines doc

**Files to Audit:**
- `pkg/store/sqlite/migrations/*.sql`
- `pkg/store/sqlite/eventstore.go`
- New: `docs/guides/security-guidelines.md`

---

### SEC-004: Error Information Disclosure

**Severity:** MEDIUM-HIGH
**Current State:** Error messages may leak sensitive information
**Risk:** Information disclosure to attackers

**Issues:**
- Database errors returned directly to clients
- Stack traces in error responses (potential)
- SQL queries visible in error messages
- Aggregate IDs exposed in constraint violations

**Required Actions:**
1. ✅ **Implement error sanitization layer**
   ```go
   func SanitizeError(err error, mode string) error {
       if mode == "production" {
           // Return generic error
           return errors.New("internal server error")
       }
       return err // Development mode
   }
   ```

2. ✅ **Add security error codes**
   - Replace sensitive errors with codes
   - Log full details server-side only
   - Return safe messages to clients

3. ✅ **Audit all error paths**
   - pkg/eventsourcing/errors.go ✅ (basic structure exists)
   - pkg/store/sqlite/eventstore.go
   - pkg/cqrs/nats/transport.go

**Files to Update:**
- `pkg/eventsourcing/errors.go`
- `pkg/middleware/recovery.go`
- New: `pkg/security/errors.go`

---

### SEC-005: Input Validation Gaps

**Severity:** MEDIUM-HIGH
**Current State:** Basic validation middleware exists but incomplete
**Risk:** Invalid input causing crashes, injection attacks

**Issues:**
- Principal ID validation commented out (pkg/middleware/validation.go:46-48)
- No comprehensive input sanitization
- Missing length limits on strings
- No regex validation for IDs/emails
- UUID format not enforced

**Required Actions:**
1. ✅ **Enforce principal ID validation**
   - Remove comment, make it mandatory
   - Add configurable option for opt-out in development

2. ✅ **Add comprehensive input validators**
   ```go
   type InputValidators struct {
       AggregateID func(string) error  // UUID v4
       CommandID   func(string) error  // UUID v4
       Email       func(string) error  // RFC 5322
       TenantID    func(string) error  // Custom format
   }
   ```

3. ✅ **Implement length limits**
   - String fields: max 1000 chars (configurable)
   - Array fields: max 100 items
   - Binary data: max 10MB
   - Document limits in protobuf schemas

**Files to Update:**
- `pkg/middleware/validation.go`
- New: `pkg/validation/validators.go`
- Proto files: Add validation annotations

---

## 🔒 Phase 1: Security Hardening (SHORT-TERM)

**Timeline:** 1-2 months
**Priority:** P1 - Essential for production

### SEC-101: Rate Limiting & DoS Protection

**Severity:** HIGH
**Current State:** No rate limiting implemented
**Risk:** Denial of service, resource exhaustion

**Required Actions:**
1. **Add rate limiting middleware**
   - Per-tenant limits
   - Per-principal limits
   - Per-command-type limits
   - Global limits

2. **Implement circuit breakers**
   - Event store operations
   - NATS connections
   - External service calls

3. **Add request throttling**
   - Token bucket algorithm
   - Sliding window counters

**New Files:**
- `pkg/middleware/ratelimit.go`
- `pkg/middleware/circuitbreaker.go`

---

### SEC-102: Audit Logging

**Severity:** HIGH
**Current State:** OpenTelemetry tracing exists ✅, but no security audit trail
**Risk:** No forensic capability, compliance violations

**Required Actions:**
1. **Implement security audit log**
   - Authentication events
   - Authorization failures
   - Data access (read/write)
   - Configuration changes
   - Admin actions

2. **Add tamper-proof logging**
   - Cryptographic signatures on log entries
   - Append-only log storage
   - Log rotation with archival

3. **Create audit query API**
   - Search by principal, tenant, time
   - Export for compliance
   - Real-time alerts

**New Files:**
- `pkg/audit/logger.go`
- `pkg/audit/storage.go`
- `pkg/audit/query.go`

---

### SEC-103: Data Encryption at Rest

**Severity:** HIGH
**Current State:** SQLite data stored unencrypted
**Risk:** Data breach if storage compromised

**Required Actions:**
1. **Implement SQLite encryption**
   - SQLCipher integration
   - Key derivation (PBKDF2/Argon2)
   - Key rotation support

2. **Encrypt sensitive event data**
   - Field-level encryption
   - Searchable encryption for queries
   - Key per tenant option

3. **Add snapshot encryption**
   - Encrypt aggregate snapshots
   - Separate encryption keys

**New Files:**
- `pkg/store/sqlite/encryption.go`
- `pkg/security/crypto.go`
- `pkg/security/keymanagement.go`

---

### SEC-104: Authorization Framework

**Severity:** MEDIUM-HIGH
**Current State:** Basic RBAC exists ✅, needs enhancement
**Risk:** Privilege escalation, unauthorized access

**Required Actions:**
1. **Enhance RBAC implementation**
   - Hierarchical roles
   - Role inheritance
   - Deny rules (not just allow)

2. **Add ABAC (Attribute-Based Access Control)**
   - Resource-based policies
   - Context-aware decisions
   - Policy DSL

3. **Implement permission caching**
   - Redis-based cache
   - TTL-based invalidation
   - Distributed cache support

**Files to Update:**
- `pkg/middleware/authorization.go`
- New: `pkg/authz/abac.go`
- New: `pkg/authz/policy.go`

---

### SEC-105: Secure Multi-Tenancy

**Severity:** MEDIUM-HIGH
**Current State:** Tenant isolation middleware exists ✅, needs hardening
**Risk:** Cross-tenant data leakage

**Required Actions:**
1. **Harden tenant validation**
   - Cryptographic tenant ID verification
   - Tenant ID in JWT claims
   - Fail-closed on validation errors

2. **Add tenant data isolation**
   - Database-level row security
   - Tenant-specific encryption keys
   - Network isolation per tenant

3. **Implement tenant quotas**
   - Storage limits
   - Event rate limits
   - Concurrent connection limits

**Files to Update:**
- `pkg/multitenancy/middleware.go`
- New: `pkg/multitenancy/isolation.go`
- New: `pkg/multitenancy/quotas.go`

---

### SEC-106: Security Testing

**Severity:** HIGH
**Current State:** Limited test coverage (18%), no security tests
**Risk:** Undetected vulnerabilities

**Required Actions:**
1. **Add security test suite**
   - SQL injection tests
   - XSS tests (if web UI added)
   - CSRF tests
   - Authentication bypass tests
   - Authorization bypass tests

2. **Implement fuzzing**
   - Protobuf input fuzzing
   - JSON input fuzzing
   - SQL query fuzzing

3. **Add penetration testing**
   - Automated scans (OWASP ZAP)
   - Manual penetration testing
   - Third-party security audit

**New Files:**
- `pkg/security/tests/injection_test.go`
- `pkg/security/tests/auth_test.go`
- `pkg/security/tests/fuzz_test.go`

---

## 🏗️ Phase 2: Production Readiness (MEDIUM-TERM)

**Timeline:** 3-4 months
**Priority:** P2 - Production hardening

### PROD-201: Observability Enhancements

**Required Actions:**
1. **Security metrics**
   - Authentication failures
   - Authorization denials
   - Rate limit triggers
   - Suspicious activity scores

2. **Advanced tracing**
   - Distributed tracing across services
   - Span annotations for security events
   - Trace sampling based on security context

3. **Alerting framework**
   - Real-time security alerts
   - Anomaly detection
   - Integration with PagerDuty/OpsGenie

**New Files:**
- `pkg/observability/security_metrics.go`
- `pkg/observability/alerts.go`

---

### PROD-202: High Availability & Resilience

**Required Actions:**
1. **Event store replication**
   - SQLite replication to PostgreSQL
   - Multi-region support
   - Automatic failover

2. **NATS clustering**
   - JetStream clustering
   - Stream replication
   - Geo-distribution

3. **Backup & recovery**
   - Automated backups
   - Point-in-time recovery
   - Disaster recovery procedures

**New Files:**
- `pkg/store/replication.go`
- `pkg/infrastructure/nats/cluster.go`
- `docs/guides/disaster-recovery.md`

---

### PROD-203: Performance & Scalability

**Required Actions:**
1. **Query optimization**
   - Index analysis
   - Query plan optimization
   - Caching strategies

2. **Connection pooling**
   - Dynamic pool sizing
   - Connection health checks
   - Graceful connection recycling

3. **Horizontal scaling**
   - Stateless service design
   - Load balancing
   - Partition strategies

**New Files:**
- `pkg/performance/cache.go`
- `pkg/performance/pooling.go`

---

### PROD-204: Compliance & Governance

**Required Actions:**
1. **GDPR compliance**
   - Right to be forgotten
   - Data portability
   - Consent management
   - Data minimization

2. **SOC 2 compliance**
   - Access controls
   - Audit trails
   - Encryption
   - Incident response

3. **Data retention policies**
   - Configurable retention
   - Automatic archival
   - Secure deletion

**New Files:**
- `pkg/compliance/gdpr.go`
- `pkg/compliance/retention.go`
- `docs/compliance/SOC2.md`

---

### PROD-205: CI/CD & DevSecOps

**Required Actions:**
1. **Automated security scanning**
   - Dependency vulnerability scanning (Dependabot, Snyk)
   - SAST (Static Application Security Testing)
   - DAST (Dynamic Application Security Testing)
   - Container image scanning

2. **Secure build pipeline**
   - Signed commits
   - Signed releases
   - SBOM (Software Bill of Materials)
   - Supply chain security

3. **Deployment security**
   - Infrastructure as Code (Terraform)
   - Secret management (Vault)
   - Immutable infrastructure
   - Blue-green deployments

**New Files:**
- `.github/workflows/security-scan.yml`
- `.github/workflows/build.yml`
- `.github/workflows/deploy.yml`
- `terraform/`

---

### PROD-206: API Security

**Required Actions:**
1. **API Gateway integration**
   - Request validation
   - Response sanitization
   - CORS configuration
   - API key management

2. **GraphQL security (if added)**
   - Query depth limiting
   - Query cost analysis
   - Introspection control

3. **Webhook security**
   - HMAC signature verification
   - Replay attack prevention
   - IP whitelisting

**New Files:**
- `pkg/api/gateway.go`
- `pkg/api/webhook.go`

---

## 🚀 Phase 3: Advanced Features (LONG-TERM)

**Timeline:** 6+ months
**Priority:** P3 - Enhancements

### ADV-301: Advanced Encryption

**Required Actions:**
1. **Homomorphic encryption**
   - Query on encrypted data
   - Privacy-preserving analytics

2. **Zero-knowledge proofs**
   - Prove data validity without revealing data
   - Privacy-preserving audits

3. **Quantum-resistant cryptography**
   - Post-quantum algorithms
   - Hybrid classical/quantum schemes

---

### ADV-302: AI-Powered Security

**Required Actions:**
1. **Anomaly detection**
   - ML-based threat detection
   - Behavioral analysis
   - Automated incident response

2. **Predictive security**
   - Vulnerability prediction
   - Attack surface analysis
   - Risk scoring

---

### ADV-303: Event Sourcing Security Features

**Required Actions:**
1. **Event signing**
   - Digital signatures on events
   - Non-repudiation
   - Event chain verification

2. **Temporal queries with privacy**
   - Time-travel queries
   - Privacy-preserving point-in-time recovery
   - Audit-safe rollback

3. **Blockchain integration**
   - Event hash anchoring
   - Immutable audit trail
   - Distributed consensus

---

## 📊 Code Quality & Architecture Improvements

### ARCH-401: Test Coverage (P1)

**Current:** 18% (12/67 files)
**Target:** 80%+ coverage

**Required Actions:**
1. **Unit tests**
   - All public APIs
   - All middleware
   - All validators

2. **Integration tests**
   - End-to-end workflows
   - Multi-tenant scenarios
   - Error paths

3. **Performance tests**
   - Load testing
   - Stress testing
   - Benchmark tests

---

### ARCH-402: Error Handling Standardization (P1)

**Required Actions:**
1. **Centralized error types**
   - Domain errors
   - Infrastructure errors
   - Security errors

2. **Error wrapping strategy**
   - Context preservation
   - Stack trace management
   - Error codes

3. **Error recovery**
   - Retry policies
   - Circuit breakers
   - Graceful degradation

---

### ARCH-403: Documentation (P2)

**Required Actions:**
1. **Security documentation**
   - Security architecture
   - Threat model
   - Security best practices
   - Incident response plan

2. **API documentation**
   - OpenAPI/Swagger specs
   - API versioning
   - Deprecation policy

3. **Runbooks**
   - Deployment procedures
   - Monitoring guides
   - Troubleshooting guides
   - Incident response

**New Files:**
- `docs/security/threat-model.md`
- `docs/security/incident-response.md`
- `docs/api/openapi.yaml`
- `docs/runbooks/deployment.md`

---

### ARCH-404: Code Organization (P2)

**Required Actions:**
1. **Package refactoring**
   - Separate security package
   - Extract validation logic
   - Domain-driven design alignment

2. **Interface improvements**
   - Reduce coupling
   - Improve testability
   - Better abstractions

3. **Code generation improvements**
   - Security annotations in protobuf
   - Auto-generated validators
   - Type-safe security policies

---

## 🎯 Priority Matrix

### Immediate (Weeks 1-2)
- [ ] SEC-001: Credentials Management
- [ ] SEC-002: TLS/Encryption
- [ ] SEC-003: SQL Injection Prevention
- [ ] SEC-004: Error Information Disclosure
- [ ] SEC-005: Input Validation Gaps

### Short-term (Months 1-2)
- [ ] SEC-101: Rate Limiting
- [ ] SEC-102: Audit Logging
- [ ] SEC-103: Data Encryption at Rest
- [ ] SEC-104: Authorization Framework
- [ ] SEC-105: Secure Multi-Tenancy
- [ ] SEC-106: Security Testing
- [ ] ARCH-401: Test Coverage
- [ ] ARCH-402: Error Handling

### Medium-term (Months 3-4)
- [ ] PROD-201: Observability Enhancements
- [ ] PROD-202: High Availability
- [ ] PROD-203: Performance
- [ ] PROD-204: Compliance
- [ ] PROD-205: CI/CD & DevSecOps
- [ ] PROD-206: API Security
- [ ] ARCH-403: Documentation
- [ ] ARCH-404: Code Organization

### Long-term (Months 6+)
- [ ] ADV-301: Advanced Encryption
- [ ] ADV-302: AI-Powered Security
- [ ] ADV-303: Event Sourcing Security Features

---

## 📈 Success Metrics

### Security Metrics
- **Zero** critical vulnerabilities in production
- **< 1 second** mean time to detect security events
- **< 5 minutes** mean time to respond to incidents
- **100%** of security patches applied within 24 hours
- **Zero** unauthorized data access events

### Quality Metrics
- **80%+** test coverage
- **Zero** P0/P1 bugs in production
- **< 100ms** p99 latency for commands
- **99.9%** uptime SLA
- **Zero** data loss events

### Compliance Metrics
- **100%** audit log coverage for sensitive operations
- **100%** encryption for data at rest and in transit
- **< 30 days** compliance with GDPR data subject requests
- **Zero** compliance violations

---

## 🔍 Security Review Summary

### Strengths
✅ **Good foundation:** Clean architecture, basic security primitives
✅ **Middleware pattern:** Easy to add security layers
✅ **Observability:** OpenTelemetry integration
✅ **Multi-tenancy:** Tenant isolation primitives exist
✅ **Validation:** Basic validation middleware
✅ **RBAC:** Role-based authorization framework

### Critical Gaps
❌ **No credential encryption:** Plaintext passwords
❌ **No TLS:** Unencrypted NATS connections
❌ **No audit logging:** Limited forensic capability
❌ **No encryption at rest:** SQLite data unencrypted
❌ **No rate limiting:** Vulnerable to DoS
❌ **Low test coverage:** 18% (target: 80%+)
❌ **No CI/CD:** Manual processes

---

## 📚 References

### Security Standards
- OWASP Top 10 (2021)
- CWE Top 25
- NIST Cybersecurity Framework
- ISO 27001
- SOC 2 Type II

### Best Practices
- Go Security Best Practices
- Event Sourcing Security Patterns
- CQRS Security Considerations
- NATS Security Guide

---

## 📝 Change Log

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-10-25 | Security Review Team | Initial roadmap |

---

## 🤝 Contributing to Security

Found a security issue? Please follow our responsible disclosure policy:

1. **Do not** open a public GitHub issue
2. Email security@[domain].com with details
3. Allow 90 days for patching before disclosure
4. We will acknowledge within 48 hours

For non-security contributions, see [CONTRIBUTING.md](../CONTRIBUTING.md)
