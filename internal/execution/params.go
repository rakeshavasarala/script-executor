package execution

import (
	"time"

	"google.golang.org/protobuf/types/known/structpb"
)

func getString(m *structpb.Struct, key, def string) string {
	if m == nil || m.Fields == nil {
		return def
	}
	f, ok := m.Fields[key]
	if !ok || f == nil {
		return def
	}
	s := f.GetStringValue()
	if s == "" {
		return def
	}
	return s
}

func getMap(m *structpb.Struct, key string) *structpb.Struct {
	if m == nil || m.Fields == nil {
		return nil
	}
	f, ok := m.Fields[key]
	if !ok || f == nil {
		return nil
	}
	return f.GetStructValue()
}

func getList(m *structpb.Struct, key string) []*structpb.Value {
	if m == nil || m.Fields == nil {
		return nil
	}
	f, ok := m.Fields[key]
	if !ok || f == nil {
		return nil
	}
	l := f.GetListValue()
	if l == nil {
		return nil
	}
	return l.Values
}

func getStringSlice(m *structpb.Struct, key string) []string {
	list := getList(m, key)
	if list == nil {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, v.GetStringValue())
	}
	return out
}

func getBool(m *structpb.Struct, key string) bool {
	if m == nil || m.Fields == nil {
		return false
	}
	f, ok := m.Fields[key]
	if !ok || f == nil {
		return false
	}
	return f.GetBoolValue()
}

func getInt(m *structpb.Struct, key string) int {
	if m == nil || m.Fields == nil {
		return 0
	}
	f, ok := m.Fields[key]
	if !ok || f == nil {
		return 0
	}
	return int(f.GetNumberValue())
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

func parsePullPolicy(s string) string {
	switch s {
	case "Always", "IfNotPresent", "Never":
		return s
	}
	return "IfNotPresent"
}
