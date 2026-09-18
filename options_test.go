package trails

import (
	"context"
	"html/template"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func funcPointer(f any) uintptr {
	return reflect.ValueOf(f).Pointer()
}

func TestWithDefaultOptionsFillsAllDefaults(t *testing.T) {
	o := WithDefaultOptions(&TrailOptions{})

	require.Equal(t, context.Background(), o.Context)
	require.Same(t, slog.Default(), o.Logger)
	require.Equal(t, funcPointer(ErrorHandlerFunc(defaultErrorHandler)), funcPointer(o.ErrorHandler))
	require.Equal(t, "127.0.0.1", o.Host)
	require.Equal(t, 3000, o.Port)
	require.IsType(t, &DefaultBinder{}, o.Binder)
	require.Equal(t, AssetsStrategyImportMap, o.AssetsStrategy)
	require.Equal(t, 5*time.Second, o.ReadHeaderTimeout)
	require.Equal(t, 30*time.Second, o.ReadTimeout)
	require.Equal(t, time.Duration(0), o.WriteTimeout)
	require.Equal(t, 120*time.Second, o.IdleTimeout)
}

func TestWithDefaultOptionsPreservesExplicitTimeouts(t *testing.T) {
	input := &TrailOptions{
		ReadHeaderTimeout: 1 * time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
	}

	o := WithDefaultOptions(input)

	require.Equal(t, 1*time.Second, o.ReadHeaderTimeout)
	require.Equal(t, 2*time.Second, o.ReadTimeout)
	require.Equal(t, 3*time.Second, o.WriteTimeout)
	require.Equal(t, 4*time.Second, o.IdleTimeout)
}

func TestWithDefaultOptionsPreservesExplicitAssetsStrategy(t *testing.T) {
	o := WithDefaultOptions(&TrailOptions{AssetsStrategy: AssetsStrategyBundler})

	require.Equal(t, AssetsStrategyBundler, o.AssetsStrategy)
}

func TestWithDefaultOptionsPreservesExplicitValues(t *testing.T) {
	ctx := context.WithValue(context.Background(), testContextKey("key"), "value")
	logger := slog.New(slog.NewTextHandler(nil, nil))
	handler := func(c *Context, err HTTPError) {}
	binder := &DefaultBinder{}

	input := &TrailOptions{
		Context:      ctx,
		Logger:       logger,
		ErrorHandler: handler,
		Binder:       binder,
		Host:         "0.0.0.0",
		Port:         8080,
	}

	o := WithDefaultOptions(input)

	require.Equal(t, ctx, o.Context)
	require.Same(t, logger, o.Logger)
	require.Equal(t, funcPointer(ErrorHandlerFunc(handler)), funcPointer(o.ErrorHandler))
	require.Same(t, binder, o.Binder)
	require.Equal(t, "0.0.0.0", o.Host)
	require.Equal(t, 8080, o.Port)
}

func TestWithDefaultOptionsPartialOverride(t *testing.T) {
	input := &TrailOptions{Host: "0.0.0.0"}

	o := WithDefaultOptions(input)

	require.Equal(t, "0.0.0.0", o.Host)
	require.Equal(t, 3000, o.Port)
	require.Equal(t, context.Background(), o.Context)
	require.Same(t, slog.Default(), o.Logger)
}

func TestWithDefaultOptionsPreservesFuncMap(t *testing.T) {
	fm := template.FuncMap{"upper": strings.ToUpper}

	o := WithDefaultOptions(&TrailOptions{FuncMap: fm})

	require.Equal(t, funcPointer(fm["upper"]), funcPointer(o.FuncMap["upper"]))
}

func TestWithDefaultOptionsDoesNotMutateInput(t *testing.T) {
	input := &TrailOptions{}

	o := WithDefaultOptions(input)

	require.NotSame(t, input, o)
	require.Nil(t, input.Context)
	require.Nil(t, input.Logger)
	require.Nil(t, input.ErrorHandler)
	require.Nil(t, input.Binder)
	require.Equal(t, "", input.Host)
	require.Equal(t, 0, input.Port)
}
