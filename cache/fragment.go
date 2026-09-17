package cache

import (
	"context"
	"fmt"
	"html/template"
	"time"

	trails "github.com/asimmons91/trails"
)

func FetchFragment(ctx context.Context, c *trails.Context, store Store, block, key string, ttl time.Duration, data any) (template.HTML, error) {
	if raw, ok, err := store.Read(ctx, key); err != nil {
		return "", fmt.Errorf("cache: reading fragment %q: %w", key, err)
	} else if ok {
		return template.HTML(raw), nil
	}

	html, err := c.RenderBlock(block, data)
	if err != nil {
		return "", err
	}

	if err := store.Write(ctx, key, []byte(html), ttl); err != nil {
		return "", fmt.Errorf("cache: writing fragment %q: %w", key, err)
	}

	return html, nil
}
