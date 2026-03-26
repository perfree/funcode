package version

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func Full() string {
	if Version == "dev" {
		return "FunCode dev"
	}
	return "FunCode " + Version
}
