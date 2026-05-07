#!/usr/bin/env node
import { readFile, writeFile, readdir, mkdir } from 'node:fs/promises';
import { join, dirname, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { processHtml } from './lib/transforms.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const FRONTEND = join(__dirname, '..');
const REPO = join(FRONTEND, '..');

const DRY_RUN = process.argv.includes('--dry-run');
const CHECK = process.argv.includes('--check');

// Walk a directory recursively and return absolute paths of HTML files.
async function listHtmlFiles(rootRel) {
  const root = join(FRONTEND, rootRel);
  const entries = await readdir(root, { withFileTypes: true, recursive: true }).catch(() => []);
  return entries
    .filter(e => e.isFile() && e.name.endsWith('.html'))
    .map(e => join(e.parentPath || root, e.name));
}

async function collect() {
  const files = new Set();
  files.add(join(FRONTEND, 'index.html'));
  for (const f of await listHtmlFiles('public')) {
    files.add(f);
  }
  return [...files].sort();
}

async function main() {
  const files = await collect();
  const report = { processed: 0, changed: 0, unchanged: 0, files: [] };
  let pendingChanges = 0;

  for (const file of files) {
    const before = await readFile(file, 'utf8');
    const after = processHtml(before);
    const changed = before !== after;
    const rel = relative(FRONTEND, file);
    report.processed++;
    if (changed) {
      report.changed++;
      report.files.push({ path: rel, changed: true });
      pendingChanges++;
      if (DRY_RUN || CHECK) {
        console.log(`[changed] ${rel}`);
      } else {
        await writeFile(file, after, 'utf8');
      }
    } else {
      report.unchanged++;
    }
  }

  await mkdir(join(REPO, 'artifacts'), { recursive: true });
  const reportPath = join(REPO, 'artifacts', 'geo-codemod-report.json');
  await writeFile(reportPath, JSON.stringify(report, null, 2), 'utf8');

  console.log(`\nProcessed ${report.processed}, changed ${report.changed}, unchanged ${report.unchanged}`);
  console.log(`Report: ${relative(REPO, reportPath)}`);

  if (CHECK && pendingChanges > 0) {
    console.error('\n[--check] pending changes detected; run `pnpm geo:upgrade` to apply.');
    process.exit(1);
  }
}

main().catch(e => {
  console.error(e);
  process.exit(1);
});
