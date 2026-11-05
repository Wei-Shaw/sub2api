package reqcompress

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

// SendFunc performs one upstream attempt. Implementations must not mutate the
// request: every attempt is treated as an independent measured request.
type SendFunc func(*http.Request) (*http.Response, error)

// Execute forwards req with the content coding negotiated for accountID.
//
// When the account is not opted in, the request is not eligible, or another
// request already owns the trial, req is sent unchanged. Otherwise codings are
// attempted in preference order and the first successful response is returned.
// A coded attempt that fails with an ambiguous upstream error is retried once as
// identity, so enabling compression can never turn a request that would
// otherwise succeed into a failure. Recognized encoding rejections continue to
// probe the remaining codings and end at identity as well.
//
// A coded attempt may be discarded, so it is measured in its own probe
// collector: only the attempt returned to the caller is adopted into the
// request's usage record, while a discarded one is logged with its wire cost.
// Probing therefore never inflates the traffic a client request reports.
func Execute(accountID int64, req *http.Request, send SendFunc) (*http.Response, error) {
	return defaultRegistry.Execute(accountID, req, send)
}

// Execute forwards req with the content coding negotiated in this registry.
func (r *Registry) Execute(accountID int64, req *http.Request, send SendFunc) (*http.Response, error) {
	if send == nil {
		return nil, errors.New("reqcompress: nil send function")
	}
	slot := r.existingSlot(accountID)
	if slot == nil {
		return send(req)
	}
	negotiation := slot.currentState()
	if negotiation == nil || !Eligible(req) {
		return send(req)
	}
	if err := r.hydrateShared(req.Context(), accountID, negotiation); err != nil {
		// Without a readable shared snapshot, probing br again could overwrite a
		// coding another instance already pinned. Send identity for this request
		// and retry shared initialization on the next one.
		slog.Warn("reqcompress shared state load failed", "account_id", accountID, "error", err)
		return send(req)
	}

	work, ownsNegotiation := negotiation.begin()
	if len(work) == 0 {
		return send(req)
	}
	levels := r.compressionLevels(req.Context())
	lastStatus := 0
	for len(work) > 0 {
		algorithm := work[0]
		work = work[1:]

		attempt, err := encodeRequestWithLevels(req, algorithm, levels)
		if err != nil {
			slog.Warn("reqcompress encode failed", "account_id", accountID,
				"algorithm", algorithm, "host", logHost(req), "method", req.Method, "error", err)
			if ownsNegotiation {
				negotiation.abortNegotiation()
			}
			return send(req)
		}
		// This attempt may be discarded, so its bytes are measured apart from the
		// request. Adopting it on delivery keeps the usage record limited to the
		// request the client actually received.
		probe := traffic.NewCollector()
		attempt = attempt.WithContext(traffic.WithCollector(attempt.Context(), probe))
		attemptedBytes := attempt.ContentLength
		originalBytes, hasOriginalBytes := traffic.OriginalRequestBodyBytes(attempt.Context())
		if hasOriginalBytes && originalBytes > 0 {
			slog.Debug("reqcompress attempt", "account_id", accountID,
				"algorithm", algorithm,
				"content_encoding", attempt.Header.Get("Content-Encoding"),
				"host", logHost(req), "method", req.Method,
				"original_bytes", originalBytes,
				"compressed_bytes", attemptedBytes,
				"ratio_percent", (1-float64(attemptedBytes)/float64(originalBytes))*100)
		} else {
			slog.Debug("reqcompress attempt", "account_id", accountID,
				"algorithm", algorithm,
				"content_encoding", attempt.Header.Get("Content-Encoding"),
				"host", logHost(req), "method", req.Method,
				"compressed_bytes", attemptedBytes)
		}
		resp, err := send(attempt)
		if resp != nil {
			lastStatus = resp.StatusCode
		}
		if err != nil {
			// A transport failure says nothing about the coding. Leave the
			// trial open so the next request probes again. The attempt never
			// delivered a response, but its sent bytes are the request's own.
			adoptDeliveredAttempt(req, probe)
			if ownsNegotiation {
				negotiation.abortNegotiation()
			}
			return resp, err
		}
		if !isEncodingRejection(resp) {
			if encodingAccepted(resp) {
				adoptDeliveredAttempt(req, probe)
				if ownsNegotiation {
					negotiation.negotiationSucceeded(algorithm)
					r.persistSharedAlgorithm(req.Context(), accountID, algorithm)
					fields := []any{"account_id", accountID, "algorithm", algorithm,
						"host", logHost(req), "method", req.Method,
						"original_bytes", originalBytes, "compressed_bytes", attemptedBytes}
					if hasOriginalBytes && originalBytes > 0 {
						fields = append(fields, "ratio_percent", (1-float64(attemptedBytes)/float64(originalBytes))*100)
					}
					slog.Info("reqcompress coding settled", fields...)
				}
				return resp, nil
			}
			if !shouldRetryIdentity(resp) {
				if ownsNegotiation {
					negotiation.abortNegotiation()
				}
				return resp, nil
			}
			status := resp.StatusCode
			if ownsNegotiation {
				// An ambiguous failure during the trial must still walk the
				// preference order: a provider may reject brotli with a generic
				// error while accepting zstd or deflate. If every coding fails,
				// the loop ends with an identity attempt and pins identity.
				slog.Warn("reqcompress coding unverified",
					append([]any{"account_id", accountID, "algorithm", algorithm,
						"status", status, "host", logHost(req), "method", req.Method,
						"upstream_request_id", upstreamRequestID(resp)},
						discardedAttemptLogFields(probe)...)...)
				drainAndClose(resp)
				negotiation.negotiationCandidateFailed(algorithm)
				continue
			}
			// The settled coding failed with an error that may or may not be
			// coding-related. Retry once as identity so the feature cannot turn a
			// request that would otherwise succeed into a failure.
			fallbackFields := append([]any{"account_id", accountID, "algorithm", algorithm,
				"status", status, "host", logHost(req), "method", req.Method,
				"upstream_request_id", upstreamRequestID(resp)},
				discardedAttemptLogFields(probe)...)
			if negotiation.claimFallbackLog(time.Now()) {
				slog.Warn("reqcompress identity fallback", fallbackFields...)
			} else {
				slog.Debug("reqcompress identity fallback", fallbackFields...)
			}
			drainAndClose(resp)
			identityResp, identityErr := send(req)
			if identityErr != nil {
				if ownsNegotiation {
					// A failed trial must not keep probing on every request.
					if negotiation.pinIdentity() {
						slog.Warn("reqcompress pinned identity", "account_id", accountID,
							"algorithm", algorithm, "status", status, "host", logHost(req),
							"method", req.Method, "reason", "fallback_error", "error", identityErr)
					}
				}
				return identityResp, identityErr
			}
			if ownsNegotiation || encodingAccepted(identityResp) {
				// A successful identity attempt proves the coding caused the
				// failure; a failed one during the trial still must not keep
				// re-probing on every request.
				reason := "fallback_succeeded"
				if !encodingAccepted(identityResp) {
					reason = "trial_failed"
				}
				if negotiation.pinIdentity() {
					if !ownsNegotiation {
						r.deleteSharedAlgorithm(req.Context(), accountID, algorithm)
					}
					slog.Warn("reqcompress pinned identity", "account_id", accountID,
						"algorithm", algorithm, "status", status, "host", logHost(req),
						"method", req.Method, "reason", reason)
				}
			}
			return identityResp, nil
		}
		status := resp.StatusCode
		slog.Warn("reqcompress coding rejected",
			append([]any{"account_id", accountID, "algorithm", algorithm,
				"status", status, "host", logHost(req), "method", req.Method,
				"upstream_request_id", upstreamRequestID(resp)},
				discardedAttemptLogFields(probe)...)...)
		drainAndClose(resp)
		if ownsNegotiation {
			negotiation.negotiationCandidateFailed(algorithm)
			continue
		}
		// The settled coding broke: drop it and, when this request becomes the
		// owner, try the remaining codings immediately.
		next, owns := negotiation.retireSettled(algorithm)
		r.deleteSharedAlgorithm(req.Context(), accountID, algorithm)
		if !owns {
			break
		}
		slog.Warn("reqcompress settled coding retired", "account_id", accountID,
			"algorithm", algorithm, "status", status, "host", logHost(req),
			"method", req.Method, "remaining_candidates", len(next))
		ownsNegotiation = true
		work = append(work, next...)
	}
	if ownsNegotiation {
		negotiation.pinIdentity()
		slog.Warn("reqcompress no coding accepted", "account_id", accountID,
			"host", logHost(req), "method", req.Method, "last_status", lastStatus)
		slog.Warn("reqcompress pinned identity", "account_id", accountID,
			"host", logHost(req), "method", req.Method,
			"reason", "all_codings_failed", "last_status", lastStatus)
	}
	return send(req)
}

