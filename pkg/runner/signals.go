package runner

import (
	"os"
	"os/signal"
	"syscall"
)

// WaitForShutdownSignal blocks until an OS interrupt or termination signal
// is received. This is typically used to gracefully shutdown services.
func WaitForShutdownSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
}
