# SPEC: Sessions Plugin (built-in)

## Purpose

Manage "New Session" and "Resume Session" functionality as a plugin. Users can browse project paths, launch new Claude sessions, and resume bookmarked sessions.

## Slug: `sessions`

## Routes

- `sessions` — default view (new session list)
- `sessions/resume` — resume tab sub-view

## State

- newList, resumeList (bubbles/list.Model)
- paths []string
- confirming, confirmYes bool
- confirmItem, confirmResume
- loading bool, spinner
- sub-tab: "new" or "resume"

## Key Bindings

| Key | Mode | Description | Promoted |
|-----|------|-------------|----------|
| enter | normal | Launch session | yes |
| del/backspace | normal | Remove from list | yes |
| up/down | normal | Navigate list | yes |
| / | normal | Filter list | yes |
| n | normal | Switch to New sub-tab | yes |
| r | normal | Switch to Resume sub-tab | yes |
| y | confirming | Confirm delete | no |
| n | confirming | Cancel delete | no |

## Event Bus

- Publishes: `project.selected` with {path, prompt} when user picks a project
- Subscribes: `session.launch` to trigger launches from other plugins

## Migrations

None — uses existing cc_bookmarks and cc_learned_paths tables.

## Behavior

1. On Init, loads paths from DB and sessions from DB
2. New sub-tab shows project paths + Browse option
3. Resume sub-tab shows bookmarked sessions
4. Enter on a path launches Claude in that directory
5. Enter on a session resumes that Claude session
6. Delete/backspace shows confirmation dialog
7. When pendingLaunchTodo is set (via event bus), shows banner "Select project for: <title>"

## Test Cases

- Init loads paths and sessions
- HandleKey "enter" on path sets Launch action
- HandleKey "enter" on session sets Launch with resume args
- HandleKey "delete" enters confirming mode
- Confirming "y" removes item
- Sub-tab switching works
