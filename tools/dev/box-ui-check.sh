#!/bin/bash
# An interactive check of the installed gnosh on an AI box, with the box's
# model (Qwen alone: the router holds one model), in a tmux session of its
# own and a scratch home, with a status line command configured: the screen
# before and after one scenario. It touches no file of the person's, and
# ends its own tmux session by name. Run it on the box:
#
#	ssh ai-remote bash -s < tools/dev/box-ui-check.sh            # basic
#	ssh ai-remote bash -s -- plan < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- background < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- btw < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- mention < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- config < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- hookwake < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- agent < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- fork < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- monitor < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- worktree < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- panel < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- suggest < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- invisible < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- failhint < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- cache < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- hooks < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- leftover < tools/dev/box-ui-check.sh
#	ssh ai-remote bash -s -- focus < tools/dev/box-ui-check.sh
#
#   basic       one short turn
#   plan        plan mode: the model presents a plan with ExitPlanMode, and
#               the question is refused (nothing is written)
#   background  a sub-agent in the background, whose report wakes the
#               session (permissions skipped, in the scratch folder)
#   btw         a side question (/btw) asked while a slow command runs:
#               answered beside the turn, which goes on (permissions
#               skipped, in the scratch folder)
#   mention     a file mentioned with @: read in front of the model with
#               the prompt, and said so under it
#   config      /config lists the options, sets one in the scratch state
#               file and says it is in force from the next start (no model)
#   hookwake    a Stop hook in the background ("asyncRewake") exits with 2
#               after the first turn and wakes the model, which answers it
#   agent       the session runs as the Explore agent (--agent): its prompt
#               and its read-only tools
#   fork        a skill with context: fork, typed: a sub-agent counts the
#               lines of notes.txt, and the model answers its report
#   monitor     the model watches a command with Monitor: its lines come as
#               notes, then the news that it ended (permissions skipped, in
#               the scratch folder)
#   panel       /panel takes the whole screen: the model writes hello.txt,
#               and the panel lists it under the files changed (permissions
#               skipped, in the scratch folder)
#   worktree    the scratch folder is a git repository, trusted: a sub-agent
#               with isolation worktree adds a line to notes.txt in a work
#               tree of its own, and the folder's notes.txt stays as it was
#               (acceptEdits)
#   wtflag      the same repository, and gnosh started with --worktree demo:
#               the session adds a line to notes.txt in .gnosh/worktrees/demo,
#               and the folder's notes.txt stays as it was (acceptEdits); the
#               ignored .env that .worktreeinclude names is copied there
#   secreview   a branch that adds a handler with a shell injection:
#               /security-review should find it
#   vim         editorMode vim: a prompt typed in INSERT mode, Esc, x in
#               NORMAL mode, and Enter sends it from NORMAL mode
#   restart     a copy of the installed gnosh, replaced by rename after the
#               first turn as an install does it: the next turn's end says a
#               newer gnosh is installed, /restart starts it in the same
#               session, and the model still knows the first turn
#   keys        a keybindings.json: Ctrl+K stashes the box and brings it
#               back, Ctrl+Y runs /cost, and the prompt then goes out
#   bgsession   /background: the session ends in the pane and goes on as a
#               run of its own, whose log gets the model's answer; once the
#               run lets go of it, gnosh -r opens it again and shows where
#               it left off
#   suggest     promptSuggestionEnabled: after the answer, the model's
#               guess at the next prompt is in the box, and Tab takes it
#   invisible   a prompt with a zero-width space in it goes without it, and
#               the screen says so
#   failhint    the model's server is a closed port: the turn fails after
#               gnosh's tries, and the empty prompt box says what to try
#               next (no model)
#   cache       three short turns, then /cost: how much of what was read
#               came from the model server's prompt cache, and any request
#               that read again what the one before it had sent
#   hooks       /hooks browses the hooks in force: the moments, a moment's
#               hooks, one hook in full; Esc closes it (no model)
#   leftover    the model starts a process that lets go of its output
#               (sleep 321 >/dev/null 2>&1 &); the result says it was left
#               running, and /exit stops it (permissions skipped)
#   relay       a channel that relays permission questions (python3
#               tools/dev/channel-webhook.py, copied to ~/newgnosh-test):
#               the question about a Write goes to it, and "yes <id>"
#               posted to it answers the question at the terminal
#   focus       focus view (viewMode "focus"): the model reads notes.txt and
#               counts its lines with a command, and the two calls are one
#               line, "Read 1 file, ran 1 command"; /focus turns it off
#   cd          a turn, then /cd to a folder of its own whose settings wait
#               for trust (said yes to) and whose GNOSH.md holds a codeword:
#               gnosh starts again there, and the model is asked where it
#               is, for the codeword, and to write here.txt (permissions
#               skipped)
#
# The full interface is tested in CI with a scripted model; this is the
# look at it with a real one, which is how a model writing gnosh's tags
# into its own reply was found.
set -u
scenario=${1:-basic}
T=$HOME/newgnosh-test/uicheck-$scenario
S=gnosh-uicheck-$scenario
flags=""
hooks=""
opts=""
base=http://127.0.0.1:8080
case "$scenario" in
basic | mention | config | fork | model | transcript | restart | keys | bgsession | invisible | cache | cron | selfloop) ;;
vim) opts='"editorMode": "vim", ' ;;
suggest) opts='"promptSuggestionEnabled": true, ' ;;
focus) opts='"viewMode": "focus", ' ;;
failhint) base=http://127.0.0.1:9 ;;
secreview) flags="--trust-folder" ;;
agent) flags="--agent Explore" ;;
plan) flags="--permission-mode plan" ;;
background | btw | monitor | panel | cd | leftover) flags="-y" ;;
worktree) flags="--permission-mode acceptEdits --trust-folder" ;;
channel) flags="-y --channels server:hook" ;;
relay) flags="--channels server:hook" ;;
entertree) flags="--trust-folder" ;;
wtflag) flags="--permission-mode acceptEdits --trust-folder --worktree demo" ;;
hookwake) hooks="\"hooks\": { \"Stop\": [ { \"hooks\": [ { \"type\": \"command\", \"asyncRewake\": true, \"command\": \"$T/config/gnosh/wake.sh\" } ] } ] }," ;;
hooks) hooks="\"hooks\": { \"PreToolUse\": [ { \"matcher\": \"Bash\", \"hooks\": [ { \"type\": \"command\", \"command\": \"true\", \"if\": \"Bash(rm *)\", \"timeout\": 5 } ] } ], \"Stop\": [ { \"hooks\": [ { \"type\": \"command\", \"command\": \"true\" } ] } ] }," ;;
*)
	echo "box-ui-check: no scenario $scenario (basic, plan, background, btw, mention, config, hookwake, hooks, leftover, agent, fork, monitor, worktree, wtflag, entertree, cron, selfloop, channel, relay, focus, panel, model, transcript, vim, secreview, restart, keys, bgsession, suggest, invisible, failhint, cache, cd)" >&2
	exit 2
	;;
