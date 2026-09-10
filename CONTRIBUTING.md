# Contributing to md2pdf

Thank you for your interest in contributing! This document explains the workflow.

## Development Setup

```sh
git clone https://github.com/135yshr/md2pdf.git
cd md2pdf
go mod download
```

Install external dependencies:

```sh
npm install -g @mermaid-js/mermaid-cli
# a Chromium for PDF output (Google Chrome also works)
brew install --cask chromium
sudo apt install fonts-noto-cjk   # Ubuntu/Debian
```

## Running Tests

```sh
# Every test. The integration test skips itself when its tools are absent.
go test ./...

# Require the integration toolchain (mmdc, a Chromium, CJK fonts): a missing
# tool fails instead of skipping. This is what CI runs.
MD2PDF_REQUIRE_INTEGRATION=1 go test ./...
```

## Pull Request Guidelines

1. Fork the repository and create a branch from `main`.
2. Keep commits focused — one logical change per commit.
3. Add or update tests for any changed behavior. `go test ./...` must pass with
   no external tools installed, so a test that needs mmdc, a Chromium or pandoc
   should either skip when the tool is absent (see `requireTool`) or
   drive a stub binary. Do not exclude tests from the run with `-run` filters or
   build tags — that has silently dropped tests here before.
4. Ensure `go test ./...` passes and `go vet ./...` reports no issues.
   CI also runs `golangci-lint run ./...`, which must be clean. Install the
   linter with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`
   rather than from a package manager: it has to be built with the Go version
   `go.mod` targets, or it refuses to read the config.
5. Write commit messages in English in the imperative mood ("Add feature", not "Added feature").
6. Open a pull request against `main` and fill in the PR template.

## Reporting Bugs

Please use the **Bug Report** issue template and include:
- md2pdf version (`md2pdf -version`)
- OS and Go version
- Minimal Markdown input that reproduces the problem
- Full error output

## Suggesting Features

Open a **Feature Request** issue describing the use case and expected behavior.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- All exported symbols must have GoDoc comments in English.
- Internal comments should also be in English.

## License

By contributing you agree that your contributions will be licensed under the MIT License.
