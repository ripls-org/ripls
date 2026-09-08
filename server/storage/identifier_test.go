package storage

import "testing"

func TestValidateIdentifier(t *testing.T) {
	valid := []string{
		"name",
		"owner_id",
		"_private",
		"CamelCase",
		"col1",
		"local_ripls_minilm_v1",
		"gear_gemini_text_embedding_004_hnsw_idx",
	}
	for _, id := range valid {
		if err := validateIdentifier(id); err != nil {
			t.Errorf("validateIdentifier(%q) unexpected error: %v", id, err)
		}
	}

	invalid := []string{
		"1=1; --",
		"user; DROP TABLE user; --",
		"foo\" OR 1=1--",
		"123start",
		"has space",
		"has-hyphen",
		"has.dot",
		"",
	}
	for _, id := range invalid {
		if err := validateIdentifier(id); err == nil {
			t.Errorf("validateIdentifier(%q) expected error, got nil", id)
		}
	}
}
