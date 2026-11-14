package config

// Paths are the directories and files gnosh uses for one run.
type Paths struct {
	// Home is the user's home directory, and Cwd the working directory.
	Home, Cwd string
	// ProjectRoot is where the project begins.
	ProjectRoot string

	// ConfigDir holds the user's own config: $GNOSH_CONFIG_DIR, else
	// $XDG_CONFIG_HOME/gnosh, else ~/.config/gnosh.
	ConfigDir string
	// StateDir holds what gnosh itself writes for the user: the state
	// file and the trust record. It is $GNOSH_DATA_DIR, else
	// $XDG_DATA_HOME/gnosh, else ~/.local/share/gnosh. No config file is
	// ever read from it.
	StateDir string
	// CacheDir is $XDG_CACHE_HOME/gnosh, else ~/.cache/gnosh.
	CacheDir string

	// UserSettings is the user's gnosh.json: where it is, whether or not
	// there is one yet.
	UserSettings string
	// StateFile is the file the choices made inside gnosh are kept in.
	StateFile string
	// TrustDir is where the record of trusted folders is kept.
	TrustDir string

	// ProjectSettings is the project's gnosh.json that was found in the
	// working directory, or "".
	ProjectSettings string
	// DataDir is the project's data folder: sessions, logs, mem. It is
	// the directory given with -D, else the closest folder of the
	// configured name (.gnosh, or the projectDataDir option) on the way up
	// to the project root, else a new one at the project root. Load does
	// not create it.
	DataDir string
	// LocalSettings is the settings file in the data folder, where
	// approvals a person saves are written.
	LocalSettings string
}
