import http from "node:http"
import { randomUUID } from "node:crypto"

const port = numberEnv("PORT", 8788)
const upstreamBaseUrl = requiredEnv("SUB2API_IMAGE_JOB_UPSTREAM_BASE_URL").replace(/\/+$/, "")
const upstreamApiKey = process.env.SUB2API_IMAGE_JOB_UPSTREAM_API_KEY || ""
const proxyApiKey = process.env.SUB2API_IMAGE_JOB_PROXY_API_KEY || ""
const allowClientUpstreamAuth = boolEnv("SUB2API_IMAGE_JOB_ALLOW_CLIENT_UPSTREAM_AUTH", false)
const requestTimeoutMs = numberEnv("SUB2API_IMAGE_JOB_REQUEST_TIMEOUT_MS", 15 * 60 * 1000)
const retentionMs = numberEnv("SUB2API_IMAGE_JOB_RETENTION_MS", 24 * 60 * 60 * 1000)
const maxBodyBytes = numberEnv("SUB2API_IMAGE_JOB_MAX_BODY_BYTES", 80 * 1024 * 1024)
const maxJobs = numberEnv("SUB2API_IMAGE_JOB_MAX_JOBS", 100)

const allowedEndpoints = new Set(["/v1/images/generations", "/v1/images/edits"])
const jobs = new Map()

function numberEnv(name, fallback) {
  const raw = process.env[name]
  if (!raw) return fallback
  const parsed = Number.parseInt(raw, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

function boolEnv(name, fallback) {
  const raw = process.env[name]
  if (raw == null || raw === "") return fallback
  return ["1", "true", "yes", "on"].includes(raw.toLowerCase())
}

function requiredEnv(name) {
  const value = process.env[name]
  if (!value) throw new Error(`${name} is required`)
  return value
}

function writeJson(res, status, payload) {
  res.writeHead(status, { "Content-Type": "application/json; charset=utf-8" })
  res.end(JSON.stringify(payload))
}

function requireProxyAuth(req, res) {
  if (!proxyApiKey) return true
  if (req.headers.authorization === `Bearer ${proxyApiKey}`) return true
  writeJson(res, 401, { error: { code: "unauthorized", message: "Invalid sidecar API key" } })
  return false
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = []
    let total = 0
    req.on("data", (chunk) => {
      total += chunk.length
      if (total > maxBodyBytes) {
        req.destroy()
        reject(new Error(`Request body exceeds ${maxBodyBytes} bytes`))
        return
      }
      chunks.push(chunk)
    })
    req.on("end", () => resolve(Buffer.concat(chunks)))
    req.on("error", reject)
  })
}

function normalizeEndpoint(endpoint) {
  const value = String(endpoint || "").trim()
  if (value === "/images/generations") return "/v1/images/generations"
  if (value === "/images/edits") return "/v1/images/edits"
  if (!allowedEndpoints.has(value)) throw new Error(`Unsupported endpoint: ${value}`)
  return value
}

function inferContentType(filename, fallback = "application/octet-stream") {
  const lower = String(filename || "").toLowerCase()
  if (lower.endsWith(".png")) return "image/png"
  if (lower.endsWith(".jpg") || lower.endsWith(".jpeg")) return "image/jpeg"
  if (lower.endsWith(".webp")) return "image/webp"
  return fallback
}

function appendFormField(form, key, value) {
  if (value == null) return
  form.append(key, typeof value === "object" ? JSON.stringify(value) : String(value))
}

function upstreamAuthorization(req) {
  if (upstreamApiKey) return `Bearer ${upstreamApiKey}`
  if (!allowClientUpstreamAuth) return ""
  const forwarded = req.headers["x-upstream-authorization"]
  return Array.isArray(forwarded) ? forwarded[0] || "" : forwarded || ""
}

function publicJob(job) {
  const payload = {
    job_id: job.job_id,
    status: job.status,
    created_at: job.created_at,
    started_at: job.started_at,
    completed_at: job.completed_at,
    endpoint: job.endpoint,
    model: job.model,
    size: job.size,
    reference_count: job.reference_count,
    http_status: job.http_status,
    upstream_request_id: job.upstream_request_id,
    error: job.error,
  }
  if (job.status === "succeeded") payload.result = job.result
  return payload
}

function pruneJobs() {
  const cutoff = Date.now() - retentionMs
  for (const [jobId, job] of jobs.entries()) {
    if (Date.parse(job.created_at) < cutoff && ["succeeded", "failed", "cancelled"].includes(job.status)) {
      jobs.delete(jobId)
    }
  }
  if (jobs.size <= maxJobs) return
  const removable = [...jobs.values()]
    .filter((job) => ["succeeded", "failed", "cancelled"].includes(job.status))
    .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at))
  for (const job of removable) {
    if (jobs.size <= maxJobs) break
    jobs.delete(job.job_id)
  }
}