esac
tmux kill-session -t "$S" 2>/dev/null
rm -rf "$T"
mkdir -p "$T/home" "$T/config/gnosh" "$T/data" "$T/cache" "$T/work"
cat > "$T/config/gnosh/statusline.sh" <<'EOF'
#!/bin/sh
in=$(cat)
model=$(printf '%s' "$in" | sed -n 's/.*"model":{"id":"\([^"]*\)".*/\1/p')
pct=$(printf '%s' "$in" | sed -n 's/.*"used_percentage":\([0-9]*\).*/\1/p')
printf 'SL %s ctx=%s%% cols=%s\n' "$model" "${pct:-?}" "$COLUMNS"
EOF
chmod 755 "$T/config/gnosh/statusline.sh"
cat > "$T/config/gnosh/wake.sh" <<EOF
#!/bin/sh
cat > /dev/null
[ -e "$T/woke" ] && exit 0
touch "$T/woke"
echo 'HOOK-WAKE: the build check failed. Reply with the one word pineapple and nothing else.' >&2
exit 2
EOF
chmod 755 "$T/config/gnosh/wake.sh"
cat > "$T/config/gnosh/gnosh.json" <<EOF
{
  "providers": { "qwen": { "type": "llamacpp", "baseUrl": "$base", "listModels": false } },
  "models": { "qwen/qwen3.8-27b": { "name": "Qwen 3.8 27B", "contextWindow": 262144 } },
  "model": { "id": "qwen/qwen3.8-27b", "thinking": false },
  "options": { $opts"requestTimeout": 900 },
  $hooks
  "statusLine": { "type": "command", "command": "$T/config/gnosh/statusline.sh", "padding": 1 }
}
EOF
printf 'alpha\nbeta\ngamma\n' > "$T/work/notes.txt"
if [ "$scenario" = channel ] || [ "$scenario" = relay ]; then
	# The webhook channel (tools/dev/channel-webhook.py), copied to
	# ~/newgnosh-test before this runs; for relay it takes the permission
	# questions too, and writes them to questions.log.
	[ -f "$HOME/newgnosh-test/channel-webhook.py" ] || { echo "box-ui-check: copy tools/dev/channel-webhook.py to ~/newgnosh-test first" >&2; exit 2; }
	asked=""
	[ "$scenario" = relay ] && asked=", \"$T/questions.log\""
	printf '{"mcpServers": {"hook": {"command": "python3", "args": ["%s", "18788", "%s"%s]}}}\n' "$HOME/newgnosh-test/channel-webhook.py" "$T/replies.log" "$asked" > "$T/config/gnosh/mcp.json"
