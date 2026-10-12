# BUAGA 定制清单

### C-417 升级保持渠道图片空价的既有扣费口径
- 文件：backend/internal/service/model_pricing_resolver.go
- 断言："resolved.BasePricing.ImageOutputPricePerToken = 0"
- 断言："resolved.BasePricing.ImageOutputPriceExplicit = true"
- 断言："func applyChannelImageInputPrice(chPricing *ChannelModelPricing, pricing *ModelPricing)"
- 说明：v0.2.9 目录图片价自动继承不适用于此次保持现有扣费口径的升级；显式图片价格与原有回退规则保留。

### C-410 压缩请求解压超限不得静默截断
- 文件：backend/internal/pkg/httputil/body.go
- 断言："io.LimitReader(reader, maxDecodedBytes+1)"
- 断言："return nil, &http.MaxBytesError{Limit: maxDecodedBytes}"
- 断言："decompressRequestBody(enc, raw, maxDecompressedBodySize)"

### C-411 压缩正文边界与完整性回归
- 文件：backend/internal/pkg/httputil/body_decoded_limit_test.go
- 断言："TestDecompressRequestBodyRejectsOversizeWithoutTruncation"
- 断言："TestReadRequestBodyWithPreallocRejectsDecodedOversize"
- 断言："TestDecompressRequestBodyRejectsCorruptCompressedTrailer"

基线：线上 R3（56d0092e9506b9a560cda8202788225e379befad），比较基准：上游 v0.2.5。

每条同时检查文件和内容。自动登记的源码锚点用于防止文件内部回退；行为仍须由测试和页面验收确认。
改动涉及锚点时，必须审查功能是否保留后再更新对应断言，禁止为了通过守卫而批量清空。

迁移前 R3 已丢失的生图推荐卡及推荐徽章在本次功能分支中恢复，以下新增断言与交互测试保护这些入口。

### C-395 DeepSeek 缓存按密钥与模型隔离且限时限量
- 文件：backend/internal/service/deepseek_reasoning_cache.go
- 断言："deepseek-replay-v1:%d:%x:"
- 断言："deepSeekReplayBudget"
- 断言："deepSeekReplayCallIdentity"

### C-396 DeepSeek Responses 主入口恢复历史思考
- 文件：backend/internal/service/openai_gateway_forward.go
- 断言："s.restoreDeepSeekReasoning(c, account, body, replayModel, mappedReplayModel)"

### C-397 DeepSeek Chat 兼容入口读写缓存
- 文件：backend/internal/service/openai_gateway_chat_completions.go
- 断言："s.restoreDeepSeekReasoning(c, account, responsesBody, originalModel, upstreamModel)"
- 断言："replay.observe([]byte(payload))"

### C-398 DeepSeek Messages 兼容入口读写缓存
- 文件：backend/internal/service/openai_gateway_messages.go
- 断言："s.restoreDeepSeekReasoning(c, account, responsesBody, originalModel, upstreamModel)"
- 断言："replay.observe([]byte(payload))"

### C-399 DeepSeek Responses 流式及聚合响应缓存
- 文件：backend/internal/service/openai_gateway_response_handling.go
- 断言："replay.observe(dataBytes)"
- 断言："s.newDeepSeekReasoningCache(c, account, originalModel, mappedModel).observe(body)"

### C-400 DeepSeek 自动透传响应缓存
- 文件：backend/internal/service/openai_gateway_passthrough.go
- 断言："replay.observe(dataBytes)"
- 断言："s.newDeepSeekReasoningCache(c, account, originalModel, mappedModel).observe(body)"

### C-402 所有视频入口提交前留记录及成功响应前落库
- 文件：backend/internal/handler/video_task_record.go
- 断言："recorder.BeginVideoTaskRecord"
- 断言："recorder.AcceptVideoTaskRecord"
- 断言："recorder.ObserveVideoTaskRecord"

### C-403 视频任务记录与原有结算隔离
- 文件：backend/internal/repository/video_task_record_repo.go
- 断言："9999-12-31 00:00:00+00"
- 断言："pending->>'tracking_only'='true'"

### C-404 视频创建编辑延长及原生协议统一记录
- 文件：backend/internal/handler/grok_media.go
- 断言："forwardRecordedVideo(c, h.gatewayService, endpoint"

### C-405 视频监控只查状态与使用记录，不发起扣费
- 文件：backend/internal/service/video_task_monitor.go
- 断言："SyncVideoTaskMonitorUsage"
- 断言："pollVideoTaskMonitor"
- 断言："videoTaskMonitorRoutingFingerprint"

### C-406 视频监控租约与完整归属使用记录同步
- 文件：backend/internal/repository/video_task_monitor_repo.go
- 断言："FOR UPDATE SKIP LOCKED"
- 断言："AND l.user_id=t.user_id AND l.api_key_id=t.api_key_id AND l.account_id=t.account_id"
- 断言："AND pending->>'tracking_only'='true'"

### C-407 视频监控隔离扣费及旧任务有限查询回归
- 文件：backend/internal/service/video_task_monitor_test.go
- 断言："TestVideoMonitorOnlyObservesAndReusesExistingUsage"
- 断言："TestVideoMonitorDoesNotPollUnknownSubmissionsOrDurableBillingTasks"
- 断言："TestVideoTaskOldPollFailuresPauseWithoutChangingBillingRetries"

### C-408 视频待核对状态与只读费用说明
- 文件：frontend/src/i18n/locales/zh/admin/videoTasks.ts
- 断言："不会发起扣费或补扣"
- 断言："review_required"

### C-409 停止后台重试但保留原归属客户端限频复查
- 文件：backend/internal/repository/video_task_repo.go
- 断言："OR ($1<>'' AND last_error='poll_exhausted' AND updated_at<=NOW()-INTERVAL '4 minutes')"

### C-401 DeepSeek 多入口工具往返与隔离回归
- 文件：backend/internal/service/deepseek_reasoning_cache_test.go
- 断言："TestDeepSeekReplayCacheForwardRoundTrip"
- 断言："TestDeepSeekReplayCacheMultipleToolTurns"
- 断言："TestDeepSeekReplayCacheRestoresHistoryAndIsolatesKeys"
- 断言："TestDeepSeekReplayCacheUnavailableDoesNotBlockRequest"

### C-386 Chat 流内错误与换号回归
- 文件：backend/internal/handler/ops_chat_stream_error_test.go
- 断言："TestOpsChatStreamFailureIsNotRecovered"
- 断言："TestOpsChatStreamErrorEnvelope"

### C-387 Chat 开始框架暂存及心跳安全换号
- 文件：backend/internal/service/openai_chat_preamble_failover_test.go
- 断言："TestChatPreambleFailover"
- 断言："TestChatPreambleHeartbeatThenRetry"
- 断言："TestChatPreambleKeepsOneTokenCompletion"

### C-388 Chat 流内错误识别
- 文件：backend/internal/handler/ops_error_logger.go
- 断言："isOpsChatSSEErrorEnvelope"
- 断言："parsed.ChatErrorEnvelope && len(service.GetOpsStreamErrors(c)) > 0"

### C-389 Chat 换号排除心跳字节
- 文件：backend/internal/handler/openai_chat_completions.go
- 断言："semanticSizeBeforeForward := service.OpenAICompactKeepaliveAdjustedWrittenSize(c)"
- 断言："openAIForwardMayFailover(c, semanticSizeBeforeForward, failoverErr)"

### C-390 Chat 开始框架全请求大小保护
- 文件：backend/internal/service/openai_gateway_chat_completions.go
- 断言："refusalDetector := &openAIChatSilentRefusalDetector{enabled: true}"
- 断言："recordOpenAIStreamKeepaliveBytes(c, written)"

### C-391 流错误分类保留请求拒绝与真实 503 换号
- 文件：backend/internal/service/openai_apikey_availability_test.go
- 断言："TestOpenAIAPIKeyAvailability_ExplicitStream503KeepsRequestRejections"
- 断言："require.False(t, openAIStreamErrorEventShouldFailover([]byte(payload), message))"
- 断言："TestOpenAIAPIKeyAvailability_NonStreamingPassthroughTerminal"

### C-393 上游账号 HTTP 放行不得影响素材出站校验
- 文件：backend/internal/service/account_http_policy_test.go
- 断言："func TestAccountHTTPPolicyPreservesOtherURLRestrictions(t *testing.T)"
- 断言："func TestAccountHTTPModelsRequestUsesConfiguredEndpoint(t *testing.T)"
- 断言："account exception must not reach media fetches"

### C-394 账号 URL 与通用下载 URL 策略分离
- 文件：backend/internal/service/openai_gateway_request_body.go
- 断言："urlvalidator.ValidateHTTPURL(raw, true, urlvalidator.ValidationOptions{"
- 断言："return urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP, s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts)"

### C-392 无状态码的临时流通知允许后续完成
- 文件：backend/internal/service/openai_gateway_response_flush_test.go
- 断言："TestOpenAIResponseFlush_RecentBareErrorAllowsCompletedBeforeIdleTimeout"
- 断言："completedWaiting := make(chan struct{})"

### C-384 Grok 视频中转响应尺寸与媒体地址兼容
- 文件：backend/internal/service/openai_video_grok_compat.go
- 断言："normalizeGrokVideoRelayGeometry"
- 断言："metadata.video_url"
- 断言："counts[key] > 1"

### C-385 Grok 视频本站提交查询下载回归
- 文件：backend/internal/service/openai_video_grok_compat_test.go
- 断言："TestOpenAIVideoGrokRelayCreatePollAndContent"
- 断言："TestOpenAIVideoGrokRelayGeometryValidation"

### C-381 用户仪表盘生图推荐入口
- 文件：frontend/src/views/user/DashboardView.vue
- 断言："v-if=\"imageWorkbench\""
- 断言："AI 生图工作台"
- 断言："item.visibility === 'user' && item.id === '97370c95df45cba1'"

### C-382 用户和管理员个人菜单生图推荐徽章
- 文件：frontend/src/components/layout/AppSidebar.vue
- 断言："item.recommended && !sidebarCollapsed"
- 断言："item.id !== imageWorkbench?.id"
- 断言："icon: BatchImageIcon, recommended: true"

### C-383 生图入口交互回归测试
- 文件：frontend/src/components/layout/__tests__/ImageWorkbenchRecommendation.spec.ts
- 断言："for (const isAdmin of [false, true])"
- 断言："expect(workbenchLinks).toHaveLength(1)"
- 断言："expect(router.currentRoute.value.path).toBe(target)"


### C-001 backend/cmd/server/VERSION
- 文件：backend/cmd/server/VERSION
- 断言："0.2.15"


### C-002 backend/cmd/server/wire_gen.go
- 文件：backend/cmd/server/wire_gen.go
- 断言："adminUsageHandler := admin.NewUsageHandler(usageService, apiKeyService, adminService, usageCleanupService, openAIGatewayService)"


### C-003 backend/ent/channelmonitor.go
- 文件：backend/ent/channelmonitor.go
- 断言："return fmt.Errorf(\"unexpected type %T for field sort_order\", values[i])"
- 断言："SortOrder int `json:\"sort_order,omitempty\"`"
- 断言："builder.WriteString(\"sort_order=\")"


### C-004 backend/ent/channelmonitor/channelmonitor.go
- 文件：backend/ent/channelmonitor/channelmonitor.go
- 断言："FieldSortOrder = \"sort_order\""
- 断言："func BySortOrder(opts ...sql.OrderTermOption) OrderOption {"
- 断言："return sql.OrderByField(FieldSortOrder, opts...).ToFunc()"


### C-005 backend/ent/channelmonitor/where.go
- 文件：backend/ent/channelmonitor/where.go
- 断言："return predicate.ChannelMonitor(sql.FieldNotIn(FieldSortOrder, vs...))"
- 断言："return predicate.ChannelMonitor(sql.FieldIn(FieldSortOrder, vs...))"
- 断言："return predicate.ChannelMonitor(sql.FieldGTE(FieldSortOrder, v))"


### C-006 backend/ent/channelmonitor_create.go
- 文件：backend/ent/channelmonitor_create.go
- 断言："return &ValidationError{Name: \"sort_order\", err: errors.New(`ent: missing required field \"ChannelMonitor.sort_order\"`)}"
- 断言："func (_c *ChannelMonitorCreate) SetNillableSortOrder(v *int) *ChannelMonitorCreate {"
- 断言："func (u *ChannelMonitorUpsertBulk) SetSortOrder(v int) *ChannelMonitorUpsertBulk {"


### C-007 backend/ent/channelmonitor_update.go
- 文件：backend/ent/channelmonitor_update.go
- 断言："func (_u *ChannelMonitorUpdateOne) SetNillableSortOrder(v *int) *ChannelMonitorUpdateOne {"
- 断言："func (_u *ChannelMonitorUpdate) SetNillableSortOrder(v *int) *ChannelMonitorUpdate {"
- 断言："func (_u *ChannelMonitorUpdateOne) AddSortOrder(v int) *ChannelMonitorUpdateOne {"


### C-008 backend/ent/migrate/schema.go
- 文件：backend/ent/migrate/schema.go
- 断言："{Name: \"usage_source\", Type: field.TypeString, Size: 16, Default: \"upstream\"},"
- 断言："{Name: \"sort_order\", Type: field.TypeInt, Default: 2147483647},"
- 断言："Name:    \"channelmonitor_sort_order\","


### C-009 backend/ent/mutation.go
- 文件：backend/ent/mutation.go
- 断言："addsort_order           *int"
- 断言："sort_order              *int"
- 断言："if m.addsort_order != nil {"


### C-010 backend/ent/runtime/runtime.go
- 文件：backend/ent/runtime/runtime.go
- 断言："usagelogDescCacheCreation1hTokens := usagelogFields[22].Descriptor()"
- 断言："usagelogDescCacheCreation5mTokens := usagelogFields[21].Descriptor()"
- 断言："usagelogDescCacheCreationTokens := usagelogFields[19].Descriptor()"


### C-011 backend/ent/schema/channel_monitor.go
- 文件：backend/ent/schema/channel_monitor.go
- 断言："index.Fields(\"sort_order\"),"
- 断言："field.Int(\"sort_order\")."
- 断言："Comment(\"Admin display order; lower values appear first\"),"


### C-012 backend/ent/schema/usage_log.go
- 文件：backend/ent/schema/usage_log.go
- 断言："Comment(\"Whether the downstream client disconnected before upstream completion\"),"
- 断言："Comment(\"Token usage source: upstream or estimated\"),"
- 断言："Default(\"upstream\")."


### C-013 backend/ent/usagelog.go
- 文件：backend/ent/usagelog.go
- 断言："case usagelog.FieldClientDisconnected, usagelog.FieldUpstreamModelMismatch, usagelog.FieldLongContextBillingApplied, usagelog.FieldStream, usagelog.FieldCacheTTLOverridden:"
- 断言："return fmt.Errorf(\"unexpected type %T for field client_disconnected\", values[i])"
- 断言："return fmt.Errorf(\"unexpected type %T for field usage_source\", values[i])"


### C-014 backend/ent/usagelog/usagelog.go
- 文件：backend/ent/usagelog/usagelog.go
- 断言："func ByClientDisconnected(opts ...sql.OrderTermOption) OrderOption {"
- 断言："return sql.OrderByField(FieldClientDisconnected, opts...).ToFunc()"
- 断言："func ByUsageSource(opts ...sql.OrderTermOption) OrderOption {"


### C-015 backend/ent/usagelog/where.go
- 文件：backend/ent/usagelog/where.go
- 断言："return predicate.UsageLog(sql.FieldContainsFold(FieldUsageSource, v))"
- 断言："return predicate.UsageLog(sql.FieldNEQ(FieldClientDisconnected, v))"
- 断言："return predicate.UsageLog(sql.FieldHasPrefix(FieldUsageSource, v))"


### C-016 backend/ent/usagelog_create.go
- 文件：backend/ent/usagelog_create.go
- 断言："return &ValidationError{Name: \"client_disconnected\", err: errors.New(`ent: missing required field \"UsageLog.client_disconnected\"`)}"
- 断言："return &ValidationError{Name: \"usage_source\", err: fmt.Errorf(`ent: validator failed for field \"UsageLog.usage_source\": %w`, err)}"
- 断言："return &ValidationError{Name: \"usage_source\", err: errors.New(`ent: missing required field \"UsageLog.usage_source\"`)}"


### C-017 backend/ent/usagelog_update.go
- 文件：backend/ent/usagelog_update.go
- 断言："return &ValidationError{Name: \"usage_source\", err: fmt.Errorf(`ent: validator failed for field \"UsageLog.usage_source\": %w`, err)}"
- 断言："func (_u *UsageLogUpdateOne) SetNillableClientDisconnected(v *bool) *UsageLogUpdateOne {"
- 断言："func (_u *UsageLogUpdateOne) SetNillableUsageSource(v *string) *UsageLogUpdateOne {"


### C-018 backend/internal/config/config.go
- 文件：backend/internal/config/config.go
- 断言："viper.SetDefault(\"security.url_allowlist.allow_insecure_http_hosts\", []string{\"gpt.us.aiapisplus.com\"})"
- 断言："OpenAIAPIKeyAvailabilityEnabled bool `mapstructure:\"openai_apikey_availability_enabled\"`"
- 断言："AllowInsecureHTTPHosts []string `mapstructure:\"allow_insecure_http_hosts\"`"


### C-019 backend/internal/config/config_test.go
- 文件：backend/internal/config/config_test.go
- 断言："t.Setenv(\"GATEWAY_SCHEDULING_SKIP_STARTUP_CLEANUP\", \"true\")"
- 断言："require.True(t, cfg.Gateway.Scheduling.SkipStartupCleanup)"


### C-020 backend/internal/config/openai_apikey_availability_test.go
- 文件：backend/internal/config/openai_apikey_availability_test.go
- 断言："require.Equal(t, tc.want, cfg.Gateway.OpenAIAPIKeyAvailabilityEnabled)"
- 断言："t.Setenv(\"GATEWAY_OPENAI_APIKEY_AVAILABILITY_ENABLED\", tc.env)"
- 断言："{name: \"enabled by environment\", env: \"true\", want: true},"


### C-021 backend/internal/handler/admin/account_handler.go
- 文件：backend/internal/handler/admin/account_handler.go
- 断言："if models, fetchErr := h.accountTestService.FetchOpenAIAccountModels(c.Request.Context(), account); fetchErr == nil && len(models) > 0 {"
- 断言："requestedModels = append(requestedModels, requestedModel)"
- 断言："requestedModels := make([]string, 0, len(mapping))"


### C-022 backend/internal/handler/admin/account_handler_available_models_test.go
- 文件：backend/internal/handler/admin/account_handler_available_models_test.go
- 断言："Body: io.NopCloser(strings.NewReader(`{\"data\":[{\"id\":\"upstream-only-compact\"},{\"id\":\"gpt-5.5\"}]}`)),"
- 断言："svc.account.Credentials[\"model_mapping\"] = map[string]any{\"my-model\": \"upstream-only-compact\"}"
- 断言："func TestAccountModelPickerUsesSavedWhitelistInsteadOfCachedUpstreamCatalog(t *testing.T) {"


### C-023 backend/internal/handler/admin/channel_handler.go
- 文件：backend/internal/handler/admin/channel_handler.go
- 断言："ExpectedUpdatedAt          *time.Time                        `json:\"expected_updated_at\"`"
- 断言："UpdatedAt:      ch.UpdatedAt.UTC().Format(time.RFC3339Nano),"
- 断言："if req.ModelPricing != nil && req.ExpectedUpdatedAt == nil {"


### C-024 backend/internal/handler/admin/channel_monitor_handler.go
- 文件：backend/internal/handler/admin/channel_monitor_handler.go
- 断言："SortOrder           int                                  `json:\"sort_order\"`"
- 断言："SortOrder int   `json:\"sort_order\"`"
- 断言："response.Success(c, gin.H{\"message\": \"Channel monitor sort order updated successfully\"})"


### C-025 backend/internal/handler/admin/channel_version_test.go
- 文件：backend/internal/handler/admin/channel_version_test.go
- 断言："req := httptest.NewRequest(http.MethodPut, \"/channels/3\", strings.NewReader(`{\"model_pricing\":[]}`))"
- 断言："stamp := time.Date(2026, 9, 14, 14, 56, 2, 337798000, time.FixedZone(\"CST\", 8*3600))"
- 断言："func TestChannelVersionResponsePreservesPrecisionAndTimezone(t *testing.T) {"


### C-026 backend/internal/handler/admin/ops_handler.go
- 文件：backend/internal/handler/admin/ops_handler.go
- 断言："if v := strings.TrimSpace(c.Query(\"include_recovered_upstream\")); v != \"\" {"
- 断言："response.BadRequest(c, \"Invalid include_recovered_upstream\")"
- 断言："filter.IncludeRecoveredUpstream = parsed"


### C-027 backend/internal/handler/admin/ops_realtime_handler.go
- 文件：backend/internal/handler/admin/ops_realtime_handler.go
- 断言："response.Error(c, http.StatusServiceUnavailable, \"Ops service not available\")"
- 断言："snapshot, err := h.opsService.GetSchedulerSnapshot(c.Request.Context())"
- 断言："func (h *OpsHandler) GetSchedulerSnapshot(c *gin.Context) {"


### C-028 backend/internal/handler/admin/setting_handler.go
- 文件：backend/internal/handler/admin/setting_handler.go
- 断言："LongNoticeContent:                                            settings.LongNoticeContent,"


### C-029 backend/internal/handler/admin/setting_handler_update.go
- 文件：backend/internal/handler/admin/setting_handler_update.go
- 断言："LongNoticeContent:                                            updatedSettings.LongNoticeContent,"
- 断言："LongNoticeContent                 *string                `json:\"long_notice_content\"`"
- 断言："LongNoticeContent:                            longNoticeContent,"


### C-030 backend/internal/handler/admin/usage_cleanup_handler_test.go
- 文件：backend/internal/handler/admin/usage_cleanup_handler_test.go
- 断言："handler := NewUsageHandler(nil, nil, nil, cleanupService, nil)"


### C-031 backend/internal/handler/admin/usage_handler.go
- 文件：backend/internal/handler/admin/usage_handler.go
- 断言："service.ListOpenAIImagePreviewURLs(records[i].RequestID, records[i].APIKeyID, records[i].ID, records[i].ImageCount),"
- 断言："data, contentType, err := service.ReadOpenAIImagePreview(log.RequestID, log.APIKeyID, index)"
- 断言："row := dto.UsageLogFromServiceAdminWithMediaPreviews(&records[i],"


