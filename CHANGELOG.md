# [v1.0.1](https://github.com/135yshr/md2pdf/compare/v1.0.0...v1.0.1) (2026-04-10)

## 🐛 Bug Fixes
- [`5bfddf3`](https://github.com/135yshr/md2pdf/commit/5bfddf3)  fix: prevent release conflict between semantic-release and GoReleaser

# v1.0.0 (2026-04-10)

## ✨ New Features
- [`3c731c0`](https://github.com/135yshr/md2pdf/commit/3c731c0)  feat: add issue templates and security policy 
- [`9f36e19`](https://github.com/135yshr/md2pdf/commit/9f36e19)  feat: copy relative images to working directory 
- [`6141e4e`](https://github.com/135yshr/md2pdf/commit/6141e4e)  feat: add Homebrew release via GoReleaser 

## 🐛 Bug Fixes
- [`d2859aa`](https://github.com/135yshr/md2pdf/commit/d2859aa)  fix: stop .gitignore from excluding cmd/md2pdf directory 
- [`96e848a`](https://github.com/135yshr/md2pdf/commit/96e848a)  fix: support macOS Chromium detection in chromiumPath 
- [`b042a76`](https://github.com/135yshr/md2pdf/commit/b042a76)  fix: validate CHROME_PATH is executable, fail fast 
- [`ed8c50c`](https://github.com/135yshr/md2pdf/commit/ed8c50c)  fix: use Node.js 22 for semantic-release v25 

## 🔒 Security Issues
- [`9c3384e`](https://github.com/135yshr/md2pdf/commit/9c3384e)  security: prevent path traversal in image copy
