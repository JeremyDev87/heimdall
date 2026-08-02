package canonjson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

func Marshal(value any) ([]byte, error)       { return marshal(value, false) }
func MarshalIndent(value any) ([]byte, error) { return marshal(value, true) }

func Digest(value any) (string, error) {
	data, err := Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func marshal(value any, pretty bool) ([]byte, error) {
	normalized, err := normalize(value)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := writeValue(&out, normalized, pretty, 0); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func normalize(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func writeValue(out *bytes.Buffer, value any, pretty bool, depth int) error {
	switch typed := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if typed {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		quoted, err := quote(typed)
		if err != nil {
			return err
		}
		out.Write(quoted)
	case json.Number:
		out.WriteString(typed.String())
	case []any:
		return writeArray(out, typed, pretty, depth)
	case map[string]any:
		return writeObject(out, typed, pretty, depth)
	default:
		return fmt.Errorf("unsupported canonical JSON value %T", value)
	}
	return nil
}

func writeArray(out *bytes.Buffer, values []any, pretty bool, depth int) error {
	if len(values) == 0 {
		out.WriteString("[]")
		return nil
	}
	out.WriteByte('[')
	for index, value := range values {
		if index > 0 {
			out.WriteByte(',')
		}
		if pretty {
			newlineIndent(out, depth+1)
		}
		if err := writeValue(out, value, pretty, depth+1); err != nil {
			return err
		}
	}
	if pretty {
		newlineIndent(out, depth)
	}
	out.WriteByte(']')
	return nil
}

func writeObject(out *bytes.Buffer, values map[string]any, pretty bool, depth int) error {
	if len(values) == 0 {
		out.WriteString("{}")
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			out.WriteByte(',')
		}
		if pretty {
			newlineIndent(out, depth+1)
		}
		quoted, err := quote(key)
		if err != nil {
			return err
		}
		out.Write(quoted)
		if pretty {
			out.WriteString(": ")
		} else {
			out.WriteByte(':')
		}
		if err := writeValue(out, values[key], pretty, depth+1); err != nil {
			return err
		}
	}
	if pretty {
		newlineIndent(out, depth)
	}
	out.WriteByte('}')
	return nil
}

func newlineIndent(out *bytes.Buffer, depth int) {
	out.WriteByte('\n')
	out.WriteString(strings.Repeat("  ", depth))
}

func quote(value string) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	quoted := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	quoted = bytes.ReplaceAll(quoted, []byte(`\u2028`), []byte("\u2028"))
	quoted = bytes.ReplaceAll(quoted, []byte(`\u2029`), []byte("\u2029"))
	return quoted, nil
}

func WriteLine(writer io.Writer, value any) error {
	data, err := Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = writer.Write(data)
	return err
}
