// Package model holds the display models of the app: the modules of the
// sidebar, the safety tiers of the clean list, and the app's own view model.
package model

// Module is a module of the sidebar.
type Module string

// The modules of the sidebar, in the order the sidebar shows them.
const (
	ModuleClean    Module = "clean"
	ModuleAIMoon   Module = "ai_moon"
	ModuleSoftware Module = "software"
	ModuleOptimize Module = "optimize"
	ModuleAnalyze  Module = "analyze"
	ModuleStatus   Module = "status"
	ModulePurge    Module = "purge"
	ModuleHistory  Module = "history"
)

// AllModules lists every module in sidebar order.
func AllModules() []Module {
	return []Module{
		ModuleClean, ModuleAIMoon, ModuleSoftware, ModuleOptimize,
		ModuleAnalyze, ModuleStatus, ModulePurge, ModuleHistory,
	}
}

// SidebarTitle is the module's title in the sidebar.
func (m Module) SidebarTitle(isZH bool) string {
	if isZH {
		switch m {
		case ModuleClean:
			return "智能清理"
		case ModuleAIMoon:
			return "AI 月球基地"
		case ModuleSoftware:
			return "软件卸载"
		case ModuleOptimize:
			return "系统优化"
		case ModuleAnalyze:
			return "磁盘分析"
		case ModuleStatus:
			return "系统状态"
		case ModulePurge:
			return "深度清理"
		case ModuleHistory:
			return "操作历史"
		}
		return string(m)
	}
	switch m {
	case ModuleClean:
		return "Smart Clean"
	case ModuleAIMoon:
		return "AI Moon Base"
	case ModuleSoftware:
		return "App Uninstall"
	case ModuleOptimize:
		return "System Tune"
	case ModuleAnalyze:
		return "Disk Analyzer"
	case ModuleStatus:
		return "System Status"
	case ModulePurge:
		return "Deep Purge"
	case ModuleHistory:
		return "History"
	}
	return string(m)
}

// HeaderTitle is the module's title in the page header.
func (m Module) HeaderTitle(isZH bool) string {
	if isZH {
		switch m {
		case ModuleClean:
			return "智能清理 · 安全可再生空间"
		case ModuleAIMoon:
			return "AI 月球基地 · 可重建的模型与缓存"
		case ModuleSoftware:
			return "软件卸载 · 应用与关联残留"
		case ModuleOptimize:
			return "系统优化 · 可回滚的维护任务"
		case ModuleAnalyze:
			return "磁盘分析 · 目录体积树图"
		case ModuleStatus:
			return "系统状态 · 实时遥测"
		case ModulePurge:
			return "深度清理 · 安装包与构建产物"
		case ModuleHistory:
			return "操作历史 · 快照与白名单"
		}
		return string(m)
	}
	switch m {
	case ModuleClean:
		return "Smart Clean · Safely regenerable space"
	case ModuleAIMoon:
		return "AI Moon Base · Rebuildable models and caches"
	case ModuleSoftware:
		return "App Uninstall · Apps and their leftovers"
	case ModuleOptimize:
		return "System Tune · Reversible maintenance tasks"
	case ModuleAnalyze:
		return "Disk Analyzer · Directory size treemap"
	case ModuleStatus:
		return "System Status · Live telemetry"
	case ModulePurge:
		return "Deep Purge · Installers and build artifacts"
	case ModuleHistory:
		return "History · Snapshots and whitelist"
	}
	return string(m)
}

// SidebarBadge is the small number the sidebar shows next to the title.
func (m Module) SidebarBadge() string {
	switch m {
	case ModuleClean:
		return "12.5 GB"
	case ModuleAIMoon:
		return "12.2 GB"
	case ModuleSoftware:
		return "12.8 GB"
	case ModuleOptimize:
		return "7 项"
	case ModuleAnalyze:
		return "2D 树图"
	case ModuleStatus:
		return "实时"
	case ModulePurge:
		return "24.9 GB"
	case ModuleHistory:
		return "3 快照"
	}
	return ""
}

// CLIBadgeText is the command line the module stands for.
func (m Module) CLIBadgeText() string {
	switch m {
	case ModuleClean:
		return "$ mo clean --dry-run"
	case ModuleAIMoon:
		return "$ mo clean --ai-care --retention 30d"
	case ModuleSoftware:
		return "$ mo uninstall --dry-run"
	case ModuleOptimize:
		return "$ mo optimize"
	case ModuleAnalyze:
		return "$ mo analyze --json ~"
	case ModuleStatus:
		return "$ mo status --json --watch"
	case ModulePurge:
		return "$ mo purge && mo installer"
	case ModuleHistory:
		return "$ mo whitelist"
	}
	return ""
}

