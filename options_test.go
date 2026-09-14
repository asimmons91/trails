package trails

import (
	"context"
	"log/slog"
	"reflect"
	"testing"

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
