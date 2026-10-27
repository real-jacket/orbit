package model

import "testing"

// Guards the invariants the safety model depends on. A protected tier that
// became selectable would let the UI delete hardware-level caches.
func TestProtectedTierIsDistinctFromOtherTiers(t *testing.T) {
	if TierProtectedSystem == TierSafeRegenerable {
		t.Fatal("protected tier equals safe tier")
	}
	if TierProtectedSystem == TierReviewOnly {
		t.Fatal("protected tier equals review tier")
	}
}

func TestEveryTierHasBothLocalizations(t *testing.T) {
	for _, tier := range AllTiers() {
		if tier.Label(true) == "" {
			t.Errorf("%s missing zh label", tier)
		}
		if tier.Label(false) == "" {
			t.Errorf("%s missing en label", tier)
		}
	}
}

func TestEveryModuleHasBothLocalizationsAndABadge(t *testing.T) {
	for _, module := range AllModules() {
		if module.SidebarTitle(true) == "" {
			t.Errorf("%s missing zh sidebar title", module)
		}
		if module.SidebarTitle(false) == "" {
			t.Errorf("%s missing en sidebar title", module)
		}
		if module.HeaderTitle(true) == "" {
			t.Errorf("%s missing zh header", module)
		}
		if module.HeaderTitle(false) == "" {
			t.Errorf("%s missing en header", module)
		}
		if module.SidebarBadge() == "" {
			t.Errorf("%s missing badge", module)
		}
		if module.CLIBadgeText() == "" {
			t.Errorf("%s missing CLI badge", module)
		}
	}
}

func TestModuleIdentifiersAreUnique(t *testing.T) {
	seen := map[Module]bool{}
	for _, m := range AllModules() {
		if seen[m] {
			t.Fatalf("duplicate module id %s", m)
		}
		seen[m] = true
	}
}

func TestSafetyTierRawValuesAreStable(t *testing.T) {
	// These strings are persisted in scan payloads; changing them breaks
	// compatibility with the frontend.
	if string(TierSafeRegenerable) != "safe" {
		t.Errorf("safe tier raw value is %q", TierSafeRegenerable)
	}
	if string(TierReviewOnly) != "review" {
		t.Errorf("review tier raw value is %q", TierReviewOnly)
	}
	if string(TierProtectedSystem) != "protected" {
		t.Errorf("protected tier raw value is %q", TierProtectedSystem)
	}
}

func TestProtectedCleanItemsAreNotSelectable(t *testing.T) {
	vm := NewViewModel()
	vm.CleanItems = []CleanCategory{{
		ID:     "protected",
		Tier:   TierProtectedSystem,
		SizeGB: 1,
	}}
	vm.ToggleCleanSelection("protected")
	if vm.CleanItems[0].IsSelected {
		t.Fatal("a protected row became selected")
	}
}

func TestDryRunIsOnByDefault(t *testing.T) {
	if !NewViewModel().DryRunMode {
		t.Fatal("dry-run must default to true")
	}
}

func TestTogglingAnUnprotectedItemFlipsSelection(t *testing.T) {
	vm := NewViewModel()
	vm.CleanItems = []CleanCategory{{
		ID:         "cache",
		Tier:       TierSafeRegenerable,
		SizeGB:     1,
		IsSelected: true,
	}}
	before := vm.CleanItems[0].IsSelected
	vm.ToggleCleanSelection("cache")
	if vm.CleanItems[0].IsSelected == before {
		t.Fatalf("selection stayed %v", before)
	}
}

func TestSelectionTotalsOnlyCountSelectedRows(t *testing.T) {
	vm := NewViewModel()
	vm.CleanItems = []CleanCategory{
		{ID: "a", SizeGB: 1.5, ItemCount: 2, IsSelected: true},
		{ID: "b", SizeGB: 2.5, ItemCount: 3, IsSelected: false},
		{ID: "c", SizeGB: 4, ItemCount: 5, IsSelected: true},
	}
	if got, want := vm.TotalSelectedCleanGB(), 5.5; got != want {
		t.Errorf("total GB = %v, want %v", got, want)
	}
	if got, want := vm.TotalSelectedCleanItemsCount(), 7; got != want {
		t.Errorf("total items = %d, want %d", got, want)
	}
}