### C-032 backend/internal/handler/admin/usage_handler_request_type_test.go
- 文件：backend/internal/handler/admin/usage_handler_request_type_test.go
- 断言："handler := NewUsageHandler(usageSvc, nil, nil, nil, nil)"


### C-033 backend/internal/handler/admin/usage_handler_search_users_test.go
- 文件：backend/internal/handler/admin/usage_handler_search_users_test.go
- 断言："handler := NewUsageHandler(nil, nil, stub, nil, nil)"


### C-034 backend/internal/handler/admin/usage_video.go
- 文件：backend/internal/handler/admin/usage_video.go
- 断言："if err := h.videoGateway.StreamAdminVideoPreview(c, account, log.RequestID); err != nil && !c.Writer.Written() {"
- 断言："response.Error(c, http.StatusGone, \"Video is no longer available from the upstream provider\")"
- 断言："response.Error(c, http.StatusBadGateway, \"Video is unavailable from the upstream provider\")"


### C-035 backend/internal/handler/admin/usage_video_test.go
- 文件：backend/internal/handler/admin/usage_video_test.go
- 断言："log := &service.UsageLog{ID: 12, RequestID: \"grok-video:wan_123\", CreatedAt: time.Now().Add(-time.Hour), VideoCount: 1}"
- 断言："func (r *adminVideoUsageRepo) GetByID(context.Context, int64) (*service.UsageLog, error) {"
- 断言："require.Equal(t, \"/api/v1/admin/usage/12/videos/0/content\", w.Body.String())"


### C-036 backend/internal/handler/admin/video_tasks.go
- 文件：backend/internal/handler/admin/video_tasks.go
- 断言："response.Error(c, http.StatusServiceUnavailable, \"Video task list is unavailable\")"
- 断言："f := service.VideoTaskAdminFilter{Page: 1, PageSize: 20, Days: 7}"
- 断言："items, total, err := h.videoGateway.ListAdminVideoTasks(ctx, f)"


### C-037 backend/internal/handler/admin/video_tasks_test.go
- 文件：backend/internal/handler/admin/video_tasks_test.go
- 断言："for _, field := range strings.Fields(\"user_id account_id group_id model completed_at next_attempt_at expected_cost charged_cost usage_log_id video_duration_seconds\") {"
- 断言："stub.items = []service.VideoTaskAdminItem{{Kind: \"submission\", APIKeyID: 2, State: \"submission_unknown\", DisplayStatus: \"submission_unknown\", BillingStatus: \"review\"}}"
- 断言："r.ServeHTTP(w, httptest.NewRequest(\"GET\", \"/api/v1/admin/video-tasks?page=2&page_size=100&days=30&status=submission_unknown&search=\"+url.QueryEscape(literal), nil))"


### C-038 backend/internal/handler/auth_handler.go
- 文件：backend/internal/handler/auth_handler.go
- 断言："if err := h.authService.VerifyCaptchaForLogin(c.Request.Context(), proof, ip.GetClientIP(c)); err != nil {"


### C-039 backend/internal/handler/dto/mappers.go
- 文件：backend/internal/handler/dto/mappers.go
- 断言："func UsageLogFromServiceAdminWithMediaPreviews(l *service.UsageLog, images, videos []string) *AdminUsageLog {"
- 断言："func UsageLogFromServiceAdminWithImagePreviews(l *service.UsageLog, previews []string) *AdminUsageLog {"
- 断言："VideoDurationSeconds:      l.VideoDurationSeconds,"


### C-040 backend/internal/handler/dto/settings.go
- 文件：backend/internal/handler/dto/settings.go
- 断言："LongNoticeContent                   string                   `json:\"long_notice_content\"`"
- 断言："LongNoticeContent           string           `json:\"long_notice_content\"`"
- 断言："LoginCaptchaEnabled                 bool                     `json:\"login_captcha_enabled\"`"


### C-041 backend/internal/handler/dto/types.go
- 文件：backend/internal/handler/dto/types.go
- 断言："VideoDurationSeconds *int         `json:\"video_duration_seconds,omitempty\"`"
- 断言："VideoResolution    *string        `json:\"video_resolution,omitempty\"`"
- 断言："ImagePreviews []string `json:\"image_previews,omitempty\"`"


### C-042 backend/internal/handler/failover_loop.go
- 文件：backend/internal/handler/failover_loop.go
- 断言："if service.IsUpstreamInsufficientBalance(failoverErr.StatusCode, failoverErr.ResponseBody) {"
- 断言："if !exists || failoverErr.SameAccountRetryDeadline.Before(deadline) {"
- 断言："if !failoverErr.SameAccountRetryDeadline.IsZero() {"


### C-043 backend/internal/handler/failover_loop_test.go
- 文件：backend/internal/handler/failover_loop_test.go
- 断言："err := &service.UpstreamFailoverError{StatusCode: 429, RetryableOnSameAccount: true, SameAccountRetryDeadline: time.Now().Add(time.Minute)}"
- 断言："require.Equal(t, FailoverContinue, fs.HandleFailoverError(context.Background(), mock, 1, service.PlatformOpenAI, 3, err))"
- 断言："require.True(t, err.SameAccountRetryDeadline.After(time.Now()), \"do not mutate a shared upstream error\")"


### C-044 backend/internal/handler/grok_media.go
- 文件：backend/internal/handler/grok_media.go
- 断言："if err := h.gatewayService.PersistOpenAIVideoTask(saveCtx, created.ResponseID, apiKey, account, subscription, pending, quote); err != nil {"
- 断言："if err := h.gatewayService.CompleteVideoSubmission(saveCtx, apiKey.ID, c.GetHeader(\"Idempotency-Key\"), created.ResponseID); err != nil {"
- 断言："if account.Platform != service.PlatformOpenAI && isGrokVideoCreateEndpoint(endpoint) && strings.TrimSpace(result.ResponseID) != \"\" {"


### C-045 backend/internal/handler/model_plaza_handler.go
- 文件：backend/internal/handler/model_plaza_handler.go
- 断言："groups, err := h.plazaService.ListCatalogGroups(c.Request.Context(), rt.Catalog)"


### C-046 backend/internal/handler/openai_alpha_search.go
- 文件：backend/internal/handler/openai_alpha_search.go
- 断言："defer func() { selection.CompleteTransient(err == nil && result != nil && c.Request.Context().Err() == nil && openAIForwardSucceededForScheduling(result), err) }()"
- 断言："if slotResult == openAISlotAcquireTransientVetoed {"
- 断言："result, err = func() (result *service.OpenAIForwardResult, err error) {"


### C-047 backend/internal/handler/openai_chat_completions.go
- 文件：backend/internal/handler/openai_chat_completions.go
- 断言："selection.CompleteTransient(err == nil && result != nil && c.Request.Context().Err() == nil && openAIForwardSucceededForScheduling(result), err)"
- 断言："if slotResult == openAISlotAcquireTransientVetoed {"
- 断言："result, err := func() (result *service.OpenAIForwardResult, err error) {"


### C-048 backend/internal/handler/openai_embeddings.go
- 文件：backend/internal/handler/openai_embeddings.go
- 断言："selection.CompleteTransient(err == nil && result != nil && c.Request.Context().Err() == nil && openAIForwardSucceededForScheduling(result), err)"
- 断言："if slotResult == openAISlotAcquireTransientVetoed {"
- 断言："result, err := func() (result *service.OpenAIForwardResult, err error) {"


### C-049 backend/internal/handler/openai_gateway_availability_test.go
- 文件：backend/internal/handler/openai_gateway_availability_test.go
- 断言："h := NewOpenAIGatewayHandler(gatewaySvc, service.NewConcurrencyService(nil), billingCacheSvc, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)"
- 断言："upstream := &availabilityHTTPUpstream{failedStatus: tc.status, failedPayload: tc.payload, transportErr: tc.transportErr, readErr: tc.readErr, failSecond: tc.failSecond}"
- 断言："func (u *availabilityHTTPUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {"


### C-050 backend/internal/handler/openai_gateway_handler.go
- 文件：backend/internal/handler/openai_gateway_handler.go
- 断言："selection.CompleteTransient(err == nil && result != nil && c.Request.Context().Err() == nil && openAIForwardSucceededForScheduling(result), err)"
- 断言："selection.CompleteTransient(turnErr == nil && openAIForwardSucceededForScheduling(result), turnErr)"
- 断言："if slotResult == openAISlotAcquireTransientVetoed {"


### C-051 backend/internal/handler/openai_images.go
- 文件：backend/internal/handler/openai_images.go
- 断言："selection.CompleteTransient(err == nil && result != nil && c.Request.Context().Err() == nil && openAIForwardSucceededForScheduling(result), err)"
- 断言："service.SaveGrokVideoPreviewAsync(value, service.ResolveUsageRecordID(c.Request.Context(), result.RequestID), apiKey.ID)"
- 断言："if videoURL, ok := c.Get(\"openai_video_preview_url\"); ok {"


### C-052 backend/internal/handler/openai_video.go
- 文件：backend/internal/handler/openai_video.go
- 断言："func allowDurableOpenAIVideoForward(c *gin.Context, lookup durableOpenAIVideoLookup, task *service.VideoTask, account *service.Account, endpoint service.GrokMediaEndpoint) bool {"
- 断言："return progress(\"outcome_unknown\", \"The provider reports expiry; generation outcome is unknown. Retry this lookup later\", \"video_outcome_unknown\", http.StatusServiceUnavailable)"
- 断言："if latest.TaskID != task.TaskID || latest.UserID != task.UserID || latest.APIKeyID != task.APIKeyID || service.ValidateDurableVideoTaskAccount(latest, account) != nil {"


### C-053 backend/internal/handler/openai_video_test.go
- 文件：backend/internal/handler/openai_video_test.go
- 断言："{name: \"poll timeout\", state: service.VideoTaskPending, lastError: \"poll_timeout\", err: context.DeadlineExceeded, detail: \"poll_timeout\", code: \"video_poll_timeout\", status: 200},"
- 断言："{name: \"repository unavailable ready\", state: service.VideoTaskReady, err: service.ErrVideoTaskRepositoryUnavailable, code: \"video_task_unavailable\", status: 503},"
- 断言："{name: \"ready busy\", state: service.VideoTaskReady, err: service.ErrVideoTaskBusy, detail: \"settlement_pending\", code: \"video_settlement_pending\", status: 200},"


### C-054 backend/internal/handler/runtime_hardening_test.go
- 文件：backend/internal/handler/runtime_hardening_test.go
- 断言："err := &service.UpstreamFailoverError{StatusCode: 403, ResponseBody: []byte(`{\"code\":\"INSUFFICIENT_BALANCE\"}`), RetryableOnSameAccount: true}"
- 断言："func TestRuntimeHardeningBalanceSkipsSameAccountRetry(t *testing.T) {"
- 断言："err.ResponseBody = []byte(`{\"code\":\"temporarily_unavailable\"}`)"


### C-055 backend/internal/handler/setting_handler.go
- 文件：backend/internal/handler/setting_handler.go
- 断言："LongNoticeContent:                   settings.LongNoticeContent,"
- 断言："LoginCaptchaEnabled:                 settings.LoginCaptchaEnabled,"


### C-056 backend/internal/repository/channel_monitor_repo.go
- 文件：backend/internal/repository/channel_monitor_repo.go
- 断言："SET sort_order = updates.sort_order::integer"
- 断言："FROM (VALUES %s) AS updates(id, sort_order)"
- 断言："func (r *channelMonitorRepository) UpdateSortOrders(ctx context.Context, updates []service.ChannelMonitorSortOrderUpdate) error {"


### C-057 backend/internal/repository/channel_monitor_sort_test.go
- 文件：backend/internal/repository/channel_monitor_sort_test.go
- 断言："mock.ExpectExec(regexp.QuoteMeta(\"LOCK TABLE channel_monitors IN SHARE ROW EXCLUSIVE MODE\"))."
- 断言："err = repo.UpdateSortOrders(context.Background(), []service.ChannelMonitorSortOrderUpdate{"
- 断言："mock.ExpectQuery(regexp.QuoteMeta(\"SELECT COUNT(*) FROM channel_monitors\"))."


### C-058 backend/internal/repository/channel_repo.go
- 文件：backend/internal/repository/channel_repo.go
- 断言："WHERE id = $10 AND updated_at = $11`,"
- 断言："return service.ErrChannelModified"


### C-059 backend/internal/repository/channel_version_test.go
- 文件：backend/internal/repository/channel_version_test.go
- 断言："err := repo.Update(context.Background(), &service.Channel{ID: 3, UpdatedAt: time.Now(), ModelPricing: []service.ChannelModelPricing{{Models: []string{\"deepseek-v4-pro\"}}}})"
- 断言："mock.ExpectExec(`(?s)UPDATE channels.*WHERE id = \\$10 AND updated_at = \\$11`).WillReturnResult(sqlmock.NewResult(0, 0))"
- 断言："func TestChannelVersionConcurrentUpdateRollsBackBeforeReplacingPricing(t *testing.T) {"


### C-060 backend/internal/repository/integration_harness_test.go
- 文件：backend/internal/repository/integration_harness_test.go
- 断言："integrationDB, err = openSQLWithRetry(ctx, dsn, 30*time.Second)"
- 断言："if dsn := os.Getenv(\"SUB2API_TEST_DATABASE_URL\"); dsn != \"\" {"
- 断言："if err := ApplyMigrations(ctx, integrationDB); err != nil {"


### C-061 backend/internal/repository/ops_repo_recovered_requests_test.go
- 文件：backend/internal/repository/ops_repo_recovered_requests_test.go
- 断言："queryPattern := `(?s)WITH combined AS \\(.*COALESCE\\(o\\.status_code, 0\\) >= 400 OR o\\.error_phase IN \\('upstream', 'account_auth'\\).*\\)`"
- 断言："func TestListRequestDetailsKeepsRecoveredUpstreamErrorsExcludedByDefault(t *testing.T) {"
- 断言："func TestListRequestDetailsIncludesRecoveredUpstreamErrorsWhenRequested(t *testing.T) {"


### C-062 backend/internal/repository/ops_repo_request_details.go
- 文件：backend/internal/repository/ops_repo_request_details.go
- 断言："errorPredicate = \"(COALESCE(o.status_code, 0) >= 400 OR o.error_phase IN ('upstream', 'account_auth'))\""
- 断言："if filter != nil && filter.IncludeRecoveredUpstream {"
- 断言："errorPredicate := \"COALESCE(o.status_code, 0) >= 400\""


### C-063 backend/internal/repository/ops_repo_request_details_test.go
- 文件：backend/internal/repository/ops_repo_request_details_test.go
- 断言："//go:build unit"


### C-064 backend/internal/repository/usage_billing_repo.go
- 文件：backend/internal/repository/usage_billing_repo.go
- 断言："func (r *usageBillingRepository) ApplyVideo(ctx context.Context, cmd *service.UsageBillingCommand, usageLog *service.UsageLog) (*service.UsageBillingApplyResult, error) {"
- 断言："return nil, errors.New(\"atomic video billing command and usage log are required\")"
- 断言："var _ service.AtomicVideoUsageBillingRepository = (*usageBillingRepository)(nil)"


### C-065 backend/internal/repository/usage_billing_video_integration_test.go
- 文件：backend/internal/repository/usage_billing_video_integration_test.go
- 断言："RateMultiplier: 1, BillingMode: &mode, VideoCount: 1, VideoResolution: &resolution, VideoDurationSeconds: &duration,"
- 断言："UserID: user.ID, GroupID: &group.ID, Key: \"sk-atomic-video-\" + uuid.NewString(), Name: \"atomic-video\", Quota: 1,"
- 断言："repo := NewUsageBillingRepository(client, integrationDB).(service.AtomicVideoUsageBillingRepository)"


### C-066 backend/internal/repository/usage_billing_video_unit_test.go
- 文件：backend/internal/repository/usage_billing_video_unit_test.go
- 断言："func expectNewVideoBilling(mock sqlmock.Sqlmock, cmd *service.UsageBillingCommand) {"
- 断言："func TestUsageBillingVideo_ExistingUnbilledLogRollsBackCharge(t *testing.T) {"
- 断言："func TestUsageBillingVideo_InsertFailureRollsBackThenRetries(t *testing.T) {"


### C-067 backend/internal/repository/usage_log_repo_insert.go
- 文件：backend/internal/repository/usage_log_repo_insert.go
- 断言："usageSource := service.UsageSourceUpstream"
- 断言："if log.UsageSource != nil && strings.TrimSpace(*log.UsageSource) != \"\" {"
- 断言："usageSource = strings.TrimSpace(*log.UsageSource)"


### C-068 backend/internal/repository/usage_log_repo_insert_shape_unit_test.go
- 文件：backend/internal/repository/usage_log_repo_insert_shape_unit_test.go
- 断言："idx := len(prepared.args) - 6"


### C-069 backend/internal/repository/usage_log_repo_query.go
- 文件：backend/internal/repository/usage_log_repo_query.go
- 断言："UsageSource:               nullStringPtr(usageSource),"
- 断言："if !v.Valid || strings.TrimSpace(v.String) == \"\" {"
- 断言："ClientDisconnected:        clientDisconnected,"


### C-070 backend/internal/repository/usage_log_repo_request_type_test.go
- 文件：backend/internal/repository/usage_log_repo_request_type_test.go
- 断言："sql.NullString{Valid: true, String: service.UsageSourceUpstream},"
- 断言："service.UsageSourceUpstream,"
- 断言："require.Equal(t, \"boolean\", usageLogInsertArgTypes[len(usageLogInsertArgTypes)-4])"


### C-071 backend/internal/repository/usage_log_session_id_unit_test.go
- 文件：backend/internal/repository/usage_log_session_id_unit_test.go
- 断言："require.Equal(t, \"boolean\", usageLogInsertArgTypes[len(usageLogInsertArgTypes)-4],"
- 断言："require.Equal(t, \"text\", usageLogInsertArgTypes[len(usageLogInsertArgTypes)-5],"
- 断言："nsEmpty := preparedEmpty.args[len(preparedEmpty.args)-5].(sql.NullString)"


### C-072 backend/internal/repository/video_submission_repo.go
- 文件：backend/internal/repository/video_submission_repo.go
- 断言："result, err := r.db.ExecContext(ctx, `UPDATE video_submissions SET task_id=$3 WHERE api_key_id=$1 AND key_hash=$2 AND (task_id='' OR task_id=$3)`, keyID, keyHash, taskID)"
- 断言："err = r.db.QueryRowContext(ctx, `SELECT payload_hash,task_id FROM video_submissions WHERE api_key_id=$1 AND key_hash=$2`, keyID, keyHash).Scan(&fingerprint, &taskID)"
- 断言："func (r *usageBillingRepository) ClaimVideoSubmission(ctx context.Context, keyID int64, keyHash, payloadHash string) (bool, string, error) {"


### C-073 backend/internal/repository/video_submission_repo_integration_test.go
- 文件：backend/internal/repository/video_submission_repo_integration_test.go
- 断言："require.EqualValues(t, 1, claims.Load(), \"concurrent creates may submit upstream only once\")"
- 断言："require.Error(t, r.CompleteVideoSubmission(ctx, 90001, key, \"task-other\"))"
- 断言："claimed, id, err := r.ClaimVideoSubmission(ctx, 90001, key, \"request-a\")"


### C-074 backend/internal/repository/video_task_admin_repo.go
- 文件：backend/internal/repository/video_task_admin_repo.go
- 断言："func (r *usageBillingRepository) ListVideoTasks(ctx context.Context, f service.VideoTaskAdminFilter) ([]service.VideoTaskAdminItem, int64, error) {"
- 断言："CASE WHEN t.last_error IN ('poll_failed','poll_timeout','upstream_failed','upstream_cancelled','upstream_expired',"
- 断言："CASE WHEN (t.pending->>'video_duration_seconds')::numeric BETWEEN 1 AND 2147483647"


### C-075 backend/internal/repository/video_task_admin_repo_integration_test.go
- 文件：backend/internal/repository/video_task_admin_repo_integration_test.go
- 断言："_, err := integrationDB.ExecContext(ctx, `INSERT INTO video_submissions(api_key_id,key_hash,payload_hash,task_id) VALUES($1,$2,'private-payload',$3)`, key.ID, hash, taskID)"
- 断言："RoutingFingerprint: strings.Repeat(\"a\", 64), Pending: service.GrokVideoPendingBilling{Model: prefix + \"-模型%_\\\\\", VideoDurationSeconds: 5, VideoResolution: \"720p\"},"
- 断言："_, err := integrationDB.ExecContext(ctx, `INSERT INTO video_submissions(api_key_id,key_hash,payload_hash) VALUES($1,'secret-gone','secret-gone')`, deletedKey.ID)"


### C-076 backend/internal/repository/video_task_admin_repo_test.go
- 文件：backend/internal/repository/video_task_admin_repo_test.go
- 断言："items, total, err := (&usageBillingRepository{db: db}).ListVideoTasks(context.Background(), service.VideoTaskAdminFilter{Page: 1, PageSize: 20, Days: 7})"
- 断言："f := service.VideoTaskAdminFilter{Page: 3, PageSize: 100, Days: 30, Search: `%_' OR TRUE; --\\`, Status: \"billing_pending\"}"
- 断言："func TestVideoTaskAdminRepositoryBoundParametersAndEmptyPage(t *testing.T) {"


### C-077 backend/internal/repository/video_task_repo.go
- 文件：backend/internal/repository/video_task_repo.go
- 断言："return scanVideoTask(r.db.QueryRowContext(ctx, `SELECT `+videoTaskColumns+` FROM video_tasks WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3`, taskID, userID, apiKeyID))"
- 断言："func (r *usageBillingRepository) ClaimOwnedVideoTask(ctx context.Context, taskID string, userID, apiKeyID int64, lease time.Duration) (*service.VideoTask, error) {"
- 断言："func (r *usageBillingRepository) FinishVideoTaskAttempt(ctx context.Context, t *service.VideoTask, state string, delay time.Duration, errorCode string) error {"


