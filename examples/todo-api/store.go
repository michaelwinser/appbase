package main

// Re-export from internal/app for use by main.go and tests.

import todoapp "github.com/michaelwinser/appbase/examples/todo-api/internal/app"

type TodoEntity = todoapp.TodoEntity
type TodoStore = todoapp.TodoStore
type TodoServer = todoapp.TodoServer

var NewTodoStore = todoapp.NewTodoStore