async function createJob(req, res) {
  const auth = upstreamAuthorization(req)
  if (!auth) {
    writeJson(res, 500, {
      error: {
        code: "upstream_auth_missing",
        message: "Configure SUB2API_IMAGE_JOB_UPSTREAM_API_KEY or allow X-Upstream-Authorization.",
      },
    })
    return
  }

  const raw = await readBody(req)
  const payload = JSON.parse(raw.toString("utf-8"))
  const endpoint = normalizeEndpoint(payload.endpoint)
  const request = payload.request && typeof payload.request === "object" ? payload.request : null
  if (!request) throw new Error("request object is required")
  const references = Array.isArray(payload.references) ? payload.references : []
  const mask = payload.mask && typeof payload.mask === "object" ? payload.mask : null

  const job = {
    job_id: randomUUID(),
    status: "queued",
    created_at: new Date().toISOString(),
    started_at: null,
    completed_at: null,
    endpoint,
    model: String(request.model || ""),
    size: String(request.size || ""),
    reference_count: references.length + (mask ? 1 : 0),
    http_status: null,
    upstream_request_id: "",
    result: null,
    error: null,
  }
  jobs.set(job.job_id, job)
  pruneJobs()

  writeJson(res, 202, { job_id: job.job_id, status: job.status, poll_url: `/image-jobs/${job.job_id}` })
  setImmediate(() => runJob(job.job_id, { endpoint, request, references, mask, auth }).catch((error) => markFailed(job.job_id, error)))
}

async function runJob(jobId, input) {
  const job = jobs.get(jobId)
  if (!job) return
  job.status = "running"
  job.started_at = new Date().toISOString()

  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), requestTimeoutMs)
  try {
    const headers = { Authorization: input.auth, Accept: "application/json", "User-Agent": "Sub2APIImageJobSidecar/0.1" }
    let body
    if (input.references.length > 0 || input.mask || input.endpoint.endsWith("/edits")) {
      const form = new FormData()
      for (const [key, value] of Object.entries(input.request)) appendFormField(form, key, value)
      for (const reference of input.references) {
        const filename = String(reference.filename || "reference.png")
        const contentType = String(reference.content_type || inferContentType(filename))
        const bytes = Buffer.from(String(reference.b64 || ""), "base64")
        form.append(reference.field_name || "image", new Blob([bytes], { type: contentType }), filename)
      }
      if (input.mask) {
        const filename = String(input.mask.filename || "mask.png")
        const contentType = String(input.mask.content_type || inferContentType(filename, "image/png"))
        const bytes = Buffer.from(String(input.mask.b64 || ""), "base64")
        form.append(input.mask.field_name || "mask", new Blob([bytes], { type: contentType }), filename)
      }
      body = form
    } else {
      headers["Content-Type"] = "application/json"
      body = JSON.stringify(input.request)
    }

    const response = await fetch(`${upstreamBaseUrl}${input.endpoint}`, { method: "POST", headers, body, signal: controller.signal })
    job.http_status = response.status
    job.upstream_request_id = response.headers.get("x-request-id") || ""
    const text = await response.text()
    try {
      job.result = text ? JSON.parse(text) : {}
    } catch {
      job.result = { raw_text: text }
    }
    if (!response.ok) throw new Error(`Upstream returned ${response.status}: ${text.slice(0, 1200)}`)
    job.status = "succeeded"
    job.completed_at = new Date().toISOString()
  } catch (error) {
    markFailed(jobId, error)
  } finally {
    clearTimeout(timeout)
  }
}

function markFailed(jobId, error) {
  const job = jobs.get(jobId)
  if (!job) return
  job.status = "failed"
  job.completed_at = new Date().toISOString()
  job.error = {
    code: error?.name === "AbortError" ? "upstream_timeout" : "upstream_error",
    message: error?.message || "Image job failed",
  }
}

function routeJobById(req, res, pathname) {
  const jobId = pathname.slice("/image-jobs/".length)
  const job = jobs.get(jobId)
  if (!job) {
    writeJson(res, 404, { error: { code: "not_found", message: "Job not found" } })
    return
  }
  if (req.method === "GET") {
    writeJson(res, 200, publicJob(job))
    return
  }
  if (req.method === "DELETE") {
    jobs.delete(jobId)
    writeJson(res, 200, { job_id: jobId, deleted: true })
    return
  }
  writeJson(res, 405, { error: { code: "method_not_allowed", message: "Method not allowed" } })
}

const server = http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url || "/", `http://${req.headers.host || "localhost"}`)
    if (req.method === "GET" && url.pathname === "/healthz") {
      writeJson(res, 200, { ok: true, jobs: jobs.size, upstream_base_url: upstreamBaseUrl })
      return
    }
    if (!requireProxyAuth(req, res)) return
    if (req.method === "POST" && url.pathname === "/image-jobs") {
      await createJob(req, res)
      return
    }
    if (url.pathname.startsWith("/image-jobs/")) {
      routeJobById(req, res, url.pathname)
      return
    }
    writeJson(res, 404, { error: { code: "not_found", message: "Route not found" } })
  } catch (error) {
    writeJson(res, 400, { error: { code: "bad_request", message: error?.message || "Bad request" } })
  }
})

server.listen(port, () => {
  console.log(`OpenAI image job sidecar listening on ${port}`)
})