### C-078 backend/internal/repository/video_task_repo_integration_test.go
- 文件：backend/internal/repository/video_task_repo_integration_test.go
- 断言："_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM video_tasks WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3`, task.TaskID, task.UserID, task.APIKeyID)"
- 断言："completion := &service.VideoTaskCompletion{Model: \"video-model\", BillingModel: \"video-model\", VideoResolution: \"720p\", VideoDurationSeconds: 5}"
- 断言："require.NoError(t, r.FinishVideoTaskAttempt(ctx, second, service.VideoTaskReady, 0, \"billing_failed\"))"


### C-079 backend/internal/repository/video_task_repo_test.go
- 文件：backend/internal/repository/video_task_repo_test.go
- 断言："require.ErrorIs(t, r.FinishVideoTaskAttempt(context.Background(), task, service.VideoTaskCompleted, 0, \"\"), service.ErrVideoTaskLeaseLost)"
- 断言："require.ErrorIs(t, r.SaveVideoTaskCompletion(context.Background(), task, &service.VideoTaskCompletion{}), service.ErrVideoTaskLeaseLost)"
- 断言："require.NoError(t, r.FinishVideoTaskAttempt(context.Background(), task, service.VideoTaskReady, 15*time.Second, \"billing_failed\"))"


### C-080 backend/internal/server/api_contract_test.go
- 文件：backend/internal/server/api_contract_test.go
- 断言："\"long_notice_content\": \"\","


### C-081 backend/internal/server/middleware/admin_video_preview.go
- 文件：backend/internal/server/middleware/admin_video_preview.go
- 断言："if token, err := c.Cookie(AdminVideoPreviewCookie); err == nil && token != \"\" {"
- 断言："c.FullPath() == \"/api/v1/admin/usage/:id/videos/:index/content\" &&"
- 断言："const AdminVideoPreviewCookie = \"buaga_video_preview\""


### C-082 backend/internal/server/middleware/admin_video_preview_auth_test.go
- 文件：backend/internal/server/middleware/admin_video_preview_auth_test.go
- 断言："user := &service.User{ID: 1, Email: \"video-test@example.com\", Status: service.StatusActive, Role: service.RoleAdmin, TokenVersion: 1, TokenVersionResolved: true}"
- 断言："r.Use(AdminVideoPreviewCookieAuth(), gin.HandlerFunc(NewAdminAuthMiddleware(auth, service.NewUserService(repo, nil, nil, nil), nil, nil)))"
- 断言："r.GET(\"/api/v1/admin/usage/:id/videos/:index/content\", func(c *gin.Context) { c.Status(200) })"


### C-083 backend/internal/server/middleware/admin_video_preview_test.go
- 文件：backend/internal/server/middleware/admin_video_preview_test.go
- 断言："{\"/api/v1/admin/usage/1/videos/0/content\", \"same-origin\", \"Bearer explicit\", \"Bearer explicit\"},"
- 断言："{\"/api/v1/admin/usage/1/videos/0/content\", \"same-origin\", \"\", \"Bearer test-jwt\"},"
- 断言："req.AddCookie(&http.Cookie{Name: AdminVideoPreviewCookie, Value: \"test-jwt\"})"


### C-084 backend/internal/server/routes/admin.go
- 文件：backend/internal/server/routes/admin.go
- 断言："usage.GET(\"/:id/videos/:index/content\", h.Admin.Usage.VideoContent)"
- 断言："usage.GET(\"/:id/videos/:index\", h.Admin.Usage.Video)"
- 断言："admin.GET(\"/video-tasks\", h.Admin.Usage.VideoTasks)"


### C-085 backend/internal/server/routes/gateway.go
- 文件：backend/internal/server/routes/gateway.go
- 断言："if getGroupPlatform(c) == service.PlatformGrok || getGroupPlatform(c) == service.PlatformComposite || getGroupPlatform(c) == service.PlatformOpenAI {"
- 断言："if platform := getGroupPlatform(c); platform == service.PlatformGrok || platform == service.PlatformComposite || platform == service.PlatformOpenAI {"


### C-086 backend/internal/server/routes/gateway_test.go
- 文件：backend/internal/server/routes/gateway_test.go
- 断言："func TestGatewayRoutesUnsupportedPlatformVideosAreRejectedAtPlatformGate(t *testing.T) {"
- 断言："require.Contains(t, w.Body.String(), \"Videos API is not supported for this platform\")"
- 断言："require.Equal(t, http.StatusNotFound, w.Code, \"platform=%s method=%s path=%s\", platform, tc.method, tc.path)"


### C-087 backend/internal/server/routes/openai_video_test.go
- 文件：backend/internal/server/routes/openai_video_test.go
- 断言："req := httptest.NewRequest(tc.method, prefix+tc.path, strings.NewReader(`{\"model\":\"grok-imagine-video-1.5（zj）\",\"seconds\":\"6\",\"size\":\"1280x720\"}`))"
- 断言："req := httptest.NewRequest(http.MethodPost, prefix+path, strings.NewReader(`{\"model\":\"video\"}`))"
- 断言："for _, path := range []string{\"/videos/edits\", \"/videos/extensions\"} {"


### C-088 backend/internal/service/account.go
- 文件：backend/internal/service/account.go
- 断言："OpenAIEndpointCapabilityVideos          OpenAIEndpointCapability = \"videos\""
- 断言："case OpenAIEndpointCapabilityVideos:"
- 断言："if a.Platform != PlatformOpenAI || a.Type != AccountTypeAPIKey {"


### C-089 backend/internal/service/account_test_models_display_test.go
- 文件：backend/internal/service/account_test_models_display_test.go
- 断言："gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {"
- 断言："require.Contains(t, string(response.Body), `\"display_name\":\"\"`, \"picker labels must not rewrite the shared upstream catalog\")"
- 断言："for i, name := range []string{\"custom-one\", \"custom-two\", \"custom-three\", \"Friendly name\"} {"


### C-090 backend/internal/service/account_test_service.go
- 文件：backend/internal/service/account_test_service.go
- 断言："AllowedHosts:           s.cfg.Security.URLAllowlist.UpstreamHosts,"
- 断言："return urlvalidator.ValidateURLFormat(raw, true, s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts)"
- 断言："AllowInsecureHTTPHosts: s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts,"


### C-091 backend/internal/service/admin_video_preview.go
- 文件：backend/internal/service/admin_video_preview.go
- 断言："func (s *OpenAIGatewayService) adminVideoPreviewFallback(c *gin.Context, account *Account, requestID, canonical, token, proxyURL string) (*http.Response, error) {"
- 断言："statusReq, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(c.Request.Context()), http.MethodGet, statusURL, nil)"
- 断言："req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(c.Request.Context()), http.MethodGet, target, nil)"


### C-092 backend/internal/service/admin_video_preview_test.go
- 文件：backend/internal/service/admin_video_preview_test.go
- 断言："account := &Account{ID: 493, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{\"base_url\": \"https://video.example/v1\", \"api_key\": \"test-secret\"}}"
- 断言："return &http.Response{StatusCode: 206, Header: http.Header{\"Content-Type\": {\"video/mp4\"}}, Body: io.NopCloser(strings.NewReader(\"0123456789abcdef\")), ContentLength: 16}, nil"
- 断言："svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &adminVideoTransport{do: func(r *http.Request) (*http.Response, error) {"


### C-093 backend/internal/service/channel_monitor_quota_mode_test.go
- 文件：backend/internal/service/channel_monitor_quota_mode_test.go
- 断言："Endpoint: \"https://8.8.8.8\", IntervalSeconds: 60, PrimaryModel: \"gpt-5\","
- 断言："Endpoint: \"https://8.8.8.8\", APIKey: \"sk\","


### C-094 backend/internal/service/channel_monitor_service.go
- 文件：backend/internal/service/channel_monitor_service.go
- 断言："func (s *ChannelMonitorService) UpdateSortOrders(ctx context.Context, updates []ChannelMonitorSortOrderUpdate) error {"
- 断言："func (s *ChannelMonitorService) ListAll(ctx context.Context) ([]*ChannelMonitor, error) {"
- 断言："UpdateSortOrders(ctx context.Context, updates []ChannelMonitorSortOrderUpdate) error"


### C-095 backend/internal/service/channel_monitor_sort_test.go
- 文件：backend/internal/service/channel_monitor_sort_test.go
- 断言："func (r *channelMonitorSortRepoStub) UpdateSortOrders(_ context.Context, updates []ChannelMonitorSortOrderUpdate) error {"
- 断言："{name: \"duplicate id\", updates: []ChannelMonitorSortOrderUpdate{{ID: 1}, {ID: 1, SortOrder: 10}}},"
- 断言："err := service.UpdateSortOrders(context.Background(), []ChannelMonitorSortOrderUpdate{{ID: 1}})"


### C-096 backend/internal/service/channel_monitor_types.go
- 文件：backend/internal/service/channel_monitor_types.go
- 断言："type ChannelMonitorSortOrderUpdate struct {"
- 断言："SortOrder       int"


### C-097 backend/internal/service/channel_service.go
- 文件：backend/internal/service/channel_service.go
- 断言："ErrChannelModified       = infraerrors.Conflict(\"CHANNEL_MODIFIED\", \"渠道配置已更新或页面版本过旧，请关闭编辑窗口并刷新页面后重新编辑，避免覆盖最新定价。\")"
- 断言："if input.ExpectedUpdatedAt != nil && !channel.UpdatedAt.Equal(*input.ExpectedUpdatedAt) {"
- 断言："ExpectedUpdatedAt          *time.Time"


### C-098 backend/internal/service/channel_version_test.go
- 文件：backend/internal/service/channel_version_test.go
- 断言："_, err := newTestChannelService(repo).Update(context.Background(), 3, &UpdateChannelInput{ExpectedUpdatedAt: &stale})"
- 断言："repo := &mockChannelRepository{getByIDFn: func(context.Context, int64) (*Channel, error) {"
- 断言："current := time.Date(2026, 9, 14, 7, 0, 0, 123456000, time.UTC)"


### C-099 backend/internal/service/cn_provider_probe_url.go
- 文件：backend/internal/service/cn_provider_probe_url.go
- 断言："AllowedHosts:           cfg.Security.URLAllowlist.UpstreamHosts,"
- 断言："normalized, err := urlvalidator.ValidateURLFormat(trimmed, allowInsecureHTTP, allowInsecureHTTPHosts)"
- 断言："allowInsecureHTTPHosts = cfg.Security.URLAllowlist.AllowInsecureHTTPHosts"


### C-100 backend/internal/service/concurrency_service_test.go
- 文件：backend/internal/service/concurrency_service_test.go
- 断言："require.Equal(t, RequestIDPrefix(), cache.cleanupPrefix)"
- 断言："ProvideConcurrencyService(cache, nil, cfg)"
- 断言："require.Empty(t, cache.cleanupPrefix)"


### C-101 backend/internal/service/crs_sync_service.go
- 文件：backend/internal/service/crs_sync_service.go
- 断言："normalized, err := urlvalidator.ValidateURLFormat(normalizedURL, s.cfg.Security.URLAllowlist.AllowInsecureHTTP, s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts)"


### C-102 backend/internal/service/domain_constants.go
- 文件：backend/internal/service/domain_constants.go
- 断言："SettingKeyLongNoticeContent = \"long_notice_content\""


### C-103 backend/internal/service/gateway_anthropic_passthrough.go
- 文件：backend/internal/service/gateway_anthropic_passthrough.go
- 断言："return nil, &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte(`{\"error\":{\"message\":\"Model recovery probe in progress\"}}`)}"
- 断言："s.recordAnthropicStreamFailure(ctx, account, input.RequestModel, &anthropicStreamFailure{reason: \"upstream_http_error\", transient: true})"
- 断言："return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, upstreamStreamErr"


### C-104 backend/internal/service/gateway_anthropic_stream_recovery.go
- 文件：backend/internal/service/gateway_anthropic_stream_recovery.go
- 断言："d := s.anthropicStreamRecovery().recordFailureWithInitialCooldown(account.ID, model, time.Now(), 0,"
- 断言："if !anthropicStreamRecoveryAccount(account) || ctx.Err() != nil || !errors.As(err, &failure) || !failure.transient {"
- 断言："logger.FromContext(ctx).Warn(\"gateway.anthropic_model_stream_cooldown\", zap.Int64(\"account_id\", account.ID),"


### C-105 backend/internal/service/gateway_anthropic_stream_recovery_test.go
- 文件：backend/internal/service/gateway_anthropic_stream_recovery_test.go
- 断言："state.recordFailureWithInitialCooldown(1, \"claude-a\", time.Now().Add(-20*time.Second), time.Second, openAITransientFailureOptions{requireProbe: true})"
- 断言："require.Equal(t, kind == \"invalid_request_error\", svc.isAccountSchedulableForModelSelection(context.Background(), account, \"claude-a\"))"
- 断言："result, err = svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, account, time.Now(), \"claude-a\")"


### C-106 backend/internal/service/gateway_bedrock.go
- 文件：backend/internal/service/gateway_bedrock.go
- 断言："setObservedResponseBytes(c, responseBodyBytesForUsageEstimate(body))"
- 断言："setUsagePresent(c, claudeUsagePresentInJSON(body))"


### C-107 backend/internal/service/gateway_forward.go
- 文件：backend/internal/service/gateway_forward.go
- 断言："return nil, &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte(`{\"error\":{\"message\":\"Model recovery probe in progress\"}}`)}"
- 断言："s.recordAnthropicStreamFailure(ctx, account, recoveryModel, &anthropicStreamFailure{reason: \"upstream_http_error\", transient: true})"
- 断言："if errors.As(err, &failover) && !failover.RequestScopedTransient && failover.StatusCode >= 500 {"


### C-108 backend/internal/service/gateway_scheduling.go
- 文件：backend/internal/service/gateway_scheduling.go
- 断言："if anthropicStreamRecoveryAccount(account) && s.anthropicStreamRecovery().isBlocked(account.ID, canonicalOpenAIAccountSchedulingModel(account, requestedModel), time.Now()) {"


### C-109 backend/internal/service/gateway_service.go
- 文件：backend/internal/service/gateway_service.go
- 断言："sessionLimitCache            SessionLimitCache // 会话数量限制缓存（仅 Anthropic OAuth/SetupToken）"
- 断言："func (r *AccountSelectionResult) CompleteTransient(success bool, observedErr ...error) {"
- 断言："rpmCache                     RPMCache          // RPM 计数缓存（仅 Anthropic OAuth/SetupToken）"


### C-110 backend/internal/service/gateway_upstream_request.go
- 文件：backend/internal/service/gateway_upstream_request.go
- 断言："AllowedHosts:           s.cfg.Security.URLAllowlist.UpstreamHosts,"
- 断言："normalized, err := urlvalidator.ValidateURLFormat(raw, true, s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts)"
- 断言："AllowInsecureHTTPHosts: s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts,"


### C-111 backend/internal/service/gateway_upstream_response.go
- 文件：backend/internal/service/gateway_upstream_response.go
- 断言："return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, incompleteAnthropicStream(\"usage incomplete: missing terminal event\")"
- 断言："return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, incompleteAnthropicStream(\"read error\")"
- 断言："s.recordAnthropicStreamFailure(ctx, account, mappedModel, failure)"


### C-112 backend/internal/service/gateway_usage_billing.go
- 文件：backend/internal/service/gateway_usage_billing.go
- 断言："UsageSource:              optionalTrimmedStringPtr(result.UsageSource),"
- 断言："ClientDisconnected:       result.ClientDisconnect,"


### C-113 backend/internal/service/gemini_chat_completions_compat_service.go
- 文件：backend/internal/service/gemini_chat_completions_compat_service.go
- 断言："if err := waitGeminiBackoff(ctx, attempt); err != nil { return nil, err }"
- 断言："if errors.Is(err, context.Canceled) { return nil, err }"
- 断言："if ctx.Err() != nil { return nil, ctx.Err() }"


### C-114 backend/internal/service/gemini_messages_compat_service.go
- 文件：backend/internal/service/gemini_messages_compat_service.go
- 断言："AllowedHosts:           s.cfg.Security.URLAllowlist.UpstreamHosts,"
- 断言："normalized, err := urlvalidator.ValidateURLFormat(raw, true, s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts)"
- 断言："AllowInsecureHTTPHosts: s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts,"


### C-115 backend/internal/service/grok_media.go
- 文件：backend/internal/service/grok_media.go
- 断言："writeGrokMediaErrorResponse(c, http.StatusBadGateway, \"invalid_video_response\", \"Video upstream returned unsupported or conflicting billing parameters\")"
- 断言："writeGrokMediaErrorResponse(c, http.StatusServiceUnavailable, \"video_pricing_unavailable\", \"Video pricing is unavailable; no video was submitted\")"
- 断言："return s.forwardGrokMedia(ctx, c, account, GrokMediaEndpointVideosGenerations, \"\", body, contentType, saveTask, beforeSubmit...)"


### C-116 backend/internal/service/grok_upstream_url.go
- 文件：backend/internal/service/grok_upstream_url.go
- 断言："AllowedHosts:           cfg.Security.URLAllowlist.UpstreamHosts,"
- 断言："return buildOpenAIVideoURL(account, cfg, endpoint, requestID)"
- 断言："return urlvalidator.ValidateURLFormat(raw, true, cfg.Security.URLAllowlist.AllowInsecureHTTPHosts)"


### C-117 backend/internal/service/image_preview.go
- 文件：backend/internal/service/image_preview.go
- 断言："for _, path := range []string{\"video.url\", \"video_url\", \"url\", \"download_url\", \"output.video_url\", \"output.url\", \"data.0.video.url\", \"data.0.video_url\", \"data.0.url\"} {"
- 断言："SaveOpenAIImagePreviewsAsync(ctx, OpenAIImagePreviewAssetsFromContext(c), ResolveUsageRecordID(ctx, result.RequestID), getAPIKeyIDFromContext(c))"
- 断言："func SaveOpenAIImagePreviewsAsync(ctx context.Context, assets []OpenAIImagePreviewAsset, requestID string, apiKeyID int64) {"


### C-118 backend/internal/service/login_captcha.go
- 文件：backend/internal/service/login_captcha.go
- 断言："func (s *AuthService) VerifyCaptchaForLogin(ctx context.Context, proof CaptchaProof, remoteIP string) error {"
- 断言："settings, err := s.settingService.settingRepo.GetMultiple(ctx, []string{SettingKeyLoginCaptchaEnabled})"
- 断言："if s.cfg != nil && s.cfg.Server.Mode == \"release\" && s.cfg.Turnstile.Required {"


### C-119 backend/internal/service/login_captcha_test.go
- 文件：backend/internal/service/login_captcha_test.go
- 断言："svc := newAuthServiceForCaptchaRepoTest(&settingRepoStub{err: errors.New(\"unavailable\")}, false, &turnstileVerifierSpy{}, nil)"
- 断言："require.ErrorIs(t, svc.VerifyCaptchaForLogin(context.Background(), CaptchaProof{}, \"\"), ErrServiceUnavailable)"
- 断言："require.Error(t, svc.VerifyCaptchaForRegister(context.Background(), CaptchaProof{}, \"203.0.113.1\", \"\"))"


### C-120 backend/internal/service/long_notice_regression_test.go
- 文件：backend/internal/service/long_notice_regression_test.go
- 断言："svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{SettingKeyLongNoticeContent:notice}}, &config.Config{})"
- 断言："require.NoError(t,svc.UpdateSettings(context.Background(),&SystemSettings{LongNoticeContent:notice}))"
- 断言："require.NoError(t,svc.UpdateSettings(context.Background(),&SystemSettings{LongNoticeContent:\"\"}))"


### C-121 backend/internal/service/model_plaza_catalog.go
- 文件：backend/internal/service/model_plaza_catalog.go
- 断言："func (s *ModelPlazaService) applyPlazaCatalog(ctx context.Context, pg *PlazaGroup, g *Group, entry PlazaCatalogGroup) {"
- 断言："pricing.Intervals = append(pricing.Intervals, PricingInterval{TierLabel: size, PerRequestPrice: &price, SortOrder: i})"
- 断言："cost := gateway.calculateOpenAIImageCost(ctx, name, key, &OpenAIForwardResult{ImageCount: 1, ImageSize: size}, 1)"


### C-122 backend/internal/service/model_plaza_catalog_test.go
- 文件：backend/internal/service/model_plaza_catalog_test.go
- 断言："groups[0].ModelPricing = []ChannelModelPricing{{Models: []string{\"gpt-image-2\"}, BillingMode: BillingModePerRequest, PerRequestPrice: testPtrFloat64(0.04)}}"
- 断言："out, err := svc.ListCatalogGroups(context.Background(), []PlazaCatalogGroup{{GroupID: 10, Models: []string{\"gpt-5.5\", \"gpt-5.6-sol\"}}})"
- 断言："svc := newPlazaServiceWithBilling(channels, groups, map[int64]string{10: PlatformAnthropic, 20: PlatformAnthropic}, nil)"


### C-123 backend/internal/service/model_plaza_service.go
- 文件：backend/internal/service/model_plaza_service.go
- 断言："func (s *ModelPlazaService) ListCatalogGroups(ctx context.Context, catalog []PlazaCatalogGroup) ([]PlazaGroup, error) {"
- 断言："entry, ok := plazaCatalogEntry(catalog, gid)"
- 断言："allowedModels = append(allowedModels, model)"


### C-124 backend/internal/service/openai_account_model_transient.go
- 文件：backend/internal/service/openai_account_model_transient.go
- 断言："func (s *openAIAccountModelTransientState) completeProbe(accountID int64, model string, id uint64, now time.Time) bool {"
- 断言："if !entry.requireProbe && (entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL) {"
- 断言："func (s *openAIAccountModelTransientState) reserveProbe(accountID int64, model string, now time.Time) (bool, uint64) {"


### C-125 backend/internal/service/openai_account_model_transient_test.go
- 文件：backend/internal/service/openai_account_model_transient_test.go
- 断言："assert.Empty(t, state.snapshot(now.Add(openAIModelTransientShortCooldown+2*time.Second)))"
- 断言："func TestOpenAIModelTransient_SnapshotIncludesActiveCooldowns(t *testing.T) {"
- 断言："func TestOpenAIModelTransient_SnapshotOmitsExpiredCooldowns(t *testing.T) {"


