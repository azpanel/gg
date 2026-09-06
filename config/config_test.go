package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestCloseCLIErrConfig(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{name: "default", want: false},
		{name: "enabled", value: "true", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			if tt.value != nil {
				v.Set("closeclierr", tt.value)
			}
			if err := NewBinder(v).Bind(Params{}); err != nil {
				t.Fatal(err)
			}
			var params Params
			if err := v.Unmarshal(&params); err != nil {
				t.Fatal(err)
			}
			if params.CloseCLIErr != tt.want {
				t.Fatalf("CloseCLIErr = %v, want %v", params.CloseCLIErr, tt.want)
			}
		})
	}
}
