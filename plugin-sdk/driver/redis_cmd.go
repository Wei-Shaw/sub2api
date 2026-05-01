// Package driver — redis_cmd.go
//
// Cmder-style result types modelled after github.com/redis/go-redis/v9.
// They wrap a parsed DoReply and expose the familiar Result/Val/Err pattern
// so plugin authors can write code that looks identical to native go-redis
// without us depending on go-redis from the SDK side.
//
// The hierarchy is intentionally narrow: only the cmd types that Cmdable
// methods actually return. Adding more is a matter of accepting a new
// DoReply shape.

package driver

import (
	"errors"
	"strconv"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// reply kind constants — must match the strings the core SDK server sets on
// DoReply.kind. Centralising them avoids typo bugs.
const (
	replyKindString = "string"
	replyKindInt    = "int"
	replyKindFloat  = "float"
	replyKindArray  = "array"
	replyKindNil    = "nil"
	replyKindStatus = "status"
	replyKindError  = "error"
)

// baseCmd captures the common fields of every Cmder result. It is embedded
// by the typed Cmd structs so they share Err()/SetVal()/setErr() semantics.
type baseCmd struct {
	err error
}

// Err returns the most recent error the command observed. It is the first
// thing plugin authors check, mirroring go-redis.
func (c *baseCmd) Err() error { return c.err }

// setErr lets the SDK promote a transport-level failure onto the cmd.
func (c *baseCmd) setErr(err error) { c.err = err }

// StatusCmd is the result of a Redis command that returns a simple-string
// status reply (SET, PING, …). The Result is the raw status text.
type StatusCmd struct {
	baseCmd
	val string
}

// Val returns the status string ("" on error).
func (c *StatusCmd) Val() string { return c.val }

// Result returns (status, err) so callers can use the go-redis idiom.
func (c *StatusCmd) Result() (string, error) { return c.val, c.err }

// StringCmd is the typed result of a command returning a single bulk string.
// Missing keys surface as ErrRedisNil on Err(), matching go-redis.
type StringCmd struct {
	baseCmd
	val string
}

func (c *StringCmd) Val() string             { return c.val }
func (c *StringCmd) Result() (string, error) { return c.val, c.err }
func (c *StringCmd) Bytes() ([]byte, error)  { return []byte(c.val), c.err }
func (c *StringCmd) Int() (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	return strconv.Atoi(c.val)
}
func (c *StringCmd) Int64() (int64, error) {
	if c.err != nil {
		return 0, c.err
	}
	return strconv.ParseInt(c.val, 10, 64)
}
func (c *StringCmd) Float64() (float64, error) {
	if c.err != nil {
		return 0, c.err
	}
	return strconv.ParseFloat(c.val, 64)
}

// BoolCmd wraps integer 0/1 replies (EXISTS, SISMEMBER, …) as booleans.
type BoolCmd struct {
	baseCmd
	val bool
}

func (c *BoolCmd) Val() bool             { return c.val }
func (c *BoolCmd) Result() (bool, error) { return c.val, c.err }

// IntCmd wraps integer replies (INCR, DEL count, LLEN, …).
type IntCmd struct {
	baseCmd
	val int64
}

func (c *IntCmd) Val() int64              { return c.val }
func (c *IntCmd) Result() (int64, error)  { return c.val, c.err }
func (c *IntCmd) Uint64() (uint64, error) { return uint64(c.val), c.err }

// FloatCmd wraps RESP3 doubles or stringified floats (INCRBYFLOAT, ZSCORE).
type FloatCmd struct {
	baseCmd
	val float64
}

func (c *FloatCmd) Val() float64             { return c.val }
func (c *FloatCmd) Result() (float64, error) { return c.val, c.err }

// StringSliceCmd wraps array replies that the user expects as []string.
// Missing-element entries (RESP nils) become "" — matching go-redis.
type StringSliceCmd struct {
	baseCmd
	val []string
}

func (c *StringSliceCmd) Val() []string             { return c.val }
func (c *StringSliceCmd) Result() ([]string, error) { return c.val, c.err }

// StringStringMapCmd is the typed result for HGETALL.
type StringStringMapCmd struct {
	baseCmd
	val map[string]string
}

func (c *StringStringMapCmd) Val() map[string]string             { return c.val }
func (c *StringStringMapCmd) Result() (map[string]string, error) { return c.val, c.err }

// DurationCmd is the typed result for TTL/PTTL. The unit is set by the
// caller (Second / Millisecond) when constructing the cmd; -2/-1 sentinels
// are surfaced unchanged.
type DurationCmd struct {
	baseCmd
	val  int64
	unit time.Duration // multiplier applied at Val() time
}

// Val returns -2/-1 verbatim (key missing / no expiration) so callers can
// keep the go-redis sentinels. Positive values are multiplied by the unit
// the caller supplied.
func (c *DurationCmd) Val() time.Duration {
	if c.val < 0 {
		return time.Duration(c.val) // preserves -1 / -2
	}
	return time.Duration(c.val) * c.unit
}

func (c *DurationCmd) Result() (time.Duration, error) { return c.Val(), c.err }

// SliceCmd is a generic mixed-typed array reply (used by HMGET, MGET, …).
// Each element is the raw reply value: string, int64, nil, []byte slice, …
type SliceCmd struct {
	baseCmd
	val []any
}

func (c *SliceCmd) Val() []any             { return c.val }
func (c *SliceCmd) Result() ([]any, error) { return c.val, c.err }

// ============================================================
// DoReply → Cmd parsers
// ============================================================
//
// These helpers consume a (DoReply, error) pair as returned from a unary
// gRPC call and translate it into the matching Cmd type. They centralise
// the kind-string switch logic so each Cmdable method stays tiny.

// applyTransportErr promotes a non-nil transport (gRPC) error onto cmd.
// It returns true if cmd is "done" (no further parsing needed).
func applyTransportErr(cmd interface{ setErr(error) }, err error) bool {
	if err != nil {
		cmd.setErr(err)
		return true
	}
	return false
}

// applyReplyErr surfaces a server-side Redis error (from DoReply.error)
// as the cmd's Err() value. Returns true if the reply was an error.
func applyReplyErr(cmd interface{ setErr(error) }, reply *pb.DoReply) bool {
	if reply == nil {
		cmd.setErr(errors.New("redis: nil reply"))
		return true
	}
	if reply.GetKind() == replyKindError && reply.GetError() != "" {
		cmd.setErr(errors.New(reply.GetError()))
		return true
	}
	return false
}

// parseStatusCmd handles SET / PING / FLUSH / … style status replies.
// Bulk strings are also accepted (some commands return OK as a bulk string).
func parseStatusCmd(reply *pb.DoReply, err error) *StatusCmd {
	cmd := &StatusCmd{}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	switch reply.GetKind() {
	case replyKindStatus, replyKindString:
		cmd.val = string(reply.GetStringValue())
	case replyKindNil:
		// SET …NX with the condition unsatisfied returns nil; surface as
		// ErrRedisNil so callers can branch.
		cmd.setErr(ErrRedisNil)
	default:
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
	}
	return cmd
}

// parseStringCmd handles bulk-string responses. Nil → ErrRedisNil.
func parseStringCmd(reply *pb.DoReply, err error) *StringCmd {
	cmd := &StringCmd{}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	switch reply.GetKind() {
	case replyKindString, replyKindStatus:
		cmd.val = string(reply.GetStringValue())
	case replyKindInt:
		cmd.val = strconv.FormatInt(reply.GetIntValue(), 10)
	case replyKindFloat:
		cmd.val = strconv.FormatFloat(reply.GetFloatValue(), 'f', -1, 64)
	case replyKindNil:
		cmd.setErr(ErrRedisNil)
	default:
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
	}
	return cmd
}

// parseIntCmd handles integer replies. Nil → 0 + ErrRedisNil so the caller
// can distinguish "key missing" from "value is 0".
func parseIntCmd(reply *pb.DoReply, err error) *IntCmd {
	cmd := &IntCmd{}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	switch reply.GetKind() {
	case replyKindInt:
		cmd.val = reply.GetIntValue()
	case replyKindString:
		// Some servers return integer-as-string for SCAN cursor etc.
		v, perr := strconv.ParseInt(string(reply.GetStringValue()), 10, 64)
		if perr != nil {
			cmd.setErr(perr)
			return cmd
		}
		cmd.val = v
	case replyKindNil:
		cmd.setErr(ErrRedisNil)
	default:
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
	}
	return cmd
}

// parseBoolCmd handles 0/1 integer replies (EXISTS, SISMEMBER, EXPIRE).
func parseBoolCmd(reply *pb.DoReply, err error) *BoolCmd {
	cmd := &BoolCmd{}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	switch reply.GetKind() {
	case replyKindInt:
		cmd.val = reply.GetIntValue() != 0
	case replyKindStatus, replyKindString:
		// SET NX returns OK on success, nil on failure. Treat OK as true.
		cmd.val = string(reply.GetStringValue()) == "OK"
	case replyKindNil:
		cmd.val = false
	default:
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
	}
	return cmd
}

// parseFloatCmd handles INCRBYFLOAT / ZSCORE.
func parseFloatCmd(reply *pb.DoReply, err error) *FloatCmd {
	cmd := &FloatCmd{}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	switch reply.GetKind() {
	case replyKindFloat:
		cmd.val = reply.GetFloatValue()
	case replyKindString:
		v, perr := strconv.ParseFloat(string(reply.GetStringValue()), 64)
		if perr != nil {
			cmd.setErr(perr)
			return cmd
		}
		cmd.val = v
	case replyKindInt:
		cmd.val = float64(reply.GetIntValue())
	case replyKindNil:
		cmd.setErr(ErrRedisNil)
	default:
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
	}
	return cmd
}

// parseDurationCmd consumes an integer (TTL / PTTL) reply with the supplied
// unit (Second or Millisecond). Negative sentinels (-1 / -2) flow through.
func parseDurationCmd(reply *pb.DoReply, err error, unit time.Duration) *DurationCmd {
	cmd := &DurationCmd{unit: unit}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	if reply.GetKind() != replyKindInt {
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
		return cmd
	}
	cmd.val = reply.GetIntValue()
	return cmd
}

// parseStringSliceCmd handles array replies whose elements are bulk strings.
// Nil array → empty slice + ErrRedisNil (matches go-redis).
func parseStringSliceCmd(reply *pb.DoReply, err error) *StringSliceCmd {
	cmd := &StringSliceCmd{}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	switch reply.GetKind() {
	case replyKindArray:
		out := make([]string, 0, len(reply.GetArray()))
		for _, el := range reply.GetArray() {
			if el == nil || el.GetKind() == replyKindNil {
				out = append(out, "")
				continue
			}
			out = append(out, replyAsString(el))
		}
		cmd.val = out
	case replyKindNil:
		cmd.setErr(ErrRedisNil)
	default:
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
	}
	return cmd
}

// parseStringStringMapCmd handles HGETALL / CONFIG GET style replies. Redis
// returns an array of [k, v, k, v, …] which we collapse into a map.
func parseStringStringMapCmd(reply *pb.DoReply, err error) *StringStringMapCmd {
	cmd := &StringStringMapCmd{val: map[string]string{}}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	switch reply.GetKind() {
	case replyKindArray:
		arr := reply.GetArray()
		if len(arr)%2 != 0 {
			cmd.setErr(errors.New("redis: HGETALL returned odd number of elements"))
			return cmd
		}
		for i := 0; i < len(arr); i += 2 {
			cmd.val[replyAsString(arr[i])] = replyAsString(arr[i+1])
		}
	case replyKindNil:
		// keep empty map; absent hash legitimately yields {}.
	default:
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
	}
	return cmd
}

// parseSliceCmd handles mixed-element arrays (HMGET, MGET) where each
// position can be a bulk string or nil. Caller reads via .Val() as []any.
func parseSliceCmd(reply *pb.DoReply, err error) *SliceCmd {
	cmd := &SliceCmd{}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	switch reply.GetKind() {
	case replyKindArray:
		out := make([]any, 0, len(reply.GetArray()))
		for _, el := range reply.GetArray() {
			out = append(out, replyAsAny(el))
		}
		cmd.val = out
	case replyKindNil:
		cmd.setErr(ErrRedisNil)
	default:
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
	}
	return cmd
}

// replyAsString folds any scalar reply into its textual form. Used when
// parsing arrays where every element is expected to be a string but the
// server may return integers (LPOP-style edge cases).
func replyAsString(el *pb.DoReply) string {
	if el == nil {
		return ""
	}
	switch el.GetKind() {
	case replyKindString, replyKindStatus:
		return string(el.GetStringValue())
	case replyKindInt:
		return strconv.FormatInt(el.GetIntValue(), 10)
	case replyKindFloat:
		return strconv.FormatFloat(el.GetFloatValue(), 'f', -1, 64)
	default:
		return ""
	}
}

// replyAsAny is the polymorphic counterpart for SliceCmd.
func replyAsAny(el *pb.DoReply) any {
	if el == nil || el.GetKind() == replyKindNil {
		return nil
	}
	switch el.GetKind() {
	case replyKindString, replyKindStatus:
		return string(el.GetStringValue())
	case replyKindInt:
		return el.GetIntValue()
	case replyKindFloat:
		return el.GetFloatValue()
	case replyKindError:
		return errors.New(el.GetError())
	case replyKindArray:
		nested := make([]any, 0, len(el.GetArray()))
		for _, c := range el.GetArray() {
			nested = append(nested, replyAsAny(c))
		}
		return nested
	default:
		return nil
	}
}

func errUnexpectedReply(kind string) error {
	return errors.New("redis: unexpected reply kind: " + kind)
}