### C-126 backend/internal/service/openai_account_runtime_block_fastpath.go
- 文件：backend/internal/service/openai_account_runtime_block_fastpath.go
- 断言："func (s *OpenAIGatewayService) recordOpenAIAccountTransientFailure(account *Account, statusCode int, responseBody []byte, now time.Time) openAIAccountTransientDecision {"
- 断言："func (s *OpenAIGatewayService) finalizeOpenAIAccountTransientSelection(selection *AccountSelectionResult, requestedModel ...string) (*AccountSelectionResult, bool) {"
- 断言："decision := s.recordOpenAIAccountModelTransientFailureWithInitialCooldown(account, model, time.Now(), initialCooldown, fmt.Sprintf(\"upstream_%d\", statusCode))"


### C-127 backend/internal/service/openai_account_runtime_block_fastpath_test.go
- 文件：backend/internal/service/openai_account_runtime_block_fastpath_test.go
- 断言："func TestOpenAIPoolModeRetryable5xx_CreatesModelTransientBlock(t *testing.T) {"
- 断言："shouldDisable := gateway.handleOpenAIAccountUpstreamError("
- 断言："[]byte(`{\"error\":{\"message\":\"upstream timeout\"}}`),"


### C-128 backend/internal/service/openai_account_scheduler.go
- 文件：backend/internal/service/openai_account_scheduler.go
- 断言："snapshot.ModelCooldowns = append(snapshot.ModelCooldowns, s.getOpenAIAccountTransientState().snapshot(now)...)"
- 断言："delay := s.nextOpenAITransientRetryDelay(ctx, groupID, requestedModel, requiredCapability, requireCompact)"
- 断言："selection, ok := s.service.finalizeOpenAIAccountTransientSelection(selection, req.RequestedModel)"


### C-129 backend/internal/service/openai_account_transient.go
- 文件：backend/internal/service/openai_account_transient.go
- 断言："func (s *openAIAccountTransientState) observeModelFailure(accountID int64, model string, status int, now time.Time) openAIAccountTransientDecision {"
- 断言："func (s *openAIAccountTransientState) recordFailureLocked(accountID int64, now time.Time, reason ...string) openAIAccountTransientDecision {"
- 断言："func (s *openAIAccountTransientState) recordFailure(accountID int64, now time.Time, reason ...string) openAIAccountTransientDecision {"


### C-130 backend/internal/service/openai_account_transient_test.go
- 文件：backend/internal/service/openai_account_transient_test.go
- 断言："for _, cause := range []error{nil, context.Canceled, bufio.ErrTooLong, ErrUpstreamResponseBodyTooLarge, io.ErrUnexpectedEOF, &net.OpError{Op: \"dial\", Err: io.EOF}} {"
- 断言："require.False(t, svc.handleOpenAIAccountUpstreamError(context.Background(), &account, http.StatusBadGateway, http.Header{}, []byte(\"upstream failed\"), \"gpt-5.1\"))"
- 断言："selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, \"\", \"pool-recovery\", \"gpt-5.1\", nil, OpenAIUpstreamTransportAny, false)"


### C-131 backend/internal/service/openai_apikey_availability_latency_test.go
- 文件：backend/internal/service/openai_apikey_availability_latency_test.go
- 断言："resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{\"Content-Type\": {\"text/event-stream\"}}, Body: body}"
- 断言："const availabilityLatencyText = \"data: {\\\"type\\\":\\\"response.output_text.delta\\\",\\\"delta\\\":\\\"first real text\\\"}\\n\\n\""
- 断言："w := &availabilityFirstTextRecorder{ResponseRecorder: httptest.NewRecorder(), firstTextFlushed: flushed}"


### C-132 backend/internal/service/openai_apikey_availability_test.go
- 文件：backend/internal/service/openai_apikey_availability_test.go
- 断言："svc.handleOpenAIAccountUpstreamError(context.Background(), account, 503, http.Header{}, []byte(`{\"error\":{\"message\":\"upstream unavailable\"}}`), \"gpt-test\")"
- 断言："body := fmt.Sprintf(\"data: {\\\"type\\\":\\\"error\\\",\\\"error\\\":{\\\"status_code\\\":%d,\\\"type\\\":\\\"server_error\\\",\\\"message\\\":\\\"Upstream unavailable\\\"}}\\n\\n\", status)"
- 断言："disabled := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusServiceUnavailable, http.Header{}, []byte(body), \"upstream-a\")"


### C-133 backend/internal/service/openai_availability_transport.go
- 文件：backend/internal/service/openai_availability_transport.go
- 断言："decision := s.recordOpenAIAccountModelTransientFailureWithInitialCooldown(account, model, time.Now(), openAIModelTransientShortCooldown, \"upstream_transport\")"
- 断言："decision := s.recordOpenAIAccountModelTransientFailureWithInitialCooldown(account, model, time.Now(), openAIModelTransientShortCooldown)"
- 断言："\"cooldown_ms\", decision.Cooldown.Milliseconds(), \"block_scope\", \"account_model\", \"cause\", \"upstream_transport\")"


### C-134 backend/internal/service/openai_availability_transport_test.go
- 文件：backend/internal/service/openai_availability_transport_test.go
- 断言："for _, cause := range []error{context.Canceled, io.ErrUnexpectedEOF, context.DeadlineExceeded, ErrUpstreamResponseBodyTooLarge} {"
- 断言："require.False(t, state.isBlocked(account.ID, unaffected, time.Now()), \"must cool only the actual upstream model\")"
- 断言："require.EqualValues(t, 1, settings.calls.Load(), \"native idle failover must preserve administrator policy\")"


### C-135 backend/internal/service/openai_first_output_timeout.go
- 文件：backend/internal/service/openai_first_output_timeout.go
- 断言："if !s.openAIAPIKeyAvailabilityEnabled(account) || !openAIAvailabilityCallerGone(ctx, c, nil) {"
- 断言："s.recordOpenAIAvailabilityTransportFailure(ctx, c, account, model, context.DeadlineExceeded)"
- 断言："if len(canonicalModel) > 0 && strings.TrimSpace(canonicalModel[0]) != \"\" {"


### C-136 backend/internal/service/openai_gateway_chat_completions.go
- 文件：backend/internal/service/openai_gateway_chat_completions.go
- 断言："s.finalizeUsageEstimate(ctx, c, account, requestBytes, result, err)"
- 断言："requestBytes := requestBodyBytesForUsageEstimate(body)"
- 断言：") (result *OpenAIForwardResult, err error) {"


### C-137 backend/internal/service/openai_gateway_chat_completions_anthropic_native.go
- 文件：backend/internal/service/openai_gateway_chat_completions_anthropic_native.go
- 断言："setUsagePresent(c, true)"


### C-138 backend/internal/service/openai_gateway_chat_completions_raw.go
- 文件：backend/internal/service/openai_gateway_chat_completions_raw.go
- 断言："ObservedResponseBytes:         responseBodyBytesForUsageEstimate(respBody),"
- 断言："observedBytes += responseBodyBytesForUsageEstimate([]byte(payload))"
- 断言："ClientDisconnect:              clientDisconnected,"


### C-139 backend/internal/service/openai_gateway_forward.go
- 文件：backend/internal/service/openai_gateway_forward.go
- 断言："return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false, upstreamModel)"
- 断言："defer func() { persistCapturedImagePreviews(ctx, c, result) }()"
- 断言："firstOutputTimeout, \"response_headers\", nil, upstreamModel,"


### C-140 backend/internal/service/openai_gateway_messages.go
- 文件：backend/internal/service/openai_gateway_messages.go
- 断言："s.finalizeUsageEstimate(ctx, c, account, requestBytes, out, err)"
- 断言："requestBytes := requestBodyBytesForUsageEstimate(body)"
- 断言：") (out *OpenAIForwardResult, err error) {"


### C-141 backend/internal/service/openai_gateway_messages_anthropic_native.go
- 文件：backend/internal/service/openai_gateway_messages_anthropic_native.go
- 断言："setObservedResponseBytes(c, usageObservedResponseBytes(c)+responseBodyBytesForUsageEstimate([]byte(trimmed)))"
- 断言："setObservedResponseBytes(c, responseBodyBytesForUsageEstimate(body))"
- 断言："setUsagePresent(c, claudeUsagePresentInJSON([]byte(trimmed)))"


### C-142 backend/internal/service/openai_gateway_passthrough.go
- 文件：backend/internal/service/openai_gateway_passthrough.go
- 断言："if shouldFailoverOpenAIPassthroughResponse(account, compactResp.StatusCode, compactBody) || s.openAIAPIKeyCredentialFailure(account, compactResp.StatusCode, compactBody) {"
- 断言："s.newOpenAIStreamFailoverErrorWithModel(c, account, true, upstreamRequestID, dataBytes, failedMessage, terminalSchedulingModel(dataBytes, failedMessage), resp.Header)"
- 断言："s.newOpenAIAvailabilityStreamTransportError(ctx, c, account, true, upstreamRequestID, openAIAvailabilityModel(account, originalModel, mappedModel), msg, err)"


### C-143 backend/internal/service/openai_gateway_request_body.go
- 文件：backend/internal/service/openai_gateway_request_body.go
- 断言："AllowedHosts:           s.cfg.Security.URLAllowlist.UpstreamHosts,"
- 断言："return urlvalidator.ValidateURLFormat(raw, true, s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts)"
- 断言："AllowInsecureHTTPHosts: s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts,"


### C-144 backend/internal/service/openai_gateway_response_handling.go
- 文件：backend/internal/service/openai_gateway_response_handling.go
- 断言："return resultWithUsage(), s.newOpenAIAvailabilityStreamTransportError(ctx, c, account, false, upstreamRequestID,"
- 断言："shouldFailover = shouldFailover || s.isOpenAIAPIKeyTransientStreamFailure(account, dataBytes, failedMessage)"
- 断言："if s.isOpenAIAPIKeyTransientStreamFailure(account, payload, message) {"


### C-145 backend/internal/service/openai_gateway_responses_anthropic_native.go
- 文件：backend/internal/service/openai_gateway_responses_anthropic_native.go
- 断言："setUsagePresent(c, true)"


### C-146 backend/internal/service/openai_gateway_scheduling.go
- 文件：backend/internal/service/openai_gateway_scheduling.go
- 断言："finalized, ok := s.finalizeOpenAIAccountTransientSelection(selection, openAITransientRequestedModel(ctx))"
- 断言："if s.hasOpenAIAccountTransientState(account.ID) && isOpenAIAccountTransientCircuitCandidate(account) {"
- 断言："if stickyAccountID > 0 && stickyAccountID == account.ID && s.concurrencyService != nil {"


### C-147 backend/internal/service/openai_gateway_service.go
- 文件：backend/internal/service/openai_gateway_service.go
- 断言："openaiAccountTransient         *openAIAccountTransientState"
- 断言："UsageSource           string // upstream 或 estimated"
- 断言："videoTaskProcessor    *videoTaskProcessor"


### C-148 backend/internal/service/openai_gateway_upstream_errors.go
- 文件：backend/internal/service/openai_gateway_upstream_errors.go
- 断言："(openAIAPIKeyAvailabilityTransientStatus(statusCode) || statusCode == http.StatusTooManyRequests || s.openAIAPIKeyCredentialFailure(account, statusCode, responseBody)) {"
- 断言："if isOpenAIUpstreamAccessStateError(\"\", body) {"
- 断言："failoverErr.RetryableOnSameAccount = false"


### C-149 backend/internal/service/openai_gateway_usage.go
- 文件：backend/internal/service/openai_gateway_usage.go
- 断言："return s.calculateVideoCostForPlatform(apiKey, billingModel, resolution, videoCount, durationSeconds, groupConfig, multiplier)"
- 断言："videoDurationSeconds := NormalizeVideoDurationForPlatform(apiKey.Group, result.VideoDurationSeconds)"
- 断言："if input.RequireAtomicVideo && input.Account != nil && input.Account.Platform == PlatformOpenAI {"


### C-150 backend/internal/service/openai_images.go
- 文件：backend/internal/service/openai_images.go
- 断言："defer func() { persistCapturedImagePreviews(ctx, c, result) }()"
- 断言："if videoURL := ExtractVideoPreviewURL(body); videoURL != \"\" {"
- 断言："CaptureOpenAIImagePreviewResponse(c, dataBytes)"


### C-151 backend/internal/service/openai_images_responses.go
- 文件：backend/internal/service/openai_images_responses.go
- 断言："CaptureOpenAIImagePreviewResponse(c, responseBody)"
- 断言："CaptureOpenAIImagePreviewResponse(c, dataBytes)"


### C-152 backend/internal/service/openai_transient_recovery.go
- 文件：backend/internal/service/openai_transient_recovery.go
- 断言："func (r *openAITransientRecovery) success(id uint64) (accepted, recovered bool) {"
- 断言："func (r *openAITransientRecovery) retryAt(blockUntil, now time.Time) time.Time {"
- 断言："func (r *openAITransientRecovery) state(blockUntil, now time.Time) string {"


### C-153 backend/internal/service/openai_transient_refinement_test.go
- 文件：backend/internal/service/openai_transient_refinement_test.go
- 断言："selection, ok := svc.finalizeOpenAIAccountTransientSelection(&AccountSelectionResult{Account: &account, Acquired: true, ReleaseFunc: func() { releases++ }}, \"a\")"
- 断言："selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, \"\", \"\", \"gpt-5.1\", nil, OpenAIUpstreamTransportAny, false)"
- 断言："svc.handleOpenAIAccountUpstreamError(context.Background(), &account, 502, http.Header{}, []byte(\"upstream failure\"), \"model-a\")"


### C-154 backend/internal/service/openai_transient_wait.go
- 文件：backend/internal/service/openai_transient_wait.go
- 断言："model, _ := ctx.Value(openAITransientRequestedModelKey{}).(string)"
- 断言："func openAITransientRequestedModel(ctx context.Context) string {"
- 断言："if !isOpenAIAccountTransientCircuitCandidate(account) {"


### C-155 backend/internal/service/openai_upstream_transport_error.go
- 文件：backend/internal/service/openai_upstream_transport_error.go
- 断言："s.recordOpenAIAvailabilityTransportFailure(ctx, c, account, firstNonEmpty(canonicalModel...), err)"
- 断言："if s.openAIAPIKeyAvailabilityEnabled(account) && openAIAvailabilityCallerGone(ctx, c, err) {"


### C-156 backend/internal/service/openai_video.go
- 文件：backend/internal/service/openai_video.go
- 断言："if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey || !supportsOpenAIVideoEndpoint(endpoint) {"
- 断言："func prepareOpenAIVideoBody(account *Account, body []byte, contentType string) ([]byte, string, GrokMediaRequestInfo, string, error) {"
- 断言："if len(accounts) > 0 && accounts[0] != nil && accounts[0].Platform == PlatformOpenAI && !isOpenAIGrokVideoProtocol(accounts[0]) {"


### C-157 backend/internal/service/openai_video_control_validation_test.go
- 文件：backend/internal/service/openai_video_control_validation_test.go
- 断言："upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{\"id\":\"unexpected\",\"status\":\"queued\"}`)}"
- 断言："c, recorder := grokMediaContentTestContext(http.MethodPost, \"https://api.example/v1/videos\", nil)"
- 断言："_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, account, body, contentType,"


### C-158 backend/internal/service/openai_video_duration_test.go
- 文件：backend/internal/service/openai_video_duration_test.go
- 断言："require.ErrorContains(t, validateOpenAIVideoResponseBilling([]byte(`{\"status\":\"completed\",\"seconds\":\"16\"}`), account), \"1 and 15\")"
- 断言："result, err = svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoStatus, \"native-30\", nil, \"\")"
- 断言："c, recorder := grokMediaContentTestContext(http.MethodGet, \"https://api.example/v1/videos/native-30\", nil)"


### C-159 backend/internal/service/openai_video_grok_compat.go
- 文件：backend/internal/service/openai_video_grok_compat.go
- 断言："for _, path := range []string{\"video.url\", \"url\", \"video_url\", \"result_url\", \"download_url\"} {"
- 断言："func normalizeOpenAIVideoResponse(account *Account, body []byte) []byte {"
- 断言："return normalizeZhongYVideoResponse(body)"


### C-160 backend/internal/service/openai_video_grok_compat_test.go
- 文件：backend/internal/service/openai_video_grok_compat_test.go
- 断言："{StatusCode: http.StatusPartialContent, Header: http.Header{\"Content-Type\": {\"video/mp4\"}, \"Content-Range\": {\"bytes 0-3/100\"}}, Body: io.NopCloser(strings.NewReader(\"mp4!\"))},"
- 断言："c, recorder = grokMediaContentTestContext(http.MethodGet, \"https://api.example/v1/videos/paid-task/content\", map[string]string{\"Range\": \"bytes=0-3\"})"
- 断言："body := []byte(`{ \"id\":\"task-1\", \"status\":\"completed\", \"model\":\"provider-video\", \"seconds\":\"8\", \"size\":\"1280x720\", \"metadata\":{\"keep\":true} }`)"


### C-161 backend/internal/service/openai_video_legacy_billing_test.go
- 文件：backend/internal/service/openai_video_legacy_billing_test.go
- 断言："result := &OpenAIForwardResult{RequestID: \"legacy-video-duration\", Model: \"custom-video\", VideoCount: 1, VideoResolution: \"720p\", VideoDurationSeconds: 30}"
- 断言："key := &APIKey{ID: 2, UserID: 1, GroupID: &groupID, Group: &Group{ID: groupID, Platform: platform, RateMultiplier: 1, VideoPrice720P: &price}}"
- 断言："cost := svc.calculateOpenAIVideoCost(context.Background(), result.Model, key, result, 1)"


### C-162 backend/internal/service/openai_video_model_validation_test.go
- 文件：backend/internal/service/openai_video_model_validation_test.go
- 断言："for _, models := range [][2]string{{\"cheap-video\", \"expensive-video\"}, {\"expensive-video\", \"cheap-video\"}, {\"cheap-video\", \"cheap-video\"}} {"
- 断言："upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{\"id\":\"unexpected\",\"status\":\"queued\"}`)}"
- 断言："c, recorder := grokMediaContentTestContext(http.MethodPost, \"https://api.example/v1/videos\", nil)"


### C-163 backend/internal/service/openai_video_multipart_test.go
- 文件：backend/internal/service/openai_video_multipart_test.go
- 断言："body, contentType := videoMultipartTestBody(t, []videoMultipartTestPart{videoMultipartTestField(\"model\", \"client-video\")})"
- 断言："func videoMultipartTestPrepare(t *testing.T, parts []videoMultipartTestPart, protocol string) []videoMultipartTestPart {"
- 断言："require.Equal(t, []videoMultipartTestPart{part}, videoMultipartTestPrepare(t, []videoMultipartTestPart{part}, \"grok\"))"


### C-164 backend/internal/service/openai_video_resolution.go
- 文件：backend/internal/service/openai_video_resolution.go
- 断言："func prepareOpenAIVideoResolution(account *Account, body []byte, contentType string, info GrokMediaRequestInfo) ([]byte, string, GrokMediaRequestInfo, error) {"
- 断言："func parseOpenAIVideoResolutionBody(body []byte, contentType string) (*openAIVideoResolutionBody, error) {"
- 断言："return fmt.Errorf(\"unsupported video %s: expected a 480p, 720p, 768p, 1080p or 1440p billing tier\", name)"


### C-165 backend/internal/service/openai_video_resolution_test.go
- 文件：backend/internal/service/openai_video_resolution_test.go
- 断言："out, outType, info, err := prepareOpenAIVideoResolution(account, body, contentType, GrokMediaRequestInfo{Resolution: \"1080p\"})"
- 断言："out, outType, info, err := prepareOpenAIVideoResolution(account, body, contentType, GrokMediaRequestInfo{Size: tc.size})"
- 断言："info := GrokMediaRequestInfo{Size: tc.size, Resolution: \"480p\", Model: \"client-video\", DurationSeconds: 6}"


### C-166 backend/internal/service/openai_video_safety.go
- 文件：backend/internal/service/openai_video_safety.go
- 断言："func (s *OpenAIGatewayService) GetOpenAIVideoLookupAccount(ctx context.Context, boundAccountID int64) (*Account, error) {"
- 断言："setOpsUpstreamError(c, 0, \"Video creation outcome is unknown; automatic retry suppressed\", \"\")"
- 断言："func openAIVideoCreateUnknown(c *gin.Context, taskID string, cause error) error {"


### C-167 backend/internal/service/openai_video_safety_test.go
- 文件：backend/internal/service/openai_video_safety_test.go
- 断言："[]byte(`{\"model\":\"video\",\"seconds\":\"6\",\"size\":\"1280x720\"}`), \"application/json\", func(*OpenAIForwardResult) error { return errors.New(\"database unavailable\") })"
- 断言："[]byte(`{\"model\":\"video\",\"seconds\":\"6\",\"size\":\"1280x720\"}`), \"application/json\", func(*OpenAIForwardResult) error { return nil })"
- 断言："func (s *openAIVideoTransportFailure) Do(*http.Request, string, int64, int) (*http.Response, error) {"


### C-168 backend/internal/service/openai_video_seedance.go
- 文件：backend/internal/service/openai_video_seedance.go
- 断言："func (s *OpenAIGatewayService) calculateVideoCostForPlatform(key *APIKey, model, resolution string, count, seconds int, cfg *VideoPriceConfig, multiplier float64) *CostBreakdown {"
- 断言："func prepareZhongYVideoBody(body []byte, contentType string, info GrokMediaRequestInfo) ([]byte, string, error) {"
- 断言："return s.billingService.CalculateVideoCost(model, resolution, count, seconds, cfg, multiplier)"


### C-169 backend/internal/service/openai_video_seedance_download.go
- 文件：backend/internal/service/openai_video_seedance_download.go
- 断言："func (s *OpenAIGatewayService) doZhongYVideoDownload(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {"
- 断言："func (s *OpenAIGatewayService) zhongYVideoDownloadURL(account *Account, raw string) (string, error) {"
- 断言："ctx := WithHTTPUpstreamPublicHostsOnly(WithHTTPUpstreamRedirectsDisabled(req.Context()))"


