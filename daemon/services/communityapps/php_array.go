package communityapps

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// The verified CA build writes PHP's serialized arrays to the cache it calls
// templates_new.json. Accept the scalar and array forms used by feed data.
const maxArrayDepth = 32

var errCacheEncoding = errors.New("unrecognized Community Applications cache encoding")

type phpField struct {
	key   string
	value any
}

type phpArray []phpField

func (a phpArray) get(key string) any {
	for _, f := range a {
		if f.key == key {
			return f.value
		}
	}
	return nil
}

func (a phpArray) text(key string) string {
	switch v := a.get(key).(type) {
	case string:
		return strings.Clone(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func (a phpArray) flag(key string) bool {
	switch v := a.get(key).(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func (a phpArray) array(key string) phpArray {
	v, _ := a.get(key).(phpArray)
	return v
}

type phpDecoder struct {
	input string
}

func (d *phpDecoder) take(prefix string) bool {
	if !strings.HasPrefix(d.input, prefix) {
		return false
	}
	d.input = d.input[len(prefix):]
	return true
}

func (d *phpDecoder) until(delimiter string) (string, error) {
	value, rest, found := strings.Cut(d.input, delimiter)
	if !found {
		return "", errCacheEncoding
	}
	d.input = rest
	return value, nil
}

func (d *phpDecoder) sizeBefore(delimiter string) (int, error) {
	value, err := d.until(delimiter)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > len(d.input) {
		return 0, errCacheEncoding
	}
	return n, nil
}

func (d *phpDecoder) arraySize() (int, error) {
	if !d.take("a:") {
		return 0, errCacheEncoding
	}
	count, err := d.sizeBefore(":{")
	// Even the shortest key/value pair takes six bytes. Validate the
	// declared size before using it to allocate memory.
	if err != nil || count > len(d.input)/6 {
		return 0, errCacheEncoding
	}
	return count, nil
}

func (d *phpDecoder) field(depth int) (phpField, error) {
	key, err := d.scalar()
	if err != nil {
		return phpField{}, err
	}
	var name string
	switch key := key.(type) {
	case string:
		name = key
	case int64:
		name = strconv.FormatInt(key, 10)
	default:
		return phpField{}, errCacheEncoding
	}
	value, err := d.value(depth)
	return phpField{key: name, value: value}, err
}

func (d *phpDecoder) value(depth int) (any, error) {
	if depth > maxArrayDepth {
		return nil, errCacheEncoding
	}
	if !strings.HasPrefix(d.input, "a:") {
		return d.scalar()
	}
	count, err := d.arraySize()
	if err != nil || count > 8192 {
		return nil, errCacheEncoding
	}
	fields := make(phpArray, 0, min(count, 64))
	for range count {
		field, err := d.field(depth + 1)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	if !d.take("}") {
		return nil, errCacheEncoding
	}
	return fields, nil
}

func (d *phpDecoder) scalar() (any, error) {
	switch {
	case d.take("N;"):
		return nil, nil
	case d.take("b:0;"):
		return false, nil
	case d.take("b:1;"):
		return true, nil
	case d.take("s:"):
		size, err := d.sizeBefore(`:"`)
		if err != nil {
			return nil, err
		}
		value := d.input[:size]
		d.input = d.input[size:]
		if !d.take(`";`) {
			return nil, errCacheEncoding
		}
		return value, nil
	case d.take("i:"):
		value, err := d.until(";")
		if err != nil {
			return nil, err
		}
		return strconv.ParseInt(value, 10, 64)
	case d.take("d:"):
		value, err := d.until(";")
		if err != nil {
			return nil, err
		}
		return strconv.ParseFloat(value, 64)
	default:
		return nil, errCacheEncoding
	}
}

func decodeCAEntries(ctx context.Context, input []byte) ([]catalogEntry, error) {
	decoder := phpDecoder{input: string(input)}
	count, err := decoder.arraySize()
	if err != nil || count > 50000 {
		return nil, errCacheEncoding
	}
	entries := make([]catalogEntry, 0, min(count, 6000))
	for range count {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		field, err := decoder.field(1)
		if err != nil {
			return nil, err
		}
		row, ok := field.value.(phpArray)
		if !ok {
			return nil, errCacheEncoding
		}
		entry, err := readEntry(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if !decoder.take("}") || decoder.input != "" {
		return nil, errCacheEncoding
	}
	return entries, nil
}
