package internal

type Configuration struct {
	LogLevel   string `flag:"loglevel"`
	Config     string
	Identifier string
	Registry   struct {
		Name string
		User string
		Pass string
	}
}
