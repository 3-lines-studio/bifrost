package env

import (
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestAppModeDetectionDevVsProd(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		wantDev  bool
		wantProd bool
	}{
		{
			name:     "dev mode with 1",
			envValue: "1",
			wantDev:  true,
			wantProd: false,
		},
		{
			name:     "prod mode with empty",
			envValue: "",
			wantDev:  false,
			wantProd: true,
		},
		{
			name:     "prod mode with 0",
			envValue: "0",
			wantDev:  false,
			wantProd: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BIFROST_DEV", tt.envValue)
			t.Setenv("BIFROST_EXPORT", "")

			mode := DetectAppMode()
			isDev := mode == core.ModeDev
			isProd := mode == core.ModeProd

			if isDev != tt.wantDev {
				t.Errorf("IsDev() = %v, want %v", isDev, tt.wantDev)
			}
			if isProd != tt.wantProd {
				t.Errorf("mode == ModeProd = %v, want %v", isProd, tt.wantProd)
			}
		})
	}
}
