package jobs

import (
	"time"

	"gorm.io/gorm"

	"formlander/internal/forms"
)

// claimLease is how long a dispatcher owns a delivery while it sends it. A
// process that stops mid-send leaves the delivery due again after it.
const claimLease = 5 * time.Minute

// claim takes a due delivery for this process before it sends it. During a
// deploy the old and the new container both run the dispatchers and can read
// the same pending delivery; only the one whose update changes the row sends
// it, so the webhook or email goes out once. Marking the result afterwards
// sets next_attempt_at again.
func claim(db *gorm.DB, model any, id uint, now time.Time) (bool, error) {
	res := db.Model(model).
		Where("id = ? AND status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
			id, []string{forms.WebhookStatusPending, forms.WebhookStatusRetrying}, now).
		Update("next_attempt_at", now.Add(claimLease))
	return res.RowsAffected == 1, res.Error
}
