#!/usr/bin/env node
import { createReadStream } from 'node:fs';
import { access, link, readFile, unlink, writeFile } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { createGunzip, createGzip } from 'node:zlib';
import { Writable } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { dirname, join, resolve } from 'node:path';
import { createHmac, randomBytes, timingSafeEqual } from 'node:crypto';
import { decryptArchive, writeArchive } from './archive.mjs';

const SECRET_NAMES = ['ACCOUNT_VAULT_ENCRYPTION_KEY', 'TOTP_ENCRYPTION_KEY', 'JWT_SECRET'];

function usage() {
  console.log(`用法：
  node portable-backup.mjs export --out backup.agbackup --secrets-file /path/to/.env [--pg-bin /path/to/bin]
  node portable-backup.mjs check --secrets-file /path/to/.env [--pg-bin /path/to/bin]
  node portable-backup.mjs verify --in backup.agbackup
  node portable-backup.mjs import --in backup.agbackup --secrets-out /path/to/restored-keys.env --replace [--pg-bin /path/to/bin]

从 BACKUP_PASSPHRASE 环境变量读取迁移包密码（至少 12 字符）。数据库连接读取
DATABASE_HOST、DATABASE_PORT、DATABASE_USER、DATABASE_DBNAME、DATABASE_PASSWORD；
也可用 --db-host、--db-port、--db-user、--db-name 指定非机密参数。
导入会替换目标数据库。操作前停止目标服务，先备份目标数据库；
完成后将 restored-keys.env 的密钥应用到目标部署配置并重启服务。`);
}

function options(args) {
  const command = args.shift();
  if (command === '--help' || command === 'help' || !command) return { command: 'help' };
  if (!['check', 'export', 'verify', 'import'].includes(command)) throw new Error('未知命令');
  const result = { command };
  for (let i = 0; i < args.length; i++) {
    const name = args[i];
    if (name === '--replace') { result.replace = true; continue; }
    if (!['--out', '--in', '--secrets-file', '--secrets-out', '--pg-bin', '--db-host', '--db-port', '--db-user', '--db-name'].includes(name) || !args[i + 1]) {
      throw new Error('参数无效；请运行 --help');
    }
    result[name.slice(2).replaceAll('-', '_')] = args[++i];
  }
  if (command === 'export' && (!result.out || !result.secrets_file)) throw new Error('导出需要 --out 和 --secrets-file');
  if (command === 'check' && !result.secrets_file) throw new Error('检查需要 --secrets-file');
  if (command === 'verify' && !result.in) throw new Error('校验需要 --in');
  if (command === 'import' && (!result.in || !result.secrets_out || !result.replace)) throw new Error('导入需要 --in、--secrets-out 和 --replace');
  return result;
}

async function readEnvironmentFile(path) {
  const file = await readFile(path, 'utf8');
  if (file.length > 1024 * 1024) throw new Error('密钥配置文件过大');
  const values = {};
  for (const line of file.split(/\r?\n/)) {
    if (!line.trim() || line.trimStart().startsWith('#')) continue;
    const match = /^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$/.exec(line);
    if (!match) continue;
    let value = match[2].trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) value = value.slice(1, -1);
    values[match[1]] = value;
  }
  return values;
}

function checkedSecrets(values) {
  const secrets = Object.fromEntries(SECRET_NAMES.map(name => [name, values[name]]));
  if (Object.values(secrets).some(value => typeof value !== 'string' || !value || value.length > 4096 || /[\r\n\0]/.test(value))) {
    throw new Error('缺少账号库、TOTP 或 JWT 固定密钥；不能生成可迁移备份');
  }
  const vault = Buffer.from(secrets.ACCOUNT_VAULT_ENCRYPTION_KEY, 'base64');
  if (vault.length !== 32 || vault.toString('base64') !== secrets.ACCOUNT_VAULT_ENCRYPTION_KEY || !/^[a-fA-F0-9]{64}$/.test(secrets.TOTP_ENCRYPTION_KEY)) {
    throw new Error('账号库或 TOTP 密钥格式无效');
  }
  return secrets;
}

function database(options, values) {
  const selected = name => options[`db_${name === 'DBNAME' ? 'name' : name.toLowerCase()}`] || process.env[`DATABASE_${name}`] || values[`DATABASE_${name}`];
  const host = selected('HOST') || '127.0.0.1';
  const port = selected('PORT') || '5432';
  const user = selected('USER') || 'sub2api';
  const name = selected('DBNAME') || 'sub2api';
  const password = process.env.DATABASE_PASSWORD || values.DATABASE_PASSWORD || process.env.PGPASSWORD;
  if (!/^\d{1,5}$/.test(port) || Number(port) < 1 || Number(port) > 65535 || !password) {
    throw new Error('数据库端口或 DATABASE_PASSWORD 无效');
  }
  return { args: ['-h', host, '-p', port, '-U', user, '-d', name], env: { ...process.env, PGPASSWORD: password } };
}

function postgres(command, args, env, bin) {
  const child = spawn(bin ? join(resolve(bin), command + (process.platform === 'win32' ? '.exe' : '')) : command, args, {
    env, stdio: command === 'pg_dump' ? ['ignore', 'pipe', 'ignore'] : ['pipe', 'ignore', 'ignore'], windowsHide: true,
  });
  const done = new Promise((accept, reject) => {
    child.once('error', () => reject(new Error(`${command} 启动失败，请安装 PostgreSQL 客户端或指定 --pg-bin`)));
    child.once('close', code => code === 0 ? accept() : reject(new Error(`${command} 执行失败，请检查数据库连接和目标版本`)));
  });
  return { child, done };
}

