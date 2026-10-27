package model

// ViewModel is the state the app's screens read, as the Swift
// MoleViewModel was.
type ViewModel struct {
	SelectedModule Module
	CleanFilter    string
	SearchQuery    string

	IsZH         bool
	IsDarkMode   bool
	DryRunMode   bool
	ReduceMotion bool

	ShowDoctorModal  bool
	ShowConfirmSheet bool
	ToastMessage     string

	AirRetention RetentionWindow
	FanMode      string

	CleanItems     []CleanCategory
	InstalledApps  []InstalledApp
	Processes      []Process
	WhitelistPaths []string
}

// NewViewModel returns the app's view model in its default state: the clean
// module, in Chinese, in dark mode, and in dry-run mode, as the Swift app's
// default state was.
func NewViewModel() *ViewModel {
	return &ViewModel{
		SelectedModule: ModuleClean,
		CleanFilter:    "all",
		IsZH:           true,
		IsDarkMode:     true,
		DryRunMode:     true,
		AirRetention:   Days30,
		FanMode:        "Auto",
	}
}

// TotalSelectedCleanGB is the size of what the user selected in the clean
// list.
func (vm *ViewModel) TotalSelectedCleanGB() float64 {
	var total float64
	for _, item := range vm.CleanItems {
		if item.IsSelected {
			total += item.SizeGB
		}
	}
	return total
}

// TotalSelectedCleanItemsCount is how many items the user selected.
func (vm *ViewModel) TotalSelectedCleanItemsCount() int {
	total := 0
	for _, item := range vm.CleanItems {
		if item.IsSelected {
			total += item.ItemCount
		}
	}
	return total
}

// ToggleCleanSelection flips whether the clean category with id is selected.
// It leaves a protected category alone, which the clean list never lets the
// user select either.
func (vm *ViewModel) ToggleCleanSelection(id string) {
	for i := range vm.CleanItems {
		if vm.CleanItems[i].ID != id {
			continue
		}
		if vm.CleanItems[i].Tier == TierProtectedSystem {
			return
		}
		vm.CleanItems[i].IsSelected = !vm.CleanItems[i].IsSelected
		return
	}
}

// ShowToast shows a message for a moment.
func (vm *ViewModel) ShowToast(msg string) {
	vm.ToastMessage = msg
}

// AddToWhitelist adds path to the whitelist, at most once.
func (vm *ViewModel) AddToWhitelist(path string) {
	for _, p := range vm.WhitelistPaths {
		if p == path {
			return
		}
	}
	vm.WhitelistPaths = append(vm.WhitelistPaths, path)
}
