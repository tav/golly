// Public Domain (-) 2012-2013 The Golly Authors.
// See the Golly UNLICENSE file for details.

package dynamodb

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	binaryField int = iota
	binarySetField
	boolField
	boolSetField
	intField
	intSetField
	int64Field
	int64SetField
	stringField
	stringSetField
	timeField
	uintField
	uintSetField
	uint64Field
	uint64SetField
)

var kindMap = [...]string{
	binaryField:    "B",
	binarySetField: "BS",
	boolField:      "N",
	boolSetField:   "NS",
	intField:       "N",
	intSetField:    "NS",
	int64Field:     "N",
	int64SetField:  "NS",
	stringField:    "S",
	stringSetField: "SS",
	timeField:      "N",
	uintField:      "N",
	uintSetField:   "NS",
	uint64Field:    "N",
	uint64SetField: "NS",
}

var (
	mutex    sync.RWMutex
	timeType = reflect.TypeOf(time.Time{})
	typeInfo = map[reflect.Type][]*fieldInfo{}
)

type fieldInfo struct {
	kind  int
	index int
	name  string
}

func Encode(v interface{}) string {
	buf := &bytes.Buffer{}
	encode(v, buf)
	return buf.String()
}

func encode(v interface{}, buf *bytes.Buffer) {

	if item, ok := v.(Item); ok {
		item.Encode(buf)
		return
	}

	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		panic("cannot encode invalid value")
	}
	rt := rv.Type()

	mutex.RLock()
	fields, present := typeInfo[rt]
	mutex.RUnlock()
	if !present {
		fields = compile(rt)
	}

	close := `{"`
	last := len(fields) - 1
	rv = rv.Elem()
	written := false

	for idx, field := range fields {

		dbKind := kindMap[field.kind]
		prefix := `"`
		suffix := `"`

		if len(dbKind) == 2 {
			prefix = "["
			suffix = "]"
		}

		fmt.Fprintf(buf, `%s%s":{"%s":%s`, close, field.name, dbKind, prefix)
		comma := ","
		if idx == last {
			comma = ""
		}

		close = fmt.Sprintf(`%s}%s"`, suffix, comma)
		written = true

		fv := rv.Field(field.index)

		switch field.kind {
		case binaryField:
			buf.WriteString(base64.StdEncoding.EncodeToString(fv.Interface().([]byte)))
		case binarySetField:
			elems := fv.Interface().([][]byte)
			for j, elem := range elems {
				buf.WriteByte('"')
				buf.WriteString(base64.StdEncoding.EncodeToString(elem))
				if j == len(elems)-1 {
					buf.WriteByte('"')
				} else {
					buf.WriteString(`",`)
				}
			}
		case boolField:
			if fv.Interface().(bool) {
				buf.WriteByte('1')
			} else {
				buf.WriteByte('0')
			}
		case boolSetField:
			elems := fv.Interface().([]bool)
			for j, elem := range elems {
				buf.WriteByte('"')
				if elem {
					buf.WriteByte('1')
				} else {
					buf.WriteByte('0')
				}
				if j == len(elems)-1 {
					buf.WriteByte('"')
				} else {
					buf.WriteString(`",`)
				}
			}
		case stringField:
			toJSON(fv.Interface().(string), buf)
		case stringSetField:
			elems := fv.Interface().([]string)
			for j, elem := range elems {
				buf.WriteByte('"')
				toJSON(elem, buf)
				if j == len(elems)-1 {
					buf.WriteByte('"')
				} else {
					buf.WriteString(`",`)
				}
			}
		case intField:
			buf.WriteString(strconv.FormatInt(int64(fv.Interface().(int)), 10))
		case intSetField:
			elems := fv.Interface().([]int)
			for j, elem := range elems {
				buf.WriteByte('"')
				buf.WriteString(strconv.FormatInt(int64(elem), 10))
				if j == len(elems)-1 {
					buf.WriteByte('"')
				} else {
					buf.WriteString(`",`)
				}
			}
		case int64Field:
			buf.WriteString(strconv.FormatInt(fv.Interface().(int64), 10))
		case int64SetField:
			elems := fv.Interface().([]int64)
			for j, elem := range elems {
				buf.WriteByte('"')
				buf.WriteString(strconv.FormatInt(elem, 10))
				if j == len(elems)-1 {
					buf.WriteByte('"')
				} else {
					buf.WriteString(`",`)
				}
			}
		case uintField:
			buf.WriteString(strconv.FormatUint(uint64(fv.Interface().(uint)), 10))
		case uintSetField:
			elems := fv.Interface().([]uint)
			for j, elem := range elems {
				buf.WriteByte('"')
				buf.WriteString(strconv.FormatUint(uint64(elem), 10))
				if j == len(elems)-1 {
					buf.WriteByte('"')
				} else {
					buf.WriteString(`",`)
				}
			}
		case uint64Field:
			buf.WriteString(strconv.FormatUint(fv.Interface().(uint64), 10))
		case uint64SetField:
			elems := fv.Interface().([]uint64)
			for j, elem := range elems {
				buf.WriteByte('"')
				buf.WriteString(strconv.FormatUint(elem, 10))
				if j == len(elems)-1 {
					buf.WriteByte('"')
				} else {
					buf.WriteString(`",`)
				}
			}
		case timeField:
			buf.WriteString(strconv.FormatInt(fv.Interface().(time.Time).UnixNano(), 10))
		}

	}

	if written {
		fmt.Fprintf(buf, "%s}", close[:len(close)-1])
	}

}

