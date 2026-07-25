#!/usr/bin/env node
'use strict';

// Recurrence guard: a Markdown inline code span (`` `like this` ``) that
// got line-wrapped by an editor/formatter renders with a spurious space
// where the line break was -- e.g. "`npm update -g\n   cli-comrade`"
// renders as "npm update -g cli-comrade" with an extra space, not the
// intended single command. This exact bug class has shipped in this
// repo's docs four times now; each one had to be caught by hand.
//
// Algorithm (a naive "are backticks balanced" count does NOT catch this --
// a span broken across a line is still an EVEN total count of backticks):
//   1. Strip fenced code blocks (``` ... ``` / ~~~ ... ~~~) first --
//      backticks inside a real code fence are literal content, not
//      inline-code delimiters, and commonly appear on their own line
//      there on purpose.
//   2. Walk the remaining text and pair up backtick characters
//      sequentially: 1st with 2nd, 3rd with 4th, and so on.
//   3. Flag any pair whose enclosed text contains a newline.
//
// Scans every *.md file in the repo except docs/history/ (an explicitly
// frozen, never-modified archival directory per this repo's own CLAUDE.md
// -- see its "Otonom Yürütme Protokolü" section) and node_modules/.

const fs = require('fs');
const path = require('path');

const REPO_ROOT = path.resolve(__dirname, '..', '..');
const EXCLUDED_DIR_NAMES = new Set(['node_modules', '.git']);
const EXCLUDED_RELATIVE_DIRS = ['docs/history'];

function isExcluded(relativePath) {
  return EXCLUDED_RELATIVE_DIRS.some(
    (excluded) => relativePath === excluded || relativePath.startsWith(excluded + path.sep)
  );
}

function listMarkdownFiles(dir, out) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    const relativePath = path.relative(REPO_ROOT, fullPath);
    if (entry.isDirectory()) {
      if (EXCLUDED_DIR_NAMES.has(entry.name) || isExcluded(relativePath)) continue;
      listMarkdownFiles(fullPath, out);
    } else if (entry.isFile() && entry.name.endsWith('.md')) {
      out.push(fullPath);
    }
  }
  return out;
}

/**
 * Blanks out fenced code block lines (opening/closing fence markers AND
 * their content), leaving everything else untouched -- including line
 * count/positions, so line numbers computed later still line up with the
 * original file.
 *
 * @param {string} text
 * @returns {string}
 */
function stripFencedBlocks(text) {
  const lines = text.split('\n');
  let inFence = false;
  const stripped = lines.map((line) => {
    const isFenceMarker = /^\s*(`{3,}|~{3,})/.test(line);
    if (isFenceMarker) {
      inFence = !inFence;
      return '';
    }
    return inFence ? '' : line;
  });
  return stripped.join('\n');
}

/**
 * @param {string} text - raw file content.
 * @returns {{lineNumber: number, snippet: string}[]} every inline code span
 *   (sequential backtick pairing, fenced blocks already stripped) whose
 *   enclosed text spans more than one line.
 */
function findLineBrokenCodeSpans(text) {
  const stripped = stripFencedBlocks(text);
  const backtickPositions = [];
  for (let i = 0; i < stripped.length; i++) {
    if (stripped[i] === '`') backtickPositions.push(i);
  }

  const violations = [];
  for (let i = 0; i + 1 < backtickPositions.length; i += 2) {
    const start = backtickPositions[i];
    const end = backtickPositions[i + 1];
    const enclosed = stripped.slice(start + 1, end);
    if (enclosed.includes('\n')) {
      const lineNumber = stripped.slice(0, start).split('\n').length;
      violations.push({ lineNumber, snippet: enclosed.replace(/\s+/g, ' ').trim() });
    }
  }
  return violations;
}

function main() {
  const files = listMarkdownFiles(REPO_ROOT, []);
  let totalViolations = 0;

  for (const file of files) {
    const text = fs.readFileSync(file, 'utf8');
    const violations = findLineBrokenCodeSpans(text);
    for (const violation of violations) {
      const relativePath = path.relative(REPO_ROOT, file);
      console.error(
        `${relativePath}:${violation.lineNumber}: line-broken inline code span -- \`${violation.snippet}\``
      );
      totalViolations++;
    }
  }

  if (totalViolations > 0) {
    console.error(
      `test-markdown-code-spans.js: ${totalViolations} line-broken inline code span(s) found across ${files.length} scanned file(s)`
    );
    process.exit(1);
  }

  console.log(`test-markdown-code-spans.js: scanned ${files.length} markdown file(s), no line-broken inline code spans found`);
}

main();