// DotColor is the color of the module's status dot.
func (m Module) DotColor() string {
	switch m {
	case ModuleClean, ModuleAIMoon:
		return "#34d399"
	case ModuleSoftware:
		return "#60a5fa"
	case ModuleOptimize:
		return "#f59e0b"
	case ModuleAnalyze:
		return "#a78bfa"
	case ModuleStatus:
		return "#22d3ee"
	case ModulePurge:
		return "#fb7185"
	case ModuleHistory:
		return "#94a3b8"
	}
	return "#94a3b8"
}

// PrimaryColor is the module's accent color.
func (m Module) PrimaryColor() string {
	switch m {
	case ModuleClean:
		return "#34d399"
	case ModuleAIMoon:
		return "#818cf8"
	case ModuleSoftware:
		return "#60a5fa"
	case ModuleOptimize:
		return "#f59e0b"
	case ModuleAnalyze:
		return "#a78bfa"
	case ModuleStatus:
		return "#22d3ee"
	case ModulePurge:
		return "#fb7185"
	case ModuleHistory:
		return "#94a3b8"
	}
	return "#94a3b8"
}

// HasPlanetaryRing reports whether the module's icon draws a planet with a
// ring.
func (m Module) HasPlanetaryRing() bool {
	return m == ModuleClean || m == ModuleAIMoon
}

// Tier grades how safe it is to remove what the clean list shows.
type Tier string

// The safety tiers, from safest to most protected.
const (
	TierSafeRegenerable Tier = "safe"
	TierReviewOnly      Tier = "review"
	TierProtectedSystem Tier = "protected"
)

// AllTiers lists every tier.
func AllTiers() []Tier {
	return []Tier{TierSafeRegenerable, TierReviewOnly, TierProtectedSystem}
}

// Label is the tier's name in the user's language.
func (t Tier) Label(isZH bool) string {
	if isZH {
		switch t {
		case TierSafeRegenerable:
			return "安全可再生"
		case TierReviewOnly:
			return "需确认"
		case TierProtectedSystem:
			return "硬件保护"
		}
		return string(t)
	}
	switch t {
	case TierSafeRegenerable:
		return "Safe to remove"
	case TierReviewOnly:
		return "Review first"
	case TierProtectedSystem:
		return "Protected"
	}
	return string(t)
}

// Subpath is one directory of a clean category.
type Subpath struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	SizeText   string `json:"sizeText"`
	IsSelected bool   `json:"isSelected"`
}

// CleanCategory is one row of the clean list.
type CleanCategory struct {
	ID               string    `json:"id"`
	Category         string    `json:"category"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	SizeGB           float64   `json:"sizeGB"`
	ItemCount        int       `json:"itemCount"`
	FilesCountText   string    `json:"filesCountText"`
	LastModifiedText string    `json:"lastModifiedText"`
	RebuildTimeText  string    `json:"rebuildTimeText"`
	CLICommand       string    `json:"cliCommand"`
	Impact           string    `json:"impactDescription"`
	Tier             Tier      `json:"tier"`
	IsSelected       bool      `json:"isSelected"`
	Subpaths         []Subpath `json:"subpaths"`
}

// RetentionWindow is one retention option of the AI moon base module.
type RetentionWindow int

// The retention options the AI moon base module offers.
const (
	Days15 RetentionWindow = 15
	Days30 RetentionWindow = 30
	Days90 RetentionWindow = 90
)

// Label is the retention window in the user's language.
func (r RetentionWindow) Label(isZH bool) string {
	days := int(r)
	if isZH {
		return "保留 " + itoa(days) + " 天"
	}
	return "Keep " + itoa(days) + " days"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// AIMoonItem is one band of the AI moon base module.
type AIMoonItem struct {
	CleanCategory
	BandTitle string `json:"bandTitle"`
}

// InstalledApp is an app the uninstall module lists.
type InstalledApp struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	AppSizeGB      float64   `json:"appSizeGB"`
	LeftoverSizeGB float64   `json:"leftoverSizeGB"`
	KeepPersonal   bool      `json:"keepPersonalData"`
	Subpaths       []Subpath `json:"subpaths"`
}

// TotalSizeGB is the app plus what it left behind.
func (a InstalledApp) TotalSizeGB() float64 {
	return a.AppSizeGB + a.LeftoverSizeGB
}

// TaskStatus is what happened to one maintenance task.
type TaskStatus string

// The outcomes of a maintenance task.
const (
	TaskApplied TaskStatus = "applied"
	TaskSkipped TaskStatus = "skipped"
)

// OptimizeTask is one row of the system tune list.
type OptimizeTask struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
}

// Process is one row of the system status process list.
type Process struct {
	PID      int     `json:"pid"`
	Name     string  `json:"name"`
	CPU      float64 `json:"cpu"`
	MemoryMB int     `json:"memory_mb"`
	Pinned   bool    `json:"pinned"`
	Path     string  `json:"path"`
}
