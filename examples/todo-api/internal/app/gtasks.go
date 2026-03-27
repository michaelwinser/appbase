package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/auth"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
	"github.com/michaelwinser/appbase/server"
)

const googleTasksBase = "https://tasks.googleapis.com/tasks/v1"

// getAccessToken returns the OAuth access token, refreshing if expired.
func getAccessToken(r *http.Request, google *auth.GoogleAuth) (string, error) {
	token := appbase.AccessToken(r)
	if token == "" {
		return "", fmt.Errorf("no Google API access token — re-login to grant Tasks permission")
	}

	// Refresh if expired
	expiry := auth.TokenExpiry(r)
	if !expiry.IsZero() && time.Now().After(expiry) && google != nil {
		// Need session to refresh — get it from middleware context
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			return "", fmt.Errorf("token expired and no session cookie for refresh")
		}
		_ = cookie // session refresh happens via the GoogleAuth method which needs a Session object
		// For now, return the expired token and let the API call fail with 401,
		// which tells the user to re-login. Full transparent refresh requires
		// the Session object in the middleware context (future enhancement).
		return token, nil
	}

	return token, nil
}

// PushToGoogleTasks pushes local todos to the user's Google Tasks list.
func (s *TodoServer) PushToGoogleTasks(w http.ResponseWriter, r *http.Request) {
	token, err := getAccessToken(r, s.Google)
	if err != nil {
		server.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	userID := appbase.UserID(r)
	if userID == "" {
		userID = "anonymous"
	}

	todos, err := s.Store.List(userID)
	if err != nil {
		server.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	synced, errors := 0, 0
	for _, todo := range todos {
		if err := createGoogleTask(r.Context(), token, todo.Title); err != nil {
			errors++
		} else {
			synced++
		}
	}

	msg := fmt.Sprintf("Pushed %d todos to Google Tasks", synced)
	if errors > 0 {
		msg += fmt.Sprintf(" (%d failed)", errors)
	}
	server.RespondJSON(w, http.StatusOK, api.SyncResult{
		Synced:  synced,
		Errors:  errors,
		Message: Ptr(msg),
	})
}

// PullFromGoogleTasks imports tasks from Google Tasks as local todos.
func (s *TodoServer) PullFromGoogleTasks(w http.ResponseWriter, r *http.Request) {
	token, err := getAccessToken(r, s.Google)
	if err != nil {
		server.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	userID := appbase.UserID(r)
	if userID == "" {
		userID = "anonymous"
	}

	tasks, err := listGoogleTasks(r.Context(), token)
	if err != nil {
		server.RespondError(w, http.StatusInternalServerError, "Google Tasks API: "+err.Error())
		return
	}

	// Get existing todos to avoid duplicates (by title)
	existing, _ := s.Store.List(userID)
	existingTitles := make(map[string]bool, len(existing))
	for _, t := range existing {
		existingTitles[strings.ToLower(t.Title)] = true
	}

	synced, skipped := 0, 0
	for _, task := range tasks {
		if task.Title == "" || existingTitles[strings.ToLower(task.Title)] {
			skipped++
			continue
		}
		if _, err := s.Store.Create(userID, task.Title); err != nil {
			skipped++
		} else {
			synced++
		}
	}

	msg := fmt.Sprintf("Imported %d tasks from Google Tasks", synced)
	if skipped > 0 {
		msg += fmt.Sprintf(" (%d skipped)", skipped)
	}
	server.RespondJSON(w, http.StatusOK, api.SyncResult{
		Synced:  synced,
		Errors:  skipped,
		Message: Ptr(msg),
	})
}

// --- Google Tasks API helpers ---

type googleTask struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

type googleTaskList struct {
	Items []googleTask `json:"items"`
}

func createGoogleTask(ctx context.Context, token, title string) error {
	body, _ := json.Marshal(googleTask{Title: title, Status: "needsAction"})
	req, err := http.NewRequestWithContext(ctx, "POST",
		googleTasksBase+"/lists/@default/tasks",
		strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func listGoogleTasks(ctx context.Context, token string) ([]googleTask, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		googleTasksBase+"/lists/@default/tasks?maxResults=100&showCompleted=false", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	var list googleTaskList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list.Items, nil
}