fi
if [ "$scenario" = keys ]; then
	cat > "$T/config/gnosh/keybindings.json" <<'EOF'
{"bindings": [{"context": "Chat", "bindings": {"ctrl+k": "chat:stash", "ctrl+y": "command:cost"}}]}
EOF
fi
# The skill only where it is used: it is about notes.txt, and a model
# asked to change that file in another scenario loads it.
if [ "$scenario" = fork ]; then
	mkdir -p "$T/config/gnosh/skills/countlines"
	cat > "$T/config/gnosh/skills/countlines/SKILL.md" <<'EOF'
---
description: Counts the lines of a file in a sub-agent.
context: fork
---
Count the lines of the file notes.txt in the working directory. Report the count as one number.
EOF
fi
cd "$T/work" || exit 1
if [ "$scenario" = worktree ] || [ "$scenario" = wtflag ] || [ "$scenario" = entertree ]; then
	git init -q && git -c user.name=check -c user.email=check@example.com add notes.txt &&
		git -c user.name=check -c user.email=check@example.com commit -q -m notes || exit 1
fi
if [ "$scenario" = wtflag ]; then
	# What a checkout lacks, which .worktreeinclude has copied in.
	printf '.env\n' > .gitignore && printf '.env\n' > .worktreeinclude && printf 'TOKEN=kz-check\n' > .env &&
		git -c user.name=check -c user.email=check@example.com add .gitignore .worktreeinclude &&
		git -c user.name=check -c user.email=check@example.com commit -q -m include || exit 1
fi
if [ "$scenario" = secreview ]; then
	# A branch that adds a handler putting a query parameter into a shell
	# command line: the one real way in for the review to find.
	g() { git -c user.name=check -c user.email=check@example.com "$@"; }
	printf 'module thumbs\n\ngo 1.22\n' > go.mod
	g init -q -b main && g add go.mod notes.txt && g commit -q -m base || exit 1
	g checkout -q -b thumbnails || exit 1
	cat > handler.go <<'EOF'
package thumbs

import (
	"net/http"
	"os/exec"
)

// Thumb makes a thumbnail of the file named in the request.
func Thumb(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("file")
	out, err := exec.Command("sh", "-c", "convert uploads/"+name+" -resize 64x64 png:-").Output()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(out)
}
EOF
	g add handler.go && g commit -q -m "thumbnails" || exit 1
fi
bin=$HOME/.local/bin/gnosh
if [ "$scenario" = restart ]; then
	# A copy of its own, so that "installing" over it touches nothing else.
	mkdir -p "$T/bin" && cp "$bin" "$T/bin/gnosh" || exit 1
	bin=$T/bin/gnosh
