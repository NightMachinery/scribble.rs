package frontend

import (
	"log"
	"net/http"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/scribble-rs/scribble.rs/internal/api"
	"github.com/scribble-rs/scribble.rs/internal/game"
	"github.com/scribble-rs/scribble.rs/internal/identity"
	"github.com/scribble-rs/scribble.rs/internal/state"
	"github.com/scribble-rs/scribble.rs/internal/translations"
	"golang.org/x/text/language"
)

const clientIDRestoreAttempted = "client_id_restore_attempted"

type lobbyPageData struct {
	*BasePageConfig
	*api.LobbyData

	Translation *translations.Translation
	Locale      string
}

type lobbyPasswordPageData struct {
	*BasePageConfig

	LobbyID      string
	ErrorMessage string

	Translation *translations.Translation
	Locale      string
}

type lobbyJsData struct {
	*BasePageConfig
	*api.GameConstants

	Translation *translations.Translation
	Locale      string
}

type restoreSessionPageData struct {
	*BasePageConfig

	ContinueURL string

	Translation *translations.Translation
	Locale      string
}

func (handler *SSRHandler) lobbyJs(writer http.ResponseWriter, request *http.Request) {
	translation, locale := determineTranslation(request)
	pageData := &lobbyJsData{
		BasePageConfig: handler.basePageConfig,
		GameConstants:  api.GameConstantsData,
		Translation:    translation,
		Locale:         locale,
	}

	writer.Header().Set("Content-Type", "text/javascript")
	// Duration of 1 year, since we use cachebusting anyway.
	writer.Header().Set("Cache-Control", "public, max-age=31536000")
	writer.WriteHeader(http.StatusOK)
	if err := handler.lobbyJsRawTemplate.ExecuteTemplate(writer, "lobby-js", pageData); err != nil {
		log.Printf("error templating JS: %s\n", err)
	}
}

// ssrEnterLobby opens a lobby, either opening it directly or asking for a lobby.
func (handler *SSRHandler) ssrEnterLobby(writer http.ResponseWriter, request *http.Request) {
	translation, locale := determineTranslation(request)
	lobby := state.GetLobby(request.PathValue("lobby_id"))
	if lobby == nil {
		handler.userFacingError(writer, translation.Get("lobby-doesnt-exist"), translation)
		return
	}

	userAgent := strings.ToLower(request.UserAgent())
	if !isHumanAgent(userAgent) {
		writer.WriteHeader(http.StatusForbidden)
		handler.userFacingError(writer, translation.Get("forbidden"), translation)
		return
	}

	player := api.GetPlayer(lobby, request)
	if player == nil && shouldAttemptClientIDRestore(request) {
		handler.renderClientIDRestorePage(writer, request, translation, locale)
		return
	}

	if player == nil && lobby.RequiresPassword() {
		if err := pageTemplates.ExecuteTemplate(writer, "lobby-password-page", &lobbyPasswordPageData{
			BasePageConfig: handler.basePageConfig,
			LobbyID:        lobby.LobbyID,
			Translation:    translation,
			Locale:         locale,
		}); err != nil {
			log.Printf("Error templating lobby password page: %s\n", err)
		}
		return
	}

	handler.ssrEnterLobbyNoChecks(lobby, writer, request,
		func() *game.Player {
			return player
		})
}

func (handler *SSRHandler) ssrJoinLobby(writer http.ResponseWriter, request *http.Request) {
	translation, locale := determineTranslation(request)
	lobby := state.GetLobby(request.PathValue("lobby_id"))
	if lobby == nil {
		handler.userFacingError(writer, translation.Get("lobby-doesnt-exist"), translation)
		return
	}
	if err := request.ParseForm(); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	lobbyPassword, passwordErr := api.ParseLobbyPassword(request.Form.Get("password"))
	if passwordErr != nil {
		if err := pageTemplates.ExecuteTemplate(writer, "lobby-password-page", &lobbyPasswordPageData{
			BasePageConfig: handler.basePageConfig,
			LobbyID:        lobby.LobbyID,
			ErrorMessage:   passwordErr.Error(),
			Translation:    translation,
			Locale:         locale,
		}); err != nil {
			log.Printf("Error templating lobby password page: %s\n", err)
		}
		return
	}

	if player := api.GetPlayer(lobby, request); player == nil && !lobby.ValidateJoinPassword(lobbyPassword) {
		if err := pageTemplates.ExecuteTemplate(writer, "lobby-password-page", &lobbyPasswordPageData{
			BasePageConfig: handler.basePageConfig,
			LobbyID:        lobby.LobbyID,
			ErrorMessage:   translation.Get("lobby-password-invalid"),
			Translation:    translation,
			Locale:         locale,
		}); err != nil {
			log.Printf("Error templating lobby password page: %s\n", err)
		}
		return
	}

	pageData := handler.joinLobbyNoChecks(lobby, writer, request,
		func() *game.Player {
			return api.GetPlayer(lobby, request)
		})
	if pageData == nil {
		return
	}

	http.Redirect(writer, request, handler.basePageConfig.RootPath+"/lobby/"+lobby.LobbyID, http.StatusFound)
}

