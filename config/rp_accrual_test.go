package config

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestRPAccrualConfig(t *testing.T) {
	for _, tt := range []struct {
		name, gameplay string
		normal, cafe   int
		invalid        bool
	}{
		{"omitted", "", 1800, 900, false},
		{"empty", `,"GameplayOptions":{}`, 1800, 900, false},
		{"old partial", `,"GameplayOptions":{"MaximumRP":1234}`, 1800, 900, false},
		{"normal only", `,"GameplayOptions":{"RPAccrualNormalSeconds":600}`, 600, 900, false},
		{"cafe only", `,"GameplayOptions":{"RPAccrualCafeSeconds":300}`, 1800, 300, false},
		{"both", `,"GameplayOptions":{"RPAccrualNormalSeconds":1,"RPAccrualCafeSeconds":2147483647}`, 1, 2147483647, false},
		{"zero", `,"GameplayOptions":{"RPAccrualNormalSeconds":0}`, 0, 0, true},
		{"negative", `,"GameplayOptions":{"RPAccrualCafeSeconds":-1}`, 0, 0, true},
		{"fraction", `,"GameplayOptions":{"RPAccrualCafeSeconds":1.5}`, 0, 0, true},
		{"string", `,"GameplayOptions":{"RPAccrualNormalSeconds":"600"}`, 0, 0, true},
		{"boolean", `,"GameplayOptions":{"RPAccrualNormalSeconds":true}`, 0, 0, true},
		{"too large", `,"GameplayOptions":{"RPAccrualCafeSeconds":2147483648}`, 0, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			old, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chdir(old); err != nil {
					t.Error(err)
				}
			})
			writeMinimalConfig(t, dir, fmt.Sprintf(`{"Host":"127.0.0.1"%s}`, tt.gameplay))
			got, err := LoadConfig()
			if tt.invalid {
				if err == nil || !strings.Contains(err.Error(), "GameplayOptions.RPAccrual") {
					t.Fatalf("expected field-specific error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.GameplayOptions.RPAccrualNormalSeconds != tt.normal || got.GameplayOptions.RPAccrualCafeSeconds != tt.cafe {
				t.Fatalf("intervals = %d/%d, want %d/%d", got.GameplayOptions.RPAccrualNormalSeconds, got.GameplayOptions.RPAccrualCafeSeconds, tt.normal, tt.cafe)
			}
			if tt.name == "old partial" && got.GameplayOptions.MaximumRP != 1234 {
				t.Fatal("existing option changed")
			}
		})
	}
}
