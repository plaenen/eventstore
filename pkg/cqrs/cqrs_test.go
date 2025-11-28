package cqrs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultTransportConfig(t *testing.T) {
	config := DefaultTransportConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 5, config.MaxReconnectAttempts)
	assert.Equal(t, 2*time.Second, config.ReconnectWait)
	assert.Equal(t, 3, config.MaxRetries)
}

func TestDefaultServerConfig(t *testing.T) {
	config := DefaultServerConfig()
	assert.NotNil(t, config)
	assert.Equal(t, "default-handlers", config.QueueGroup)
	assert.Equal(t, 100, config.MaxConcurrent)
	assert.Equal(t, 30*time.Second, config.HandlerTimeout)
}

func TestDefaultSubjectBuilder(t *testing.T) {
	builder := &DefaultSubjectBuilder{}
	subject := builder.BuildSubject(context.Background(), "pkg", "svc", "method")
	assert.Equal(t, "pkg.svc.method", subject)
}

func TestPrefixedSubjectBuilder(t *testing.T) {
	t.Run("StaticPrefix", func(t *testing.T) {
		builder := &PrefixedSubjectBuilder{
			Prefix: "test",
		}
		subject := builder.BuildSubject(context.Background(), "pkg", "svc", "method")
		assert.Equal(t, "test.pkg.svc.method", subject)
	})

	t.Run("DynamicPrefix", func(t *testing.T) {
		builder := &PrefixedSubjectBuilder{
			PrefixFunc: func(ctx context.Context) string {
				if val, ok := ctx.Value("tenant").(string); ok {
					return val
				}
				return "default"
			},
		}

		ctx := context.WithValue(context.Background(), "tenant", "tenant1")
		subject := builder.BuildSubject(ctx, "pkg", "svc", "method")
		assert.Equal(t, "tenant1.pkg.svc.method", subject)

		subject = builder.BuildSubject(context.Background(), "pkg", "svc", "method")
		assert.Equal(t, "default.pkg.svc.method", subject)
	})

	t.Run("NoPrefix", func(t *testing.T) {
		builder := &PrefixedSubjectBuilder{}
		subject := builder.BuildSubject(context.Background(), "pkg", "svc", "method")
		assert.Equal(t, "pkg.svc.method", subject)
	})
}

func TestContextSubjectBuilder(t *testing.T) {
	ctx := context.Background()

	// Default
	builder := SubjectBuilderFromContext(ctx)
	assert.IsType(t, &DefaultSubjectBuilder{}, builder)

	// With builder
	customBuilder := &PrefixedSubjectBuilder{Prefix: "custom"}
	ctx = WithSubjectBuilder(ctx, customBuilder)
	builder = SubjectBuilderFromContext(ctx)
	assert.Equal(t, customBuilder, builder)
}

func TestClientOption(t *testing.T) {
	// Mock client that implements the setter interface
	mockClient := &mockClient{}

	builder := &DefaultSubjectBuilder{}
	opt := WithClientSubjectBuilder(builder)

	opt(mockClient)
	assert.Equal(t, builder, mockClient.builder)
}

type mockClient struct {
	builder SubjectBuilder
}

func (m *mockClient) setSubjectBuilder(b SubjectBuilder) {
	m.builder = b
}
