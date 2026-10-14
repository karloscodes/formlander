package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetails(t *testing.T) {
	t.Run("lists the fields of a submission as labels and values", func(t *testing.T) {
		raw := `{"use_case":"A waitlist","email":"ada@example.com","topics":["news","beta"],"age":36,"consent":true,"cf-turnstile-response":"token"}`

		got := details(raw)

		assert.Equal(t, []Detail{
			{Label: "Age", Value: "36"},
			{Label: "Consent", Value: "true"},
			{Label: "Email", Value: "ada@example.com"},
			{Label: "Topics", Value: "news, beta"},
			{Label: "Use case", Value: "A waitlist"},
		}, got)
	})

	t.Run("returns nothing for a payload that is not an object", func(t *testing.T) {
		assert.Nil(t, details(`["a","b"]`))
		assert.Nil(t, details(`not json`))
	})
}

func TestFileSize(t *testing.T) {
	assert.Equal(t, "512 bytes", fileSize(512))
	assert.Equal(t, "2 KB", fileSize(2048))
	assert.Equal(t, "1.5 MB", fileSize(3<<19))
}
