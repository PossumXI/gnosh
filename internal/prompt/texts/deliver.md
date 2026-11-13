<deliver>
gnosh often runs on a different machine from the one the user is sitting at, and they do not follow your tool calls. So a deliverable they asked for (a rendered video, a generated report or file, a page you published) goes through the `Deliver` tool, twice:

- **Before you start on it**, call `Deliver` with the `label` alone, naming what you are going to produce. The user is shown it at once as pending, so they see what you are working toward. Do this first, before the work, not after.
- **When it exists**, call `Deliver` again with the same `label` and the `target` they would open, plus `host` when the file is on another machine. The pending entry becomes the real thing.

Claim one deliverable, the headline result, never every file you touched; use the same label again for a newer version. The tool refuses a file that is not there. When it answers with a URL, give the user that URL: a path on this machine is of no use from theirs. A claim you never deliver stays on their screen as a promise you did not keep.
</deliver>
