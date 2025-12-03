package accounts_test

import (
	"testing"
	"time"

	"formlander/internal/accounts"
	"formlander/internal/pkg/testsupport"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func createTestUser(t *testing.T, db *gorm.DB, email, password string, withLastLogin bool) *accounts.User {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	user := &accounts.User{
		Email:        email,
		PasswordHash: string(hash),
	}

	if withLastLogin {
		now := time.Now()
		user.LastLoginAt = &now
	}

	err = db.Create(user).Error
	require.NoError(t, err)

	return user
}

func TestAuthenticate(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("authenticates with correct credentials", func(t *testing.T) {
		createTestUser(t, db, "test@example.com", "password123", false)

		result, err := accounts.Authenticate(logger, db, "test@example.com", "password123")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "test@example.com", result.User.Email)
		assert.True(t, result.IsFirstLogin, "Should be first login")
		assert.NotNil(t, result.User.LastLoginAt, "LastLoginAt should be set")
	})

	t.Run("updates last login on subsequent logins", func(t *testing.T) {
		db2 := testsupport.SetupTestDB(t)
		createTestUser(t, db2, "user@example.com", "password123", true)

		result, err := accounts.Authenticate(logger, db2, "user@example.com", "password123")
		require.NoError(t, err)
		assert.False(t, result.IsFirstLogin, "Should not be first login")
	})

	t.Run("rejects wrong password", func(t *testing.T) {
		db3 := testsupport.SetupTestDB(t)
		createTestUser(t, db3, "user@example.com", "correctpassword", false)

		_, err := accounts.Authenticate(logger, db3, "user@example.com", "wrongpassword")
		assert.ErrorIs(t, err, accounts.ErrInvalidCredentials)
	})

	t.Run("rejects non-existent user", func(t *testing.T) {
		db4 := testsupport.SetupTestDB(t)

		_, err := accounts.Authenticate(logger, db4, "nonexistent@example.com", "password")
		assert.ErrorIs(t, err, accounts.ErrInvalidCredentials)
	})

	t.Run("validates required fields", func(t *testing.T) {
		db5 := testsupport.SetupTestDB(t)

		_, err := accounts.Authenticate(logger, db5, "", "password")
		assert.ErrorIs(t, err, accounts.ErrMissingFields)

		_, err = accounts.Authenticate(logger, db5, "user@example.com", "")
		assert.ErrorIs(t, err, accounts.ErrMissingFields)
	})

	t.Run("normalizes email to lowercase", func(t *testing.T) {
		db6 := testsupport.SetupTestDB(t)
		createTestUser(t, db6, "user@example.com", "password123", false)

		result, err := accounts.Authenticate(logger, db6, "USER@EXAMPLE.COM", "password123")
		require.NoError(t, err)
		assert.Equal(t, "user@example.com", result.User.Email)
	})

	t.Run("trims whitespace from email", func(t *testing.T) {
		db7 := testsupport.SetupTestDB(t)
		createTestUser(t, db7, "user@example.com", "password123", false)

		result, err := accounts.Authenticate(logger, db7, "  user@example.com  ", "password123")
		require.NoError(t, err)
		assert.Equal(t, "user@example.com", result.User.Email)
	})
}

