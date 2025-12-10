package jobs

import (
	"errors"
	"strings"
	"testing"
	"time"

	"formlander/internal/config"
)

func TestNewRetryStrategy(t *testing.T) {
	cfg := &config.Config{}
	strategy := NewRetryStrategy(cfg)
	
	if strategy == nil {
		t.Fatal("NewRetryStrategy() returned nil")
	}
	
	if strategy.cfg != cfg {
		t.Error("NewRetryStrategy() did not set config correctly")
	}
}

func TestNextRetry(t *testing.T) {
	cfg := &config.Config{}
	cfg.Webhook.BackoffSchedule = "60,300,900,3600" // 1min, 5min, 15min, 1hour in seconds
	strategy := NewRetryStrategy(cfg)
	
	now := time.Now().UTC()
	
	tests := []struct {
		name           string
		attempt        int
		expectedOffset time.Duration
	}{
		{
			name:           "first retry",
			attempt:        1,
			expectedOffset: 60 * time.Second,
		},
		{
			name:           "second retry",
			attempt:        2,
			expectedOffset: 300 * time.Second,
		},
		{
			name:           "third retry",
			attempt:        3,
			expectedOffset: 900 * time.Second,
		},
		{
			name:           "fourth retry",
			attempt:        4,
			expectedOffset: 3600 * time.Second,
		},
		{
			name:           "beyond schedule uses last value",
			attempt:        10,
			expectedOffset: 3600 * time.Second,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextRetry := strategy.NextRetry(tt.attempt)
			if nextRetry == nil {
				t.Fatal("NextRetry() returned nil")
			}
			
			// Check the retry time is approximately correct (within 1 second)
			expected := now.Add(tt.expectedOffset)
			diff := nextRetry.Sub(expected)
			if diff < -time.Second || diff > time.Second {
				t.Errorf("NextRetry() = %v, want approximately %v (diff: %v)", *nextRetry, expected, diff)
			}
		})
	}
}

func TestNextRetryEmptySchedule(t *testing.T) {
	cfg := &config.Config{}
	cfg.Webhook.BackoffSchedule = ""
	strategy := NewRetryStrategy(cfg)
	
	nextRetry := strategy.NextRetry(1)
	if nextRetry != nil {
		t.Errorf("NextRetry() with empty schedule = %v, want nil", nextRetry)
	}
}

func TestShouldRetry(t *testing.T) {
	cfg := &config.Config{}
	strategy := NewRetryStrategy(cfg)
	
	tests := []struct {
		name         string
		attemptCount int
		want         bool
	}{
		{
			name:         "first attempt",
			attemptCount: 1,
			want:         true,
		},
		{
			name:         "second attempt",
			attemptCount: 2,
			want:         true,
		},
		{
			name:         "third attempt - at limit",
			attemptCount: 3,
			want:         false,
		},
		{
			name:         "fourth attempt - beyond limit",
			attemptCount: 4,
			want:         false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strategy.ShouldRetry(tt.attemptCount)
			if got != tt.want {
				t.Errorf("ShouldRetry(%d) = %v, want %v", tt.attemptCount, got, tt.want)
			}
		})
	}
}

func TestTruncateError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantLen   int
		wantEmpty bool
	}{
		{
			name:      "nil error",
			err:       nil,
			wantEmpty: true,
		},
		{
			name:    "short error",
			err:     errors.New("short error message"),
			wantLen: 19,
		},
		{
			name:    "error with whitespace",
			err:     errors.New("  error with spaces  "),
			wantLen: len(strings.TrimSpace("  error with spaces  ")),
		},
		{
			name:    "long error gets truncated",
			err:     errors.New(strings.Repeat("a", 600)),
			wantLen: 500,
		},
		{
			name:    "exactly 500 chars",
			err:     errors.New(strings.Repeat("b", 500)),
			wantLen: 500,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateError(tt.err)
			
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("TruncateError() = %q, want empty string", got)
				}
				return
			}
			
			if len(got) != tt.wantLen {
				t.Errorf("TruncateError() length = %d, want %d", len(got), tt.wantLen)
			}
			
			// Check for whitespace trimming
			if strings.TrimSpace(got) != got {
				t.Error("TruncateError() should trim whitespace")
			}
		})
	}
}

func TestUpdateOptions(t *testing.T) {
	t.Run("WithAttemptCount", func(t *testing.T) {
		values := make(map[string]any)
		opt := WithAttemptCount(5)
		opt(values)
		
		if values["attempt_count"] != 5 {
			t.Errorf("WithAttemptCount() set %v, want 5", values["attempt_count"])
		}
	})
	
	t.Run("WithNextAttempt", func(t *testing.T) {
		values := make(map[string]any)
		now := time.Now()
		opt := WithNextAttempt(&now)
		opt(values)
		
		if values["next_attempt_at"] != &now {
			t.Error("WithNextAttempt() did not set next_attempt_at correctly")
		}
	})
	
	t.Run("WithNextAttempt nil", func(t *testing.T) {
		values := make(map[string]any)
		opt := WithNextAttempt(nil)
		opt(values)
		
		val, exists := values["next_attempt_at"]
		if !exists {
			t.Error("WithNextAttempt(nil) should set key")
		}
		// val will be (*time.Time)(nil) which is not the same as untyped nil
		// Just check that it exists and can be used in database updates
		if _, ok := val.(*time.Time); !ok && val != nil {
			t.Errorf("WithNextAttempt(nil) set wrong type: %T", val)
		}
	})
	
	t.Run("multiple options", func(t *testing.T) {
		values := make(map[string]any)
		now := time.Now()
		
		opt1 := WithAttemptCount(3)
		opt2 := WithNextAttempt(&now)
		
		opt1(values)
		opt2(values)
		
		if values["attempt_count"] != 3 {
			t.Error("Multiple options: attempt_count not set")
		}
		if values["next_attempt_at"] != &now {
			t.Error("Multiple options: next_attempt_at not set")
		}
	})
}

func TestNewEventUpdater(t *testing.T) {
	type TestModel struct {
		ID uint
	}
	
	model := &TestModel{}
	updater := NewEventUpdater(model)
	
	if updater == nil {
		t.Fatal("NewEventUpdater() returned nil")
	}
	
	if updater.model != model {
		t.Error("NewEventUpdater() did not set model correctly")
	}
}

func TestRetryScheduleParsing(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		want     []int
	}{
		{
			name:     "standard schedule",
			schedule: "60,300,900",
			want:     []int{60, 300, 900},
		},
		{
			name:     "single value",
			schedule: "120",
			want:     []int{120},
		},
		{
			name:     "empty schedule",
			schedule: "",
			want:     nil, // Empty schedule should return default
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Webhook.BackoffSchedule = tt.schedule
			strategy := NewRetryStrategy(cfg)
			
			// Get the parsed schedule via WebhookBackoff
			schedule := cfg.WebhookBackoff()
			
			// Handle empty/nil schedule
			if tt.want == nil {
				// For empty schedule, we get the default from config
				if len(schedule) == 0 {
					next := strategy.NextRetry(1)
					if next != nil {
						t.Error("NextRetry() should return nil for empty schedule")
					}
				}
				return
			}
			
			if len(schedule) != len(tt.want) {
				t.Errorf("Schedule length = %d, want %d", len(schedule), len(tt.want))
				return
			}
			
			for i, v := range tt.want {
				if schedule[i] != v {
					t.Errorf("Schedule[%d] = %d, want %d", i, schedule[i], v)
				}
			}
		})
	}
}
