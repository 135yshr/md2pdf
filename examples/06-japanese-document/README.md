# Example: 完全日本語ドキュメント（PRD）

仮想プロダクト「ChouseiNeko v2.0」のプロダクト要件定義書（PRD）です。
**完全に日本語で書かれた実務文書**として、md2pdf の日本語レンダリング
品質を確認するためのショーケースです。

`examples/01-design-doc` が「英語ベース＋日本語混在の技術文書」だったのに
対し、こちらは「**敬体（ですます調）で書かれた、非エンジニアにも読まれる
ビジネス文書**」を想定しています。

## 含まれるファイル

- `input.md` — 入力Markdown
- `output.pdf` — md2pdf で生成されたPDF（リポジトリにコミット）

## このサンプルが示すもの

- ✅ 完全日本語の見出し・本文（敬体）
- ✅ Mermaid **フローチャート**（v1 と v2.0 の操作フロー比較）
- ✅ Mermaid **ガントチャート**（リリース計画）
- ✅ GitHub Flavored Markdown の表（KPI・ペルソナ・成功指標など複数）
- ✅ コードブロック（JSON によるAPIスキーマ例）
- ✅ ユーザーインタビューの肉声を再現する引用ブロック
- ✅ MUST / SHOULD / COULD で整理した箇条書きとスコープ定義
- ✅ 変更履歴・関連ドキュメント・用語集など実務PRDの定型要素

## PDFを再生成する

```sh
md2pdf -o examples/06-japanese-document/output.pdf examples/06-japanese-document/input.md
```

書式の調整例:

```sh
md2pdf \
  -page-size A4 \
  -margin-top 20mm -margin-bottom 20mm \
  -margin-left 18mm -margin-right 18mm \
  -o examples/06-japanese-document/output.pdf \
  examples/06-japanese-document/input.md
```

## 想定される利用シーン

- プロダクトマネージャーが Markdown で書いた PRD を、社内向けに PDF で配布
- 経営層・営業・カスタマーサクセスへの共有資料として配布
- レビュー後にバージョン管理した PDF をクライアントへ提示