func TestChangePassword(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("changes password successfully", func(t *testing.T) {
		createTestUser(t, db, "user@example.com", "oldpassword", false)

		err := accounts.ChangePassword(logger, db, "user@example.com", "oldpassword", "newpassword123")
		require.NoError(t, err)

		// Verify new password works
		result, err := accounts.Authenticate(logger, db, "user@example.com", "newpassword123")
		require.NoError(t, err)
		assert.NotNil(t, result)

		// Verify old password doesn't work
		_, err = accounts.Authenticate(logger, db, "user@example.com", "oldpassword")
		assert.ErrorIs(t, err, accounts.ErrInvalidCredentials)
	})

	t.Run("rejects weak password", func(t *testing.T) {
		db2 := testsupport.SetupTestDB(t)
		createTestUser(t, db2, "user@example.com", "oldpassword", false)

		err := accounts.ChangePassword(logger, db2, "user@example.com", "oldpassword", "weak")
		assert.ErrorIs(t, err, accounts.ErrWeakPassword)
	})

	t.Run("validates current password", func(t *testing.T) {
		db3 := testsupport.SetupTestDB(t)
		createTestUser(t, db3, "user@example.com", "correctpassword", false)

		err := accounts.ChangePassword(logger, db3, "user@example.com", "wrongpassword", "newpassword123")
		assert.ErrorIs(t, err, accounts.ErrPasswordMismatch)
	})

	t.Run("rejects non-existent user", func(t *testing.T) {
		db4 := testsupport.SetupTestDB(t)

		err := accounts.ChangePassword(logger, db4, "nonexistent@example.com", "old", "newpassword123")
		assert.ErrorIs(t, err, accounts.ErrUserNotFound)
	})

	t.Run("accepts password with exactly 8 characters", func(t *testing.T) {
		db5 := testsupport.SetupTestDB(t)
		createTestUser(t, db5, "user@example.com", "oldpassword", false)

		err := accounts.ChangePassword(logger, db5, "user@example.com", "oldpassword", "12345678")
		require.NoError(t, err)
	})
}

func TestGetByEmail(t *testing.T) {
	db := testsupport.SetupTestDB(t)

	t.Run("retrieves existing user", func(t *testing.T) {
		created := createTestUser(t, db, "user@example.com", "password123", false)

		user, err := accounts.GetByEmail(db, "user@example.com")
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, created.ID, user.ID)
		assert.Equal(t, "user@example.com", user.Email)
	})

	t.Run("returns error for non-existent user", func(t *testing.T) {
		db2 := testsupport.SetupTestDB(t)

		_, err := accounts.GetByEmail(db2, "nonexistent@example.com")
		assert.ErrorIs(t, err, accounts.ErrUserNotFound)
	})
}

func TestPasswordHashing(t *testing.T) {
	t.Run("password is properly hashed", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		user := createTestUser(t, db, "user@example.com", "mypassword", false)

		// Password should be hashed, not stored in plaintext
		assert.NotEqual(t, "mypassword", user.PasswordHash)
		assert.Greater(t, len(user.PasswordHash), 50, "Hash should be substantial length")

		// Should start with bcrypt prefix
		assert.Contains(t, user.PasswordHash, "$2a$", "Should use bcrypt")
	})

	t.Run("same password generates different hashes", func(t *testing.T) {
		hash1, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
		hash2, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

		// Due to salt, hashes should be different
		assert.NotEqual(t, string(hash1), string(hash2))
	})
}

func TestAuthenticationSecurity(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("password comparison uses bcrypt", func(t *testing.T) {
		// Verify that wrong password attempts use bcrypt (timing-safe)
		createTestUser(t, db, "user@example.com", "password123", false)

		start := time.Now()
		_, err := accounts.Authenticate(logger, db, "user@example.com", "wrongpassword")
		duration := time.Since(start)

		// bcrypt should take at least 10ms (usually 50-100ms)
		assert.Error(t, err)
		assert.Greater(t, duration.Milliseconds(), int64(10), "bcrypt comparison should take measurable time")
	})

	t.Run("prevents SQL injection in email", func(t *testing.T) {
		db2 := testsupport.SetupTestDB(t)
		createTestUser(t, db2, "user@example.com", "password123", false)

		// Try SQL injection
		_, err := accounts.Authenticate(logger, db2, "user@example.com' OR '1'='1", "password123")
		assert.Error(t, err, "Should not authenticate with SQL injection attempt")
	})
}

func TestFirstLoginDetection(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("detects first login correctly", func(t *testing.T) {
		createTestUser(t, db, "user@example.com", "password123", false)

		// First login
		result1, err := accounts.Authenticate(logger, db, "user@example.com", "password123")
		require.NoError(t, err)
		assert.True(t, result1.IsFirstLogin)

		// Second login
		result2, err := accounts.Authenticate(logger, db, "user@example.com", "password123")
		require.NoError(t, err)
		assert.False(t, result2.IsFirstLogin)
	})
}
