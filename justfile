# Costa CLI Justfile
# Just is a command runner: https://just.systems

# List all available recipes
default:
    @just --list

# Install development tools (golangci-lint, govulncheck)
install-tools:
    @echo "Installing development tools..."
    @go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    @go install golang.org/x/vuln/cmd/govulncheck@latest
    @echo "✓ Tools installed successfully"

# Check if golangci-lint is installed, install if missing
_ensure-golangci-lint:
    #!/usr/bin/env bash
    set -euo pipefail
    version="v1.64.5"
    mkdir -p .bin
    echo "Installing golangci-lint ${version} with $(go version)..."
    GOBIN="$PWD/.bin" go install "github.com/golangci/golangci-lint/cmd/golangci-lint@${version}"
    "$PWD/.bin/golangci-lint" version

# Check if govulncheck is installed, install if missing
_ensure-govulncheck:
    #!/usr/bin/env bash
    set -euo pipefail
    version="v1.1.4"   # pick a version and pin it
    mkdir -p .bin
    echo "Installing govulncheck ${version} with $(go version)..."
    GOBIN="$PWD/.bin" go install "golang.org/x/vuln/cmd/govulncheck@${version}"
    "$PWD/.bin/govulncheck" -version || true

# Build the costa binary
build:
    go build -ldflags="-X 'github.com/costa-app/costa-cli/pkg/version.Version=v0.0.0' -X 'github.com/costa-app/costa-cli/pkg/version.Commit=$(git describe --always --dirty --abbrev=7 2>/dev/null || echo none)' -X 'github.com/costa-app/costa-cli/pkg/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" -o costa ./cmd/costa

# Run all tests
test:
    go test ./...

# Format all Go files
fmt:
    go fmt ./...

# Run go vet
vet:
    go vet ./...

# Tidy go.mod
tidy:
    go mod tidy

# Run golangci-lint
lint: _ensure-golangci-lint
    .bin/golangci-lint run

# Fix linting issues automatically where possible
lint-fix: _ensure-golangci-lint
    .bin/golangci-lint run --fix

# Run vulnerability check
vuln: _ensure-govulncheck
    .bin/govulncheck ./...

# Run all checks (fmt, vet, tidy, lint, test, vuln) - same as CI
ci: fmt vet tidy lint test vuln
    @echo "All CI checks passed!"

# Run all checks (lint + test)
check: lint test

# Run costa status command
status: build
    ./costa status

# Run costa with arbitrary args, e.g. `just run version`
run *ARGS: build
    ./costa {{ARGS}}

# Clean build artifacts
clean:
    rm -f costa
    rm -f coverage.out coverage.html

# Install costa to $GOPATH/bin
install:
    go install ./cmd/costa

# Create and push a new release tag (usage: just release v1.2.3)
release VERSION:
    #!/usr/bin/env bash
    set -euo pipefail

    # Refuse if working tree is not clean (tracked or untracked changes)
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "Error: working tree not clean. Commit/stash changes first."
        git status --short
        exit 1
    fi

    # TODO: In the future, enforce releases only from main and in sync with origin
    # branch="$(git rev-parse --abbrev-ref HEAD)"
    # if [[ "$branch" != "main" ]]; then
    #     echo "Error: release must be cut from main (current: $branch)"
    #     exit 1
    # fi
    # git fetch -q origin
    # if ! git diff --quiet --exit-code "origin/$branch"...HEAD; then
    #     echo "Error: local branch not in sync with origin/$branch"
    #     exit 1
    # fi

    # Validate SemVer (allows dotted prerelease identifiers)
    if [[ ! "{{VERSION}}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]]; then
        echo "Error: VERSION must be in format v1.2.3 or v1.2.3-alpha.0"
        exit 1
    fi

    # Ensure tag does not already exist
    if git tag | grep -q "^{{VERSION}}$"; then
        echo "Error: Tag {{VERSION}} already exists"
        exit 1
    fi

    echo "Creating release {{VERSION}}..."
    git tag -a "{{VERSION}}" -m "Release {{VERSION}}"
    git push origin "{{VERSION}}"
    echo "✓ Tag {{VERSION}} pushed. GitHub Actions will build and publish the release."

# Run all pre-commit checks (same as CI)
pre-commit: clean ci
