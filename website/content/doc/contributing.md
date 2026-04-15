---
title: "Contributing"
description: "How to contribute to md2pdf."
weight: 40
---

Contributions are welcome! Here's how to get started.

## Development setup

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

## Running tests

```sh
# Unit tests (no external dependencies required)
go test ./internal/converter/ -run 'Test[^C]'

# All tests including integration
go test ./...
```

## Pull request guidelines

1. Fork the repository and create a branch from `main`.
2. Keep commits focused — one logical change per commit.
3. Add or update tests for any changed behaviour.
4. Ensure `go test ./...` passes and `go vet ./...` reports no issues.
5. Write commit messages in English in the imperative mood.
6. Open a pull request against `main`.

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- All exported symbols must have GoDoc comments in English.
- Errors crossing package boundaries must be wrapped.

## License

By contributing you agree that your contributions will be licensed under the MIT License.
