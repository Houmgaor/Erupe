package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Editing config.json in place. Viper matches keys case-insensitively and
// ignores the ones it does not know, so a typo silently leaves an option at
// its default: UnknownKeys reports those, and SetKeys changes options while
// keeping the operator's key spelling, key order and every other entry.

// deprecatedKeys maps retired option names, lower-cased, to the option that
// replaced them. They are ignored like any unknown key; knowing the
// replacement lets the warning say what to use instead.
var deprecatedKeys = map[string]string{
	"disablesoftcrash": "DisableShutdownCountdown",
}

// Replacement returns the option that replaced a deprecated key, or "".
func Replacement(key string) string {
	return deprecatedKeys[strings.ToLower(key)]
}

// member is one key of a JSON object, with its value left undecoded.
type member struct {
	key string
	val json.RawMessage
}

// parseObject reads a JSON object keeping its keys in order.
func parseObject(data []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("config file is not a JSON object")
	}
	var out []member
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		out = append(out, member{key: tok.(string), val: raw})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return out, nil
}

func encodeObject(members []member) []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := marshalNoEscape(m.key)
		b.Write(k)
		b.WriteByte(':')
		b.Write(m.val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// marshalNoEscape encodes v without turning < and > into <: login
// notices are MHFML, and an operator reading the file expects <BODY>.
func marshalNoEscape(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

func findMember(members []member, name string) int {
	for i, m := range members {
		if strings.EqualFold(m.key, name) {
			return i
		}
	}
	return -1
}

// SetKeys returns data with the options in set replaced (or added) and the
// options in unset removed, so they fall back to their default. Keys are
// dotted option paths; values must already be valid JSON. The result is
// indented with two spaces.
func SetKeys(data []byte, set map[string]json.RawMessage, unset []string) ([]byte, error) {
	root, err := parseObject(data)
	if err != nil {
		return nil, err
	}
	for key, val := range set {
		var compact bytes.Buffer
		if err := json.Compact(&compact, val); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		if root, err = setPath(root, strings.Split(key, "."), compact.Bytes()); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
	}
	for _, key := range unset {
		if root, err = setPath(root, strings.Split(key, "."), nil); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
	}
	var out bytes.Buffer
	if err := json.Indent(&out, encodeObject(root), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// setPath sets (val != nil) or removes (val == nil) path inside members.
func setPath(members []member, path []string, val json.RawMessage) ([]member, error) {
	i := findMember(members, path[0])
	if len(path) == 1 {
		switch {
		case val == nil && i >= 0:
			return append(members[:i], members[i+1:]...), nil
		case val == nil:
			return members, nil
		case i >= 0:
			members[i].val = val
			return members, nil
		default:
			return append(members, member{key: path[0], val: val}), nil
		}
	}
	var child []member
	if i >= 0 {
		var err error
		if child, err = parseObject(members[i].val); err != nil {
			return nil, fmt.Errorf("%s is not an object", members[i].key)
		}
	} else if val == nil {
		return members, nil
	}
	child, err := setPath(child, path[1:], val)
	if err != nil {
		return nil, err
	}
	if i >= 0 {
		members[i].val = encodeObject(child)
		return members, nil
	}
	return append(members, member{key: path[0], val: encodeObject(child)}), nil
}

// HasKey reports whether the file sets the option key itself (in any
// case), as opposed to leaving it at its default.
func HasKey(data []byte, key string) bool {
	members, err := parseObject(data)
	if err != nil {
		return false
	}
	path := strings.Split(key, ".")
	for n, name := range path {
		i := findMember(members, name)
		if i < 0 {
			return false
		}
		if n == len(path)-1 {
			return true
		}
		if members, err = parseObject(members[i].val); err != nil {
			return false
		}
	}
	return false
}

// UnknownKeys lists the keys of a config.json document that match no
// option, such as a misspelt name or an option put in the wrong section.
// Erupe ignores them, so the option they were meant for keeps its default.
func UnknownKeys(data []byte) ([]string, error) {
	var tree interface{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	var out []string
	unknownKeys(tree, reflect.TypeOf(Config{}), "", &out)
	sort.Strings(out)
	return out, nil
}

func unknownKeys(node interface{}, t reflect.Type, path string, out *[]string) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch n := node.(type) {
	case map[string]interface{}:
		if t.Kind() != reflect.Struct {
			return
		}
		for key, val := range n {
			sub := key
			if path != "" {
				sub = path + "." + key
			}
			ft, ok := structField(t, key)
			if !ok {
				*out = append(*out, sub)
				continue
			}
			unknownKeys(val, ft, sub, out)
		}
	case []interface{}:
		if t.Kind() != reflect.Slice {
			return
		}
		for i, val := range n {
			unknownKeys(val, t.Elem(), fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}

// structField finds the field of t that Viper would decode key into.
func structField(t reflect.Type, key string) (reflect.Type, bool) {
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
		if strings.EqualFold(name, key) {
			return sf.Type, true
		}
	}
	return nil, false
}

// ValidateValue checks that raw is a valid value for f: the right JSON
// type, within the Go type's range and the option's bounds, and one of
// its allowed values. Structured options must not carry unknown keys.
func ValidateValue(f Field, raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	ptr := reflect.New(f.typ)
	if err := dec.Decode(ptr.Interface()); err != nil {
		return fmt.Errorf("%s: invalid value: %w", f.Key, err)
	}
	if dec.More() {
		return fmt.Errorf("%s: invalid value", f.Key)
	}
	v := ptr.Elem()
	var n float64
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n = float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n = float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		n = v.Float()
	case reflect.String:
		if len(f.Options) > 0 {
			for _, o := range f.Options {
				if strings.EqualFold(o, v.String()) {
					return nil
				}
			}
			return fmt.Errorf("%s: must be one of %s", f.Key, strings.Join(f.Options, ", "))
		}
		return nil
	default:
		return nil
	}
	if f.Min != nil && n < *f.Min {
		return fmt.Errorf("%s: must be at least %v", f.Key, *f.Min)
	}
	if f.Max != nil && n > *f.Max {
		return fmt.Errorf("%s: must be at most %v", f.Key, *f.Max)
	}
	return nil
}
