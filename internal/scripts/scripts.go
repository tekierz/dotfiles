package scripts

// HKScript is the hotkey reference script that delegates to dotfiles hotkeys
const HKScript = `#!/usr/bin/env bash
#
# hk - Hotkey reference
#
# Prefer the new TUI (dotfiles hotkeys) when available; fall back to the static
# table below for environments without the Go binary.
#

tool="${1:-}"
if command -v dotfiles >/dev/null 2>&1; then
  if [[ -n "$tool" ]]; then
    dotfiles hotkeys --skip-intro --tool "$tool" && exit 0
  else
    dotfiles hotkeys --skip-intro && exit 0
  fi
fi

cat << 'EOF'
╔══════════════════════════════════════════════════════════════════════════════╗
║                      HOTKEY REFERENCE (emacs/Mac style)                      ║
╚══════════════════════════════════════════════════════════════════════════════╝

┌─────────────────────────────────────────────────────────────────────────────┐
│ TMUX (prefix = Ctrl-a)                                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│ Ctrl-a |         Split vertical          Alt-Arrow      Navigate panes     │
│ Ctrl-a -         Split horizontal        Alt-1/2/3/4/5  Switch window      │
│ Ctrl-a c         New window              Ctrl-a d       Detach             │
│ Ctrl-a Arrow     Navigate panes          Ctrl-a r       Reload config      │
│ Ctrl-a S-Arrow   Resize panes            Ctrl-a [       Copy mode          │
│ Ctrl-a n/p       Next/prev window                                          │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ ZSH (emacs/Mac-style)                                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│ Ctrl-a/e         Start/end of line       Ctrl-p/n       History up/down    │
│ Ctrl-x Ctrl-e    Edit command in nvim    Ctrl-w         Delete word back   │
│ Ctrl-r           Search history          Ctrl-f         Accept suggestion  │
│ Alt-b/f          Word back/forward       Ctrl-k         Kill to end        │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ YAZI (file manager, launch with: y)                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│ Arrow keys       Navigate                Ctrl-h         Toggle hidden      │
│ Shift-Up/Down    Move 5 lines            Ctrl-f         Search             │
│ Home/End         Top/bottom              PageUp/Down    Half page          │
│ Space            Toggle select           Ctrl-a         Select all         │
│ Enter            Open                    Ctrl-n         New file/dir       │
│ Ctrl-c           Copy                    F2             Rename             │
│ Ctrl-x           Cut                     Delete         Trash              │
│ Ctrl-v           Paste                   Shift-Delete   Delete permanent   │
│ q                Quit (cd to dir)        Q              Quit (stay)        │
│ Alt-h            Go home                 Alt-d          Go ~/Downloads     │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ FZF (fuzzy finder)                                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│ Ctrl-r           Fuzzy search history    Ctrl-t         Fuzzy find file    │
│ Alt-c            Fuzzy cd into dir       **<Tab>        Path completion    │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ GHOSTTY                                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│ Super-c/v        Copy/Paste (macOS-style) Super-1/2/3... Switch to tab N   │
│ Super-{/}        Prev/next tab           Ctrl-Shift-n   New window         │
│ Ctrl-Shift-,     Reload config           Ctrl-Shift-c/v Alternate copy/paste│
│ Cmd-c/v          Copy/Paste (macOS)      Cmd-1/2/3...   Switch tab (macOS) │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ NVIM (basics)                                                               │
├─────────────────────────────────────────────────────────────────────────────┤
│ i                Insert mode             Esc            Normal mode        │
│ :w               Save                    :q             Quit               │
│ :wq              Save and quit           :q!            Quit no save       │
│ Arrow keys       Navigate                dd             Delete line        │
│ yy               Yank line               p              Paste              │
│ u                Undo                    Ctrl-r         Redo               │
│ /                Search                  n/N            Next/prev match    │
│ Space            Leader key (menu)       :Tutor         Built-in tutorial  │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ SSHH (quick SSH connect)                                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│ sshh             Show menu & select      sshh 1/2/3     Connect directly   │
│ sshh list        List all hosts          sshh edit      Edit ~/.sshh       │
│ sshh add "Name" "user@host" [port]       Add new host                      │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ CAFF (caffeine - prevent sleep)                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ caff             Toggle on/off           caff status    Check state        │
│ caff on          Keep awake              caff off       Allow sleep        │
└─────────────────────────────────────────────────────────────────────────────┘
EOF
`

// CaffScript is the caffeine script to keep the system awake
const CaffScript = `#!/usr/bin/env bash

PIDFILE="/tmp/caffeine-$USER.pid"

status() {
    if [[ -f "$PIDFILE" ]] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "☕ Caffeine: ON (PID $(cat "$PIDFILE"))"
        return 0
    else
        echo "😴 Caffeine: OFF"
        rm -f "$PIDFILE" 2>/dev/null
        return 1
    fi
}

start() {
    if status > /dev/null 2>&1; then
        echo "☕ Caffeine is already running"
        return 0
    fi

    if [[ "$OSTYPE" == "darwin"* ]]; then
        caffeinate -di &
    else
        systemd-inhibit --what=idle:sleep:handle-lid-switch \
            --who="caffeine" \
            --why="User requested stay awake" \
            --mode=block \
            sleep infinity &
    fi

    echo $! > "$PIDFILE"
    echo "☕ Caffeine: ON - system will stay awake"
}

stop() {
    if [[ -f "$PIDFILE" ]]; then
        pid=$(cat "$PIDFILE")
        if kill "$pid" 2>/dev/null; then
            rm -f "$PIDFILE"
            echo "😴 Caffeine: OFF - sleep enabled"
            return 0
        fi
    fi
    echo "😴 Caffeine is not running"
    rm -f "$PIDFILE" 2>/dev/null
}

toggle() {
    if status > /dev/null 2>&1; then
        stop
    else
        start
    fi
}

case "${1:-toggle}" in
    on|start) start ;;
    off|stop) stop ;;
    status) status ;;
    toggle|"") toggle ;;
    *)
        echo "Usage: caff [on|off|status|toggle]"
        exit 1
        ;;
esac
`

