package simulation

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed data/experiences.json
var experiencesFS embed.FS

//go:embed data/requests.json
var requestsFS embed.FS

//go:embed data/chat_messages.json
var chatMessagesFS embed.FS

// LoadExperienceTemplates loads experience templates from the embedded JSON file.
func LoadExperienceTemplates() ([]ExperienceTemplate, error) {
	data, err := experiencesFS.ReadFile("data/experiences.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded experiences: %w", err)
	}

	var templates []ExperienceTemplate
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, fmt.Errorf("unmarshal experiences: %w", err)
	}

	return templates, nil
}

// LoadRequestTemplates loads request templates from the embedded JSON file.
func LoadRequestTemplates() ([]RequestTemplate, error) {
	data, err := requestsFS.ReadFile("data/requests.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded requests: %w", err)
	}

	var templates []RequestTemplate
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, fmt.Errorf("unmarshal requests: %w", err)
	}

	return templates, nil
}

// LoadChatTemplates loads chat message templates from the embedded JSON file.
func LoadChatTemplates() (*ChatTemplateSet, error) {
	data, err := chatMessagesFS.ReadFile("data/chat_messages.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded chat messages: %w", err)
	}

	var templates ChatTemplateSet
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, fmt.Errorf("unmarshal chat messages: %w", err)
	}

	return &templates, nil
}
