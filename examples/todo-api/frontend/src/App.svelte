<script lang="ts">
  import { onMount } from 'svelte';
  import {
    getAuthStatus,
    getLoginURL,
    logout,
    listTodos,
    createTodo,
    deleteTodo,
    type Todo,
    type AuthStatus,
  } from './lib/api';

  let auth: AuthStatus = { loggedIn: false };
  let todos: Todo[] = [];
  let newTitle = '';
  let loading = true;
  let error = '';

  onMount(async () => {
    try {
      auth = await getAuthStatus();
      if (auth.loggedIn) {
        todos = await listTodos();
      }
    } catch (e) {
      error = 'Failed to connect to server';
    }
    loading = false;
  });

  async function handleLogin() {
    try {
      const url = await getLoginURL();
      window.location.href = url;
    } catch (e) {
      error = 'Login not available';
    }
  }

  async function handleLogout() {
    await logout();
    auth = { loggedIn: false };
    todos = [];
  }

  async function handleAdd() {
    if (!newTitle.trim()) return;
    try {
      const todo = await createTodo(newTitle.trim());
      todos = [todo, ...todos];
      newTitle = '';
      error = '';
    } catch (e) {
      error = 'Failed to create todo';
    }
  }

  async function handleDelete(id: string) {
    try {
      await deleteTodo(id);
      todos = todos.filter((t) => t.id !== id);
      error = '';
    } catch (e) {
      error = 'Failed to delete todo';
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') handleAdd();
  }
</script>

<main>
  <h1>Todo</h1>

  {#if loading}
    <p class="muted">Loading...</p>
  {:else if !auth.loggedIn}
    <div class="login-card">
      <p>Sign in to manage your todos</p>
      <button onclick={handleLogin}>Sign in with Google</button>
    </div>
  {:else}
    <p class="user">
      Signed in as <strong>{auth.email}</strong>
      <button class="link" onclick={handleLogout}>Sign out</button>
    </p>

    <div class="add-form">
      <input
        type="text"
        bind:value={newTitle}
        onkeydown={handleKeydown}
        placeholder="What needs to be done?"
      />
      <button onclick={handleAdd}>Add</button>
    </div>

    {#if error}
      <p class="error">{error}</p>
    {/if}

    {#if todos.length === 0}
      <p class="muted">No todos yet. Add one above.</p>
    {:else}
      <ul class="todo-list">
        {#each todos as todo (todo.id)}
          <li>
            <span class="title">{todo.title}</span>
            <button class="delete" onclick={() => handleDelete(todo.id)}>&times;</button>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</main>

<style>
  :global(body) {
    font-family: system-ui, -apple-system, sans-serif;
    margin: 0;
    padding: 0;
    background: #f8f9fa;
    color: #333;
  }

  main {
    max-width: 600px;
    margin: 2rem auto;
    padding: 0 1rem;
  }

  h1 {
    font-size: 1.8rem;
    margin-bottom: 1rem;
  }

  .login-card {
    background: white;
    border-radius: 12px;
    padding: 2rem;
    text-align: center;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  }

  .login-card button {
    padding: 0.75rem 1.5rem;
    border: 1px solid #ddd;
    border-radius: 8px;
    background: white;
    font-size: 1rem;
    cursor: pointer;
  }

  .login-card button:hover {
    background: #f0f0f0;
  }

  .user {
    color: #666;
    margin-bottom: 1rem;
  }

  .user .link {
    background: none;
    border: none;
    color: #0066cc;
    cursor: pointer;
    font-size: inherit;
    padding: 0;
    margin-left: 0.5rem;
  }

  .add-form {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  .add-form input {
    flex: 1;
    padding: 0.6rem 0.8rem;
    border: 1px solid #ddd;
    border-radius: 8px;
    font-size: 1rem;
  }

  .add-form button {
    padding: 0.6rem 1.2rem;
    border: none;
    border-radius: 8px;
    background: #333;
    color: white;
    font-size: 1rem;
    cursor: pointer;
  }

  .add-form button:hover {
    background: #555;
  }

  .error {
    color: #cc3333;
    font-size: 0.9rem;
  }

  .muted {
    color: #999;
  }

  .todo-list {
    list-style: none;
    padding: 0;
  }

  .todo-list li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 0;
    border-bottom: 1px solid #eee;
  }

  .todo-list .title {
    flex: 1;
  }

  .todo-list .delete {
    background: none;
    border: none;
    color: #cc3333;
    font-size: 1.3rem;
    cursor: pointer;
    padding: 0 0.5rem;
    opacity: 0.4;
  }

  .todo-list .delete:hover {
    opacity: 1;
  }
</style>
