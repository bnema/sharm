package jsonfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	t.Run("creates store successfully", func(t *testing.T) {
		tempDir := t.TempDir()
		store, err := NewStore(tempDir)

		assert.NoError(t, err)
		assert.NotNil(t, store)
		assert.NotNil(t, store.media)
	})

	t.Run("loads existing data from file", func(t *testing.T) {
		tempDir := t.TempDir()
		mediaPath := filepath.Join(tempDir, "media.json")

		media := []*domain.Media{
			{ID: "test1", OriginalName: "video1.mp4"},
			{ID: "test2", OriginalName: "video2.mp4"},
		}
		data, _ := json.MarshalIndent(media, "", "  ")
		require.NoError(t, os.WriteFile(mediaPath, data, 0600))

		store, err := NewStore(tempDir)

		assert.NoError(t, err)
		assert.Len(t, store.media, 2)
		assert.Equal(t, "video1.mp4", store.media["test1"].OriginalName)
		assert.Equal(t, "video2.mp4", store.media["test2"].OriginalName)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		mediaPath := filepath.Join(tempDir, "media.json")

		require.NoError(t, os.WriteFile(mediaPath, []byte("invalid json"), 0600))

		store, err := NewStore(tempDir)

		assert.Error(t, err)
		assert.Nil(t, store)
	})

	t.Run("creates empty store if file doesn't exist", func(t *testing.T) {
		tempDir := t.TempDir()

		store, err := NewStore(tempDir)

		assert.NoError(t, err)
		assert.Empty(t, store.media)
		_, err = os.Stat(filepath.Join(tempDir, "media.json"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("handles empty JSON file", func(t *testing.T) {
		tempDir := t.TempDir()
		mediaPath := filepath.Join(tempDir, "media.json")

		require.NoError(t, os.WriteFile(mediaPath, []byte(""), 0600))

		store, err := NewStore(tempDir)

		assert.NoError(t, err)
		assert.Empty(t, store.media)
	})
}

func TestStoreSave(t *testing.T) {
	t.Run("saves new media", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)

		err := store.Save(media)

		assert.NoError(t, err)
		assert.Contains(t, store.media, media.ID)
	})

	t.Run("updates existing media", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
		require.NoError(t, store.Save(media))

		media.MarkAsDone("/converted.mp4", domain.CodecH264, 1920, 1080, "/thumb.jpg", 1024000)
		err := store.Save(media)

		assert.NoError(t, err)
		retrieved, _ := store.Get(media.ID)
		assert.Equal(t, domain.MediaStatusDone, retrieved.Status)
		assert.Equal(t, "/converted.mp4", retrieved.ConvertedPath)
	})

	t.Run("persists to JSON file", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
		require.NoError(t, store.Save(media))

		mediaPath := filepath.Join(tempDir, "media.json")
		data, err := os.ReadFile(mediaPath)

		assert.NoError(t, err)
		var loaded []*domain.Media
		assert.NoError(t, json.Unmarshal(data, &loaded))
		assert.Len(t, loaded, 1)
		assert.Equal(t, media.ID, loaded[0].ID)
	})

	t.Run("uses mutex for concurrent safety", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
				store.Save(media)
			}()
		}
		wg.Wait()

		assert.Len(t, store.media, 10)
	})

	t.Run("creates temp file then renames for atomic write", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
		err := store.Save(media)

		assert.NoError(t, err)

		mediaPath := filepath.Join(tempDir, "media.json")
		tmpPath := mediaPath + ".tmp"

		_, err = os.Stat(mediaPath)
		assert.NoError(t, err)

		_, err = os.Stat(tmpPath)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestStoreGet(t *testing.T) {
	t.Run("returns existing media", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
		require.NoError(t, store.Save(media))

		retrieved, err := store.Get(media.ID)

		assert.NoError(t, err)
		assert.Equal(t, media.ID, retrieved.ID)
		assert.Equal(t, "test.mp4", retrieved.OriginalName)
	})

	t.Run("returns ErrNotFound for non-existent ID", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		retrieved, err := store.Get("nonexistent")

		assert.Error(t, err)
		assert.Equal(t, domain.ErrNotFound, err)
		assert.Nil(t, retrieved)
	})

	t.Run("returns correct media data", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
		media.MarkAsDone("/converted.mp4", domain.CodecAV1, 1280, 720, "/thumb.jpg", 512000)
		require.NoError(t, store.Save(media))

		retrieved, err := store.Get(media.ID)

		assert.NoError(t, err)
		assert.Equal(t, media.ID, retrieved.ID)
		assert.Equal(t, domain.MediaTypeVideo, retrieved.Type)
		assert.Equal(t, "test.mp4", retrieved.OriginalName)
		assert.Equal(t, "/path/to/test.mp4", retrieved.OriginalPath)
		assert.Equal(t, "/converted.mp4", retrieved.ConvertedPath)
		assert.Equal(t, domain.MediaStatusDone, retrieved.Status)
		assert.Equal(t, domain.CodecAV1, retrieved.Codec)
		assert.Equal(t, 1280, retrieved.Width)
		assert.Equal(t, 720, retrieved.Height)
		assert.Equal(t, "/thumb.jpg", retrieved.ThumbPath)
		assert.Equal(t, int64(512000), retrieved.FileSize)
		assert.Equal(t, 7, retrieved.RetentionDays)
	})
}