fi
envs="env -i HOME=$T/home XDG_CONFIG_HOME=$T/config XDG_DATA_HOME=$T/data XDG_CACHE_HOME=$T/cache PATH=/usr/local/bin:/usr/bin:/bin TERM=tmux-256color LANG=C.UTF-8"
tmux new-session -d -s "$S" -x 110 -y 40 "$envs $bin $flags"
sleep 4
echo "=== before"
tmux capture-pane -p -t "$S"
screen() { tmux capture-pane -p -S -80 -t "$S"; }
case "$scenario" in
basic)
	tmux send-keys -t "$S" "Reply with exactly these three words: all is well" Enter
	done_when() { [ "$(screen | grep -c -i "all is well")" -ge 2 ] && screen | grep -q "ctx=[0-9]"; }
	;;
plan)
	tmux send-keys -t "$S" "Create a file hello.txt that says hi. Plan it first, then present the plan for approval." Enter
	done_when() { screen | grep -q "Carry out this plan"; }
	;;
background)
	tmux send-keys -t "$S" "Call the Agent tool with run_in_background set to true, to have a sub-agent count the lines of notes.txt. Then end your turn; when its report comes, tell me the count." Enter
	done_when() { screen | grep -q "background sub-agent finished" && [ "$(screen | grep -c "done in")" -ge 2 ]; }
	;;
focus)
	tmux send-keys -t "$S" "Read notes.txt with the Read tool, then count its lines with the Bash tool running wc -l notes.txt, then say the count in one line." Enter
	for i in $(seq 1 300); do
		sleep 1
		screen | grep -q "done in" && break
	done
	tmux send-keys -t "$S" "/focus" Enter
	done_when() { screen | grep -q "Read 1 file, ran 1 command" && screen | grep -q 'viewMode is "\{0,1\}default"\{0,1\} from now on'; }
	;;
mention)
	tmux send-keys -t "$S" "What is the second line of @notes.txt? Answer with that one word and call no tool." Enter
	done_when() { screen | grep -q "@notes.txt read" && screen | grep -q "done in"; }
	;;
leftover)
	tmux set-option -t "$S" remain-on-exit on >/dev/null
	tmux send-keys -t "$S" "Run exactly this command with the Bash tool, and nothing else: sleep 321 >/dev/null 2>&1 & echo started" Enter
	done_when() { screen | grep -q "let go of its output" && screen | grep -q "done in"; }
	;;
hooks)
	# The browser: the moments, PreToolUse's hooks, the hook in full.
	tmux send-keys -t "$S" "/hooks" Enter
	sleep 2
	screen | grep -q "The hooks in force" || echo "box-ui-check: no browser"
	tmux send-keys -t "$S" Enter
	sleep 1
	tmux send-keys -t "$S" Enter
	done_when() { screen | grep -q "A PreToolUse hook." && screen | grep -q "Only if: Bash(rm \*)" && screen | grep -q "From: "; }
	;;
config)
	# Bare /config opens the picker of the options; Esc closes it.
	tmux send-keys -t "$S" "/config" Enter
	sleep 2
	tmux send-keys -t "$S" Escape
	sleep 1
	tmux send-keys -t "$S" "/config memoryText=full" Enter
	sleep 2
	tmux send-keys -t "$S" "/config memoryText" Enter
	done_when() { screen | grep -q "set with /config to full: in force when gnosh next starts"; }
	;;
agent)
	tmux send-keys -t "$S" "Which files are in this folder? Answer in one line." Enter
	done_when() { screen | grep -q "notes.txt" && screen | grep -q "done in"; }
	;;
fork)
	tmux send-keys -t "$S" "/countlines" Enter
	done_when() { screen | grep -q "ran in a sub-agent" && [ "$(screen | grep -c "done in")" -ge 1 ]; }
	;;
model)
	# The picker only: Esc keeps the model, so the router swaps nothing.
	tmux send-keys -t "$S" "/model" Enter
	done_when() { screen | grep -q "Which model?"; }
	;;
