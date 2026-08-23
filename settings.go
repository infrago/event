package event

import (
	"strconv"
	"strings"
	"time"

	. "github.com/infrago/base"
)

func DurationSetting(setting Map, key string, def time.Duration) time.Duration {
	switch v := setting[key].(type) {
	case time.Duration:
		if v >= 0 {
			return v
		}
	case int:
		if v >= 0 {
			return time.Duration(v) * time.Second
		}
	case int64:
		if v >= 0 {
			return time.Duration(v) * time.Second
		}
	case float64:
		if v >= 0 {
			return time.Duration(v * float64(time.Second))
		}
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return def
		}
		if d, err := time.ParseDuration(text); err == nil && d >= 0 {
			return d
		}
		if n, err := strconv.Atoi(text); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return def
}

func IntSetting(setting Map, key string, def int) int {
	switch v := setting[key].(type) {
	case int:
		if v >= 0 {
			return v
		}
	case int64:
		if v >= 0 {
			return int(v)
		}
	case float64:
		if v >= 0 {
			return int(v)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

func Int64Setting(setting Map, key string, def int64) int64 {
	n := IntSetting(setting, key, int(def))
	if n <= 0 {
		return def
	}
	return int64(n)
}

func StringSetting(setting Map, key, def string) string {
	if v, ok := setting[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return def
}
