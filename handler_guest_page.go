package main

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
)

const guestPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Join Call — Wally Conference</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #1a1a2e; color: #eee;
    display: flex; align-items: center; justify-content: center;
    min-height: 100vh; padding: 1rem;
  }
  .card {
    background: #16213e; border-radius: 12px; padding: 2rem;
    max-width: 400px; width: 100%%; box-shadow: 0 4px 24px rgba(0,0,0,.4);
  }
  h1 { font-size: 1.4rem; margin-bottom: .25rem; }
  .subtitle { color: #888; font-size: .85rem; margin-bottom: 1.5rem; word-break: break-all; }
  label { display: block; font-size: .9rem; margin-bottom: .4rem; color: #aaa; }
  input {
    width: 100%%; padding: .65rem .8rem; border-radius: 8px;
    border: 1px solid #333; background: #0f3460; color: #eee;
    font-size: 1rem; margin-bottom: 1rem; outline: none;
  }
  input:focus { border-color: #e94560; }
  button {
    width: 100%%; padding: .7rem; border-radius: 8px; border: none;
    background: #e94560; color: #fff; font-size: 1rem; font-weight: 600;
    cursor: pointer; transition: background .2s;
  }
  button:hover { background: #c73e54; }
  button:disabled { background: #555; cursor: not-allowed; }
  .error { color: #e94560; font-size: .85rem; margin-top: .5rem; }
  .success { color: #4ec94e; font-size: .85rem; margin-top: .5rem; }
  .branding { text-align: center; margin-top: 1.5rem; color: #555; font-size: .75rem; }
  .branding a { color: #888; text-decoration: none; }
</style>
</head>
<body>
<div class="card">
  <h1>Join Call</h1>
  <p class="subtitle">%s</p>
  <form id="joinForm">
    <label for="name">Your name</label>
    <input id="name" type="text" placeholder="Enter your display name" maxlength="50" required autofocus>
    <button type="submit" id="joinBtn">Join Call</button>
    <div id="msg"></div>
  </form>
  <div class="branding">Powered by <a href="https://github.com/LaPingvino/wally-conference">Wally Conference</a></div>
</div>
<script>
const roomId = %q;
const form = document.getElementById('joinForm');
const nameInput = document.getElementById('name');
const btn = document.getElementById('joinBtn');
const msg = document.getElementById('msg');

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  const displayName = nameInput.value.trim();
  if (!displayName) return;
  btn.disabled = true;
  btn.textContent = 'Joining...';
  msg.textContent = '';
  msg.className = '';
  try {
    const resp = await fetch('./join', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({room_id: roomId, display_name: displayName}),
    });
    const data = await resp.json();
    if (!resp.ok) {
      throw new Error(data.error || 'Join failed');
    }
    if (data.ec_url) {
      msg.className = 'success';
      msg.textContent = 'Joining call...';
      window.location.href = data.ec_url;
    } else {
      throw new Error('No call URL returned');
    }
  } catch (err) {
    msg.className = 'error';
    msg.textContent = err.message;
    btn.disabled = false;
    btn.textContent = 'Join Call';
  }
});
</script>
</body>
</html>`

// HandleGuestPage serves a self-contained HTML guest join page.
func (svc *Service) HandleGuestPage(w http.ResponseWriter, r *http.Request) {
	rawRoomID := r.PathValue("roomID")
	if rawRoomID == "" {
		http.Error(w, "Missing room ID", http.StatusBadRequest)
		return
	}
	roomID, err := url.PathUnescape(rawRoomID)
	if err != nil {
		roomID = rawRoomID
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, guestPageTemplate, html.EscapeString(roomID), roomID)
}