### C-170 backend/internal/service/openai_video_seedance_download_test.go
- 文件：backend/internal/service/openai_video_seedance_download_test.go
- 断言："result, err := svc.ForwardGrokMedia(context.Background(), c, seedanceAccount(), GrokMediaEndpointVideoContent, \"task-1\", nil, \"\")"
- 断言："grokMediaContentStatusResponse(`{\"id\":\"task-1\",\"status\":\"completed\",\"seconds\":\"30\",\"download_url\":\"/v1/videos/task-1/content\"}`),"
- 断言："{StatusCode: 200, Header: http.Header{\"Content-Type\": []string{\"video/mp4\"}}, Body: io.NopCloser(strings.NewReader(\"mp4!\"))},"


### C-171 backend/internal/service/openai_video_seedance_test.go
- 文件：backend/internal/service/openai_video_seedance_test.go
- 断言："{StatusCode: 206, Header: http.Header{\"Content-Type\": []string{\"video/mp4\"}, \"Content-Range\": []string{\"bytes 0-3/100\"}}, Body: io.NopCloser(strings.NewReader(\"mp4!\"))},"
- 断言："cost := svc.calculateVideoCostForPlatform(&APIKey{Group: &Group{Platform: PlatformOpenAI}}, \"future-model\", \"720p\", 1, 30, &VideoPriceConfig{Price720P: &price}, 1)"
- 断言："raw := normalizeZhongYVideoError([]byte(`{\"code\":\"fail_to_fetch_task\",\"message\":\"{\\\"error\\\":{\\\"code\\\":\\\"missing_seconds\\\",\\\"message\\\":\\\"seconds required\\\"}}\"}`))"


### C-172 backend/internal/service/openai_video_test.go
- 文件：backend/internal/service/openai_video_test.go
- 断言："body := []byte(`{\"model\":\"client-video\",\"prompt\":\"waves\",\"seconds\":\"6\",\"size\":\"1280x720\",\"input_reference\":\"https://example.com/ref.png\",\"custom_option\":true}`)"
- 断言："c, recorder := grokMediaContentTestContext(http.MethodGet, \"https://api.example/v1/videos/task-1/content\", map[string]string{\"Range\": \"bytes=0-3\"})"
- 断言："for _, endpoint := range []GrokMediaEndpoint{GrokMediaEndpointImagesGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions} {"


### C-173 backend/internal/service/openai_ws_forwarder_support.go
- 文件：backend/internal/service/openai_ws_forwarder_support.go
- 断言："selection, ok := s.finalizeOpenAIAccountTransientSelection(selection, requestedModel)"
- 断言："_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)"
- 断言："selection := attachSelectionProfitGate(ctx, &AccountSelectionResult{"


### C-174 backend/internal/service/ops_cleanup_service.go
- 文件：backend/internal/service/ops_cleanup_service.go
- 断言："logger.LegacyPrintf(\"service.ops_cleanup\", \"[OpsCleanup] admin media preview cleanup failed: %v\", err)"
- 断言："if err := CleanupAdminMediaPreviews(now); err != nil {"


### C-175 backend/internal/service/ops_request_details.go
- 文件：backend/internal/service/ops_request_details.go
- 断言："IncludeRecoveredUpstream bool"


### C-176 backend/internal/service/ops_scheduler.go
- 文件：backend/internal/service/ops_scheduler.go
- 断言："snapshot.ModelCooldowns = append(snapshot.ModelCooldowns, s.gatewayService.anthropicStreamRecovery().snapshot(time.Now())...)"
- 断言："ModelCooldowns: []OpenAIAccountModelTransientSnapshot{},"
- 断言："func (s *OpsService) GetSchedulerSnapshot(ctx context.Context) (*OpenAIAccountSchedulerSnapshot, error) {"


### C-177 backend/internal/service/pricing_service.go
- 文件：backend/internal/service/pricing_service.go
- 断言："normalized, err := urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP, s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts)"
- 断言："AllowInsecureHTTPHosts: s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts,"
- 断言："AllowPrivate:           s.cfg.Security.URLAllowlist.AllowPrivateHosts,"


### C-178 backend/internal/service/ratelimit_service.go
- 文件：backend/internal/service/ratelimit_service.go
- 断言："if s.guardUpstreamBalance(ctx, account, statusCode, responseBody) {"
- 断言："func (s *RateLimitService) handleOpenAIAPIKeyAvailabilityRetryAfter(ctx context.Context, account *Account, headers http.Header, body []byte) bool {"
- 断言："if quotaReset := parseOpenAIRateLimitResetTime(body); quotaReset != nil && time.Unix(*quotaReset, 0).After(until) {"


### C-179 backend/internal/service/ratelimit_service_openai_team_linked_test.go
- 文件：backend/internal/service/ratelimit_service_openai_team_linked_test.go
- 断言："require.Empty(t, repo.setErrorIDs, \"insufficient balance uses the custom temporary cooldown, not a permanent auth error\")"
- 断言："require.Equal(t, []string{\"upstream_insufficient_balance\"}, blocker.reasons)"
- 断言："require.Equal(t, int64(1), blocker.accounts[0].ID)"


### C-180 backend/internal/service/runtime_hardening_test.go
- 文件：backend/internal/service/runtime_hardening_test.go
- 断言："require.Eventually(t, func() bool { _, err := findOpenAIImagePreview(resolved, apiKeyID, 0); return err == nil }, 3*time.Second, 10*time.Millisecond)"
- 断言："require.Equal(t, []string{\"/api/v1/admin/usage/9/images/0\"}, ListOpenAIImagePreviewURLs(resolved, apiKeyID, 9, 1))"
- 断言："require.False(t, IsUpstreamInsufficientBalance(200, []byte(`{\"code\":\"INSUFFICIENT_BALANCE\"}`)))"


### C-181 backend/internal/service/setting_parse.go
- 文件：backend/internal/service/setting_parse.go
- 断言："LongNoticeContent:                            settings[SettingKeyLongNoticeContent],"


### C-182 backend/internal/service/setting_public.go
- 文件：backend/internal/service/setting_public.go
- 断言："LongNoticeContent                   string                   `json:\"long_notice_content\"`"
- 断言："LongNoticeContent:                   settings[SettingKeyLongNoticeContent],"
- 断言："LongNoticeContent:                   settings.LongNoticeContent,"


### C-183 backend/internal/service/setting_update.go
- 文件：backend/internal/service/setting_update.go
- 断言："updates[SettingKeyLongNoticeContent] = settings.LongNoticeContent"


### C-184 backend/internal/service/settings_view.go
- 文件：backend/internal/service/settings_view.go
- 断言："LongNoticeContent                   string"
- 断言："LongNoticeContent           string"
- 断言："LoginCaptchaEnabled                 bool"


### C-185 backend/internal/service/update_custom_image_test.go
- 文件：backend/internal/service/update_custom_image_test.go
- 断言："require.ErrorIs(t, s.RollbackToVersion(context.Background(), \"0.2.4\"), ErrCustomImageManaged)"
- 断言："require.ErrorIs(t, s.PerformUpdate(context.Background()), ErrCustomImageManaged)"
- 断言："func TestCustomImageRejectsInPlaceUpdateAndRollbackBeforeIO(t *testing.T) {"


### C-186 backend/internal/service/update_service.go
- 文件：backend/internal/service/update_service.go
- 断言："ErrCustomImageManaged = infraerrors.Conflict(\"CUSTOM_IMAGE_MANAGED\", \"当前为 BUAGA 定制版，请使用已验证的定制镜像及配套源码升级或回滚，禁止以官方二进制覆盖定制功能。\")"
- 断言："return []RollbackVersion{}, nil"
- 断言："if s.buildType == \"custom\" {"


### C-187 backend/internal/service/upstream_balance_guard.go
- 文件：backend/internal/service/upstream_balance_guard.go
- 断言："func (s *RateLimitService) guardUpstreamBalance(ctx context.Context, account *Account, status int, body []byte) bool {"
- 断言："if account == nil || account.IsCNProvider() || !IsUpstreamInsufficientBalance(status, body) {"
- 断言："slog.Warn(\"upstream_balance_cooldown_save_failed\", \"account_id\", account.ID, \"error\", err)"


### C-188 backend/internal/service/upstream_models.go
- 文件：backend/internal/service/upstream_models.go
- 断言："preferredProvider, providerMatched := matchModelsDevProvider(registry, upstreamModelRegistryBaseURL(account))"
- 断言："if !upstreamModelMetadataIsComplete(catalog.Metadata[modelID]) {"
- 断言："Code:     UpstreamModelMetadataIncompleteCode,"


### C-189 backend/internal/service/upstream_models_relay_regression_test.go
- 文件：backend/internal/service/upstream_models_relay_regression_test.go
- 断言："upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}}"
- 断言："svc := &AccountTestService{httpUpstream: upstream, accountRepo: repo, cfg: upstreamModelSyncTestConfig()}"
- 断言："svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}"


### C-190 backend/internal/service/upstream_models_test.go
- 文件：backend/internal/service/upstream_models_test.go
- 断言："require.True(t, upstreamModelMetadataIsComplete(upstreamMetadataFromModelsDevModel(modelID, model)))"
- 断言："svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}"
- 断言："require.True(t, upstreamModelMetadataIsComplete(metadata[\"codex-auto-review\"]))"


### C-191 backend/internal/service/usage_billing.go
- 文件：backend/internal/service/usage_billing.go
- 断言："ApplyVideo(ctx context.Context, cmd *UsageBillingCommand, usageLog *UsageLog) (*UsageBillingApplyResult, error)"
- 断言："type AtomicVideoUsageBillingRepository interface {"
- 断言："VideoUsageLog *UsageLog"


### C-192 backend/internal/service/usage_billing_apply.go
- 文件：backend/internal/service/usage_billing_apply.go
- 断言："func applyVideoUsageBilling(ctx context.Context, requestID string, usageLog *UsageLog, p *postUsageBillingParams, deps *billingDeps, repo UsageBillingRepository) (bool, error) {"
- 断言："return applyVideoUsageBillingCommand(ctx, buildUsageBillingCommand(requestID, usageLog, p), usageLog, p, deps, repo)"
- 断言："return false, errors.New(\"atomic video usage billing requires a video usage log and dependencies\")"


### C-193 backend/internal/service/usage_billing_video_cost.go
- 文件：backend/internal/service/usage_billing_video_cost.go
- 断言："func (s *OpenAIGatewayService) ResolveVideoTaskCost(ctx context.Context, input *OpenAIRecordUsageInput) (*VideoTaskCostSnapshot, error) {"
- 断言："InboundEndpoint: optionalTrimmedStringPtr(input.InboundEndpoint), UpstreamEndpoint: optionalTrimmedStringPtr(input.UpstreamEndpoint),"
- 断言："resolution, seconds, err := videoTaskBillingUnits(account.Platform, model, result.VideoResolution, result.VideoDurationSeconds)"


### C-194 backend/internal/service/usage_billing_video_cost_test.go
- 文件：backend/internal/service/usage_billing_video_cost_test.go
- 断言："func videoTaskQuoteFixture() (*OpenAIGatewayService, *OpenAIRecordUsageInput, *videoUsageAtomicBillingRepoStub, *openAIRecordUsageLogRepoStub) {"
- 断言："Models: []string{\"seedance-custom\"}, BillingMode: BillingModeVideo, PerRequestPrice: &price,"
- 断言："legacy := svc.billingService.CalculateVideoCost(input.Result.Model, \"720p\", 1, 30, nil, 1)"


### C-195 backend/internal/service/usage_billing_video_test.go
- 文件：backend/internal/service/usage_billing_video_test.go
- 断言："func (r *videoUsageAtomicBillingRepoStub) ApplyVideo(ctx context.Context, cmd *UsageBillingCommand, usageLog *UsageLog) (*UsageBillingApplyResult, error) {"
- 断言："require.Zero(t, usageRepo.calls, \"video logs must not escape the billing transaction, including errors and duplicates\")"
- 断言："require.Greater(t, cmd.VideoUsageLog.ActualCost, 0.0, \"failed transactions must not replace the row with zero cost\")"


### C-196 backend/internal/service/usage_estimation.go
- 文件：backend/internal/service/usage_estimation.go
- 断言："if result != nil && (result.ImageCount > 0 || result.VideoCount > 0 || result.WebSearchCalls > 0 || result.SearchCount > 0 || result.AudioUsage != nil) {"
- 断言："if account != nil && (result.UsagePresent || result.UsageSource == UsageSourceUpstream || openAIUsageHasTokens(&result.Usage)) {"
- 断言："if state.first.IsZero() || now.Sub(state.first) > missingUsageAccountCooldown { state = missingUsageObservation{first: now} }"


### C-197 backend/internal/service/usage_estimation_test.go
- 文件：backend/internal/service/usage_estimation_test.go
- 断言："(&OpenAIGatewayService{}).finalizeUsageEstimate(context.Background(), nil, nil, 400, result, errors.New(\"upstream failed\"))"
- 断言："func TestFinalizeOpenAIUsageEstimate_UpstreamUsageIsAuthoritative(t *testing.T) {"
- 断言："require.Equal(t, UsageSourceUpstream, result.UsageSource)"


### C-198 backend/internal/service/usage_log.go
- 文件：backend/internal/service/usage_log.go
- 断言："ClientDisconnected bool"
- 断言："UsageSource *string"


### C-199 backend/internal/service/video_submission.go
- 文件：backend/internal/service/video_submission.go
- 断言："func (s *OpenAIGatewayService) BeginVideoSubmission(ctx context.Context, keyID, groupID int64, key, contentType string, body []byte) (string, error) {"
- 断言："var ErrVideoSubmissionPending = errors.New(\"video submission is in progress or its outcome is unknown; do not resubmit with a new key\")"
- 断言："claimed, taskID, err := repo.ClaimVideoSubmission(ctx, keyID, videoSubmissionKeyHash(key), hex.EncodeToString(digest.Sum(nil)))"


### C-200 backend/internal/service/video_submission_test.go
- 文件：backend/internal/service/video_submission_test.go
- 断言："func (r *videoSubmissionStub) ClaimVideoSubmission(_ context.Context, id int64, key, hash string) (bool, string, error) {"
- 断言："func (r *videoSubmissionStub) CompleteVideoSubmission(_ context.Context, id int64, key, task string) error {"
- 断言："_, err = s.BeginVideoSubmission(ctx, 1, 2, \"same\", \"application/json\", []byte(`{\"model\":\"different\"}`))"


### C-201 backend/internal/service/video_task.go
- 文件：backend/internal/service/video_task.go
- 断言："func (s *OpenAIGatewayService) LoadDurableVideoTask(ctx context.Context, taskID string, userID, apiKeyID int64) (*VideoTask, error) {"
- 断言："!strings.EqualFold(strings.TrimSpace(*quote.Log.VideoResolution), strings.TrimSpace(pending.VideoResolution)) {"
- 断言："quote.Log.VideoCount != 1 || *quote.Log.VideoDurationSeconds != pending.VideoDurationSeconds ||"


### C-202 backend/internal/service/video_task_admin.go
- 文件：backend/internal/service/video_task_admin.go
- 断言："func (s *OpenAIGatewayService) ListAdminVideoTasks(ctx context.Context, f VideoTaskAdminFilter) ([]VideoTaskAdminItem, int64, error) {"
- 断言："ListVideoTasks(context.Context, VideoTaskAdminFilter) ([]VideoTaskAdminItem, int64, error)"
- 断言："if repo, ok := s.usageBillingRepo.(VideoTaskAdminRepository); ok && repo != nil {"


### C-203 backend/internal/service/video_task_test.go
- 文件：backend/internal/service/video_task_test.go
- 断言："p := &videoTaskProcessor{repo: repo, poll: svc.pollDurableVideoTask, bill: func(context.Context, *VideoTask) error { t.Fatal(\"must not bill without completion\"); return nil }}"
- 断言："key.Group = &Group{ID: *task.GroupID, Platform: PlatformOpenAI, RateMultiplier: 1, SubscriptionType: SubscriptionTypeSubscription, Hydrated: true, VideoPrice720P: &price}"
- 断言："upstream := &grokMediaContentUpstreamStub{response: &http.Response{StatusCode: tc.httpStatus, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}}"


### C-204 backend/internal/service/video_task_worker.go
- 文件：backend/internal/service/video_task_worker.go
- 断言："p := &videoTaskProcessor{repo: repo, poll: s.pollDurableVideoTask, bill: func(ctx context.Context, t *VideoTask) error { return s.billDurableVideoTask(ctx, t, apiKeyService) }}"
- 断言："logger.LegacyPrintf(\"service.video_task\", \"video settlement deferred user=%d key=%d account=%d\", t.UserID, t.APIKeyID, t.AccountID)"
- 断言："func (s *OpenAIGatewayService) pollDurableVideoTask(ctx context.Context, t *VideoTask) (*VideoTaskCompletion, string, error) {"


### C-205 backend/internal/service/wire.go
- 文件：backend/internal/service/wire.go
- 断言："logger.LegacyPrintf(\"service.concurrency\", \"Warning: startup cleanup stale process slots failed: %v\", err)"
- 断言："if err := svc.CleanupStaleProcessSlots(context.Background()); err != nil {"
- 断言："if cfg == nil || !cfg.Gateway.Scheduling.SkipStartupCleanup {"


### C-206 backend/internal/util/urlvalidator/validator.go
- 文件：backend/internal/util/urlvalidator/validator.go
- 断言："func ValidateURLFormat(raw string, allowInsecureHTTP bool, allowInsecureHTTPHosts ...[]string) (string, error) {"
- 断言："insecureHTTPAllowed := scheme == \"http\" && (opts.AllowInsecureHTTP || isAllowedHost(host, normalizeAllowlist(opts.AllowInsecureHTTPHosts)))"
- 断言："insecureHTTPAllowed := scheme == \"http\" && isAllowedHost(host, normalizeAllowlist(insecureHTTPHosts))"


### C-207 backend/internal/util/urlvalidator/validator_test.go
- 文件：backend/internal/util/urlvalidator/validator_test.go
- 断言："if _, err := ValidateHTTPURL(\"http://gpt.us.aiapisplus.com/\", false, ValidationOptions{AllowInsecureHTTPHosts: []string{\"gpt.us.aiapisplus.com\"}}); err != nil {"
- 断言："if _, err := ValidateHTTPURL(\"http://example.com\", false, ValidationOptions{AllowInsecureHTTPHosts: []string{\"gpt.us.aiapisplus.com\"}}); err == nil {"
- 断言："t.Fatalf(\"expected configured insecure HTTP host to pass, got %v\", err)"


### C-208 backend/internal/web/embed_on.go
- 文件：backend/internal/web/embed_on.go
- 断言："settingsJSON = externalizeSiteLogo(settingsJSON)"
- 断言："if strings.HasPrefix(path, siteLogoPrefix) {"
- 断言："s.serveSiteLogo(c)"


### C-209 backend/internal/web/site_logo.go
- 文件：backend/internal/web/site_logo.go
- 断言："c.Header(\"Cache-Control\", staticAssetsCacheControl)"
- 断言："func siteLogoAsset(settingsJSON []byte) (path, mime string, data []byte) {"
- 断言："ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)"


### C-210 backend/internal/web/site_logo_test.go
- 文件：backend/internal/web/site_logo_test.go
- 断言："input, _ := json.Marshal(map[string]string{\"site_logo\": \"data:image/png;base64,\" + base64.StdEncoding.EncodeToString(buf.Bytes()), \"site_name\": \"BUAGA\"})"
- 断言："if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), buf.Bytes()) || response.Header().Get(\"Cache-Control\") != staticAssetsCacheControl {"
- 断言："if cfg[\"site_logo\"] != path || cfg[\"site_name\"] != \"BUAGA\" {"


### C-211 backend/migrations/231_channel_monitor_sort_order.sql
- 文件：backend/migrations/231_channel_monitor_sort_order.sql
- 断言："SET sort_order = (SELECT COALESCE(MAX(id), 0) FROM channel_monitors) - id"
- 断言："ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;"
- 断言："CREATE INDEX IF NOT EXISTS idx_channel_monitors_sort_order"


### C-212 backend/migrations/232_channel_monitor_sort_order.sql
- 文件：backend/migrations/232_channel_monitor_sort_order.sql
- 断言："ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 2147483647;"
- 断言："CREATE INDEX IF NOT EXISTS idx_channel_monitors_sort_order"
- 断言："ALTER COLUMN sort_order SET DEFAULT 2147483647;"


### C-213 backend/migrations/234_add_usage_log_billing_metadata.sql
- 文件：backend/migrations/234_add_usage_log_billing_metadata.sql
- 断言："COMMENT ON COLUMN usage_logs.client_disconnected IS 'Whether the downstream client disconnected before upstream completion';"
- 断言："COMMENT ON COLUMN usage_logs.usage_source IS 'Token usage source: upstream or estimated';"
- 断言："ADD COLUMN IF NOT EXISTS usage_source VARCHAR(16) NOT NULL DEFAULT 'upstream',"


### C-214 backend/migrations/235_group_model_allowlist.sql
- 文件：backend/migrations/235_group_model_allowlist.sql
- 断言："ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb,"
- 断言："DROP TRIGGER IF EXISTS sync_groups_model_allowlist_columns_trigger ON groups;"
- 断言："ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;"


### C-215 backend/migrations/237_video_tasks.sql
- 文件：backend/migrations/237_video_tasks.sql
- 断言："-- New OpenAI video tasks only. No submission/idempotency-key records here."
- 断言："CREATE INDEX IF NOT EXISTS idx_video_tasks_due"
- 断言："ON video_tasks (next_attempt_at, lease_until)"


### C-216 backend/migrations/238_video_submissions.sql
- 文件：backend/migrations/238_video_submissions.sql
- 断言："CREATE TABLE IF NOT EXISTS video_submissions ("
- 断言："-- submission must never be reclaimed automatically: the provider may be billing it."
- 断言："-- Keep the acceptance record independent of short-lived Redis state. An unknown"


### C-217 frontend/src/.codex-work/model-list-fix-20260908/source/frontend/src/components/admin/account/AccountTestModal.vue
- 文件：frontend/src/.codex-work/model-list-fix-20260908/source/frontend/src/components/admin/account/AccountTestModal.vue
- 断言："<div v-if=\"uploadImagePreview\" class=\"overflow-hidden rounded-lg border border-gray-200 dark:border-dark-500\">"
- 断言："return grokTestMode.value === 'text' || grokTestMode.value === 'image' || grokTestMode.value === 'video'"
- 断言："addLine(t('admin.accounts.videoReceived', { count: generatedVideos.value.length }), 'text-purple-300')"