// encodingAccepted reports whether a response proves the upstream decoded the
// request entity. Only success settles a coding: a 401/429/5xx from a different
// failure layer would otherwise pin a coding that was never exercised.
func encodingAccepted(resp *http.Response) bool {
	return resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 400
}

// adoptDeliveredAttempt publishes the probe of the attempt being returned to the
// caller. The request's collector aggregates every attempt it is given, so an
// attempt that is delivered must be moved into it while a discarded probe stays
// out of the usage record.
func adoptDeliveredAttempt(req *http.Request, probe *traffic.Collector) {
	if req == nil || probe == nil {
		return
	}
	if target := traffic.FromContext(req.Context()); target != nil {
		target.Adopt(probe)
	}
}

// discardedAttemptLogFields reports the wire cost of an attempt that is retried
// or dropped, so probing remains observable without being charged to the
// request's usage record.
func discardedAttemptLogFields(probe *traffic.Collector) []any {
	usage := probe.Snapshot()
	if usage == nil || usage.Upstream == nil || usage.Upstream.Request == nil {
		return nil
	}
	measurement := usage.Upstream.Request
	fields := make([]any, 0, 6)
	if measurement.OriginalBytes != nil {
		fields = append(fields, "original_bytes", *measurement.OriginalBytes)
	}
	if measurement.ActualBytes != nil {
		fields = append(fields, "sent_bytes", *measurement.ActualBytes)
	}
	if ratio := measurement.Ratio(); ratio != nil {
		fields = append(fields, "ratio_percent", *ratio)
	}
	return fields
}

// drainAndClose releases a rejected attempt while keeping the connection
// reusable when the remaining error body is small.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxRejectionBodyBytes))
	_ = resp.Body.Close()
}
