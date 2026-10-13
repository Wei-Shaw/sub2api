import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createHmac, randomBytes } from 'node:crypto';
import { createReadStream } from 'node:fs';
import { copyFile, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { basename, join, resolve, sep } from 'node:path';
import { Readable } from 'node:stream';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { gzipSync } from 'node:zlib';
import { decryptArchive, writeArchive } from './archive.mjs';

const run = promisify(execFile);

async function removeTestDir(dir) {
  const root = resolve(tmpdir()) + sep;
  const target = resolve(dir);
  if (!target.startsWith(root) || !basename(target).startsWith('astrogate-')) throw new Error('invalid test cleanup path');
  await rm(target, { recursive: true, force: true });
}

test('portable archive restores all bytes and secret metadata', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'astrogate-test-'));
  try {
    const file = join(dir, 'backup.agbackup');
    const dump = gzipSync(Buffer.from('CREATE TABLE sample (id integer);\nINSERT INTO sample VALUES (1);'));
    const metadata = { version: 1, created_at: new Date().toISOString(), secrets: {
      ACCOUNT_VAULT_ENCRYPTION_KEY: randomBytes(32).toString('base64'),
      TOTP_ENCRYPTION_KEY: randomBytes(32).toString('hex'),
      JWT_SECRET: randomBytes(32).toString('hex'),
    } };
    await writeArchive(file, 'correct horse battery staple', metadata, Readable.from([dump]));
    const archiveBytes = await readFile(file);
    assert.equal(archiveBytes.includes(Buffer.from('CREATE TABLE')), false);
    assert.equal(archiveBytes.includes(Buffer.from('ACCOUNT_VAULT_ENCRYPTION_KEY')), false);
    const { stdout } = await run(process.execPath, [fileURLToPath(new URL('./portable-backup.mjs', import.meta.url)), 'verify', '--in', file], {
      env: { ...process.env, BACKUP_PASSPHRASE: 'correct horse battery staple' },
    });
    assert.match(stdout, /校验通过/);
    const restored = await decryptArchive(file, 'correct horse battery staple');
    try {
      assert.deepEqual(restored.metadata, metadata);
      const chunks = [];
      for await (const chunk of createReadStream(restored.payloadPath, { start: restored.dumpOffset })) chunks.push(chunk);
      assert.deepEqual(Buffer.concat(chunks), dump);
    } finally { await rm(restored.payloadPath); }
    await assert.rejects(decryptArchive(file, 'incorrect password for test'), /密码错误|损坏/);
    const tampered = await readFile(file);
    tampered[tampered.length - 20] ^= 1;
    await writeFile(file, tampered);
    await assert.rejects(decryptArchive(file, 'correct horse battery staple'), /密码错误|损坏/);
  } finally { await removeTestDir(dir); }
});

test('export then import preserves database stream and fixed keys', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'astrogate-cli-test-'));
  try {
    const extension = process.platform === 'win32' ? '.exe' : '';
    const executable = join(dir, `pg_dump${extension}`);
    await run('go', ['build', '-o', executable, fileURLToPath(new URL('./testdata/fake-pg.go', import.meta.url))]);
    await copyFile(executable, join(dir, `psql${extension}`));
    const keys = {
      ACCOUNT_VAULT_ENCRYPTION_KEY: randomBytes(32).toString('base64'),
      TOTP_ENCRYPTION_KEY: randomBytes(32).toString('hex'),
      JWT_SECRET: randomBytes(32).toString('hex'),
    };
    const sourceSecrets = join(dir, 'source.env');
    await writeFile(sourceSecrets, Object.entries(keys).map(([key, value]) => `${key}=${value}\n`).join('') + 'DATABASE_PASSWORD=test-only\n');
    const archive = join(dir, 'portable.agbackup');
    const restoredKeys = join(dir, 'restored.env');
    const restoredSQL = join(dir, 'restored.sql');
    const fingerprint = createHmac('sha256', Buffer.from(keys.ACCOUNT_VAULT_ENCRYPTION_KEY, 'base64'))
      .update('sub2api.accountvault.key-check.v1').digest('hex');
    const env = { ...process.env, BACKUP_PASSPHRASE: 'synthetic test passphrase', DATABASE_PASSWORD: 'test-only',
      FAKE_VAULT_FINGERPRINT: fingerprint, FAKE_RESTORE_OUTPUT: restoredSQL };
    const cli = fileURLToPath(new URL('./portable-backup.mjs', import.meta.url));
    await run(process.execPath, [cli, 'export', '--out', archive, '--secrets-file', sourceSecrets, '--pg-bin', dir], { env });
    await run(process.execPath, [cli, 'import', '--in', archive, '--secrets-out', restoredKeys, '--replace', '--pg-bin', dir], { env });
    assert.match(await readFile(restoredSQL, 'utf8'), /INSERT INTO sample VALUES \(42\)/);
    const output = await readFile(restoredKeys, 'utf8');
    for (const [name, value] of Object.entries(keys)) assert.ok(output.includes(`${name}=${value}\n`));
    await assert.rejects(run(process.execPath, [cli, 'import', '--in', archive, '--secrets-out', join(dir, 'wrong.env'), '--replace', '--pg-bin', dir],
      { env: { ...env, BACKUP_PASSPHRASE: 'incorrect test password' } }));
  } finally { await removeTestDir(dir); }
});
