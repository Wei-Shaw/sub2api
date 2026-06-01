package driver

import (
	"reflect"
	"strings"
	"time"
)

// ColumnTypeDatabaseTypeName returns the PostgreSQL type name for the column
// at the given index (e.g. "int8", "varchar", "timestamptz"). Satisfies
// the optional driver.RowsColumnTypeDatabaseTypeName interface so that
// (*sql.Rows).ColumnTypes() can populate ColumnType.DatabaseTypeName().
func (r *grpcRows) ColumnTypeDatabaseTypeName(index int) string {
	if index < 0 || index >= len(r.colTypes) {
		return ""
	}
	return r.colTypes[index]
}

// ColumnTypeScanType returns the Go reflect.Type that is suitable for
// scanning the column at the given index. Satisfies the optional
// driver.RowsColumnTypeScanType interface used by ent's unknown-column
// branch to infer Go types from database column metadata.
func (r *grpcRows) ColumnTypeScanType(index int) reflect.Type {
	return pgTypeToGoType(r.ColumnTypeDatabaseTypeName(index))
}

// pgTypeToGoType maps a PostgreSQL type name (lowercase) to the Go
// reflect.Type that database/sql would use for scanning. The mapping
// covers the common PG types; unknown types fall back to any (interface{}).
func pgTypeToGoType(name string) reflect.Type {
	switch strings.ToLower(name) {
	case "int2", "int4", "int8", "serial", "bigserial", "smallserial":
		return reflect.TypeOf(int64(0))
	case "float4", "float8", "numeric", "decimal":
		return reflect.TypeOf(float64(0))
	case "bool":
		return reflect.TypeOf(false)
	case "varchar", "text", "char", "bpchar", "name", "uuid",
		"interval", "inet", "cidr", "money":
		return reflect.TypeOf("")
	case "bytea":
		return reflect.TypeOf([]byte{})
	case "timestamp", "timestamptz", "date", "time", "timetz":
		return reflect.TypeOf(time.Time{})
	case "json", "jsonb":
		return reflect.TypeOf([]byte{})
	case "_int4", "_int8":
		return reflect.TypeOf([]int64{})
	case "_text", "_varchar":
		return reflect.TypeOf([]string{})
	default:
		return reflect.TypeOf((*any)(nil)).Elem()
	}
}