### C-218 frontend/src/.codex-work/model-list-fix-20260908/source/frontend/src/components/admin/account/__tests__/AccountTestModal.spec.ts
- 文件：frontend/src/.codex-work/model-list-fix-20260908/source/frontend/src/components/admin/account/__tests__/AccountTestModal.spec.ts
- 断言："expect(preview.attributes('src')).toBe('data:image/png;base64,QUJD')"
- 断言："if (key === 'admin.accounts.imagePreviewAlt' && params?.index) {"
- 断言："const preview = wrapper.find('img[alt=\"test-image-1\"]')"


### C-219 frontend/src/App.vue
- 文件：frontend/src/App.vue
- 断言："const localPreviewMode = import.meta.env.VITE_LOCAL_PREVIEW === 'true'"
- 断言："if (!localPreviewMode && authStore.isAuthenticated) {"
- 断言："if (import.meta.env.VITE_LOCAL_PREVIEW !== 'true') {"


### C-220 frontend/src/api/admin/__tests__/videoTasks.spec.ts
- 文件：frontend/src/api/admin/__tests__/videoTasks.spec.ts
- 断言："expect(apiClient.get).toHaveBeenCalledWith('/admin/video-tasks', {"
- 断言："describe('admin video task API', () => {"
- 断言："params: { page: 3, page_size: 100, search: ' %_\\\\ task ', status: 'query_retry', days: 30 }, signal: controller.signal"


### C-221 frontend/src/api/admin/accounts.ts
- 文件：frontend/src/api/admin/accounts.ts
- 断言："complete_count?: number"
- 断言："model_ids?: string[]"
- 断言："total_count?: number"


### C-222 frontend/src/api/admin/channelMonitor.ts
- 文件：frontend/src/api/admin/channelMonitor.ts
- 断言："updates: Array<{ id: number; sort_order: number }>"
- 断言："sort_order: number"
- 断言："const { data } = await apiClient.put<{ message: string }>('/admin/channel-monitors/sort-order', {"


### C-223 frontend/src/api/admin/channels.ts
- 文件：frontend/src/api/admin/channels.ts
- 断言："expected_updated_at?: string"


### C-224 frontend/src/api/admin/dashboard.ts
- 文件：frontend/src/api/admin/dashboard.ts
- 断言："/**"


### C-225 frontend/src/api/admin/groups.ts
- 文件：frontend/src/api/admin/groups.ts
- 断言："export async function getModelsListCandidates("
- 断言："`/admin/groups/${id}/models-list-candidates`,"
- 断言："getModelsListCandidates,"


### C-226 frontend/src/api/admin/ops.ts
- 文件：frontend/src/api/admin/ops.ts
- 断言："state?: 'cooldown' | 'awaiting_probe' | 'probing' | 'recovering'"
- 断言："include_recovered_upstream?: boolean"
- 断言："model_cooldowns: OpsModelCooldown[]"


### C-227 frontend/src/api/admin/settings.ts
- 文件：frontend/src/api/admin/settings.ts
- 断言："long_notice_content?: string;"
- 断言："long_notice_content: string;"


### C-228 frontend/src/api/admin/usage.ts
- 文件：frontend/src/api/admin/usage.ts
- 断言："/**"


### C-229 frontend/src/api/admin/videoTasks.ts
- 文件：frontend/src/api/admin/videoTasks.ts
- 断言："const { data } = await apiClient.get<PaginatedResponse<VideoTaskAdminItem>>('/admin/video-tasks', {"
- 断言："export type VideoTaskStatus = typeof videoTaskStatuses[number]"
- 断言："): Promise<PaginatedResponse<VideoTaskAdminItem>> {"


### C-230 frontend/src/api/channels.ts
- 文件：frontend/src/api/channels.ts
- 断言："per_request_price: number | null"


### C-231 frontend/src/api/client.ts
- 文件：frontend/src/api/client.ts
- 断言："adapter: import.meta.env.VITE_LOCAL_PREVIEW === 'true' ? localPreviewAdapter : undefined"
- 断言："headers: { 'Content-Type': 'application/json' },"


### C-232 frontend/src/api/localPreview.ts
- 文件：frontend/src/api/localPreview.ts
- 断言："if (path === '/usage/stats') return { ...dashboardStats, period: 'month', total_cache_tokens: 42000, average_duration_ms: 832 }"
- 断言："if (path === '/admin/payment/orders') return paginated(previewOrders.map(order => ({ ...order, user: previewUser })), config)"
- 断言："if (path === '/admin/usage') return paginated([{ ...usageLog, image_previews: ['/api/v1/admin/usage/1/images/0'] }], config)"


### C-233 frontend/src/api/modelPlaza.ts
- 文件：frontend/src/api/modelPlaza.ts
- 断言："/**"


### C-234 frontend/src/api/usage.ts
- 文件：frontend/src/api/usage.ts
- 断言："/**"


### C-235 frontend/src/components/TurnstileWidget.vue
- 文件：frontend/src/components/TurnstileWidget.vue
- 断言："<button type=\"button\" class=\"ml-2 underline\" @click=\"initialize\">{{ t('auth.captchaRetry') }}</button>"
- 断言："<p v-if=\"loading\" role=\"status\" class=\"text-sm text-gray-500\">{{ t('auth.captchaLoading') }}</p>"
- 断言："<div v-if=\"failed\" role=\"alert\" class=\"text-sm text-red-600\">"


### C-236 frontend/src/components/__tests__/TurnstileWidget.spec.ts
- 文件：frontend/src/components/__tests__/TurnstileWidget.spec.ts
- 断言："expect(render).toHaveBeenCalledWith(expect.any(HTMLElement), expect.objectContaining({ sitekey: 'late-key' }))"
- 断言："it('times out instead of leaving an invisible challenge indefinitely', async () => {"
- 断言："it('retries a failed script and recovers without reloading the page', async () => {"


### C-237 frontend/src/components/account/CNProviderQuotaCell.vue
- 文件：frontend/src/components/account/CNProviderQuotaCell.vue
- 断言："<div class=\"h-1.5 w-16 shrink-0 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600\">"
- 断言："<span :class=\"['shrink-0 font-medium', utilizationTextColor(tier.used_percent)]\">"
- 断言："if (diffMs < 3_600_000) return `${Math.max(1, Math.round(diffMs / 60_000))}m`"


### C-238 frontend/src/components/account/CreateAccountModal.vue
- 文件：frontend/src/components/account/CreateAccountModal.vue
- 断言："await adminAPI.accounts.syncUpstreamModels(account.id)"
- 断言："<p class=\"input-hint\">{{ t('admin.accounts.expiresAtHint') }}</p>"
- 断言："const authStore = useAuthStore()"


### C-239 frontend/src/components/account/EditAccountModal.vue
- 文件：frontend/src/components/account/EditAccountModal.vue
- 断言："<p class=\"input-hint\">{{ t('admin.accounts.expiresAtHint') }}</p>"
- 断言："const authStore = useAuthStore()"
- 断言："v-if=\"!authStore.isSimpleMode\""


### C-240 frontend/src/components/account/ModelWhitelistSelector.vue
- 文件：frontend/src/components/account/ModelWhitelistSelector.vue
- 断言："appStore.showWarning(formatUpstreamModelSyncWarning(warnings, t))"


### C-241 frontend/src/components/account/UsageProgressBar.vue
- 文件：frontend/src/components/account/UsageProgressBar.vue
- 断言：":class=\"['w-[32px] shrink-0 rounded px-1 text-center text-[10px] font-medium', labelClass]\""
- 断言："<!-- Label badge (fixed width for alignment) -->"
- 断言："} else if (props.utilization >= 80) {"


### C-242 frontend/src/components/account/__tests__/AccountUsageCell.spec.ts
- 文件：frontend/src/components/account/__tests__/AccountUsageCell.spec.ts
- 断言："CNProviderBalanceCell: { template: '<div data-test=\"cn-balance-cell\" />' }"
- 断言："CNProviderQuotaCell: { template: '<div data-test=\"cn-quota-cell\" />' },"


### C-243 frontend/src/components/account/__tests__/CNProviderQuotaCell.spec.ts
- 文件：frontend/src/components/account/__tests__/CNProviderQuotaCell.spec.ts
- 断言："it('keeps the compact quota stack readable inside the account table cell', async () => {"
- 断言："const labels = root.findAll('[data-test=\"cn-provider-quota-label\"]')"
- 断言："const tiers = root.findAll('[data-test=\"cn-provider-quota-tier\"]')"


### C-244 frontend/src/components/account/__tests__/CreateAccountModal.spec.ts
- 文件：frontend/src/components/account/__tests__/CreateAccountModal.spec.ts
- 断言："it('does not show a capability warning after post-create model sync', async () => {"
- 断言："expect(showWarningMock).not.toHaveBeenCalledWith("


### C-245 frontend/src/components/account/__tests__/EditAccountModal.spec.ts
- 文件：frontend/src/components/account/__tests__/EditAccountModal.spec.ts
- 断言："it('replaces the saved expiry with the date selected in the custom form', async () => {"
- 断言："it('can clear the selected expiry before saving the account', async () => {"
- 断言："for (const selected of ['2028-03-29T12:34', '2029-02-28T12:34']) {"


### C-246 frontend/src/components/account/__tests__/ModelWhitelistSelector.spec.ts
- 文件：frontend/src/components/account/__tests__/ModelWhitelistSelector.spec.ts
- 断言："expect(showSuccess).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsSuccess')"
- 断言："models: ['x-preview-f-free']"
- 断言："it('reports success when model IDs sync without capability metadata', async () => {"


### C-247 frontend/src/components/account/__tests__/UsageProgressBar.spec.ts
- 文件：frontend/src/components/account/__tests__/UsageProgressBar.spec.ts
- 断言："import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'"


### C-248 frontend/src/components/admin/account/ReAuthAccountModal.vue
- 文件：frontend/src/components/admin/account/ReAuthAccountModal.vue
- 断言："@validate-refresh-token=\"handleGrokValidateRefreshToken\""


### C-249 frontend/src/components/admin/account/__tests__/ReAuthAccountModal.grok.spec.ts
- 文件：frontend/src/components/admin/account/__tests__/ReAuthAccountModal.grok.spec.ts
- 断言："expect(source).toContain('@validate-refresh-token=\"handleGrokValidateRefreshToken\"')"


### C-250 frontend/src/components/admin/announcements/LongNoticeEditor.vue
- 文件：frontend/src/components/admin/announcements/LongNoticeEditor.vue
- 断言："function syncTags(block: LongNoticeDocument['blocks'][number], index: number) { block.tags = (tagsText.value[index] || '').split(',').map(tag => tag.trim()).filter(Boolean) }"
- 断言："<div class=\"grid gap-5 p-5\" :class=\"previewOpen ? 'lg:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]' : ''\">"
- 断言："const document = ref<LongNoticeDocument>(emptyLongNoticeDocument())"


### C-251 frontend/src/components/admin/group/CodexManifestAccountsField.vue
- 文件：frontend/src/components/admin/group/CodexManifestAccountsField.vue
- 断言："'relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors',"
- 断言："'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',"
- 断言："fallback_to_scheduler: !config.fallback_to_scheduler,"


### C-252 frontend/src/components/admin/group/ReasoningEffortPolicyFields.vue
- 文件：frontend/src/components/admin/group/ReasoningEffortPolicyFields.vue
- 断言：":options=\"reasoningEffortOptions\""


### C-253 frontend/src/components/admin/group/__tests__/ReasoningEffortPolicyFields.spec.ts
- 文件：frontend/src/components/admin/group/__tests__/ReasoningEffortPolicyFields.spec.ts
- 断言："import { mount } from \"@vue/test-utils\";"


### C-254 frontend/src/components/admin/monitor/MonitorFiltersBar.vue
- 文件：frontend/src/components/admin/monitor/MonitorFiltersBar.vue
- 断言：":title=\"t('admin.channelMonitor.sortOrder')\""
- 断言："<Icon name=\"menu\" size=\"md\" class=\"mr-2\" />"
- 断言："{{ t('admin.channelMonitor.sortOrder') }}"


### C-255 frontend/src/components/admin/usage/UsageFilters.vue
- 文件：frontend/src/components/admin/usage/UsageFilters.vue
- 断言："<div class=\"newapi-usage-filter-grid flex-1\">"


### C-256 frontend/src/components/admin/usage/UsageStatsCards.vue
- 文件：frontend/src/components/admin/usage/UsageStatsCards.vue
- 断言："<template>"


### C-257 frontend/src/components/admin/usage/UsageTable.vue
- 文件：frontend/src/components/admin/usage/UsageTable.vue
- 断言："<button type=\"button\" class=\"absolute right-5 top-5 rounded bg-black/50 px-3 py-2 text-xl text-white\" title=\"关闭预览\" @click=\"previewVideo = ''\">×</button>"
- 断言："<button type=\"button\" class=\"absolute right-5 top-5 rounded bg-black/50 px-3 py-2 text-xl text-white\" title=\"关闭预览\" @click=\"previewImage = ''\">×</button>"
- 断言："<div v-if=\"previewVideo\" class=\"fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-6\" @click.self=\"previewVideo = ''\">"


### C-258 frontend/src/components/admin/usage/__tests__/UsageFilters.spec.ts
- 文件：frontend/src/components/admin/usage/__tests__/UsageFilters.spec.ts
- 断言："import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'"


### C-259 frontend/src/components/admin/usage/__tests__/UsageStatsCards.spec.ts
- 文件：frontend/src/components/admin/usage/__tests__/UsageStatsCards.spec.ts
- 断言："import { describe, expect, it, vi } from 'vitest'"


### C-260 frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts
- 文件：frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts
- 断言："data: [{ ...baseImageRow, model: 'gpt-5.6-sol', upstream_response_model: 'gpt-5.6-luna', stream: true, native_compaction_v2: flag }],"
- 断言："await wrapper.setProps({ data: [{ ...baseImageRow, request_id: 'next-page', image_previews: ['/api/v1/admin/usage/2/images/0'] }] })"
- 断言："props: { data: [{ ...baseImageRow, image_previews: ['/api/v1/admin/usage/1/images/0'] }], loading: false, columns: [] },"


### C-261 frontend/src/components/channels/SupportedModelChip.vue
- 文件：frontend/src/components/channels/SupportedModelChip.vue
- 断言："function formatInterval(iv: UserPricingInterval, mode: BillingMode): string {"
- 断言："if (mode === BILLING_MODE_PER_REQUEST || mode === BILLING_MODE_IMAGE) {"
- 断言："const prices = resolveIntervalPrices(iv, props.model.pricing ?? {})"


### C-262 frontend/src/components/common/AnnouncementBell.vue
- 文件：frontend/src/components/common/AnnouncementBell.vue
- 断言："<p v-if=\"longNotice.blocks.length === 0\" class=\"py-14 text-center text-sm text-gray-400\">{{ t('announcements.empty') }}</p>"
- 断言："const longNotice = computed(() => parseLongNoticeDocument(appStore.cachedPublicSettings?.long_notice_content))"
- 断言："<button class=\"newapi-icon-button\" @click=\"close\"><Icon name=\"x\" size=\"sm\" /></button>"


### C-263 frontend/src/components/common/GroupSelector.vue
- 文件：frontend/src/components/common/GroupSelector.vue
- 断言："const compositeIds = new Set(props.groups.filter((group) => group.platform === 'composite').map((group) => group.id))"
- 断言：":title=\"t('admin.groups.rateAndAccounts', { rate: group.rate_multiplier, count: group.account_count || 0 })\""
- 断言："if (selected.length !== props.modelValue.length) emit('update:modelValue', selected)"


### C-264 frontend/src/components/common/HelpTooltip.vue
- 文件：frontend/src/components/common/HelpTooltip.vue
- 断言："<div class=\"pointer-events-none absolute -bottom-1 left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900 dark:bg-gray-800\"></div>"
- 断言："window.addEventListener('scroll', onViewportChange, { capture: true, passive: true })"
- 断言："props.interactive ? 'pointer-events-auto' : 'pointer-events-none',"


### C-265 frontend/src/components/common/LocaleSwitcher.vue
- 文件：frontend/src/components/common/LocaleSwitcher.vue
- 断言："const currentLocale = computed(() => localeOptions.find((l) => l.code === currentLocaleCode.value) || localeOptions[0])"
- 断言："const currentLocaleCode = computed(() => locale?.value || localeOptions[0]?.code || 'en')"
- 断言："const localeOptions = Array.isArray(availableLocales) ? availableLocales : []"


### C-266 frontend/src/components/common/LongNoticeRenderer.vue
- 文件：frontend/src/components/common/LongNoticeRenderer.vue
- 断言："function toneClasses(tone?: LongNoticeTone) { return tones[tone || 'blue'] }"
- 断言："defineProps<{ document: LongNoticeDocument }>()"
- 断言："<div class=\"text-xs text-gray-500 dark:text-gray-400\">{{ metric.label }}</div><div class=\"mt-1 text-base font-semibold text-gray-900 dark:text-white\">{{ metric.value }}</div>"


### C-267 frontend/src/components/common/MonitorQuotaView.vue
- 文件：frontend/src/components/common/MonitorQuotaView.vue
- 断言："<span v-if=\"row.tier.reset_at\" class=\"truncate text-gray-400 dark:text-gray-500\" :title=\"row.tier.reset_at\">"
- 断言："<div class=\"h-1.5 w-16 shrink-0 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600\">"
- 断言："<div v-for=\"row in tierRows\" :key=\"row.key\" class=\"flex items-center gap-1.5 text-[10px]\">"


### C-268 frontend/src/components/common/SiteVersionBadge.vue
- 文件：frontend/src/components/common/SiteVersionBadge.vue
- 断言："<p v-if=\"info?.build_type === 'custom'\" class=\"version-status\">BUAGA 定制版：升级和回滚须使用已验证的定制镜像。</p>"
- 断言：".version-panel-header { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; border-bottom: 1px solid #eaecf0; font-weight: 600; }"
- 断言：".version-trigger, .version-public { display: inline-flex; align-items: center; gap: 6px; font-size: 11px; font-weight: 500; line-height: 16px; color: #667085; }"


### C-269 frontend/src/components/common/VersionBadge.vue
- 文件：frontend/src/components/common/VersionBadge.vue
- 断言："<p v-if=\"isCustomBuild\" class=\"text-xs text-gray-500 dark:text-dark-400\">{{ t('version.customModeHint') }}</p>"
- 断言："{{ t(isCustomBuild ? 'version.customModeHint' : 'version.sourceModeHint') }}"
- 断言："<div v-else class=\"border-t border-gray-100 pt-2 dark:border-dark-700\">"


### C-270 frontend/src/components/common/__tests__/AnnouncementBell.spec.ts
- 文件：frontend/src/components/common/__tests__/AnnouncementBell.spec.ts
- 断言："vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: {}, showSuccess: vi.fn(), showError: vi.fn() }) }))"
- 断言："it.each(['footer', 'cross', 'escape', 'outside'])('ends the batch via %s and retains the other unread announcements', async (method) => {"
- 断言："vi.mock('@/utils/format', () => ({ formatRelativeTime: () => 'now', formatRelativeWithDateTime: () => 'now' }))"


### C-271 frontend/src/components/common/__tests__/GroupSelector.spec.ts
- 文件：frontend/src/components/common/__tests__/GroupSelector.spec.ts
- 断言："it('does not remove selections merely hidden by platform or search filters', async () => {"
- 断言："it('waits for group data before removing known composite selections', async () => {"
- 断言："expect(wrapper.emitted('update:modelValue')).toEqual([[[1, 99]]])"


### C-272 frontend/src/components/common/__tests__/HelpTooltip.spec.ts
- 文件：frontend/src/components/common/__tests__/HelpTooltip.spec.ts
- 断言："it('allows non-interactive hover details to pass pointer events through', async () => {"
- 断言："expect(remove.mock.calls.filter(([name]) => name === 'scroll')).toHaveLength(1)"
- 断言："expect(add.mock.calls.filter(([name]) => name === 'scroll')).toHaveLength(0)"


### C-273 frontend/src/components/common/__tests__/MonitorQuotaView.spec.ts
- 文件：frontend/src/components/common/__tests__/MonitorQuotaView.spec.ts
- 断言："const rows = wrapper.findAll('[data-testid=\"monitor-quota-view\"] .flex.items-center')"
- 断言："expect(rows.length).toBeGreaterThanOrEqual(3)"
- 断言："expect(html).toContain('bg-emerald-500')"


### C-274 frontend/src/components/common/__tests__/SiteVersionBadge.spec.ts
- 文件：frontend/src/components/common/__tests__/SiteVersionBadge.spec.ts
- 断言："checkUpdates.mockResolvedValue({ ...latest, has_update: false, warning: 'cached', release_info: { html_url: 'javascript:alert(1)' } })"
- 断言："it('treats cached warnings as unconfirmed and rejects unexpected release URLs', async () => {"
- 断言："beforeEach(() => { auth.isAdmin = true; checkUpdates.mockReset(); checkUpdates.mockResolvedValue(latest) })"


### C-275 frontend/src/components/layout/AppHeader.vue
- 文件：frontend/src/components/layout/AppHeader.vue
- 断言："<button class=\"newapi-search hidden md:flex\" type=\"button\" title=\"搜索\" @click=\"openSearch\">"
- 断言："<div class=\"newapi-search-dialog\" role=\"dialog\" aria-modal=\"true\" aria-label=\"页面搜索\">"
- 断言："<p v-if=\"filteredSearchResults.length === 0\" class=\"newapi-search-empty\">没有匹配的页面</p>"


### C-276 frontend/src/components/layout/AppLayout.vue
- 文件：frontend/src/components/layout/AppLayout.vue
- 断言："const localPreviewMode = import.meta.env.VITE_LOCAL_PREVIEW === 'true'"
- 断言："<div class=\"newapi-page-heading\">"
- 断言："<div class=\"newapi-page-body\">"


### C-277 frontend/src/components/layout/AppSidebar.vue
- 文件：frontend/src/components/layout/AppSidebar.vue
- 断言："const isAdminView = computed(() => isAdmin.value || (previewMode.value && route.path.startsWith('/admin')))"
- 断言："const flagAvailableChannels = () => previewMode.value || makeSidebarFlag(FeatureFlags.availableChannels)()"
- 断言："const flagPluginManagement = () => previewMode.value || makeSidebarFlag(FeatureFlags.pluginManagement)()"


