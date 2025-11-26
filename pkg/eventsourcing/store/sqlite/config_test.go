package sqlite

import (
	"strings"
	"testing"
	"time"
)

func TestEventStoreConfig(t *testing.T) {
	tests := []struct {
		name     string
		opts     []EventStoreOption
		wantDSN  string
		contains []string
	}{
		{
			name:    "Default",
			opts:    []EventStoreOption{},
			wantDSN: "eventeventsourcing.db",
		},
		{
			name: "WithEncryption",
			opts: []EventStoreOption{
				WithDSN("file:test.db"),
				WithEncryption("secret-key"),
			},
			wantDSN: "file:test.db?_key=secret-key",
		},
		{
			name: "WithSyncInterval",
			opts: []EventStoreOption{
				WithDSN("file:test.db"),
				WithSyncInterval(5 * time.Minute),
			},
			wantDSN: "file:test.db?_sync_interval=5m0s",
		},
		{
			name: "WithAuthToken",
			opts: []EventStoreOption{
				WithDSN("libsql://remote.db"),
				WithAuthToken("jwt-token"),
			},
			wantDSN: "libsql://remote.db?_auth_token=jwt-token",
		},
		{
			name: "Combined Options",
			opts: []EventStoreOption{
				WithDSN("file:test.db"),
				WithEncryption("key"),
				WithSyncInterval(1 * time.Second),
				WithAuthToken("token"),
			},
			contains: []string{
				"file:test.db",
				"_key=key",
				"_sync_interval=1s",
				"_auth_token=token",
			},
		},
		{
			name: "Embedded Replica with Auth",
			opts: []EventStoreOption{
				WithLibSQLEmbeddedReplica("local.db", "libsql://remote", "token"),
			},
			wantDSN: "file:local.db?_embedded_replica=1&_sync_url=libsql://remote&_auth_token=token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultEventStoreConfig()
			for _, opt := range tt.opts {
				opt(&config)
			}

			if tt.wantDSN != "" && config.dsn != tt.wantDSN {
				t.Errorf("got DSN %q, want %q", config.dsn, tt.wantDSN)
			}

			for _, substr := range tt.contains {
				if !strings.Contains(config.dsn, substr) {
					t.Errorf("DSN %q missing substring %q", config.dsn, substr)
				}
			}
		})
	}
}
