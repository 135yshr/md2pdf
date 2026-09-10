#!/usr/bin/env node
//
// Fails unless the pull request title would produce a release.
//
// This repository squash-merges, so the pull request title becomes the commit
// subject on main, and `semantic-release-gitmoji` reads the *leading* gitmoji of
// that subject to size the release. A title without one is not an error
// anywhere: `Auto Release` succeeds, reports "There will be no new version",
// and the goreleaser job is skipped by its `new_tag != ''` guard. Nothing is
// tagged, no GitHub release is published and the Homebrew formula is not
// updated -- silently. That happened twice before this check existed.
//
// The accepted tokens are not listed here. The check loads `.releaserc.json`
// and calls the plugin's own `getReleaseType`, so it cannot disagree with what
// the release will actually do.
//
// Usage: PR_TITLE='<title>' node .github/scripts/check-pr-title.js
//
// The title arrives through the environment rather than argv on purpose: it is
// attacker-controlled text, and interpolating it into a workflow `run:` string
// is the standard GitHub Actions script-injection hole.

const fs = require('fs')
const path = require('path')

const PLUGIN = 'semantic-release-gitmoji'

const ReleaseNotes = require(`${PLUGIN}/lib/release-notes`)
const getConfig = require(`${PLUGIN}/lib/helper/get-config`)
const { hasEmoji, emojify } = require(`${PLUGIN}/node_modules/node-emoji`)

const REPO_ROOT = path.join(__dirname, '..', '..')
const RELEASERC = path.join(REPO_ROOT, '.releaserc.json')

// Pull the gitmoji plugin's options out of the semantic-release config, which
// lists plugins as either a bare name or a [name, options] pair.
function pluginOptions () {
  const { plugins = [] } = JSON.parse(fs.readFileSync(RELEASERC, 'utf8'))
  const entry = plugins.find((p) => (Array.isArray(p) ? p[0] : p) === PLUGIN)
  if (!entry) throw new Error(`${RELEASERC} does not configure ${PLUGIN}`)
  return Array.isArray(entry) ? entry[1] : {}
}

// getConfig drops any shortcode node-emoji cannot resolve, without saying so, so
// a typo in the rules disables that gitmoji as quietly as a missing one in a
// title. `:camera_flash:` and `:monocle_face:` were both dead this way.
function unresolvableRules (rules) {
  return Object.entries(rules).flatMap(([bump, tokens]) =>
    tokens.filter((t) => !hasEmoji(t)).map((t) => `${bump}: ${t}`))
}

// The release type the title would produce, or undefined for no release. This
// is the plugin's production path: the same parser, the same rules, the same
// comparison -- given one synthetic commit whose subject is the title.
function releaseType (title, config) {
  const context = {
    commits: [{ subject: title, message: title, body: '' }],
    options: { repositoryUrl: 'https://github.com/135yshr/md2pdf.git' }
  }
  return new ReleaseNotes(context, config.releaseNotes)
    .getReleaseType(config.releaseRules)
}

function explain (rules) {
  const line = (bump) => `  ${bump.padEnd(5)} ${rules[bump].slice(0, 6).join(' ')}` +
    (rules[bump].length > 6 ? ` (and ${rules[bump].length - 6} more)` : '')
  return [
    '',
    'The title must start with a gitmoji, in :shortcode: form, for example:',
    '',
    '  :sparkles: feat: add a -doctor flag',
    '  :bug: fix: find Noto Sans CJK where it is actually installed',
    '',
    'Some of the tokens .releaserc.json accepts:',
    '',
    line('major'),
    line('minor'),
    line('patch'),
    '',
    'Two things this check is strict about, both for a reason:',
    '',
    '  * The gitmoji must be first. Only the start of the subject is read, so',
    '    "fix: :bug: ..." releases nothing.',
    '  * Write the :shortcode:, not the character. They are not always the same',
    '    emoji -- :construction_worker: is a different grapheme from a pasted',
    '    construction worker, and only the shortcode matches the rules.'
  ].join('\n')
}

function main () {
  const title = process.env.PR_TITLE
  if (title === undefined || title.trim() === '') {
    console.error('check-pr-title: PR_TITLE is empty')
    return 1
  }

  const options = pluginOptions()
  const config = getConfig(options)

  const dead = unresolvableRules(options.releaseRules ?? {})
  if (dead.length > 0) {
    console.error('check-pr-title: .releaserc.json lists shortcodes node-emoji cannot resolve.')
    console.error('These are dropped without a warning, so a title using one releases nothing:')
    dead.forEach((d) => console.error(`  ${d}`))
    return 1
  }

  const type = releaseType(title, config)
  if (!type) {
    console.error(`check-pr-title: this title would not produce a release:\n\n  ${title}`)
    console.error(explain(options.releaseRules ?? config.releaseRules))
    return 1
  }

  console.log(`check-pr-title: ${emojify(title)}`)
  console.log(`check-pr-title: merging this would produce a "${type}" release.`)
  return 0
}

process.exit(main())
