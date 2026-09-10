<!--
  The title must start with a gitmoji, in :shortcode: form, e.g.

      :sparkles: feat: add a -doctor flag
      :bug: fix: find Noto Sans CJK where it is actually installed

  It becomes the squash commit subject on main, and that is what decides the
  release. Without one, nothing is tagged and nothing ships. See "Releases" in
  CONTRIBUTING.md.
-->

## Summary

<!-- What does this PR change and why? -->

## Type of Change

- [ ] Bug fix
- [ ] New feature
- [ ] Documentation update
- [ ] Refactoring / code cleanup

## Testing

- [ ] Unit tests pass with no external tools installed (`go test ./...`)
- [ ] Integration tests pass (`MD2PDF_REQUIRE_INTEGRATION=1 go test ./...`)
- [ ] New tests added for changed behavior

## Checklist

- [ ] The title starts with a gitmoji, so this will actually be released
- [ ] `go vet ./...` reports no issues and `golangci-lint run ./...` is clean
- [ ] All exported symbols have GoDoc comments in English
- [ ] CONTRIBUTING.md guidelines followed
