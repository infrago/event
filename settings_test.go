package event

import (
	"testing"
	"time"

	. "github.com/infrago/base"
)

func TestSettingHelpers(t *testing.T) {
	setting := Map{
		"duration": "250ms",
		"int":      "7",
		"int64":    float64(9),
		"string":   " value ",
	}
	if got := DurationSetting(setting, "duration", time.Second); got != 250*time.Millisecond {
		t.Fatalf("duration = %v", got)
	}
	if got := IntSetting(setting, "int", 1); got != 7 {
		t.Fatalf("int = %d", got)
	}
	if got := Int64Setting(setting, "int64", 1); got != 9 {
		t.Fatalf("int64 = %d", got)
	}
	if got := StringSetting(setting, "string", "x"); got != "value" {
		t.Fatalf("string = %q", got)
	}
}
