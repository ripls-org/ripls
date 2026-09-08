package firebase

import (
	"context"
	"testing"
)

func TestNewApp_WithProjectID(t *testing.T) {
	app, err := NewApp(context.Background(), "test-project")
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	if app == nil {
		t.Fatal("NewApp returned nil app")
	}
}

func TestNewApp_NoProjectID(t *testing.T) {
	app, err := NewApp(context.Background(), "")
	if err != nil {
		t.Fatalf("NewApp with empty project ID returned error: %v", err)
	}
	if app == nil {
		t.Fatal("NewApp returned nil app")
	}
}