### C-278 frontend/src/components/layout/AuthLayout.vue
- 文件：frontend/src/components/layout/AuthLayout.vue
- 断言："const siteName = computed(() => appStore.siteName || 'BuaGa API')"


### C-279 frontend/src/components/layout/PremiumLoginLayout.vue
- 文件：frontend/src/components/layout/PremiumLoginLayout.vue
- 断言："const siteName = computed(() => appStore.siteName || 'BuaGa API')"
- 断言：".premium-login-tools > button { display: inline-flex; width: 2.25rem; height: 2.25rem; align-items: center; justify-content: center; border-radius: .5rem; color: #64748b; }"
- 断言：".premium-login-form-column { display: flex; min-width: 0; align-items: center; justify-content: center; padding: 3rem clamp(1.75rem,5vw,4.75rem); background: #fff; }"


### C-280 frontend/src/components/layout/TablePageLayout.vue
- 文件：frontend/src/components/layout/TablePageLayout.vue
- 断言："<div v-if=\"$slots.actions || $slots.filters\" class=\"newapi-data-toolbar\">"
- 断言："<!-- NewAPI-style toolbar: filters stay left, actions stay right. -->"
- 断言："<div v-if=\"$slots.actions\" class=\"newapi-data-toolbar-actions\">"


### C-281 frontend/src/components/layout/__tests__/AppSidebar.spec.ts
- 文件：frontend/src/components/layout/__tests__/AppSidebar.spec.ts
- 断言："expect(componentSource).toContain('const flagSubscription = () => previewMode.value || makeSidebarFlag(FeatureFlags.subscription)()')"
- 断言："expect(componentSource).toMatch(/path: '\\/admin\\/video-tasks'[^\\n]+icon: VideoTaskIcon/)"
- 断言："describe('AppSidebar video and scheduler icons', () => {"


### C-282 frontend/src/components/modelPlaza/PlazaModelPricingTable.vue
- 文件：frontend/src/components/modelPlaza/PlazaModelPricingTable.vue
- 断言："return intervals.some((iv) => iv.cache_write_price != null || iv.cache_write_1h_price != null || iv.cache_read_price != null)"
- 断言："<table class=\"w-full min-w-[1000px] table-fixed border-collapse text-sm tabular-nums\">"
- 断言："return sortByContext(m.pricing?.intervals ?? [])"


### C-283 frontend/src/components/modelPlaza/PlazaNavBar.vue
- 文件：frontend/src/components/modelPlaza/PlazaNavBar.vue
- 断言："const siteName = computed(() => settings.value?.site_name || 'BuaGa API')"


### C-284 frontend/src/components/modelPlaza/__tests__/PlazaModelPricingTable.spec.ts
- 文件：frontend/src/components/modelPlaza/__tests__/PlazaModelPricingTable.spec.ts
- 断言："import { describe, expect, it, vi } from 'vitest'"


### C-285 frontend/src/components/user/dashboard/UserDashboardCharts.vue
- 文件：frontend/src/components/user/dashboard/UserDashboardCharts.vue
- 断言："<div class=\"newapi-chart-card card relative overflow-hidden p-4\">"
- 断言："<div class=\"newapi-dashboard-charts space-y-5\">"
- 断言："<div class=\"newapi-chart-toolbar card p-3\">"


### C-286 frontend/src/components/user/dashboard/UserDashboardQuickActions.vue
- 文件：frontend/src/components/user/dashboard/UserDashboardQuickActions.vue
- 断言："<div class=\"newapi-quick-actions card\">"
- 断言："<button @click=\"router.push('/usage')\" class=\"group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-emerald-50/70 dark:hover:bg-emerald-900/15\">"
- 断言："<button @click=\"router.push('/redeem')\" class=\"group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-amber-50/70 dark:hover:bg-amber-900/15\">"


### C-287 frontend/src/components/user/dashboard/UserDashboardRecentUsage.vue
- 文件：frontend/src/components/user/dashboard/UserDashboardRecentUsage.vue
- 断言："<div class=\"newapi-recent-card card\">"
- 断言："<span class=\"flex h-6 w-6 shrink-0 items-center justify-center rounded bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300\"><Icon name=\"beaker\" size=\"xs\" /></span>"
- 断言："<span>{{ t('dashboard.model') }}</span><span>{{ t('dashboard.time') }}</span><span class=\"text-right\">{{ t('dashboard.cost') }}</span>"


### C-288 frontend/src/components/user/dashboard/UserDashboardStats.vue
- 文件：frontend/src/components/user/dashboard/UserDashboardStats.vue
- 断言："<span class=\"newapi-metric-icon bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300\"><Icon name=\"dollar\" size=\"sm\" /></span>"
- 断言："<span class=\"newapi-metric-icon bg-violet-50 text-violet-600 dark:bg-violet-900/30 dark:text-violet-300\"><Icon name=\"bolt\" size=\"sm\" /></span>"
- 断言："<span class=\"newapi-metric-icon bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300\"><Icon name=\"cube\" size=\"sm\" /></span>"


### C-289 frontend/src/components/user/profile/ProfileInfoCard.vue
- 文件：frontend/src/components/user/profile/ProfileInfoCard.vue
- 断言："class=\"newapi-profile-avatar flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-primary-600 text-xl font-bold text-white\""
- 断言："class=\"newapi-profile-metric rounded-lg border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-900\""
- 断言："class=\"newapi-profile-header card overflow-hidden\""


### C-290 frontend/src/composables/useModelWhitelist.ts
- 文件：frontend/src/composables/useModelWhitelist.ts
- 断言："// ====================="


### C-291 frontend/src/i18n/locales/en/admin/accounts.ts
- 文件：frontend/src/i18n/locales/en/admin/accounts.ts
- 断言："toModel: 'Upstream model',"
- 断言："windowMonthly: 'Monthly window',"
- 断言："windowWeekly: 'Weekly window',"


### C-292 frontend/src/i18n/locales/en/admin/channels.ts
- 文件：frontend/src/i18n/locales/en/admin/channels.ts
- 断言："sortOrderHint: 'Drag the handle to change the channel display order. Channels at the top appear first.',"
- 断言："failedToUpdateSortOrder: 'Failed to save channel display order. Refresh and try again.',"
- 断言："emptyModelsInPricing: 'Add at least one model to each pricing entry',"


### C-293 frontend/src/i18n/locales/en/admin/index.ts
- 文件：frontend/src/i18n/locales/en/admin/index.ts
- 断言："import overview from './overview'"


### C-294 frontend/src/i18n/locales/en/admin/ops.ts
- 文件：frontend/src/i18n/locales/en/admin/ops.ts
- 断言："description: 'View OpenAI account switches, transient cooldowns, and request feedback',"
- 断言："upstreamErrorRateMaxPercentHint: 'Maximum acceptable percentage of upstream errors.',"
- 断言："cooldownHint: 'In-process account + model combinations currently cooling down',"


### C-295 frontend/src/i18n/locales/en/admin/overview.ts
- 文件：frontend/src/i18n/locales/en/admin/overview.ts
- 断言："hint: 'Only changes the {endpoint} response. Whitelist model calls and account routing are unchanged.',"
- 断言："disabledHint: 'Not enabled: manifest requests go through scheduler account selection.',"
- 断言："title: 'Pinned Accounts for Codex Model Manifest',"


### C-296 frontend/src/i18n/locales/en/admin/resources.ts
- 文件：frontend/src/i18n/locales/en/admin/resources.ts
- 断言："export default {"


### C-297 frontend/src/i18n/locales/en/admin/settings.ts
- 文件：frontend/src/i18n/locales/en/admin/settings.ts
- 断言："siteNamePlaceholder: 'BuaGa API',"
- 断言："fromNamePlaceholder: 'BuaGa API',"
- 断言："grokCrossClientMapHint: 'Disabled by default. When enabled, GPT, Codex, o-series, and Claude model IDs are routed to the default Grok text model above.',"


### C-298 frontend/src/i18n/locales/en/admin/videoTasks.ts
- 文件：frontend/src/i18n/locales/en/admin/videoTasks.ts
- 断言："upstream_expired: 'The upstream service reported an expired result. Generation and billing outcomes still need review; the video will not be generated again.',"
- 断言："billing_failed: 'Billing settlement is not yet confirmed. The system will reconcile it again without generating another video.',"
- 断言："query_retry: 'A status check could not be completed. The system will retry the check; this does not create another video.',"


### C-299 frontend/src/i18n/locales/en/common.ts
- 文件：frontend/src/i18n/locales/en/common.ts
- 断言："workspaceFeatureRouting: 'Smart routing and automatic failover',"
- 断言："workspaceDescription: 'Manage keys, subscriptions, usage and channel health in one clear, controlled workspace.',"
- 断言："loginSettingsFailed: 'Login security settings could not load. Reload before signing in.',"


### C-300 frontend/src/i18n/locales/en/dashboard.ts
- 文件：frontend/src/i18n/locales/en/dashboard.ts
- 断言："usageAtGlanceDesc: 'Monitor requests, tokens, and cost in real time',"
- 断言："keysInUse: '{count} API keys in use',"
- 断言："allCompactionTypes: 'All Requests',"


### C-301 frontend/src/i18n/locales/en/landing.ts
- 文件：frontend/src/i18n/locales/en/landing.ts
- 断言："livePreview: 'Live preview',"
- 断言："heroEyebrow: 'Unified, reliable and observable AI gateway',"
- 断言："title: 'API request examples',"


### C-302 frontend/src/i18n/locales/en/misc.ts
- 文件：frontend/src/i18n/locales/en/misc.ts
- 断言："customModeHint: 'BUAGA custom build: upgrade or roll back only with verified custom images and matching source.',"
- 断言："systemDescription: 'Latest platform updates and notices',"
- 断言："noHistory: 'No announcement history',"


### C-303 frontend/src/i18n/locales/zh/admin/accounts.ts
- 文件：frontend/src/i18n/locales/zh/admin/accounts.ts
- 断言："syncUpstreamModelsRegistryUnavailable: '模型列表已同步；能力资料源暂时不可用，本次获取 {count}/{total} 个。未补全：{models}。已保存的资料保留。',"
- 断言："syncUpstreamModelsMetadataDetails: '模型列表已同步；已获取 {count}/{total} 个模型的能力资料。尚缺资料：{models}。这不代表模型调用失败。',"
- 断言："windowMonthly: '每月窗口',"


### C-304 frontend/src/i18n/locales/zh/admin/channels.ts
- 文件：frontend/src/i18n/locales/zh/admin/channels.ts
- 断言："sortOrderHint: '拖动左侧手柄调整渠道展示顺序，排在前面的渠道会优先显示。',"
- 断言："failedToUpdateSortOrder: '保存渠道展示顺序失败，请刷新后重试',"
- 断言："emptyModelsInPricing: '请为每条定价添加至少一个模型',"


### C-305 frontend/src/i18n/locales/zh/admin/index.ts
- 文件：frontend/src/i18n/locales/zh/admin/index.ts
- 断言："import overview from './overview'"


### C-306 frontend/src/i18n/locales/zh/admin/ops.ts
- 文件：frontend/src/i18n/locales/zh/admin/ops.ts
- 断言："upstreamErrorRateMaxPercentHint: '可接受的上游错误比例上限。',"
- 断言："upstreamErrorRateMaxPercent: '上游错误率上限（%）',"
- 断言："cooldownHint: '当前处于进程内临时冷却的账号 + 模型组合',"


### C-307 frontend/src/i18n/locales/zh/admin/overview.ts
- 文件：frontend/src/i18n/locales/zh/admin/overview.ts
- 断言："reasoningEffortMappingsHint: '类型和模型均可留空，表示匹配全部模型。同一类型和模型下可添加多条请求值映射，例如前缀 gpt 同时将 high、xhigh 转到 medium。精确优先于前后缀，更长前后缀优先。',"
- 断言："tooltip: '启用后，当请求包含 MCP 工具时，会在 system prompt 中注入 XML 格式调用协议提示词。关闭此选项可避免对某些客户端造成干扰。',"
- 断言："hint: '开启后，该分组的 Codex 客户端 /models 请求只用选定账号向上游拉取并按 slug 合并，不经过调度器；限流/过载中的选定账号仍会被使用。',"


### C-308 frontend/src/i18n/locales/zh/admin/resources.ts
- 文件：frontend/src/i18n/locales/zh/admin/resources.ts
- 断言："export default {"


### C-309 frontend/src/i18n/locales/zh/admin/settings.ts
- 文件：frontend/src/i18n/locales/zh/admin/settings.ts
- 断言："siteNamePlaceholder: 'BuaGa API',"
- 断言："fromNamePlaceholder: 'BuaGa API',"
- 断言："grokCrossClientMapHint: '默认关闭。开启后，GPT、Codex、o 系列和 Claude 模型 ID 会路由到左侧默认 Grok 文本模型。',"


### C-310 frontend/src/i18n/locales/zh/admin/videoTasks.ts
- 文件：frontend/src/i18n/locales/zh/admin/videoTasks.ts
- 断言："upstream_expired: '上游报告结果已过期，生成及费用结果仍待核查，不会重新生成视频。',"
- 断言："upstream_cancelled: '上游服务报告任务已取消。',"
- 断言："upstream_failed: '上游服务报告任务失败。',"


### C-311 frontend/src/i18n/locales/zh/common.ts
- 文件：frontend/src/i18n/locales/zh/common.ts
- 断言："workspaceDescription: '统一管理密钥、订阅、用量与渠道状态，让每一次调用都清晰可控。',"
- 断言："workspaceAppSupportDescription: '支持一键配置并适配主流客户端',"
- 断言："loginSettingsFailed: '登录安全配置加载失败，请重新加载后登录。',"


### C-312 frontend/src/i18n/locales/zh/dashboard.ts
- 文件：frontend/src/i18n/locales/zh/dashboard.ts
- 断言："usageAtGlanceDesc: '查看请求、Token 和费用的实时汇总',"
- 断言："keysInUse: '{count} 个 API 密钥正在使用',"
- 断言："allCompactionTypes: '全部请求',"


### C-313 frontend/src/i18n/locales/zh/landing.ts
- 文件：frontend/src/i18n/locales/zh/landing.ts
- 断言："livePreview: '实时预览',"
- 断言："heroEyebrow: '统一、稳定、可观测的 AI 网关',"
- 断言："response: 'Response',"


### C-314 frontend/src/i18n/locales/zh/misc.ts
- 文件：frontend/src/i18n/locales/zh/misc.ts
- 断言："customModeHint: 'BUAGA 定制版：升级和回滚须使用已验证的定制镜像及配套源码。',"
- 断言："systemDescription: '最新平台更新和通知',"
- 断言："noHistory: '暂无历史公告',"


### C-315 frontend/src/main.ts
- 文件：frontend/src/main.ts
- 断言："if (appStore.siteName && appStore.siteName !== 'BuaGa API') {"


### C-316 frontend/src/router/__tests__/guards.spec.ts
- 文件：frontend/src/router/__tests__/guards.spec.ts
- 断言："it('管理员简易模式访问 /admin/groups 重定向到 /admin/dashboard', () => {"
- 断言："expect(redirect).toBe('/admin/dashboard')"


### C-317 frontend/src/router/__tests__/title.spec.ts
- 文件：frontend/src/router/__tests__/title.spec.ts
- 断言："expect(resolveDocumentTitle('Dashboard', '')).toBe('Dashboard - BuaGa API')"
- 断言："expect(resolveDocumentTitle(undefined, '   ')).toBe('BuaGa API')"


### C-318 frontend/src/router/__tests__/video-tasks.spec.ts
- 文件：frontend/src/router/__tests__/video-tasks.spec.ts
- 断言："expect(adminNavigation).toMatch(/path: '\\/admin\\/scheduler'[^\\n]+\\n\\s*\\{ path: '\\/admin\\/video-tasks', label: t\\('admin.videoTasks.title'\\), icon: VideoTaskIcon \\}/)"
- 断言："vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'Test', backendModeEnabled: false, cachedPublicSettings: null }) }))"
- 断言："expect(source.slice(0, source.indexOf('const adminNavItems = computed'))).not.toContain(\"path: '/admin/video-tasks'\")"


### C-319 frontend/src/router/index.ts
- 文件：frontend/src/router/index.ts
- 断言："authStore.initializeLocalPreview(to.path.startsWith('/admin') ? 'admin' : 'user')"
- 断言："const localPreviewMode = import.meta.env.VITE_LOCAL_PREVIEW === 'true'"
- 断言："component: () => import('@/views/admin/VideoTasksView.vue'),"


### C-320 frontend/src/router/title.ts
- 文件：frontend/src/router/title.ts
- 断言："const normalizedSiteName = typeof siteName === 'string' && siteName.trim() ? siteName.trim() : 'BuaGa API'"


### C-321 frontend/src/stores/announcements.ts
- 文件：frontend/src/stores/announcements.ts
- 断言："if (currentPopup.value) shownPopupIds.add(currentPopup.value.id)"
- 断言："if (id !== undefined) void markAsRead(id)"
- 断言："for (const popup of popupQueue.value) {"


### C-322 frontend/src/stores/app.ts
- 文件：frontend/src/stores/app.ts
- 断言："siteName.value = config.site_name || 'BuaGa API'"
- 断言："const siteName = ref<string>('BuaGa API')"


### C-323 frontend/src/stores/auth.ts
- 文件：frontend/src/stores/auth.ts
- 断言："function initializeLocalPreview(role: 'user' | 'admin'): void {"
- 断言："username: role === 'admin' ? 'preview-admin' : 'preview-user',"
- 断言："if (import.meta.env.VITE_LOCAL_PREVIEW !== 'true') return"


### C-324 frontend/src/style.css
- 文件：frontend/src/style.css
- 断言：".newapi-search-result { display: flex; width: 100%; align-items: center; gap: 0.55rem; border-radius: 0.4rem; padding: 0.55rem 0.5rem; color: rgb(39 39 42); font-size: 0.75rem; }"
- 断言：".newapi-summary-card { border-color: var(--overview-border) !important; border-radius: 0.75rem !important; box-shadow: 0 1px 2px rgb(0 0 0 / 0.035) !important; }"
- 断言：".newapi-metric-icon { display: inline-flex; width: 1.75rem; height: 1.75rem; flex: none; align-items: center; justify-content: center; border-radius: 0.4rem; }"


### C-325 frontend/src/styles/onboarding.css
- 文件：frontend/src/styles/onboarding.css
- 断言："/* Sub2API Interactive Tour Styles - DOM Restructured Version */"


### C-326 frontend/src/types/index.ts
- 文件：frontend/src/types/index.ts
- 断言："usage_source?: 'upstream' | 'estimated' | null"
- 断言："long_notice_content?: string"
- 断言："video_previews?: string[]"


### C-327 frontend/src/types/longNotice.ts
- 文件：frontend/src/types/longNotice.ts
- 断言："export type LongNoticeBlockType = 'status' | 'heading' | 'text' | 'metrics' | 'feature' | 'callout' | 'buttons'"
- 断言："export const emptyLongNoticeDocument = (): LongNoticeDocument => ({ version: 1, blocks: [] })"
- 断言："if (parsed.version !== 1 || !Array.isArray(parsed.blocks)) return emptyLongNoticeDocument()"


### C-328 frontend/src/utils/__tests__/formatDateTimeLocalInput.spec.ts
- 文件：frontend/src/utils/__tests__/formatDateTimeLocalInput.spec.ts
- 断言："const spy = vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(() => { throw new Error('unavailable') })"
- 断言："it('validates leap days and years below 100 without the Date constructor offset', () => {"
- 断言："expect(parseDateTimeLocalInput('2024-02-29T12:00')).not.toBeNull()"


### C-329 frontend/src/utils/__tests__/intervalPrices.spec.ts
- 文件：frontend/src/utils/__tests__/intervalPrices.spec.ts
- 断言："expect(resolveIntervalPrices({ ...interval, input_multiplier: 2, output_multiplier: 1.5, cache_read_multiplier: 3 }, base))"
- 断言："it('applies input, output and cache-read multipliers without changing base prices', () => {"
- 断言："cache_write_1h_price: null, cache_read_price: null, per_request_price: null"


### C-330 frontend/src/utils/__tests__/upstreamModelSync.spec.ts
- 文件：frontend/src/utils/__tests__/upstreamModelSync.spec.ts
- 断言："const warning = { code: 'upstream_model_metadata_partial', message: '', model_ids: ['hy4'], complete_count: 25, total_count: 26, reason: 'metadata_missing' }"
- 断言："expect(formatUpstreamModelSyncWarning([{ ...warning, reason: 'registry_unavailable' }], t)).toBe('admin.accounts.syncUpstreamModelsRegistryUnavailable')"
- 断言："expect(t).toHaveBeenLastCalledWith('admin.accounts.syncUpstreamModelsMetadataDetails', { count: 25, total: 26, models: 'hy4' })"


### C-331 frontend/src/utils/format.ts
- 文件：frontend/src/utils/format.ts
- 断言："const [year, month, day, hour, minute, second] = [yearText, monthText, dayText, hourText, minuteText, secondText].map(Number)"
- 断言："if (year < 1 || month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59) return null"
- 断言："const [, yearText, monthText, dayText, hourText, minuteText, secondText = '0', fraction = ''] = match"


### C-332 frontend/src/utils/pricing.ts
- 文件：frontend/src/utils/pricing.ts
- 断言："'input_price' | 'output_price' | 'cache_write_price' | 'cache_write_1h_price' | 'cache_read_price' | 'per_request_price'>>"
- 断言："cache_write_price: interval.cache_write_price ?? scaled(base.cache_write_price, interval.cache_write_multiplier),"
- 断言："cache_read_price: interval.cache_read_price ?? scaled(base.cache_read_price, interval.cache_read_multiplier),"


### C-333 frontend/src/utils/turnstileLoader.ts
- 文件：frontend/src/utils/turnstileLoader.ts
- 断言："element.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?onload=onTurnstileLoad&render=explicit'"
- 断言："const timer = setTimeout(() => finish(new Error('Turnstile load timed out')), 15000)"
- 断言："const selector = 'script[src*=\"challenges.cloudflare.com/turnstile/v0/api.js\"]'"


