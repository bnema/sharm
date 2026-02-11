package templates

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigRendersPasswordAndPWAControls(t *testing.T) {
	var out bytes.Buffer

	err := Config("", "dev").Render(context.Background(), &out)
	require.NoError(t, err)

	html := out.String()
	assert.Contains(t, html, "Change Password")
	assert.Contains(t, html, `hx-post="/change-password"`)
	assert.Contains(t, html, "Appearance")
	assert.Contains(t, html, `id="theme-mode-select"`)
	assert.Contains(t, html, `value="auto"`)
	assert.Contains(t, html, `value="dark"`)
	assert.Contains(t, html, `value="light"`)
	assert.Contains(t, html, "PWA")
	assert.Contains(t, html, ">Install<")
	assert.Contains(t, html, ">Reinstall<")
	assert.Contains(t, html, ">Clear local PWA data<")
	assert.Contains(t, html, "Menu &gt; Install app")
}

func TestConfigSetupTemplateDoesNotDefineChangePasswordBlocks(t *testing.T) {
	content, err := os.ReadFile("setup.templ")
	require.NoError(t, err)

	source := string(content)
	assert.NotContains(t, source, "templ ChangePassword(")
	assert.NotContains(t, source, "templ ChangePasswordSuccess(")
}
