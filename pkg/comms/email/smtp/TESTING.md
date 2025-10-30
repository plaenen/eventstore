# SMTP Sender Testing Guide

This package includes comprehensive integration tests using **Testcontainers** for automatic Mailpit management.

## Quick Start

### Prerequisites

- Docker installed and running
- Go 1.21+

### Run Tests

```bash
# Run all integration tests
cd pkg/email/smtp
go test -tags=integration -v

# Run specific test
go test -tags=integration -v -run TestMailpitIntegration_BasicEmail

# Run with timeout
go test -tags=integration -v -timeout=5m
```

## What Gets Tested

### ✅ Testcontainers Integration Tests

Tests automatically start/stop Mailpit containers:

- **Basic Email** - Simple text email
- **HTML Email** - HTML with plain text fallback
- **Attachments** - File attachments
- **Batch Sending** - Multiple emails in one connection
- **Options Pattern** - Various configuration options

### Features Tested

| Feature | Test Coverage |
|---------|--------------|
| Plain text emails | ✅ |
| HTML emails | ✅ |
| Multipart (text + HTML) | ✅ |
| Attachments | ✅ |
| Inline images (CID) | ✅ |
| CC recipients | ✅ |
| BCC recipients | ✅ |
| Custom headers | ✅ |
| Email priority | ✅ |
| Batch sending | ✅ |
| Options pattern | ✅ |
| Connection testing | ✅ |

## Test Architecture

### Testcontainers Approach (Recommended)

The testcontainers tests (`smtp_integration_test.go`) automatically:

1. Start a Mailpit container before each test
2. Configure SMTP sender with correct ports
3. Run test scenarios
4. Clean up container after test

**Advantages:**
- ✅ Zero manual setup
- ✅ Isolated per test
- ✅ Automatically managed lifecycle
- ✅ Parallel-safe
- ✅ CI/CD friendly

**Example:**

```go
func TestMailpitIntegration_BasicEmail(t *testing.T) {
    host, port, webUI, cleanup := setupMailpit(t)
    defer cleanup()

    sender := smtp.NewSender(
        smtp.WithHost(host),
        smtp.WithPort(mustAtoi(port)),
        smtp.WithSkipVerify(),
    )

    // Test your email sending...
}
```

### Manual Approach (Optional)

Alternative: Start Mailpit manually using Docker Compose:

```bash
# Start Mailpit
docker-compose up -d

# Access Web UI
open http://localhost:8025

# Run legacy integration tests
go test -tags=integration -v ./integration_test.go

# Stop Mailpit
docker-compose down
```

## Unit Tests

Run unit tests without Docker:

```bash
# Run unit tests only (no integration tag)
go test -v

# Run with coverage
go test -v -cover

# Generate coverage report
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Viewing Test Emails

When running integration tests, Mailpit provides a web UI:

**With Testcontainers:**
- Each test logs its Web UI URL
- Look for: `📧 View at: http://localhost:XXXXX`
- Port is dynamically assigned

**With Docker Compose:**
- Always at: http://localhost:8025

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run integration tests
        run: |
          cd pkg/email/smtp
          go test -tags=integration -v -timeout=10m
```

**Note:** Testcontainers automatically uses Docker in CI environments.

### GitLab CI Example

```yaml
test:integration:
  image: golang:1.21
  services:
    - docker:dind
  variables:
    DOCKER_HOST: tcp://docker:2375
    DOCKER_DRIVER: overlay2
  script:
    - cd pkg/email/smtp
    - go test -tags=integration -v -timeout=10m
```

## Troubleshooting

### "Cannot connect to Docker daemon"

**Problem:** Testcontainers can't reach Docker

**Solutions:**
```bash
# Check Docker is running
docker ps

# Check Docker socket permissions (Linux)
sudo chmod 666 /var/run/docker.sock

# Or add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

### "Port already in use"

**Problem:** Another Mailpit instance is running

**Solution:**
```bash
# Find and stop existing containers
docker ps
docker stop <container-id>

# Or stop all Mailpit containers
docker ps -a | grep mailpit | awk '{print $1}' | xargs docker stop
```

### Tests timeout

**Problem:** Container takes too long to start

**Solution:**
```bash
# Increase timeout
go test -tags=integration -v -timeout=10m

# Pre-pull the image
docker pull axllent/mailpit:latest

# Check Docker resources (increase if needed)
docker info
```

### "Container failed to start"

**Problem:** Mailpit container won't start

**Debug:**
```bash
# Check Docker logs
docker logs <container-id>

# Try running Mailpit manually
docker run --rm -p 1025:1025 -p 8025:8025 axllent/mailpit

# Check available ports
netstat -an | grep LISTEN
```

## Performance Testing

Test batch sending performance:

```bash
# Run batch test with timing
go test -tags=integration -v -run TestMailpitIntegration_BatchSend

# Example output:
# ✅ Sent 10 emails in 234ms (42.74 emails/sec)
```

## Writing New Tests

Template for new integration tests:

```go
func TestMailpitIntegration_YourTest(t *testing.T) {
    host, port, webUI, cleanup := setupMailpit(t)
    defer cleanup()

    sender := smtp.NewSender(
        smtp.WithHost(host),
        smtp.WithPort(mustAtoi(port)),
        smtp.WithSkipVerify(),
    )

    // Your test code here
    email := emailpkg.NewEmail().
        From("test@example.com").
        To("recipient@example.com").
        Subject("Your Test").
        TextBody("Test body").
        Build()

    ctx := context.Background()
    if err := sender.Send(ctx, email); err != nil {
        t.Fatalf("Send failed: %v", err)
    }

    t.Log("✅ Test passed")
    t.Logf("📧 View at: %s", webUI)
}
```

## Best Practices

1. **Use Testcontainers** for integration tests (automatic setup)
2. **Use `defer cleanup()`** to ensure containers are stopped
3. **Log Web UI URLs** for debugging (` t.Logf("📧 View at: %s", webUI)`)
4. **Test with context timeouts** to prevent hanging
5. **Run tests in parallel** when possible (testcontainers handles isolation)
6. **Pre-pull images** in CI for faster tests

## Resources

- [Testcontainers Go](https://golang.testcontainers.org/)
- [Mailpit Documentation](https://github.com/axllent/mailpit)
- [SMTP RFC 5321](https://tools.ietf.org/html/rfc5321)
