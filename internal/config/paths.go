package config

// Paths are the directories and files gnosh uses for one run.
type Paths struct {
	// Home is the user's home directory, and Cwd the working directory.
	Home, Cwd string
	// ProjectRoot is where the project begins.
	ProjectRoot string

	// ConfigDir holds the user's own config: $GNOSH_GLOBAL_CONFIG, else
	// $XDG_CONFIG_HOME/gnosh, else ~/.config/gnosh.
	ConfigDir string
	// StateDir holds what gnosh itself writes for the user: the state
	// file and the trust record. It is $GNOSH_GLOBAL_DATA, else
	// $XDG_DATA_HOME/gnosh, else ~/.local/share/gnosh. No config file is
	// ever read from it.
	StateDir string
	// CacheDir is $XDG_CACHE_HOME/gnosh, else ~/.cache/gnosh.
	CacheDir string

	// UserRC is the user's gnoshrc that was read, or "" when there is
	// none. It can be the file's old name (see CFG-5).
	UserRC string
	// UserSettings is where the user's gnosh.json is looked for.
	UserSettings string
	// StateFile is the file the choices made inside gnosh are kept in.
	StateFile string
	// TrustDir is where the record of trusted folders is kept.
	TrustDir string

	// ProjectRC and ProjectSettings are the project's gnoshrc and
	// gnosh.json that were found in the working directory, or "".
	ProjectRC, ProjectSettings string
	// DataDir is the project's data folder: sessions, logs, mem. It is
	// the directory given with -D, else the closest folder of the
	// configured name (.gnosh) on the way up to the project root, else a
	// new one at the project root. Load does not create it.
	DataDir string
	// LocalSettings is the settings file in the data folder, where
	// approvals a person saves are written.
	LocalSettings string
}