transcript)
	tmux send-keys -t "$S" "Reply with exactly these three words: all is well" Enter
	for i in $(seq 1 300); do
		sleep 1
		screen | grep -q "done in" && break
	done
	tmux send-keys -t "$S" C-o
	done_when() { screen | grep -q "transcript ·"; }
	;;
panel)
	tmux send-keys -t "$S" "/panel" Enter
	sleep 2
	tmux send-keys -t "$S" "Create the file hello.txt that says hi, with the Write tool, and say done." Enter
	done_when() { screen | grep -q "Files changed" && screen | grep -q "hello.txt +1" && screen | grep -q "done in"; }
	;;
worktree)
	tmux send-keys -t "$S" "Call the Agent tool with isolation set to worktree, to have a sub-agent add the line delta at the end of notes.txt. Then tell me where its change is." Enter
	# (The model words it "work tree" or "worktree"; the files say the rest.)
	done_when() { screen | grep -q "done in"; }
	;;
wtflag)
	tmux send-keys -t "$S" "Add the line delta at the end of notes.txt, then say where notes.txt is." Enter
	done_when() { screen | grep -q "done in"; }
	;;
relay)
	# The question about a Write is answered from the channel ("yes <id>"),
	# while it stands open at the terminal too.
	tmux send-keys -t "$S" "Create the file relay.txt that says hi, with the Write tool, then say done." Enter
	for i in $(seq 1 300); do
		sleep 1
		[ -s "$T/questions.log" ] && break
	done
	id=$(head -c 5 "$T/questions.log" 2>/dev/null)
	echo "=== the question the channel got: $(head -1 "$T/questions.log" 2>/dev/null || echo none)"
	curl -s -d "yes $id" 127.0.0.1:18788 >/dev/null
	done_when() { [ -f "$T/work/relay.txt" ] && screen | grep -q "allowed Write" && screen | grep -q "done in"; }
	;;
channel)
	# An event pushed into the idle session wakes the model, which answers
	# through the channel's reply tool.
	for i in $(seq 1 30); do
		sleep 1
		screen | grep -q "Channels: events from server:hook" && break
	done
	sleep 3
	curl -s -d "The build of main failed. Answer on the channel with exactly the word: seen" 127.0.0.1:18788 >/dev/null
	done_when() { [ -s "$T/replies.log" ] && screen | grep -q "done in"; }
	;;
cron)
	# /loop 1m: the prompt runs at once and again each minute, between
	# turns; asked, the model cancels the task with CronDelete.
	tmux send-keys -t "$S" "/loop 1m Reply with exactly one word: tick" Enter
	for i in $(seq 1 240); do
		sleep 1
		[ "$(screen | grep -c "Scheduled task .* runs")" -ge 2 ] && [ "$(screen | grep -c "done in")" -ge 3 ] && break
	done
	echo "=== after two scheduled runs"
	screen | grep -v '^ *$' | tail -14
	tmux send-keys -t "$S" "Cancel the scheduled tick task." Enter
	done_when() { [ "$(screen | grep -c "done in")" -ge 4 ] && screen | grep -q "CronDelete"; }
	;;
selfloop)
	# /loop <prompt> with no interval: the model schedules each next run
	# (ScheduleWakeup) and stops the loop when told the task is done.
	tmux send-keys -t "$S" "/loop Reply with exactly one word, tock, then schedule the next run in one minute. On the third run, end the loop instead." Enter
	done_when() { screen | grep -q "self-paced /loop is over"; }
	;;
entertree)
	# EnterWorktree: gnosh starts again in the tree, in this pane, and the
	# model goes on there by itself; ExitWorktree takes it back.
	tmux send-keys -t "$S" "Work in a worktree named demo. Once you are in it, run pwd and tell me the directory." Enter
	for i in $(seq 1 300); do
		sleep 1
		[ -d "$T/work/.gnosh/worktrees/demo" ] && screen | grep -q "worktrees/demo" && screen | grep -q "done in" && break
	done
	echo "=== in the tree"
	screen | grep -v '^ *$' | tail -16
	tmux send-keys -t "$S" "Leave the worktree and remove it." Enter
	done_when() { [ ! -d "$T/work/.gnosh/worktrees/demo" ] && [ "$(screen | grep -c "done in")" -ge 1 ] && screen | grep -q "goes back to"; }
	;;