### C-334 frontend/src/utils/upstreamModelSync.ts
- 文件：frontend/src/utils/upstreamModelSync.ts
- 断言："const warning = warnings.find(item => item.code === 'upstream_model_metadata_incomplete')"
- 断言："?? warnings.find(item => item.code === 'upstream_model_metadata_partial')"
- 断言："return t(warning.code === 'upstream_model_metadata_partial'"


### C-335 frontend/src/views/HomeView.vue
- 文件：frontend/src/views/HomeView.vue
- 断言："const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'BuaGa API')"
- 断言：".premium-hero h1 { max-width: 100%; color: #0f172a; font-size: clamp(3.25rem, 7vw, 5.8rem); line-height: .98; font-weight: 760; letter-spacing: 0; overflow-wrap: anywhere; }"
- 断言："[{ text: '  \"candidates\": [{ \"content\": { \"parts\": [{ \"text\": ', tone: 'cyan' }, { text: '\"Gemini request served.\"', tone: 'green' }, { text: ' }] } }],', tone: 'cyan' }],"


### C-336 frontend/src/views/KeyUsageView.vue
- 文件：frontend/src/views/KeyUsageView.vue
- 断言："const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'BuaGa API')"
- 断言："params.set('timezone', getBrowserTimeZone())"


### C-337 frontend/src/views/admin/AccountsView.vue
- 文件：frontend/src/views/admin/AccountsView.vue
- 断言："<TablePageLayout class=\"newapi-admin-workspace\">"
- 断言：":interactive=\"false\""


### C-338 frontend/src/views/admin/AnnouncementsView.vue
- 文件：frontend/src/views/admin/AnnouncementsView.vue
- 断言："<div class=\"px-3 pt-4 sm:px-5\"><LongNoticeEditor /></div>"


### C-339 frontend/src/views/admin/ChannelMonitorView.vue
- 文件：frontend/src/views/admin/ChannelMonitorView.vue
- 断言："sortableMonitors.value.map((monitor, index) => ({ id: monitor.id, sort_order: index * 10 }))"
- 断言："class=\"flex items-center gap-3 rounded-xl border border-blue-100/80 bg-blue-50/40 p-3 transition-shadow hover:shadow-sm dark:border-dark-600 dark:bg-dark-700/70\""
- 断言："<button type=\"button\" class=\"btn btn-secondary\" :disabled=\"sortSubmitting\" @click=\"closeSortDialog\">"


### C-340 frontend/src/views/admin/ChannelsView.vue
- 文件：frontend/src/views/admin/ChannelsView.vue
- 断言："appStore.showError(extractApiErrorMessage(error, '无法读取最新渠道配置，请刷新页面后重试'))"
- 断言："channel = await adminAPI.channels.getById(channel.id)"
- 断言："expected_updated_at: editingChannel.value.updated_at,"


### C-341 frontend/src/views/admin/DashboardView.vue
- 文件：frontend/src/views/admin/DashboardView.vue
- 断言："<div class=\"newapi-system-row\"><span><Icon name=\"key\" size=\"xs\" />{{ t('admin.dashboard.apiKeys') }}</span><b>{{ stats.active_api_keys }} / {{ stats.total_api_keys }}</b></div>"
- 断言："<div class=\"newapi-system-row\"><span><Icon name=\"users\" size=\"xs\" />{{ t('admin.dashboard.users') }}</span><b>{{ formatNumber(stats.total_users) }}</b></div>"
- 断言："<span class=\"newapi-metric-icon bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300\"><Icon name=\"dollar\" size=\"sm\" /></span>"


### C-342 frontend/src/views/admin/GroupsView.vue
- 文件：frontend/src/views/admin/GroupsView.vue
- 断言："class=\"relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none\""
- 断言："resetModelsListState(editModelsListState, editForm.platform === editingGroup.value.platform ? editingGroup.value.models_list_config : undefined);"
- 断言："class=\"pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out\""


### C-343 frontend/src/views/admin/RedeemView.vue
- 文件：frontend/src/views/admin/RedeemView.vue
- 断言："if (!batchUpdateForm.expires_at_local || Number.isNaN(expiresAt.getTime())) {"
- 断言："const expiresAt = new Date(batchUpdateForm.expires_at_local)"
- 断言："appStore.showError(t('admin.redeem.expiryDaysRequired'))"


### C-344 frontend/src/views/admin/SettingsView.vue
- 文件：frontend/src/views/admin/SettingsView.vue
- 断言："(form.payment_product_name_prefix || \"BuaGa API\") +"
- 断言："long_notice_content: form.long_notice_content,"
- 断言："long_notice_content: \"\","


### C-345 frontend/src/views/admin/SubscriptionsView.vue
- 文件：frontend/src/views/admin/SubscriptionsView.vue
- 断言："<TablePageLayout class=\"newapi-admin-workspace\">"
- 断言："class=\"font-medium text-primary-600 hover:underline dark:text-primary-400\""


### C-346 frontend/src/views/admin/UsageView.vue
- 文件：frontend/src/views/admin/UsageView.vue
- 断言："<div class=\"newapi-usage-workspace space-y-5\">"
- 断言："<div class=\"newapi-filter-toolbar card p-3\">"
- 断言："filters.value = { start_date: startDate.value, end_date: endDate.value, request_type: undefined, billing_type: null, billing_mode: undefined }"


### C-347 frontend/src/views/admin/UsersView.vue
- 文件：frontend/src/views/admin/UsersView.vue
- 断言："<TablePageLayout class=\"newapi-admin-workspace\">"


### C-348 frontend/src/views/admin/VideoTasksView.vue
- 文件：frontend/src/views/admin/VideoTasksView.vue
- 断言："<div class=\"mt-2 whitespace-nowrap text-xs text-gray-500 dark:text-gray-400\">{{ t('admin.videoTasks.fields.expected') }}: {{ formatCost(row.expected_cost, locale) }}</div>"
- 断言："<div class=\"mt-1 whitespace-nowrap text-xs text-gray-500 dark:text-gray-400\">{{ t('admin.videoTasks.fields.charged') }}: {{ formatCost(row.charged_cost, locale) }}</div>"
- 断言："<p class=\"py-8 text-center text-sm text-gray-500 dark:text-gray-400\">{{ t(loadFailed ? 'admin.videoTasks.loadFailed' : 'admin.videoTasks.empty') }}</p>"


### C-349 frontend/src/views/admin/__tests__/ChannelMonitorView.grok.spec.ts
- 文件：frontend/src/views/admin/__tests__/ChannelMonitorView.grok.spec.ts
- 断言："expect(providerButtons).toHaveLength(PROVIDERS.length)"


### C-350 frontend/src/views/admin/__tests__/GroupsView.codexManifest.spec.ts
- 文件：frontend/src/views/admin/__tests__/GroupsView.codexManifest.spec.ts
- 断言："expect(wrapper.find('[data-testid=\"codex-manifest-field\"]').exists()).toBe(false);"
- 断言："expect(updateGroup).toHaveBeenCalledWith(42, expect.objectContaining({"
- 断言："expose({ validate: () => true, resetValidation: () => undefined });"


### C-351 frontend/src/views/admin/__tests__/GroupsView.columnSettings.spec.ts
- 文件：frontend/src/views/admin/__tests__/GroupsView.columnSettings.spec.ts
- 断言："getModelsListCandidates.mockResolvedValue([])"
- 断言："getModelsListCandidates.mockReset()"
- 断言："getModelsListCandidates: vi.fn(),"


### C-352 frontend/src/views/admin/__tests__/GroupsView.duplicate.spec.ts
- 文件：frontend/src/views/admin/__tests__/GroupsView.duplicate.spec.ts
- 断言："getModelsListCandidates.mockResolvedValue([])"
- 断言："getModelsListCandidates: vi.fn(),"
- 断言："models_list_config: undefined,"


### C-353 frontend/src/views/admin/__tests__/SettingsView.spec.ts
- 文件：frontend/src/views/admin/__tests__/SettingsView.spec.ts
- 断言："import { beforeEach, describe, expect, it, vi } from \"vitest\";"


### C-354 frontend/src/views/admin/__tests__/UsageView.spec.ts
- 文件：frontend/src/views/admin/__tests__/UsageView.spec.ts
- 断言："import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'"


### C-355 frontend/src/views/admin/__tests__/VideoTasksView.spec.ts
- 文件：frontend/src/views/admin/__tests__/VideoTasksView.spec.ts
- 断言："for (const code of ['poll_failed', 'poll_timeout', 'upstream_failed', 'upstream_expired', 'completion_save_failed', 'billing_failed', 'upstream_cancelled', 'poll_not_found', 'poll_exhausted', 'account_route_changed']) {"
- 断言："expect(select('video-task-page-size').props('options').map((option: { value: number }) => option.value)).toEqual([20, 50, 100])"
- 断言："...({ prompt: 'PRIVATE PROMPT', url: 'https://private.invalid/video', api_key: 'PRIVATE TOKEN' } as object)"


### C-356 frontend/src/views/admin/__tests__/groupsModelsList.spec.ts
- 文件：frontend/src/views/admin/__tests__/groupsModelsList.spec.ts
- 断言："it(\"keeps selected models in payload even when disabled so reopening can restore choices\", () => {"
- 断言："it(\"keeps saved selections and marks new candidates as unselected when editing\", () => {"
- 断言："it(\"preserves explicitly unselected saved candidates when candidates refresh\", () => {"


### C-357 frontend/src/views/admin/__tests__/groupsModelsListCandidates.spec.ts
- 文件：frontend/src/views/admin/__tests__/groupsModelsListCandidates.spec.ts
- 断言："it(\"rejects stale candidate responses after a newer platform request starts\", () => {"
- 断言："it(\"rejects responses for a previous edit group even with the same platform\", () => {"
- 断言："it(\"tracks create and edit requests independently\", () => {"


### C-358 frontend/src/views/admin/__tests__/groupsModelsListLayout.spec.ts
- 文件：frontend/src/views/admin/__tests__/groupsModelsListLayout.spec.ts
- 断言："expect(groupsViewSource).not.toContain(\"sticky top-0\");"
- 断言："expect(groupsViewSource).toContain(\"max-h-64 space-y-2 overflow-y-auto p-2\");"
- 断言："it(\"uses a wide dialog and keeps model pricing controls responsive\", () => {"


### C-359 frontend/src/views/admin/__tests__/groupsReasoningEffort.spec.ts
- 文件：frontend/src/views/admin/__tests__/groupsReasoningEffort.spec.ts
- 断言："import { describe, expect, it } from \"vitest\";"


### C-360 frontend/src/views/admin/groupsModelsList.ts
- 文件：frontend/src/views/admin/groupsModelsList.ts
- 断言："selected: selected && (currentKnown.has(id) || savedSelected.has(id) || state.savedModels.length === 0),"
- 断言："export const buildModelsListConfig = (state: ModelsListState): ModelsListConfig => ({"
- 断言："export const toggleModelsListItem = (state: ModelsListState, modelID: string) => {"


### C-361 frontend/src/views/admin/groupsModelsListCandidates.ts
- 文件：frontend/src/views/admin/groupsModelsListCandidates.ts
- 断言："export const createModelsListCandidatesTracker = (): ModelsListCandidatesTracker => {"
- 断言："isCurrent(requestID: number, request: ModelsListCandidatesRequest): boolean;"
- 断言："const currentByMode: Partial<Record<ModelsListCandidatesMode, {"


### C-362 frontend/src/views/admin/groupsReasoningEffort.ts
- 文件：frontend/src/views/admin/groupsReasoningEffort.ts
- 断言："const to = normalizeReasoningEffortForPlatform(platform, mapping.to);"
- 断言："} else if (!normalizeReasoningEffortForPlatform(platform, to)) {"


### C-363 frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue
- 文件：frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue
- 断言："include_recovered_upstream: props.preset.include_recovered_upstream"
- 断言："props.preset.include_recovered_upstream,"
- 断言："include_recovered_upstream?: boolean"


### C-364 frontend/src/views/admin/scheduler/SchedulerMonitorView.vue
- 文件：frontend/src/views/admin/scheduler/SchedulerMonitorView.vue
- 断言："<h2 class=\"text-base font-bold text-gray-900 dark:text-white\">{{ t('admin.ops.scheduler.cooldownList') }}</h2>"
- 断言："<p class=\"mt-1 text-xs text-gray-500 dark:text-gray-400\">{{ t('admin.ops.scheduler.cooldownListHint') }}</p>"
- 断言："opsAPI.listUpstreamErrors({ time_range: '1h', platform: 'openai', page: 1, page_size: 8, view: 'errors' })"


### C-365 frontend/src/views/admin/scheduler/schedulerRecovery.spec.ts
- 文件：frontend/src/views/admin/scheduler/schedulerRecovery.spec.ts
- 断言："expect(recoveryReasonLabel({ ...row, reason: 'upstream_502' }, 'zh')).toBe('上游 HTTP 502')"
- 断言："const row: OpsModelCooldown = { account_id: 1, model: '*', failure_streak: 1,"
- 断言："expect(recoveryReasonLabel({ ...row, reason: 'cross_model_503' }, 'en')).toBe('Persistent failures across models')"


### C-366 frontend/src/views/admin/scheduler/schedulerRecovery.ts
- 文件：frontend/src/views/admin/scheduler/schedulerRecovery.ts
- 断言："if (row.reason?.startsWith('anthropic_')) return zh ? '上游流式响应失败（仅此模型）' : 'Upstream stream failure (this model only)'"
- 断言："if (row.reason === 'upstream_transport') return zh ? '上游传输中断' : 'Upstream transport interrupted'"
- 断言："if (row.reason === 'upstream_connection') return zh ? '上游连接失败' : 'Upstream connection failed'"


### C-367 frontend/src/views/admin/videoTasksPresentation.ts
- 文件：frontend/src/views/admin/videoTasksPresentation.ts
- 断言："export function elapsedParts(item: VideoTaskAdminItem): { hours: number; minutes: number; seconds: number } | null {"
- 断言："export function displayStatus(item: VideoTaskAdminItem, now = Date.now()): VideoTaskStatus | 'unknown' {"
- 断言："return videoTaskStatuses.includes(status as VideoTaskStatus) ? status as VideoTaskStatus : 'unknown'"


### C-368 frontend/src/views/auth/EmailVerifyView.vue
- 文件：frontend/src/views/auth/EmailVerifyView.vue
- 断言："siteName.value = settings.site_name || 'BuaGa API'"
- 断言："const siteName = ref<string>('BuaGa API')"


### C-369 frontend/src/views/auth/LoginView.vue
- 文件：frontend/src/views/auth/LoginView.vue
- 断言：".premium-login-form-kicker { display: inline-flex; align-items: center; gap: .35rem; color: rgb(37 99 235); font-size: .7rem; font-weight: 650; }"
- 断言：".premium-login-submit { min-height: 3rem; border-radius: .55rem; background: rgb(37 99 235); box-shadow: 0 10px 22px rgb(37 99 235 / .18); }"
- 断言："() => isLoading.value || passkeyLoading.value || !publicSettingsLoaded.value || publicSettingsFailed.value || agreementGateActive.value"


### C-370 frontend/src/views/auth/RegisterView.vue
- 文件：frontend/src/views/auth/RegisterView.vue
- 断言："siteName.value = settings.site_name || 'BuaGa API'"
- 断言："const siteName = ref<string>('BuaGa API')"


### C-371 frontend/src/views/public/LegalDocumentView.vue
- 文件：frontend/src/views/public/LegalDocumentView.vue
- 断言："const siteName = computed(() => settings.value?.site_name || 'BuaGa API')"


### C-372 frontend/src/views/user/DashboardView.vue
- 文件：frontend/src/views/user/DashboardView.vue
- 断言："<div class=\"newapi-overview space-y-5\">"


### C-373 frontend/src/views/user/KeysView.vue
- 文件：frontend/src/views/user/KeysView.vue
- 断言："<TablePageLayout class=\"newapi-keys-workspace\">"


### C-374 frontend/src/views/user/PaymentView.vue
- 文件：frontend/src/views/user/PaymentView.vue
- 断言："<div v-if=\"tabs.length > 1 && paymentPhase === 'select' && !selectedPlan\" class=\"newapi-wallet-tabs flex space-x-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-800\">"
- 断言："<div class=\"newapi-wallet-page mx-auto max-w-5xl space-y-5\">"


### C-375 frontend/src/views/user/ProfileView.vue
- 文件：frontend/src/views/user/ProfileView.vue
- 断言："class=\"newapi-profile-page mx-auto max-w-[1000px] space-y-5\""


### C-376 frontend/src/views/user/RedeemView.vue
- 文件：frontend/src/views/user/RedeemView.vue
- 断言："<div class=\"newapi-redeem-page mx-auto max-w-4xl space-y-5\">"
- 断言："<div class=\"newapi-redeem-balance card overflow-hidden\">"
- 断言："<div class=\"newapi-redeem-history card\">"


### C-377 frontend/src/views/user/SubscriptionsView.vue
- 文件：frontend/src/views/user/SubscriptionsView.vue
- 断言："class=\"newapi-subscription-item overflow-hidden rounded-2xl border bg-white dark:bg-dark-800\""
- 断言："<div v-else class=\"newapi-subscription-grid grid gap-4 lg:grid-cols-2\">"
- 断言："<div class=\"newapi-subscriptions-workspace space-y-5\">"


### C-378 frontend/src/views/user/UsageView.vue
- 文件：frontend/src/views/user/UsageView.vue
- 断言："<div class=\"newapi-usage-workspace space-y-5\">"
- 断言："<div class=\"newapi-filter-toolbar card p-3\">"
- 断言："<div class=\"newapi-filter-panel card p-4\">"


### C-379 frontend/src/views/user/UserOrdersView.vue
- 文件：frontend/src/views/user/UserOrdersView.vue
- 断言："<div class=\"newapi-orders-workspace space-y-4\">"
- 断言："<div class=\"newapi-orders-toolbar card p-3\">"


### C-380 frontend/src/views/user/__tests__/UsageView.spec.ts
- 文件：frontend/src/views/user/__tests__/UsageView.spec.ts
- 断言："list.mockResolvedValue({ items: [{ id: 1, name: 'demo-key' }] })"

### C-413 视频成功结果异步持久化 7 天缓存
- 文件：backend/internal/service/video_preview_cache.go
- 断言："videoPreviewCacheRetention  = 7 * 24 * time.Hour"
- 断言："return os.Rename(stage, filepath.Join(v.root, key))"
- 断言："func (s *OpenAIGatewayService) ServeCachedAdminVideoPreview"

### C-414 视频缓存按成功完整文件发布并安全清理
- 文件：backend/internal/service/video_preview_capture.go
- 断言："func (s *OpenAIGatewayService) captureCompletedVideo"
- 断言："streamGrokSignedVideoPreviewURL"

### C-415 视频缓存回看优先于上游
- 文件：backend/internal/handler/admin/usage_video.go
- 断言："ServeCachedAdminVideoPreview(c, log)"
- 断言："video account is unavailable"

### C-416 DeepSeek 缓存命中用量兼容
- 文件：backend/internal/service/openai_gateway_response_handling.go
- 断言："value.Get(\"prompt_cache_hit_tokens\")"
- 断言："value.Get(\"input_tokens_details.cached_tokens\")"

### C-420 Grok 新生成任务预报价与持久化自动结算
- 文件：backend/internal/handler/grok_media.go
- 断言："GrokVideoDurableSettlementEnabled()"
- 断言："PersistDurableVideoTask(saveCtx"
- 断言："shouldBindLegacyVideoTask = !durableGrokCreate"

### C-421 Grok 无客户端查询结算与历史任务隔离回归
- 文件：backend/internal/service/grok_video_settlement_test.go
- 断言："TestGrokVideoSettlementWithoutClientPolling"
- 断言："TestGrokVideoSettlementPreflightAndPersistenceFailure"
- 断言："TestGrokVideoSettlementAmbiguousAcceptanceDoesNotFailover"

### C-422 Grok 结算分阶段发布默认关闭
- 文件：backend/internal/config/config.go
- 断言："viper.SetDefault(\"gateway.grok_video_durable_settlement_enabled\", false)"

### C-423 公网直连由应用提供独立 TLS 监听
- 文件：backend/cmd/server/direct_api_tls.go
- 断言："func startDirectAPITLS(base *http.Server, forwardedIPHeaders func() []string) *http.Server"
- 断言："server.ServeTLS(listener, \"\", \"\")"
- 断言："!directAPIAllowedPath(path, r.Method)"

### C-424 直连与主服务共享应用并同步关闭
- 文件：backend/cmd/server/main.go
- 断言："directServer := startDirectAPITLS(app.Server, func() []string {"
- 断言："directServer.Shutdown(ctx)"

### C-425 直连证书隔离续签
- 文件：ops/release/sync-direct-api-cert.sh
- 断言："destination=/opt/sub2api-return/direct-api-tls"
- 断言："mv -Tf \"$destination/.current-next\" \"$destination/current\""

### C-426 上游流内超时按账号模型避让并换号
- 文件：backend/internal/service/openai_gateway_passthrough.go
- 断言："func isOpenAIStreamTimeoutEvent(payload []byte) bool"
- 断言："failoverErr.Reason = OpenAIStreamTimeoutReason"
- 断言："account.IsPoolMode() && isOpenAIStreamTimeoutEvent(payload)"
- 断言："upstream_stream_timeout"

### C-427 流超时换号耗尽后返回 503 语义错误
- 文件：backend/internal/handler/openai_gateway_handler.go
- 断言："if failoverErr.Reason == service.OpenAIStreamTimeoutReason {"
- 断言："http.StatusServiceUnavailable, \"server_error\", \"stream_timeout\""

### C-428 Anthropic 池账号模型限流冷却与首输出等待上限
- 文件：backend/internal/service/gateway_anthropic_availability.go
- 断言："const anthropicFirstOutputWait = 30 * time.Second"
- 断言："func (s *GatewayService) recordAnthropicRateLimit"
- 断言："func guardAnthropicFirstOutput"

### C-429 Anthropic 冷却恢复与长回复安全回归
- 文件：backend/internal/service/gateway_anthropic_availability_test.go
- 断言："func TestAnthropicAvailabilityCooldownRecovery"
- 断言："func TestAnthropicAvailabilityTimeoutHeadersAndHeartbeats"
- 断言："func TestAnthropicAvailabilityReplayAndStopDeadline"
