package config

import (
	"os"
	"testing"
)

func TestNewConfigBuildsWorkerAddressFromHostAndPort(t *testing.T) {
	t.Setenv("WORKER_HOST", "127.0.0.1")
	t.Setenv("WORKER_PORT", ":30031")
	unsetEnv(t, "WORKER_ADDRESS")

	conf := NewConfig()
	if conf.WorkerAddress != "127.0.0.1:30031" {
		t.Fatalf("expected worker address 127.0.0.1:30031, got %s", conf.WorkerAddress)
	}
}

func TestNewConfigWorkerAddressOverride(t *testing.T) {
	t.Setenv("WORKER_HOST", "127.0.0.1")
	t.Setenv("WORKER_PORT", ":30031")
	t.Setenv("WORKER_ADDRESS", "worker.example:50051")

	conf := NewConfig()
	if conf.WorkerAddress != "worker.example:50051" {
		t.Fatalf("expected worker address override, got %s", conf.WorkerAddress)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	previous, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("failed to unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	})
}
