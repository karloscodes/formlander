package accounts

import (
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EndedSession is a session that was signed out before it expired. The
// session cookie is signed, not stored, so a copy of it would stay valid
// until its expiry unless the sign-out is recorded here.
type EndedSession struct {
	ID     uint `gorm:"primaryKey"`
	UserID uint `gorm:"uniqueIndex:idx_ended_sessions_user_issued;not null"`
	// IssuedAt is the issued_at of the session cookie, in microseconds since
	// 1970. It tells the sessions of one user apart.
	IssuedAt  int64     `gorm:"uniqueIndex:idx_ended_sessions_user_issued;not null"`
	CreatedAt time.Time `gorm:"index"`
}

// EndSession records that the session of userID issued at issuedAt is signed
// out. Records older than maxAge go away: the sessions they name have expired.
func EndSession(logger *slog.Logger, db *gorm.DB, userID uint, issuedAt time.Time, maxAge time.Duration) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("created_at < ?", time.Now().Add(-maxAge)).Delete(&EndedSession{}).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&EndedSession{UserID: userID, IssuedAt: issuedAt.UnixMicro()}).Error
	})
}

// SessionValid reports whether a session of userID issued at issuedAt still
// counts: the user exists, the session is not older than the last password
// change, and nobody signed it out. An error counts as not valid, so a failed
// lookup never lets a session in.
func SessionValid(db *gorm.DB, userID uint, issuedAt time.Time) bool {
	user, err := FindByID(db, userID)
	if err != nil || !user.SessionIsCurrent(issuedAt) {
		return false
	}
	ended, err := SessionEnded(db, userID, issuedAt)
	return err == nil && !ended
}

// SessionEnded reports whether the session of userID issued at issuedAt was
// signed out.
func SessionEnded(db *gorm.DB, userID uint, issuedAt time.Time) (bool, error) {
	var count int64
	err := db.Model(&EndedSession{}).
		Where("user_id = ? AND issued_at = ?", userID, issuedAt.UnixMicro()).
		Count(&count).Error
	return count > 0, err
}
