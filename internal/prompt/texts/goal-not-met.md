The check of `{{name}}` found it not met: {{reason}}
The goal: {{condition}}
Do what is missing, then close it again: `mem.Done("{{name}}", "<command> → <result>")`. If what is missing is the user's to do or decide, do not do it for them: ask with `mem.Ask("{{name}}", "…")`. Either way, tell the user what the check found.
