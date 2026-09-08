package storage

import (
	"strings"
	"testing"

	"go.ripls.org/ripls/server/logging"
)

func TestSafeDSNFields(t *testing.T) {
	tests := []struct {
		name             string
		dsn              string
		wantHost         string
		wantDBName       string
		wantUser         string
		wantSSLMode      string
		wantPasswordFree bool
	}{
		{
			name:             "full dsn with password",
			dsn:              "postgres://ripls:supersecret@db.example.com:5432/mydb?sslmode=require",
			wantHost:         "db.example.com:5432",
			wantDBName:       "mydb",
			wantUser:         "ripls",
			wantSSLMode:      "require",
			wantPasswordFree: true,
		},
		{
			name:             "dsn without sslmode",
			dsn:              "postgres://user:pass@localhost:5432/testdb",
			wantHost:         "localhost:5432",
			wantDBName:       "testdb",
			wantUser:         "user",
			wantSSLMode:      "",
			wantPasswordFree: true,
		},
		{
			name:             "dev connection string with sslmode",
			dsn:              "postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable",
			wantHost:         "localhost:5432",
			wantDBName:       "ripls",
			wantUser:         "ripls",
			wantSSLMode:      "disable",
			wantPasswordFree: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := SafeDSNFields(tc.dsn)
			if len(fields)%2 != 0 {
				t.Fatalf("SafeDSNFields returned odd-length slice: %v", fields)
			}
			kv := make(map[string]string, len(fields)/2)
			for i := 0; i < len(fields); i += 2 {
				k, ok := fields[i].(string)
				if !ok {
					t.Fatalf("key at index %d is not a string: %v", i, fields[i])
				}
				v, ok := fields[i+1].(string)
				if !ok {
					t.Fatalf("value at index %d is not a string: %v", i+1, fields[i+1])
				}
				kv[k] = v
			}

			if got := kv["db_host"]; got != tc.wantHost {
				t.Errorf("db_host = %q; want %q", got, tc.wantHost)
			}
			if got := kv["db_name"]; got != tc.wantDBName {
				t.Errorf("db_name = %q; want %q", got, tc.wantDBName)
			}
			if got := kv["db_user"]; got != tc.wantUser {
				t.Errorf("db_user = %q; want %q", got, tc.wantUser)
			}
			if tc.wantSSLMode != "" {
				if got := kv["db_sslmode"]; got != tc.wantSSLMode {
					t.Errorf("db_sslmode = %q; want %q", got, tc.wantSSLMode)
				}
			} else {
				if _, ok := kv["db_sslmode"]; ok {
					t.Errorf("db_sslmode present but should be absent")
				}
			}

			if tc.wantPasswordFree {
				for k, v := range kv {
					if v == "supersecret" || v == "pass" || v == "ripls_dev" {
						t.Errorf("password value found in field %q", k)
					}
				}
				// verify the full DSN itself is not among the values
				for i := 1; i < len(fields); i += 2 {
					if fields[i] == tc.dsn {
						t.Error("full DSN with password found in logged fields")
					}
				}
			}
		})
	}
}

// TestInitializeDatabaseRejectsNonPostgresDSNWithoutLeakingIt asserts the
// invalid-scheme error never echoes the DSN — it can embed the DB password.
func TestInitializeDatabaseRejectsNonPostgresDSNWithoutLeakingIt(t *testing.T) {
	const dsn = "mysql://user:supersecret@host:3306/db"
	logger := logging.NewLogger(logging.Options{Level: "error", Format: "json"})
	_, err := InitializeDatabase(t.Context(), dsn, logger)
	if err == nil {
		t.Fatal("expected error for non-postgres DSN")
	}
	if strings.Contains(err.Error(), "supersecret") || strings.Contains(err.Error(), dsn) {
		t.Errorf("error leaks the DSN: %v", err)
	}
	if !strings.Contains(err.Error(), "postgres://") {
		t.Errorf("error should name the expected schemes: %v", err)
	}
}
