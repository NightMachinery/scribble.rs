package frontend

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scribble-rs/scribble.rs/internal/api"
	"github.com/scribble-rs/scribble.rs/internal/config"
	"github.com/scribble-rs/scribble.rs/internal/translations"
	"github.com/stretchr/testify/require"
)

func Test_templateLobbyPage(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	err := pageTemplates.ExecuteTemplate(&buffer,
		"lobby-page", &lobbyPageData{
			BasePageConfig: &BasePageConfig{
				checksums: make(map[string]string),
			},
			LobbyData: &api.LobbyData{
				SettingBounds: config.Default.LobbySettingBounds,
				GameConstants: api.GameConstantsData,
			},
			Translation: translations.DefaultTranslation,
		})
	if err != nil {
		t.Errorf("Error templating: %s", err)
	}
}

func Test_templateErrorPage(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	err := pageTemplates.ExecuteTemplate(&buffer,
		"error-page", &errorPageData{
			BasePageConfig: &BasePageConfig{},
			ErrorMessage:   "KEK",
			Translation:    translations.DefaultTranslation,
			Locale:         "en-US",
		})
	if err != nil {
		t.Errorf("Error templating: %s", err)
	}
}

func Test_templateIndexPage(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&config.Config{})
	require.NoError(t, err)
	createPageData := handler.createDefaultIndexPageData()
	createPageData.Translation = translations.DefaultTranslation

	var buffer bytes.Buffer
	if err := pageTemplates.ExecuteTemplate(&buffer, "index", createPageData); err != nil {
		t.Errorf("Error templating: %s", err)
	}
	require.True(t, strings.Contains(buffer.String(), "allowed_edit_distance_percent"))
}

func TestRenderedEnglishLobbyJavaScriptIsSyntaxValid(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required to syntax-check rendered JavaScript")
	}

	handler, err := NewHandler(&config.Default)
	require.NoError(t, err)

	var buffer bytes.Buffer
	err = handler.lobbyJsRawTemplate.ExecuteTemplate(&buffer, "lobby-js", &lobbyJsData{
		BasePageConfig: handler.basePageConfig,
		GameConstants:  api.GameConstantsData,
		Translation:    translations.DefaultTranslation,
		Locale:         "en-us",
	})
	require.NoError(t, err)

	rendered := buffer.String()
	require.NotContains(t, rendered, "'Scribble.rs couldn't establish a socket connection.\n")

	jsPath := filepath.Join(t.TempDir(), "lobby.js")
	require.NoError(t, os.WriteFile(jsPath, buffer.Bytes(), 0o600))

	output, err := exec.Command("node", "--check", jsPath).CombinedOutput()
	require.NoError(t, err, string(output))
}

func Test_createDefaultIndexPageDataUsesUpdatedLobbyDefaults(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&config.Default)
	require.NoError(t, err)

	pageData := handler.createDefaultIndexPageData()
	require.Equal(t, "120", pageData.DrawingTime)
	require.Equal(t, "false", pageData.AssignRandomNames)
}

func Test_templateLobbyPasswordPage(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	err := pageTemplates.ExecuteTemplate(&buffer,
		"lobby-password-page", &lobbyPasswordPageData{
			BasePageConfig: &BasePageConfig{
				checksums: make(map[string]string),
			},
			LobbyID:     "TEST",
			Translation: translations.DefaultTranslation,
			Locale:      "en-US",
		})
	if err != nil {
		t.Errorf("Error templating: %s", err)
	}
}
