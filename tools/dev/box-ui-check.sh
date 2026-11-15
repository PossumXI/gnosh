#!/bin/bash
# An interactive check of the installed gnosh on an AI box, with the box's
# model (Qwen alone: the router holds one model), in a tmux session of its
# own and a scratch home: the screen before and after one short turn, with
# a status line command configured. It touches no file of the person's,
# and ends its own tmux session by name. Run it on the box:
#
#	ssh ai-remote bash -s < tools/dev/box-ui-check.sh
#
# The full interface is tested in CI with a scripted model; this is the
# one look at it with a real one, which is how a model writing gnosh's
# tags into its own reply was found.
set -u
T=$HOME/newgnosh-test/uicheck
S=gnosh-uicheck
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
cat > "$T/config/gnosh/gnosh.json" <<EOF
{
  "providers": { "qwen": { "type": "llamacpp", "baseUrl": "http://127.0.0.1:8080", "listModels": false } },
  "models": { "qwen/qwen3.8-27b": { "name": "Qwen 3.8 27B", "contextWindow": 262144 } },
  "model": { "id": "qwen/qwen3.8-27b", "thinking": false },
  "options": { "requestTimeout": 900 },
  "statusLine": { "type": "command", "command": "$T/config/gnosh/statusline.sh", "padding": 1 }
}
EOF
cd "$T/work" || exit 1
tmux new-session -d -s "$S" -x 110 -y 28 "env -i HOME=$T/home XDG_CONFIG_HOME=$T/config XDG_DATA_HOME=$T/data XDG_CACHE_HOME=$T/cache PATH=/usr/local/bin:/usr/bin:/bin TERM=tmux-256color LANG=C.UTF-8 $HOME/.local/bin/gnosh"
sleep 4
echo "=== before"
tmux capture-pane -p -t "$S"
tmux send-keys -t "$S" "Reply with exactly these three words: all is well" Enter
for i in $(seq 1 120); do
	sleep 1
	if [ "$(tmux capture-pane -p -t "$S" | grep -c -i "all is well")" -ge 2 ] && tmux capture-pane -p -t "$S" | grep -q "ctx=[0-9]"; then
		sleep 2
		break
	fi
done
echo "=== after ($i s)"
tmux capture-pane -p -t "$S"
tmux send-keys -t "$S" "/exit" Enter
sleep 2
tmux kill-session -t "$S" 2>/dev/null
echo "=== done"
