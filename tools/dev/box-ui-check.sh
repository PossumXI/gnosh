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
case "$scenario" in
basic | mention | config | fork) ;;
agent) flags="--agent Explore" ;;
plan) flags="--permission-mode plan" ;;
background | btw) flags="-y" ;;
hookwake) hooks="\"hooks\": { \"Stop\": [ { \"hooks\": [ { \"type\": \"command\", \"asyncRewake\": true, \"command\": \"$T/config/gnosh/wake.sh\" } ] } ] }," ;;
*)
	echo "box-ui-check: no scenario $scenario (basic, plan, background, btw, mention, config, hookwake, agent, fork)" >&2
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
  "providers": { "qwen": { "type": "llamacpp", "baseUrl": "http://127.0.0.1:8080", "listModels": false } },
  "models": { "qwen/qwen3.8-27b": { "name": "Qwen 3.8 27B", "contextWindow": 262144 } },
  "model": { "id": "qwen/qwen3.8-27b", "thinking": false },
  "options": { "requestTimeout": 900 },
  $hooks
  "statusLine": { "type": "command", "command": "$T/config/gnosh/statusline.sh", "padding": 1 }
}
EOF
printf 'alpha\nbeta\ngamma\n' > "$T/work/notes.txt"
mkdir -p "$T/config/gnosh/skills/countlines"
cat > "$T/config/gnosh/skills/countlines/SKILL.md" <<'EOF'
---
description: Counts the lines of a file in a sub-agent.
context: fork
---
Count the lines of the file notes.txt in the working directory. Report the count as one number.
EOF
cd "$T/work" || exit 1
tmux new-session -d -s "$S" -x 110 -y 40 "env -i HOME=$T/home XDG_CONFIG_HOME=$T/config XDG_DATA_HOME=$T/data XDG_CACHE_HOME=$T/cache PATH=/usr/local/bin:/usr/bin:/bin TERM=tmux-256color LANG=C.UTF-8 $HOME/.local/bin/gnosh $flags"
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
mention)
	tmux send-keys -t "$S" "What is the second line of @notes.txt? Answer with that one word and call no tool." Enter
	done_when() { screen | grep -q "@notes.txt read" && screen | grep -q "done in"; }
	;;
config)
	tmux send-keys -t "$S" "/config" Enter
	sleep 2
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
if [ "$scenario" = config ]; then
	echo "=== state file: $(tr -d '\n' < "$T/data/gnosh/state.json" 2>/dev/null || echo none)"
fi
if [ "$scenario" = plan ]; then
	echo "=== hello.txt: $(cat "$T/work/hello.txt" 2>/dev/null || echo none)"
	tmux send-keys -t "$S" Escape
	sleep 2
fi
tmux send-keys -t "$S" "/exit" Enter
sleep 2
tmux kill-session -t "$S" 2>/dev/null
echo "=== done"
