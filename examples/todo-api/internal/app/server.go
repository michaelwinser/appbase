package app

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/auth"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
	"github.com/michaelwinser/appbase/server"
)

// Ensure TodoServer implements the generated interface.
var _ api.ServerInterface = (*TodoServer)(nil)

// TodoServer implements the generated ServerInterface.
type TodoServer struct {
	Store  *TodoStore
	Google *auth.GoogleAuth // nil when auth not configured (local mode)
}

func (s *TodoServer) ListTodos(w http.ResponseWriter, r *http.Request) {
	userID := appbase.UserID(r)
	if userID == "" {
		userID = "anonymous"
	}
	items, err := s.Store.List(userID)
	if err != nil {
		server.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	todos := make([]api.Todo, len(items))
	for i, t := range items {
		ca, _ := time.Parse(time.RFC3339, t.CreatedAt)
		todos[i] = api.Todo{
			Id:        t.ID,
			UserId:    t.UserID,
			Title:     t.Title,
			Done:      t.Done,
			CreatedAt: ca,
		}
	}
	server.RespondJSON(w, http.StatusOK, todos)
}

func (s *TodoServer) CreateTodo(w http.ResponseWriter, r *http.Request) {
	userID := appbase.UserID(r)
	if userID == "" {
		userID = "anonymous"
	}
	var req api.CreateTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
		server.RespondError(w, http.StatusBadRequest, "title is required")
		return
	}
	todo, err := s.Store.Create(userID, req.Title)
	if err != nil {
		server.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ca, _ := time.Parse(time.RFC3339, todo.CreatedAt)
	server.RespondJSON(w, http.StatusCreated, api.Todo{
		Id:        todo.ID,
		UserId:    todo.UserID,
		Title:     todo.Title,
		Done:      todo.Done,
		CreatedAt: ca,
	})
}

func (s *TodoServer) DeleteTodo(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.Store.Delete(id); err != nil {
		server.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondJSON(w, http.StatusOK, api.OkResponse{Ok: Ptr("true")})
}

func Ptr(s string) *string { return &s }
