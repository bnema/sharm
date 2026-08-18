package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreEnforcesSingleUserAndSessionVersion(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.CreateUser("admin", "hash"))
	assert.Error(t, store.CreateUser("second", "hash"))

	user, err := store.GetUser("admin")
	require.NoError(t, err)
	assert.Zero(t, user.SessionVersion)

	require.NoError(t, store.IncrementSessionVersion(user.ID))
	user, err = store.GetUser("admin")
	require.NoError(t, err)
	assert.Equal(t, int64(1), user.SessionVersion)
}
