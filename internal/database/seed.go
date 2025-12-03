package database

import (
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"formlander/internal/accounts"
	"formlander/internal/forms"
	"formlander/internal/integrations"
)

// Seed populates the database with sample data for development/testing.
func Seed(db *gorm.DB) error {
	// Create admin user if not exists
	var userCount int64
	if err := db.Model(&accounts.User{}).Count(&userCount).Error; err != nil {
		return fmt.Errorf("count users: %w", err)
	}

	if userCount == 0 {
		// Create default admin for seeding (no password change required for dev)
		hash, err := bcrypt.GenerateFromPassword([]byte("formlander"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}

		now := time.Now()
		admin := &accounts.User{
			Email:        "admin@formlander.local",
			PasswordHash: string(hash),
			LastLoginAt:  &now, // Set to now = not first login, no password change required (for dev)
		}

		if err := db.Create(admin).Error; err != nil {
			return fmt.Errorf("create admin user: %w", err)
		}
		fmt.Println("✓ Created admin user: admin@formlander.local / formlander")
	} else {
		fmt.Println("✓ Admin user already exists")
	}

	// Create default settings if not exists
	var settingsCount int64
	if err := db.Model(&accounts.Settings{}).Count(&settingsCount).Error; err != nil {
		return fmt.Errorf("count settings: %w", err)
	}

	if settingsCount == 0 {
		settings := &accounts.Settings{}

		if err := db.Create(settings).Error; err != nil {
			return fmt.Errorf("create settings: %w", err)
		}
		fmt.Println("✓ Created default settings")
	}

	// Create default mailer profile if not exists
	var mailerCount int64
	if err := db.Model(&integrations.MailerProfile{}).Count(&mailerCount).Error; err != nil {
		return fmt.Errorf("count mailer profiles: %w", err)
	}

	if mailerCount == 0 {
		mailer := &integrations.MailerProfile{
			Name:             "default",
			Provider:         "mailgun",
			DefaultFromName:  "Formlander",
			DefaultFromEmail: "no-reply@example.com",
		}

		if err := db.Create(mailer).Error; err != nil {
			return fmt.Errorf("create default mailer profile: %w", err)
		}
		fmt.Println("✓ Created default mailer profile")
	}

	// Create default captcha profile if not exists
	var captchaCount int64
	if err := db.Model(&integrations.CaptchaProfile{}).Count(&captchaCount).Error; err != nil {
		return fmt.Errorf("count captcha profiles: %w", err)
	}

	if captchaCount == 0 {
		siteKeys := []map[string]string{
			{"host_pattern": "*", "site_key": ""},
		}
		siteKeysJSON, _ := json.Marshal(siteKeys)

		policy := map[string]interface{}{
			"required": false,
			"action":   "submit",
			"widget":   "managed",
		}
		policyJSON, _ := json.Marshal(policy)

		captcha := &integrations.CaptchaProfile{
			Name:         "default",
			Provider:     "turnstile",
			SiteKeysJSON: string(siteKeysJSON),
			PolicyJSON:   string(policyJSON),
		}

		if err := db.Create(captcha).Error; err != nil {
			return fmt.Errorf("create default captcha profile: %w", err)
		}
		fmt.Println("✓ Created default captcha profile")
	}

	// Check if we already have forms
	var formCount int64
	if err := db.Model(&forms.Form{}).Count(&formCount).Error; err != nil {
		return fmt.Errorf("count forms: %w", err)
	}

	if formCount > 0 {
		fmt.Println("✓ Sample forms already exist")
		return nil
	}

	// Create sample forms
	contactForm := &forms.Form{
		Name: "Contact Form",
		Slug: "contact",
	}

	if err := db.Create(contactForm).Error; err != nil {
		return fmt.Errorf("create contact form: %w", err)
	}
	fmt.Println("✓ Created contact form")

	newsletterForm := &forms.Form{
		Name: "Newsletter Signup",
		Slug: "newsletter",
	}

	if err := db.Create(newsletterForm).Error; err != nil {
		return fmt.Errorf("create newsletter form: %w", err)
	}
	fmt.Println("✓ Created newsletter form")

	feedbackForm := &forms.Form{
		Name: "Feedback Form",
		Slug: "feedback",
	}

	if err := db.Create(feedbackForm).Error; err != nil {
		return fmt.Errorf("create feedback form: %w", err)
	}
	fmt.Println("✓ Created feedback form")

	// Create sample submissions for contact form
	sampleSubmissions := []map[string]interface{}{
		{
			"name":    "Alice Johnson",
			"email":   "alice@example.com",
			"message": "I love using Formlander! It's so easy to set up.",
		},
		{
			"name":    "Bob Smith",
			"email":   "bob@example.com",
			"message": "Quick question about webhook configuration. Can I use multiple URLs?",
		},
		{
			"name":    "Charlie Brown",
			"email":   "charlie@example.com",
			"message": "Just wanted to say thanks for creating this tool!",
		},
	}

	for i, data := range sampleSubmissions {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal submission %d: %w", i, err)
		}

		submission := &forms.Submission{
			FormID:    contactForm.ID,
			DataJSON:  string(jsonData),
			IPHash:    fmt.Sprintf("hash_%d", i),
			UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
			IsSpam:    false,
			CreatedAt: time.Now().Add(-time.Duration(i*24) * time.Hour),
		}

		if err := db.Create(submission).Error; err != nil {
			return fmt.Errorf("create submission %d: %w", i, err)
		}
	}
	fmt.Printf("✓ Created %d sample submissions\n", len(sampleSubmissions))

	// Create sample submissions for newsletter
	newsletterSubmissions := []map[string]interface{}{
		{"email": "subscriber1@example.com"},
		{"email": "subscriber2@example.com"},
	}

	for i, data := range newsletterSubmissions {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal newsletter submission %d: %w", i, err)
		}

		submission := &forms.Submission{
			FormID:    newsletterForm.ID,
			DataJSON:  string(jsonData),
			IPHash:    fmt.Sprintf("hash_news_%d", i),
			UserAgent: "Mozilla/5.0",
			IsSpam:    false,
			CreatedAt: time.Now().Add(-time.Duration(i*12) * time.Hour),
		}

		if err := db.Create(submission).Error; err != nil {
			return fmt.Errorf("create newsletter submission %d: %w", i, err)
		}
	}
	fmt.Printf("✓ Created %d newsletter submissions\n", len(newsletterSubmissions))

	fmt.Println("\n✅ Database seeded successfully!")
	return nil
}
