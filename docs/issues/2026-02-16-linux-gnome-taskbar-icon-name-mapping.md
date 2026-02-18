# Linux GNOME Taskbar Icon/Name Mapping Fails in Dev Runs (2026-02-16)

Source runs:
- Reproduction run: `make run` (local, Ubuntu 24.04 GNOME/X11, 2026-02-16)
- Follow-up run: `make clean` then `make run` (same host/session)
- Branch: `chore/replace-iconset-keenbench-logo-v2`

## Issue 1: Dock/taskbar does not show KeenBench icon/name during Linux dev run

- Status: Open
- Severity: Medium (desktop integration regression in Linux dev workflow)
- Area: Linux GTK runner / GNOME shell mapping
- Expected: Ubuntu GNOME dock shows the KeenBench icon and app name `KeenBench`.
- Actual: Dock/taskbar icon remains missing/default; hover label shows `Com.keenbench.app`.

Evidence:
- User reproduction:
  - `make run` -> no icon in taskbar, hover label `Com.keenbench.app`.
  - `make clean` then `make run` -> same result.
- Local desktop entry content was present and valid fields were populated:
  - `~/.local/share/applications/com.keenbench.app.desktop`
  - `Name=KeenBench`
  - `Icon=keenbench`
- Running window metadata (captured with `wmctrl -lx`):
  - `com.keenbench.app.Com.keenbench.app ... KeenBench`
- Window icon payload probe (captured with `xprop`):
  - `_NET_WM_ICON: not found`

Notes:
- This issue is currently unresolved for `make run`/`flutter run -d linux` on Ubuntu GNOME.
- We are documenting and proceeding; no further Linux icon/name debugging is in scope for this pass.
