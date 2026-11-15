The user asks for a status line, a command whose output gnosh shows under the prompt box. It is to show: {{wish}}

Write it as an executable script at {{script}}. It gets the session's facts as JSON on stdin, and its width as COLUMNS; the line it prints is the status line. This session's JSON is below: use only its fields. Try the script on it.

Then add "statusLine": {"type": "command", "command": "{{script}}"} to {{settings}} with the Edit tool, keeping the rest of the file (or write the file with that one setting). It shows from gnosh's next start.

{{sample}}
