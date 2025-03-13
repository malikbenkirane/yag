package test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitInit(t *testing.T) string {

	t.Helper()

	n, err := os.MkdirTemp(".", "test_")
	require.NoError(t, err)

	err = os.Chdir(n)
	require.NoError(t, err)

	err = exec.Command("git", "init").Run()
	require.NoError(t, err)

	return n

}

func TestStates(t *testing.T) {

	var err error

	h := func(t *testing.T, dir string) {
		t.Helper()

		err = os.Chdir("..")
		require.NoError(t, err)

		err = os.RemoveAll(filepath.Join("..", dir))
		require.NoError(t, err)
	}

	t.Run("untracked unstaged", func(t *testing.T) {
		dir := gitInit(t)
		defer h(t, dir)
		err = exec.Command("touch", "a").Run()
		require.NoError(t, err)

		for _, class := range []string{"untracked", "changed"} {
			for _, staged := range []string{"unstaged", "staged"} {
				expected := ""
				if class == "untracked" && staged == "unstaged" {
					expected = "a\n"
				}
				cname := fmt.Sprintf("list_%s_%s", class, staged)
				c := exec.Command("yag", "test", cname)
				var buf bytes.Buffer
				c.Stdout = &buf
				err = c.Run()
				require.NoError(t, err)
				assert.Equal(t, expected, buf.String(), "class=%q,%q,dir=%q", class, staged, dir)
			}
		}
	})

	t.Run("untracked staged", func(t *testing.T) {
		dir := gitInit(t)
		defer h(t, dir)

		err := exec.Command("touch", "a").Run()
		require.NoError(t, err)

		err = exec.Command("git", "add", "a").Run()
		require.NoError(t, err)

		for _, class := range []string{"untracked", "changed"} {
			for _, staged := range []string{"unstaged", "staged"} {
				expected := ""
				if class == "untracked" && staged == "staged" {
					expected = "a\n"
				}
				cname := fmt.Sprintf("list_%s_%s", class, staged)
				c := exec.Command("yag", "test", cname)
				var buf bytes.Buffer
				c.Stdout = &buf
				err = c.Run()
				require.NoError(t, err)
				assert.Equal(t, expected, buf.String())
			}
		}
	})

	t.Run("changed unstaged", func(t *testing.T) {
		dir := gitInit(t)
		defer h(t, dir)

		err := exec.Command("touch", "a").Run()
		require.NoError(t, err)

		err = exec.Command("git", "add", "a").Run()
		require.NoError(t, err)

		err = exec.Command("git", "commit", "-m", "hi").Run()
		require.NoError(t, err)

		f, err := os.OpenFile("a", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		require.NoError(t, err)
		_, err = fmt.Fprintln(f, "modified")
		require.NoError(t, err)
		err = f.Close()
		require.NoError(t, err)

		for _, class := range []string{"untracked", "changed"} {
			for _, staged := range []string{"unstaged", "staged"} {
				expected := ""
				if class == "changed" && staged == "unstaged" {
					expected = "a\n"
				}
				cname := fmt.Sprintf("list_%s_%s", class, staged)
				c := exec.Command("yag", "test", cname)
				var b bytes.Buffer
				c.Stdout = &b
				err = c.Run()
				require.NoError(t, err)
				assert.Equal(t, expected, b.String())
			}
		}
	})

	t.Run("changed staged", func(t *testing.T) {
		dir := gitInit(t)
		defer h(t, dir)

		err := exec.Command("touch", "a").Run()
		require.NoError(t, err)

		err = exec.Command("git", "add", "a").Run()
		require.NoError(t, err)

		err = exec.Command("git", "commit", "-m", "hi").Run()
		require.NoError(t, err)

		f, err := os.OpenFile("a", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		require.NoError(t, err)
		_, err = fmt.Fprintln(f, "modified")
		require.NoError(t, err)
		err = f.Close()
		require.NoError(t, err)

		err = exec.Command("git", "add", "a").Run()
		require.NoError(t, err)

		for _, class := range []string{"untracked", "changed"} {
			for _, staged := range []string{"unstaged", "staged"} {
				expected := ""
				if class == "changed" && staged == "staged" {
					expected = "a\n"
				}
				cname := fmt.Sprintf("list_%s_%s", class, staged)
				c := exec.Command("yag", "test", cname)
				var b bytes.Buffer
				c.Stdout = &b
				err = c.Run()
				require.NoError(t, err)
				assert.Equal(t, expected, b.String())
			}
		}
	})
}
