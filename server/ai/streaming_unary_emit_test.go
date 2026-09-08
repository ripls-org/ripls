package ai

import (
	"encoding/json"
	"testing"
)

// The emit*FromUnary helpers turn a unary provider result into the synthetic
// field-event stream the service layer consumes. The client's create preview
// is filled ONLY from these mid-stream events (the terminal payload's
// title/description are ignored by the reducer), so every user-visible text
// field the unary result carries must be emitted here — dropping one silently
// breaks the Save gate for providers without native streaming (#2687).

// collectFieldKeys runs emit against buffered channels and returns the
// emitted events keyed by field, decoding each value as a JSON string.
func collectFieldKeys(t *testing.T, emit func(fields chan<- FieldEvent)) map[StreamFieldKey]string {
	t.Helper()
	fields := make(chan FieldEvent, 16)
	emit(fields)
	close(fields)
	got := map[StreamFieldKey]string{}
	for ev := range fields {
		var s string
		if err := json.Unmarshal(ev.Value, &s); err != nil {
			// Slice-valued fields (search_keywords) aren't strings; record
			// presence with the raw payload so key assertions still work.
			got[ev.Key] = string(ev.Value)
			continue
		}
		got[ev.Key] = s
	}
	return got
}

func TestEmitExperienceFromUnaryEmitsAllPreviewFields(t *testing.T) {
	result := &ExperienceGeneration{
		Title:          "Sunday Dinner",
		Description:    "Bring the whole crew.",
		LocationQuery:  "our place",
		SearchKeywords: []string{"dinner"},
		Date:           "2026-05-10",
		Time:           "10:30",
		TimeConfidence: "EXPLICIT",
	}
	final := make(chan ExperienceStreamFinal, 1)
	got := collectFieldKeys(t, func(fields chan<- FieldEvent) {
		emitExperienceFromUnary(result, nil, fields, final)
	})

	for key, want := range map[StreamFieldKey]string{
		StreamFieldTitle:          "Sunday Dinner",
		StreamFieldDescription:    "Bring the whole crew.",
		StreamFieldLocationQuery:  "our place",
		StreamFieldDate:           "2026-05-10",
		StreamFieldTime:           "10:30",
		StreamFieldTimeConfidence: "EXPLICIT",
	} {
		if got[key] != want {
			t.Errorf("field %q = %q, want %q", key, got[key], want)
		}
	}
	if _, ok := got[StreamFieldSearchKeywords]; !ok {
		t.Errorf("search_keywords not emitted")
	}
	if f := <-final; f.Result != result {
		t.Errorf("final does not carry the unary result")
	}
}

func TestEmitRequestFromUnaryEmitsTitleAndDescription(t *testing.T) {
	result := &RequestGeneration{
		Title:       "Need a ladder",
		Description: "Two hours on Saturday.",
	}
	final := make(chan RequestStreamFinal, 1)
	got := collectFieldKeys(t, func(fields chan<- FieldEvent) {
		emitRequestFromUnary(result, nil, fields, final)
	})
	if got[StreamFieldTitle] != "Need a ladder" {
		t.Errorf("title = %q", got[StreamFieldTitle])
	}
	if got[StreamFieldDescription] != "Two hours on Saturday." {
		t.Errorf("description = %q", got[StreamFieldDescription])
	}
	if f := <-final; f.Result != result {
		t.Errorf("final does not carry the unary result")
	}
}

func TestEmitGearFromUnaryEmitsTitleAndDescription(t *testing.T) {
	result := &GearGeneration{
		Title:       "Cordless Drill",
		Description: "Barely used, comes with two batteries.",
	}
	final := make(chan GearStreamFinal, 1)
	got := collectFieldKeys(t, func(fields chan<- FieldEvent) {
		emitGearFromUnary(result, nil, fields, final)
	})
	if got[StreamFieldTitle] != "Cordless Drill" {
		t.Errorf("title = %q", got[StreamFieldTitle])
	}
	if got[StreamFieldDescription] != "Barely used, comes with two batteries." {
		t.Errorf("description = %q", got[StreamFieldDescription])
	}
	if f := <-final; f.Result != result {
		t.Errorf("final does not carry the unary result")
	}
}

func TestEmitGearDetectionFromUnaryEmitsTitleAndDescription(t *testing.T) {
	result := &GearDetection{
		Title:       "Spaghetti",
		Description: "Unopened box of dried spaghetti.",
	}
	final := make(chan GearDetectionStreamFinal, 1)
	got := collectFieldKeys(t, func(fields chan<- FieldEvent) {
		emitGearDetectionFromUnary(result, nil, fields, final)
	})
	if got[StreamFieldTitle] != "Spaghetti" {
		t.Errorf("title = %q", got[StreamFieldTitle])
	}
	if got[StreamFieldDescription] != "Unopened box of dried spaghetti." {
		t.Errorf("description = %q", got[StreamFieldDescription])
	}
	if f := <-final; f.Result != result {
		t.Errorf("final does not carry the unary result")
	}
}

// Empty fields must be skipped, not emitted as empty events — the client
// treats any arriving description as authoritative until user-edited.
func TestEmitFromUnarySkipsEmptyFields(t *testing.T) {
	final := make(chan GearDetectionStreamFinal, 1)
	got := collectFieldKeys(t, func(fields chan<- FieldEvent) {
		emitGearDetectionFromUnary(&GearDetection{Title: "Only Title"}, nil, fields, final)
	})
	if _, ok := got[StreamFieldDescription]; ok {
		t.Errorf("empty description should not be emitted")
	}
	<-final
}
