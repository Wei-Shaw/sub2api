import http from 'k6/http';
import { check REDACTED from 'k6';
import { Rate, Trend REDACTED from 'k6/metrics';

const pooledBaseURL = (__ENV.POOLED_BASE_URL || 'http://127.0.0.1:5231').replace(/\/$/, '');
const oneToOneBaseURL = (__ENV.ONE_TO_ONE_BASE_URL || '').replace(/\/$/, '');
const wsAPIKey = (__ENV.WS_API_KEY || '').trim();
const model = __ENV.MODEL || 'gpt-5.1';
const timeout = __ENV.TIMEOUT || '180s';
const duration = __ENV.DURATION || '5m';
const pooledRPS = Number(__ENV.POOLED_RPS || 12);
const oneToOneRPS = Number(__ENV.ONE_TO_ONE_RPS || 12);
const preAllocatedVUs = Number(__ENV.PRE_ALLOCATED_VUS || 50);
const maxVUs = Number(__ENV.MAX_VUS || 400);

const pooledDurationMs = new Trend('openai_ws_pooled_duration_ms', true);
const oneToOneDurationMs = new Trend('openai_ws_one_to_one_duration_ms', true);
const pooledTTFTMs = new Trend('openai_ws_pooled_ttft_ms', true);
const oneToOneTTFTMs = new Trend('openai_ws_one_to_one_ttft_ms', true);
const pooledNon2xxRate = new Rate('openai_ws_pooled_non2xx_rate');
const oneToOneNon2xxRate = new Rate('openai_ws_one_to_one_non2xx_rate');

export const options = {
  scenarios: {
    pooled_mode: {
      executor: 'constant-arrival-rate',
      exec: 'runPooledMode',
      rate: pooledRPS,
      timeUnit: '1s',
      duration,
      preAllocatedVUs,
      maxVUs,
      tags: { mode: 'pooled' REDACTED,
    REDACTED,
    one_to_one_mode: {
      executor: 'constant-arrival-rate',
      exec: 'runOneToOneMode',
      rate: oneToOneRPS,
      timeUnit: '1s',
      duration,
      preAllocatedVUs,
      maxVUs,
      tags: { mode: 'one_to_one' REDACTED,
      startTime: '5s',
    REDACTED,
  REDACTED,
  thresholds: {
    openai_ws_pooled_non2xx_rate: ['rate<0.02'],
    openai_ws_one_to_one_non2xx_rate: ['rate<0.02'],
    openai_ws_pooled_duration_ms: ['p(95)<3000', 'p(99)<6000'],
    openai_ws_one_to_one_duration_ms: ['p(95)<6000', 'p(99)<10000'],
  REDACTED,
REDACTED;

function buildHeaders() {
  const headers = {
    'Content-Type': 'application/json',
    'User-Agent': 'codex_cli_rs/0.98.0',
  REDACTED;
  if (wsAPIKey) {
    headers.Authorization = `Bearer ${wsAPIKeyREDACTED`;
  REDACTED
  return headers;
REDACTED

function buildBody() {
  return JSON.stringify({
    model,
    stream: false,
    input: [
      {
        role: 'user',
        content: [{ type: 'input_text', text: '请回复: pong' REDACTED],
      REDACTED,
    ],
    max_output_tokens: 48,
  REDACTED);
REDACTED

function send(baseURL, mode) {
  if (!baseURL) {
    return null;
  REDACTED
  const res = http.post(`${baseURLREDACTED/v1/responses`, buildBody(), {
    headers: buildHeaders(),
    timeout,
    tags: { mode REDACTED,
  REDACTED);
  check(res, {
    'status is 2xx': (r) => r.status >= 200 && r.status < 300,
  REDACTED);
  return res;
REDACTED

export function runPooledMode() {
  const res = send(pooledBaseURL, 'pooled');
  if (!res) {
    return;
  REDACTED
  pooledDurationMs.add(res.timings.duration, { mode: 'pooled' REDACTED);
  pooledTTFTMs.add(res.timings.waiting, { mode: 'pooled' REDACTED);
  pooledNon2xxRate.add(res.status < 200 || res.status >= 300, { mode: 'pooled' REDACTED);
REDACTED

export function runOneToOneMode() {
  if (!oneToOneBaseURL) {
    return;
  REDACTED
  const res = send(oneToOneBaseURL, 'one_to_one');
  if (!res) {
    return;
  REDACTED
  oneToOneDurationMs.add(res.timings.duration, { mode: 'one_to_one' REDACTED);
  oneToOneTTFTMs.add(res.timings.waiting, { mode: 'one_to_one' REDACTED);
  oneToOneNon2xxRate.add(res.status < 200 || res.status >= 300, { mode: 'one_to_one' REDACTED);
REDACTED

export function handleSummary(data) {
  return {
    stdout: `\nOpenAI WS 池化 vs 1:1 对比压测完成\n${JSON.stringify(data.metrics, null, 2)REDACTED\n`,
    'docs/perf/openai-ws-pooling-compare-summary.json': JSON.stringify(data, null, 2),
  REDACTED;
REDACTED
