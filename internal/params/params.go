package params

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
)

// GetString returns a string parameter.
func GetString(params *structpb.Struct, key string) (string, error) {
	if params == nil || params.Fields == nil {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	f, ok := params.Fields[key]
	if !ok || f == nil {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	return f.GetStringValue(), nil
}

// GetStringOrDefault returns a string or default.
func GetStringOrDefault(params *structpb.Struct, key, def string) string {
	if params == nil || params.Fields == nil {
		return def
	}
	f, ok := params.Fields[key]
	if !ok || f == nil {
		return def
	}
	s := f.GetStringValue()
	if s == "" {
		return def
	}
	return s
}

// GetMap returns a nested map/struct.
func GetMap(params *structpb.Struct, key string) *structpb.Struct {
	if params == nil || params.Fields == nil {
		return nil
	}
	f, ok := params.Fields[key]
	if !ok || f == nil {
		return nil
	}
	return f.GetStructValue()
}

// GetStringSlice returns a string array.
func GetStringSlice(params *structpb.Struct, key string) []string {
	if params == nil || params.Fields == nil {
		return nil
	}
	f, ok := params.Fields[key]
	if !ok || f == nil {
		return nil
	}
	list := f.GetListValue()
	if list == nil {
		return nil
	}
	out := make([]string, 0, len(list.Values))
	for _, v := range list.Values {
		out = append(out, v.GetStringValue())
	}
	return out
}

// GetBool returns a boolean parameter.
func GetBool(params *structpb.Struct, key string) bool {
	if params == nil || params.Fields == nil {
		return false
	}
	f, ok := params.Fields[key]
	if !ok || f == nil {
		return false
	}
	return f.GetBoolValue()
}

// GetInt returns an integer parameter.
func GetInt(params *structpb.Struct, key string) int {
	if params == nil || params.Fields == nil {
		return 0
	}
	f, ok := params.Fields[key]
	if !ok || f == nil {
		return 0
	}
	return int(f.GetNumberValue())
}

// GetDuration parses a duration string like "5m", "1h".
func GetDuration(params *structpb.Struct, key string) (time.Duration, error) {
	s := GetStringOrDefault(params, key, "")
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}
