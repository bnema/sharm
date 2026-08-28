package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUploadSessionRemainsActiveWhileOptionalOriginalFinalizes(t *testing.T) {
	session := UploadSession{
		Status: UploadSessionStatusActive,
		Assets: []UploadAsset{
			{Role: AssetRolePrimaryH264, Status: AssetStatusAvailable},
			{Role: AssetRoleOriginal, Status: AssetStatusFinalizing},
		},
	}

	session.RefreshStatus()

	assert.Equal(t, UploadSessionStatusActive, session.Status)
}
