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
pip install playwright
playwright install chromium
sudo apt install fonts-noto-cjk   # Ubuntu/Debian
```

## Running Tests

```sh
# Unit tests (no external dependencies required)
go test ./internal/converter/ -run 'Test[^C]'

# All tests including integration
go test ./...
```

## Pull Request Guidelines

1. Fork the repository and create a branch from `main`.
2. Keep commits focused — one logical change per commit.
3. Add or update tests for any changed behaviour.
4. Ensure `go test ./...` passes and `go vet ./...` reports no issues.
5. Write commit messages in English in the imperative mood ("Add feature", not "Added feature").
6. Open a pull request against `main` and fill in the PR template.

## Reporting Bugs

Please use the **Bug Report** issue template and include:
- md2pdf version (`md2pdf -version`)
- OS and Go version
- Minimal Markdown input that reproduces the problem
- Full error output

## Suggesting Features

Open a **Feature Request** issue describing the use case and expected behaviour.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- All exported symbols must have GoDoc comments in English.
- Internal comments should also be in English.

## License

By contributing you agree that your contributions will be licensed under the MIT License.
