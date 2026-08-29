package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUploadSessionRefreshStatus(t *testing.T) {
	tests := []struct {
		name   string
		assets []UploadAsset
		want   UploadSessionStatus
	}{
		{
			name:   "failed primary fails session",
			assets: []UploadAsset{{Role: AssetRolePrimaryH264, Status: AssetStatusFailed}},
			want:   UploadSessionStatusFailed,
		},
		{
			name:   "canceled primary fails session",
			assets: []UploadAsset{{Role: AssetRolePrimaryH264, Status: AssetStatusCanceled}},
			want:   UploadSessionStatusFailed,
		},
		{
			name: "terminal optional assets complete session",
			assets: []UploadAsset{
				{Role: AssetRolePrimaryH264, Status: AssetStatusAvailable},
				{Role: AssetRoleOriginal, Status: AssetStatusFailed},
			},
			want: UploadSessionStatusCompleted,
		},
		{
			name:   "unfinished primary keeps session active",
			assets: []UploadAsset{{Role: AssetRolePrimaryH264, Status: AssetStatusUploading}},
			want:   UploadSessionStatusActive,
		},
		{
			name: "finalizing original keeps session active",
			assets: []UploadAsset{
				{Role: AssetRolePrimaryH264, Status: AssetStatusAvailable},
				{Role: AssetRoleOriginal, Status: AssetStatusFinalizing},
			},
			want: UploadSessionStatusActive,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := UploadSession{Status: UploadSessionStatusActive, Assets: tt.assets}

			session.RefreshStatus()

			assert.Equal(t, tt.want, session.Status)
		})
	}
}
