import fnmatch
import hashlib
import socket
import threading
import time

HOST = "127.0.0.1"
PORT = 6379

store = {}
expire_at = {}
scripts = {}
subscribers = {}
lock = threading.RLock()


def now():
    return time.time()


def to_text(value):
    if isinstance(value, bytes):
        return value.decode(errors="ignore")
    return str(value)


def to_bytes(value):
    if value is None:
        return b""
    if isinstance(value, bytes):
        return value
    return str(value).encode()


def cleanup_key(key):
    deadline = expire_at.get(key)
    if deadline is not None and deadline <= now():
        store.pop(key, None)
        expire_at.pop(key, None)
        return True
    return False


def cleanup_all():
    for key in list(expire_at.keys()):
        cleanup_key(key)


def get_value(key, default_type="string"):
    cleanup_key(key)
    value = store.get(key)
    if value is None:
        if default_type == "set":
            value = set()
        elif default_type == "hash":
            value = {}
        elif default_type == "list":
            value = []
        elif default_type == "zset":
            value = {}
        else:
            value = b""
        store[key] = value
    return value


def encode(value):
    if value is None:
        return b"$-1\r\n"
    if isinstance(value, bytes):
        return b"$" + str(len(value)).encode() + b"\r\n" + value + b"\r\n"
    if isinstance(value, str):
        data = value.encode()
        return b"$" + str(len(data)).encode() + b"\r\n" + data + b"\r\n"
    if isinstance(value, bool):
        return integer(1 if value else 0)
    if isinstance(value, int):
        return integer(value)
    if isinstance(value, float):
        data = ("%f" % value).rstrip("0").rstrip(".").encode()
        return b"$" + str(len(data)).encode() + b"\r\n" + data + b"\r\n"
    if isinstance(value, (list, tuple)):
        out = b"*" + str(len(value)).encode() + b"\r\n"
        for item in value:
            out += encode(item)
        return out
    return b"+OK\r\n"


def simple(text):
    return ("+" + text + "\r\n").encode()


def error(text):
    return ("-ERR " + text + "\r\n").encode()


def integer(number):
    return (":" + str(int(number)) + "\r\n").encode()


def read_line(conn):
    data = b""
    while not data.endswith(b"\r\n"):
        chunk = conn.recv(1)
        if not chunk:
            return None
        data += chunk
    return data[:-2]


def read_command(conn):
    first = read_line(conn)
    if first is None:
        return None
    if not first:
        return []
    if first.startswith(b"*"):
        count = int(first[1:] or b"0")
        parts = []
        for _ in range(count):
            length_line = read_line(conn)
            if length_line is None or not length_line.startswith(b"$"):
                return None
            length = int(length_line[1:])
            if length < 0:
                parts.append(None)
                continue
            data = b""
            while len(data) < length + 2:
                chunk = conn.recv(length + 2 - len(data))
                if not chunk:
                    return None
                data += chunk
            parts.append(data[:length])
        return parts
    return first.split()


def scan_keys(pattern="*", count=100):
    cleanup_all()
    keys = []
    for key in list(store.keys()):
        if cleanup_key(key):
            continue
        name = to_text(key)
        if fnmatch.fnmatch(name, pattern):
            keys.append(key)
        if len(keys) >= count:
            break
    return keys


def handle_script(args):
    if not args:
        return error("wrong number of arguments for SCRIPT")
    sub = to_text(args[0]).upper()
    if sub == "LOAD" and len(args) >= 2:
        body = args[1]
        sha = hashlib.sha1(body).hexdigest().encode()
        scripts[sha] = body
        return encode(sha)
    if sub == "EXISTS":
        return encode([1 if sha in scripts else 0 for sha in args[1:]])
    if sub == "FLUSH":
        scripts.clear()
        return simple("OK")
    return simple("OK")


