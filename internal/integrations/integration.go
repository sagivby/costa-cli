package integrations

import "context"

// Scope represents the configuration scope (user or project)
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// ApplyOpts contains options for applying integration configuration
type ApplyOpts struct {
	Scope            Scope
	TokenOverride    string
	BackupDir        string
	IDE              string // IDE to configure (vscode, cursor, jetbrains) - for Kilo
	Force            bool   // Skip confirmation prompt (auto-yes)
	RefreshTokenOnly bool
	DryRun           bool
	RequireInstalled bool
	EnableStatusLine bool // Enable status line in Claude Code
	SkipStatusLine   bool // Skip status line prompt
}

// ApplyResult contains the result of applying configuration
type ApplyResult struct {
	BackupPath    string
	ConfigPath    string
	UpdatedKeys   []string
	UnchangedKeys []string
	Warnings      []string
	Changed       bool
}

// StatusResult contains the status of an integration
type StatusResult struct {
	Version       string
	Scope         Scope
	ConfigPath    string
	Model         string
	TokenRedacted string
	Missing       []string
	Installed     bool
	ConfigExists  bool
	IsCosta       bool
}

// Integration represents a third-party tool integration
type Integration interface {
	// Name returns the name of the integration
	Name() string

	// Apply applies the integration configuration
	Apply(ctx context.Context, opts ApplyOpts) (ApplyResult, error)

	// Status returns the current status of the integration
	Status(ctx context.Context, scope Scope) (StatusResult, error)
}
