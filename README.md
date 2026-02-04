# costa cli [![Release](https://img.shields.io/github/v/release/costa-app/costa-cli?color=4f46e5&logo=github&logoColor=white)](https://github.com/costa-app/costa-cli/releases/latest) [![Pre-release](https://img.shields.io/github/v/release/costa-app/costa-cli?include_prereleases&label=pre-release&color=818cf8&logo=github&logoColor=white)](https://github.com/costa-app/costa-cli/releases)

Command-line tool for managing [Costa](https://costa.app) and integrating it with IDEs.



[![Go Version](https://img.shields.io/badge/go-1.25-blue)](https://go.dev/)
[![Lint](https://github.com/costa-app/costa-cli/workflows/lint/badge.svg)](https://github.com/costa-app/costa-cli/actions)
[![Test](https://github.com/costa-app/costa-cli/workflows/test/badge.svg)](https://github.com/costa-app/costa-cli/actions)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)

Install from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=costa.costa-code)

## Overview

`costa` is a CLI that authenticates you to Costa's team coding platform and configures supported AI coding tools to use Costa's API and models.

Features:
- Connect Costa to your AI coding tools (including Claude Code, Codex and many more...)
- Check your current Costa usage (sessions and points)
- View your Costa threads (coming soon)

## Installation

### Homebrew (macOS)

```bash
brew install --cask costa-app/costa-cli/costa
```

### Pre-built Binaries

Download the latest release for your platform from [GitHub Releases](https://github.com/costa-app/costa-cli/releases).

macOS universal binary (Apple Silicon + Intel):
```bash
curl -L https://github.com/costa-app/costa-cli/releases/latest/download/costa_darwin_all -o costa
chmod +x costa
sudo mv costa /usr/local/bin/
```

Linux (amd64):
```bash
curl -LO https://github.com/costa-app/costa-cli/releases/latest/download/costa_<VERSION>_linux_amd64.tar.gz
tar xzf costa_*_linux_amd64.tar.gz
sudo mv costa /usr/local/bin/
```

Linux (arm64):
```bash
curl -LO https://github.com/costa-app/costa-cli/releases/latest/download/costa_<VERSION>_linux_arm64.tar.gz
tar xzf costa_*_linux_arm64.tar.gz
sudo mv costa /usr/local/bin/
```

### From Source

Requires Go 1.25 or later:

```bash
go install github.com/costa-app/costa-cli/cmd/costa@latest
```

Or clone and build:

```bash
git clone https://github.com/costa-app/costa-cli.git
cd costa-cli
go build -o costa ./cmd/costa
```

## Quick Start

1. **Authenticate to Costa:**
   ```bash
   costa login
   ```
   This opens your browser to complete OAuth authentication and stores credentials in `~/.config/costa/token.json`.

2. **Configure Claude Code:**
   ```bash
   costa setup claude-code
   ```
   This configures Claude Code (CLI or VS Code extension) to use Costa's API. Settings are written to `~/.claude/settings.json` with automatic backup.

3. **Verify your setup:**
   ```bash
   costa setup status claude-code
   ```

## Usage

### Authentication

```bash
# Log in via OAuth2
costa login

# View current authentication status
costa token

# View token details as JSON
costa token --json

# Log out (removes stored credentials)
costa logout
```

### Claude Code Integration

Configure Claude Code to use Costa:

```bash
# Interactive setup (user scope, default)
costa setup claude-code

# Project-scoped setup (writes to ./.claude/settings.json)
costa setup claude-code --project

# Dry run (preview changes without applying)
costa setup claude-code --dry-run

# Non-interactive mode
costa setup claude-code --yes

# Update all settings (not just token)
costa setup claude-code --update

# Only refresh the authentication token
costa setup claude-code --refresh-token-only

# Check current configuration status
costa setup status claude-code
```

The setup command:
- Merges settings non-destructively (preserves your custom keys)
- Creates timestamped backups before making changes
- Always updates the auth token if it has changed
- Only overwrites other settings when `--update` is specified
- Supports both user (`~/.claude/settings.json`) and project (`./.claude/settings.json`) scopes

### Version Information

```bash
# Short version
costa version

# Detailed build info
costa status
```

## Configuration

### Environment Variables

- `COSTA_BASE_URL` - Override the Costa API base URL (default: `https://ai.costa.app`)
- `COSTA_DEBUG` - Enable debug logging (set to `1`)

### Files Created

- `~/.config/costa/token.json` - OAuth and coding tokens (mode 0600)
- `~/.claude/settings.json` or `./.claude/settings.json` - Claude Code configuration
- `~/.config/costa/backups/claude-code/settings-<timestamp>.json` - Automatic backups

## Development

### Prerequisites

- Go 1.25 or later
- [just](https://just.systems/) (optional, for task running)
- [mise](https://mise.jdx.dev/) (optional, for version management)

### Building

```bash
# Simple build
go build -o costa ./cmd/costa

# Or with just
just build

# Build with version info
just build-release v1.0.0
```

### Testing

```bash
# Run tests
go test ./...

# Or with just
just test

# Run all CI checks (format, vet, lint, test, vuln)
just ci
```

### Project Structure

```
.
├── cmd/costa/              # Application entrypoint
├── internal/
│   ├── cli/                # Command implementations (login, setup, etc.)
│   ├── auth/               # OAuth2 and token management
│   ├── integrations/       # IDE integration implementations
│   │   └── claudecode/     # Claude Code integration
│   └── debug/              # Debug utilities
├── pkg/
│   └── version/            # Version information
├── .github/workflows/      # CI/CD pipelines
├── .goreleaser.yml         # Release configuration
└── justfile                # Development tasks
```

### Release Process

Releases are automated via GitHub Actions:

```bash
# Create and push a new tag
just release v1.0.0
```

This triggers the release workflow which:
- Builds universal macOS binaries (Apple Silicon + Intel)
- Builds Linux binaries (amd64 + arm64)
- Signs and notarizes macOS binaries (when credentials are configured)
- Updates Homebrew tap for macOS
- Creates a GitHub release with artifacts and changelog

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

Quick checklist:
- PR title must follow [Conventional Commits](https://www.conventionalcommits.org/) format (enforced by CI)
- All tests pass: `go test ./...`
- All checks pass: `just ci` (formatting, linting, tests, vulnerability scan)

We use squash-merge, so your PR title becomes the commit message on main.

## License

BSD 3-Clause License - see [LICENSE](LICENSE) for details.

## Support

- Report issues: https://github.com/costa-app/costa-cli/issues
- Documentation: https://docs.costa.app