secreview)
	tmux send-keys -t "$S" "/security-review" Enter
	done_when() { screen | grep -q "done in"; }
	;;
vim)
	# Typed in INSERT mode, Esc to NORMAL (the status line says so), x takes
	# the stray last letter, and Enter sends from NORMAL mode; the empty box
	# is in INSERT mode again.
	tmux send-keys -t "$S" "Reply with exactly these three words: all is wellX" Escape
	sleep 1
	echo "=== after Esc: $(screen | grep -E -o -- "-- (NORMAL|INSERT|VISUAL) --" | tail -1)"
	tmux send-keys -t "$S" x
	sleep 1
	tmux send-keys -t "$S" Enter
	done_when() { [ "$(screen | grep -c -i "all is well")" -ge 2 ] && screen | grep -q "done in" && screen | grep -q -- "-- INSERT --"; }
	;;
monitor)
	tmux send-keys -t "$S" "Use the Monitor tool to watch this exact command: for i in 1 2 3; do echo tick \$i; sleep 3; done; echo MONITOR-DONE. Then end your turn. When its lines come, say only the last line you were given." Enter
	done_when() { screen | grep -q "passed on" && screen | grep -q "background command ended" && [ "$(screen | grep -c "done in")" -ge 2 ]; }
	;;
restart)
	tmux send-keys -t "$S" "Reply with exactly these three words: all is well" Enter
	for i in $(seq 1 300); do
		sleep 1
		screen | grep -q "done in" && break
	done
	cp "$bin" "$bin.new" && mv "$bin.new" "$bin"
	tmux send-keys -t "$S" "Reply with exactly the one word ready, and call no tool." Enter
	for i in $(seq 1 300); do
		sleep 1
		screen | grep -q "A newer gnosh has been installed" && break
	done
	echo "=== the newer gnosh said: $(screen | grep -c "A newer gnosh has been installed") time(s)"
	tmux send-keys -t "$S" "/restart" Enter
	sleep 8
	echo "=== after /restart"
	screen | tail -12
	tmux send-keys -t "$S" "What were the three words I first asked you to reply with? Answer with only those three words and call no tool." Enter
	done_when() { [ "$(screen | grep -c -i "all is well")" -ge 3 ] && [ "$(screen | grep -c "done in")" -ge 3 ]; }
	;;
cd)
	mkdir -p "$T/lib"
	echo 'In this project the codeword is PERSIMMON. Say it when asked for the codeword.' > "$T/lib/GNOSH.md"
	echo '{ "permissions": { "allow": ["Bash(ls *)"] } }' > "$T/lib/gnosh.json"
	tmux send-keys -t "$S" "Reply with exactly these three words: all is well" Enter
	for i in $(seq 1 300); do
		sleep 1
		screen | grep -q "done in" && break
	done
	tmux send-keys -t "$S" "/cd ../lib" Enter
	for i in $(seq 1 30); do
		sleep 1
		screen | grep -q "Trust .*/lib?" && break
	done
	echo "=== the trust question"
	screen | grep -v '^ *$' | tail -14
	# (A question takes no key in its first moments: keys typed before it
	# was seen are not its answer.)
	sleep 2
	tmux send-keys -t "$S" Down
	sleep 1
	tmux send-keys -t "$S" Enter
	# (The prompt waits for the gnosh that starts in lib: typed while the
	# question is up, its letters would move the question's choice.)
	for i in $(seq 1 60); do
		sleep 1
		screen | grep -q "folder .*/lib" && break
	done
	sleep 2
	echo "=== after /cd"
	screen | grep -v '^ *$' | tail -8
	tmux send-keys -t "$S" "In one line: which folder are you working in now, and what is the codeword of this project? Then write the word moved into a file named here.txt in the working directory." Enter
	# (The first turn's end has scrolled out of the 80 lines screen reads
	# by then: the turn in lib is the one whose end follows the codeword.)
	done_when() { [ -s "$T/lib/here.txt" ] && screen | grep -A4 PERSIMMON | grep -q "done in"; }
	;;
