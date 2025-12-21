package forms

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"

	"formlander/internal/integrations"
)

const (
	WebhookStatusPending    = "pending"
	WebhookStatusDelivering = "delivering"
	WebhookStatusDelivered  = "delivered"
	WebhookStatusRetrying   = "retrying"
	WebhookStatusFailed     = "failed"

	// DefaultRetryLimit is the opinionated retry count for all deliveries
	DefaultRetryLimit = 3
)

// Form denotes a configured form endpoint.
type Form struct {
	ID                   uint                         `gorm:"primaryKey"`
	PublicID             string                       `gorm:"size:20;uniqueIndex;not null"`
	Name                 string                       `gorm:"size:255;not null"`
	Slug                 string                       `gorm:"size:255;uniqueIndex;not null"`
	Token                string                       `gorm:"size:64;uniqueIndex;not null"`
	AllowedOrigins       string                       `gorm:"type:text"` // Required. Comma-separated domains (example.com,*.example.com) or * for all
	UseSDK               bool                         `gorm:"not null;default:false"` // Include JavaScript SDK in form code
	GeneratedHTML        string                       `gorm:"type:text"` // AI-generated form HTML (optional)
	CaptchaProfileID     *uint                        `gorm:"index"`     // Foreign key to CaptchaProfile
	CaptchaProfile       *integrations.CaptchaProfile `gorm:"constraint:OnDelete:SET NULL"`
	CaptchaOverridesJSON string                       `gorm:"type:text"` // JSON: {required, action, widget}
	CreatedAt            time.Time
	UpdatedAt            time.Time

	Submissions     []Submission
	WebhookDelivery *WebhookDelivery `gorm:"constraint:OnDelete:CASCADE"`
	EmailDelivery   *EmailDelivery   `gorm:"constraint:OnDelete:CASCADE"`
}

// BeforeCreate ensures generated identifiers exist for new forms.
func (f *Form) BeforeCreate(tx *gorm.DB) error {
	if f.PublicID == "" {
		f.PublicID = GeneratePublicID()
	}
	if f.Token == "" {
		token, err := generateToken(24)
		if err != nil {
			return err
		}
		f.Token = token
	}
	if f.Slug == "" {
		f.Slug = Slugify(f.Name)
	}
	return nil
}

// BeforeSave keeps the slug in sync with the current name when unset by user.
func (f *Form) BeforeSave(tx *gorm.DB) error {
	if f.Slug == "" {
		f.Slug = Slugify(f.Name)
	}
	return nil
}

// GeneratePublicID generates a 20-character hex public ID for a form
func GeneratePublicID() string {
	buf := make([]byte, 10) // 10 bytes = 20 hex characters
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

func generateToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Slugify converts input string to a URL-friendly slug
func Slugify(input string) string {
	if input == "" {
		return GeneratePublicID()
	}

	var builder strings.Builder
	builder.Grow(len(input))

	lastWasDash := false
	for _, r := range strings.ToLower(input) {
		switch {
		case r == '-' || r == '_':
			if !lastWasDash && builder.Len() > 0 {
				builder.WriteRune('-')
				lastWasDash = true
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastWasDash = false
		case unicode.IsSpace(r):
			if !lastWasDash && builder.Len() > 0 {
				builder.WriteRune('-')
				lastWasDash = true
			}
		default:
			// Skip any remaining characters.
		}
	}

	slug := builder.String()
	if slug == "" {
		return GeneratePublicID()
	}
	return slug
}

// WebhookDelivery captures webhook configuration for a form.
type WebhookDelivery struct {
	ID          uint   `gorm:"primaryKey"`
	FormID      uint   `gorm:"uniqueIndex;not null"`
	Form        *Form  `gorm:"constraint:OnDelete:CASCADE"`
	Enabled     bool   `gorm:"not null;default:false"`
	URL         string `gorm:"type:text"`
	Secret      string `gorm:"size:255"`
	HeadersJSON string `gorm:"type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EmailDelivery captures email forwarding configuration for a form.
type EmailDelivery struct {
	ID              uint                        `gorm:"primaryKey"`
	FormID          uint                        `gorm:"uniqueIndex;not null"`
	Form            *Form                       `gorm:"constraint:OnDelete:CASCADE"`
	Enabled         bool                        `gorm:"not null;default:false"`
	MailerProfileID *uint                       `gorm:"index"` // Foreign key to MailerProfile
	MailerProfile   *integrations.MailerProfile `gorm:"constraint:OnDelete:SET NULL"`
	OverridesJSON   string                      `gorm:"type:text"` // JSON: {to, cc, bcc, subject, template, tags, reply_to}
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Submission stores the payload received from a public form post.
type Submission struct {
	ID        uint   `gorm:"primaryKey"`
	FormID    uint   `gorm:"index;not null"`
	Form      *Form  `gorm:"constraint:OnDelete:CASCADE"`
	DataJSON  string `gorm:"type:text;not null"`
	IPHash    string `gorm:"size:128;index"`
	UserAgent string `gorm:"type:text"`
	IsSpam    bool   `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time

	WebhookEvents []WebhookEvent
	EmailEvents   []EmailEvent
}

// WebhookEvent captures delivery attempts for a submission.
type WebhookEvent struct {
	ID             uint        `gorm:"primaryKey"`
	SubmissionID   uint        `gorm:"index;not null"`
	Submission     *Submission `gorm:"constraint:OnDelete:CASCADE"`
	Status         string      `gorm:"size:32;index;not null"`
	AttemptCount   int         `gorm:"not null;default:0"`
	LastAttemptErr string      `gorm:"type:text"`
	NextAttemptAt  *time.Time
	LastAttemptAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewWebhookEvent prepares a pending webhook delivery scheduled for the provided time.
func NewWebhookEvent(submissionID uint, scheduledAt time.Time) *WebhookEvent {
	ts := scheduledAt.UTC()
	return &WebhookEvent{
		SubmissionID:  submissionID,
		Status:        WebhookStatusPending,
		AttemptCount:  0,
		NextAttemptAt: &ts,
	}
}

// EmailEvent captures outbound email forwarding attempts.
type EmailEvent struct {
	ID             uint        `gorm:"primaryKey"`
	SubmissionID   uint        `gorm:"index;not null"`
	Submission     *Submission `gorm:"constraint:OnDelete:CASCADE"`
	Status         string      `gorm:"size:32;index;not null"`
	AttemptCount   int         `gorm:"not null;default:0"`
	LastAttemptErr string      `gorm:"type:text"`
	NextAttemptAt  *time.Time
	LastAttemptAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewEmailEvent prepares a pending email forwarding attempt.
func NewEmailEvent(submissionID uint, scheduledAt time.Time) *EmailEvent {
	ts := scheduledAt.UTC()
	return &EmailEvent{
		SubmissionID:  submissionID,
		Status:        WebhookStatusPending,
		AttemptCount:  0,
		NextAttemptAt: &ts,
	}
}
