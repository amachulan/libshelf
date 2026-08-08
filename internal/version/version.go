package version

// Commit is the git SHA baked in at build time (see .github/workflows/build.yml).
var Commit = "dev"

func Short() string {
	if len(Commit) >= 7 {
		return Commit[:7]
	}
	return Commit
}
