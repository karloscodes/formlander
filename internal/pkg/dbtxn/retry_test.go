package dbtxn

import (
	"errors"
	"strings"
	"testing"
)

func TestIsBusyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "database is locked",
			err:  errors.New("database is locked"),
			want: true,
		},
		{
			name: "database is busy",
			err:  errors.New("database is busy"),
			want: true,
		},
		{
			name: "database table is locked",
			err:  errors.New("database table is locked"),
			want: true,
		},
		{
			name: "SQL statements in progress",
			err:  errors.New("SQL statements in progress"),
			want: true,
		},
		{
			name: "case sensitive - lowercase",
			err:  errors.New("the database is locked by another process"),
			want: true,
		},
		{
			name: "case sensitive - mixed",
			err:  errors.New("Database Is Busy"),
			want: false, // Case matters
		},
		{
			name: "unrelated error",
			err:  errors.New("permission denied"),
			want: false,
		},
		{
			name: "connection error",
			err:  errors.New("unable to open database file"),
			want: false,
		},
		{
			name: "constraint violation",
			err:  errors.New("UNIQUE constraint failed"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBusyError(tt.err)
			if got != tt.want {
				t.Errorf("isBusyError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsBusyErrorSubstrings(t *testing.T) {
	// Test that error detection works with substrings
	busyMessages := []string{
		"database is locked",
		"database is busy",
		"database table is locked",
		"SQL statements in progress",
	}

	for _, msg := range busyMessages {
		t.Run(msg, func(t *testing.T) {
			// Test with prefix
			err := errors.New("error: " + msg)
			if !isBusyError(err) {
				t.Errorf("isBusyError() should detect '%s' with prefix", msg)
			}

			// Test with suffix
			err = errors.New(msg + " - please retry")
			if !isBusyError(err) {
				t.Errorf("isBusyError() should detect '%s' with suffix", msg)
			}

			// Test with both
			err = errors.New("error: " + msg + " - retry needed")
			if !isBusyError(err) {
				t.Errorf("isBusyError() should detect '%s' with prefix and suffix", msg)
			}
		})
	}
}

func TestIsBusyErrorCaseSensitivity(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "exact match lowercase",
			msg:  "database is locked",
			want: true,
		},
		{
			name: "uppercase LOCKED",
			msg:  "database is LOCKED",
			want: false, // Case-sensitive
		},
		{
			name: "uppercase DATABASE",
			msg:  "DATABASE is locked",
			want: false, // Case-sensitive
		},
		{
			name: "all uppercase",
			msg:  "DATABASE IS LOCKED",
			want: false, // Case-sensitive
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.msg)
			got := isBusyError(err)
			if got != tt.want {
				t.Errorf("isBusyError(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestIsBusyErrorMultipleMatches(t *testing.T) {
	// Test error message with multiple busy indicators
	err := errors.New("database is locked and database is busy")
	if !isBusyError(err) {
		t.Error("isBusyError() should detect error with multiple busy indicators")
	}
}

func TestIsBusyErrorRealWorldMessages(t *testing.T) {
	// Real-world error messages that might occur
	realErrors := []struct {
		msg  string
		want bool
	}{
		{
			msg:  "unable to open database file: out of memory",
			want: false,
		},
		{
			msg:  "disk I/O error",
			want: false,
		},
		{
			msg:  "database disk image is malformed",
			want: false,
		},
		{
			msg:  "attempt to write a readonly database",
			want: false,
		},
		{
			msg:  "database is locked",
			want: true,
		},
		{
			msg:  "sqlite3: database is locked",
			want: true,
		},
		{
			msg:  "Error 5 (database is locked)",
			want: true,
		},
	}

	for _, tt := range realErrors {
		t.Run(tt.msg, func(t *testing.T) {
			err := errors.New(tt.msg)
			got := isBusyError(err)
			if got != tt.want {
				t.Errorf("isBusyError(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

// Benchmark to ensure isBusyError is fast (it's in hot path)
func BenchmarkIsBusyError(b *testing.B) {
	err := errors.New("database is locked")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isBusyError(err)
	}
}

func BenchmarkIsBusyErrorNil(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isBusyError(nil)
	}
}

func BenchmarkIsBusyErrorNotBusy(b *testing.B) {
	err := errors.New("some other error message")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isBusyError(err)
	}
}

func TestIsBusyErrorEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "empty string error",
			err:  errors.New(""),
			want: false,
		},
		{
			name: "just whitespace",
			err:  errors.New("   "),
			want: false,
		},
		{
			name: "partial match 'locked' only",
			err:  errors.New("locked"),
			want: false, // Need full phrase "database is locked"
		},
		{
			name: "partial match 'database' only",
			err:  errors.New("database"),
			want: false,
		},
		{
			name: "almost matching",
			err:  errors.New("database was locked"),
			want: false, // Not exact substring
		},
		{
			name: "newlines in error",
			err:  errors.New("error occurred:\ndatabase is locked\nplease retry"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBusyError(tt.err)
			if got != tt.want {
				t.Errorf("isBusyError(%q) = %v, want %v", tt.err.Error(), got, tt.want)
			}
		})
	}
}

func TestIsBusyErrorPerformance(t *testing.T) {
	// Ensure the function uses Contains and not slower methods
	err := errors.New(strings.Repeat("x", 10000) + "database is locked" + strings.Repeat("y", 10000))
	
	if !isBusyError(err) {
		t.Error("isBusyError() should find match in large error message")
	}
}
