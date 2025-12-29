package destination

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalDir(t *testing.T) {
	dir := t.TempDir()

	ld, err := NewLocalDir(LocalDirConfig{Path: dir})
	require.NoError(t, err)

	require.Equal(t, []FileInfo{}, must(ld.ReadDir("/")))
	require.False(t, must(ld.FileExists("test.txt")))
	require.NoError(t, ld.WriteFile("test.txt", []byte("a test")))
	require.True(t, must(ld.FileExists("test.txt")))
	require.Equal(t, []byte("a test"), must(ld.ReadFile("test.txt")))
	require.Equal(t, []FileInfo{{Name: "test.txt", IsDir: false}}, must(ld.ReadDir("/")))

	require.False(t, must(ld.FileExists("subdir/stuff.txt")))
	require.NoError(t, ld.WriteFile("subdir/stuff.txt", []byte("täßt")))
	require.True(t, must(ld.FileExists("subdir/stuff.txt")))
	require.Equal(t, []byte("täßt"), must(ld.ReadFile("subdir/stuff.txt")))
	require.Equal(t, []FileInfo{{Name: "stuff.txt", IsDir: false}}, must(ld.ReadDir("subdir")))
	dirContent := must(ld.ReadDir("/"))
	require.Len(t, dirContent, 2)
	require.Contains(t, dirContent, FileInfo{Name: "test.txt", IsDir: false})
	require.Contains(t, dirContent, FileInfo{Name: "subdir", IsDir: true})

	require.NoError(t, ld.DeleteDir("subdir"))
	require.Equal(t, []FileInfo{{Name: "test.txt", IsDir: false}}, must(ld.ReadDir("/")))

	require.NoError(t, ld.DeleteDir("test.txt"))
	require.Equal(t, []FileInfo{}, must(ld.ReadDir("/")))
}

//TODO test some error cases

func must[T any](result T, err error) T {
	if err != nil {
		panic(err)
	}
	return result
}