bgsession)
	# The pane stays when gnosh ends, so that what it says last is seen.
	tmux set-option -t "$S" remain-on-exit on >/dev/null
	tmux send-keys -t "$S" "Reply with exactly the one word ready, and call no tool." Enter
	for i in $(seq 1 300); do
		sleep 1
		screen | grep -q "done in" && break
	done
	tmux send-keys -t "$S" "/background Reply with exactly these three words: all is well" Enter
	# (The log's first line holds the prompt: the answer is a line of its own.)
	done_when() { grep -qix "all is well" "$T"/data/gnosh/projects/*/*.background.log 2>/dev/null; }
	;;
keys)
	tmux send-keys -t "$S" "Reply with exactly these three words: all is well"
	sleep 1
	tmux send-keys -t "$S" C-k
	sleep 1
	echo "=== after Ctrl+K the box holds: $(screen | grep -c "Reply with exactly these three words") line(s) of the prompt"
	tmux send-keys -t "$S" C-y
	sleep 2
	tmux send-keys -t "$S" C-k
	sleep 1
	tmux send-keys -t "$S" Enter
	done_when() { screen | grep -q "Nothing has been asked of a model" && [ "$(screen | grep -c -i "all is well")" -ge 2 ] && screen | grep -q "done in"; }
	;;
suggest)
	tmux send-keys -t "$S" "What is the second line of notes.txt? Answer with that one word." Enter
	done_when() { screen | grep -q "(Tab takes it)"; }
	;;
invisible)
	# A zero-width space (UTF-8 e2 80 8b) inside a word, typed as text.
	tmux send-keys -t "$S" -l "$(printf 'Reply with exactly these three words: all is we\342\200\213ll')"
	tmux send-keys -t "$S" Enter
	done_when() { screen | grep -q "Took out 1 invisible character" && screen | grep -q "done in"; }
	;;
failhint)
	tmux send-keys -t "$S" "Reply with exactly these three words: all is well" Enter
	done_when() { screen | grep -q 'to try again'; }
	;;
cache)
	for w in one two three; do
		tmux send-keys -t "$S" "Reply with exactly the one word $w, and call no tool." Enter
		for i in $(seq 1 120); do
			sleep 1
			[ "$(screen | grep -c "done in")" -ge "$(case $w in one) echo 1;; two) echo 2;; three) echo 3;; esac)" ] && break
		done
	done
	tmux send-keys -t "$S" "/cost" Enter
	done_when() { screen | grep -q "cache    \|written  "; }
	;;
hookwake)
	tmux send-keys -t "$S" "Reply with exactly these three words: all is well" Enter
	done_when() { screen | grep -q "HOOK-WAKE" && [ "$(screen | grep -c "done in")" -ge 2 ]; }
	;;
btw)
	tmux send-keys -t "$S" "Run this exact shell command with the Bash tool and nothing else: sleep 40 && echo slept" Enter
	for i in $(seq 1 120); do
		sleep 1
		screen | grep -q "sleep 40" && break
	done
	sleep 3
	tmux send-keys -t "$S" "/btw what command is running right now?" Enter
	done_when() { screen | grep -A4 "/btw what command" | tail -n +2 | grep -q -i "sleep" && screen | grep -q "done in"; }
	;;
esac
for i in $(seq 1 300); do
	sleep 1
	if done_when; then
		sleep 2
		break
	fi
done
echo "=== after ($i s)"
screen
if [ "$scenario" = basic ]; then
	# The prompt and the answer with their colours, ESC shown as ^[.
	echo "=== colours:"
	tmux capture-pane -p -e -S -80 -t "$S" | grep -a -i "all is well" | cat -v | cut -c1-300
fi
if [ "$scenario" = leftover ]; then
	echo "=== sleep 321 before /exit: $(ps -eo args | grep -c '^sleep 321$')"
	tmux send-keys -t "$S" "/exit" Enter
	sleep 4
	echo "=== sleep 321 after /exit: $(ps -eo args | grep -c '^sleep 321$')"
fi
if [ "$scenario" = bgsession ]; then
	echo "=== the run's log: $(cat "$T"/data/gnosh/projects/*/*.background.log 2>/dev/null | tr '\n' ' ')"
	log=$(ls "$T"/data/gnosh/projects/*/*.background.log 2>/dev/null | head -1)
	id=$(basename "$log" .background.log)
	for i in $(seq 1 120); do
		$envs $bin session list 2>/dev/null | grep "$id" | grep -q "(open now)" || break
		sleep 1
	done
	tmux respawn-pane -k -t "$S" "$envs $bin -r $id"
	sleep 6
	echo "=== opened again with gnosh -r"
	screen | grep -v '^ *$' | tail -12
fi
if [ "$scenario" = suggest ]; then
	tmux send-keys -t "$S" Tab
	sleep 1
	echo "=== after Tab"
	screen | grep -v '^ *$' | tail -6
	tmux send-keys -t "$S" C-u
fi
if [ "$scenario" = config ]; then
	echo "=== state file: $(tr -d '\n' < "$T/data/gnosh/state.json" 2>/dev/null || echo none)"
fi
if [ "$scenario" = cd ]; then
	echo "=== here.txt in lib: $(cat "$T/lib/here.txt" 2>/dev/null || echo none); in work: $(cat "$T/work/here.txt" 2>/dev/null || echo none)"
	# (The projects' folders begin with a dash: ls is told where its
	# options end.)
	echo "=== the session files: $(cd "$T/data/gnosh/projects" && ls -d -- */*.jsonl 2>/dev/null | tr '\n' ' ')"
