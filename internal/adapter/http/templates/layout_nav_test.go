package templates

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayoutNavRendersConfigAndRemovesPasswordDialog(t *testing.T) {
	var out bytes.Buffer

	err := Layout(LayoutProps{
		Title:       "Sharm",
		ShowNav:     true,
		ActiveRoute: "dashboard",
		Version:     "dev",
	}).Render(context.Background(), &out)
	require.NoError(t, err)

	html := out.String()
	assert.Contains(t, html, `<a href="/config" class="nav-link nav-link--icon"`)
	assert.Contains(t, html, `<a href="/config" class="bottom-nav-item"`)
	assert.Contains(t, html, `<span>Config</span>`)
	assert.NotContains(t, html, "password-dialog")
}

func TestLayoutNavMarksConfigActiveWhenRouteIsConfig(t *testing.T) {
	var out bytes.Buffer

	err := Layout(LayoutProps{
		Title:       "Sharm",
		ShowNav:     true,
		ActiveRoute: "config",
		Version:     "dev",
	}).Render(context.Background(), &out)
	require.NoError(t, err)

	html := out.String()
	assert.Contains(t, html, `<a href="/config" class="nav-link nav-link--icon" aria-current="page" title="Configuration">`)
	assert.Contains(t, html, `<a href="/config" class="bottom-nav-item" aria-current="page">`)
}
