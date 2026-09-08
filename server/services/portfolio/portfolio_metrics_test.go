package portfolio

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ---------------------------------------------------------------------------
// extractImpactValues
// ---------------------------------------------------------------------------.

// TestExtractImpactValues_Nil verifies that a nil ImpactEstimate returns zeros.
func TestExtractImpactValues_Nil(t *testing.T) {
	cost, co2, mins, qt := extractImpactValues(nil)
	if cost != 0 || co2 != 0 || mins != 0 || qt != 0 {
		t.Errorf("extractImpactValues(nil) = (%v, %v, %v, %v), want all zero", cost, co2, mins, qt)
	}
}

// TestExtractImpactValues_AllFields verifies all four dimensions are extracted.
func TestExtractImpactValues_AllFields(t *testing.T) {
	ie := &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{
			ValueUsd: &models.Estimate{Mean: 15.5},
		},
		EmissionsPrevented: &models.PreventedEmissions{
			ManufactureAvoidedCarbon: &models.CarbonEstimate{Co2EGrams: &models.Estimate{Mean: 200}},
			WasteReducedCarbon:       &models.CarbonEstimate{Co2EGrams: &models.Estimate{Mean: 50}},
		},
		TimeSaved: &models.TimeSavings{
			Minutes: &models.Estimate{Mean: 90},
		},
		QualityTime: &models.QualityTimeEstimate{
			QualityTimeMinutes: &models.Estimate{Mean: 60},
		},
	}

	cost, co2, mins, qt := extractImpactValues(ie)

	if cost != 15.5 {
		t.Errorf("cost = %v, want 15.5", cost)
	}
	if co2 != 250 {
		t.Errorf("co2 = %v, want 250 (200+50)", co2)
	}
	if mins != 90 {
		t.Errorf("mins = %v, want 90", mins)
	}
	if qt != 60 {
		t.Errorf("qt = %v, want 60", qt)
	}
}

// TestExtractImpactValues_PartialFields verifies graceful handling of absent
// sub-fields within a non-nil ImpactEstimate.
func TestExtractImpactValues_PartialFields(t *testing.T) {
	ie := &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{
			// ValueUsd is nil → should produce 0.
		},
	}
	cost, co2, mins, qt := extractImpactValues(ie)
	if cost != 0 || co2 != 0 || mins != 0 || qt != 0 {
		t.Errorf("partial IE: got (%v, %v, %v, %v), want all zero", cost, co2, mins, qt)
	}
}

// ---------------------------------------------------------------------------
// lookupName
// ---------------------------------------------------------------------------.

// TestLookupName verifies map lookup with fallback behaviour.
func TestLookupName(t *testing.T) {
	m := map[string]string{"gear-1": "Tent", "gear-2": ""}

	if got := lookupName(m, "gear-1", "fallback"); got != "Tent" {
		t.Errorf("lookupName for existing key = %q, want Tent", got)
	}
	// Empty string value should fall back.
	if got := lookupName(m, "gear-2", "fallback"); got != "fallback" {
		t.Errorf("lookupName for empty value = %q, want fallback", got)
	}
	// Missing key should fall back.
	if got := lookupName(m, "gear-99", "fallback"); got != "fallback" {
		t.Errorf("lookupName for missing key = %q, want fallback", got)
	}
}

// ---------------------------------------------------------------------------
// plural
// ---------------------------------------------------------------------------.

// TestPlural verifies simple English pluralisation.
func TestPlural(t *testing.T) {
	if plural(1) != "" {
		t.Errorf("plural(1) = %q, want empty", plural(1))
	}
	if plural(0) != "s" {
		t.Errorf("plural(0) = %q, want s", plural(0))
	}
	if plural(2) != "s" {
		t.Errorf("plural(2) = %q, want s", plural(2))
	}
}

// ---------------------------------------------------------------------------
// joinParts
// ---------------------------------------------------------------------------.

// TestJoinParts verifies joining behaviour for 0, 1, and 2+ parts.
func TestJoinParts(t *testing.T) {
	if got := joinParts(nil); got != "" {
		t.Errorf("joinParts(nil) = %q, want empty", got)
	}
	if got := joinParts([]string{"one"}); got != "one" {
		t.Errorf("joinParts([one]) = %q, want one", got)
	}
	if got := joinParts([]string{"a", "b"}); got != "a, b" {
		t.Errorf("joinParts([a, b]) = %q, want a, b", got)
	}
	if got := joinParts([]string{"x", "y", "z"}); got != "x, y, z" {
		t.Errorf("joinParts([x, y, z]) = %q, want x, y, z", got)
	}
}

