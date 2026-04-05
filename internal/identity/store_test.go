package identity

import (
	"path/filepath"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/require"
)

func TestStorePersistsNamesAcrossInit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "player-identities.json")
	clientID := uuid.Must(uuid.NewV4())

	require.NoError(t, Init(path))
	require.NoError(t, SetName(clientID, "Persistent Player"))

	require.NoError(t, Init(path))

	name, ok := GetName(clientID)
	require.True(t, ok)
	require.Equal(t, "Persistent Player", name)
}