def handle_eval(args):
    if len(args) < 2:
        return error("wrong number of arguments for EVAL")

    # Minimal Lua compatibility for go-redis callers. Sub2API uses scripts for
    # rate-limit, concurrency slots, and cache helpers. Reply types matter:
    # user-slot/rate-limit scripts are parsed as arrays; some helpers want int.
    try:
        key_count = int(to_text(args[1]))
    except Exception:
        key_count = 0
    keys = args[2:2 + max(0, key_count)]
    argv = args[2 + max(0, key_count):]
    key_text = " ".join(to_text(k) for k in keys).lower()
    script_text = to_text(args[0]).lower() if args else ""

    def key_matches(*needles):
        return any(n in key_text for n in needles) or any(n in script_text for n in needles)

    if key_matches("rate_limit:", "ratelimit", "login_rate", "auth_rate"):
        # Shape: [allowed, remaining, retry_after]
        return encode([1, 60, 0])

    if key_matches(
        "user_slot",
        "userslot",
        "concurrency:user",
        "concurrency:account",
        "account:active_index",
        "slot",
        "acquire",
        "release",
    ):
        # Concurrency/user-slot scripts are parsed as redis script arrays.
        # Returning int64 causes: expected redis script array, got int64 -> 503.
        token = argv[0] if argv else b"1"
        return encode([1, token, 0])

    if key_matches("umq", "lock_index", "active_index", "wait_index", "cleanup"):
        # Background workers often scan indexes as arrays.
        return encode([])

    # Leader-election locks: implement a minimal SET NX / renew path so a
    # single local instance can own ops/metrics collectors after restart.
    if key_matches("leader:lock:", ":leader", "ops:metrics", "ops:aggregation", "ops:scheduled"):
        key = keys[0] if keys else None
        token = argv[0] if argv else b"1"
        # ARGV may be ms or seconds depending on client; treat large values as ms.
        ttl_raw = 30.0
        if len(argv) >= 2:
            try:
                ttl_raw = float(to_text(argv[1]))
            except Exception:
                ttl_raw = 30.0
        ttl = ttl_raw / 1000.0 if ttl_raw > 1000 else ttl_raw
        if key is None:
            return integer(0)

        cleanup_key(key)
        current = store.get(key)
        script_l = script_text
        releasing = any(w in script_l for w in ("del", "release", "unlock", "delete"))
        renewing = any(w in script_l for w in ("pexpire", "expire", "renew", "extend", "keepalive"))

        if releasing:
            if current is not None and to_bytes(current) == to_bytes(token):
                store.pop(key, None)
                expire_at.pop(key, None)
                return integer(1)
            return integer(0)

        if current is None:
            store[key] = to_bytes(token)
            expire_at[key] = now() + max(ttl, 1.0)
            return integer(1)

        if to_bytes(current) == to_bytes(token):
            expire_at[key] = now() + max(ttl, 1.0)
            return integer(1)

        if renewing:
            return integer(0)

        # Default for unknown lock scripts: do not steal foreign locks.
        return integer(0)

    if key_matches("sched:", "cache:", "pricing"):
        return integer(1)

    # Default array reply is safer for go-redis SliceCmd callers.
    return encode([1, 0, 0])


