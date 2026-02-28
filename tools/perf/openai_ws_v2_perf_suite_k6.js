import http from 'k6/http';
import { check, sleep REDACTED from 'k6';
import { Rate, Trend REDACTED from 'k6/metrics';

const baseURL = (__ENV.BASE_URL || 'http://127.0.0.1:5231').replace(/\/$/, '');
const wsAPIKey = (__ENV.WS_API_KEY || '').trim();
const wsHotspotAPIKey = (__ENV.WS_HOTSPOT_API_KEY || wsAPIKey).trim();
const model = __ENV.MODEL || 'gpt-5.3-codex';
const duration = __ENV.DURATION || '5m';
const timeout = __ENV.TIMEOUT || '180s';

const shortRPS = Number(__ENV.SHORT_RPS || 12);
const longRPS = Number(__ENV.LONG_RPS || 4);
const errorRPS = Number(__ENV.ERROR_RPS || 2);
const hotspotRPS = Number(__ENV.HOTSPOT_RPS || 10);
const preAllocatedVUs = Number(__ENV.PRE_ALLOCATED_VUS || 50);
const maxVUs = Number(__ENV.MAX_VUS || 400);

const reqDurationMs = new Trend('openai_ws_v2_perf_req_duration_ms', true);
const ttftMs = new Trend('openai_ws_v2_perf_ttft_ms', true);
const non2xxRate = new Rate('openai_ws_v2_perf_non2xx_rate');
const doneRate = new Rate('openai_ws_v2_perf_done_rate');
const expectedErrorRate = new Rate('openai_ws_v2_perf_expected_error_rate');

export const options = {
  scenarios: {
    short_request: {
      executor: 'constant-arrival-rate',
      exec: 'runShortRequest',
      rate: shortRPS,
      timeUnit: '1s',
      duration,
      preAllocatedVUs,
      maxVUs,
      tags: { scenario: 'short_request' REDACTED,
    REDACTED,
    long_request: {
      executor: 'constant-arrival-rate',
      exec: 'runLongRequest',
      rate: longRPS,
      timeUnit: '1s',
      duration,
      preAllocatedVUs: Math.max(20, Math.ceil(longRPS * 6)),
      maxVUs: Math.max(100, Math.ceil(longRPS * 20)),
      tags: { scenario: 'long_request' REDACTED,
    REDACTED,
    error_injection: {
      executor: 'constant-arrival-rate',
      exec: 'runErrorInjection',
      rate: errorRPS,
      timeUnit: '1s',
      duration,
      preAllocatedVUs: Math.max(8, Math.ceil(errorRPS * 4)),
      maxVUs: Math.max(40, Math.ceil(errorRPS * 12)),
      tags: { scenario: 'error_injection' REDACTED,
    REDACTED,
    hotspot_account: {
      executor: 'constant-arrival-rate',
      exec: 'runHotspotAccount',
      rate: hotspotRPS,
      timeUnit: '1s',
      duration,
      preAllocatedVUs: Math.max(16, Math.ceil(hotspotRPS * 3)),
      maxVUs: Math.max(80, Math.ceil(hotspotRPS * 10)),
      tags: { scenario: 'hotspot_account' REDACTED,
    REDACTED,
  REDACTED,
  thresholds: {
    openai_ws_v2_perf_non2xx_rate: ['rate<0.05'],
    openai_ws_v2_perf_req_duration_ms: ['p(95)<5000', 'p(99)<9000'],
    openai_ws_v2_perf_ttft_ms: ['p(99)<2000'],
    openai_ws_v2_perf_done_rate: ['rate>0.95'],
  REDACTED,
REDACTED;

function buildHeaders(apiKey, opts = {REDACTED) {
  const headers = {
    'Content-Type': 'application/json',
    'User-Agent': 'codex_cli_rs/0.104.0',
    'OpenAI-Beta': 'responses_websockets=2026-02-06,responses=experimental',
  REDACTED;
  if (apiKey) {
    headers.Authorization = `Bearer ${apiKeyREDACTED`;
  REDACTED
  if (opts.sessionID) {
    headers.session_id = opts.sessionID;
  REDACTED
  if (opts.conversationID) {
    headers.conversation_id = opts.conversationID;
  REDACTED
  return headers;
REDACTED

function shortBody() {
  return JSON.stringify({
    model,
    stream: false,
    input: [
      {
        role: 'user',
        content: [{ type: 'input_text', text: '请回复一个词：pong' REDACTED],
      REDACTED,
    ],
    max_output_tokens: 64,
  REDACTED);
REDACTED

function longBody() {
  const tools = [];
  for (let i = 0; i < 28; i += 1) {
    tools.push({
      type: 'function',
      name: `perf_tool_${iREDACTED`,
      description: 'load test tool schema',
      parameters: {
        type: 'object',
        properties: {
          query: { type: 'string' REDACTED,
          limit: { type: 'number' REDACTED,
          with_cache: { type: 'boolean' REDACTED,
        REDACTED,
        required: ['query'],
      REDACTED,
    REDACTED);
  REDACTED

  const input = [];
  for (let i = 0; i < 20; i += 1) {
    input.push({
      role: 'user',
      content: [{ type: 'input_text', text: `长请求压测消息 ${iREDACTED: 请输出简要摘要。` REDACTED],
    REDACTED);
  REDACTED

  return JSON.stringify({
    model,
    stream: false,
    input,
    tools,
    parallel_tool_calls: true,
    max_output_tokens: 256,
    reasoning: { effort: 'medium' REDACTED,
    instructions: '你是压测助手，简洁回复。',
  REDACTED);
REDACTED

function errorInjectionBody() {
  return JSON.stringify({
    model,
    stream: false,
    previous_response_id: `resp_not_found_${__VUREDACTED_${__ITERREDACTED`,
    input: [
      {
        role: 'user',
        content: [{ type: 'input_text', text: '触发错误注入路径。' REDACTED],
      REDACTED,
    ],
  REDACTED);
REDACTED

function postResponses(apiKey, body, tags, opts = {REDACTED) {
  const res = http.post(`${baseURLREDACTED/v1/responses`, body, {
    headers: buildHeaders(apiKey, opts),
    timeout,
    tags,
  REDACTED);
  reqDurationMs.add(res.timings.duration, tags);
  ttftMs.add(res.timings.waiting, tags);
  non2xxRate.add(res.status < 200 || res.status >= 300, tags);
  return res;
REDACTED

function hasDone(res) {
  return !!res && !!res.body && res.body.indexOf('[DONE]') >= 0;
REDACTED

export function runShortRequest() {
  const tags = { scenario: 'short_request' REDACTED;
  const res = postResponses(wsAPIKey, shortBody(), tags);
  check(res, { 'short status is 2xx': (r) => r.status >= 200 && r.status < 300 REDACTED);
  doneRate.add(hasDone(res) || (res.status >= 200 && res.status < 300), tags);
REDACTED

export function runLongRequest() {
  const tags = { scenario: 'long_request' REDACTED;
  const res = postResponses(wsAPIKey, longBody(), tags);
  check(res, { 'long status is 2xx': (r) => r.status >= 200 && r.status < 300 REDACTED);
  doneRate.add(hasDone(res) || (res.status >= 200 && res.status < 300), tags);
REDACTED

export function runErrorInjection() {
  const tags = { scenario: 'error_injection' REDACTED;
  const res = postResponses(wsAPIKey, errorInjectionBody(), tags);
  // 错误注入场景允许 4xx/5xx，重点观测 fallback 和错误路径抖动。
  expectedErrorRate.add(res.status >= 400, tags);
  doneRate.add(hasDone(res), tags);
REDACTED

export function runHotspotAccount() {
  const tags = { scenario: 'hotspot_account' REDACTED;
  const opts = {
    sessionID: 'perf-hotspot-session-fixed',
    conversationID: 'perf-hotspot-conversation-fixed',
  REDACTED;
  const res = postResponses(wsHotspotAPIKey, shortBody(), tags, opts);
  check(res, { 'hotspot status is 2xx': (r) => r.status >= 200 && r.status < 300 REDACTED);
  doneRate.add(hasDone(res) || (res.status >= 200 && res.status < 300), tags);
  sleep(0.01);
REDACTED

export function handleSummary(data) {
  return {
    stdout: `\nOpenAI WSv2 性能套件压测完成\n${JSON.stringify(data.metrics, null, 2)REDACTED\n`,
    'docs/perf/openai-ws-v2-perf-suite-summary.json': JSON.stringify(data, null, 2),
  REDACTED;
REDACTED
