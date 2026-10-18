package repository

// Parameters: group, model, effort, service tier, endpoint, reference model, bank revision.
// Missing protocol/effort on failures is counted separately rather than inferred as success.
const accountQualityRowsSQL = `WITH members AS (
  SELECT a.id,a.name,a.platform,a.type,a.status,a.schedulable,a.priority,a.concurrency
  FROM accounts a
  WHERE a.deleted_at IS NULL AND (($1>0 AND EXISTS(
  SELECT 1
  FROM account_groups ag
  WHERE ag.account_id=a.id AND ag.group_id=$1)) OR ($1=0 AND NOT EXISTS(
  SELECT 1
  FROM account_groups ag JOIN groups g ON g.id=ag.group_id AND g.deleted_at IS NULL
  WHERE ag.account_id=a.id)))),
usage AS (
  SELECT u.account_id,COUNT(*) n,COUNT(first_token_ms) FILTER(WHERE first_token_ms>0) ns,percentile_cont(.5) WITHIN GROUP(ORDER BY first_token_ms) FILTER(WHERE first_token_ms>0) p50,percentile_cont(.95) WITHIN GROUP(ORDER BY first_token_ms) FILTER(WHERE first_token_ms>0) p95,100.0*SUM(cache_read_tokens)/NULLIF(SUM(input_tokens::bigint+cache_creation_tokens+cache_read_tokens),0) cache_rate,MAX(created_at) last_at
  FROM usage_logs u
  JOIN members m ON m.id=u.account_id
  WHERE u.created_at>NOW()-interval '24 hours' AND (($1>0 AND u.group_id=$1) OR ($1=0 AND u.group_id IS NULL)) AND ($2='' OR COALESCE(NULLIF(u.requested_model,''),u.model)=$2) AND ($2='' OR CASE u.inbound_endpoint WHEN '/v1/chat/completions' THEN 'chat_completions' WHEN '/v1/messages' THEN 'messages' WHEN '/v1beta/models' THEN 'generate_content' WHEN '/v1/responses' THEN 'responses' ELSE '' END=$5) AND ($2='' OR CASE WHEN $3='unspecified' THEN u.requested_reasoning_effort IS NULL ELSE COALESCE(u.reasoning_effort,'')=$3 END) AND ($2='' OR COALESCE(NULLIF(u.service_tier,'default'),'')=$4)
  GROUP BY u.account_id),
failures AS (
 SELECT e.account_id,
 COUNT(DISTINCT COALESCE(NULLIF(e.request_id,''),e.id::text)) FILTER(WHERE $2='' OR (COALESCE(NULLIF(e.requested_model,''),e.model)=$2 AND CASE WHEN e.request_path LIKE '%/chat/completions' THEN 'chat_completions' WHEN e.request_path LIKE '%/messages' THEN 'messages' WHEN e.request_path LIKE '%:generateContent' OR e.request_path LIKE '%:streamGenerateContent' THEN 'generate_content' WHEN e.request_path LIKE '%/responses' THEN 'responses' ELSE '' END=$5 AND e.quality_scope_known AND COALESCE(e.quality_reasoning_effort,'unspecified')=$3 AND COALESCE(NULLIF(e.quality_service_tier,'default'),'')=$4)) n,
 COUNT(DISTINCT COALESCE(NULLIF(e.request_id,''),e.id::text)) FILTER(WHERE $2<>'' AND (COALESCE(COALESCE(NULLIF(e.requested_model,''),e.model),'')='' OR CASE WHEN e.request_path LIKE '%/chat/completions' THEN 'chat_completions' WHEN e.request_path LIKE '%/messages' THEN 'messages' WHEN e.request_path LIKE '%:generateContent' OR e.request_path LIKE '%:streamGenerateContent' THEN 'generate_content' WHEN e.request_path LIKE '%/responses' THEN 'responses' ELSE '' END='' OR (CASE WHEN e.request_path LIKE '%/chat/completions' THEN 'chat_completions' WHEN e.request_path LIKE '%/messages' THEN 'messages' WHEN e.request_path LIKE '%:generateContent' OR e.request_path LIKE '%:streamGenerateContent' THEN 'generate_content' WHEN e.request_path LIKE '%/responses' THEN 'responses' ELSE '' END=$5 AND NOT e.quality_scope_known))) unknown_scope
 FROM ops_error_logs e JOIN members m ON m.id=e.account_id
 WHERE e.created_at>NOW()-interval '24 hours' AND NOT e.is_business_limited AND COALESCE(e.error_owner,'')<>'client'
 AND (($1>0 AND e.group_id=$1) OR ($1=0 AND e.group_id IS NULL)) AND ($2='' OR COALESCE(NULLIF(e.requested_model,''),e.model)=$2 OR COALESCE(COALESCE(NULLIF(e.requested_model,''),e.model),'')='')
 AND NOT EXISTS(SELECT 1 FROM usage_logs u WHERE u.request_id=e.request_id AND u.account_id=e.account_id AND u.group_id IS NOT DISTINCT FROM e.group_id AND u.created_at>NOW()-interval '24 hours')
 GROUP BY e.account_id),
trace AS (
  SELECT DISTINCT ON(account_id) account_id,verdict,created_at,evidence->'attribution'->>'prediction' candidate
  FROM account_quality_results
  WHERE model=$2 AND effort=$3 AND service_tier=$4 AND endpoint=$5 AND benchmark='modeltrace-v1' AND evidence->>'expected_model'=$6 AND evidence->>'bank_revision'=$7
  ORDER BY account_id,created_at DESC),
latest_candy AS (
  SELECT DISTINCT ON(account_id) account_id,verdict,created_at
  FROM account_quality_results
  WHERE model=$2 AND effort=$3 AND service_tier=$4 AND endpoint=$5 AND benchmark='candy-v1' AND detail NOT LIKE 'reused_result:%'
  ORDER BY account_id,created_at DESC),
latest_verification AS (
  SELECT DISTINCT ON(account_id) account_id,verdict,created_at
  FROM account_quality_results
  WHERE model=$2 AND effort=$3 AND service_tier=$4 AND endpoint=$5 AND benchmark IN ('arithmetic-v1','candy-variant-v1','context-short-v1','context-medium-v1','context-long-v1') AND verdict='incorrect' AND created_at>NOW()-interval '1 hour'
  ORDER BY account_id,created_at DESC),
candy_summary AS (
  SELECT account_id,COUNT(*) FILTER(WHERE verdict IN ('passed','incorrect')) samples,COUNT(*) FILTER(WHERE verdict='passed') passed
  FROM account_quality_results
  WHERE model=$2 AND effort=$3 AND service_tier=$4 AND endpoint=$5 AND benchmark='candy-v1' AND detail NOT LIKE 'reused_result:%' AND created_at>NOW()-interval '1 hour'
  GROUP BY account_id)
  SELECT m.*,COALESCE(u.n,0),COALESCE(u.ns,0),u.p50,u.p95,u.cache_rate,u.last_at,CASE WHEN q.verdict='passed' AND verify.verdict='incorrect' AND verify.created_at>=q.created_at THEN 'inconsistent' ELSE COALESCE(q.verdict,'untested') END,COALESCE(qs.samples,0),COALESCE(qs.passed,0),q.created_at,COALESCE(f.n,0),COALESCE(f.unknown_scope,0),COALESCE(tr.verdict,'untested'),tr.created_at,COALESCE(tr.candidate,'')
  FROM members m
  LEFT JOIN failures f ON f.account_id=m.id
  LEFT JOIN trace tr ON tr.account_id=m.id
  LEFT JOIN usage u ON u.account_id=m.id
  LEFT JOIN latest_candy q ON q.account_id=m.id
  LEFT JOIN latest_verification verify ON verify.account_id=m.id
  LEFT JOIN candy_summary qs ON qs.account_id=m.id
  ORDER BY m.name,m.id`