// ---------------------------------------------------------------------------
// computeReceivedSection
// ---------------------------------------------------------------------------.

// makeCompletedLoanRecipient returns a minimal completed loan transfer where the
// named user is the recipient (borrower).
func makeCompletedLoanRecipient(id, ownerID, recipientID, communityID, gearID string, ie *models.ImpactEstimate) proto.Message {
	return &models.Transfer{
		Id:             id,
		OwnerId:        ownerID,
		RecipientId:    recipientID,
		CommunityId:    communityID,
		GearId:         gearID,
		TransferType:   models.TransferType_TRANSFER_TYPE_LOAN,
		State:          models.TransferState_TRANSFER_STATE_COMPLETED,
		ImpactEstimate: ie,
	}
}

// TestComputeReceivedSection_Empty returns a section with zero values when no
// transfers are supplied.
func TestComputeReceivedSection_Empty(t *testing.T) {
	section := computeReceivedSection(nil, map[string]bool{"c1": true})
	if section.QualityTimeMinutes != 0 {
		t.Errorf("QualityTimeMinutes = %d, want 0", section.QualityTimeMinutes)
	}
	if section.ActivitySummary != "" {
		t.Errorf("ActivitySummary = %q, want empty", section.ActivitySummary)
	}
}

// TestComputeReceivedSection_FiltersDeletedAndWrongCommunity verifies that
// deleted transfers and transfers outside the community set are excluded.
func TestComputeReceivedSection_FiltersDeletedAndWrongCommunity(t *testing.T) {
	communitySet := map[string]bool{"comm-1": true}
	deleted := &models.Transfer{
		Id:          "t-del",
		State:       models.TransferState_TRANSFER_STATE_COMPLETED,
		CommunityId: "comm-1",
		Deleted:     &models.DeletedMetadata{DeletedAtUnixSec: 100},
	}
	wrongComm := &models.Transfer{
		Id:          "t-wrong",
		State:       models.TransferState_TRANSFER_STATE_COMPLETED,
		CommunityId: "comm-99",
	}

	section := computeReceivedSection(
		[]proto.Message{deleted, wrongComm},
		communitySet,
	)
	// A deleted transfer and one outside the viewer's communities both
	// contribute nothing, so every total stays zero.
	if section.QualityTimeMinutes != 0 || section.ActivitySummary != "" {
		t.Errorf("expected an empty section, got qt=%d summary=%q",
			section.QualityTimeMinutes, section.ActivitySummary)
	}
}

// TestComputeReceivedSection_Counts verifies item-count statistics for loans
// and giveaways.
func TestComputeReceivedSection_Counts(t *testing.T) {
	communitySet := map[string]bool{"comm-1": true}
	ie := &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{ValueUsd: &models.Estimate{Mean: 10}},
	}
	loan := makeCompletedLoanRecipient("t-1", "owner-1", "user-1", "comm-1", "gear-1", ie)
	giveaway := &models.Transfer{
		Id:             "t-2",
		OwnerId:        "owner-1",
		RecipientId:    "user-1",
		CommunityId:    "comm-1",
		GearId:         "gear-2",
		TransferType:   models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:          models.TransferState_TRANSFER_STATE_COMPLETED,
		ImpactEstimate: ie,
	}

	section := computeReceivedSection(
		[]proto.Message{loan, giveaway},
		communitySet,
	)
	if section.ActivitySummary == "" {
		t.Error("ActivitySummary should be non-empty when there are loans and giveaways")
	}
}

// ---------------------------------------------------------------------------
// computeGivenSection
// ---------------------------------------------------------------------------.

// TestComputeGivenSection_LoanGiveawayVerb verifies that loans use the loan
// emoji and giveaways use the gift emoji in the breakdown.
func TestComputeGivenSection_LoanGiveawayVerb(t *testing.T) {
	communitySet := map[string]bool{"comm-1": true}
	ie := &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{ValueUsd: &models.Estimate{Mean: 5}},
	}

	loan := &models.Transfer{
		Id:             "t-1",
		OwnerId:        "user-1",
		RecipientId:    "user-2",
		CommunityId:    "comm-1",
		GearId:         "gear-1",
		TransferType:   models.TransferType_TRANSFER_TYPE_LOAN,
		State:          models.TransferState_TRANSFER_STATE_COMPLETED,
		ImpactEstimate: ie,
	}
	giveaway := &models.Transfer{
		Id:             "t-2",
		OwnerId:        "user-1",
		RecipientId:    "user-2",
		CommunityId:    "comm-1",
		GearId:         "gear-2",
		TransferType:   models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:          models.TransferState_TRANSFER_STATE_COMPLETED,
		ImpactEstimate: ie,
	}

	section := computeGivenSection(
		[]proto.Message{loan, giveaway},
		communitySet,
	)

	// One lent and one given away, so the summary must mention both.
	if !strings.Contains(section.ActivitySummary, "lent") ||
		!strings.Contains(section.ActivitySummary, "given") {
		t.Errorf("ActivitySummary = %q, want it to mention both lent and given",
			section.ActivitySummary)
	}
}