def handle(parts):
    if not parts:
        return simple("OK")
    cmd = to_text(parts[0]).upper()
    args = parts[1:]

    with lock:
        if cmd == "PING":
            return encode(args[0] if args else b"PONG")
        if cmd in ("AUTH", "SELECT", "READONLY", "READWRITE", "RESET"):
            return simple("OK")
        if cmd == "HELLO":
            return encode([b"server", b"mini-redis", b"version", b"0.3", b"proto", 2])
        if cmd == "CLIENT":
            return simple("OK")
        if cmd == "QUIT":
            return simple("OK")
        if cmd == "DBSIZE":
            cleanup_all()
            return integer(len(store))
        if cmd == "TIME":
            current = time.time()
            seconds = int(current)
            microseconds = int((current - seconds) * 1000000)
            return encode([str(seconds).encode(), str(microseconds).encode()])
        if cmd == "TYPE":
            key = args[0]
            cleanup_key(key)
            value = store.get(key)
            if value is None:
                return simple("none")
            if isinstance(value, set):
                return simple("set")
            if isinstance(value, dict):
                return simple("hash")
            if isinstance(value, list):
                return simple("list")
            return simple("string")

        if cmd == "SET":
            if len(args) < 2:
                return error("wrong number of arguments for SET")
            key, value = args[0], args[1]
            nx = False
            xx = False
            ttl = None
            idx = 2
            while idx < len(args):
                opt = to_text(args[idx]).upper()
                if opt == "NX":
                    nx = True
                    idx += 1
                elif opt == "XX":
                    xx = True
                    idx += 1
                elif opt in ("EX", "PX") and idx + 1 < len(args):
                    ttl = int(args[idx + 1])
                    if opt == "PX":
                        ttl = ttl / 1000
                    idx += 2
                else:
                    idx += 1
            exists = not cleanup_key(key) and key in store
            if nx and exists:
                return encode(None)
            if xx and not exists:
                return encode(None)
            store[key] = value
            expire_at.pop(key, None)
            if ttl is not None:
                expire_at[key] = now() + ttl
            return simple("OK")
        if cmd == "SETNX":
            key, value = args[0], args[1]
            exists = not cleanup_key(key) and key in store
            if exists:
                return integer(0)
            store[key] = value
            return integer(1)
        if cmd == "GET":
            key = args[0]
            cleanup_key(key)
            value = store.get(key)
            if isinstance(value, bytes):
                return encode(value)
            return encode(None)
        if cmd == "MGET":
            values = []
            for key in args:
                cleanup_key(key)
                value = store.get(key)
                values.append(value if isinstance(value, bytes) else None)
            return encode(values)
        if cmd == "MSET":
            for idx in range(0, len(args) - 1, 2):
                store[args[idx]] = args[idx + 1]
                expire_at.pop(args[idx], None)
            return simple("OK")
        if cmd == "GETDEL":
            key = args[0]
            cleanup_key(key)
            value = store.pop(key, None)
            expire_at.pop(key, None)
            return encode(value if isinstance(value, bytes) else None)
        if cmd == "DEL":
            count = 0
            for key in args:
                if key in store:
                    count += 1
                store.pop(key, None)
                expire_at.pop(key, None)
            return integer(count)
        if cmd == "UNLINK":
            count = 0
            for key in args:
                if key in store:
                    count += 1
                store.pop(key, None)
                expire_at.pop(key, None)
            return integer(count)
        if cmd == "EXISTS":
            count = 0
            for key in args:
                if not cleanup_key(key) and key in store:
                    count += 1
            return integer(count)
        if cmd in ("EXPIRE", "PEXPIRE"):
            key = args[0]
            cleanup_key(key)
            if key not in store:
                return integer(0)
            ttl = int(args[1])
            if cmd == "PEXPIRE":
                ttl = ttl / 1000
            expire_at[key] = now() + ttl
            return integer(1)
        if cmd in ("EXPIREAT", "PEXPIREAT"):
            key = args[0]
            cleanup_key(key)
            if key not in store:
                return integer(0)
            ts = int(args[1])
            if cmd == "PEXPIREAT":
                ts = ts / 1000
            expire_at[key] = ts
            return integer(1)
        if cmd == "PERSIST":
            key = args[0]
            existed = key in expire_at
            expire_at.pop(key, None)
            return integer(1 if existed else 0)
        if cmd in ("TTL", "PTTL"):
            key = args[0]
            cleanup_key(key)
            if key not in store:
                return integer(-2)
            if key not in expire_at:
                return integer(-1)
            remaining = max(0, expire_at[key] - now())
            if cmd == "PTTL":
                remaining *= 1000
            return integer(remaining)
        if cmd == "INCR":
            key = args[0]
            cleanup_key(key)
            value = int((store.get(key) or b"0").decode()) + 1
            store[key] = str(value).encode()
            return integer(value)
        if cmd == "DECR":
            key = args[0]
            cleanup_key(key)
            value = int((store.get(key) or b"0").decode()) - 1
            store[key] = str(value).encode()
            return integer(value)
        if cmd == "INCRBY":
            key = args[0]
            amount = int(args[1])
            cleanup_key(key)
            value = int((store.get(key) or b"0").decode()) + amount
            store[key] = str(value).encode()
            return integer(value)
        if cmd == "DECRBY":
            key = args[0]
            amount = int(args[1])
            cleanup_key(key)
            value = int((store.get(key) or b"0").decode()) - amount
            store[key] = str(value).encode()
            return integer(value)
        if cmd == "KEYS":
            pattern = to_text(args[0]) if args else "*"
            return encode(scan_keys(pattern, 1000000))
        if cmd == "SCAN":
            pattern = "*"
            count = 100
            idx = 1
            while idx < len(args):
                opt = to_text(args[idx]).upper()
                if opt == "MATCH" and idx + 1 < len(args):
                    pattern = to_text(args[idx + 1])
                    idx += 2
                elif opt == "COUNT" and idx + 1 < len(args):
                    count = int(args[idx + 1])
                    idx += 2
                else:
                    idx += 1
            return encode([b"0", scan_keys(pattern, count)])
        if cmd == "FLUSHDB" or cmd == "FLUSHALL":
            store.clear()
            expire_at.clear()
            return simple("OK")

        if cmd == "SADD":
            key = args[0]
            s = get_value(key, "set")
            if not isinstance(s, set):
                return error("WRONGTYPE Operation against a key holding the wrong kind of value")
            added = 0
            for member in args[1:]:
                if member not in s:
                    s.add(member)
                    added += 1
            return integer(added)
        if cmd == "SREM":
            key = args[0]
            cleanup_key(key)
            s = store.get(key, set())
            removed = 0
            if isinstance(s, set):
                for member in args[1:]:
                    if member in s:
                        s.remove(member)
                        removed += 1
            return integer(removed)
        if cmd == "SMEMBERS":
            key = args[0]
            cleanup_key(key)
            s = store.get(key, set())
            return encode(list(s) if isinstance(s, set) else [])
        if cmd == "SCARD":
            key = args[0]
            cleanup_key(key)
            s = store.get(key, set())
            return integer(len(s) if isinstance(s, set) else 0)
        if cmd == "SISMEMBER":
            key, member = args[0], args[1]
            cleanup_key(key)
            s = store.get(key, set())
            return integer(1 if isinstance(s, set) and member in s else 0)
        if cmd == "SPOP":
            key = args[0]
            count = int(args[1]) if len(args) > 1 else None
            cleanup_key(key)
            s = store.get(key, set())
            if not isinstance(s, set) or not s:
                return encode([] if count is not None else None)
            if count is None:
                member = next(iter(s))
                s.remove(member)
                return encode(member)
            out = []
            for _ in range(min(count, len(s))):
                member = next(iter(s))
                s.remove(member)
                out.append(member)
            return encode(out)

        if cmd == "HSET":
            key = args[0]
            h = get_value(key, "hash")
            if not isinstance(h, dict):
                return error("WRONGTYPE Operation against a key holding the wrong kind of value")
            added = 0
            for idx in range(1, len(args) - 1, 2):
                field, value = args[idx], args[idx + 1]
                if field not in h:
                    added += 1
                h[field] = value
            return integer(added)
        if cmd == "HGET":
            key, field = args[0], args[1]
            cleanup_key(key)
            h = store.get(key, {})
            return encode(h.get(field) if isinstance(h, dict) else None)
        if cmd == "HMGET":
            key = args[0]
            cleanup_key(key)
            h = store.get(key, {})
            return encode([h.get(field) if isinstance(h, dict) else None for field in args[1:]])
        if cmd == "HGETALL":
            key = args[0]
            cleanup_key(key)
            h = store.get(key, {})
            out = []
            if isinstance(h, dict):
                for field, value in h.items():
                    out.extend([field, value])
            return encode(out)
        if cmd == "HDEL":
            key = args[0]
            cleanup_key(key)
            h = store.get(key, {})
            removed = 0
            if isinstance(h, dict):
                for field in args[1:]:
                    if field in h:
                        removed += 1
                        h.pop(field, None)
            return integer(removed)
        if cmd == "HINCRBY":
            key, field, amount = args[0], args[1], int(args[2])
            h = get_value(key, "hash")
            current = int((h.get(field) or b"0").decode()) + amount
            h[field] = str(current).encode()
            return integer(current)

        if cmd in ("LPUSH", "RPUSH"):
            key = args[0]
            lst = get_value(key, "list")
            if not isinstance(lst, list):
                return error("WRONGTYPE Operation against a key holding the wrong kind of value")
            for value in args[1:]:
                if cmd == "LPUSH":
                    lst.insert(0, value)
                else:
                    lst.append(value)
            return integer(len(lst))
        if cmd in ("LPOP", "RPOP"):
            key = args[0]
            cleanup_key(key)
            lst = store.get(key, [])
            if not isinstance(lst, list) or not lst:
                return encode(None)
            return encode(lst.pop(0) if cmd == "LPOP" else lst.pop())
        if cmd == "LLEN":
            key = args[0]
            cleanup_key(key)
            lst = store.get(key, [])
            return integer(len(lst) if isinstance(lst, list) else 0)

        if cmd == "ZADD":
            key = args[0]
            z = get_value(key, "zset")
            if not isinstance(z, dict):
                return error("WRONGTYPE Operation against a key holding the wrong kind of value")
            added = 0
            idx = 1
            while idx + 1 < len(args):
                try:
                    score = float(to_text(args[idx]))
                    member = args[idx + 1]
                    idx += 2
                except ValueError:
                    idx += 1
                    continue
                if member not in z:
                    added += 1
                z[member] = score
            return integer(added)
        if cmd == "ZREM":
            key = args[0]
            cleanup_key(key)
            z = store.get(key, {})
            removed = 0
            if isinstance(z, dict):
                for member in args[1:]:
                    if member in z:
                        removed += 1
                        z.pop(member, None)
            return integer(removed)
        if cmd == "ZINCRBY":
            key = args[0]
            amount = float(to_text(args[1]))
            member = args[2]
            z = get_value(key, "zset")
            if not isinstance(z, dict):
                return error("WRONGTYPE Operation against a key holding the wrong kind of value")
            z[member] = float(z.get(member, 0)) + amount
            return encode(z[member])
        if cmd == "ZSCORE":
            key, member = args[0], args[1]
            cleanup_key(key)
            z = store.get(key, {})
            if not isinstance(z, dict) or member not in z:
                return encode(None)
            return encode(z[member])
        if cmd == "ZCOUNT":
            key = args[0]
            min_score = float("-inf") if to_text(args[1]) == "-inf" else float(to_text(args[1]).lstrip("("))
            max_score = float("inf") if to_text(args[2]) == "+inf" else float(to_text(args[2]).lstrip("("))
            cleanup_key(key)
            z = store.get(key, {})
            if not isinstance(z, dict):
                return integer(0)
            return integer(sum(1 for score in z.values() if min_score <= score <= max_score))
        if cmd == "ZREMRANGEBYSCORE":
            key = args[0]
            min_score = float("-inf") if to_text(args[1]) == "-inf" else float(to_text(args[1]).lstrip("("))
            max_score = float("inf") if to_text(args[2]) == "+inf" else float(to_text(args[2]).lstrip("("))
            cleanup_key(key)
            z = store.get(key, {})
            removed = 0
            if isinstance(z, dict):
                for member, score in list(z.items()):
                    if min_score <= score <= max_score:
                        z.pop(member, None)
                        removed += 1
            return integer(removed)
        if cmd in ("ZRANGE", "ZREVRANGE"):
            key = args[0]
            start = int(args[1]) if len(args) > 1 else 0
            stop = int(args[2]) if len(args) > 2 else -1
            cleanup_key(key)
            z = store.get(key, {})
            if not isinstance(z, dict):
                return encode([])
            items = sorted(z.items(), key=lambda item: item[1], reverse=(cmd == "ZREVRANGE"))
            if stop < 0:
                stop = len(items) + stop
            sliced = items[start:stop + 1]
            return encode([member for member, _score in sliced])
        if cmd == "ZRANGEBYSCORE":
            # Sub2API concurrency cleanup uses ZRANGEBYSCORE on active indexes.
            # Falling through to +OK makes go-redis fail with:
            # can't parse array/set/push reply: "+OK"
            if len(args) < 3:
                return error("wrong number of arguments for ZRANGEBYSCORE")
            key = args[0]
            min_raw = to_text(args[1])
            max_raw = to_text(args[2])
            min_score = float("-inf") if min_raw == "-inf" else float(min_raw.lstrip("("))
            max_score = float("inf") if max_raw == "+inf" else float(max_raw.lstrip("("))
            with_scores = any(to_text(a).upper() == "WITHSCORES" for a in args[3:])
            cleanup_key(key)
            z = store.get(key, {})
            if not isinstance(z, dict):
                return encode([])
            items = sorted(
                ((member, score) for member, score in z.items() if min_score <= score <= max_score),
                key=lambda item: item[1],
            )
            if with_scores:
                out = []
                for member, score in items:
                    out.append(member)
                    out.append(score)
                return encode(out)
            return encode([member for member, _score in items])
        if cmd == "ZCARD":
            key = args[0]
            cleanup_key(key)
            z = store.get(key, {})
            return integer(len(z) if isinstance(z, dict) else 0)

        if cmd == "SCRIPT":
            return handle_script(args)
        if cmd in ("EVAL", "EVALSHA"):
            return handle_eval(args)
        if cmd == "PUBLISH":
            return integer(0)
        if cmd == "SUBSCRIBE":
            channel = args[0] if args else b""
            return encode([b"subscribe", channel, 1])
        if cmd == "UNSUBSCRIBE":
            channel = args[0] if args else b""
            return encode([b"unsubscribe", channel, 0])

        # Unknown command: return empty array for scan-like callers and integer 0
        # for counters. Avoid bare +OK — go-redis fails parsing it as Slice/Int.
        if cmd.endswith("RANGE") or cmd.endswith("MEMBERS") or cmd in ("KEYS", "SCAN"):
            return encode([])
        return integer(0)


def client_thread(conn):
    try:
        while True:
            parts = read_command(conn)
            if parts is None:
                return
            response = handle(parts)
            conn.sendall(response)
            if parts and to_text(parts[0]).upper() == "QUIT":
                return
    except Exception as exc:
        try:
            conn.sendall(error(str(exc)))
        except Exception:
            pass
    finally:
        try:
            conn.close()
        except Exception:
            pass


def main():
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind((HOST, PORT))
    server.listen(128)
    print(f"mini-redis listening on {HOST}:{PORT}", flush=True)
    while True:
        conn, _ = server.accept()
        threading.Thread(target=client_thread, args=(conn,), daemon=True).start()


if __name__ == "__main__":
    main()