func Decode(v interface{}, data map[string]map[string]interface{}) {
	decode(v, data)
}

func decode(v interface{}, data map[string]map[string]interface{}) {

	if item, ok := v.(Item); ok {
		item.Decode(data)
		return
	}

	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		panic("cannot decode into invalid value")
	}
	rt := rv.Type()

	mutex.RLock()
	fields, present := typeInfo[rt]
	mutex.RUnlock()
	if !present {
		fields = compile(rt)
	}

	rv = rv.Elem()
	for _, field := range fields {
		switch field.kind {
		case binaryField, boolField, intField, int64Field, stringField, timeField, uintField, uint64Field:
			if val, ok := data[field.name][kindMap[field.kind]].(string); ok {
				switch field.kind {
				case binaryField:
					tmp, _ := base64.StdEncoding.DecodeString(val)
					rv.Field(field.index).SetBytes(tmp)
				case boolField:
					if val == "1" {
						rv.Field(field.index).SetBool(true)
					} else if val == "0" {
						rv.Field(field.index).SetBool(false)
					}
				case stringField:
					rv.Field(field.index).SetString(val)
				case intField:
					tmp, _ := strconv.ParseInt(val, 10, 64)
					rv.Field(field.index).SetInt(tmp)
				case int64Field:
					tmp, _ := strconv.ParseInt(val, 10, 64)
					rv.Field(field.index).SetInt(tmp)
				case uintField:
					tmp, _ := strconv.ParseUint(val, 10, 64)
					rv.Field(field.index).SetUint(tmp)
				case uint64Field:
					tmp, _ := strconv.ParseUint(val, 10, 64)
					rv.Field(field.index).SetUint(tmp)
				case timeField:
					tmp, _ := strconv.ParseInt(val, 10, 64)
					bin, err := time.Unix(0, tmp).MarshalBinary()
					if err == nil {
						if tobj, ok := rv.Field(field.index).Interface().(time.Time); ok {
							tobj.UnmarshalBinary(bin)
							rv.Field(field.index).Set(reflect.ValueOf(tobj))
						}
					}
				}
			}
		case binarySetField, boolSetField, intSetField, int64SetField, stringSetField, uintSetField, uint64SetField:
			if svals, ok := data[field.name][kindMap[field.kind]].([]interface{}); ok {
				vals := make([]string, len(svals))
				for k, val := range svals {
					vals[k] = val.(string)
				}
				switch field.kind {
				case binarySetField:
					nv := make([][]byte, len(vals))
					for j, val := range vals {
						tmp, _ := base64.StdEncoding.DecodeString(val)
						nv[j] = tmp
					}
					rv.Field(field.index).Set(reflect.ValueOf(nv))
				case boolSetField:
					nv := make([]bool, len(vals))
					for j, val := range vals {
						if val == "1" {
							nv[j] = true
						} else if val == "0" {
							nv[j] = false
						}
					}
					rv.Field(field.index).Set(reflect.ValueOf(nv))
				case stringSetField:
					rv.Field(field.index).Set(reflect.ValueOf(vals))
				case intSetField:
					nv := make([]int, len(vals))
					for j, val := range vals {
						tmp, _ := strconv.ParseInt(val, 10, 64)
						nv[j] = int(tmp)
					}
					rv.Field(field.index).Set(reflect.ValueOf(nv))
				case int64SetField:
					nv := make([]int64, len(vals))
					for j, val := range vals {
						tmp, _ := strconv.ParseInt(val, 10, 64)
						nv[j] = tmp
					}
					rv.Field(field.index).Set(reflect.ValueOf(nv))
				case uintSetField:
					nv := make([]uint, len(vals))
					for j, val := range vals {
						tmp, _ := strconv.ParseUint(val, 10, 64)
						nv[j] = uint(tmp)
					}
					rv.Field(field.index).Set(reflect.ValueOf(nv))
				case uint64SetField:
					nv := make([]uint64, len(vals))
					for j, val := range vals {
						tmp, _ := strconv.ParseUint(val, 10, 64)
						nv[j] = tmp
					}
					rv.Field(field.index).Set(reflect.ValueOf(nv))
				}
			}
		}
	}

}

