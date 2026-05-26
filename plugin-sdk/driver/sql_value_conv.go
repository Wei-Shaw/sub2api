package driver

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// ---------------------------------------------------------------------------
// Value conversion helpers
// ---------------------------------------------------------------------------

// namedValuesToProto converts driver.NamedValue args to their protobuf
// representation for transmission over gRPC.
func namedValuesToProto(args []driver.NamedValue) ([]*pb.SQLValue, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make([]*pb.SQLValue, 0, len(args))
	for _, a := range args {
		v, err := goValueToProto(a.Value)
		if err != nil {
			return nil, fmt.Errorf("plugin-sdk: arg %d: %w", a.Ordinal, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// valuesToNamed wraps positional driver.Value args as driver.NamedValue with
// 1-based ordinals so they can be forwarded to namedValuesToProto.
func valuesToNamed(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, v := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return out
}

// goValueToProto maps a single Go value to a protobuf SQLValue.
func goValueToProto(v interface{}) (*pb.SQLValue, error) {
	if v == nil {
		return &pb.SQLValue{Value: &pb.SQLValue_Null{Null: true}}, nil
	}
	switch x := v.(type) {
	case bool:
		return &pb.SQLValue{Value: &pb.SQLValue_BoolValue{BoolValue: x}}, nil
	case int:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case int8:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case int16:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case int32:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case int64:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: x}}, nil
	case uint8:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case uint16:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case uint32:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case uint64:
		if x > math.MaxInt64 {
			return nil, fmt.Errorf("plugin-sdk: uint64 value %d overflows int64", x)
		}
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case float32:
		return &pb.SQLValue{Value: &pb.SQLValue_FloatValue{FloatValue: float64(x)}}, nil
	case float64:
		return &pb.SQLValue{Value: &pb.SQLValue_FloatValue{FloatValue: x}}, nil
	case string:
		return &pb.SQLValue{Value: &pb.SQLValue_StringValue{StringValue: x}}, nil
	case []byte:
		return &pb.SQLValue{Value: &pb.SQLValue_BytesValue{BytesValue: x}}, nil
	case time.Time:
		return &pb.SQLValue{Value: &pb.SQLValue_TimeValue{TimeValue: x.Format(time.RFC3339Nano)}}, nil
	default:
		return nil, fmt.Errorf("unsupported argument type %T", v)
	}
}

// sqlValueToDriver converts a protobuf SQLValue back to a driver.Value.
func sqlValueToDriver(v *pb.SQLValue) driver.Value {
	if v == nil {
		return nil
	}
	switch x := v.GetValue().(type) {
	case *pb.SQLValue_Null:
		return nil
	case *pb.SQLValue_BoolValue:
		return x.BoolValue
	case *pb.SQLValue_IntValue:
		return x.IntValue
	case *pb.SQLValue_FloatValue:
		return x.FloatValue
	case *pb.SQLValue_StringValue:
		return x.StringValue
	case *pb.SQLValue_BytesValue:
		out := make([]byte, len(x.BytesValue))
		copy(out, x.BytesValue)
		return out
	case *pb.SQLValue_TimeValue:
		t, err := time.Parse(time.RFC3339Nano, x.TimeValue)
		if err != nil {
			return x.TimeValue // parse failure: degrade to string
		}
		return t
	default:
		return nil
	}
}

// isolationLevelString maps a sql.IsolationLevel to its PostgreSQL wire
// representation. Returns an empty string for LevelDefault (use server
// default) and an error for levels not supported by PostgreSQL.
func isolationLevelString(level sql.IsolationLevel) (string, error) {
	switch level {
	case sql.LevelDefault:
		return "", nil
	case sql.LevelReadUncommitted:
		return "read_uncommitted", nil
	case sql.LevelReadCommitted:
		return "read_committed", nil
	case sql.LevelRepeatableRead:
		return "repeatable_read", nil
	case sql.LevelSerializable:
		return "serializable", nil
	default:
		return "", fmt.Errorf("plugin-sdk: unsupported isolation level: %v", level)
	}
}