async function verifyVaultKey(db, bin, keyText) {
  const executable = bin ? join(resolve(bin), 'psql' + (process.platform === 'win32' ? '.exe' : '')) : 'psql';
  const child = spawn(executable, ['-X', '-v', 'ON_ERROR_STOP=1', '-t', '-A', ...db.args,
    '-c', 'SELECT key_fingerprint FROM account_vault_metadata WHERE id=1'], {
    env: db.env, stdio: ['ignore', 'pipe', 'ignore'], windowsHide: true,
  });
  const output = [];
  let size = 0;
  child.stdout.on('data', chunk => {
    size += chunk.length;
    if (size <= 128) output.push(chunk);
    else child.kill();
  });
  await new Promise((accept, reject) => {
    child.once('error', () => reject(new Error('psql 启动失败，请安装 PostgreSQL 客户端或指定 --pg-bin')));
    child.once('close', code => code === 0 ? accept() : reject(new Error('无法核对账号库密钥，请检查数据库连接和迁移版本')));
  });
  const fingerprint = Buffer.concat(output).toString('utf8').trim();
  if (!fingerprint) return; // A new vault has no key binding yet.
  const computed = createHmac('sha256', Buffer.from(keyText, 'base64'))
    .update('sub2api.accountvault.key-check.v1').digest('hex');
  if (!/^[a-f0-9]{64}$/.test(fingerprint) ||
      !timingSafeEqual(Buffer.from(fingerprint, 'hex'), Buffer.from(computed, 'hex'))) {
    throw new Error('账号库密钥与数据库不匹配，已停止导出');
  }
}

async function exists(path) {
  try { await access(path); return true; }
  catch { return false; }
}

async function exportBackup(opts, password) {
  const { secrets, db } = await checkSource(opts);
  const target = resolve(opts.out);
  if (await exists(target)) throw new Error('目标备份文件已存在');
  const partial = join(dirname(target), `.${randomBytes(12).toString('hex')}.partial`);
  const { child, done } = postgres('pg_dump', [...db.args, '--no-owner', '--no-acl', '--clean', '--if-exists'], db.env, opts.pg_bin);
  void done.catch(() => {});
  const compressor = createGzip();
  child.stdout.on('error', error => compressor.destroy(error));
  child.stdout.pipe(compressor);
  try {
    await writeArchive(partial, password, { version: 1, created_at: new Date().toISOString(), secrets }, compressor);
    await done;
    await link(partial, target); // Exclusive: never overwrite another backup.
    await unlink(partial);
    console.log('迁移备份已生成：' + target);
  } catch (error) {
    child.kill();
    await done.catch(() => {});
    await unlink(partial).catch(() => {});
    throw error;
  }
}

async function checkSource(opts) {
  const values = await readEnvironmentFile(opts.secrets_file);
  const secrets = checkedSecrets({ ...values, ...process.env });
  const db = database(opts, values);
  await verifyVaultKey(db, opts.pg_bin, secrets.ACCOUNT_VAULT_ENCRYPTION_KEY);
  return { secrets, db };
}

async function importBackup(opts, password) {
  const secretTarget = resolve(opts.secrets_out);
  if (await exists(secretTarget)) throw new Error('密钥输出文件已存在，请选择新路径');
  const decrypted = await decryptArchive(resolve(opts.in), password);
  try {
    const secrets = validateMetadata(decrypted.metadata);
    const destinationValues = opts.secrets_file ? await readEnvironmentFile(opts.secrets_file) : {};
    const db = database(opts, destinationValues);
    const { child, done } = postgres('psql', ['-X', '-v', 'ON_ERROR_STOP=1', '--single-transaction', ...db.args], db.env, opts.pg_bin);
    void done.catch(() => {});
    try {
      await pipeline(createReadStream(decrypted.payloadPath, { start: decrypted.dumpOffset }), createGunzip(), child.stdin);
      await done;
    } catch {
      child.kill();
      await done.catch(() => {});
      throw new Error('数据库恢复失败，未输出密钥；请检查 PostgreSQL 版本和目标数据库');
    }
    const content = SECRET_NAMES.map(name => `${name}=${secrets[name]}\n`).join('');
    await writeFile(secretTarget, content, { flag: 'wx', mode: 0o600 });
    console.log('数据库已恢复；密钥已写入：' + secretTarget);
    console.log('将这些密钥应用到目标部署配置并重启服务，账号库及 2FA 才可使用。');
  } finally { await unlink(decrypted.payloadPath).catch(() => {}); }
}

function validateMetadata(metadata) {
  if (metadata?.version !== 1 || typeof metadata.created_at !== 'string') throw new Error('备份版本不受支持');
  return checkedSecrets(metadata.secrets || {});
}

async function verifyBackup(opts, password) {
  const decrypted = await decryptArchive(resolve(opts.in), password);
  try {
    validateMetadata(decrypted.metadata);
    await pipeline(createReadStream(decrypted.payloadPath, { start: decrypted.dumpOffset }), createGunzip(),
      new Writable({ write(_chunk, _encoding, callback) { callback(); } }));
    console.log('迁移包密码、文件完整性和压缩数据校验通过。');
  } finally { await unlink(decrypted.payloadPath).catch(() => {}); }
}

async function main() {
  const opts = options(process.argv.slice(2));
  if (opts.command === 'help') return usage();
  if (opts.command === 'check') {
    await checkSource(opts);
    console.log('数据库连接和可迁移密钥检查通过。');
    return;
  }
  const password = process.env.BACKUP_PASSPHRASE;
  if (!password) throw new Error('请通过 BACKUP_PASSPHRASE 环境变量提供备份密码');
  if (opts.command === 'export') await exportBackup(opts, password);
  else if (opts.command === 'verify') await verifyBackup(opts, password);
  else await importBackup(opts, password);
}

main().catch(error => { console.error(error.message); process.exitCode = 1; });