func (handler *SSRHandler) ssrEnterLobbyNoChecks(
	lobby *game.Lobby,
	writer http.ResponseWriter,
	request *http.Request,
	getPlayer func() *game.Player,
) {
	pageData := handler.joinLobbyNoChecks(lobby, writer, request, getPlayer)

	// If the pagedata isn't initialized, it means the synchronized block has exited.
	// In this case we don't want to template the lobby, since an error has occurred
	// and probably already has been handled.
	if pageData != nil {
		if err := pageTemplates.ExecuteTemplate(writer, "lobby-page", pageData); err != nil {
			log.Printf("Error templating lobby: %s\n", err)
		}
	}
}

func (handler *SSRHandler) joinLobbyNoChecks(
	lobby *game.Lobby,
	writer http.ResponseWriter,
	request *http.Request,
	getPlayer func() *game.Player,
) *lobbyPageData {
	translation, locale := determineTranslation(request)
	requestAddress := api.GetIPAddressFromRequest(request)
	api.SetDiscordCookies(writer, request)

	var pageData *lobbyPageData
	lobby.Synchronized(func() {
		player := getPlayer()

		if player == nil {
			if !lobby.HasFreePlayerSlot() {
				handler.userFacingError(writer, translation.Get("lobby-full"), translation)
				return
			}

			if !lobby.CanIPConnect(requestAddress) {
				handler.userFacingError(writer, translation.Get("lobby-ip-limit-excceeded"), translation)
				return
			}

			newPlayer := lobby.JoinPlayer(api.GetPlayername(request))

			newPlayer.SetLastKnownAddress(requestAddress)
			api.SetGameplayCookies(writer, request, newPlayer, lobby)
			if err := identity.SetName(newPlayer.GetClientID(), newPlayer.Name); err != nil {
				log.Printf("error persisting player display name: %v", err)
			}
		} else {
			player.SetLastKnownAddress(requestAddress)
			api.SetGameplayCookies(writer, request, player, lobby)
		}

		pageData = &lobbyPageData{
			BasePageConfig: handler.basePageConfig,
			LobbyData:      api.CreateLobbyData(handler.cfg, lobby),
			Translation:    translation,
			Locale:         locale,
		}
	})

	return pageData
}

func determineTranslation(r *http.Request) (*translations.Translation, string) {
	languageTags, _, err := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if err == nil {
		for _, languageTag := range languageTags {
			fullLanguageIdentifier := languageTag.String()
			fullLanguageIdentifierLowercased := strings.ToLower(fullLanguageIdentifier)
			translation := translations.GetLanguage(fullLanguageIdentifierLowercased)
			if translation != nil {
				return translation, fullLanguageIdentifierLowercased
			}

			baseLanguageIdentifier, _ := languageTag.Base()
			baseLanguageIdentifierLowercased := strings.ToLower(baseLanguageIdentifier.String())
			translation = translations.GetLanguage(baseLanguageIdentifierLowercased)
			if translation != nil {
				return translation, baseLanguageIdentifierLowercased
			}
		}
	}

	return translations.DefaultTranslation, "en-us"
}

func shouldAttemptClientIDRestore(request *http.Request) bool {
	if request.URL.Query().Get(clientIDRestoreAttempted) != "" {
		return false
	}

	userSession, err := api.GetUserSession(request)
	if err != nil {
		userSession = uuid.Nil
	}
	if userSession != uuid.Nil {
		return false
	}

	clientID, err := api.GetClientID(request)
	if err != nil {
		clientID = uuid.Nil
	}

	return clientID == uuid.Nil
}

func (handler *SSRHandler) renderClientIDRestorePage(
	writer http.ResponseWriter,
	request *http.Request,
	translation *translations.Translation,
	locale string,
) {
	continueURL := *request.URL
	queryValues := continueURL.Query()
	queryValues.Set(clientIDRestoreAttempted, "1")
	continueURL.RawQuery = queryValues.Encode()

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	if err := pageTemplates.ExecuteTemplate(writer, "restore-session-page", &restoreSessionPageData{
		BasePageConfig: handler.basePageConfig,
		ContinueURL:    continueURL.String(),
		Translation:    translation,
		Locale:         locale,
	}); err != nil {
		log.Printf("Error templating restore session page: %s\n", err)
	}
}
