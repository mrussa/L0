package cache_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mrussa/L0/internal/cache"
	"github.com/mrussa/L0/internal/repo"
)

func TestNew_IsEmpty(t *testing.T) {
	t.Parallel()

	c := cache.New()
	require.Equal(t, 0, c.Len(), "новый кеш должен быть пустым")

	_, ok := c.Get("missing")
	require.False(t, ok, "должен быть miss для несуществующего ключа")
}

func TestSetAndGet(t *testing.T) {
	t.Parallel()

	c := cache.New()
	ord := repo.Order{OrderUID: "u1"}

	c.Set("u1", ord)

	got, ok := c.Get("u1")
	require.True(t, ok)
	require.Equal(t, ord, got)
	require.Equal(t, 1, c.Len())
}

func TestOverwrite_UpdatesValueAndDoesNotGrow(t *testing.T) {
	t.Parallel()

	c := cache.New()
	first := repo.Order{OrderUID: "first"}
	second := repo.Order{OrderUID: "second"}

	c.Set("same", first)
	require.Equal(t, 1, c.Len())

	c.Set("same", second)
	require.Equal(t, 1, c.Len(), "overwrite не должен увеличивать размер")

	got, ok := c.Get("same")
	require.True(t, ok)
	require.Equal(t, "second", got.OrderUID, "значение должно быть перезаписано")
}

func TestDelete_RemovesKey(t *testing.T) {
	t.Parallel()

	c := cache.New()
	c.Set("x", repo.Order{OrderUID: "x"})
	require.Equal(t, 1, c.Len())

	c.Delete("x")
	_, ok := c.Get("x")
	require.False(t, ok)
	require.Equal(t, 0, c.Len())
}

func TestDelete_MissingIsNoop(t *testing.T) {
	t.Parallel()

	c := cache.New()
	c.Set("a", repo.Order{OrderUID: "a"})
	require.Equal(t, 1, c.Len())

	c.Delete("nope")
	require.Equal(t, 1, c.Len())
}

func TestConcurrent_SetGetDelete(t *testing.T) {
	c := cache.New()

	const n = 200

	doneSet := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			k := fmt.Sprintf("k%03d", i)
			c.Set(k, repo.Order{OrderUID: k})
		}
		close(doneSet)
	}()
	<-doneSet
	require.Equal(t, n, c.Len(), "после записи всех ключей размер должен совпадать")

	for _, i := range []int{0, 1, 2, 50, 123, 199} {
		k := fmt.Sprintf("k%03d", i)
		got, ok := c.Get(k)
		require.True(t, ok, "должен быть hit для %s", k)
		require.Equal(t, k, got.OrderUID)
	}

	doneDel := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			k := fmt.Sprintf("k%03d", i)
			c.Delete(k)
		}
		close(doneDel)
	}()
	<-doneDel
	require.Equal(t, 0, c.Len(), "после удаления всех ключей кеш должен быть пуст")
}
