// Package deliver is deliverables: the way the model names the thing a
// person asked for, the list of those things, and the small server that
// hands a file over as an address.
//
// gnosh usually runs on a machine its user is not sitting at, and the
// user does not follow the tool calls. What they want to know is where the
// thing they asked for is: the video, the report, the page. So the model
// has a tool for exactly that, Deliver, and the interface shows what it
// was told, apart from the files the agent merely changed. The package is
// one app.Feature, made by New and registered in the program's
// features.go. It follows spec/gnosh/harness-tools.md, section 7
// (TOOL-50 to TOOL-52), and spec/gnosh/harness-ui.md, UI-33.
//
// # The tool
//
// Deliver takes a label, a target and a host (tool.go).
//
//   - A label alone claims a deliverable: it is pending, and the person
//     sees what is being worked toward before the work is done.
//   - A label with a target delivers it. The target is a file, or an
//     address that begins http:// or https://, which is taken as given.
//     A file must exist and be a regular file that can be read: recording
//     a deliverable is proof that it exists, not one more claim.
//   - The same label again takes the place of the entry before, so the
//     list holds the latest version of each thing. Two labels that differ
//     only in letter case or in their blanks are one label.
//   - With host, the file is on a declared machine. It is looked for
//     there, and copied back, by the road a Bash call for that host takes
//     (shelltool.Manager.CopyFromHost): the system's ssh, with the
//     arguments and the rules of every command on a host, and nothing
//     else. A file larger than Options.MaxCopyBytes is recorded with its
//     path there and is not copied.
//
// A failure the model can act on is a failed result in plain words: what
// was wrong, and what to do next. What the model reads of a call that went
// well is that the label is claimed or recorded, the file or the address,
// and the address the file is served at when it is served.
//
// The tool is offered in a session a person attends, and to the session
// the person is in, never to a sub-agent. A run (gnosh -p) has nobody to
// show anything to: the feature adds nothing to it, and the part of the
// system prompt about deliverables goes with the tool
// (prompt.SystemInput).
//
// A call that only records asks nothing of the person, because it changes
// nothing: a claim, an address, a file in the project. What a call
// reaches beyond that is judged as the same reach is judged everywhere
// (the gate of internal/app), and the tool says what it is when it
// describes the call. A file of this machine is handed over, which is a
// reading of it: the tool names the file (core.ActionRead), so a Read
// rule that refuses a path refuses to deliver it, and a file outside the
// working directories asks as reading it does. A call with a host runs a
// command on that machine, says so (core.ActionExecute, with the very
// command), and is judged as that command on that host is.
//
// # The list
//
// The list is built from the calls recorded in the session's file and
// from nothing else (list.go): every result of a Deliver call that went
// well holds the entry as it then stood (app.MetaDeliverable), and the
// list is those entries, the latest of each label. So it is there again
// after a restart, with -c and with -r, in a branch of the session, and
// nothing extra is stored for it. While a session runs, the list follows
// the file: an entry joins it when its result is in the file
// (Contribution.AfterStored), not before.
//
// The feature hands the list to the session (Session.Deliverables), and
// the front ends do the rest: a line when a deliverable is claimed and a
// line when it is delivered, the counts on the status line, and the
// command /deliverables, which is also /artifacts.
//
// # Served at an address
//
// A path on the machine gnosh runs on is of little use from another one.
// So a delivered file is served over HTTP (server.go) at
//
//	http://127.0.0.1:7777/<token>/<file name>
//
// where the token is 32 bytes from crypto/rand, written in the 43
// characters of unpadded URL-safe base64. A file is found by its token
// alone: the request names no path, so no request can reach a file that
// was not delivered. There is no listing. Only GET and HEAD are answered,
// nothing is ever written, every answer says nosniff, and the media type
// comes from gnosh's own table (media.go), so that a video plays in the
// browser and an image shows.
//
// The server listens on this machine only. The option deliverablesServer
// moves it to another port of this machine or switches it off; it cannot
// be put on an address other machines reach. From another machine the
// person forwards the port (ssh -L). When the port is taken, a free one is
// used and the person is told which. The server is started when a session
// a person attends first has something to serve: at its start when the
// project already has delivered files, and otherwise at the first file
// that is delivered. A run never starts it.
//
// # The register
//
// Which token stands for which file is kept in the project's data folder
// (register.go), so that an address outlives the session that made it:
//
//	.gnosh/deliverables/register.json    the tokens
//	.gnosh/deliverables/files/           the copies of files from hosts
//
// A file on this machine is served from where it is. A file from a host
// is served from its copy. The same file delivered again keeps its token,
// so its address stays the same and shows the newer version.
//
// The project's data folder lies inside the project, where a repository
// that was cloned can bring files of its own. A register that names a
// file is a grant to serve it, so every entry carries a seal: an HMAC
// under a key that is kept in gnosh's own data directory, outside every
// project, and is made the first time it is needed. An entry whose seal
// does not fit was not written by this account's gnosh and is not served.
package deliver
