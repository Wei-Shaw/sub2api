import { createCipheriv, createDecipheriv, randomBytes, scryptSync } from 'node:crypto';
import { createReadStream, createWriteStream } from 'node:fs';
import { open, unlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Transform } from 'node:stream';
import { pipeline } from 'node:stream/promises';

const MAGIC = Buffer.from('AGPORT01');
const HEADER_SIZE = MAGIC.length + 16 + 12;
const TAG_SIZE = 16;
const MAX_METADATA = 16 * 1024;
const KDF = { N: 1 << 15, r: 8, p: 1, maxmem: 128 * 1024 * 1024 };

function keyFor(passphrase, salt) {
  if (typeof passphrase !== 'string' || passphrase.length < 12) {
    throw new Error('备份密码至少需要 12 个字符');
  }
  return scryptSync(passphrase, salt, 32, KDF);
}

function metadataBytes(metadata) {
  const encoded = Buffer.from(JSON.stringify(metadata));
  if (encoded.length > MAX_METADATA) throw new Error('备份元数据过大');
  const length = Buffer.alloc(4);
  length.writeUInt32BE(encoded.length);
  return Buffer.concat([length, encoded]);
}

function cryptTransform(cipher) {
  return new Transform({
    transform(chunk, _, callback) {
      try { callback(null, cipher.update(chunk)); }
      catch (error) { callback(error); }
    },
    flush(callback) {
      try { callback(null, cipher.final()); }
      catch { callback(new Error('备份密码错误或文件已损坏')); }
    },
  });
}

// The header is authenticated as AAD. The encrypted payload starts with a
// bounded JSON metadata block, followed by a gzip-compressed pg_dump stream.
export async function writeArchive(filePath, passphrase, metadata, compressedDump) {
  const salt = randomBytes(16);
  const iv = randomBytes(12);
  const header = Buffer.concat([MAGIC, salt, iv]);
  const key = keyFor(passphrase, salt);
  const cipher = createCipheriv('aes-256-gcm', key, iv);
  key.fill(0);
  cipher.setAAD(header);
  const destination = createWriteStream(filePath, { flags: 'wx', mode: 0o600 });
  try {
    destination.write(header);
    destination.write(cipher.update(metadataBytes(metadata)));
    await pipeline(compressedDump, new Transform({
      transform(chunk, _, callback) {
        try { callback(null, cipher.update(chunk)); }
        catch (error) { callback(error); }
      },
      flush(callback) {
        try { callback(null, Buffer.concat([cipher.final(), cipher.getAuthTag()])); }
        catch (error) { callback(error); }
      },
    }), destination);
  } catch (error) {
    destination.destroy();
    await unlink(filePath).catch(() => {});
    throw error;
  }
}

async function readExactly(handle, bytes, position) {
  const buffer = Buffer.alloc(bytes);
  let offset = 0;
  while (offset < bytes) {
    const { bytesRead } = await handle.read(buffer, offset, bytes - offset, position + offset);
    if (!bytesRead) throw new Error('备份文件不完整');
    offset += bytesRead;
  }
  return buffer;
}

export async function decryptArchive(filePath, passphrase) {
  const source = await open(filePath, 'r');
  let tempPath;
  try {
    const stat = await source.stat();
    if (stat.size < HEADER_SIZE + TAG_SIZE + 4) throw new Error('备份文件不完整');
    const header = await readExactly(source, HEADER_SIZE, 0);
    if (!header.subarray(0, MAGIC.length).equals(MAGIC)) throw new Error('不是支持的迁移备份文件');
    const tag = await readExactly(source, TAG_SIZE, stat.size - TAG_SIZE);
    const key = keyFor(passphrase, header.subarray(MAGIC.length, MAGIC.length + 16));
    const decipher = createDecipheriv('aes-256-gcm', key, header.subarray(MAGIC.length + 16));
    key.fill(0);
    decipher.setAAD(header);
    decipher.setAuthTag(tag);
    tempPath = join(tmpdir(), `astrogate-restore-${process.pid}-${randomBytes(12).toString('hex')}`);
    await pipeline(
      createReadStream(filePath, { start: HEADER_SIZE, end: stat.size - TAG_SIZE - 1 }),
      cryptTransform(decipher),
      createWriteStream(tempPath, { flags: 'wx', mode: 0o600 }),
    );
    const plaintext = await open(tempPath, 'r');
    try {
      const size = (await plaintext.stat()).size;
      const length = (await readExactly(plaintext, 4, 0)).readUInt32BE();
      if (length < 2 || length > MAX_METADATA || size <= 4 + length) throw new Error('备份内容格式无效');
      const metadata = JSON.parse((await readExactly(plaintext, length, 4)).toString('utf8'));
      return { metadata, payloadPath: tempPath, dumpOffset: 4 + length };
    } finally { await plaintext.close(); }
  } catch (error) {
    if (tempPath) await unlink(tempPath).catch(() => {});
    throw error;
  } finally { await source.close(); }
}