func compile(it reflect.Type) []*fieldInfo {

	if it.Kind() != reflect.Ptr {
		panic("dynamodb: can only encode/decode pointers to struct types")
	}

	rt := it.Elem()
	if rt.Kind() != reflect.Struct {
		panic("dynamodb: can only encode/decode pointers to struct types")
	}

	fields := []*fieldInfo{}
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.Anonymous {
			continue
		}
		name := ""
		if tag := field.Tag.Get("ddb"); tag != "" {
			if tag == "-" {
				continue
			}
			name = tag
		}
		if name == "" {
			name = field.Name
			rune, _ := utf8.DecodeRuneInString(name)
			if !unicode.IsUpper(rune) {
				continue
			}
		}
		kind := -1
		switch field.Type.Kind() {
		case reflect.String:
			kind = stringField
		case reflect.Slice:
			switch field.Type.Elem().Kind() {
			case reflect.Uint8:
				kind = binaryField
			case reflect.String:
				kind = stringSetField
			case reflect.Int:
				kind = intSetField
			case reflect.Int64:
				kind = int64SetField
			case reflect.Slice:
				if field.Type.Elem().Elem().Kind() == reflect.Uint8 {
					kind = binarySetField
				}
			case reflect.Uint:
				kind = uintSetField
			case reflect.Uint64:
				kind = uint64SetField
			case reflect.Bool:
				kind = boolSetField
			}
		case reflect.Int:
			kind = intField
		case reflect.Int64:
			kind = int64Field
		case reflect.Struct:
			if field.Type == timeType {
				kind = timeField
			}
		case reflect.Uint:
			kind = uintField
		case reflect.Uint64:
			kind = uint64Field
		case reflect.Bool:
			kind = boolField
		}
		if kind == -1 {
			panic("dynamodb: unsupported field type: " + field.Type.Elem().Kind().String())
		}
		fields = append(fields, &fieldInfo{
			kind:  kind,
			index: i,
			name:  name,
		})

	}

	mutex.Lock()
	typeInfo[it] = fields
	mutex.Unlock()

	return fields

}

// Adapted from the encoding/json package in the standard
// library.
const hex = "0123456789abcdef"

func toJSON(s string, buf *bytes.Buffer) {
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if 0x20 <= b && b != '\\' && b != '"' && b != '<' && b != '>' && b != '&' {
				i++
				continue
			}
			if start < i {
				buf.WriteString(s[start:i])
			}
			switch b {
			case '\\', '"':
				buf.WriteByte('\\')
				buf.WriteByte(b)
			case '\n':
				buf.WriteByte('\\')
				buf.WriteByte('n')
			case '\r':
				buf.WriteByte('\\')
				buf.WriteByte('r')
			default:
				buf.WriteString(`\u00`)
				buf.WriteByte(hex[b>>4])
				buf.WriteByte(hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				buf.WriteString(s[start:i])
			}
			buf.WriteString(`\ufffd`)
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		buf.WriteString(s[start:])
	}
}