// SSHHScript is the SSH quick connect manager
const SSHHScript = `#!/usr/bin/env bash
#
# sshh - Quick SSH connection manager
# Reads hosts from ~/.sshh and provides a simple menu
#

CONFIG_FILE="$HOME/.sshh"

CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
PURPLE='\033[0;35m'
RED='\033[0;31m'
DIM='\033[2m'
NC='\033[0m'

if [[ ! -f "$CONFIG_FILE" ]]; then
    echo -e "${RED}✗${NC} Config file not found: ${CYAN}$CONFIG_FILE${NC}"
    echo ""
    echo "Create it with format: name | user@host | port (port optional)"
    echo -e "Example: ${CYAN}Work Server${NC} | ${GREEN}admin@192.168.1.100${NC}"
    exit 1
fi

declare -a NAMES CONNECTIONS PORTS

while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    IFS='|' read -ra PARTS <<< "$line"
    name=$(echo "${PARTS[0]}" | xargs)
    conn=$(echo "${PARTS[1]}" | xargs)
    port=$(echo "${PARTS[2]:-22}" | xargs)
    if [[ -n "$name" && -n "$conn" ]]; then
        NAMES+=("$name")
        CONNECTIONS+=("$conn")
        PORTS+=("$port")
    fi
done < "$CONFIG_FILE"

if [[ ${#NAMES[@]} -eq 0 ]]; then
    echo -e "${RED}✗${NC} No hosts found in ${CYAN}$CONFIG_FILE${NC}"
    exit 1
fi

show_menu() {
    echo ""
    echo -e "${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${PURPLE}  SSH Quick Connect${NC}"
    echo -e "${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    for i in "${!NAMES[@]}"; do
        num=$((i + 1))
        port_display=""
        [[ "${PORTS[$i]}" != "22" ]] && port_display="${DIM}:${PORTS[$i]}${NC}"
        printf "  ${YELLOW}%d${NC}  ${CYAN}%-20s${NC} ${GREEN}%s${NC}%s\n" \
            "$num" "${NAMES[$i]}" "${CONNECTIONS[$i]}" "$port_display"
    done
    echo ""
    echo -e "  ${DIM}q  Quit${NC}"
    echo ""
    echo -e "${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

if [[ -n "$1" ]]; then
    if [[ "$1" =~ ^[0-9]+$ ]]; then
        idx=$(($1 - 1))
        if [[ $idx -ge 0 && $idx -lt ${#NAMES[@]} ]]; then
            echo -e "${GREEN}▶${NC} Connecting to ${CYAN}${NAMES[$idx]}${NC}..."
            [[ "${PORTS[$idx]}" != "22" ]] && exec ssh -p "${PORTS[$idx]}" "${CONNECTIONS[$idx]}"
            exec ssh "${CONNECTIONS[$idx]}"
        fi
        echo -e "${RED}✗${NC} Invalid selection: $1" && exit 1
    elif [[ "$1" == "list" || "$1" == "-l" ]]; then
        show_menu && exit 0
    elif [[ "$1" == "edit" || "$1" == "-e" ]]; then
        ${EDITOR:-nvim} "$CONFIG_FILE" && exit 0
    elif [[ "$1" == "add" ]]; then
        shift
        if [[ $# -ge 2 ]]; then
            echo "$1 | $2 | ${3:-22}" >> "$CONFIG_FILE"
            echo -e "${GREEN}✓${NC} Added: ${CYAN}$1${NC} → ${GREEN}$2${NC}"
            exit 0
        fi
        echo "Usage: sshh add \"Name\" \"user@host\" [port]" && exit 1
    elif [[ "$1" == "help" || "$1" == "-h" ]]; then
        echo "Usage: sshh [command|number]"
        echo "  (none)    Show menu"
        echo "  <num>     Connect to host #"
        echo "  list      Show hosts"
        echo "  edit      Edit config"
        echo "  add       Add host"
        exit 0
    fi
    echo -e "${RED}✗${NC} Unknown: $1" && exit 1
fi

show_menu
echo -ne "Select [1-${#NAMES[@]}]: "
read -r choice
[[ "$choice" == "q" || "$choice" == "Q" ]] && exit 0
if [[ "$choice" =~ ^[0-9]+$ ]]; then
    idx=$((choice - 1))
    if [[ $idx -ge 0 && $idx -lt ${#NAMES[@]} ]]; then
        echo -e "\n${GREEN}▶${NC} Connecting to ${CYAN}${NAMES[$idx]}${NC}...\n"
        [[ "${PORTS[$idx]}" != "22" ]] && exec ssh -p "${PORTS[$idx]}" "${CONNECTIONS[$idx]}"
        exec ssh "${CONNECTIONS[$idx]}"
    fi
fi
echo -e "${RED}✗${NC} Invalid selection" && exit 1
`

// GetScript returns the script content for a given utility name
func GetScript(name string) string {
	switch name {
	case "hk":
		return HKScript
	case "caff":
		return CaffScript
	case "sshh":
		return SSHHScript
	default:
		return ""
	}
}