fi
if [ "$scenario" = worktree ]; then
	echo "=== notes.txt here: $(tr '\n' ' ' < "$T/work/notes.txt")"
	echo "=== work trees: $(git -C "$T/work" worktree list | tr '\n' ';')"
	for d in "$T"/work/.gnosh/worktrees/agent-*; do
		[ -d "$d" ] && echo "=== notes.txt in $d: $(tr '\n' ' ' < "$d/notes.txt")"
	done
fi
if [ "$scenario" = channel ]; then
	echo "=== replies: $(cat "$T/replies.log" 2>/dev/null || echo none)"
fi
if [ "$scenario" = entertree ]; then
	echo "=== work trees: $(git -C "$T/work" worktree list | tr '\n' ';')"
	echo "=== branches: $(git -C "$T/work" branch --list | tr '\n' ' ')"
	echo "=== the session files: $(cd "$T/data/gnosh/projects" && ls -d -- */*.jsonl 2>/dev/null | tr '\n' ' ')"
fi
if [ "$scenario" = wtflag ]; then
	echo "=== notes.txt here: $(tr '\n' ' ' < "$T/work/notes.txt")"
	echo "=== notes.txt in the work tree: $(tr '\n' ' ' < "$T/work/.gnosh/worktrees/demo/notes.txt" 2>/dev/null || echo none)"
	echo "=== .env in the work tree: $(cat "$T/work/.gnosh/worktrees/demo/.env" 2>/dev/null || echo none)"
	echo "=== branches: $(git -C "$T/work" branch --list | tr '\n' ' ')"
fi
if [ "$scenario" = plan ]; then
	echo "=== hello.txt: $(cat "$T/work/hello.txt" 2>/dev/null || echo none)"
	tmux send-keys -t "$S" Escape
	sleep 2
fi
if [ "$scenario" = model ] || [ "$scenario" = transcript ]; then
	# Esc keeps the model; q leaves the view, and the screen is as it was.
	if [ "$scenario" = model ]; then key=Escape; else key=q; fi
	tmux send-keys -t "$S" "$key"
	sleep 2
	echo "=== after $key"
	screen
fi
tmux send-keys -t "$S" "/exit" Enter
sleep 2
tmux kill-session -t "$S" 2>/dev/null
echo "=== done"
