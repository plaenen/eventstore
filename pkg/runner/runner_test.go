package runner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockService is a mock implementation of Service
type MockService struct {
	mock.Mock
	name string
}

func (m *MockService) Name() string {
	return m.name
}

func (m *MockService) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockService) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockConfigProvider is a mock implementation of config.Provider
type MockConfigProvider[T any] struct {
	mock.Mock
}

func (m *MockConfigProvider[T]) Get(ctx context.Context) (T, error) {
	args := m.Called(ctx)
	return args.Get(0).(T), args.Error(1)
}

func (m *MockConfigProvider[T]) Watch(ctx context.Context, handler func(T)) (func(), error) {
	args := m.Called(ctx, handler)
	return args.Get(0).(func()), args.Error(1)
}

func (m *MockConfigProvider[T]) Latest() (T, error) {
	args := m.Called()
	return args.Get(0).(T), args.Error(1)
}

func (m *MockConfigProvider[T]) Close() error {
	args := m.Called()
	return args.Error(0)
}

type TestConfig struct {
	Value string
}

func TestRunner_Run_Success(t *testing.T) {
	svc1 := &MockService{name: "svc1"}
	svc2 := &MockService{name: "svc2"}

	svc1.On("Start", mock.Anything).Return(nil)
	svc2.On("Start", mock.Anything).Return(nil)

	// Expect stop in reverse order
	var stopOrder []string
	var mu sync.Mutex

	svc2.On("Stop", mock.Anything).Run(func(args mock.Arguments) {
		mu.Lock()
		stopOrder = append(stopOrder, "svc2")
		mu.Unlock()
	}).Return(nil)

	svc1.On("Stop", mock.Anything).Run(func(args mock.Arguments) {
		mu.Lock()
		stopOrder = append(stopOrder, "svc1")
		mu.Unlock()
	}).Return(nil)

	r := New[TestConfig]([]Service{svc1, svc2})

	ctx, cancel := context.WithCancel(context.Background())

	// Run in background
	errCh := make(chan error)
	go func() {
		errCh <- r.Run(ctx)
	}()

	// Give time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	err := <-errCh
	assert.NoError(t, err)

	svc1.AssertExpectations(t)
	svc2.AssertExpectations(t)

	mu.Lock()
	assert.Equal(t, []string{"svc2", "svc1"}, stopOrder, "Services should stop in reverse order")
	mu.Unlock()
}

func TestRunner_Run_StartupFailure(t *testing.T) {
	svc1 := &MockService{name: "svc1"}
	svc2 := &MockService{name: "svc2"}

	svc1.On("Start", mock.Anything).Return(nil)
	svc2.On("Start", mock.Anything).Return(errors.New("startup failed"))

	// Only svc1 should be stopped
	svc1.On("Stop", mock.Anything).Return(nil)

	r := New[TestConfig]([]Service{svc1, svc2})

	err := r.Run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "startup failed")

	svc1.AssertExpectations(t)
	svc2.AssertExpectations(t)
}

func TestRunner_Run_ShutdownFailure(t *testing.T) {
	svc1 := &MockService{name: "svc1"}

	svc1.On("Start", mock.Anything).Return(nil)
	svc1.On("Stop", mock.Anything).Return(errors.New("shutdown failed"))

	r := New[TestConfig]([]Service{svc1})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediate shutdown

	err := r.Run(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown failed")

	svc1.AssertExpectations(t)
}