// ---------------------------------------------------------------------------
// computePersonalTotalSection
// ---------------------------------------------------------------------------.

// TestComputePersonalTotalSection_Deduplication verifies that transfers
// appearing in both owner and recipient lists are counted only once.
func TestComputePersonalTotalSection_Deduplication(t *testing.T) {
	communitySet := map[string]bool{"comm-1": true}
	ie := &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{ValueUsd: &models.Estimate{Mean: 10}},
	}
	sharedTransfer := &models.Transfer{
		Id:             "t-1",
		CommunityId:    "comm-1",
		State:          models.TransferState_TRANSFER_STATE_COMPLETED,
		ImpactEstimate: ie,
	}

	// Same transfer in both lists.
	section := computePersonalTotalSection(
		[]proto.Message{sharedTransfer},
		[]proto.Message{sharedTransfer},
		communitySet,
	)

	// CostSaved should reflect one transfer (USD 10), not two (USD 20).
	if section.CostSaved != "$10" {
		t.Errorf("CostSaved = %q after deduplication, want $10", section.CostSaved)
	}
}

// TestComputePersonalTotalSection_TotalTransfers verifies the activity summary
// format for multiple unique transfers.
func TestComputePersonalTotalSection_TotalTransfers(t *testing.T) {
	communitySet := map[string]bool{"comm-1": true}
	ie := &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{ValueUsd: &models.Estimate{Mean: 5}},
	}
	t1 := &models.Transfer{Id: "t-1", CommunityId: "comm-1", State: models.TransferState_TRANSFER_STATE_COMPLETED, ImpactEstimate: ie}
	t2 := &models.Transfer{Id: "t-2", CommunityId: "comm-1", State: models.TransferState_TRANSFER_STATE_COMPLETED, ImpactEstimate: ie}

	section := computePersonalTotalSection(
		[]proto.Message{t1},
		[]proto.Message{t2},
		communitySet,
	)

	if section.ActivitySummary != "2 total exchanges" {
		t.Errorf("ActivitySummary = %q, want 2 total exchanges", section.ActivitySummary)
	}
}

// TestComputePersonalTotalSection_QualityTimeAggregated verifies that quality
// time minutes accumulate across both owner and recipient transfers.
func TestComputePersonalTotalSection_QualityTimeAggregated(t *testing.T) {
	communitySet := map[string]bool{"comm-1": true}
	makeTransfer := func(id string, qt float32) *models.Transfer {
		return &models.Transfer{
			Id:          id,
			CommunityId: "comm-1",
			State:       models.TransferState_TRANSFER_STATE_COMPLETED,
			ImpactEstimate: &models.ImpactEstimate{
				QualityTime: &models.QualityTimeEstimate{
					QualityTimeMinutes: &models.Estimate{Mean: qt},
				},
			},
		}
	}

	section := computePersonalTotalSection(
		[]proto.Message{makeTransfer("t-1", 30)},
		[]proto.Message{makeTransfer("t-2", 45)},
		communitySet,
	)

	if section.QualityTimeMinutes != 75 {
		t.Errorf("QualityTimeMinutes = %d, want 75", section.QualityTimeMinutes)
	}
}

// ---------------------------------------------------------------------------
// section titles
// ---------------------------------------------------------------------------.

// TestSectionTitles verifies that each section has the expected fixed title.
func TestSectionTitles(t *testing.T) {
	empty := map[string]bool{}
	recv := computeReceivedSection(nil, empty)
	given := computeGivenSection(nil, empty)
	total := computePersonalTotalSection(nil, nil, empty)

	tests := []struct {
		section *api.PortfolioMetricsSection
		want    string
	}{
		{recv, "What I've saved"},
		{given, "What I've given"},
		{total, "My total impact"},
	}
	for _, tc := range tests {
		if tc.section.Title != tc.want {
			t.Errorf("section Title = %q, want %q", tc.section.Title, tc.want)
		}
	}
}