func TestStoreDelete(t *testing.T) {
	t.Run("deletes existing media", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
		require.NoError(t, store.Save(media))

		err := store.Delete(media.ID)

		assert.NoError(t, err)
		assert.NotContains(t, store.media, media.ID)
		_, err = store.Get(media.ID)
		assert.Error(t, err)
		assert.Equal(t, domain.ErrNotFound, err)
	})

	t.Run("persists deletion to JSON file", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
		require.NoError(t, store.Save(media))
		require.NoError(t, store.Delete(media.ID))

		newStore, err := NewStore(tempDir)
		assert.NoError(t, err)
		assert.Empty(t, newStore.media)
	})

	t.Run("no error deleting non-existent media", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		err := store.Delete("nonexistent")

		assert.NoError(t, err)
	})
}

func TestStoreListExpired(t *testing.T) {
	t.Run("returns only expired media", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		expiredMedia := domain.NewMedia(domain.MediaTypeVideo, "expired.mp4", "/path/to/expired.mp4", -1)
		validMedia := domain.NewMedia(domain.MediaTypeVideo, "valid.mp4", "/path/to/valid.mp4", 7)

		require.NoError(t, store.Save(expiredMedia))
		require.NoError(t, store.Save(validMedia))

		expired, err := store.ListExpired()

		assert.NoError(t, err)
		assert.Len(t, expired, 1)
		assert.Equal(t, expiredMedia.ID, expired[0].ID)
	})

	t.Run("returns empty list if none expired", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media1 := domain.NewMedia(domain.MediaTypeVideo, "test1.mp4", "/path/to/test1.mp4", 7)
		media2 := domain.NewMedia(domain.MediaTypeVideo, "test2.mp4", "/path/to/test2.mp4", 30)

		require.NoError(t, store.Save(media1))
		require.NoError(t, store.Save(media2))

		expired, err := store.ListExpired()

		assert.NoError(t, err)
		assert.Empty(t, expired)
	})

	t.Run("returns multiple expired items", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		expired1 := domain.NewMedia(domain.MediaTypeVideo, "expired1.mp4", "/path/to/expired1.mp4", -1)
		expired2 := domain.NewMedia(domain.MediaTypeVideo, "expired2.mp4", "/path/to/expired2.mp4", -1)
		validMedia := domain.NewMedia(domain.MediaTypeVideo, "valid.mp4", "/path/to/valid.mp4", 7)

		require.NoError(t, store.Save(expired1))
		require.NoError(t, store.Save(expired2))
		require.NoError(t, store.Save(validMedia))

		expired, err := store.ListExpired()

		assert.NoError(t, err)
		assert.Len(t, expired, 2)

		ids := make(map[string]bool)
		for _, m := range expired {
			ids[m.ID] = true
		}
		assert.Contains(t, ids, expired1.ID)
		assert.Contains(t, ids, expired2.ID)
		assert.NotContains(t, ids, validMedia.ID)
	})

	t.Run("ignores non-expired media", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		now := time.Now()
		expiredMedia := &domain.Media{
			ID:        "expired",
			ExpiresAt: now.Add(-time.Hour),
		}
		validMedia := &domain.Media{
			ID:        "valid",
			ExpiresAt: now.Add(time.Hour),
		}

		require.NoError(t, store.Save(expiredMedia))
		require.NoError(t, store.Save(validMedia))

		expired, err := store.ListExpired()

		assert.NoError(t, err)
		assert.Len(t, expired, 1)
		assert.Equal(t, expiredMedia.ID, expired[0].ID)
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Run("multiple goroutines can read simultaneously", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
		require.NoError(t, store.Save(media))

		var wg sync.WaitGroup
		errors := make(chan error, 100)

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := store.Get(media.ID)
				errors <- err
			}()
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			assert.NoError(t, err)
		}
	})

	t.Run("write operations are mutually exclusive", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
				media.ID = "media" + string(rune('0'+idx))
				store.Save(media)
			}(i)
		}
		wg.Wait()

		assert.Len(t, store.media, 10)
	})

	t.Run("no race conditions", func(t *testing.T) {
		tempDir := t.TempDir()
		store, _ := NewStore(tempDir)

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)
				store.Save(media)
				store.ListExpired()
			}()
		}
		wg.Wait()

		assert.Greater(t, len(store.media), 0)
	})
}
