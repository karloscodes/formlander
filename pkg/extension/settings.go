// Package formlander provides extension points for settings
package extension

import (
	"html/template"
	"sync"
	
	"github.com/gofiber/fiber/v2"
)

// SettingsItem represents a configuration item in the settings page
type SettingsItem struct {
	Title       string
	Description string
	URL         string
	Icon        template.HTML
	Color       string
}

var (
	settingsItems []SettingsItem
	settingsMutex sync.RWMutex
	settingsDataProvider func() fiber.Map
)

// RegisterSettingsItem adds a settings configuration item
func RegisterSettingsItem(item SettingsItem) {
	settingsMutex.Lock()
	defer settingsMutex.Unlock()
	settingsItems = append(settingsItems, item)
}

// GetSettingsItems returns all registered settings items
func GetSettingsItems() []SettingsItem {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	items := make([]SettingsItem, len(settingsItems))
	copy(items, settingsItems)
	return items
}

// SetSettingsDataProvider allows pro to customize settings page data
func SetSettingsDataProvider(provider func() fiber.Map) {
	settingsMutex.Lock()
	defer settingsMutex.Unlock()
	settingsDataProvider = provider
}

// GetSettingsData returns settings data (calls provider if set)
func GetSettingsData() fiber.Map {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	if settingsDataProvider != nil {
		return settingsDataProvider()
	}
	return fiber.Map{
		"ProSettingsItems": GetSettingsItems(),
	}
}
