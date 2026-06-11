#!/usr/bin/env node

import readline from "node:readline";

const args = process.argv.slice(2);
if (args.length === 0 || args[0].startsWith("-")) {
  console.error("usage: codex-mcp-sse-proxy <sse-url> [--header 'Name: Value']");
  process.exit(2);
}

const serverURL = args[0];
const headers = {
  Accept: "text/event-stream",
};

for (let i = 1; i < args.length; i += 1) {
  if (args[i] !== "--header" || i+1 >= args.length) {
    console.error(`unsupported argument: ${args[i]}`);
    process.exit(2);
  }
  const header = args[i+1];
  i += 1;
  const sep = header.indexOf(":");
  if (sep <= 0) {
    console.error(`invalid header argument: ${header}`);
    process.exit(2);
  }
  headers[header.slice(0, sep).trim()] = header.slice(sep + 1).trim();
}

let endpointURL = "";
let readyResolve;
let readyDone = false;
const ready = new Promise((resolve) => {
	readyResolve = resolve;
});

function markReady() {
	if (readyDone || endpointURL === "") {
		return;
	}
	readyDone = true;
	readyResolve();
}

function writeJSON(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`);
}

function rpcID(line) {
	try {
		const value = JSON.parse(line);
    if (Object.prototype.hasOwnProperty.call(value, "id")) {
      return value.id;
    }
  } catch {
    // Keep malformed input handling in the remote server path.
  }
	return undefined;
}

function rpcMethod(line) {
	try {
		const value = JSON.parse(line);
		return typeof value.method === "string" ? value.method : "";
	} catch {
		return "";
	}
}

function writeRPCError(id, message) {
  if (id === undefined) {
    console.error(message);
    return;
  }
  writeJSON({
    jsonrpc: "2.0",
    id,
    error: {
      code: -32000,
      message,
    },
  });
}

function consumeSSEBlock(block) {
  let event = "message";
  const data = [];
  for (const rawLine of block.split(/\r?\n/)) {
    if (rawLine === "" || rawLine.startsWith(":")) {
      continue;
    }
    const sep = rawLine.indexOf(":");
    const field = sep < 0 ? rawLine : rawLine.slice(0, sep);
    let value = sep < 0 ? "" : rawLine.slice(sep + 1);
    if (value.startsWith(" ")) {
      value = value.slice(1);
    }
    if (field === "event") {
      event = value;
    } else if (field === "data") {
      data.push(value);
    }
  }
  const payload = data.join("\n");
	if (event === "endpoint") {
		endpointURL = new URL(payload, serverURL).toString();
		setTimeout(markReady, 1000);
		return;
	}
	if (payload.trim().startsWith("{")) {
		try {
			const message = JSON.parse(payload);
			if (message?.method === "sse/connection") {
				markReady();
				return;
			}
		} catch {
			// Forward malformed remote payloads unchanged.
		}
		process.stdout.write(`${payload}\n`);
	}
}

async function connectSSE() {
  const response = await fetch(serverURL, { headers });
  if (!response.ok || !response.body) {
    throw new Error(`SSE connect failed with HTTP ${response.status}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      throw new Error("SSE stream closed");
    }
    buffer += decoder.decode(value, { stream: true });
    for (;;) {
      const match = buffer.match(/\r?\n\r?\n/);
      if (!match) {
        break;
      }
      const block = buffer.slice(0, match.index);
      buffer = buffer.slice(match.index + match[0].length);
      consumeSSEBlock(block);
    }
  }
}

async function postRPC(line) {
	await ready;
	let lastError;
	for (let attempt = 0; attempt < 20; attempt += 1) {
		const response = await fetch(endpointURL, {
			method: "POST",
			headers: {
				...headers,
				Accept: "application/json, text/event-stream",
				"Content-Type": "application/json",
			},
			body: line,
		});
		if (response.ok) {
			return;
		}
		const detail = await response.text().catch(() => "");
		lastError = new Error(`MCP POST failed with HTTP ${response.status}${detail ? `: ${detail}` : ""}`);
		if (response.status !== 400 || !detail.includes("No active transport")) {
			break;
		}
		await new Promise((resolve) => setTimeout(resolve, 250));
	}
	throw lastError;
}

connectSSE().catch((error) => {
  console.error(error?.message || String(error));
  process.exit(1);
});

const rl = readline.createInterface({
	input: process.stdin,
	crlfDelay: Infinity,
});

let queue = Promise.resolve();

rl.on("line", (line) => {
	const trimmed = line.trim();
	if (trimmed === "") {
		return;
	}
	const id = rpcID(trimmed);
	const method = rpcMethod(trimmed);
	queue = queue.then(async () => {
		if (method === "notifications/initialized") {
			return;
		}
		await postRPC(trimmed);
	}).catch((error) => {
		writeRPCError(id, error?.message || String(error));
	});
});

rl.on("close", () => {
  process.exit(0);
});
