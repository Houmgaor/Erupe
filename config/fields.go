package config

import (
	"reflect"
	"strings"
)

// FieldKind is how the config editor presents and validates an option.
type FieldKind string

const (
	KindBool   FieldKind = "bool"
	KindInt    FieldKind = "int"    // integer, bounded by its Go type (and Min/Max)
	KindNumber FieldKind = "number" // floating point
	KindString FieldKind = "string"
	KindList   FieldKind = "list" // list of scalars, see Field.Elem
	KindJSON   FieldKind = "json" // anything structured: edited as JSON
)

// Field describes one editable option of config.json.
type Field struct {
	Key         string    `json:"key"`   // dotted path, e.g. "GameplayOptions.MaximumRP"
	Group       string    `json:"group"` // top-level section, "General" for top-level scalars
	Kind        FieldKind `json:"kind"`
	Elem        FieldKind `json:"elem,omitempty"` // element kind of a KindList
	Min         *float64  `json:"min,omitempty"`
	Max         *float64  `json:"max,omitempty"`
	Options     []string  `json:"options,omitempty"` // allowed values of a KindString
	Description string    `json:"description"`
	Secret      bool      `json:"secret,omitempty"`  // value never sent back to the browser
	Warning     string    `json:"warning,omitempty"` // shown next to options that can break a server

	index []int        // reflect field index path into Config
	typ   reflect.Type // Go type of the option
}

// Type is the Go type of the option.
func (f Field) Type() reflect.Type { return f.typ }

// ValueOf returns the option's value in c, in a form that encodes to the
// JSON config.json expects: byte slices become lists of numbers rather
// than base64 strings.
func (f Field) ValueOf(c *Config) interface{} {
	return plain(reflect.ValueOf(c).Elem().FieldByIndex(f.index))
}

func plain(v reflect.Value) interface{} {
	if v.Kind() == reflect.Slice && !v.IsNil() {
		switch v.Type().Elem().Kind() {
		case reflect.Uint8:
			out := make([]int, v.Len())
			for i := range out {
				out[i] = int(v.Index(i).Uint())
			}
			return out
		case reflect.Slice:
			out := make([]interface{}, v.Len())
			for i := range out {
				out[i] = plain(v.Index(i))
			}
			return out
		}
	}
	return v.Interface()
}

// Fields lists every option of Config in declaration order.
func Fields() []Field {
	var out []Field
	walkFields(reflect.TypeOf(Config{}), nil, "", "", &out)
	return out
}

// FieldByKey finds an option by its dotted path, ignoring case as Viper does.
func FieldByKey(key string) (Field, bool) {
	for _, f := range Fields() {
		if strings.EqualFold(f.Key, key) {
			return f, true
		}
	}
	return Field{}, false
}

func walkFields(t reflect.Type, index []int, prefix, group string, out *[]Field) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name := sf.Name
		if tag := sf.Tag.Get("mapstructure"); tag != "" {
			if tag == "-" {
				continue
			}
			name = strings.Split(tag, ",")[0]
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		idx := append(append([]int{}, index...), i)
		g := group
		if sf.Type.Kind() == reflect.Struct {
			if g == "" {
				g = name
			}
			walkFields(sf.Type, idx, key, g, out)
			continue
		}
		f := Field{Key: key, index: idx, typ: sf.Type}
		f.Kind, f.Elem = kindOf(sf.Type)
		switch {
		case g != "":
		case f.Kind == KindJSON:
			g = name // a top-level list of structures is a section of its own
		default:
			g = "General"
		}
		f.Group = g
		if f.Kind == KindInt {
			f.Min, f.Max = intRange(sf.Type)
		}
		meta := fieldMeta[key]
		f.Description = meta.desc
		f.Secret = meta.secret
		f.Warning = meta.warning
		f.Options = meta.options
		if meta.min != nil {
			f.Min = meta.min
		}
		if meta.max != nil {
			f.Max = meta.max
		}
		*out = append(*out, f)
	}
}

func kindOf(t reflect.Type) (FieldKind, FieldKind) {
	switch t.Kind() {
	case reflect.Bool:
		return KindBool, ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return KindInt, ""
	case reflect.Float32, reflect.Float64:
		return KindNumber, ""
	case reflect.String:
		return KindString, ""
	case reflect.Slice:
		if elem, _ := kindOf(t.Elem()); elem == KindInt || elem == KindString || elem == KindNumber {
			return KindList, elem
		}
	}
	return KindJSON, ""
}

func intRange(t reflect.Type) (*float64, *float64) {
	bits := t.Bits()
	var lo, hi float64
	switch t.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		lo, hi = 0, float64(uint64(1)<<bits-1)
	default:
		lo, hi = -float64(uint64(1)<<(bits-1)), float64(uint64(1)<<(bits-1)-1)
	}
	return &lo, &hi
}

type meta struct {
	desc     string
	secret   bool
	warning  string
	options  []string
	min, max *float64
}

func bound(v float64) *float64 { return &v }
